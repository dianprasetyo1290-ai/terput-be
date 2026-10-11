package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

// The identity service has no DB test harness (no sqlmock/testcontainers
// dependency, and store.Open talks to a real Postgres). To still exercise the
// internal guardian-read queries without a database we register a throwaway
// database/sql driver that records the SQL + bound args it received and
// returns one canned result set. This validates the parts that could actually
// regress in code: tenant-scope argument binding, the scanned projection, and
// the shape of the role predicates baked into the SQL. It is NOT a SQL engine,
// so it cannot prove those predicates are semantically correct against
// Postgres — that needs an integration test with the real schema.

type stubResult struct {
	columns []string
	rows    [][]driver.Value
}

type stubDriver struct {
	result   stubResult
	lastSQL  string
	lastArgs []driver.Value
}

func (d *stubDriver) Open(string) (driver.Conn, error) { return &stubConn{d: d}, nil }

type stubConn struct{ d *stubDriver }

func (c *stubConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("stub driver: Prepare unsupported")
}
func (c *stubConn) Close() error { return nil }
func (c *stubConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("stub driver: Begin unsupported")
}

func (c *stubConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.d.lastSQL = query
	c.d.lastArgs = make([]driver.Value, len(args))
	for i, a := range args {
		c.d.lastArgs[i] = a.Value
	}
	return &stubRows{columns: c.d.result.columns, rows: c.d.result.rows}, nil
}

type stubRows struct {
	columns []string
	rows    [][]driver.Value
	pos     int
}

func (r *stubRows) Columns() []string { return r.columns }
func (r *stubRows) Close() error      { return nil }
func (r *stubRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.pos])
	r.pos++
	return nil
}

var stubDriverSeq int64

func newStubDB(t *testing.T, result stubResult) (*sql.DB, *stubDriver) {
	t.Helper()
	d := &stubDriver{result: result}
	name := fmt.Sprintf("identity-store-stub-%d", atomic.AddInt64(&stubDriverSeq, 1))
	sql.Register(name, d)
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("open stub db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, d
}

// school_id must be the $1 tenant-scoping argument and both live-role checks
// must stay in the WHERE clause; otherwise an internal caller could read a
// guardian's links in the wrong school or vouch for a guardian whose
// orang_tua role was revoked.
func TestGuardianStudentAccessScopesAndChecksRoles(t *testing.T) {
	db, drv := newStubDB(t, stubResult{columns: []string{"exists"}, rows: [][]driver.Value{{true}}})

	allowed, err := GuardianStudentAccess(db, "school-1", "guardian-1", "student-1")
	if err != nil {
		t.Fatalf("GuardianStudentAccess: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true for an EXISTS row of true")
	}
	if want := []driver.Value{"school-1", "guardian-1", "student-1"}; !reflect.DeepEqual(drv.lastArgs, want) {
		t.Fatalf("bound args = %v, want %v", drv.lastArgs, want)
	}
	for _, needle := range []string{
		"gl.school_id=$1",
		"gl.guardian_user_id=$2",
		"gl.student_user_id=$3",
		"gl.status='active'",
		"role='orang_tua'",
		"role='siswa'",
		"status='active'",
		"u.deleted_at IS NULL",
	} {
		if !strings.Contains(drv.lastSQL, needle) {
			t.Fatalf("query missing %q:\n%s", needle, drv.lastSQL)
		}
	}
}

func TestGuardianStudentAccessDenied(t *testing.T) {
	db, _ := newStubDB(t, stubResult{columns: []string{"exists"}, rows: [][]driver.Value{{false}}})

	allowed, err := GuardianStudentAccess(db, "school-1", "guardian-1", "student-1")
	if err != nil {
		t.Fatalf("GuardianStudentAccess: %v", err)
	}
	if allowed {
		t.Fatal("expected allowed=false when there is no active, role-consistent link")
	}
}

// The internal children read returns exactly the {student_user_id,name,nisn}
// projection (nullable NISN preserved), tenant-scoped by school_id + guardian.
func TestLinkedGuardianChildrenScansMinimalProjection(t *testing.T) {
	db, drv := newStubDB(t, stubResult{
		columns: []string{"student_user_id", "name", "nisn"},
		rows: [][]driver.Value{
			{"student-1", "Ani", "123"},
			{"student-2", "Budi", nil},
		},
	})

	children, err := LinkedGuardianChildren(db, "school-1", "guardian-1")
	if err != nil {
		t.Fatalf("LinkedGuardianChildren: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("got %d children, want 2", len(children))
	}
	if children[0].StudentUserID != "student-1" || children[0].Name != "Ani" || children[0].NISN == nil || *children[0].NISN != "123" {
		t.Fatalf("unexpected first child: %+v", children[0])
	}
	if children[1].NISN != nil {
		t.Fatalf("nullable nisn must stay nil, got %v", *children[1].NISN)
	}
	if want := []driver.Value{"school-1", "guardian-1"}; !reflect.DeepEqual(drv.lastArgs, want) {
		t.Fatalf("bound args = %v, want %v", drv.lastArgs, want)
	}
	if !strings.Contains(drv.lastSQL, "gl.school_id=$1") || !strings.Contains(drv.lastSQL, "role='orang_tua'") {
		t.Fatalf("query must be tenant-scoped and role-checked:\n%s", drv.lastSQL)
	}
}

// Empty result set must serialize as an empty slice (WriteJSON relies on the
// store returning a non-nil slice so the internal `data` is [] not null).
func TestLinkedGuardianChildrenEmptyIsNonNil(t *testing.T) {
	db, _ := newStubDB(t, stubResult{columns: []string{"student_user_id", "name", "nisn"}})

	children, err := LinkedGuardianChildren(db, "school-1", "guardian-1")
	if err != nil {
		t.Fatalf("LinkedGuardianChildren: %v", err)
	}
	if children == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(children) != 0 {
		t.Fatalf("got %d children, want 0", len(children))
	}
}
