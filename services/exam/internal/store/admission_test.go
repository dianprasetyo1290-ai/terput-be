package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
)

// admissionTestDB matches the convention store/exams_test.go already uses:
// run against EXAM_TEST_DATABASE_URL, else the docker-compose dev Postgres on
// :5480, and skip (never fail) when no database is reachable.
func admissionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("EXAM_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://sekoci:change-me@localhost:5480/skci_exam?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("skipping: could not open %s: %v", dsn, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("skipping: dev postgres not reachable at %s: %v", dsn, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testUUID(t *testing.T, ctx context.Context, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRowContext(ctx, "SELECT gen_random_uuid()").Scan(&id); err != nil {
		t.Fatalf("gen_random_uuid: %v", err)
	}
	return id
}

// createAdmissionExam provisions a real admission exam the way
// InternalUpsertAdmissionExam does, and registers cleanup.
func createAdmissionExam(t *testing.T, ctx context.Context, s *Store, schoolID, extRef string) *model.Exam {
	t.Helper()
	now := time.Now()
	e, err := s.UpsertAdmissionExam(ctx, AdmissionExamInput{
		SchoolID: schoolID, ExternalRef: extRef, Title: "Tes Seleksi " + extRef,
		StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
		// CreatedBy is a UUID column; InternalUpsertAdmissionExam passes the
		// calling school's id here.
		CreatedBy:        schoolID,
		AdmissionContext: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("UpsertAdmissionExam: %v", err)
	}
	t.Cleanup(func() {
		if _, err := s.DB.ExecContext(context.Background(), `DELETE FROM exams WHERE id=$1`, e.ID); err != nil {
			t.Logf("cleanup admission exam %s: %v", e.ID, err)
		}
	})
	return e
}

// createAcademicExam inserts a legacy-shaped (audience defaults to 'academic')
// exam and returns its id. It exists to prove applicant operations can never
// reach it, and that student flows still work on it.
func createAcademicExam(t *testing.T, ctx context.Context, db *sql.DB, schoolID string) string {
	t.Helper()
	academicYearID, termID := testUUID(t, ctx, db), testUUID(t, ctx, db)
	classroomID, subjectID, teacherID := testUUID(t, ctx, db), testUUID(t, ctx, db), testUUID(t, ctx, db)

	var examTypeID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO exam_types (school_id, academic_year_id, name, mapping_rapor, created_by)
		VALUES ($1,$2,'admission-isolation type','sumatif',$3) RETURNING id`,
		schoolID, academicYearID, teacherID).Scan(&examTypeID); err != nil {
		t.Fatalf("insert exam_type: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM exam_types WHERE id=$1`, examTypeID)
	})

	now := time.Now()
	var examID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO exams (school_id, academic_year_id, term_id, exam_type_id, title, classroom_id, subject_id,
			teacher_id, start_at, end_at, is_published, created_by)
		VALUES ($1,$2,$3,$4,'Sumatif akademik',$5,$6,$7,$8,$9,true,$7) RETURNING id`,
		schoolID, academicYearID, termID, examTypeID, classroomID, subjectID, teacherID, now.Add(-time.Hour), now.Add(time.Hour)).
		Scan(&examID); err != nil {
		t.Fatalf("insert academic exam: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM exams WHERE id=$1`, examID)
	})
	return examID
}

func insertStudentAttempt(t *testing.T, ctx context.Context, db *sql.DB, schoolID, examID, studentID string, score float64) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO exam_attempts (school_id, academic_year_id, exam_id, participant_type, participant_id,
			user_id, classroom_id, status, score, submitted_at)
		VALUES ($1,$2,$3,'student',$4,$4,$5,'selesai',$6,now())`,
		schoolID, testUUID(t, ctx, db), examID, studentID, testUUID(t, ctx, db), score); err != nil {
		t.Fatalf("insert student attempt: %v", err)
	}
}

func insertApplicantAttempt(t *testing.T, ctx context.Context, db *sql.DB, schoolID, examID, applicantID string, score float64) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO exam_attempts (school_id, exam_id, participant_type, participant_id, status, score, submitted_at)
		VALUES ($1,$2,'applicant',$3,'selesai',$4,now())`, schoolID, examID, applicantID, score); err != nil {
		t.Fatalf("insert applicant attempt: %v", err)
	}
}

// TestUpsertAdmissionExam_IdempotentAndAcademicFieldFree covers both halves of
// the admission exam contract: it upserts on (school, external_reference) so a
// retried provisioning call is a no-op, and it never needs a
// classroom/subject/term/academic year — those columns are NULL, which
// migration 0008's exams_audience_context_check requires for audience
// 'admission'. It also reads the row back through GetExam, which is the code
// path that used to blow up ("converting NULL to string is unsupported") on
// exactly those NULL columns.
func TestUpsertAdmissionExam_IdempotentAndAcademicFieldFree(t *testing.T) {
	db := admissionTestDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID := testUUID(t, ctx, db)

	extRef := "REG-2026-" + testUUID(t, ctx, db)[:8]
	now := time.Now()
	in := AdmissionExamInput{
		SchoolID: schoolID, ExternalRef: extRef, Title: "Seleksi Gelombang 1",
		StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
		CreatedBy:        schoolID,
		AdmissionContext: map[string]interface{}{"gelombang": "1"},
	}
	first, err := s.UpsertAdmissionExam(ctx, in)
	if err != nil {
		t.Fatalf("UpsertAdmissionExam (first): %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM exams WHERE id=$1`, first.ID) })

	if first.Audience != model.AudienceAdmission {
		t.Errorf("audience = %q, want %q", first.Audience, model.AudienceAdmission)
	}
	// The external reference is what makes the upsert idempotent, so an upsert
	// without one is refused rather than silently creating an unidentifiable
	// exam row.
	if _, err := s.UpsertAdmissionExam(ctx, AdmissionExamInput{SchoolID: schoolID, Title: "tanpa external reference"}); !errors.Is(err, ErrAdmissionExternalRefRequired) {
		t.Errorf("upsert without external_reference: err = %v, want ErrAdmissionExternalRefRequired", err)
	}

	in.Title = "Seleksi Gelombang 1 (revisi)"
	second, err := s.UpsertAdmissionExam(ctx, in)
	if err != nil {
		t.Fatalf("UpsertAdmissionExam (retry): %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("retry created a second exam (%s != %s): upsert is not idempotent", second.ID, first.ID)
	}
	if second.Title != "Seleksi Gelombang 1 (revisi)" {
		t.Errorf("retry did not update the title, got %q", second.Title)
	}

	var examCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM exams WHERE school_id=$1 AND audience='admission'
		AND admission_context->>'external_reference'=$2`, schoolID, extRef).Scan(&examCount); err != nil {
		t.Fatalf("count admission exams: %v", err)
	}
	if examCount != 1 {
		t.Errorf("admission exams for external_reference %q = %d, want 1", extRef, examCount)
	}

	var academicYearID, termID, classroomID, subjectID sql.NullString
	if err := db.QueryRowContext(ctx, `
		SELECT academic_year_id, term_id, classroom_id, subject_id FROM exams WHERE id=$1`, first.ID).
		Scan(&academicYearID, &termID, &classroomID, &subjectID); err != nil {
		t.Fatalf("read admission exam columns: %v", err)
	}
	for name, v := range map[string]sql.NullString{
		"academic_year_id": academicYearID, "term_id": termID,
		"classroom_id": classroomID, "subject_id": subjectID,
	} {
		if v.Valid {
			t.Errorf("%s = %q, want NULL: an admission exam must not claim academic ownership", name, v.String)
		}
	}

	got, err := s.GetExam(ctx, first.ID, schoolID)
	if err != nil {
		t.Fatalf("GetExam(admission exam): %v", err)
	}
	if got == nil {
		t.Fatal("GetExam returned nil for an existing admission exam")
	}
	if got.Audience != model.AudienceAdmission {
		t.Errorf("GetExam audience = %q, want %q", got.Audience, model.AudienceAdmission)
	}
	if got.AcademicYearID != "" || got.TermID != "" || got.ClassroomID != "" || got.SubjectID != "" {
		t.Errorf("GetExam leaked academic context %+v for an admission exam", got)
	}
	if got.AdmissionContext == nil {
		t.Error("GetExam did not return admission_context")
	}

	listed, _, err := s.ListExams(ctx, ExamFilter{SchoolID: schoolID, Title: in.Title}, 1, 50)
	if err != nil {
		t.Fatalf("ListExams: %v", err)
	}
	var seen bool
	for _, e := range listed {
		if e.ID == first.ID {
			seen = true
		}
	}
	if !seen {
		t.Error("ListExams did not return the admission exam (NULL academic columns must not break the list scan)")
	}
}

// TestProvisionApplicant_Idempotent is the provisioning idempotency check:
// admission retries (network timeout, worker restart) must converge on ONE
// participant row, and a retry must never blank out a display name or external
// reference a previous call recorded. It also pins the school/audience guard —
// an applicant can only be attached to an admission exam in the same school.
func TestProvisionApplicant_Idempotent(t *testing.T) {
	db := admissionTestDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID, otherSchoolID := testUUID(t, ctx, db), testUUID(t, ctx, db)
	exam := createAdmissionExam(t, ctx, s, schoolID, "REG-PROV-"+testUUID(t, ctx, db)[:8])
	applicantID := testUUID(t, ctx, db)

	display := "Budi Santoso"
	first, err := s.ProvisionApplicant(ctx, schoolID, exam.ID, applicantID, "REG-001", &display)
	if err != nil {
		t.Fatalf("ProvisionApplicant (first): %v", err)
	}
	if first.ParticipantType != model.ParticipantApplicant {
		t.Errorf("participant_type = %q, want %q", first.ParticipantType, model.ParticipantApplicant)
	}
	if first.ExternalReference == nil || *first.ExternalReference != "REG-001" {
		t.Errorf("external_reference = %v, want REG-001", first.ExternalReference)
	}

	// Retry with nothing: same row, nothing lost.
	second, err := s.ProvisionApplicant(ctx, schoolID, exam.ID, applicantID, "", nil)
	if err != nil {
		t.Fatalf("ProvisionApplicant (retry): %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("retry created a new participant row %s (want %s)", second.ID, first.ID)
	}
	if second.DisplayName == nil || *second.DisplayName != display {
		t.Errorf("retry blanked display_name: %v", second.DisplayName)
	}
	if second.ExternalReference == nil || *second.ExternalReference != "REG-001" {
		t.Errorf("retry blanked external_reference: %v", second.ExternalReference)
	}

	// Retry with corrected data: still one row, updated in place.
	updated := "Budi Santoso, S.Kom"
	third, err := s.ProvisionApplicant(ctx, schoolID, exam.ID, applicantID, "", &updated)
	if err != nil {
		t.Fatalf("ProvisionApplicant (update): %v", err)
	}
	if third.ID != first.ID || third.DisplayName == nil || *third.DisplayName != updated {
		t.Errorf("update did not reuse the row: id=%s display_name=%v", third.ID, third.DisplayName)
	}

	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM exam_participants
		WHERE exam_id=$1 AND participant_type='applicant' AND participant_id=$2`, exam.ID, applicantID).Scan(&count); err != nil {
		t.Fatalf("count participants: %v", err)
	}
	if count != 1 {
		t.Errorf("participant rows = %d, want exactly 1 (provisioning must be idempotent)", count)
	}

	// Unknown exam id -> a typed "not found" so the handler answers 404.
	if _, err := s.ProvisionApplicant(ctx, schoolID, testUUID(t, ctx, db), applicantID, "", nil); !errors.Is(err, ErrAdmissionExamNotFound) {
		t.Errorf("provisioning against an unknown exam: err = %v, want ErrAdmissionExamNotFound", err)
	}
	// Another school's admission exam id -> same 404, never a cross-tenant row.
	if _, err := s.ProvisionApplicant(ctx, otherSchoolID, exam.ID, applicantID, "", nil); !errors.Is(err, ErrAdmissionExamNotFound) {
		t.Errorf("provisioning across schools: err = %v, want ErrAdmissionExamNotFound", err)
	}
	// An ACADEMIC exam must never accept an applicant.
	academicExamID := createAcademicExam(t, ctx, db, schoolID)
	if _, err := s.ProvisionApplicant(ctx, schoolID, academicExamID, applicantID, "", nil); !errors.Is(err, ErrAdmissionExamNotFound) {
		t.Errorf("provisioning an applicant onto an academic exam: err = %v, want ErrAdmissionExamNotFound", err)
	}
}

// TestParticipantAccessToken_HashedStoredAndValidated pins the token storage
// contract: only a hash is persisted (a DB dump must not yield a usable
// credential), and validation requires this service's own unrevoked, unexpired
// row — a correctly-signed token that this service never issued, or that has
// expired/been revoked, is refused.
func TestParticipantAccessToken_HashedStoredAndValidated(t *testing.T) {
	db := admissionTestDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID := testUUID(t, ctx, db)
	exam := createAdmissionExam(t, ctx, s, schoolID, "REG-TOKEN-"+testUUID(t, ctx, db)[:8])
	applicantID := testUUID(t, ctx, db)
	if _, err := s.ProvisionApplicant(ctx, schoolID, exam.ID, applicantID, "", nil); err != nil {
		t.Fatalf("ProvisionApplicant: %v", err)
	}

	raw := "eyJhbGciOiJIUzI1NiJ9.test-participant-token"
	hash := common.TokenHash(raw)
	if err := s.SaveParticipantToken(ctx, hash, schoolID, exam.ID, applicantID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveParticipantToken: %v", err)
	}
	// Re-issuing the identical token must be a no-op, not a unique violation.
	if err := s.SaveParticipantToken(ctx, hash, schoolID, exam.ID, applicantID, time.Now().Add(time.Hour)); err != nil {
		t.Errorf("re-saving the same token hash failed: %v", err)
	}

	var stored []string
	rows, err := db.QueryContext(ctx, `SELECT token_hash FROM participant_access_tokens WHERE school_id=$1 AND participant_id=$2`, schoolID, applicantID)
	if err != nil {
		t.Fatalf("read participant_access_tokens: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			t.Fatalf("scan token_hash: %v", err)
		}
		stored = append(stored, h)
	}
	if len(stored) != 1 {
		t.Fatalf("stored token rows = %d, want 1", len(stored))
	}
	if stored[0] != hash {
		t.Errorf("stored token_hash = %q, want the sha256 of the token", stored[0])
	}
	if stored[0] == raw || strings.Contains(stored[0], raw) {
		t.Error("the raw participant token was stored: a database read would hand out a working credential")
	}

	ok, err := s.ValidateParticipantToken(ctx, hash, schoolID, exam.ID, applicantID)
	if err != nil || !ok {
		t.Fatalf("ValidateParticipantToken(issued) = %v, %v; want true, nil", ok, err)
	}
	for _, c := range []struct {
		name                              string
		hash, school, examID, applicantID string
	}{
		{"unissued token", common.TokenHash("some-other-token"), schoolID, exam.ID, applicantID},
		{"other applicant", hash, schoolID, exam.ID, testUUID(t, ctx, db)},
		{"other exam", hash, schoolID, testUUID(t, ctx, db), applicantID},
		{"other school", hash, testUUID(t, ctx, db), exam.ID, applicantID},
	} {
		got, err := s.ValidateParticipantToken(ctx, c.hash, c.school, c.examID, c.applicantID)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got {
			t.Errorf("%s was accepted", c.name)
		}
	}

	// Expired token: signed correctly, but no longer live.
	expiredRaw := raw + "-expired"
	expiredHash := common.TokenHash(expiredRaw)
	if err := s.SaveParticipantToken(ctx, expiredHash, schoolID, exam.ID, applicantID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("SaveParticipantToken (expired): %v", err)
	}
	if ok, err := s.ValidateParticipantToken(ctx, expiredHash, schoolID, exam.ID, applicantID); err != nil || ok {
		t.Errorf("expired token: ValidateParticipantToken = %v, %v; want false, nil", ok, err)
	}

	// Revoked token: still unexpired, explicitly revoked.
	if _, err := db.ExecContext(ctx, `UPDATE participant_access_tokens SET revoked_at=now() WHERE token_hash=$1`, hash); err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	if ok, err := s.ValidateParticipantToken(ctx, hash, schoolID, exam.ID, applicantID); err != nil || ok {
		t.Errorf("revoked token: ValidateParticipantToken = %v, %v; want false, nil", ok, err)
	}
}

// TestAdmissionResult_ApplicantOnly: the admission result contract must return
// an APPLICANT attempt on an ADMISSION exam and nothing else. A student id, a
// student attempt, or an applicant attempt sitting on an academic exam all
// have to come back nil.
func TestAdmissionResult_ApplicantOnly(t *testing.T) {
	db := admissionTestDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID := testUUID(t, ctx, db)
	exam := createAdmissionExam(t, ctx, s, schoolID, "REG-RESULT-"+testUUID(t, ctx, db)[:8])
	applicantID, studentID := testUUID(t, ctx, db), testUUID(t, ctx, db)
	insertApplicantAttempt(t, ctx, db, schoolID, exam.ID, applicantID, 88)

	res, err := s.AdmissionResult(ctx, schoolID, exam.ID, applicantID)
	if err != nil {
		t.Fatalf("AdmissionResult(applicant): %v", err)
	}
	if res == nil {
		t.Fatal("AdmissionResult returned nil for an applicant who submitted")
	}
	if res.ParticipantType != model.ParticipantApplicant {
		t.Errorf("participant_type = %q, want %q", res.ParticipantType, model.ParticipantApplicant)
	}
	if res.Score == nil || *res.Score != 88 {
		t.Errorf("score = %v, want 88", res.Score)
	}
	if !res.Final {
		t.Error("final = false for a 'selesai' attempt")
	}

	// A student id has no applicant attempt here.
	if res, err := s.AdmissionResult(ctx, schoolID, exam.ID, studentID); err != nil || res != nil {
		t.Errorf("AdmissionResult(student on admission exam) = %+v, %v; want nil, nil", res, err)
	}

	// An applicant attempt on an ACADEMIC exam must not be readable as an
	// admission result — the join requires e.audience='admission'.
	academicExamID := createAcademicExam(t, ctx, db, schoolID)
	insertApplicantAttempt(t, ctx, db, schoolID, academicExamID, applicantID, 99)
	if res, err := s.AdmissionResult(ctx, schoolID, academicExamID, applicantID); err != nil || res != nil {
		t.Errorf("AdmissionResult(applicant on academic exam) = %+v, %v; want nil, nil", res, err)
	}

	// Another school must not read this school's result.
	if res, err := s.AdmissionResult(ctx, testUUID(t, ctx, db), exam.ID, applicantID); err != nil || res != nil {
		t.Errorf("AdmissionResult(foreign school) = %+v, %v; want nil, nil", res, err)
	}
}

// TestExamScoresAndMonitor_NeverIncludeApplicants is the leak check item 2 asks
// for. The report/score surface (GET /exams/{id}/scores, consumed by
// report-service for e-raport) and the live monitor must only ever see STUDENT
// attempts, even in the worst case where an applicant attempt exists on the
// very same academic exam.
func TestExamScoresAndMonitor_NeverIncludeApplicants(t *testing.T) {
	db := admissionTestDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID := testUUID(t, ctx, db)

	studentID, applicantID := testUUID(t, ctx, db), testUUID(t, ctx, db)
	academicExamID := createAcademicExam(t, ctx, db, schoolID)
	insertStudentAttempt(t, ctx, db, schoolID, academicExamID, studentID, 70)
	// Worst case: an applicant attempt on the SAME academic exam. It must stay
	// invisible to both the score report and the monitor.
	insertApplicantAttempt(t, ctx, db, schoolID, academicExamID, applicantID, 100)

	scores, err := s.ExamScores(ctx, academicExamID)
	if err != nil {
		t.Fatalf("ExamScores(academic): %v", err)
	}
	if len(scores) != 1 {
		t.Fatalf("ExamScores(academic) returned %d rows, want 1 (student only): %+v", len(scores), scores)
	}
	if scores[0].UserID != studentID {
		t.Errorf("ExamScores row user_id = %q, want the student %q", scores[0].UserID, studentID)
	}
	if scores[0].Score == nil || *scores[0].Score != 70 {
		t.Errorf("ExamScores returned the applicant's score for the student: %+v", scores[0])
	}

	monitor, err := s.ListAttemptsByExam(ctx, academicExamID)
	if err != nil {
		t.Fatalf("ListAttemptsByExam(academic): %v", err)
	}
	if len(monitor) != 1 || monitor[0].ParticipantType != model.ParticipantStudent || monitor[0].UserID != studentID {
		t.Errorf("monitor roster = %+v, want only the student attempt", monitor)
	}

	// And the mirror image: an admission exam has no student scores at all.
	admissionExam := createAdmissionExam(t, ctx, s, schoolID, "REG-ISO-"+testUUID(t, ctx, db)[:8])
	insertApplicantAttempt(t, ctx, db, schoolID, admissionExam.ID, applicantID, 95)
	scores, err = s.ExamScores(ctx, admissionExam.ID)
	if err != nil {
		t.Fatalf("ExamScores(admission): %v", err)
	}
	if len(scores) != 0 {
		t.Errorf("ExamScores(admission exam) leaked %d applicant rows into the academic report: %+v", len(scores), scores)
	}
	if attempts, err := s.ListAttemptsByExam(ctx, admissionExam.ID); err != nil || len(attempts) != 0 {
		t.Errorf("ListAttemptsByExam(admission exam) = %+v, %v; want empty", attempts, err)
	}
}

// TestParticipantTypeColumnIsAStrictUnion proves the DB itself bounds
// participant_type to the two legal values (7 or 9 characters, nothing else) —
// the same closed set model.ValidParticipantType enforces in Go. Any other
// value, however close to a real one, must be rejected by the CHECK constraint
// rather than stored and later treated as a real participant kind.
func TestParticipantTypeColumnIsAStrictUnion(t *testing.T) {
	db := admissionTestDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID := testUUID(t, ctx, db)
	exam := createAdmissionExam(t, ctx, s, schoolID, "REG-TYPE-"+testUUID(t, ctx, db)[:8])

	for _, bad := range []string{"applicantt", "applicants", "student ", " applicant", "Applicant", "app", ""} {
		_, err := db.ExecContext(ctx, `
			INSERT INTO exam_attempts (school_id, exam_id, participant_type, participant_id, status)
			VALUES ($1,$2,$3,$4,'belum_mulai')`, schoolID, exam.ID, bad, testUUID(t, ctx, db))
		if err == nil {
			t.Errorf("participant_type=%q was accepted; the union must be closed", bad)
			continue
		}
		if !strings.Contains(err.Error(), "participant_type") && !strings.Contains(err.Error(), "constraint") {
			t.Errorf("participant_type=%q rejected for the wrong reason: %v", bad, err)
		}
	}

	// The two legal values still insert (applicant with user_id NULL, student
	// with user_id = participant_id, per exam_attempts_typed_legacy_ref_check).
	for _, good := range []string{model.ParticipantApplicant} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO exam_attempts (school_id, exam_id, participant_type, participant_id, status)
			VALUES ($1,$2,$3,$4,'belum_mulai')`, schoolID, exam.ID, good, testUUID(t, ctx, db)); err != nil {
			t.Errorf("participant_type=%q should be accepted: %v", good, err)
		}
	}
}

// TestLegacyStudentRowsStillWork is the backward-compatibility check for
// existing students: pre-migration rows were backfilled to
// participant_type='student', participant_id=user_id (migration 0008), and that
// shape must keep working for every student-facing read — the score report,
// the monitor roster and the by-user attempt lookup.
func TestLegacyStudentRowsStillWork(t *testing.T) {
	db := admissionTestDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID := testUUID(t, ctx, db)
	studentID := testUUID(t, ctx, db)
	examID := createAcademicExam(t, ctx, db, schoolID)

	// Exactly the post-backfill shape of an old row.
	insertStudentAttempt(t, ctx, db, schoolID, examID, studentID, 81)

	var participantType, participantID string
	var userID sql.NullString
	if err := db.QueryRowContext(ctx, `
		SELECT participant_type, participant_id, user_id FROM exam_attempts
		WHERE exam_id=$1 AND user_id=$2`, examID, studentID).Scan(&participantType, &participantID, &userID); err != nil {
		t.Fatalf("read legacy attempt: %v", err)
	}
	if participantType != model.ParticipantStudent || participantID != studentID || !userID.Valid || userID.String != studentID {
		t.Fatalf("unexpected stored shape: type=%q participant_id=%q user_id=%v", participantType, participantID, userID)
	}

	scores, err := s.ExamScores(ctx, examID)
	if err != nil {
		t.Fatalf("ExamScores: %v", err)
	}
	if len(scores) != 1 || scores[0].UserID != studentID {
		t.Fatalf("legacy student row missing from the score report: %+v", scores)
	}

	attempt, err := s.GetAttempt(ctx, examID, studentID)
	if err != nil {
		t.Fatalf("GetAttempt (legacy by user_id): %v", err)
	}
	if attempt == nil || attempt.ParticipantType != model.ParticipantStudent || attempt.UserID != studentID {
		t.Fatalf("GetAttempt did not return the legacy student attempt: %+v", attempt)
	}

	byID, err := s.GetAttemptByID(ctx, attempt.ID, schoolID)
	if err != nil || byID == nil {
		t.Fatalf("GetAttemptByID: %+v, %v", byID, err)
	}

	roster, err := s.ListAttemptsByExam(ctx, examID)
	if err != nil {
		t.Fatalf("ListAttemptsByExam: %v", err)
	}
	if len(roster) != 1 || roster[0].UserID != studentID {
		t.Fatalf("legacy student missing from the monitor roster: %+v", roster)
	}

	// And it must still be invisible to the admission contract.
	if res, err := s.AdmissionResult(ctx, schoolID, examID, studentID); err != nil || res != nil {
		t.Errorf("AdmissionResult(student) = %+v, %v; want nil, nil", res, err)
	}
}

// TestGetParticipantAttempt_RejectsInvalidParticipantType is the store-side
// half of the strict participant-type rule: a claim-derived value that is not
// one of the two literals must be refused before it reaches SQL.
func TestGetParticipantAttempt_RejectsInvalidParticipantType(t *testing.T) {
	db := admissionTestDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID := testUUID(t, ctx, db)
	exam := createAdmissionExam(t, ctx, s, schoolID, "REG-TYPELOOKUP-"+testUUID(t, ctx, db)[:8])
	applicantID := testUUID(t, ctx, db)
	insertApplicantAttempt(t, ctx, db, schoolID, exam.ID, applicantID, 50)

	for _, bad := range []string{"", "applicantt", "Applicant", "applicant "} {
		if _, err := s.GetParticipantAttempt(ctx, exam.ID, bad, applicantID); err == nil {
			t.Errorf("GetParticipantAttempt(participantType=%q) = nil error, want a rejection", bad)
		}
		if _, err := s.FocusLostParticipant(ctx, exam.ID, bad, applicantID); err == nil {
			t.Errorf("FocusLostParticipant(participantType=%q) = nil error, want a rejection", bad)
		}
	}

	got, err := s.GetParticipantAttempt(ctx, exam.ID, model.ParticipantApplicant, applicantID)
	if err != nil {
		t.Fatalf("GetParticipantAttempt(applicant): %v", err)
	}
	if got == nil || got.ParticipantID != applicantID {
		t.Fatalf("applicant attempt not found: %+v", got)
	}
}
