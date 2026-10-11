package handler

import (
	"context"
	"time"

	"skci/report/internal/store"
	"skci/report/internal/upstream"
)

// GuardianAccessChecker answers the Portal Orang Tua authorization question:
// "is this guardian (wali / orang_tua) linked to this student, at this
// school?". *upstream.IdentityClient implements it by calling
// identity-service's internal /internal/guardians/... access endpoint; tests
// substitute a fake so the gate can be exercised without a network or a
// database.
type GuardianAccessChecker interface {
	GuardianStudentAccess(ctx context.Context, guardianUserID, studentUserID, schoolID string) (bool, error)
}

// Deps bundles everything the handlers need: the DB store and one typed
// client per sibling service this report-service aggregates from.
type Deps struct {
	Store      *store.Store
	Identity   *upstream.IdentityClient
	Academic   *upstream.AcademicClient
	Attendance *upstream.AttendanceClient
	Exam       *upstream.ExamClient
	// GuardianAuth gates every /parent/... (Portal Orang Tua) read. Nil means
	// the identity integration is not configured at all -- the gate then
	// fails closed (503) rather than open, so a missing env var can never
	// expose one student's rapor to an unrelated wali.
	GuardianAuth GuardianAccessChecker
}

type Handler struct {
	d *Deps
}

func New(d *Deps) *Handler {
	return &Handler{d: d}
}

func nowPtr() *time.Time {
	t := time.Now()
	return &t
}
