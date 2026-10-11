package common

import (
	"context"
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
