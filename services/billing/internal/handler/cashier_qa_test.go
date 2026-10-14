package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"skci/billing/internal/common"
)

// TestCashierManualAllocationAndDeposit covers BUG-006/058: manual splits
// are checked against the student's own open bills, and an overpayment is
// parked as deposit (deposit_movements.ref) instead of aborting the tx.
func TestCashierManualAllocationAndDeposit(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID, studentID, otherStudent := genUUID(), genUUID(), genUUID()
	schemeID, billID, foreignBill := genUUID(), genUUID(), genUUID()
	_, srv := setupTestApp(t, db, genUUID(), studentID)
	defer srv.Close()

	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, $3, 'SPP QA', 'active', true, 10)`, schemeID, schoolID, genUUID())
	for _, b := range []struct{ id, student string }{{billID, studentID}, {foreignBill, otherStudent}} {
		mustExec(t, db, `INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month,
			base_idr, discount_idr, net_idr, status, due_at)
			VALUES ($1, $2, $3, $4, 2026, 7, 300000, 0, 300000, 'unpaid', '2026-07-10T00:00:00Z')`,
			b.id, schoolID, b.student, schemeID)
	}
	token := signToken(t, genUUID(), schoolID, common.RoleKasir)

	post := func(body map[string]any) (int, []byte) {
		payload, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/cashier/payments", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}

	cases := []struct {
		name   string
		allocs []map[string]any
		amount int64
		want   int
	}{
		{"other student's bill", []map[string]any{{"bill_id": foreignBill, "amount_idr": 100000}}, 100000, 422},
		{"above outstanding", []map[string]any{{"bill_id": billID, "amount_idr": 300001}}, 400000, 422},
		{"allocations above payment", []map[string]any{{"bill_id": billID, "amount_idr": 200000}}, 100000, 422},
	}
	for _, tc := range cases {
		code, body := post(map[string]any{"student_user_id": studentID, "amount_idr": tc.amount, "method": "cash",
			"bill_allocations": tc.allocs, "idempotency_key": genUUID()})
		if code != tc.want {
			t.Errorf("%s: status=%d body=%s", tc.name, code, body)
		}
	}

	code, body := post(map[string]any{"student_user_id": studentID, "amount_idr": 350000, "method": "cash",
		"payer_name": "Ibu Ani", "idempotency_key": genUUID()})
	if code != http.StatusOK {
		t.Fatalf("overpayment status=%d body=%s", code, body)
	}
	var res struct {
		Data struct {
			ID         string `json:"id"`
			DepositIDR int64  `json:"deposit_idr"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &res)
	if res.Data.DepositIDR != 50000 {
		t.Fatalf("deposit=%d want 50000", res.Data.DepositIDR)
	}
	var bal, cpDeposit int64
	var source, status string
	_ = db.QueryRow(`SELECT balance_idr FROM student_deposits WHERE school_id=$1 AND student_user_id=$2`, schoolID, studentID).Scan(&bal)
	_ = db.QueryRow(`SELECT deposit_idr FROM cashier_payments WHERE id=$1`, res.Data.ID).Scan(&cpDeposit)
	_ = db.QueryRow(`SELECT source FROM bill_payments WHERE student_bill_id=$1`, billID).Scan(&source)
	_ = db.QueryRow(`SELECT status FROM student_bills WHERE id=$1`, billID).Scan(&status)
	if bal != 50000 || cpDeposit != 50000 || source != "cashier" || status != "paid" {
		t.Fatalf("balance=%d cp.deposit=%d source=%s status=%s", bal, cpDeposit, source, status)
	}
	var otherStatus string
	_ = db.QueryRow(`SELECT status FROM student_bills WHERE id=$1`, foreignBill).Scan(&otherStatus)
	if otherStatus != "unpaid" {
		t.Fatalf("foreign bill touched: %s", otherStatus)
	}
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}
