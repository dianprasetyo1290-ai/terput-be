package handler

import (
	"time"

	"skci/report/internal/store"
	"skci/report/internal/upstream"
)

// Deps bundles everything the handlers need: the DB store and one typed
// client per sibling service this report-service aggregates from.
type Deps struct {
	Store      *store.Store
	Identity   *upstream.IdentityClient
	Academic   *upstream.AcademicClient
	Attendance *upstream.AttendanceClient
	Exam       *upstream.ExamClient
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
