package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"skci/academic/internal/common"
	"skci/academic/internal/store"
)

// ---------------------------------------------------------------------------
// In-process fake for the pgx database/sql driver.
//
// There is no Postgres test harness in this service (see the DB-gated test in
// internal/store/admission_enrollment_test.go, which follows the repo-wide
// skip-when-unreachable convention from exam-service). To exercise the whole
// request path deterministically — pending-placement short circuit, the
// school-scoped classroom lookup, and the (academic_year_id, student_user_id)
// upsert — these tests back *store.Store with a tiny driver that answers the
// exact SQL the store issues. It reimplements the two queries' semantics; it
// does NOT enforce real Postgres constraints, so the DB-gated store test is
// the check on the actual ON CONFLICT SQL.
// ---------------------------------------------------------------------------

const (
	testInternalKey = "test-internal-key"
	testJWTSecret   = "test-jwt-secret"
)

var classroomCols = []string{
	"id", "school_id", "academic_year_id", "name", "grade_level",
	"homeroom_teacher_user_id", "created_at", "updated_at",
}

var enrollmentCols = []string{
	"id", "school_id", "academic_year_id", "classroom_id", "student_user_id",
	"status", "enrolled_at", "created_at", "updated_at",
}

type fakeClassroom struct {
	id             string
	schoolID       string
	academicYearID string
}

type fakeEnrollment struct {
	id             string
	schoolID       string
	academicYearID string
	classroomID    string
	studentUserID  string
	status         string
	enrolledAt     time.Time
	createdAt      time.Time
	updatedAt      time.Time
}

type fakeAcademicDB struct {
	mu          sync.Mutex
	classrooms  []fakeClassroom
	enrollments []*fakeEnrollment
	seq         int
	queries     int
}

var (
	fakeDriverOnce sync.Once
	fakeDBsMu      sync.Mutex
	fakeDBs        = map[string]*fakeAcademicDB{}
)

type fakeDriver struct{}

func (fakeDriver) Open(name string) (driver.Conn, error) {
	fakeDBsMu.Lock()
	db := fakeDBs[name]
	fakeDBsMu.Unlock()
	if db == nil {
		return nil, errors.New("unknown fake db: " + name)
	}
	return &fakeConn{db: db}, nil
}

func openFakeDB(t *testing.T) (*fakeAcademicDB, *sql.DB) {
	t.Helper()
	fakeDriverOnce.Do(func() { sql.Register("academicfake", fakeDriver{}) })
	fdb := &fakeAcademicDB{}
	name := "fake-" + t.Name()
	fakeDBsMu.Lock()
	fakeDBs[name] = fdb
	fakeDBsMu.Unlock()
	db, err := sql.Open("academicfake", name)
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return fdb, db
}

type fakeConn struct{ db *fakeAcademicDB }

func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported by fake driver")
}
func (c *fakeConn) Close() error { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) {
	return nil, errors.New("txn not supported by fake driver")
}

func (c *fakeConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.db.query(query, args)
}

func (db *fakeAcademicDB) query(query string, args []driver.NamedValue) (driver.Rows, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.queries++
	q := strings.Join(strings.Fields(query), " ")

	switch {
	case strings.Contains(q, "FROM classrooms WHERE id = $1"):
		id, schoolID := asString(args[0]), asString(args[1])
		for _, c := range db.classrooms {
			if c.id == id && (schoolID == "" || c.schoolID == schoolID) {
				now := time.Now()
				return &fakeRows{
					cols: classroomCols,
					rows: [][]driver.Value{{
						c.id, c.schoolID, c.academicYearID, "Kelas " + c.id, nil, nil, now, now,
					}},
				}, nil
			}
		}
		return &fakeRows{cols: classroomCols}, nil

	case strings.Contains(q, "INSERT INTO class_enrollments"):
		schoolID, ayID := asString(args[0]), asString(args[1])
		classroomID, studentID := asString(args[2]), asString(args[3])
		now := time.Now()
		var e *fakeEnrollment
		for _, existing := range db.enrollments {
			if existing.academicYearID == ayID && existing.studentUserID == studentID {
				e = existing
				break
			}
		}
		if e == nil {
			db.seq++
			e = &fakeEnrollment{
				id: fmt.Sprintf("enrollment-%d", db.seq), schoolID: schoolID, academicYearID: ayID,
				classroomID: classroomID, studentUserID: studentID, status: "active",
				enrolledAt: now, createdAt: now, updatedAt: now,
			}
			db.enrollments = append(db.enrollments, e)
		} else {
			e.classroomID = classroomID
			e.status = "active"
			e.updatedAt = now
		}
		return &fakeRows{
			cols: enrollmentCols,
			rows: [][]driver.Value{{
				e.id, e.schoolID, e.academicYearID, e.classroomID, e.studentUserID,
				e.status, e.enrolledAt, e.createdAt, e.updatedAt,
			}},
		}, nil

	default:
		return nil, errors.New("unexpected query in fake driver: " + q)
	}
}

func asString(v driver.NamedValue) string {
	if v.Value == nil {
		return ""
	}
	s, _ := v.Value.(string)
	return s
}

type fakeRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestHandler(t *testing.T) (*fakeAcademicDB, *Handler) {
	t.Helper()
	fdb, db := openFakeDB(t)
	return fdb, New(store.New(db), nil)
}

// doUpsert drives the handler through the real InternalOrJWTMiddleware so the
// X-Internal-Key path (and the "no claims in context" behavior the handler
// relies on) is exercised exactly as in main.go's internal write group.
func doUpsert(t *testing.T, h *Handler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/admission/enrollments/upsert", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", testInternalKey)
	rec := httptest.NewRecorder()
	mw := common.InternalOrJWTMiddleware(testJWTSecret, testInternalKey)
	mw(http.HandlerFunc(h.UpsertAdmissionEnrollment)).ServeHTTP(rec, req)
	return rec
}

func baseBody(schoolID, studentID string) map[string]any {
	return map[string]any{
		"school_id":      schoolID,
		"student_id":     studentID,
		"application_id": "app-1",
		"conversion_ref": "app:app-1",
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// No placement yet: answer 200 pending_placement and touch the database not at
// all — the student must land in the placement queue, not fail the saga.
func TestUpsertAdmissionEnrollmentPendingPlacement(t *testing.T) {
	for _, tc := range []struct {
		name     string
		class    string
		acadYear string
	}{
		{"both missing", "", ""},
		{"classroom missing", "", "year-1"},
		{"academic year missing", "classroom-1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fdb, h := newTestHandler(t)
			body := baseBody("school-1", "student-1")
			if tc.class != "" {
				body["classroom_id"] = tc.class
			}
			if tc.acadYear != "" {
				body["academic_year_id"] = tc.acadYear
			}

			rec := doUpsert(t, h, body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
			var env struct {
				Data map[string]string `json:"data"`
			}
			decode(t, rec, &env)
			if env.Data["status"] != "pending_placement" {
				t.Fatalf("data.status = %q, want pending_placement", env.Data["status"])
			}
			if env.Data["message"] != "classroom and academic year not yet assigned" {
				t.Fatalf("data.message = %q", env.Data["message"])
			}
			if fdb.queries != 0 {
				t.Fatalf("pending placement touched the DB: %d queries", fdb.queries)
			}
			if len(fdb.enrollments) != 0 {
				t.Fatalf("pending placement wrote %d enrollment row(s)", len(fdb.enrollments))
			}
		})
	}
}

// Calling twice with a different classroom must reuse the single
// (academic_year_id, student_user_id) row and re-point it — idempotent upsert.
func TestUpsertAdmissionEnrollmentIdempotentUpdatesClassroom(t *testing.T) {
	fdb, h := newTestHandler(t)
	fdb.classrooms = []fakeClassroom{
		{id: "classroom-a", schoolID: "school-1", academicYearID: "year-1"},
		{id: "classroom-b", schoolID: "school-1", academicYearID: "year-1"},
	}

	first := baseBody("school-1", "student-1")
	first["classroom_id"] = "classroom-a"
	first["academic_year_id"] = "year-1"
	rec1 := doUpsert(t, h, first)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want 200 (body %s)", rec1.Code, rec1.Body.String())
	}
	got1 := decodeEnrollment(t, rec1)
	if got1.ClassroomID != "classroom-a" || got1.Status != "active" {
		t.Fatalf("first call = %+v, want classroom-a active", got1)
	}
	if got1.ID == "" {
		t.Fatalf("first call returned an empty enrollment id")
	}

	second := baseBody("school-1", "student-1")
	second["classroom_id"] = "classroom-b"
	second["academic_year_id"] = "year-1"
	rec2 := doUpsert(t, h, second)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second call status = %d, want 200 (body %s)", rec2.Code, rec2.Body.String())
	}
	got2 := decodeEnrollment(t, rec2)
	if got2.ClassroomID != "classroom-b" {
		t.Fatalf("second call classroom = %q, want classroom-b", got2.ClassroomID)
	}
	if got2.ID != got1.ID {
		t.Fatalf("idempotency broken: first id %q, second id %q", got1.ID, got2.ID)
	}
	if len(fdb.enrollments) != 1 {
		t.Fatalf("got %d enrollment rows after two calls, want 1", len(fdb.enrollments))
	}
	if fdb.enrollments[0].classroomID != "classroom-b" {
		t.Fatalf("stored classroom = %q, want classroom-b", fdb.enrollments[0].classroomID)
	}
}

// A classroom belonging to another school must be refused (404, no write) —
// the endpoint is reachable with X-Internal-Key and no user JWT, so the body's
// school_id is the only tenant boundary.
func TestUpsertAdmissionEnrollmentRejectsOtherSchoolClassroom(t *testing.T) {
	fdb, h := newTestHandler(t)
	fdb.classrooms = []fakeClassroom{
		{id: "classroom-foreign", schoolID: "school-other", academicYearID: "year-1"},
	}

	body := baseBody("school-1", "student-1")
	body["classroom_id"] = "classroom-foreign"
	body["academic_year_id"] = "year-1"
	rec := doUpsert(t, h, body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}
	if len(fdb.enrollments) != 0 {
		t.Fatalf("cross-school classroom wrote %d enrollment row(s)", len(fdb.enrollments))
	}
}

// A classroom that exists in the caller's school but in a different academic
// year must be refused (400, no write) too.
func TestUpsertAdmissionEnrollmentRejectsOtherYearClassroom(t *testing.T) {
	fdb, h := newTestHandler(t)
	fdb.classrooms = []fakeClassroom{
		{id: "classroom-other-year", schoolID: "school-1", academicYearID: "year-2"},
	}

	body := baseBody("school-1", "student-1")
	body["classroom_id"] = "classroom-other-year"
	body["academic_year_id"] = "year-1"
	rec := doUpsert(t, h, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if len(fdb.enrollments) != 0 {
		t.Fatalf("wrong-year classroom wrote %d enrollment row(s)", len(fdb.enrollments))
	}
}

// Required identifiers are enforced before any work happens.
func TestUpsertAdmissionEnrollmentRequiresIdentifiers(t *testing.T) {
	fdb, h := newTestHandler(t)
	rec := doUpsert(t, h, map[string]any{
		"school_id":      "school-1",
		"application_id": "app-1",
		"conversion_ref": "app:app-1",
		// student_id missing
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if fdb.queries != 0 {
		t.Fatalf("invalid request touched the DB: %d queries", fdb.queries)
	}
}

type enrollmentView struct {
	ID             string `json:"id"`
	ClassroomID    string `json:"classroom_id"`
	AcademicYearID string `json:"academic_year_id"`
	StudentUserID  string `json:"student_user_id"`
	Status         string `json:"status"`
}

func decodeEnrollment(t *testing.T, rec *httptest.ResponseRecorder) enrollmentView {
	t.Helper()
	var env struct {
		Data enrollmentView `json:"data"`
	}
	decode(t, rec, &env)
	return env.Data
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}
