package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"skci/identity/internal/common"
)

// Students and parents cannot resolve other accounts' names; the request is
// refused before the database is touched.
func TestUserNamesRefusesStudentsAndParents(t *testing.T) {
	a := &App{}
	for _, role := range []string{common.RoleSiswa, common.RoleOrangTua} {
		req := httptest.NewRequest(http.MethodGet, "/users/names?ids=44444444-4444-4444-4444-444444444444", nil)
		req = req.WithContext(context.WithValue(req.Context(), common.ClaimsCtxKey, &common.Claims{ActiveRole: role, SchoolID: "11111111-1111-1111-1111-111111111111"}))
		rec := httptest.NewRecorder()
		a.UserNames(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403", role, rec.Code)
		}
	}
}
