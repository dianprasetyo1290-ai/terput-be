package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

type stubRoster struct {
	students map[string]map[string]bool // teacher -> student set
	err      error
}

func (s stubRoster) Configured() bool { return true }

func (s stubRoster) TeacherStudentIDs(_ context.Context, _, teacher string) (map[string]bool, error) {
	return s.students[teacher], s.err
}

func (s stubRoster) StudentGradeLevel(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (s stubRoster) SchoolGradeLevels(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

// A guru may only read bills of students in their own classes; every other
// teacher request is refused before the database is touched (DB is nil).
func TestTeacherBillScope(t *testing.T) {
	roster := stubRoster{students: map[string]map[string]bool{"guru-1": {"own-student": true}}}
	cases := []struct {
		name, role, query string
		roster            ClassRoster
		want              int
	}{
		{"no student filter", common.RoleGuru, "", roster, http.StatusForbidden},
		{"other class student", common.RoleGuru, "?student_user_id=other-student", roster, http.StatusForbidden},
		{"walikelas other student", common.RoleWalikelas, "?student_user_id=other-student", roster, http.StatusForbidden},
		{"roster error fails closed", common.RoleGuru, "?student_user_id=own-student", stubRoster{err: errors.New("down")}, http.StatusForbidden},
		{"no roster configured", common.RoleGuru, "?student_user_id=own-student", nil, http.StatusForbidden},
	}
	for _, tc := range cases {
		app := New(nil, config.Config{}, stubIdentity{}, nil)
		app.Roster = tc.roster
		r := chi.NewRouter()
		r.Use(common.JWTMiddleware(testSecret))
		r.Get("/admin/bills", app.ListBills)
		req := httptest.NewRequest("GET", "/admin/bills"+tc.query, nil)
		req.Header.Set("Authorization", "Bearer "+signToken(t, "guru-1", "school-1", tc.role))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}

	// Own student passes the guard (stops before the DB query).
	app := New(nil, config.Config{}, stubIdentity{}, nil)
	app.Roster = roster
	c := &common.Claims{UserID: "guru-1", SchoolID: "school-1", ActiveRole: common.RoleGuru}
	if !app.teacherMaySee(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), c, "school-1", "own-student") {
		t.Error("own student should be allowed")
	}
	admin := &common.Claims{UserID: "a", SchoolID: "school-1", ActiveRole: common.RoleAdmin}
	if !app.teacherMaySee(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), admin, "school-1", "") {
		t.Error("admin should pass through")
	}
}
