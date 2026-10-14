package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/security"
)

func masterRouter(app *App) http.Handler {
	r := chi.NewRouter()
	r.Post("/auth/login", app.Login)
	r.Post("/auth/mfa/verify", app.VerifyMFA)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTSessionMiddleware(testSecret, app.ValidateSession))
		pr.Post("/schools", app.CreateSchool)
		pr.Put("/schools/{id}", app.UpdateSchool)
		pr.Post("/schools/{id}/deactivate", app.DeactivateSchool)
		pr.Post("/users", app.CreateUser)
		pr.Put("/users/{id}", app.UpdateUser)
		pr.Get("/academic-years", app.ListAcademicYears)
		pr.Get("/academic-years/{id}", app.GetAcademicYear)
		pr.Post("/academic-years", app.CreateAcademicYear)
		pr.Put("/academic-years/{id}", app.UpdateAcademicYear)
		pr.Delete("/academic-years/{id}", app.DeleteAcademicYear)
		pr.Post("/academic-years/{id}/set-current", app.SetCurrentAcademicYear)
		pr.Post("/academic-years/{id}/lock", app.LockAcademicYear)
		pr.Post("/academic-years/{id}/unlock", app.UnlockAcademicYear)
		pr.Post("/students/{id}/status", app.ChangeStudentStatus)
		pr.Get("/students/{id}/status-history", app.StudentStatusHistory)
	})
	return r
}

// loginAny logs in, completing TOTP enrollment/verification when required.
func loginAny(t *testing.T, h http.Handler, email, password string) string {
	t.Helper()
	code, body := login(t, h, email, password)
	if code != http.StatusOK {
		t.Fatalf("login %d %v", code, body)
	}
	data, _ := body["data"].(map[string]any)
	if data["mfa_required"] != true {
		return tokenOf(t, body)
	}
	secret, _ := data["secret"].(string)
	if secret == "" {
		t.Fatal("test helper only supports first-time enrollment")
	}
	good, _ := security.TOTPCode(secret, time.Now())
	_, body = doJSON(t, h, http.MethodPost, "/auth/mfa/verify", "", map[string]string{"mfa_token": data["mfa_token"].(string), "code": good})
	return tokenOf(t, body)
}

func dataMap(body map[string]any) map[string]any {
	d, _ := body["data"].(map[string]any)
	return d
}

// BUG-013/069/068/012/045
func TestSchoolValidation(t *testing.T) {
	db := openTestDB(t)
	h := masterRouter(newTestApp(db))
	sa := seedUser(t, db, common.RoleSuperadmin)
	tok := loginAny(t, h, sa.email, sa.password)
	code := "QV" + sa.userID[:6]

	cases := []struct {
		name   string
		body   map[string]any
		status int
		field  string
	}{
		{"no code, no level", map[string]any{"name": "[QA] Unit Tanpa Kode"}, http.StatusBadRequest, "code"},
		{"bad settlement", map[string]any{"name": "U", "code": code + "X", "level": "SD", "settlement_account_number": "12ab"}, http.StatusBadRequest, "settlement_bank_name"},
		{"valid", map[string]any{"name": "Unit QA", "code": code, "level": "SD", "principal_name": "Bu Kepala",
			"accounting_entity_code": "ENT-01", "settlement_bank_name": "BSI", "settlement_account_number": "7123456789", "settlement_account_name": "Yayasan Teratai"}, http.StatusCreated, ""},
		{"duplicate code -> 409, no SQL text", map[string]any{"name": "Unit QA 2", "code": code, "level": "SMP"}, http.StatusConflict, "code"},
	}
	var schoolID string
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, body := doJSON(t, h, http.MethodPost, "/schools", tok, c.body)
			if st != c.status {
				t.Fatalf("status %d %v", st, body)
			}
			if c.field != "" {
				errs, _ := body["errors"].(map[string]any)
				if errs[c.field] == nil {
					t.Fatalf("field %s not flagged: %v", c.field, body)
				}
			}
			if msg, _ := body["message"].(string); containsAny(msg, "duplicate key", "SQLSTATE", "violates") {
				t.Fatalf("SQL leaked: %q", msg)
			}
			if st == http.StatusCreated {
				d := dataMap(body)
				schoolID, _ = d["id"].(string)
				if d["principal_name"] != "Bu Kepala" || d["settlement_account_number"] != "7123456789" {
					t.Fatalf("fields not stored: %v", d)
				}
			}
		})
	}
	// An update that omits the new fields keeps them (older forms).
	st, body := doJSON(t, h, http.MethodPut, "/schools/"+schoolID, tok, map[string]any{"name": "Unit QA", "code": code, "level": "SD"})
	if st != http.StatusOK || dataMap(body)["settlement_bank_name"] != "BSI" {
		t.Fatalf("update %d %v", st, body)
	}
	// Admin cannot change the settlement account.
	admin := seedUser(t, db, common.RoleAdmin)
	mustExec(t, db, `UPDATE user_school_roles SET school_id=$1 WHERE user_id=$2`, schoolID, admin.userID)
	adminTok := loginAny(t, h, admin.email, admin.password)
	st, _ = doJSON(t, h, http.MethodPut, "/schools/"+schoolID, adminTok, map[string]any{"name": "Unit QA", "code": code, "level": "SD", "settlement_account_number": "999999"})
	if st != http.StatusForbidden {
		t.Fatalf("admin settlement change = %d, want 403", st)
	}

	// BUG-014: deactivation with active users needs confirmation + reason.
	st, body = doJSON(t, h, http.MethodPost, "/schools/"+schoolID+"/deactivate", tok, nil)
	if st != http.StatusConflict || dataMap(body)["requires_confirmation"] != true {
		t.Fatalf("deactivate without confirm %d %v", st, body)
	}
	st, _ = doJSON(t, h, http.MethodPost, "/schools/"+schoolID+"/deactivate", tok, map[string]any{"confirm": true, "reason": "unit ditutup"})
	if st != http.StatusOK {
		t.Fatalf("deactivate confirmed = %d", st)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		for i := 0; i+len(x) <= len(s); i++ {
			if s[i:i+len(x)] == x {
				return true
			}
		}
	}
	return false
}

// BUG-084/074/016
func TestUserValidationAndDuplicates(t *testing.T) {
	db := openTestDB(t)
	h := masterRouter(newTestApp(db))
	admin := seedUser(t, db, common.RoleAdmin)
	tok := loginAny(t, h, admin.email, admin.password)

	st, body := doJSON(t, h, http.MethodPost, "/users", tok, map[string]any{"name": "X", "email": "bukan-email", "role": "siswa"})
	if st != http.StatusBadRequest {
		t.Fatalf("bad email = %d %v", st, body)
	}
	mk := func(email string) string {
		st, body := doJSON(t, h, http.MethodPost, "/users", tok, map[string]any{"name": "Siswa " + email, "email": email, "role": "siswa"})
		if st != http.StatusCreated {
			t.Fatalf("create %d %v", st, body)
		}
		return dataMap(body)["id"].(string)
	}
	a := mk("sa-" + admin.userID[:8] + "@test.local")
	b := mk("sb-" + admin.userID[:8] + "@test.local")
	now := time.Now().UnixNano()
	nisn := fmt.Sprintf("%010d", now%1e10)
	nik := fmt.Sprintf("%016d", now%1e16)

	cases := []struct {
		name   string
		id     string
		body   map[string]any
		status int
	}{
		{"NISN letters", a, map[string]any{"name": "A", "nisn": "ABC123XYZ"}, http.StatusBadRequest},
		{"NIK short", a, map[string]any{"name": "A", "nik": "123"}, http.StatusBadRequest},
		{"A gets NISN", a, map[string]any{"name": "A", "nisn": nisn, "nik": nik}, http.StatusOK},
		{"B same NISN -> warning", b, map[string]any{"name": "B", "nisn": nisn}, http.StatusConflict},
		{"B same NIK -> warning", b, map[string]any{"name": "B", "nik": nik}, http.StatusConflict},
		{"B confirmed", b, map[string]any{"name": "B", "nisn": nisn, "confirm_duplicate": true}, http.StatusOK},
		{"A unrelated edit not blocked", a, map[string]any{"name": "A2", "nisn": nisn}, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, body := doJSON(t, h, http.MethodPut, "/users/"+c.id, tok, c.body)
			if st != c.status {
				t.Fatalf("%d %v", st, body)
			}
			if st == http.StatusConflict {
				d := dataMap(body)
				if dups, _ := d["duplicates"].([]any); len(dups) == 0 {
					t.Fatalf("no duplicates listed: %v", body)
				}
			}
		})
	}
	// NIK omitted on a later edit keeps the stored value.
	var stored *string
	_ = db.QueryRow(`SELECT nik FROM users WHERE id=$1`, a).Scan(&stored)
	if stored == nil || *stored != nik {
		t.Fatalf("nik wiped: %v", stored)
	}
}

// BUG-070/015/150
func TestAcademicYearDatesAndLock(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := masterRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	tok := loginAny(t, h, admin.email, admin.password)

	st, body := doJSON(t, h, http.MethodPost, "/academic-years", tok, map[string]any{"label": "Tahun X", "start_date": "2026-10-14", "end_date": "2026-01-01"})
	if st != http.StatusBadRequest {
		t.Fatalf("end<start = %d %v", st, body)
	}
	st, body = doJSON(t, h, http.MethodPost, "/academic-years", tok, map[string]any{"label": "Tahun X"})
	if st != http.StatusBadRequest {
		t.Fatalf("no dates, free label = %d %v", st, body)
	}
	st, body = doJSON(t, h, http.MethodPost, "/academic-years", tok, map[string]any{"label": "2025/2026"})
	if st != http.StatusCreated || dataMap(body)["start_date"] != "2025-07-01" || dataMap(body)["end_date"] != "2026-06-30" {
		t.Fatalf("label default %d %v", st, body)
	}
	oldID := dataMap(body)["id"].(string)
	st, body = doJSON(t, h, http.MethodPost, "/academic-years", tok, map[string]any{"label": "2026/2027", "start_date": "2026-07-13", "end_date": "2027-06-26"})
	if st != http.StatusCreated {
		t.Fatalf("create %d %v", st, body)
	}
	newID := dataMap(body)["id"].(string)

	st, body = doJSON(t, h, http.MethodPost, "/academic-years/"+newID+"/set-current", tok, nil)
	if locked, _ := dataMap(body)["locked_year_ids"].([]any); st != http.StatusOK || len(locked) != 1 || locked[0] != oldID {
		t.Fatalf("set-current %d %v", st, body)
	}
	for _, c := range []struct {
		method, path string
		body         map[string]any
	}{
		{http.MethodPut, "/academic-years/" + oldID, map[string]any{"label": "2025/2026 ubah"}},
		{http.MethodDelete, "/academic-years/" + oldID, nil},
		{http.MethodPost, "/academic-years/" + oldID + "/set-current", nil},
	} {
		if st, body := doJSON(t, h, c.method, c.path, tok, c.body); st != http.StatusConflict {
			t.Fatalf("%s %s on locked = %d %v", c.method, c.path, st, body)
		}
	}
	// Admin cannot unlock; superadmin can with a reason.
	if st, _ := doJSON(t, h, http.MethodPost, "/academic-years/"+oldID+"/unlock", tok, map[string]any{"reason": "koreksi nilai rapor"}); st != http.StatusForbidden {
		t.Fatalf("admin unlock = %d", st)
	}
	sa := seedUser(t, db, common.RoleSuperadmin)
	saTok := loginAny(t, h, sa.email, sa.password)
	if st, _ := doJSON(t, h, http.MethodPost, "/academic-years/"+oldID+"/unlock", saTok, map[string]any{"reason": "x"}); st != http.StatusBadRequest {
		t.Fatalf("unlock short reason = %d", st)
	}
	if st, body := doJSON(t, h, http.MethodPost, "/academic-years/"+oldID+"/unlock", saTok, map[string]any{"reason": "koreksi nilai rapor semester 2"}); st != http.StatusOK {
		t.Fatalf("unlock %d %v", st, body)
	}
	if st, _ := doJSON(t, h, http.MethodPut, "/academic-years/"+oldID, tok, map[string]any{"label": "2025/2026", "end_date": "2025-01-01"}); st != http.StatusBadRequest {
		t.Fatalf("update end<start = %d", st)
	}

	// BUG-150: kepala_yayasan gets 403.
	ky := seedUser(t, db, common.RoleKepalaYayasan)
	kyTok := loginAny(t, h, ky.email, ky.password)
	if st, _ := doJSON(t, h, http.MethodGet, "/academic-years", kyTok, nil); st != http.StatusForbidden {
		t.Fatalf("kepala_yayasan list = %d, want 403", st)
	}
	if st, _ := doJSON(t, h, http.MethodGet, "/academic-years", tok, nil); st != http.StatusOK {
		t.Fatalf("admin list = %d", st)
	}
}

// BUG-017/075
func TestStudentStatusFlow(t *testing.T) {
	db := openTestDB(t)
	h := masterRouter(newTestApp(db))
	admin := seedUser(t, db, common.RoleAdmin)
	tok := loginAny(t, h, admin.email, admin.password)
	st, body := doJSON(t, h, http.MethodPost, "/users", tok, map[string]any{"name": "Siswa S", "email": "ss-" + admin.userID[:8] + "@test.local", "role": "siswa", "password": "siswa1234"})
	if st != http.StatusCreated {
		t.Fatalf("create %d %v", st, body)
	}
	sid := dataMap(body)["id"].(string)
	email := dataMap(body)["email"].(string)
	studentTok := loginAny(t, h, email, "siswa1234")

	steps := []struct {
		name   string
		body   map[string]any
		status int
	}{
		{"aktif -> keluar without reason", map[string]any{"status": "keluar"}, http.StatusUnprocessableEntity},
		{"aktif -> lulus", map[string]any{"status": "lulus"}, http.StatusOK},
		{"lulus -> aktif refused", map[string]any{"status": "aktif"}, http.StatusUnprocessableEntity},
		{"lulus -> aktif correction by admin refused", map[string]any{"status": "aktif", "correction": true, "reason": "salah input kelulusan"}, http.StatusUnprocessableEntity},
	}
	for _, c := range steps {
		t.Run(c.name, func(t *testing.T) {
			if st, body := doJSON(t, h, http.MethodPost, "/students/"+sid+"/status", tok, c.body); st != c.status {
				t.Fatalf("%d %v", st, body)
			}
		})
	}
	// A graduated student's siswa session is refused.
	if st, _ := doJSON(t, h, http.MethodGet, "/academic-years", studentTok, nil); st != http.StatusUnauthorized {
		t.Fatalf("graduated student token = %d, want 401", st)
	}
	ks := seedUser(t, db, common.RoleKepalaSekolah)
	mustExec(t, db, `UPDATE user_school_roles SET school_id=$1 WHERE user_id=$2`, admin.schoolID, ks.userID)
	ksTok := loginAny(t, h, ks.email, ks.password)
	if st, body := doJSON(t, h, http.MethodPost, "/students/"+sid+"/status", ksTok, map[string]any{"status": "aktif", "correction": true, "reason": "salah input kelulusan"}); st != http.StatusOK {
		t.Fatalf("kepsek correction %d %v", st, body)
	}
	st, body = doJSON(t, h, http.MethodGet, "/students/"+sid+"/status-history", tok, nil)
	if hist, _ := body["data"].([]any); st != http.StatusOK || len(hist) != 2 {
		t.Fatalf("history %d %v", st, body)
	}
}
