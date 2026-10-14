package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"

	"skci/identity/internal/common"
	"skci/identity/internal/security"
	"skci/identity/internal/store"
)

// openTestDB opens TEST_PG_DSN (all identity migrations applied) or skips.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("cannot open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

const testSecret = "test-jwt-secret"

func newTestApp(db *sql.DB) *App {
	app := NewApp(db, testSecret, "internal-key")
	app.CoreURL = "" // no audit dispatch from tests
	app.NotificationURL = ""
	return app
}

// securityRouter mounts the routes the security flows exercise, with the
// same middleware wiring as cmd/server/main.go.
func securityRouter(app *App) http.Handler {
	r := chi.NewRouter()
	r.Post("/auth/login", app.Login)
	r.Post("/auth/mfa/verify", app.VerifyMFA)
	r.Get("/auth/session-check", app.SessionCheck)
	r.Post("/auth/password/forgot", app.ForgotPassword)
	r.Get("/auth/password/reset/{token}", app.CheckResetToken)
	r.Post("/auth/password/reset", app.ResetPasswordWithToken)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTSessionMiddleware(testSecret, app.ValidateSession))
		pr.Get("/auth/me", app.Me)
		pr.Post("/auth/logout", app.Logout)
		pr.Get("/auth/sessions", app.MySessions)
		pr.Post("/auth/sessions/revoke-all", app.RevokeAllMySessions)
		pr.Get("/auth/login-history", app.MyLoginHistory)
		pr.Post("/auth/switch-role", app.SwitchRole)
		pr.Delete("/users/{id}", app.DeleteUser)
		pr.Delete("/users/{id}/roles/{role}", app.RemoveUserRole)
		pr.Post("/users/{id}/roles", app.AssignUserRole)
		pr.Post("/users/{id}/unlock", app.UnlockUserAccount)
	})
	return r
}

type seeded struct {
	schoolID string
	userID   string
	email    string
	password string
}

// seedUser creates a unit and an account holding roles there.
func seedUser(t *testing.T, db *sql.DB, roles ...string) seeded {
	t.Helper()
	s := seeded{schoolID: newIdentityTestUUID(), userID: newIdentityTestUUID(), password: "rahasia123"}
	s.email = fmt.Sprintf("qa-%s@test.local", s.userID[:8])
	hash, _ := common.HashPassword(s.password)
	mustExec(t, db, `INSERT INTO schools (id, name, code, level) VALUES ($1, 'QA Unit', $2, 'SD')`, s.schoolID, "QA"+s.schoolID[:6])
	mustExec(t, db, `INSERT INTO users (id, name, email, password_hash) VALUES ($1, 'QA User', $2, $3)`, s.userID, s.email, hash)
	for _, ro := range roles {
		if ro == common.RoleSuperadmin {
			mustExec(t, db, `INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1, NULL, $2)`, s.userID, ro)
			continue
		}
		mustExec(t, db, `INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1, $2, $3)`, s.userID, s.schoolID, ro)
	}
	return s
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func doJSON(t *testing.T, h http.Handler, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func login(t *testing.T, h http.Handler, email, password string) (int, map[string]any) {
	return doJSON(t, h, http.MethodPost, "/auth/login", "", map[string]string{"email": email, "password": password})
}

func tokenOf(t *testing.T, body map[string]any) string {
	t.Helper()
	data, _ := body["data"].(map[string]any)
	tok, _ := data["token"].(string)
	if tok == "" {
		t.Fatalf("no token in %v", body)
	}
	return tok
}

func sessionCheck(h http.Handler, token string) int {
	req := httptest.NewRequest(http.MethodGet, "/auth/session-check", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// BUG-022: deleting a user kills their token everywhere (identity routes and
// the nginx session-check used by every other service).
func TestTokenRejectedAfterUserDeleted(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := securityRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	victim := seedUser(t, db, common.RoleGuru)
	mustExec(t, db, `UPDATE user_school_roles SET school_id=$1 WHERE user_id=$2`, admin.schoolID, victim.userID)

	code, body := login(t, h, victim.email, victim.password)
	if code != http.StatusOK {
		t.Fatalf("login %d %v", code, body)
	}
	victimTok := tokenOf(t, body)
	if c := sessionCheck(h, victimTok); c != http.StatusNoContent {
		t.Fatalf("session-check before delete = %d", c)
	}
	_, body = login(t, h, admin.email, admin.password)
	adminTok := tokenOf(t, body)
	if code, body := doJSON(t, h, http.MethodDelete, "/users/"+victim.userID, adminTok, nil); code != http.StatusOK {
		t.Fatalf("delete %d %v", code, body)
	}
	if c := sessionCheck(h, victimTok); c != http.StatusUnauthorized {
		t.Fatalf("session-check after delete = %d, want 401", c)
	}
	if code, _ := doJSON(t, h, http.MethodGet, "/auth/me", victimTok, nil); code != http.StatusUnauthorized {
		t.Fatalf("/auth/me after delete = %d, want 401", code)
	}
}

// BUG-024: removing the active role refuses the old token at once.
func TestTokenRejectedAfterRoleRemoved(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := securityRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	guru := seedUser(t, db, common.RoleGuru, common.RoleOrangTua)
	mustExec(t, db, `UPDATE user_school_roles SET school_id=$1 WHERE user_id=$2`, admin.schoolID, guru.userID)

	_, body := login(t, h, guru.email, guru.password)
	guruTok := tokenOf(t, body)
	_, body = login(t, h, admin.email, admin.password)
	adminTok := tokenOf(t, body)
	if code, body := doJSON(t, h, http.MethodDelete, "/users/"+guru.userID+"/roles/guru", adminTok, nil); code != http.StatusOK {
		t.Fatalf("remove role %d %v", code, body)
	}
	if c := sessionCheck(h, guruTok); c != http.StatusUnauthorized {
		t.Fatalf("session-check after role removal = %d, want 401", c)
	}
	// A deactivated (inactive) role is refused too.
	_, body = login(t, h, guru.email, guru.password) // now logs in as orang_tua
	parentTok := tokenOf(t, body)
	mustExec(t, db, `UPDATE user_school_roles SET status='inactive' WHERE user_id=$1 AND role='orang_tua'`, guru.userID)
	if c := sessionCheck(h, parentTok); c != http.StatusUnauthorized {
		t.Fatalf("session-check after role deactivation = %d, want 401", c)
	}
}

// BUG-025: N failed logins lock the account; correct password is refused
// while locked; admin unlock restores access; login history records it all.
func TestLockoutAfterFailedLogins(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := securityRouter(app)
	u := seedUser(t, db, common.RoleGuru)
	max := security.DefaultPolicy().MaxFailedLogins

	for i := 1; i < max; i++ {
		if code, _ := login(t, h, u.email, "salah-"+fmt.Sprint(i)); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i, code)
		}
	}
	if code, _ := login(t, h, u.email, "salah-terakhir"); code != http.StatusLocked {
		t.Fatalf("attempt %d = %d, want 423", max, code)
	}
	if code, _ := login(t, h, u.email, u.password); code != http.StatusLocked {
		t.Fatalf("correct password while locked = %d, want 423", code)
	}
	var failures int
	_ = db.QueryRow(`SELECT count(*) FROM login_events WHERE user_id=$1 AND NOT success`, u.userID).Scan(&failures)
	if failures != max+1 {
		t.Fatalf("login_events failures = %d, want %d", failures, max+1)
	}

	admin := seedUser(t, db, common.RoleAdmin)
	mustExec(t, db, `UPDATE user_school_roles SET school_id=$1 WHERE user_id=$2`, admin.schoolID, u.userID)
	_, body := login(t, h, admin.email, admin.password)
	if code, body := doJSON(t, h, http.MethodPost, "/users/"+u.userID+"/unlock", tokenOf(t, body), nil); code != http.StatusOK {
		t.Fatalf("unlock %d %v", code, body)
	}
	if code, _ := login(t, h, u.email, u.password); code != http.StatusOK {
		t.Fatalf("login after unlock = %d", code)
	}
}

// BUG-025 idle timeout + BUG-083 sessions & logout-all.
func TestIdleTimeoutAndLogoutAll(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := securityRouter(app)
	u := seedUser(t, db, common.RoleGuru)

	_, b1 := login(t, h, u.email, u.password)
	_, b2 := login(t, h, u.email, u.password)
	t1, t2 := tokenOf(t, b1), tokenOf(t, b2)

	code, body := doJSON(t, h, http.MethodGet, "/auth/sessions", t1, nil)
	if list, _ := body["data"].([]any); code != http.StatusOK || len(list) != 2 {
		t.Fatalf("sessions %d %v", code, body)
	}
	if code, body := doJSON(t, h, http.MethodGet, "/auth/login-history", t1, nil); code != http.StatusOK || body["meta"] == nil {
		t.Fatalf("login-history %d %v", code, body)
	}

	// Session 2 idle beyond the policy -> refused.
	c2, _ := common.ParseToken(t2, testSecret)
	mustExec(t, db, `UPDATE user_sessions SET last_seen_at = now() - interval '2 hours' WHERE id=$1`, c2.SessionID)
	if c := sessionCheck(h, t2); c != http.StatusUnauthorized {
		t.Fatalf("idle session-check = %d, want 401", c)
	}

	// Logout-all from session 1 (keep none) kills session 1 too.
	if code, _ := doJSON(t, h, http.MethodPost, "/auth/sessions/revoke-all", t1, map[string]bool{"keep_current": false}); code != http.StatusOK {
		t.Fatalf("revoke-all %d", code)
	}
	if c := sessionCheck(h, t1); c != http.StatusUnauthorized {
		t.Fatalf("after revoke-all = %d, want 401", c)
	}

	// Plain logout.
	_, b3 := login(t, h, u.email, u.password)
	t3 := tokenOf(t, b3)
	if code, _ := doJSON(t, h, http.MethodPost, "/auth/logout", t3, nil); code != http.StatusOK {
		t.Fatalf("logout %d", code)
	}
	if c := sessionCheck(h, t3); c != http.StatusUnauthorized {
		t.Fatalf("after logout = %d, want 401", c)
	}
}

// Tokens identity did not sign (guardian / participant) and requests without
// a token pass the nginx check untouched.
func TestSessionCheckPassThrough(t *testing.T) {
	db := openTestDB(t)
	h := securityRouter(newTestApp(db))
	if c := sessionCheck(h, ""); c != http.StatusNoContent {
		t.Fatalf("no token = %d", c)
	}
	if c := sessionCheck(h, "not-a-jwt"); c != http.StatusNoContent {
		t.Fatalf("foreign token = %d", c)
	}
}

// BUG-047: an MFA role must enroll TOTP at first login and verify afterwards.
func TestMFAEnrollAndVerify(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := securityRouter(app)
	u := seedUser(t, db, common.RoleFinanceManager)

	code, body := login(t, h, u.email, u.password)
	data, _ := body["data"].(map[string]any)
	if code != http.StatusOK || data["mfa_required"] != true || data["mfa_enroll"] != true || data["token"] != nil {
		t.Fatalf("first login %d %v", code, body)
	}
	secret, _ := data["secret"].(string)
	mfaTok, _ := data["mfa_token"].(string)
	if code, _ := doJSON(t, h, http.MethodPost, "/auth/mfa/verify", "", map[string]string{"mfa_token": mfaTok, "code": "000000"}); code != http.StatusUnauthorized {
		t.Fatalf("wrong code = %d", code)
	}
	good, _ := security.TOTPCode(secret, time.Now())
	code, body = doJSON(t, h, http.MethodPost, "/auth/mfa/verify", "", map[string]string{"mfa_token": mfaTok, "code": good})
	if code != http.StatusOK {
		t.Fatalf("verify %d %v", code, body)
	}
	tokenOf(t, body)
	// The challenge is single use.
	if code, _ := doJSON(t, h, http.MethodPost, "/auth/mfa/verify", "", map[string]string{"mfa_token": mfaTok, "code": good}); code != http.StatusUnauthorized {
		t.Fatalf("replayed challenge = %d", code)
	}
	var enc string
	_ = db.QueryRow(`SELECT mfa_secret_enc FROM users WHERE id=$1`, u.userID).Scan(&enc)
	if enc == "" || enc == secret {
		t.Fatalf("secret not stored encrypted: %q", enc)
	}

	// Second login: verify step only, no new secret.
	code, body = login(t, h, u.email, u.password)
	data, _ = body["data"].(map[string]any)
	if code != http.StatusOK || data["mfa_required"] != true || data["mfa_enroll"] == true || data["secret"] != nil {
		t.Fatalf("second login %d %v", code, body)
	}
	good, _ = security.TOTPCode(secret, time.Now())
	if code, body := doJSON(t, h, http.MethodPost, "/auth/mfa/verify", "", map[string]string{"mfa_token": data["mfa_token"].(string), "code": good}); code != http.StatusOK {
		t.Fatalf("second verify %d %v", code, body)
	}
}

// BUG-023: a forbidden role combination is refused with 409.
func TestAssignRoleRejectsForbiddenCombination(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := securityRouter(app)
	sa := seedUser(t, db, common.RoleSuperadmin)
	target := seedUser(t, db, common.RoleFinanceManager)
	_, body := login(t, h, sa.email, sa.password)
	data, _ := body["data"].(map[string]any)
	// superadmin needs MFA: complete enrollment.
	secret, _ := data["secret"].(string)
	good, _ := security.TOTPCode(secret, time.Now())
	_, body = doJSON(t, h, http.MethodPost, "/auth/mfa/verify", "", map[string]string{"mfa_token": data["mfa_token"].(string), "code": good})
	saTok := tokenOf(t, body)

	code, body := doJSON(t, h, http.MethodPost, "/users/"+target.userID+"/roles", saTok, map[string]string{"role": "auditor", "school_id": target.schoolID})
	if code != http.StatusConflict {
		t.Fatalf("auditor onto finance_manager = %d %v, want 409", code, body)
	}
	code, body = doJSON(t, h, http.MethodPost, "/users/"+target.userID+"/roles", saTok, map[string]string{"role": "guru", "school_id": target.schoolID})
	if code != http.StatusCreated {
		t.Fatalf("guru onto finance_manager = %d %v, want 201", code, body)
	}
}

// BUG-085: reset link is single use, expires, revokes sessions and never
// reveals whether an email exists.
func TestPasswordResetLink(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := securityRouter(app)
	u := seedUser(t, db, common.RoleGuru)
	_, body := login(t, h, u.email, u.password)
	oldTok := tokenOf(t, body)

	for _, email := range []string{u.email, "tidak-ada@test.local"} {
		if code, _ := doJSON(t, h, http.MethodPost, "/auth/password/forgot", "", map[string]string{"email": email}); code != http.StatusOK {
			t.Fatalf("forgot(%s) = %d", email, code)
		}
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM password_reset_tokens WHERE user_id=$1 AND used_at IS NULL`, u.userID).Scan(&n)
	if n != 1 {
		t.Fatalf("pending reset tokens = %d, want 1", n)
	}

	// A known token (the emailed one is not observable from the test).
	tok, hash, _ := security.NewOpaqueToken()
	if err := store.CreatePasswordResetToken(t.Context(), db, u.userID, hash, "", 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	if code, _ := doJSON(t, h, http.MethodGet, "/auth/password/reset/"+tok, "", nil); code != http.StatusOK {
		t.Fatalf("check valid = %d", code)
	}
	if code, _ := doJSON(t, h, http.MethodPost, "/auth/password/reset", "", map[string]string{"token": tok, "password": "pendek"}); code != http.StatusBadRequest {
		t.Fatalf("weak password = %d", code)
	}
	if code, b := doJSON(t, h, http.MethodPost, "/auth/password/reset", "", map[string]string{"token": tok, "password": "baru12345"}); code != http.StatusOK {
		t.Fatalf("reset = %d %v", code, b)
	}
	if c := sessionCheck(h, oldTok); c != http.StatusUnauthorized {
		t.Fatalf("old session after reset = %d, want 401", c)
	}
	if code, _ := doJSON(t, h, http.MethodPost, "/auth/password/reset", "", map[string]string{"token": tok, "password": "lagi12345"}); code != http.StatusGone {
		t.Fatalf("reuse = %d, want 410", code)
	}
	if code, _ := login(t, h, u.email, "baru12345"); code != http.StatusOK {
		t.Fatalf("login with new password = %d", code)
	}

	// Expired link.
	tok2, hash2, _ := security.NewOpaqueToken()
	_ = store.CreatePasswordResetToken(t.Context(), db, u.userID, hash2, "", 30*time.Minute)
	mustExec(t, db, `UPDATE password_reset_tokens SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 minute' WHERE token_hash=$1`, hash2)
	if code, _ := doJSON(t, h, http.MethodGet, "/auth/password/reset/"+tok2, "", nil); code != http.StatusGone {
		t.Fatalf("expired check = %d, want 410", code)
	}
	if code, _ := doJSON(t, h, http.MethodGet, "/auth/password/reset/bogus", "", nil); code != http.StatusNotFound {
		t.Fatalf("unknown = %d, want 404", code)
	}
}
