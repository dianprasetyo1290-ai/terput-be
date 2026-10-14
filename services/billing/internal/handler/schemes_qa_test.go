package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// TestSchemeTariffControls covers BUG-051/086/124/127/171/172.
func TestSchemeTariffControls(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	core := &fakeCore{}
	ds := httptest.NewServer(core.handler())
	defer ds.Close()

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(ds.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8080"
	r := chi.NewRouter()
	r.Post("/internal/schemes/changes/{id}/callback", app.SchemeChangeCallback)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Post("/admin/schemes", app.CreateScheme)
		pr.Put("/admin/schemes/{id}", app.UpdateScheme)
		pr.Get("/admin/schemes/{id}", app.GetScheme)
		pr.Get("/admin/schemes/{id}/changes", app.ListSchemeChanges)
	})
	schoolID, year, maker := genUUID(), genUUID(), genUUID()
	api := apiCaller{t: t, r: r, token: signToken(t, maker, schoolID, common.RoleFinanceManager)}
	internal := apiCaller{t: t, r: r}

	scheme := func(name string, comps ...map[string]any) map[string]any {
		return map[string]any{"academic_year_id": year, "grade_level": "99", "name": name, "status": "active", "components": comps}
	}
	spp := func(amount int64) map[string]any {
		return map[string]any{"code": "SPP", "name": "SPP", "kind": "monthly", "amount_idr": amount}
	}

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"duplicate component code", scheme("Dup", spp(1), map[string]any{"code": "spp", "name": "x", "amount_idr": 2}), http.StatusUnprocessableEntity},
		{"zero tariff without free flag", scheme("Zero", spp(0)), http.StatusUnprocessableEntity},
		{"negative tariff", scheme("Neg", spp(-1)), http.StatusUnprocessableEntity},
		{"bad window", func() map[string]any {
			b := scheme("Win", spp(1))
			b["effective_from"], b["effective_to"] = "2026-12-01", "2026-07-01"
			return b
		}(), http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		if code, env := api.do(http.MethodPost, "/admin/schemes", tc.body); code != tc.want {
			t.Errorf("%s: %d %v", tc.name, code, env)
		}
	}

	// Free zero-tariff component is accepted when flagged.
	free := scheme("Gratis", map[string]any{"code": "KEG", "name": "Kegiatan", "amount_idr": 0, "is_free": true})
	free["grade_level"] = "98"
	if code, env := api.do(http.MethodPost, "/admin/schemes", free); code != http.StatusCreated {
		t.Fatalf("free component: %d %v", code, env)
	}

	b := scheme("[QA] Skema B", spp(500000))
	b["effective_to"] = "2026-12-31"
	code, env := api.do(http.MethodPost, "/admin/schemes", b)
	if code != http.StatusCreated {
		t.Fatalf("create B: %d %v", code, env)
	}
	schemeB := dataOf(env)["id"].(string)

	// Same year + grade + component with an overlapping window: double tariff.
	if code, env := api.do(http.MethodPost, "/admin/schemes", scheme("[QA] Skema C", spp(550000))); code != http.StatusConflict {
		t.Fatalf("double tariff: %d %v", code, env)
	}
	// A follow-up tariff from January is a new validity window, not a double.
	c := scheme("[QA] Skema C 2027", spp(550000))
	c["effective_from"] = "2027-01-01"
	if code, env := api.do(http.MethodPost, "/admin/schemes", c); code != http.StatusCreated {
		t.Fatalf("next window: %d %v", code, env)
	}

	// Renaming an active scheme applies directly (no tariff change).
	rename := scheme("[QA] Skema B Reguler", spp(500000))
	rename["effective_to"] = "2026-12-31"
	if code, env := api.do(http.MethodPut, "/admin/schemes/"+schemeB, rename); code != http.StatusOK {
		t.Fatalf("rename: %d %v", code, env)
	}

	// Tariff change: not applied, sent to the Approval Center.
	raise := scheme("[QA] Skema B Reguler", spp(550000))
	raise["effective_to"] = "2026-12-31"
	code, env = api.do(http.MethodPut, "/admin/schemes/"+schemeB, raise)
	if code != http.StatusAccepted || dataOf(env)["status"] != "pending_approval" {
		t.Fatalf("tariff change: %d %v", code, env)
	}
	changeID := dataOf(env)["change_id"].(string)
	appr := core.lastApproval(t)
	if appr.RequestType != "fee_change" || appr.MakerUserID != maker || appr.AmountIDR == nil || *appr.AmountIDR != 550000 ||
		appr.CallbackURL != "http://billing:8080/internal/schemes/changes/"+changeID+"/callback" {
		t.Fatalf("approval request = %+v", appr)
	}
	var amount int64
	_ = db.QueryRow(`SELECT amount_idr FROM billing_scheme_components WHERE scheme_id=$1`, schemeB).Scan(&amount)
	if amount != 500000 {
		t.Fatalf("tariff applied before approval: %d", amount)
	}
	// A second change while one is pending is refused.
	if code, env := api.do(http.MethodPut, "/admin/schemes/"+schemeB, raise); code != http.StatusConflict {
		t.Fatalf("second pending change: %d %v", code, env)
	}

	approver := genUUID()
	if code, env := internal.do(http.MethodPost, "/internal/schemes/changes/"+changeID+"/callback",
		map[string]any{"approval_id": appr.SourceID, "decision": "approved", "decided_by": approver}); code != http.StatusOK || dataOf(env)["status"] != "applied" {
		t.Fatalf("approve callback: %d %v", code, env)
	}
	_ = db.QueryRow(`SELECT amount_idr FROM billing_scheme_components WHERE scheme_id=$1`, schemeB).Scan(&amount)
	if amount != 550000 {
		t.Fatalf("approved tariff not applied: %d", amount)
	}
	// Replayed callback is acknowledged without side effects.
	if code, env := internal.do(http.MethodPost, "/internal/schemes/changes/"+changeID+"/callback",
		map[string]any{"decision": "rejected"}); code != http.StatusOK || dataOf(env)["status"] != "applied" {
		t.Fatalf("replay: %d %v", code, env)
	}

	// Audit: actor plus before/after amounts, locally and centrally.
	var actor string
	var before, after []byte
	_ = db.QueryRow(`SELECT actor_id, before_data, after_data FROM billing_audit_events
		WHERE aggregate_id=$1 AND action='scheme_change_applied'`, schemeB).Scan(&actor, &before, &after)
	if actor != approver || !containsAll(string(before), "500000") || !containsAll(string(after), "550000") {
		t.Fatalf("audit actor=%s before=%s after=%s", actor, before, after)
	}
	core.mu.Lock()
	central := len(core.audits)
	core.mu.Unlock()
	if central == 0 {
		t.Fatal("no central audit event")
	}

	// Core refuses (no fee_change flow): nothing is left pending.
	core.mu.Lock()
	core.refuse = true
	core.mu.Unlock()
	raise2 := scheme("[QA] Skema B Reguler", spp(600000))
	raise2["effective_to"] = "2026-12-31"
	code, env = api.do(http.MethodPut, "/admin/schemes/"+schemeB, raise2)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("refused approval: %d %v", code, env)
	}
	var pending int
	_ = db.QueryRow(`SELECT count(*) FROM billing_scheme_changes WHERE scheme_id=$1 AND status='pending_approval'`, schemeB).Scan(&pending)
	if pending != 0 {
		t.Fatalf("pending changes left behind: %d", pending)
	}
	if code, env := api.do(http.MethodGet, "/admin/schemes/"+schemeB+"/changes", nil); code != http.StatusOK || env["meta"] == nil {
		t.Fatalf("list changes: %d %v", code, env)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
