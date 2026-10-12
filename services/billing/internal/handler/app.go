package handler

import (
	"context"
	"database/sql"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/config"
	"skci/billing/internal/nicepay"
	"skci/billing/internal/storage"
)

// IdentityChecker is the subset of the identity client the handlers need:
// proof that a guardian user really is linked to the child they are reading.
// *client.Client satisfies it; tests use a stub.
type IdentityChecker interface {
	GuardianHasAccess(ctx context.Context, guardianUserID, studentUserID, schoolID string) (bool, error)
}

// ClassRoster resolves the students a teacher may see, and student grade levels (academic-service).
// *client.Client satisfies it; tests use a stub.
type ClassRoster interface {
	Configured() bool
	TeacherStudentIDs(ctx context.Context, schoolID, teacherUserID string) (map[string]bool, error)
	StudentGradeLevel(ctx context.Context, schoolID, studentUserID string) (string, error)
	SchoolGradeLevels(ctx context.Context, schoolID string) ([]string, error)
}

// App carries every dependency the HTTP handlers need. Handlers talk to the
// database directly through App.DB (the service is small enough that a query
// layer would only add indirection); pure domain rules live in internal/model.
type App struct {
	DB           *sql.DB
	Config       config.Config
	Storage      storage.Storage
	Identity     IdentityChecker
	// Roster limits guru/walikelas bill reads to their own classes. Nil
	// denies every teacher read.
	Roster       ClassRoster
	Nicepay      *nicepay.Client
	Core         *client.Client
	Notification *client.Client
	Accounting   *client.Client

	// T12 (kantin & dompet). IdentitySvc serves feature flags, role
	// accounts and guardian lists (nil = features off, fail closed); Ops
	// validates rooms (nil = stored unvalidated); PurposeCharger is the K1
	// top-up charge implemented by T14 (nil = online top-up 503); Clock is
	// a test clock (nil = time.Now).
	IdentitySvc    *client.Client
	Ops            *client.Client
	PurposeCharger purposeCharger
	Clock          func() time.Time
}

// New wires the application graph.
func New(db *sql.DB, cfg config.Config, identity IdentityChecker, store storage.Storage) *App {
	return &App{
		DB:       db,
		Config:   cfg,
		Storage:  store,
		Identity: identity,
	}
}
