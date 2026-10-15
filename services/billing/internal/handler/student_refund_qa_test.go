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

// TestStudentRefundDepositSplit covers QA-BILL round 3: a withdrawal/transfer
// refund (Menu 78) is paid out of the overpayment deposit first, then out of
// bill payments; processing the deposit portion reduces student_deposits
// with a ledger movement and an audit entry and never reverses bill
// payments, and a combined refund books Dr deposit + Dr refund / Cr cash.
func TestStudentRefundDepositSplit(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	core := &fakeCore{}
	srv := httptest.NewServer(core.handler())
	defer srv.Close()

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(srv.URL, "k")
	app.Notification = client.New(srv.URL, "k")
	app.IdentitySvc = client.New(srv.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8080"
	r := chi.NewRouter()
	r.Post("/internal/students/{studentUserId}/refund", app.InternalStudentRefund)
	r.Get("/internal/students/{studentUserId}/financial-summary", app.InternalStudentFinancialSummary)
	r.Post("/internal/refunds/{id}/callback", app.RefundCallback)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Get("/admin/refunds", app.ListRefunds)
		pr.Post("/admin/refunds/{id}/process", app.ProcessRefund)
	})

	schoolID, studentID, schemeID := genUUID(), genUUID(), genUUID()
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, $3, 'SPP QA', 'active', true, 10)`, schemeID, schoolID, genUUID())
	// Two paid bills: the older one (Aug) 1.000.000, the newer one (Sep) 500.000.
	paidBill := func(month int, amount int64, created string) (billID, payID string) {
		billID, payID, itemID := genUUID(), genUUID(), genUUID()
		mustExec(t, db, `INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month,
			base_idr, discount_idr, net_idr, status, due_at, created_at) VALUES ($1,$2,$3,$4,2026,$5,$6,0,$6,'paid','2026-09-10',$7)`,
			billID, schoolID, studentID, schemeID, month, amount, created)
		mustExec(t, db, `INSERT INTO bill_items (id, school_id, bill_id, name, component_code, amount_idr, net_idr)
			VALUES ($1,$2,$3,'SPP','SPP',$4,$4)`, itemID, schoolID, billID, amount)
		mustExec(t, db, `INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, transfer_at, reference, object_key, mime,
			size_bytes, sha256, status, idempotency_key, uploader_user_id, verified_at, source)
			VALUES ($1,$2,$3,$4,'2026-09-01T09:00:00Z','TRX','k','image/png',1,'x','verified',$5,$6,now(),'upload')`,
			payID, schoolID, billID, amount, "bp-"+payID, genUUID())
		mustExec(t, db, `INSERT INTO bill_payment_allocations (school_id, payment_id, bill_item_id, amount_idr) VALUES ($1,$2,$3,$4)`,
			schoolID, payID, itemID, amount)
		return billID, payID
	}
	_, oldPay := paidBill(8, 1000000, "2026-08-01T00:00:00Z")
	newBill, newPay := paidBill(9, 500000, "2026-09-01T00:00:00Z")
	mustExec(t, db, `INSERT INTO student_deposits (school_id, student_user_id, balance_idr) VALUES ($1,$2,300000)`, schoolID, studentID)
	mustExec(t, db, `INSERT INTO billing_journal_settings (school_id, cash_acc, refund_acc, deposit_acc) VALUES ($1,'1-1100','4-1900','2-1500')`, schoolID)

	maker, checker, fmID := genUUID(), genUUID(), genUUID()
	internal := apiCaller{t: t, r: r}
	fm := apiCaller{t: t, r: r, token: signToken(t, fmID, schoolID, common.RoleFinanceManager)}
	request := func(amount int64, key string) (int, map[string]any) {
		return internal.do(http.MethodPost, "/internal/students/"+studentID+"/refund", map[string]any{
			"school_id": schoolID, "amount_idr": amount, "reason": "Mutasi keluar", "idempotency_key": key, "requested_by": maker})
	}
	approveAndProcess := func(id, ref string) (int, map[string]any) {
		if code, env := internal.do(http.MethodPost, "/internal/refunds/"+id+"/callback",
			map[string]any{"decision": "approved", "decided_by": checker}); code != http.StatusOK {
			t.Fatalf("approve %s: %d %v", id, code, env)
		}
		return fm.do(http.MethodPost, "/admin/refunds/"+id+"/process", map[string]any{"transfer_ref": ref})
	}
	deposit := func() int64 {
		var v int64
		_ = db.QueryRow(`SELECT balance_idr FROM student_deposits WHERE school_id=$1 AND student_user_id=$2`, schoolID, studentID).Scan(&v)
		return v
	}
	reversed := func(payID string) int64 {
		var v int64
		_ = db.QueryRow(`SELECT COALESCE(sum(amount_idr),0) FROM bill_payment_reversals WHERE payment_id=$1`, payID).Scan(&v)
		return v
	}
	parts := func(refundID string) map[string]int64 {
		out := map[string]int64{}
		rows, err := db.Query(`SELECT student_bill_id::text, amount_idr FROM bill_refund_parts WHERE refund_id=$1`, refundID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var b string
			var a int64
			_ = rows.Scan(&b, &a)
			out[b] = a
		}
		return out
	}

	// 1. Requests the deposit and the bills cannot cover are refused, nothing stored.
	limitCases := []struct {
		name   string
		amount int64
		want   int
	}{
		{"zero", 0, http.StatusUnprocessableEntity},
		{"over deposit + paid bills", 300000 + 1500000 + 1, http.StatusUnprocessableEntity},
	}
	for _, tc := range limitCases {
		if code, env := request(tc.amount, "lim-"+tc.name); code != tc.want {
			t.Errorf("%s: %d %v", tc.name, code, env)
		}
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM bill_refunds WHERE school_id=$1`, schoolID).Scan(&n)
	if n != 0 {
		t.Fatalf("refused requests left %d rows", n)
	}

	// 2. Deposit only: no bill part; processing lowers the deposit and leaves payments alone.
	code, env := request(200000, "dep-only")
	if code != http.StatusCreated || dataOf(env)["deposit_idr"].(float64) != 200000 {
		t.Fatalf("deposit-only request: %d %v", code, env)
	}
	depOnly := dataOf(env)["id"].(string)
	if p := parts(depOnly); len(p) != 0 {
		t.Fatalf("deposit-only refund has bill parts %v", p)
	}
	if code, env := approveAndProcess(depOnly, "TRF-DEP-1"); code != http.StatusOK {
		t.Fatalf("process deposit-only: %d %v", code, env)
	}
	if deposit() != 100000 || reversed(oldPay) != 0 || reversed(newPay) != 0 {
		t.Fatalf("after deposit refund: deposit=%d reversed old=%d new=%d", deposit(), reversed(oldPay), reversed(newPay))
	}
	var mvAmount, mvAfter int64
	if err := db.QueryRow(`SELECT amount_idr, balance_after_idr FROM deposit_movements WHERE school_id=$1 AND ref=$2`,
		schoolID, "refund:"+depOnly).Scan(&mvAmount, &mvAfter); err != nil || mvAmount != -200000 || mvAfter != 100000 {
		t.Fatalf("deposit movement: %d %d %v", mvAmount, mvAfter, err)
	}
	var auditAfter string
	if err := db.QueryRow(`SELECT after_data::text FROM billing_audit_events WHERE aggregate_type='student_deposit' AND aggregate_id=$1 AND action='deposit_refunded'`,
		studentID).Scan(&auditAfter); err != nil {
		t.Fatalf("deposit audit missing: %v", err)
	}

	// 3. Combined: the rest of the deposit plus the newest bill.
	code, env = request(600000, "combo")
	if code != http.StatusCreated || dataOf(env)["deposit_idr"].(float64) != 100000 {
		t.Fatalf("combined request: %d %v", code, env)
	}
	combo := dataOf(env)["id"].(string)
	if p := parts(combo); len(p) != 1 || p[newBill] != 500000 {
		t.Fatalf("combined parts = %v", p)
	}
	// While pending it holds the deposit and the newer bill: only the older bill is left.
	if code, env := request(1000001, "too-much"); code != http.StatusUnprocessableEntity {
		t.Fatalf("pending refund not held: %d %v", code, env)
	}
	code, env = internal.do(http.MethodGet, "/internal/students/"+studentID+"/financial-summary?school_id="+schoolID, nil)
	if code != http.StatusOK || dataOf(env)["refundable_idr"].(float64) != 1000000 {
		t.Fatalf("summary refundable while pending: %d %v", code, env)
	}
	if code, env := approveAndProcess(combo, "TRF-COMBO-1"); code != http.StatusOK {
		t.Fatalf("process combined: %d %v", code, env)
	}
	if deposit() != 0 || reversed(newPay) != 500000 || reversed(oldPay) != 0 {
		t.Fatalf("after combined: deposit=%d reversed new=%d old=%d", deposit(), reversed(newPay), reversed(oldPay))
	}
	var newStatus string
	_ = db.QueryRow(`SELECT status FROM student_bills WHERE id=$1`, newBill).Scan(&newStatus)
	if newStatus == "paid" {
		t.Fatal("bill whose payment was refunded is still paid")
	}
	code, env = fm.do(http.MethodGet, "/admin/refunds?student_user_id="+studentID, nil)
	if items, _ := env["data"].([]any); code != http.StatusOK || len(items) != 2 {
		t.Fatalf("list: %d %v", code, env)
	}

	// 4. Deposit spent elsewhere between approval and payout: processing refuses.
	mustExec(t, db, `UPDATE student_deposits SET balance_idr=40000 WHERE school_id=$1 AND student_user_id=$2`, schoolID, studentID)
	_, env = request(40000, "late")
	late := dataOf(env)["id"].(string)
	mustExec(t, db, `UPDATE student_deposits SET balance_idr=10000 WHERE school_id=$1 AND student_user_id=$2`, schoolID, studentID)
	if code, env := approveAndProcess(late, "TRF-LATE-1"); code != http.StatusUnprocessableEntity {
		t.Fatalf("process with short deposit: %d %v", code, env)
	}
	var lateStatus string
	_ = db.QueryRow(`SELECT status FROM bill_refunds WHERE id=$1`, late).Scan(&lateStatus)
	if deposit() != 10000 || lateStatus != "approved" {
		t.Fatalf("short deposit changed state: deposit=%d status=%s", deposit(), lateStatus)
	}

	// 5. Journal of the day: refund account for the bill part, deposit liability for the deposit part.
	now := time.Now()
	draft, err := app.buildRefund(context.Background(), schoolID, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil || draft == nil {
		t.Fatalf("refund journal: %v", err)
	}
	got := map[string][2]int64{}
	for _, l := range draft.lines {
		got[l.AccCode] = [2]int64{l.DebitIDR, l.CreditIDR}
	}
	want := map[string][2]int64{"4-1900": {500000, 0}, "2-1500": {300000, 0}, "1-1100": {0, 800000}}
	if len(got) != len(want) {
		t.Fatalf("journal lines %v", got)
	}
	for acc, w := range want {
		if got[acc] != w {
			t.Errorf("journal %s = %v want %v", acc, got[acc], w)
		}
	}
	if len(draft.refs["bill_refunds"]) != 2 {
		t.Fatalf("journal refs %v", draft.refs)
	}
}
