package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// TestRefundApprovalCenter covers BUG-043/044/135/138: refunds are decided
// in core's Approval Center (with the amount for stage rules), finance
// managers can request them, a rejection notifies requester and guardians
// without touching the paid balance, processing needs a transfer reference,
// and the list is paginated.
func TestRefundApprovalCenter(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	core := &fakeCore{guardians: []string{genUUID()}}
	srv := httptest.NewServer(core.handler())
	defer srv.Close()

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(srv.URL, "k")
	app.Notification = client.New(srv.URL, "k")
	app.IdentitySvc = client.New(srv.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8080"
	app.Clock = func() time.Time { return time.Date(2026, 10, 14, 3, 0, 0, 0, time.UTC) }
	r := chi.NewRouter()
	r.Post("/internal/refunds/{id}/callback", app.RefundCallback)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Get("/admin/refunds", app.ListRefunds)
		pr.Post("/admin/refunds", app.CreateRefund)
		pr.Post("/admin/refunds/{id}/decision", app.DecideRefund)
		pr.Post("/admin/refunds/{id}/process", app.ProcessRefund)
	})

	schoolID, studentID, schemeID, billID, payID := genUUID(), genUUID(), genUUID(), genUUID(), genUUID()
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, $3, 'SPP QA', 'active', true, 10)`, schemeID, schoolID, genUUID())
	mustExec(t, db, `INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month,
		base_idr, discount_idr, net_idr, status, due_at) VALUES ($1, $2, $3, $4, 2026, 9, 7500000, 0, 7500000, 'paid', '2026-09-10')`,
		billID, schoolID, studentID, schemeID)
	mustExec(t, db, `INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, transfer_at, reference, object_key, mime, size_bytes, sha256,
		status, idempotency_key, uploader_user_id, verified_at, source)
		VALUES ($1, $2, $3, 7500000, '2026-09-01T09:00:00Z', 'TRX123', 'k', 'image/png', 1, 'x', 'verified', $4, $5, now(), 'upload')`,
		payID, schoolID, billID, "bp-"+payID, genUUID())
	itemID := genUUID()
	mustExec(t, db, `INSERT INTO bill_items (id, school_id, bill_id, name, component_code, amount_idr, net_idr)
		VALUES ($1, $2, $3, 'SPP', 'SPP', 7500000, 7500000)`, itemID, schoolID, billID)
	mustExec(t, db, `INSERT INTO bill_payment_allocations (school_id, payment_id, bill_item_id, amount_idr) VALUES ($1, $2, $3, 7500000)`,
		schoolID, payID, itemID)

	maker, checker := genUUID(), genUUID()
	fm := apiCaller{t: t, r: r, token: signToken(t, maker, schoolID, common.RoleFinanceManager)}
	other := apiCaller{t: t, r: r, token: signToken(t, genUUID(), schoolID, common.RoleFinanceManager)}
	internal := apiCaller{t: t, r: r}
	callback := func(id string, body map[string]any) (int, map[string]any) {
		return internal.do(http.MethodPost, "/internal/refunds/"+id+"/callback", body)
	}
	paidBalance := func() int64 {
		var v int64
		_ = db.QueryRow(`SELECT p.amount_idr - COALESCE((SELECT sum(amount_idr) FROM bill_payment_reversals WHERE payment_id=p.id),0)
			FROM bill_payments p WHERE p.id=$1`, payID).Scan(&v)
		return v
	}

	// 1. Finance manager requests a large refund: sent to core with the amount.
	code, env := fm.do(http.MethodPost, "/admin/refunds", map[string]any{"student_bill_id": billID, "amount_idr": 6000000, "reason": "Kelebihan bayar"})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, env)
	}
	rejectID := dataOf(env)["id"].(string)
	appr := core.lastApproval(t)
	if appr.RequestType != "refund" || appr.AmountIDR == nil || *appr.AmountIDR != 6000000 || appr.MakerUserID != maker ||
		appr.CallbackURL != "http://billing:8080/internal/refunds/"+rejectID+"/callback" {
		t.Fatalf("approval request = %+v", appr)
	}
	// Local decision is no longer possible for Approval Center refunds.
	if code, env := other.do(http.MethodPost, "/admin/refunds/"+rejectID+"/decision", map[string]any{"action": "approve"}); code != http.StatusConflict {
		t.Fatalf("local decision: %d %v", code, env)
	}
	// The pending refund holds the refundable balance.
	if code, env := fm.do(http.MethodPost, "/admin/refunds", map[string]any{"student_bill_id": billID, "amount_idr": 2000000, "reason": "x"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("over-refund: %d %v", code, env)
	}

	cbCases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"unknown decision", map[string]any{"decision": "maybe", "decided_by": checker}, http.StatusUnprocessableEntity},
		{"maker approves own", map[string]any{"decision": "approved", "decided_by": maker}, http.StatusUnprocessableEntity},
		{"wrong approval id", map[string]any{"approval_id": "other", "decision": "approved", "decided_by": checker}, http.StatusConflict},
	}
	for _, tc := range cbCases {
		if code, env := callback(rejectID, tc.body); code != tc.want {
			t.Errorf("%s: %d %v", tc.name, code, env)
		}
	}

	// 2. Rejected in core: requester and guardian are told, balance unchanged.
	if code, env := callback(rejectID, map[string]any{"approval_id": "appr-" + rejectID, "decision": "rejected", "decided_by": checker, "comment": "Sudah dikompensasi"}); code != http.StatusOK || dataOf(env)["status"] != "rejected" {
		t.Fatalf("reject: %d %v", code, env)
	}
	if paidBalance() != 7500000 {
		t.Fatalf("rejection changed the paid balance: %d", paidBalance())
	}
	if n := core.notificationsTo(maker); len(n) != 1 || n[0].Type != "refund_rejected" {
		t.Fatalf("requester notifications = %+v", n)
	}
	if n := core.notificationsTo(core.guardians[0]); len(n) != 1 || n[0].Type != "refund_rejected" {
		t.Fatalf("guardian notifications = %+v", n)
	}
	// Replay is a no-op.
	if code, env := callback(rejectID, map[string]any{"decision": "approved", "decided_by": checker}); code != http.StatusOK || dataOf(env)["status"] != "rejected" {
		t.Fatalf("replay: %d %v", code, env)
	}

	// 3. Approved then processed against a transfer reference.
	_, env = fm.do(http.MethodPost, "/admin/refunds", map[string]any{"student_bill_id": billID, "amount_idr": 500000, "reason": "Batal kegiatan"})
	okID := dataOf(env)["id"].(string)
	if code, env := fm.do(http.MethodPost, "/admin/refunds/"+okID+"/process", map[string]any{"transfer_ref": "TRF-1"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("process before approval: %d %v", code, env)
	}
	if code, env := callback(okID, map[string]any{"decision": "approved", "decided_by": checker}); code != http.StatusOK || dataOf(env)["status"] != "approved" {
		t.Fatalf("approve: %d %v", code, env)
	}
	procCases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"missing transfer ref", map[string]any{}, http.StatusUnprocessableEntity},
		{"future transfer date", map[string]any{"transfer_ref": "TRF-1", "transfer_date": "2026-10-20"}, http.StatusUnprocessableEntity},
		{"valid", map[string]any{"transfer_ref": "BCA-REF-001", "transfer_date": "2026-10-13", "note": "rek. ortu"}, http.StatusOK},
		{"already processed", map[string]any{"transfer_ref": "BCA-REF-002"}, http.StatusUnprocessableEntity},
	}
	for _, tc := range procCases {
		if code, env := fm.do(http.MethodPost, "/admin/refunds/"+okID+"/process", tc.body); code != tc.want {
			t.Errorf("%s: %d %v", tc.name, code, env)
		}
	}
	if paidBalance() != 7000000 {
		t.Fatalf("paid balance after refund = %d", paidBalance())
	}
	var ref string
	_ = db.QueryRow(`SELECT transfer_ref FROM bill_refunds WHERE id=$1`, okID).Scan(&ref)
	if ref != "BCA-REF-001" {
		t.Fatalf("transfer_ref = %q", ref)
	}
	if n := core.notificationsTo(core.guardians[0]); len(n) != 2 || n[1].Type != "refund_processed" {
		t.Fatalf("guardian payout notification = %+v", n)
	}

	// 4. Same transfer reference cannot pay out a second refund.
	_, env = fm.do(http.MethodPost, "/admin/refunds", map[string]any{"student_bill_id": billID, "amount_idr": 100000, "reason": "Selisih"})
	dupID := dataOf(env)["id"].(string)
	_, _ = callback(dupID, map[string]any{"decision": "approved", "decided_by": checker})
	if code, env := fm.do(http.MethodPost, "/admin/refunds/"+dupID+"/process", map[string]any{"transfer_ref": "BCA-REF-001"}); code != http.StatusConflict {
		t.Fatalf("duplicate transfer ref: %d %v", code, env)
	}

	// 5. Core refuses (no refund flow): no refund row is left behind.
	core.mu.Lock()
	core.refuse = true
	core.mu.Unlock()
	if code, env := fm.do(http.MethodPost, "/admin/refunds", map[string]any{"student_bill_id": billID, "amount_idr": 1000, "reason": "x"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("refused: %d %v", code, env)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM bill_refunds WHERE student_bill_id=$1`, billID).Scan(&n)
	if n != 3 {
		t.Fatalf("refund rows = %d, want 3", n)
	}

	// 6. Pagination with meta.
	code, env = fm.do(http.MethodGet, "/admin/refunds?per_page=2&page=2&student_user_id="+studentID, nil)
	meta, _ := env["meta"].(map[string]any)
	items, _ := env["data"].([]any)
	if code != http.StatusOK || meta["total"].(float64) != 3 || meta["per_page"].(float64) != 2 || len(items) != 1 {
		t.Fatalf("pagination: %d meta=%v items=%d", code, meta, len(items))
	}
}
