package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/admission/internal/common"
)

func round2bRouter(a *App) *chi.Mux {
	a.Config.JWTSecret = testJWTSecret
	r := chi.NewRouter()
	r.With(common.DashboardAuth(testJWTSecret)).Get("/admin/dashboard", a.AdminDashboard)
	r.Group(func(ad chi.Router) {
		ad.Use(common.AdminAuth(testJWTSecret))
		ad.Get("/admin/selection/formulas", a.ListFormulas)
		ad.Post("/admin/selection/formulas", a.CreateFormula)
		ad.Get("/admin/selection/rankings", a.LatestRanking)
		ad.Get("/admin/payments", a.PaymentQueue)
	})
	return r
}

// BUG-161: a formula belongs to an explicit wave and track of its period.
func TestFormulaRequiresScope(t *testing.T) {
	a := testApp(t)
	h := round2bRouter(a)
	s := seedSchool(t, a.DB, 10)
	other := seedSchool(t, a.DB, 10)
	admin := identityToken(t, "admin-1", s.school, "admin", "Admin Uji")
	otherPeriod := mustID(t, a.DB, `INSERT INTO admission_periods(school_id,name,academic_year,status) VALUES($1,'PPDB Lama','2026/2027','closed') RETURNING id`, s.school)
	otherWave := mustID(t, a.DB, `INSERT INTO admission_waves(school_id,period_id,name,registration_start,registration_end,payment_end,reregistration_end)
		VALUES($1,$2,'Gel Lama',now()-interval '90 day',now()-interval '60 day',now()-interval '55 day',now()-interval '30 day') RETURNING id`, s.school, otherPeriod)
	components := []map[string]any{{"name": "Tes tertulis", "source": "manual", "weight": 100}}
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"no wave/track", map[string]any{"period_id": s.period, "name": "F", "components": components}, 422},
		{"wave from other period", map[string]any{"period_id": s.period, "wave_id": otherWave, "track_id": s.track, "name": "F", "components": components}, 422},
		{"wave from other school", map[string]any{"period_id": s.period, "wave_id": other.wave, "track_id": s.track, "name": "F", "components": components}, 422},
		{"valid", map[string]any{"period_id": s.period, "wave_id": s.wave, "track_id": s.track, "name": "F", "components": components}, 201},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := call(t, h, "POST", "/admin/selection/formulas", admin, tc.body)
			if code != tc.want && !(tc.want == 201 && code == 200) {
				t.Fatalf("got %d %v", code, out)
			}
		})
	}
	code, out := call(t, h, "GET", "/admin/selection/formulas?wave_id="+s.wave+"&track_id="+s.track, admin, nil)
	items, _ := out["data"].([]any)
	if code != 200 || len(items) != 1 {
		t.Fatalf("list %d %v", code, out)
	}
	f := items[0].(map[string]any)
	if f["wave_name"] != "Gelombang 1" || f["track_name"] != "Reguler" {
		t.Fatalf("scope names missing: %v", f)
	}
	if n := countRows(t, a, `SELECT count(*) FROM audit_events WHERE school_id=$1 AND action='formula_created'`, s.school); n != 1 {
		t.Fatalf("formula_created audits: %d", n)
	}
}

// The final ranking can be reloaded with each entry's decision.
func TestLatestRanking(t *testing.T) {
	a := testApp(t)
	h := round2bRouter(a)
	s := seedSchool(t, a.DB, 1)
	admin := identityToken(t, "admin-1", s.school, "admin", "Admin Uji")
	path := "/admin/selection/rankings?wave_id=" + s.wave + "&track_id=" + s.track
	if code, out := call(t, h, "GET", path, admin, nil); code != 200 || out["data"] != nil {
		t.Fatalf("no run yet: %d %v", code, out)
	}
	if code, _ := call(t, h, "GET", "/admin/selection/rankings", admin, nil); code != 422 {
		t.Fatalf("missing scope accepted: %d", code)
	}
	first := seedApplication(t, a.DB, s, s.guardian, "offered")
	second := seedApplication(t, a.DB, s, s.guardian, "waitlisted")
	formula := mustID(t, a.DB, `INSERT INTO selection_formula_versions(school_id,period_id,wave_id,track_id,version,formula,status,checksum)
		VALUES($1,$2,$3,$4,1,'{}','published','x') RETURNING id`, s.school, s.period, s.wave, s.track)
	run := mustID(t, a.DB, `INSERT INTO ranking_runs(school_id,period_id,wave_id,track_id,formula_version_id,version,input_checksum,status,created_by)
		VALUES($1,$2,$3,$4,$5,1,'abc','final','admin-1') RETURNING id`, s.school, s.period, s.wave, s.track, formula)
	mustExec(t, a.DB, `INSERT INTO ranking_entries(school_id,ranking_run_id,application_id,rank,total_score,scores_snapshot,recommendation)
		VALUES($1,$2,$3,1,9000,'{}','accepted'),($1,$2,$4,2,7000,'{}','waitlisted')`, s.school, run, first, second)
	mustExec(t, a.DB, `INSERT INTO decisions(school_id,application_id,decision,actor_id,version,status) VALUES($1,$2,'offered','admin-1',1,'published')`, s.school, first)

	code, out := call(t, h, "GET", path, admin, nil)
	data, _ := out["data"].(map[string]any)
	if code != 200 || data == nil {
		t.Fatalf("latest %d %v", code, out)
	}
	entries := data["entries"].([]any)
	if len(entries) != 2 || data["quota"].(float64) != 1 {
		t.Fatalf("entries %v", data)
	}
	top := entries[0].(map[string]any)
	if top["decision"] != "accepted" || top["decision_status"] != "published" || top["recommendation"] != "accepted" {
		t.Fatalf("top entry %v", top)
	}
	if _, ok := entries[1].(map[string]any)["decision"]; ok {
		t.Fatalf("undecided entry carries a decision: %v", entries[1])
	}
}

// New proofs (status submitted) appear under the default "perlu review" filter.
func TestPaymentQueuePendingIncludesSubmitted(t *testing.T) {
	a := testApp(t)
	h := round2bRouter(a)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 10)
	admin := identityToken(t, "admin-1", s.school, "admin", "Admin Uji")
	app := seedApplication(t, a.DB, s, s.guardian, "submitted")
	charge, err := a.issueCharge(ctx, s.school, app, stageRegistration, issueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, status := range []string{"submitted", "pending_review", "verified"} {
		mustExec(t, a.DB, `INSERT INTO payment_submissions(school_id,application_id,charge_id,amount_idr,object_key,mime,size_bytes,sha256,status,uploader_type,uploader_id,idempotency_key)
			VALUES($1,$2,$3,100000,'k','image/png',10,'h',$4,'guardian',$5,$6)`, s.school, app, charge, status, s.guardian, "key-"+string(rune('a'+i)))
	}
	cases := []struct {
		filter string
		want   int
	}{{"pending_review", 2}, {"verified", 1}, {"", 3}, {"rejected", 0}}
	for _, tc := range cases {
		code, out := call(t, h, "GET", "/admin/payments?status="+tc.filter, admin, nil)
		data, _ := out["data"].(map[string]any)
		items, _ := data["items"].([]any)
		if code != 200 || len(items) != tc.want {
			t.Fatalf("filter %q: %d items=%d %v", tc.filter, code, len(items), out["message"])
		}
	}
}

// BUG-030: leaders read the dashboard and get filter options from it.
func TestDashboardOptionsForLeaders(t *testing.T) {
	a := testApp(t)
	h := round2bRouter(a)
	s := seedSchool(t, a.DB, 25)
	for _, role := range []string{"kepala_sekolah", "kepala_yayasan", "admin"} {
		code, out := call(t, h, "GET", "/admin/dashboard", identityToken(t, "u-"+role, s.school, role, role), nil)
		data, _ := out["data"].(map[string]any)
		if code != 200 || data == nil {
			t.Fatalf("%s: %d %v", role, code, out)
		}
		opts, _ := data["options"].(map[string]any)
		if opts == nil || len(opts["periods"].([]any)) != 1 || len(opts["waves"].([]any)) != 1 || len(opts["tracks"].([]any)) != 1 {
			t.Fatalf("%s options: %v", role, data["options"])
		}
	}
	if code, _ := call(t, h, "GET", "/admin/dashboard", identityToken(t, "g", s.school, "guru", "Guru"), nil); code != 403 {
		t.Fatalf("guru must be refused: %d", code)
	}
	if code, _ := call(t, h, "GET", "/admin/selection/formulas", identityToken(t, "k", s.school, "kepala_sekolah", "Kepsek"), nil); code != 403 {
		t.Fatalf("leaders must not reach admin endpoints: %d", code)
	}
}

// BUG-040: the application detail shows the refund owed to a dropped applicant.
func TestApplicationDetailRefunds(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 10)
	app := seedApplication(t, a.DB, s, s.guardian, "offered")
	mustExec(t, a.DB, `UPDATE admission_periods SET refund_policy_bp=5000,refund_admin_fee_idr=50000 WHERE id=$1`, s.period)
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordRefundDue(ctx, tx, s.school, app, stageReregistration, 1000000); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	mustExec(t, a.DB, `UPDATE applications SET status='dropped',drop_reason='Batas daftar ulang terlewati' WHERE id=$1`, app)
	req := httptest.NewRequest("GET", "/", nil)
	detail, err := a.applicationDetail(req, s.school, app, "guardian", s.guardian)
	if err != nil {
		t.Fatal(err)
	}
	refunds := detail["refunds"].([]map[string]any)
	if len(refunds) != 1 || refunds[0]["refundable_idr"].(int64) != 450000 || refunds[0]["status"] != "due" {
		t.Fatalf("refunds %v", refunds)
	}
	if detail["drop_reason"] != "Batas daftar ulang terlewati" {
		t.Fatalf("drop reason %v", detail["drop_reason"])
	}
	if _, err := a.applicationDetail(req, s.school, app, "guardian", mustID(t, a.DB, `SELECT gen_random_uuid()::text`)); err == nil {
		t.Fatal("another guardian read the application")
	}
}
