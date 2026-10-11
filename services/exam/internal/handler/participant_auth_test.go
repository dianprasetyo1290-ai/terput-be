package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/exam/internal/cache"
	"skci/exam/internal/common"
	"skci/exam/internal/model"
)

const (
	testParticipantSecret = "test-participant-secret-9f2c"
	testJWTSecret         = "test-user-jwt-secret-4b71"
	testInternalKey       = "test-internal-key"
)

const (
	testExamA = "11111111-1111-1111-1111-111111111111"
	testExamB = "22222222-2222-2222-2222-222222222222"
)

// signParticipant mints an applicant token exactly the way
// InternalIssueParticipantAccess does.
func signParticipant(t *testing.T, claims common.ParticipantClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testParticipantSecret))
	if err != nil {
		t.Fatalf("sign participant token: %v", err)
	}
	return tok
}

// participantClaims builds a well-formed applicant token's claims, with the
// two fields under test (participant_type and scope) drawn from the caller.
func participantClaims(participantType, scope string, expires time.Time) common.ParticipantClaims {
	return common.ParticipantClaims{
		SchoolID:        "33333333-3333-3333-3333-333333333333",
		ExamID:          testExamA,
		ParticipantType: participantType,
		Scope:           scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "44444444-4444-4444-4444-444444444444",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	}
}

// TestParticipantJWTMiddleware_TokenScope is the "penolakan token participant
// kedaluwarsa/salah scope" check. Every applicant route sits behind this
// middleware, so a token that is expired, carries the wrong scope, claims a
// different participant kind, or was signed with another key must be refused
// before any handler (and therefore any DB access) runs.
func TestParticipantJWTMiddleware_TokenScope(t *testing.T) {
	valid := participantClaims(model.ParticipantApplicant, common.ParticipantScopeExamAttempt, time.Now().Add(time.Hour))
	expired := participantClaims(model.ParticipantApplicant, common.ParticipantScopeExamAttempt, time.Now().Add(-time.Minute))
	wrongScope := participantClaims(model.ParticipantApplicant, "exam:read", time.Now().Add(time.Hour))
	emptyScope := participantClaims(model.ParticipantApplicant, "", time.Now().Add(time.Hour))
	studentType := participantClaims(model.ParticipantStudent, common.ParticipantScopeExamAttempt, time.Now().Add(time.Hour))
	overlongType := participantClaims("applicantt", common.ParticipantScopeExamAttempt, time.Now().Add(time.Hour))

	otherSecret, err := jwt.NewWithClaims(jwt.SigningMethodHS256, valid).SignedString([]byte("some-other-service-secret"))
	if err != nil {
		t.Fatalf("sign foreign token: %v", err)
	}
	noneAlg, err := jwt.NewWithClaims(jwt.SigningMethodNone, valid).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign alg=none token: %v", err)
	}

	cases := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{"valid applicant token", "Bearer " + signParticipant(t, valid), http.StatusOK},
		{"lowercase scheme is still a bearer token", "bearer " + signParticipant(t, valid), http.StatusOK},
		{"no header", "", http.StatusUnauthorized},
		{"bearer with no token", "Bearer ", http.StatusUnauthorized},
		{"not a jwt", "Bearer not-a-jwt", http.StatusUnauthorized},
		{"other scheme", "Basic " + signParticipant(t, valid), http.StatusUnauthorized},
		{"expired token", "Bearer " + signParticipant(t, expired), http.StatusUnauthorized},
		{"wrong scope", "Bearer " + signParticipant(t, wrongScope), http.StatusUnauthorized},
		{"empty scope", "Bearer " + signParticipant(t, emptyScope), http.StatusUnauthorized},
		{"participant_type=student", "Bearer " + signParticipant(t, studentType), http.StatusUnauthorized},
		{"over-long participant_type", "Bearer " + signParticipant(t, overlongType), http.StatusUnauthorized},
		{"signed by another service", "Bearer " + otherSecret, http.StatusUnauthorized},
		{"alg=none", "Bearer " + noneAlg, http.StatusUnauthorized},
	}

	var gotClaims *common.ParticipantClaims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = common.ParticipantFromCtx(r)
		w.WriteHeader(http.StatusOK)
	})
	mw := common.ParticipantJWTMiddleware(testParticipantSecret)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotClaims = nil
			req := httptest.NewRequest(http.MethodPost, "/participant/exams/"+testExamA+"/enter", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()
			mw(next).ServeHTTP(rec, req)
			if rec.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, c.wantStatus, rec.Body.String())
			}
			if c.wantStatus == http.StatusOK {
				if gotClaims == nil {
					t.Fatal("valid token accepted but no claims in context")
				}
				if gotClaims.ExamID != testExamA || gotClaims.ParticipantType != model.ParticipantApplicant {
					t.Errorf("claims not carried through: %+v", gotClaims)
				}
			} else if gotClaims != nil {
				t.Errorf("rejected token still reached the handler with claims %+v", gotClaims)
			}
		})
	}
}

// TestParticipantRouter_Isolation is the router-level half: the participant
// group must reject a normal user JWT (the applicant routes would otherwise be
// reachable by any logged-in user), and an applicant token scoped to exam A
// must be refused on exam B.
//
// The handler is built with a nil Store on purpose: every case below must be
// decided by authentication/authorization, before any query.
func TestParticipantRouter_Isolation(t *testing.T) {
	h := New(nil, nil, testParticipantSecret, time.Minute)
	router := NewRouter(h, testJWTSecret, testInternalKey, []string{"*"}, cache.New("", 0))

	userJWT, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "55555555-5555-5555-5555-555555555555", "school_id": "33333333-3333-3333-3333-333333333333",
		"active_role": "siswa", "roles": []string{"siswa"}, "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign user jwt: %v", err)
	}

	applicantForA := signParticipant(t, participantClaims(model.ParticipantApplicant, common.ParticipantScopeExamAttempt, time.Now().Add(time.Hour)))

	do := func(method, path, auth string, internal bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		if internal {
			req.Header.Set("X-Internal-Key", testInternalKey)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	cases := []struct {
		name     string
		method   string
		path     string
		auth     string
		internal bool
		want     int
	}{
		{
			name:   "user JWT cannot enter the participant runner",
			method: http.MethodPost, path: "/participant/exams/" + testExamA + "/enter",
			auth: userJWT, want: http.StatusUnauthorized,
		},
		{
			name:   "user JWT cannot read a participant attempt",
			method: http.MethodGet, path: "/participant/exams/" + testExamA + "/attempt",
			auth: userJWT, want: http.StatusUnauthorized,
		},
		{
			name:   "applicant token is refused on another exam",
			method: http.MethodPost, path: "/participant/exams/" + testExamB + "/enter",
			auth: applicantForA, want: http.StatusForbidden,
		},
		{
			name:   "applicant token cannot be replayed on a user route",
			method: http.MethodGet, path: "/me/exams?status=today",
			auth: applicantForA, want: http.StatusUnauthorized,
		},
		{
			name:   "admission result is internal-only, no user JWT fallthrough",
			method: http.MethodGet, path: "/internal/admission/exams/" + testExamA + "/participants/66666666-6666-6666-6666-666666666666/result?school_id=33333333-3333-3333-3333-333333333333",
			auth: userJWT, want: http.StatusUnauthorized,
		},
		{
			name:   "admission result rejects a wrong internal key",
			method: http.MethodGet, path: "/internal/admission/exams/" + testExamA + "/participants/66666666-6666-6666-6666-666666666666/result?school_id=33333333-3333-3333-3333-333333333333",
			want: http.StatusUnauthorized,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(c.method, c.path, c.auth, c.internal)
			if rec.Code != c.want {
				t.Fatalf("%s %s: status = %d, want %d (body %s)", c.method, c.path, rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

// TestNewRequiresParticipantSecret pins that the handler can never be built
// with an empty applicant-token key — the failure mode is a self-minted HS256
// token being accepted on every /participant/** route.
func TestNewRequiresParticipantSecret(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New(..., participantSecret=\"\") did not panic: an empty key would be accepted")
		}
	}()
	New(nil, nil, "", time.Minute)
}

// TestNewParticipantSecretIsKeptCovered checks the value actually reaches the
// handler (and therefore the signing/middleware path) unchanged, and that a
// zero TTL degrades to the documented default rather than to no expiry.
func TestNewParticipantSecretIsKept(t *testing.T) {
	h := New(nil, nil, testParticipantSecret, 0)
	if h.ParticipantSecret != testParticipantSecret {
		t.Errorf("ParticipantSecret = %q, want %q", h.ParticipantSecret, testParticipantSecret)
	}
	if h.ParticipantTokenTTL != DefaultParticipantTokenTTL {
		t.Errorf("ParticipantTokenTTL = %v, want the %v default", h.ParticipantTokenTTL, DefaultParticipantTokenTTL)
	}
}

// TestInternalProvisionApplicantRejectsBadRequest documents that the internal
// provisioning contract rejects an incomplete body before touching the store
// (nil Store: a 400 here proves no query was attempted).
func TestInternalProvisionApplicantRejectsBadRequest(t *testing.T) {
	h := New(nil, nil, testParticipantSecret, time.Minute)
	router := NewRouter(h, testJWTSecret, testInternalKey, []string{"*"}, cache.New("", 0))

	for _, body := range []string{`{}`, `{"school_id":"s"}`, `{"applicant_id":"a"}`} {
		req := httptest.NewRequest(http.MethodPost, "/internal/admission/exams/"+testExamA+"/participants", strings.NewReader(body))
		req.Header.Set("X-Internal-Key", testInternalKey)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
}
