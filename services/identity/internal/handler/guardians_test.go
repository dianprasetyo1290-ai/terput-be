package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"skci/identity/internal/common"
)

func TestAdminAssignableDoesNotEscalatePrivilege(t *testing.T) {
	for _, role := range []string{common.RoleGuru, common.RoleSiswa, common.RoleOrangTua, common.RolePengawas} {
		if !adminAssignable(role) {
			t.Fatalf("expected %s to be admin-assignable", role)
		}
	}
	for _, role := range []string{common.RoleAdmin, common.RoleSuperadmin, common.RoleKepalaSekolah, common.RoleKepalaYayasan} {
		if adminAssignable(role) {
			t.Fatalf("privileged role %s must not be admin-assignable", role)
		}
	}
}

func TestInternalOnlyMiddleware(t *testing.T) {
	h := common.InternalOnlyMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !common.IsInternal(r) {
			t.Error("internal marker missing")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, tc := range []struct {
		name, key string
		want      int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "wrong", key: "not-secret", want: http.StatusUnauthorized},
		{name: "valid", key: "secret", want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/internal/guardians/upsert", nil)
			if tc.key != "" {
				req.Header.Set("X-Internal-Key", tc.key)
			}
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != tc.want {
				t.Fatalf("got %d, want %d", res.Code, tc.want)
			}
		})
	}
}

// internalGuardianSchool is the tenant-scoping guard shared by both internal
// guardian reads: only a non-blank ?school_id= is accepted, so a missing or
// whitespace tenant scope is rejected rather than silently read cross-school.
func TestInternalGuardianSchoolScope(t *testing.T) {
	for _, tc := range []struct {
		name   string
		query  string
		wantID string
		wantOK bool
	}{
		{name: "missing", query: "", wantOK: false},
		{name: "blank", query: "?school_id=", wantOK: false},
		{name: "whitespace only", query: "?school_id=%20%20", wantOK: false},
		{name: "present", query: "?school_id=s1", wantID: "s1", wantOK: true},
		{name: "trimmed", query: "?school_id=%20s1%20", wantID: "s1", wantOK: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/internal/guardians/g1/children"+tc.query, nil)
			id, ok := internalGuardianSchool(r)
			if id != tc.wantID || ok != tc.wantOK {
				t.Fatalf("internalGuardianSchool(%q) = (%q,%v), want (%q,%v)", tc.query, id, ok, tc.wantID, tc.wantOK)
			}
		})
	}
}

// A missing tenant scope must be rejected before any query runs. App.DB is
// deliberately nil: if the guard ever fell through, this would 500/panic
// instead of the expected 400.
func TestInternalGuardianEndpointsRequireSchoolID(t *testing.T) {
	app := &App{}
	for _, tc := range []struct {
		name string
		h    http.HandlerFunc
		path string
	}{
		{name: "children", h: app.InternalGuardianChildren, path: "/internal/guardians/g1/children"},
		{name: "access", h: app.InternalGuardianStudentAccess, path: "/internal/guardians/g1/students/s1/access"},
		{name: "children whitespace", h: app.InternalGuardianChildren, path: "/internal/guardians/g1/children?school_id=%20"},
		{name: "access whitespace", h: app.InternalGuardianStudentAccess, path: "/internal/guardians/g1/students/s1/access?school_id=%20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.h(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", w.Code)
			}
		})
	}
}
