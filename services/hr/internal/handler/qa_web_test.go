package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/hr/internal/common"
)

// superadminToken mirrors identity's superadmin JWT: no school_id claim.
func superadminToken() string {
	claims := common.Claims{
		UserID:     "40000000-0000-0000-0000-000000000001",
		ActiveRole: common.RoleSuperadmin,
		Role:       common.RoleSuperadmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "40000000-0000-0000-0000-000000000001",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWT))
	return tok
}

func doGet(t *testing.T, h http.Handler, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// Every list page the QA sweep opened must answer 200 on a real schema: this
// catches queries against columns that do not exist (e.nip, pos.title) and
// the superadmin empty-school_id 500 (QA BUG-066, BUG-149).
func TestQAWeb_ListEndpointsAgainstSchema(t *testing.T) {
	router := setupTestApp(t).Router()
	sa := superadminToken()
	paths := []string{
		"/training/courses", "/training/mandatory", "/training/assignments",
		"/onboarding/templates",
		"/ats/requisitions", "/ats/postings", "/ats/applications", "/ats/offers",
		"/ats/stages", "/ats/scorecard-templates", "/ats/talent-pool",
		"/payroll/components", "/payroll/inputs?period=2026-09", "/payroll/runs",
		"/payroll/payslips", "/payroll/journals",
		"/employees", "/departments", "/positions", "/org-chart",
		"/leave-types", "/document-types", "/leave-requests", "/attendance",
	}
	for _, p := range paths {
		sep := "?"
		if strings.Contains(p, "?") {
			sep = "&"
		}
		rr := doGet(t, router, sa, p+sep+"school_id="+schoolID)
		if rr.Code != http.StatusOK {
			t.Errorf("superadmin GET %s = %d: %s", p, rr.Code, rr.Body.String())
		}
	}
}

func TestQAWeb_SuperadminWithoutSchoolIsBadRequestNotSQL(t *testing.T) {
	router := setupTestApp(t).Router()
	sa := superadminToken()
	for _, p := range []string{"/training/courses", "/training/courses?school_id=", "/payroll/runs?school_id=not-a-uuid"} {
		rr := doGet(t, router, sa, p)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", p, rr.Code)
		}
		if strings.Contains(rr.Body.String(), "SQLSTATE") || strings.Contains(rr.Body.String(), "uuid") {
			t.Errorf("GET %s leaks driver error: %s", p, rr.Body.String())
		}
	}
}

func TestQAWeb_InternalErrorHidesDriverMessage(t *testing.T) {
	rr := httptest.NewRecorder()
	common.InternalError(rr, "Gagal memuat data", &pgLikeError{})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "SQLSTATE") || !strings.Contains(rr.Body.String(), "Gagal memuat data") {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

type pgLikeError struct{}

func (*pgLikeError) Error() string {
	return `invalid input syntax for type uuid: "" (SQLSTATE 22P02)`
}

// Role matrix from the T2–T5 endpoint tables (QA BUG-067 + role sweep).
func TestQAWeb_RoleMatrix(t *testing.T) {
	router := setupTestApp(t).Router()
	tok := func(role string) string { return createToken("50000000-0000-0000-0000-000000000001", role) }
	cases := []struct {
		role, path string
		want       int
	}{
		{common.RolePayroll, "/payroll/runs", 200},
		{common.RolePayroll, "/payroll/inputs?period=2026-09", 200},
		{common.RolePayroll, "/payroll/components", 200},
		{common.RolePayroll, "/payroll/payslips", 200},
		{common.RolePayroll, "/payroll/journals", 200},
		{common.RolePayroll, "/employees", 403},
		{common.RoleFinanceManager, "/payroll/runs", 200},
		{common.RoleFinanceManager, "/payroll/journals", 200},
		{common.RoleFinanceManager, "/payroll/inputs?period=2026-09", 403},
		{common.RoleRecruiter, "/ats/requisitions", 200},
		{common.RoleRecruiter, "/ats/postings", 200},
		{common.RoleRecruiter, "/ats/talent-pool", 200},
		{common.RoleRecruiter, "/payroll/runs", 403},
		{common.RoleTrainingAdmin, "/training/courses", 200},
		{common.RoleTrainingAdmin, "/training/assignments", 200},
		{common.RoleTrainingAdmin, "/training/mandatory", 200},
		{common.RoleKepalaSekolah, "/org-chart", 200},
		{common.RoleKepalaSekolah, "/departments", 200},
		{common.RoleKepalaSekolah, "/positions", 200},
		{common.RoleKepalaSekolah, "/training/assignments", 200},
		{common.RoleKepalaSekolah, "/ats/requisitions", 200},
		{common.RoleKepalaSekolah, "/employees", 403},
		{common.RoleHRAdmin, "/training/mandatory", 200},
		{common.RoleHRAdmin, "/payroll/inputs?period=2026-09", 200},
		{common.RolePegawai, "/training/courses", 403},
		{common.RolePegawai, "/attendance", 403},
		{common.RolePegawai, "/me/training/catalog", 200},
		{common.RolePegawai, "/leave-types", 200},
		{common.RolePegawai, "/document-types", 200},
		{common.RoleGuru, "/me/training/catalog", 200},
		// No employee row for this user: a clean 404 the UI renders as
		// "belum terdaftar sebagai pegawai", never a 403/500.
		{common.RoleGuru, "/me/attendance", 404},
	}
	for _, c := range cases {
		rr := doGet(t, router, tok(c.role), c.path)
		if rr.Code != c.want {
			t.Errorf("%s GET %s = %d, want %d: %s", c.role, c.path, rr.Code, c.want, rr.Body.String())
		}
	}
}
