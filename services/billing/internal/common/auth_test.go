package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireBillReader(t *testing.T) {
	cases := []struct {
		role, wantSchool string
		ok               bool
	}{
		{RoleAdmin, "s1", true},
		{RoleGuru, "s1", true}, // ?school_id=other must be ignored for teachers
		{RoleWalikelas, "s1", true},
		{RoleParent, "", false},
		{RoleStudent, "", false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/admin/bills?school_id=other", nil)
		r = r.WithContext(context.WithValue(r.Context(), claimsCtxKey, &Claims{ActiveRole: tc.role, SchoolID: "s1"}))
		c, school := RequireBillReader(httptest.NewRecorder(), r)
		if (c != nil) != tc.ok || school != tc.wantSchool {
			t.Errorf("%s: ok=%v school=%q", tc.role, c != nil, school)
		}
	}
}

func TestFinanceStaffAndViewer(t *testing.T) {
	cases := []struct {
		role          string
		staff, viewer bool
	}{
		{RoleAdmin, true, true},
		{RoleSuperadmin, true, true},
		{RoleFinanceManager, true, true},
		{RoleFinanceAdmin, true, true},
		{RoleKepalaYayasan, false, true},
		{RoleKepalaSekolah, false, true},
		{RoleAccounting, false, false},
		{RoleKasir, false, false},
		{RoleParent, false, false},
		{RoleStudent, false, false},
		{RoleGuru, false, false},
	}
	for _, tc := range cases {
		mk := func() *httptest.ResponseRecorder { return httptest.NewRecorder() }
		req := func() *http.Request {
			r := httptest.NewRequest("GET", "/admin/x?school_id=other", nil)
			return r.WithContext(context.WithValue(r.Context(), claimsCtxKey, &Claims{ActiveRole: tc.role, SchoolID: "s1"}))
		}
		w := mk()
		c, school := RequireFinanceStaff(w, req())
		if (c != nil) != tc.staff {
			t.Errorf("staff %s: got %v code=%d", tc.role, c != nil, w.Code)
		}
		if c != nil {
			if school != "s1" { // JWT school always wins over ?school_id=
				t.Errorf("staff %s: school %q", tc.role, school)
			}
		}
		if c == nil && w.Code != http.StatusForbidden {
			t.Errorf("staff %s: code %d want 403", tc.role, w.Code)
		}
		c, _ = RequireFinanceViewer(mk(), req())
		if (c != nil) != tc.viewer {
			t.Errorf("viewer %s: got %v", tc.role, c != nil)
		}
		if IsFinanceStaff(tc.role) != tc.staff {
			t.Errorf("IsFinanceStaff %s", tc.role)
		}
	}
}
