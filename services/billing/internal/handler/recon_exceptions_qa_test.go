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

// TestReconExceptionMakerChecker covers BUG-056: a reconciliation exception
// is resolved only after another person approves in the Approval Center; an
// approved adjustment journal lands in the journal outbox, a rejection
// reopens the exception.
func TestReconExceptionMakerChecker(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	core := &fakeCore{}
	srv := httptest.NewServer(core.handler())
	defer srv.Close()

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(srv.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8080"
	r := chi.NewRouter()
	r.Post("/internal/reconciliation/exceptions/{id}/callback", app.ReconExceptionCallback)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Get("/admin/reconciliation/exceptions", app.AdminListExceptions)
		pr.Post("/admin/reconciliation/exceptions/{id}/resolve", app.AdminResolveException)
	})
	schoolID, maker, checker := genUUID(), genUUID(), genUUID()
	api := apiCaller{t: t, r: r, token: signToken(t, maker, schoolID, common.RoleFinanceManager)}
	internal := apiCaller{t: t, r: r}
	exA, exB := genUUID(), genUUID()
	for _, e := range []struct{ id, ref string }{{exA, "TRX-A"}, {exB, "TRX-B"}} {
		mustExec(t, db, `INSERT INTO reconciliation_exceptions (id, school_id, provider, kind, ref, detail)
			VALUES ($1, $2, 'nicepay', 'amount_mismatch', $3, '{"expected_idr":500000,"settled_idr":497500}')`, e.id, schoolID, e.ref)
	}
	resolve := func(id string, body map[string]any) (int, map[string]any) {
		return api.do(http.MethodPost, "/admin/reconciliation/exceptions/"+id+"/resolve", body)
	}
	status := func(id string) string {
		var s string
		_ = db.QueryRow(`SELECT status FROM reconciliation_exceptions WHERE id=$1`, id).Scan(&s)
		return s
	}
	adj := []map[string]any{
		{"acc_code": "6-1100", "description": "Biaya admin bank", "debit_idr": 2500},
		{"acc_code": "1-1200", "description": "Kliring gateway", "credit_idr": 2500},
	}

	invalid := []struct {
		name string
		body map[string]any
	}{
		{"no note", map[string]any{"adjustment_lines": adj}},
		{"unbalanced", map[string]any{"resolution_note": "x", "adjustment_lines": []map[string]any{
			{"acc_code": "6-1100", "debit_idr": 2500}, {"acc_code": "1-1200", "credit_idr": 2000}}}},
	}
	for _, tc := range invalid {
		if code, env := resolve(exA, tc.body); code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %v", tc.name, code, env)
		}
	}

	// Maker submits: not resolved, waiting for approval.
	code, env := resolve(exA, map[string]any{"resolution_note": "Potongan biaya admin bank", "adjustment_lines": adj})
	if code != http.StatusAccepted || status(exA) != "pending_approval" {
		t.Fatalf("submit: %d %v status=%s", code, env, status(exA))
	}
	appr := core.lastApproval(t)
	if appr.RequestType != "recon_adjustment" || appr.MakerUserID != maker || appr.AmountIDR == nil || *appr.AmountIDR != 2500 {
		t.Fatalf("approval = %+v", appr)
	}
	if code, _ := resolve(exA, map[string]any{"resolution_note": "lagi"}); code != http.StatusConflict {
		t.Fatalf("double submit: %d", code)
	}
	// The maker cannot approve their own resolution.
	if code, _ := internal.do(http.MethodPost, "/internal/reconciliation/exceptions/"+exA+"/callback",
		map[string]any{"decision": "approved", "decided_by": maker}); code != http.StatusUnprocessableEntity {
		t.Fatalf("self approval: %d", code)
	}
	code, env = internal.do(http.MethodPost, "/internal/reconciliation/exceptions/"+exA+"/callback",
		map[string]any{"decision": "approved", "decided_by": checker})
	if code != http.StatusOK || status(exA) != "resolved" {
		t.Fatalf("approve: %d %v", code, env)
	}
	var kind, outboxStatus string
	if err := db.QueryRow(`SELECT kind, status FROM gateway_journal_postings WHERE exception_id=$1`, exA).Scan(&kind, &outboxStatus); err != nil ||
		kind != "recon_adjustment" || outboxStatus != "pending" {
		t.Fatalf("outbox row: %v %s %s", err, kind, outboxStatus)
	}
	var resolvedBy string
	_ = db.QueryRow(`SELECT resolved_by FROM reconciliation_exceptions WHERE id=$1`, exA).Scan(&resolvedBy)
	if resolvedBy != checker {
		t.Fatalf("resolved_by = %s", resolvedBy)
	}

	// Explanation without adjustment, rejected: back to open with the note.
	if code, env := resolve(exB, map[string]any{"resolution_note": "Selisih waktu cair"}); code != http.StatusAccepted {
		t.Fatalf("submit B: %d %v", code, env)
	}
	if appr := core.lastApproval(t); appr.AmountIDR != nil {
		t.Fatalf("no-adjustment approval carries an amount: %v", *appr.AmountIDR)
	}
	internal.do(http.MethodPost, "/internal/reconciliation/exceptions/"+exB+"/callback",
		map[string]any{"decision": "rejected", "decided_by": checker, "comment": "Lampirkan mutasi H+1"})
	var note string
	_ = db.QueryRow(`SELECT decision_note FROM reconciliation_exceptions WHERE id=$1`, exB).Scan(&note)
	if status(exB) != "open" || note != "Lampirkan mutasi H+1" {
		t.Fatalf("rejected B: %s %q", status(exB), note)
	}

	// Core refuses: the exception stays open.
	core.mu.Lock()
	core.refuse = true
	core.mu.Unlock()
	if code, _ := resolve(exB, map[string]any{"resolution_note": "coba lagi"}); code != http.StatusUnprocessableEntity || status(exB) != "open" {
		t.Fatalf("refused: %d %s", code, status(exB))
	}

	code, env = api.do(http.MethodGet, "/admin/reconciliation/exceptions?status=resolved&per_page=10", nil)
	items, _ := env["data"].([]any)
	if code != http.StatusOK || len(items) != 1 || env["meta"] == nil {
		t.Fatalf("list: %d %v", code, env)
	}
}
