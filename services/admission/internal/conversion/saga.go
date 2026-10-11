// Package conversion implements the applicant -> student conversion saga.
//
// The saga runs four ordered, individually idempotent steps against sibling
// services:
//
//	guardian (identity)  -> student (identity) -> link (identity) -> enrollment (academic)
//
// There is no distributed transaction. After each step the caller persists the
// record (step state + the returned ids) so a crash or a downstream outage can
// be resumed from exactly where it stopped. Downstream services dedupe on a
// stable external reference, so replaying a step never creates a duplicate
// user or guardian link.
package conversion

import (
	"context"
	"errors"
	"fmt"
)

// Source identifies admission as the origin of every conversion call.
const Source = "admission"

// Step names, used both as map keys and in the persisted step payloads.
const (
	StepGuardian   = "guardian"
	StepStudent    = "student"
	StepLink       = "link"
	StepEnrollment = "enrollment"
)

// Overall saga statuses (mirrored by student_conversions.status).
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Per-step statuses.
const (
	StepPending = "pending"
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
	StepSkipped = "skipped"
)

// ErrNotAvailable signals that a downstream endpoint is not deployed yet
// (HTTP 404). For the optional enrollment step this is tolerated: the step is
// recorded as skipped and the rest of the conversion still completes.
var ErrNotAvailable = errors.New("downstream endpoint is not available")

// StepState is the persisted status of one saga step.
type StepState struct {
	Status   string `json:"status"`
	Ref      string `json:"ref,omitempty"`
	Error    string `json:"error,omitempty"`
	Attempts int    `json:"attempts"`
}

// Record is the mutable saga state. The Ids are persisted between attempts so
// a resume never re-creates a user or link.
type Record struct {
	ID            string
	SchoolID      string
	ApplicationID string
	Key           string // idempotency key, stable per application

	ConversionRef string // stable external reference shared with identity
	GuardianRef   string // stable external ref for the guardian user
	StudentRef    string // stable external ref for the student user

	GuardianID   string
	StudentID    string
	LinkID       string
	EnrollmentID string

	Status      string
	CurrentStep string
	LastError   string

	// Payload copied from admission so the adapters can send real data.
	GuardianName  string
	GuardianEmail string
	GuardianPhone string

	ApplicantName  string
	ApplicantNISN  string
	ApplicantEmail string
	ApplicantPhone string

	Relationship string

	Steps map[string]*StepState
}

// Identity is the subset of the identity internal API the saga depends on.
type Identity interface {
	UpsertGuardian(ctx context.Context, r Record) (string, error)
	UpsertStudent(ctx context.Context, r Record) (string, error)
	LinkGuardian(ctx context.Context, r Record) (string, error)
}

// Academic provisions the optional enrollment after conversion.
type Academic interface {
	Enroll(ctx context.Context, r Record) (string, error)
}

// Persist writes the current record back to durable storage. It is called
// after every transition (start, each step, terminal state).
type Persist func(ctx context.Context, r Record) error

// Step returns the (initialised) state for a named step.
func (r *Record) Step(name string) *StepState {
	if r.Steps == nil {
		r.Steps = map[string]*StepState{}
	}
	if r.Steps[name] == nil {
		r.Steps[name] = &StepState{Status: StepPending}
	}
	return r.Steps[name]
}

// Run executes the saga, resuming from the first incomplete step. It returns
// the final record (also persisted by the callback) and the first hard error.
// A missing academic enrollment endpoint is not a hard error.
func Run(ctx context.Context, r Record, identity Identity, academic Academic, persist Persist, enroll bool) (Record, error) {
	if r.Steps == nil {
		r.Steps = map[string]*StepState{}
	}
	r.Status = StatusRunning
	if err := persist(ctx, r); err != nil {
		return r, err
	}

	if err := r.runStep(ctx, persist, StepGuardian, &r.GuardianID, func() (string, error) {
		return identity.UpsertGuardian(ctx, r)
	}); err != nil {
		return r.fail(ctx, persist, err)
	}

	if err := r.runStep(ctx, persist, StepStudent, &r.StudentID, func() (string, error) {
		return identity.UpsertStudent(ctx, r)
	}); err != nil {
		return r.fail(ctx, persist, err)
	}

	if err := r.runStep(ctx, persist, StepLink, &r.LinkID, func() (string, error) {
		return identity.LinkGuardian(ctx, r)
	}); err != nil {
		return r.fail(ctx, persist, err)
	}

	if enroll && r.EnrollmentID == "" {
		if academic == nil {
			r.skip(StepEnrollment, "academic client unavailable")
		} else {
			id, err := academic.Enroll(ctx, r)
			switch {
			case errors.Is(err, ErrNotAvailable):
				r.skip(StepEnrollment, "academic enrollment endpoint not available")
			case err != nil:
				return r.failStep(ctx, persist, StepEnrollment, err)
			default:
				r.EnrollmentID = id
				step := r.Step(StepEnrollment)
				step.Status = StepDone
				step.Ref = id
				step.Error = ""
			}
		}
		if err := persist(ctx, r); err != nil {
			return r, err
		}
	}

	r.Status = StatusCompleted
	r.CurrentStep = ""
	r.LastError = ""
	if err := persist(ctx, r); err != nil {
		return r, err
	}
	return r, nil
}

// runStep executes one idempotent step unless it is already done.
func (r *Record) runStep(ctx context.Context, persist Persist, name string, target *string, fn func() (string, error)) error {
	step := r.Step(name)
	if step.Status == StepDone && *target != "" {
		return nil
	}
	step.Attempts++
	step.Status = StepRunning
	step.Error = ""
	r.CurrentStep = name
	if err := persist(ctx, *r); err != nil {
		return err
	}
	id, err := fn()
	if err != nil {
		step.Status = StepFailed
		step.Error = err.Error()
		return err
	}
	step.Status = StepDone
	step.Ref = id
	step.Error = ""
	*target = id
	return persist(ctx, *r)
}

// skip records that a step was intentionally bypassed (e.g. optional
// enrollment whose endpoint is not deployed yet).
func (r *Record) skip(name, reason string) {
	step := r.Step(name)
	step.Status = StepSkipped
	step.Error = reason
	step.Ref = ""
}

// failStep marks a hard step failure and persists the failed state.
func (r *Record) failStep(ctx context.Context, persist Persist, name string, err error) (Record, error) {
	step := r.Step(name)
	step.Status = StepFailed
	step.Error = err.Error()
	return r.fail(ctx, persist, err)
}

// fail records the terminal failed state with its reason.
func (r Record) fail(ctx context.Context, persist Persist, err error) (Record, error) {
	r.Status = StatusFailed
	r.LastError = err.Error()
	if persistErr := persist(ctx, r); persistErr != nil {
		return r, fmt.Errorf("%w (persist failed: %v)", err, persistErr)
	}
	return r, err
}
