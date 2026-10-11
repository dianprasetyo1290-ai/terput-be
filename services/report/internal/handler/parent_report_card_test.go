package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"skci/report/internal/cache"
	"skci/report/internal/common"
)

// fakeGuardianAccess is the mock GuardianAccessChecker: it reports `linked`
// for the student ids listed in its map and can be told to fail the way a
// down/unreachable identity-service would. Every call is recorded so a test
// can assert the gate asked about the right guardian/student/school.
type fakeGuardianAccess struct {
	linked map[string]bool
	err    error
	calls  []guardianCall
}

type guardianCall struct {
	guardian, student, school string
}

func (f *fakeGuardianAccess) GuardianStudentAccess(_ context.Context, guardianUserID, studentUserID, schoolID string) (bool, error) {
	f.calls = append(f.calls, guardianCall{guardianUserID, studentUserID, schoolID})
	if f.err != nil {
		return false, f.err
	}
	return f.linked[studentUserID], nil
}

// parentRequest builds a request already carrying the given claims and the
// chi studentUserId URL param, exactly as the router + JWTMiddleware would
// present it to RequireGuardianStudent.
func parentRequest(t *testing.T, claims *common.Claims, studentUserID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/parent/children/"+studentUserID+"/report-cards", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("studentUserId", studentUserID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	if claims != nil {
		ctx = context.WithValue(ctx, common.ClaimsCtxKey, claims)
	}
	return req.WithContext(ctx)
}

// serveGuardian runs the gate in front of a recording next handler and
// returns the status and whether the downstream handler was reached.
func serveGuardian(h *Handler, req *http.Request) (int, bool) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h.RequireGuardianStudent(next).ServeHTTP(rec, req)
	return rec.Code, called
}

func orangTua() *common.Claims {
	return &common.Claims{UserID: "wali-1", SchoolID: "school-1", ActiveRole: "orang_tua"}
}

// A wali with no link to the student named in the URL is refused, and the
// downstream rapor handler never runs.
func TestGuardianGateDeniesUnlinkedWali(t *testing.T) {
	fake := &fakeGuardianAccess{linked: map[string]bool{}}
	h := New(&Deps{GuardianAuth: fake})

	code, called := serveGuardian(h, parentRequest(t, orangTua(), "stu-1"))
	if code != http.StatusForbidden {
		t.Errorf("unlinked wali: status = %d, want 403", code)
	}
	if called {
		t.Error("unlinked wali must not reach the rapor handler")
	}
	if len(fake.calls) != 1 || fake.calls[0] != (guardianCall{"wali-1", "stu-1", "school-1"}) {
		t.Errorf("gate must ask identity about the JWT guardian + path student + JWT school, got %+v", fake.calls)
	}
}

// A linked wali passes and the downstream rapor handler runs.
func TestGuardianGateAllowsLinkedWali(t *testing.T) {
	fake := &fakeGuardianAccess{linked: map[string]bool{"stu-1": true}}
	h := New(&Deps{GuardianAuth: fake})

	code, called := serveGuardian(h, parentRequest(t, orangTua(), "stu-1"))
	if code != http.StatusOK {
		t.Errorf("linked wali: status = %d, want 200", code)
	}
	if !called {
		t.Error("linked wali must reach the rapor handler")
	}
}

// The gate is per-student: being linked to one child must not grant access to
// another student swapped into the URL (their classmate, or any other user).
func TestGuardianGateScopesToTheRequestedStudent(t *testing.T) {
	fake := &fakeGuardianAccess{linked: map[string]bool{"stu-1": true}} // linked to stu-1 only
	h := New(&Deps{GuardianAuth: fake})

	if code, called := serveGuardian(h, parentRequest(t, orangTua(), "stu-2")); code != http.StatusForbidden || called {
		t.Errorf("other student: status = %d, called = %v; want 403 and no handler", code, called)
	}
	// Sanity: the same wali still gets their own child.
	if code, called := serveGuardian(h, parentRequest(t, orangTua(), "stu-1")); code != http.StatusOK || !called {
		t.Errorf("own child: status = %d, called = %v; want 200 and handler", code, called)
	}
}

// Fail closed, never open: an unconfigured checker or an identity-service
// that could not answer yields 503 (not a permissive 200, and not a 403 that
// would wrongly imply "confirmed not linked").
func TestGuardianGateFailsClosed(t *testing.T) {
	// IDENTITY_SERVICE_URL not configured -> nil checker.
	if code, called := serveGuardian(New(&Deps{}), parentRequest(t, orangTua(), "stu-1")); code != http.StatusServiceUnavailable || called {
		t.Errorf("unconfigured identity: status = %d, called = %v; want 503 and no handler", code, called)
	}
	// Identity reachable-but-erroring (timeout/500/unreachable).
	down := &fakeGuardianAccess{linked: map[string]bool{"stu-1": true}, err: errors.New("identity unreachable")}
	if code, called := serveGuardian(New(&Deps{GuardianAuth: down}), parentRequest(t, orangTua(), "stu-1")); code != http.StatusServiceUnavailable || called {
		t.Errorf("identity down: status = %d, called = %v; want 503 and no handler", code, called)
	}
}

// Non-parent tokens do not even reach the identity check -- staff/admin use
// their own endpoints; the parent routes are for wali only.
func TestGuardianGateRejectsNonParentRole(t *testing.T) {
	fake := &fakeGuardianAccess{linked: map[string]bool{"stu-1": true}}
	h := New(&Deps{GuardianAuth: fake})
	claims := &common.Claims{UserID: "guru-1", SchoolID: "school-1", ActiveRole: "guru"}

	if code, called := serveGuardian(h, parentRequest(t, claims, "stu-1")); code != http.StatusForbidden || called {
		t.Errorf("guru token: status = %d, called = %v; want 403 and no handler", code, called)
	}
	if len(fake.calls) != 0 {
		t.Errorf("non-parent role must not trigger a guardian lookup, got %+v", fake.calls)
	}
}

// A request with no claims in context (should not happen behind
// JWTMiddleware, but the gate must not fall open if it ever does).
func TestGuardianGateRequiresClaims(t *testing.T) {
	h := New(&Deps{GuardianAuth: &fakeGuardianAccess{linked: map[string]bool{"stu-1": true}}})
	if code, called := serveGuardian(h, parentRequest(t, nil, "stu-1")); code != http.StatusUnauthorized || called {
		t.Errorf("no claims: status = %d, called = %v; want 401 and no handler", code, called)
	}
}

// The /parent/... routes are registered in the JWT group and wired to the
// gate: an unsigned request is refused by JWTMiddleware (proving the routes
// exist and route correctly rather than 404ing), and a valid orang_tua token
// for an unlinked student is refused by the gate (proving the group's
// middleware is attached). Neither path touches the DB, so this runs with no
// store.
func TestParentRoutesRegisteredAndGated(t *testing.T) {
	const secret = "test-secret"
	fake := &fakeGuardianAccess{linked: map[string]bool{"stu-1": true}} // NOT linked to stu-2
	r := NewRouter(&Deps{GuardianAuth: fake}, secret, []string{"http://localhost:5173"}, cache.New("", time.Second))

	// No token -> 401 from JWTMiddleware, not a 404 route miss or a panic.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/parent/children/stu-1/report-cards", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: status = %d, want 401 (route should exist behind JWT)", rec.Code)
	}

	// Valid orang_tua token, unlinked student -> 403 from the gate, before
	// any store access.
	token := mintTestToken(t, secret, *orangTua())
	req := httptest.NewRequest(http.MethodGet, "/parent/children/stu-2/report-cards", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("linked-to-other-student: status = %d, want 403", rec.Code)
	}
}

func mintTestToken(t *testing.T, secret string, claims common.Claims) string {
	t.Helper()
	claims.RegisteredClaims = jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return signed
}
