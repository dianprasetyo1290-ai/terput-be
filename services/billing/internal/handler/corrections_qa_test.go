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

// TestCorrectionAndResendRules covers BUG-164 (an online payment is not
// cancelled through a correction; refund instead) and BUG-163 (receipt
// resend keeps the receipt number and is rate limited per school).
func TestCorrectionAndResendRules(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	core := &fakeCore{}
	srv := httptest.NewServer(core.handler())
	defer srv.Close()

	now := time.Date(2026, 10, 14, 3, 0, 0, 0, time.UTC)
	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(srv.URL, "k")
	app.Notification = client.New(srv.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8080"
	app.Clock = func() time.Time { return now }
	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Post("/admin/payment-corrections", app.AdminCreateCorrection)
	r.Post("/admin/receipts/{id}/resend", app.AdminResendReceipt)

	schoolID, studentID, cashID, receiptID := genUUID(), genUUID(), genUUID(), genUUID()
	mustExec(t, db, `INSERT INTO cashier_payments (id, school_id, student_user_id, method, amount_idr, received_by, note, status, idempotency_key)
		VALUES ($1, $2, $3, 'cash', 100000, $4, 'x', 'recorded', $5)`, cashID, schoolID, studentID, genUUID(), "cp-"+cashID)
	mustExec(t, db, `INSERT INTO receipts (id, school_id, receipt_no, source, source_id, sent_at)
		VALUES ($1, $2, 'KW-QA-0001', 'cashier', $3, $4)`, receiptID, schoolID, cashID, now.Add(-time.Hour))
	api := apiCaller{t: t, r: r, token: signToken(t, genUUID(), schoolID, common.RoleFinanceManager)}

	corr := []struct {
		name string
		body map[string]any
		want int
	}{
		{"online payment goes to refund", map[string]any{"source": "gateway", "source_id": genUUID(), "kind": "cancel", "reason": "x"}, http.StatusUnprocessableEntity},
		{"unknown source", map[string]any{"source": "manual", "source_id": cashID, "kind": "cancel", "reason": "x"}, http.StatusUnprocessableEntity},
		{"reassign not supported", map[string]any{"source": "cashier", "source_id": cashID, "kind": "reassign", "reason": "x"}, http.StatusUnprocessableEntity},
		{"unknown cashier payment", map[string]any{"source": "cashier", "source_id": genUUID(), "kind": "cancel", "reason": "x"}, http.StatusNotFound},
		{"cashier cancel", map[string]any{"source": "cashier", "source_id": cashID, "kind": "cancel", "reason": "Salah input"}, http.StatusOK},
		{"second pending", map[string]any{"source": "cashier", "source_id": cashID, "kind": "cancel", "reason": "lagi"}, http.StatusConflict},
	}
	for _, tc := range corr {
		if code, env := api.do(http.MethodPost, "/admin/payment-corrections", tc.body); code != tc.want {
			t.Errorf("%s: %d %v", tc.name, code, env)
		}
	}
	if appr := core.lastApproval(t); appr.RequestType != "payment_correction" {
		t.Fatalf("approval = %+v", appr)
	}

	// Resend: allowed (sent an hour ago), then throttled; number unchanged.
	code, env := api.do(http.MethodPost, "/admin/receipts/"+receiptID+"/resend", nil)
	if code != http.StatusOK || dataOf(env)["receipt_no"] != "KW-QA-0001" {
		t.Fatalf("first resend: %d %v", code, env)
	}
	if code, env := api.do(http.MethodPost, "/admin/receipts/"+receiptID+"/resend", nil); code != http.StatusTooManyRequests {
		t.Fatalf("throttled resend: %d %v", code, env)
	}
	if n := core.notificationsTo(studentID); len(n) != 1 {
		t.Fatalf("notifications = %d, want 1", len(n))
	}
	var count, receipts int
	_ = db.QueryRow(`SELECT resend_count FROM receipts WHERE id=$1`, receiptID).Scan(&count)
	_ = db.QueryRow(`SELECT count(*) FROM receipts WHERE source_id=$1`, cashID).Scan(&receipts)
	if count != 1 || receipts != 1 {
		t.Fatalf("resend_count=%d receipts=%d", count, receipts)
	}
	// A school that disables the cooldown can resend at once.
	mustExec(t, db, `INSERT INTO gateway_settings (school_id, receipt_resend_cooldown_minutes) VALUES ($1, 0)`, schoolID)
	if code, env := api.do(http.MethodPost, "/admin/receipts/"+receiptID+"/resend", nil); code != http.StatusOK {
		t.Fatalf("no cooldown: %d %v", code, env)
	}
	// The transactions screen addresses a receipt by its payment id.
	if code, env := api.do(http.MethodPost, "/admin/receipts/"+cashID+"/resend", nil); code != http.StatusOK {
		t.Fatalf("resend by payment id: %d %v", code, env)
	}
}
