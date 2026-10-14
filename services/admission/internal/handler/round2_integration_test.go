package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"skci/admission/internal/common"
)

const testJWTSecret = "test-identity-secret"

func identityToken(t *testing.T, userID, schoolID, role, name string) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, common.AdminClaims{
		UserID: userID, Name: name, SchoolID: schoolID, ActiveRole: role, Roles: []string{role},
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func call(t *testing.T, h http.Handler, method, path, token string, body any) (int, map[string]any) {
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

func round2Router(a *App) http.Handler {
	a.Config.JWTSecret = testJWTSecret
	a.Config.GuardianJWTSecret = "test-guardian-secret"
	a.Config.GuardianTTL = time.Hour
	r := chi.NewRouter()
	r.With(common.ParentAuth(testJWTSecret)).Post("/public/session/sekoci", a.SekociSession)
	r.Group(func(ad chi.Router) {
		ad.Use(common.AdminAuth(testJWTSecret))
		ad.Get("/admin/config/form-fields", a.ListFormFields)
		ad.Post("/admin/config/form-fields", a.CreateFormField)
		ad.Get("/admin/refunds", a.ListRefunds)
		ad.Post("/admin/refunds/{id}/process", a.ProcessRefund)
		ad.Post("/admin/refunds/{id}/waive", a.WaiveRefund)
	})
	return r
}

// BUG-095: fields configured for the period are required before submit.
func TestFormFieldsBlockSubmit(t *testing.T) {
	a := testApp(t)
	h := round2Router(a)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 10)
	admin := identityToken(t, "admin-1", s.school, "admin", "Admin Uji")

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"custom select", map[string]any{"period_id": s.period, "key": "hobi", "label": "Hobi", "type": "select", "options": []string{"Musik", "Olahraga"}, "required": true}, 201},
		{"builtin toggle", map[string]any{"period_id": s.period, "key": "nisn", "label": "NISN", "type": "text", "required": true}, 201},
		{"duplicate key", map[string]any{"period_id": s.period, "key": "hobi", "label": "Hobi 2", "type": "text"}, 409},
		{"select without options", map[string]any{"period_id": s.period, "key": "warna", "label": "Warna", "type": "select"}, 422},
	}
	for _, c := range cases {
		if code, out := call(t, h, "POST", "/admin/config/form-fields", admin, c.body); code != c.want {
			t.Fatalf("%s: %d %v", c.name, code, out)
		}
	}
	if code, out := call(t, h, "GET", "/admin/config/form-fields?period_id="+s.period, admin, nil); code != 200 {
		t.Fatalf("list: %d %v", code, out)
	}

	app := seedApplication(t, a.DB, s, s.guardian, "draft")
	missing, err := missingFormFields(ctx, a.DB, s.school, app)
	if err != nil || len(missing) != 2 {
		t.Fatalf("missing %v %v", missing, err)
	}
	if err := a.submit(ctx, s.school, s.guardian, app, 1); err == nil || !strings.Contains(err.Error(), "formulir wajib") {
		t.Fatalf("submit with empty form fields: %v", err)
	}
	if _, err := normalizeApplicantCustom(ctx, a.DB, s.school, map[string]any{"hobi": "Memasak"}); err == nil {
		t.Fatal("value outside options accepted")
	}
	vals, err := normalizeApplicantCustom(ctx, a.DB, s.school, map[string]any{"hobi": "Musik"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(vals)
	mustExec(t, a.DB, `UPDATE applicants SET custom_fields=$1,nisn='0012345678' WHERE id=(SELECT applicant_id FROM applications WHERE id=$2)`, raw, app)
	if missing, err = missingFormFields(ctx, a.DB, s.school, app); err != nil || len(missing) != 0 {
		t.Fatalf("still missing %v %v", missing, err)
	}
}

// BUG-093: overlapping waves only warn within the same jenjang.
func TestWaveWarningsPerLevel(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 10)
	mustExec(t, a.DB, `UPDATE admission_waves SET education_level='SD',entry_grade=1 WHERE id=$1`, s.wave)
	start, end := time.Now(), time.Now().Add(5*24*time.Hour)
	for _, c := range []struct {
		level string
		want  int
	}{{"SD", 1}, {"SMP", 0}, {"", 1}} {
		got, err := waveWarnings(ctx, a.DB, s.school, s.period, "", c.level, start, end)
		if err != nil || len(got) != c.want {
			t.Fatalf("level %q: %v %v", c.level, got, err)
		}
	}
}

// BUG-040: a drop creates a refund under the period policy; the committee
// records the payout once, the guardian is notified, the due amount clears.
func TestRefundRecordAndProcess(t *testing.T) {
	a := testApp(t)
	h := round2Router(a)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 10)
	mustExec(t, a.DB, `UPDATE admission_periods SET refund_policy_bp=5000,refund_admin_fee_idr=50000 WHERE id=$1`, s.period)
	app := seedApplication(t, a.DB, s, s.guardian, "dropped")
	other := seedApplication(t, a.DB, s, s.guardian, "dropped")

	record := func(id string, paid int64) int64 {
		tx, err := a.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		n, err := recordRefundDue(ctx, tx, s.school, id, stageReregistration, paid)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := record(app, 1000000); got != 450000 {
		t.Fatalf("refundable %d", got)
	}
	if got := record(app, 1000000); got != 450000 || countRows(t, a, `SELECT count(*) FROM admission_refunds WHERE application_id=$1`, app) != 1 {
		t.Fatal("refund recorded twice")
	}
	if n := countRows(t, a, `SELECT count(*) FROM applications WHERE id=$1 AND refund_due_idr=450000`, app); n != 1 {
		t.Fatal("refund_due_idr not set once")
	}
	if got := record(other, 40000); got != 0 || countRows(t, a, `SELECT count(*) FROM admission_refunds WHERE application_id=$1 AND status='waived'`, other) != 1 {
		t.Fatal("fee larger than refund must close the refund")
	}

	admin := identityToken(t, "admin-1", s.school, "admin", "Admin Uji")
	refund := mustID(t, a.DB, `SELECT id::text FROM admission_refunds WHERE application_id=$1`, app)
	for _, c := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"unknown method", map[string]any{"method": "cek"}, 422},
		{"transfer without account", map[string]any{"method": "transfer", "reference": "TRX1"}, 422},
		{"future date", map[string]any{"method": "cash", "reference": "KW-1", "paid_at": time.Now().Add(48 * time.Hour).Format(time.RFC3339)}, 422},
		{"transfer", map[string]any{"method": "transfer", "bank_name": "BRI", "account_name": "Wali Uji", "account_number": "1234567890", "reference": "TRX1"}, 200},
		{"twice", map[string]any{"method": "cash", "reference": "KW-1"}, 409},
	} {
		if code, out := call(t, h, "POST", "/admin/refunds/"+refund+"/process", admin, c.body); code != c.want {
			t.Fatalf("%s: %d %v", c.name, code, out)
		}
	}
	if n := countRows(t, a, `SELECT count(*) FROM applications WHERE id=$1 AND refund_due_idr=0`, app); n != 1 {
		t.Fatal("refund_due_idr not cleared")
	}
	if n := countRows(t, a, `SELECT count(*) FROM guardian_notifications WHERE application_id=$1 AND type='refund_paid'`, app); n != 1 {
		t.Fatal("guardian not notified")
	}
	code, out := call(t, h, "GET", "/admin/refunds?status=paid", admin, nil)
	items, _ := out["data"].([]any)
	if code != 200 || len(items) != 1 || items[0].(map[string]any)["account_number"] != "******7890" {
		t.Fatalf("list %d %v", code, out)
	}
	// Another school cannot touch it; a viewer role cannot process.
	foreign := identityToken(t, "admin-2", "00000000-0000-0000-0000-000000000000", "admin", "Lain")
	if code, _ := call(t, h, "POST", "/admin/refunds/"+refund+"/waive", foreign, map[string]any{"reason": "x"}); code != 404 {
		t.Fatalf("cross-school waive: %d", code)
	}
	viewer := identityToken(t, "ks-1", s.school, "kepala_sekolah", "Kepala")
	if code, _ := call(t, h, "GET", "/admin/refunds", viewer, nil); code != 403 {
		t.Fatalf("viewer: %d", code)
	}
}

// BUG-108: a SEKOCI parent session becomes a guardian session, linked once.
func TestSekociSessionExchange(t *testing.T) {
	a := testApp(t)
	h := round2Router(a)
	s := seedSchool(t, a.DB, 10)
	user := mustID(t, a.DB, `SELECT gen_random_uuid()::text`)
	parent := identityToken(t, user, s.school, "orang_tua", "Ibu Sekoci")
	var first string
	for i := 0; i < 2; i++ {
		code, out := call(t, h, "POST", "/public/session/sekoci", parent, nil)
		data, _ := out["data"].(map[string]any)
		if code != 200 || data["token"] == "" {
			t.Fatalf("exchange %d %v", code, out)
		}
		id := data["guardian"].(map[string]any)["id"].(string)
		if first == "" {
			first = id
		} else if id != first {
			t.Fatal("second login created another guardian")
		}
	}
	if n := countRows(t, a, `SELECT count(*) FROM guardians WHERE school_id=$1 AND identity_user_id=$2`, s.school, user); n != 1 {
		t.Fatalf("guardians linked: %d", n)
	}
	for _, role := range []string{"admin", "siswa"} {
		if code, _ := call(t, h, "POST", "/public/session/sekoci", identityToken(t, user, s.school, role, "X"), nil); code != 403 {
			t.Fatalf("role %s must be refused: %d", role, code)
		}
	}
	if code, _ := call(t, h, "POST", "/public/session/sekoci", "", nil); code != 401 {
		t.Fatal("anonymous exchange accepted")
	}
}
