package conversion

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

// fakeIdentity emulates identity's upsert semantics: repeating a call for the
// same external reference returns the same id instead of creating a duplicate.
type fakeIdentity struct {
	guardians map[string]string
	students  map[string]string
	links     map[string]string

	guardianCalls, studentCalls, linkCalls int

	failGuardian, failStudent, failLink error
}

func newFakeIdentity() *fakeIdentity {
	return &fakeIdentity{
		guardians: map[string]string{},
		students:  map[string]string{},
		links:     map[string]string{},
	}
}

func (f *fakeIdentity) UpsertGuardian(_ context.Context, r Record) (string, error) {
	f.guardianCalls++
	if f.failGuardian != nil {
		err := f.failGuardian
		f.failGuardian = nil
		return "", err
	}
	if id, ok := f.guardians[r.GuardianRef]; ok {
		return id, nil
	}
	id := "guardian-" + strconv.Itoa(len(f.guardians)+1)
	f.guardians[r.GuardianRef] = id
	return id, nil
}

func (f *fakeIdentity) UpsertStudent(_ context.Context, r Record) (string, error) {
	f.studentCalls++
	if f.failStudent != nil {
		err := f.failStudent
		f.failStudent = nil
		return "", err
	}
	if id, ok := f.students[r.StudentRef]; ok {
		return id, nil
	}
	id := "student-" + strconv.Itoa(len(f.students)+1)
	f.students[r.StudentRef] = id
	return id, nil
}

func (f *fakeIdentity) LinkGuardian(_ context.Context, r Record) (string, error) {
	f.linkCalls++
	if f.failLink != nil {
		err := f.failLink
		f.failLink = nil
		return "", err
	}
	key := r.GuardianID + ":" + r.StudentID
	if id, ok := f.links[key]; ok {
		return id, nil
	}
	id := "link-" + strconv.Itoa(len(f.links)+1)
	f.links[key] = id
	return id, nil
}

type fakeAcademic struct {
	id    string
	err   error
	calls int
}

func (f *fakeAcademic) Enroll(_ context.Context, _ Record) (string, error) {
	f.calls++
	return f.id, f.err
}

func baseRecord() Record {
	return Record{
		ID:            "conv-1",
		SchoolID:      "school-1",
		ApplicationID: "app-1",
		Key:           "idem-1",
		ConversionRef: "PPDB-2026-0001",
		GuardianRef:   "guardian:g1",
		StudentRef:    "applicant:a1",
		GuardianName:  "Ibu Sari",
		ApplicantName: "Andi",
		ApplicantNISN: "0012345678",
		Steps:         map[string]*StepState{},
	}
}

func TestSagaHappyPath(t *testing.T) {
	identity := newFakeIdentity()
	academic := &fakeAcademic{id: "enrollment-1"}
	var persisted []Record
	persist := func(_ context.Context, r Record) error {
		persisted = append(persisted, r)
		return nil
	}

	rec, err := Run(context.Background(), baseRecord(), identity, academic, persist, true)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if rec.Status != StatusCompleted {
		t.Fatalf("status=%s want completed", rec.Status)
	}
	if rec.GuardianID == "" || rec.StudentID == "" || rec.LinkID == "" || rec.EnrollmentID == "" {
		t.Fatalf("missing ids: %+v", rec)
	}
	if rec.Step(StepGuardian).Status != StepDone || rec.Step(StepEnrollment).Status != StepDone {
		t.Fatalf("steps not done: %+v", rec.Steps)
	}
	if identity.guardianCalls != 1 || identity.studentCalls != 1 || identity.linkCalls != 1 {
		t.Fatalf("call counts g=%d s=%d l=%d", identity.guardianCalls, identity.studentCalls, identity.linkCalls)
	}
	if len(persisted) == 0 {
		t.Fatal("persist must be called at least once")
	}
}

func TestSagaResumeDoesNotDuplicate(t *testing.T) {
	identity := newFakeIdentity()
	identity.failLink = errors.New("identity unavailable")
	academic := &fakeAcademic{id: "enrollment-1"}
	persist := func(context.Context, Record) error { return nil }

	// First attempt: link step fails after guardian+student succeed.
	first, err := Run(context.Background(), baseRecord(), identity, academic, persist, false)
	if err == nil {
		t.Fatal("expected the first run to fail at the link step")
	}
	if first.Status != StatusFailed {
		t.Fatalf("status=%s want failed", first.Status)
	}
	if first.GuardianID == "" || first.StudentID == "" {
		t.Fatalf("guardian/student should have been persisted before the failure: %+v", first)
	}
	if identity.guardianCalls != 1 || identity.studentCalls != 1 {
		t.Fatalf("unexpected early calls g=%d s=%d", identity.guardianCalls, identity.studentCalls)
	}

	// Resume from the persisted record: the completed steps must be skipped.
	second, err := Run(context.Background(), first, identity, academic, persist, false)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if second.Status != StatusCompleted {
		t.Fatalf("resume status=%s want completed", second.Status)
	}
	if identity.guardianCalls != 1 || identity.studentCalls != 1 {
		t.Fatalf("resume re-created upstream users: g=%d s=%d", identity.guardianCalls, identity.studentCalls)
	}
	if identity.linkCalls != 2 {
		t.Fatalf("link should have been attempted twice, got %d", identity.linkCalls)
	}
	if len(identity.guardians) != 1 || len(identity.students) != 1 || len(identity.links) != 1 {
		t.Fatalf("duplicates created: guardians=%d students=%d links=%d",
			len(identity.guardians), len(identity.students), len(identity.links))
	}
}

func TestSagaSkipsCompletedSteps(t *testing.T) {
	identity := newFakeIdentity()
	record := baseRecord()
	record.GuardianID = "guardian-existing"
	record.Steps[StepGuardian] = &StepState{Status: StepDone, Ref: "guardian-existing"}

	persist := func(context.Context, Record) error { return nil }
	if _, err := Run(context.Background(), record, identity, &fakeAcademic{}, persist, false); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if identity.guardianCalls != 0 {
		t.Fatalf("completed guardian step should be skipped, calls=%d", identity.guardianCalls)
	}
}

func TestSagaToleratesMissingEnrollmentEndpoint(t *testing.T) {
	identity := newFakeIdentity()
	academic := &fakeAcademic{err: ErrNotAvailable}
	persist := func(context.Context, Record) error { return nil }

	rec, err := Run(context.Background(), baseRecord(), identity, academic, persist, true)
	if err != nil {
		t.Fatalf("a missing enrollment endpoint must not fail the saga: %v", err)
	}
	if rec.Status != StatusCompleted {
		t.Fatalf("status=%s want completed", rec.Status)
	}
	if rec.EnrollmentID != "" {
		t.Fatalf("enrollment id should stay empty, got %q", rec.EnrollmentID)
	}
	if rec.Step(StepEnrollment).Status != StepSkipped {
		t.Fatalf("enrollment step=%s want skipped", rec.Step(StepEnrollment).Status)
	}
}

func TestSagaHardEnrollmentFailureStops(t *testing.T) {
	identity := newFakeIdentity()
	academic := &fakeAcademic{err: errors.New("enrollment rejected")}
	persist := func(context.Context, Record) error { return nil }

	rec, err := Run(context.Background(), baseRecord(), identity, academic, persist, true)
	if err == nil {
		t.Fatal("a real enrollment error must fail the saga")
	}
	if rec.Status != StatusFailed {
		t.Fatalf("status=%s want failed", rec.Status)
	}
	// The identity steps up to the link must still have completed.
	if rec.GuardianID == "" || rec.StudentID == "" || rec.LinkID == "" {
		t.Fatalf("identity steps should be preserved: %+v", rec)
	}
}
