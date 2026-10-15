package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// TestWalletRefundFlow covers QA-BILL round 4 (FR-125-005): closing a wallet
// with disposition refund goes to core's Approval Center like every other
// refund; a refusal is reported to the exception queue and reopens the
// wallet; a rejection reopens it; an approved refund is paid out from Menu
// 25 with a refund_exit movement, closes the wallet and the closure, is
// audited, and the daily wallet journal books Dr wallet liability / Cr cash.
func TestWalletRefundFlow(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	core := &fakeCore{}
	coreSrv := httptest.NewServer(core.handler())
	defer coreSrv.Close()
	identity := chi.NewRouter()
	identity.Get("/schools/{id}/features", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusOK, []map[string]any{{"feature_key": featureWallet, "enabled": true}}, nil)
	})
	identity.Get("/internal/students/{id}/guardians", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusOK, []map[string]string{}, nil)
	})
	idSrv := httptest.NewServer(identity)
	defer idSrv.Close()

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(coreSrv.URL, "k")
	app.Notification = client.New(coreSrv.URL, "k")
	app.IdentitySvc = client.New(idSrv.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8080"
	r := chi.NewRouter()
	r.Post("/internal/refunds/{id}/callback", app.RefundCallback)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Post("/admin/wallets/{id}/close", app.AdminCloseWallet)
		pr.Get("/admin/refunds", app.ListRefunds)
		pr.Post("/admin/refunds/{id}/process", app.ProcessRefund)
	})

	schoolID, studentID := genUUID(), genUUID()
	allowance, savings := genUUID(), genUUID()
	mustExec(t, db, `INSERT INTO wallets (id, school_id, student_user_id, kind, balance_idr) VALUES ($1,$3,$4,'allowance',75000), ($2,$3,$4,'savings',120000)`,
		allowance, savings, schoolID, studentID)
	mustExec(t, db, `INSERT INTO wallet_accounts (school_id, allowance_liability_acc, savings_liability_acc, cash_acc, tenant_payable_acc, canteen_revenue_acc)
		VALUES ($1,'2-2101','2-2102','1-1101','2-2201','4-3101')`, schoolID)

	admin := apiCaller{t: t, r: r, token: signToken(t, genUUID(), schoolID, common.RoleAdmin)}
	fm := apiCaller{t: t, r: r, token: signToken(t, genUUID(), schoolID, common.RoleFinanceManager)}
	internal := apiCaller{t: t, r: r}
	checker := genUUID()
	closeWallet := func(id string) (int, map[string]any) {
		return admin.do(http.MethodPost, "/admin/wallets/"+id+"/close", map[string]any{"disposition": "refund", "reason": "Siswa keluar"})
	}
	decide := func(refundID, decision string) {
		if code, env := internal.do(http.MethodPost, "/internal/refunds/"+refundID+"/callback",
			map[string]any{"decision": decision, "decided_by": checker, "comment": "cek"}); code != http.StatusOK {
			t.Fatalf("%s %s: %d %v", decision, refundID, code, env)
		}
	}
	state := func(walletID string) (walletStatus string, balance int64, closure string) {
		_ = db.QueryRow(`SELECT status, balance_idr FROM wallets WHERE id=$1`, walletID).Scan(&walletStatus, &balance)
		_ = db.QueryRow(`SELECT status FROM wallet_closures WHERE wallet_id=$1 ORDER BY created_at DESC LIMIT 1`, walletID).Scan(&closure)
		return
	}

	// 1. Core has no refund flow: nothing stays pending, the wallet reopens,
	//    and the refusal lands in the exception queue.
	core.mu.Lock()
	core.refuse = true
	core.mu.Unlock()
	if code, env := closeWallet(allowance); code != http.StatusUnprocessableEntity {
		t.Fatalf("refused close: %d %v", code, env)
	}
	if ws, _, cl := state(allowance); ws != "active" || cl != "rejected" {
		t.Fatalf("after refusal: wallet=%s closure=%s", ws, cl)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM bill_refunds WHERE wallet_id=$1`, allowance).Scan(&n)
	core.mu.Lock()
	excs := append([]client.ExceptionEvent(nil), core.excs...)
	core.refuse = false
	core.mu.Unlock()
	if n != 0 || len(excs) != 1 || excs[0].Kind != "callback" || excs[0].Severity != "critical" {
		t.Fatalf("refusal: refunds=%d exceptions=%+v", n, excs)
	}

	// 2. Close both wallets: each is an Approval Center refund request.
	opened := map[string]string{}
	for _, wid := range []string{allowance, savings} {
		code, env := closeWallet(wid)
		if code != http.StatusOK || dataOf(env)["status"] != "pending_approval" {
			t.Fatalf("close %s: %d %v", wid, code, env)
		}
		opened[wid] = dataOf(env)["refund_id"].(string)
		appr := core.lastApproval(t)
		if appr.RequestType != reqTypeRefund || appr.SourceID != opened[wid] || appr.AmountIDR == nil ||
			appr.CallbackURL != "http://billing:8080/internal/refunds/"+opened[wid]+"/callback" {
			t.Fatalf("approval request %+v", appr)
		}
		if ws, _, cl := state(wid); ws != "closing" || cl != "pending_approval" {
			t.Fatalf("pending: wallet=%s closure=%s", ws, cl)
		}
	}
	if code, env := fm.do(http.MethodPost, "/admin/refunds/"+opened[allowance]+"/process", map[string]any{"transfer_ref": "W-0"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("process before approval: %d %v", code, env)
	}

	// 3. Rejected in core: the savings wallet is usable again, balance untouched.
	decide(opened[savings], "rejected")
	if ws, bal, cl := state(savings); ws != "active" || bal != 120000 || cl != "rejected" {
		t.Fatalf("after rejection: wallet=%s balance=%d closure=%s", ws, bal, cl)
	}

	// 4. Approved and paid out: wallet emptied with a refund_exit movement and closed.
	decide(opened[allowance], "approved")
	if _, _, cl := state(allowance); cl != "approved" {
		t.Fatalf("closure after approval = %s", cl)
	}
	if code, env := fm.do(http.MethodPost, "/admin/refunds/"+opened[allowance]+"/process", map[string]any{"transfer_ref": "W-ALW-1"}); code != http.StatusOK {
		t.Fatalf("process wallet refund: %d %v", code, env)
	}
	if ws, bal, cl := state(allowance); ws != "closed" || bal != 0 || cl != "completed" {
		t.Fatalf("after payout: wallet=%s balance=%d closure=%s", ws, bal, cl)
	}
	var kind string
	var amount, after int64
	if err := db.QueryRow(`SELECT kind, amount_idr, balance_after_idr FROM wallet_movements WHERE wallet_id=$1 AND idempotency_key=$2`,
		allowance, "wallet-refund:"+opened[allowance]).Scan(&kind, &amount, &after); err != nil || kind != "refund_exit" || amount != -75000 || after != 0 {
		t.Fatalf("movement: %s %d %d %v", kind, amount, after, err)
	}
	if err := db.QueryRow(`SELECT 1 FROM billing_audit_events WHERE aggregate_type='wallet' AND aggregate_id=$1 AND action='wallet_refunded'`,
		allowance).Scan(&n); err != nil {
		t.Fatalf("wallet audit missing: %v", err)
	}
	// Processing again is refused and moves nothing.
	if code, _ := fm.do(http.MethodPost, "/admin/refunds/"+opened[allowance]+"/process", map[string]any{"transfer_ref": "W-ALW-2"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("second process: %d", code)
	}

	// 5. The list tells wallet refunds apart from bill refunds.
	code, env := fm.do(http.MethodGet, "/admin/refunds?student_user_id="+studentID, nil)
	items, _ := env["data"].([]any)
	if code != http.StatusOK || len(items) != 2 {
		t.Fatalf("list: %d %v", code, env)
	}
	for _, it := range items {
		m := it.(map[string]any)
		if m["id"] == opened[allowance] && (m["wallet_kind"] != "allowance" || m["status"] != "processed") {
			t.Fatalf("wallet refund row %v", m)
		}
	}

	// 6. Balance changed between request and payout: refused, wallet untouched.
	code, env = closeWallet(savings)
	if code != http.StatusOK {
		t.Fatalf("re-close savings: %d %v", code, env)
	}
	again := dataOf(env)["refund_id"].(string)
	decide(again, "approved")
	mustExec(t, db, `UPDATE wallets SET balance_idr=130000 WHERE id=$1`, savings)
	if code, env := fm.do(http.MethodPost, "/admin/refunds/"+again+"/process", map[string]any{"transfer_ref": "W-SAV-1"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("changed balance: %d %v", code, env)
	}
	if ws, bal, _ := state(savings); ws != "closing" || bal != 130000 {
		t.Fatalf("changed balance moved money: wallet=%s balance=%d", ws, bal)
	}

	// 7. Daily wallet journal: Dr allowance liability / Cr wallet cash.
	now := time.Now()
	draft, err := app.buildWallet(context.Background(), schoolID, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil || draft == nil {
		t.Fatalf("wallet journal: %v", err)
	}
	got := map[string][2]int64{}
	for _, l := range draft.lines {
		got[l.AccCode] = [2]int64{l.DebitIDR, l.CreditIDR}
	}
	if len(got) != 2 || got["2-2101"] != [2]int64{75000, 0} || got["1-1101"] != [2]int64{0, 75000} {
		t.Fatalf("journal lines %v", got)
	}
	// Bill refund journal does not count the wallet refund twice.
	if d, err := app.buildRefund(context.Background(), schoolID, now.Add(-time.Hour), now.Add(time.Hour)); err != nil || d != nil {
		t.Fatalf("bill refund journal picked up a wallet refund: %+v %v", d, err)
	}
}
