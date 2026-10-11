package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"skci/attendance/internal/cache"
	"skci/attendance/internal/client"
	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
	"skci/attendance/internal/store"
)

const (
	parentTestJWTSecret = "parent-test-jwt-secret"
	parentTestInternal  = "parent-test-internal-key"
	parentTestGuardian  = "guardian-user-1"
	parentTestStudent   = "student-user-1"
	parentTestOther     = "student-user-2"
	parentTestSchool    = "school-1"
)

type guardianCall struct {
	guardianID, studentID, schoolID string
}

// fakeGuardianAccess stands in for identity-service's access endpoint. When
// linked is non-empty, only that student is considered linked; otherwise err
// decides.
type fakeGuardianAccess struct {
	err    error
	linked string
	calls  []guardianCall
}

func (f *fakeGuardianAccess) CheckGuardianAccess(_ context.Context, guardianID, studentID, schoolID string) error {
	f.calls = append(f.calls, guardianCall{guardianID, studentID, schoolID})
	if f.linked != "" && studentID != f.linked {
		return client.ErrGuardianNoAccess
	}
	return f.err
}

type fakeParentAttendance struct {
	rows  []model.ParentAttendanceDay
	err   error
	calls []store.ParentAttendanceFilter
}

func (f *fakeParentAttendance) ListStudentAttendanceRange(_ context.Context, flt store.ParentAttendanceFilter) ([]model.ParentAttendanceDay, error) {
	f.calls = append(f.calls, flt)
	return f.rows, f.err
}

func parentClaims(role, userID, schoolID string) *common.Claims {
	return &common.Claims{UserID: userID, SchoolID: schoolID, ActiveRole: role, Roles: []string{role}}
}

// parentRequest builds a request carrying both the chi URL param and the JWT
// claims a real request would have after middleware.
func parentRequest(method, target, studentID string, claims *common.Claims) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("studentUserId", studentID)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	if claims != nil {
		ctx = context.WithValue(ctx, common.ClaimsCtxKey, claims)
	}
	return r.WithContext(ctx)
}

func decodeParentResponse(t *testing.T, rec *httptest.ResponseRecorder) parentChildAttendanceResponse {
	t.Helper()
	var env struct {
		Data parentChildAttendanceResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
	return env.Data
}

func TestParentChildAttendanceAllowsLinkedGuardian(t *testing.T) {
	catatan := "demam"
	ga := &fakeGuardianAccess{}
	pa := &fakeParentAttendance{rows: []model.ParentAttendanceDay{
		{SessionDate: "2026-01-02", Status: "hadir", ClassroomID: "c-1"},
		{SessionDate: "2026-01-03", Status: "sakit", Catatan: &catatan, ClassroomID: "c-1"},
		{SessionDate: "2026-01-04", Status: "tanpa_keterangan", ClassroomID: "c-1"},
		{SessionDate: "2026-01-05", Status: "belum_presensi", ClassroomID: "c-1"},
	}}
	h := &Handler{guardianAccess: ga, parentAttendance: pa}

	rec := httptest.NewRecorder()
	h.ParentChildAttendance(rec, parentRequest(http.MethodGet,
		"/parent/children/"+parentTestStudent+"/attendance?from=2026-01-01&to=2026-01-31",
		parentTestStudent, parentClaims("orang_tua", parentTestGuardian, parentTestSchool)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	body := decodeParentResponse(t, rec)
	if body.StudentID != parentTestStudent {
		t.Errorf("student_id = %q, want %q", body.StudentID, parentTestStudent)
	}
	if body.From != "2026-01-01" || body.To != "2026-01-31" {
		t.Errorf("from/to = %q/%q, want 2026-01-01/2026-01-31", body.From, body.To)
	}
	// belum_presensi is listed but not counted.
	if body.Summary.Hadir != 1 || body.Summary.Sakit != 1 || body.Summary.Izin != 0 || body.Summary.Alfa != 1 {
		t.Errorf("summary = %+v, want hadir=1 sakit=1 izin=0 alfa=1", body.Summary)
	}
	if len(body.Daily) != 4 {
		t.Fatalf("daily = %d rows, want 4", len(body.Daily))
	}
	if body.Daily[2].Status != "tanpa_keterangan" {
		t.Errorf("daily[2].status = %q, want tanpa_keterangan", body.Daily[2].Status)
	}

	// Guard check used the caller's own sub/school, never anything from the URL.
	if len(ga.calls) != 1 {
		t.Fatalf("guardian check calls = %d, want 1", len(ga.calls))
	}
	if ga.calls[0].guardianID != parentTestGuardian || ga.calls[0].studentID != parentTestStudent || ga.calls[0].schoolID != parentTestSchool {
		t.Errorf("guardian check args = %+v", ga.calls[0])
	}
	if len(pa.calls) != 1 {
		t.Fatalf("store calls = %d, want 1", len(pa.calls))
	}
	if got := pa.calls[0]; got.StudentID != parentTestStudent || got.SchoolID != parentTestSchool || got.From != "2026-01-01" || got.To != "2026-01-31" {
		t.Errorf("store filter = %+v", got)
	}
}

func TestParentChildAttendanceRejectsUnlinkedGuardian(t *testing.T) {
	ga := &fakeGuardianAccess{err: client.ErrGuardianNoAccess}
	pa := &fakeParentAttendance{}
	h := &Handler{guardianAccess: ga, parentAttendance: pa}

	rec := httptest.NewRecorder()
	h.ParentChildAttendance(rec, parentRequest(http.MethodGet,
		"/parent/children/"+parentTestStudent+"/attendance?from=2026-01-01&to=2026-01-31",
		parentTestStudent, parentClaims("orang_tua", parentTestGuardian, parentTestSchool)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
	if len(pa.calls) != 0 {
		t.Fatalf("store must not be read for an unlinked guardian, got %d calls", len(pa.calls))
	}
}

func TestParentChildAttendanceScopesToRequestedStudent(t *testing.T) {
	// Linked only to parentTestStudent; the caller tries the sibling's id.
	ga := &fakeGuardianAccess{linked: parentTestStudent}
	pa := &fakeParentAttendance{}
	h := &Handler{guardianAccess: ga, parentAttendance: pa}

	rec := httptest.NewRecorder()
	h.ParentChildAttendance(rec, parentRequest(http.MethodGet,
		"/parent/children/"+parentTestOther+"/attendance?from=2026-01-01&to=2026-01-31",
		parentTestOther, parentClaims("orang_tua", parentTestGuardian, parentTestSchool)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a child the guardian is not linked to (body=%s)", rec.Code, rec.Body.String())
	}
	if len(pa.calls) != 0 {
		t.Fatalf("store must not be read for a non-linked child, got %d calls", len(pa.calls))
	}
}

func TestParentChildAttendanceIgnoresCallerSuppliedScope(t *testing.T) {
	ga := &fakeGuardianAccess{}
	pa := &fakeParentAttendance{}
	h := &Handler{guardianAccess: ga, parentAttendance: pa}

	rec := httptest.NewRecorder()
	// Adversarial query params: the handler must scope by the path student and
	// the JWT school, ignoring school_id/student_id query overrides.
	h.ParentChildAttendance(rec, parentRequest(http.MethodGet,
		"/parent/children/"+parentTestStudent+"/attendance?from=2026-01-01&to=2026-01-31&school_id=other-school&student_id="+parentTestOther,
		parentTestStudent, parentClaims("orang_tua", parentTestGuardian, parentTestSchool)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if len(pa.calls) != 1 {
		t.Fatalf("store calls = %d, want 1", len(pa.calls))
	}
	if got := pa.calls[0]; got.StudentID != parentTestStudent || got.SchoolID != parentTestSchool {
		t.Errorf("store filter widened by query params: %+v", got)
	}
}

func TestParentChildAttendanceDateValidation(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		want     int
	}{
		{"missing both", "", "", http.StatusBadRequest},
		{"missing to", "2026-01-01", "", http.StatusBadRequest},
		{"bad from", "2026/01/01", "2026-01-31", http.StatusBadRequest},
		{"bad to", "2026-01-01", "31-01-2026", http.StatusBadRequest},
		{"impossible month", "2026-13-01", "2026-13-31", http.StatusBadRequest},
		{"from after to", "2026-02-01", "2026-01-01", http.StatusBadRequest},
		{"valid", "2026-01-01", "2026-01-31", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ga := &fakeGuardianAccess{}
			pa := &fakeParentAttendance{}
			h := &Handler{guardianAccess: ga, parentAttendance: pa}

			rec := httptest.NewRecorder()
			h.ParentChildAttendance(rec, parentRequest(http.MethodGet,
				"/parent/children/"+parentTestStudent+"/attendance?from="+tc.from+"&to="+tc.to,
				parentTestStudent, parentClaims("orang_tua", parentTestGuardian, parentTestSchool)))

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusOK && len(pa.calls) != 1 {
				t.Fatalf("valid range must reach the store, calls = %d", len(pa.calls))
			}
			if tc.want == http.StatusBadRequest && len(pa.calls) != 0 {
				t.Fatalf("invalid range must not reach the store, calls = %d", len(pa.calls))
			}
		})
	}
}

func TestParentChildAttendanceRejectsWrongRole(t *testing.T) {
	ga := &fakeGuardianAccess{}
	pa := &fakeParentAttendance{}
	h := &Handler{guardianAccess: ga, parentAttendance: pa}

	rec := httptest.NewRecorder()
	h.ParentChildAttendance(rec, parentRequest(http.MethodGet,
		"/parent/children/"+parentTestStudent+"/attendance?from=2026-01-01&to=2026-01-31",
		parentTestStudent, parentClaims("guru", parentTestGuardian, parentTestSchool)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
	if len(ga.calls) != 0 {
		t.Fatalf("role check must short-circuit before the guardian lookup, got %d calls", len(ga.calls))
	}
}

func TestParentChildAttendanceUnconfiguredClientFailsClosed(t *testing.T) {
	pa := &fakeParentAttendance{}
	// A client with an empty identity URL is "not configured" -- the handler
	// must deny, never fall through to serving the child's data.
	h := &Handler{guardianAccess: client.NewWithBaseURLs("", "", ""), parentAttendance: pa}

	rec := httptest.NewRecorder()
	h.ParentChildAttendance(rec, parentRequest(http.MethodGet,
		"/parent/children/"+parentTestStudent+"/attendance?from=2026-01-01&to=2026-01-31",
		parentTestStudent, parentClaims("orang_tua", parentTestGuardian, parentTestSchool)))

	if rec.Code < 400 {
		t.Fatalf("status = %d, want a denial (>=400) when the guardian checker is unavailable", rec.Code)
	}
	if len(pa.calls) != 0 {
		t.Fatalf("store must not be read when the guardian check cannot run, got %d calls", len(pa.calls))
	}
}

// ---- router-level checks ----------------------------------------------

func parentTestRouter(h *Handler) http.Handler {
	return NewRouter(h, RouterConfig{
		JWTSecret:   parentTestJWTSecret,
		InternalKey: parentTestInternal,
		CORSOrigins: []string{"http://localhost:5173"},
		Cache:       cache.New("", 0),
	})
}

func mintParentJWT(t *testing.T, role, userID, schoolID string) string {
	t.Helper()
	claims := common.Claims{
		UserID:     userID,
		SchoolID:   schoolID,
		ActiveRole: role,
		Roles:      []string{role},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(parentTestJWTSecret))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signed
}

func TestParentRouteRequiresJWT(t *testing.T) {
	router := parentTestRouter(&Handler{guardianAccess: &fakeGuardianAccess{}, parentAttendance: &fakeParentAttendance{}})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/parent/children/"+parentTestStudent+"/attendance?from=2026-01-01&to=2026-01-31", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestParentRouteRejectsWriteMethods(t *testing.T) {
	router := parentTestRouter(&Handler{guardianAccess: &fakeGuardianAccess{}, parentAttendance: &fakeParentAttendance{}})
	req := httptest.NewRequest(http.MethodPost, "/parent/children/"+parentTestStudent+"/attendance", nil)
	req.Header.Set("Authorization", "Bearer "+mintParentJWT(t, "orang_tua", parentTestGuardian, parentTestSchool))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d, want 405 (read-only endpoint)", rec.Code)
	}
}

func TestParentRouteAllowsLinkedGuardian(t *testing.T) {
	router := parentTestRouter(&Handler{
		guardianAccess:   &fakeGuardianAccess{linked: parentTestStudent},
		parentAttendance: &fakeParentAttendance{rows: []model.ParentAttendanceDay{{SessionDate: "2026-01-02", Status: "hadir"}}},
	})
	req := httptest.NewRequest(http.MethodGet,
		"/parent/children/"+parentTestStudent+"/attendance?from=2026-01-01&to=2026-01-31", nil)
	req.Header.Set("Authorization", "Bearer "+mintParentJWT(t, "orang_tua", parentTestGuardian, parentTestSchool))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if body := decodeParentResponse(t, rec); body.Summary.Hadir != 1 {
		t.Errorf("summary = %+v, want hadir=1", body.Summary)
	}
}

func TestParentRouteWrongRoleForbidden(t *testing.T) {
	router := parentTestRouter(&Handler{guardianAccess: &fakeGuardianAccess{}, parentAttendance: &fakeParentAttendance{}})
	req := httptest.NewRequest(http.MethodGet,
		"/parent/children/"+parentTestStudent+"/attendance?from=2026-01-01&to=2026-01-31", nil)
	req.Header.Set("Authorization", "Bearer "+mintParentJWT(t, "siswa", parentTestStudent, parentTestSchool))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
