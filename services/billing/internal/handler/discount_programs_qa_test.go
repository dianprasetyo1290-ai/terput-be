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

// TestDiscountProgramApproval covers BUG-055: a scholarship created in the app
// stays inactive until the Approval Center approves it (maker ≠ approver),
// only applies inside its validity period and can be deactivated.
func TestDiscountProgramApproval(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	core := &fakeCore{}
	srv := httptest.NewServer(core.handler())
	defer srv.Close()

	schoolID, schemeID, year, student := genUUID(), genUUID(), genUUID(), genUUID()
	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(srv.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8080"
	app.Roster = enrollmentRoster{grades: map[string]string{student: "7"}}
	r := chi.NewRouter()
	r.Post("/internal/discounts/{id}/callback", app.DiscountCallback)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Post("/admin/bills/generate", app.GenerateBills)
		pr.Get("/admin/discounts", app.ListDiscounts)
		pr.Post("/admin/discounts", app.CreateDiscount)
		pr.Post("/admin/discounts/{id}/deactivate", app.DeactivateDiscount)
	})
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, grade_level, name, status, installment, due_day, effective_from, effective_to)
		VALUES ($1, $2, $3, '7', 'SMP', 'active', false, 10, '2026-07-01', '2027-06-30')`, schemeID, schoolID, year)
	mustExec(t, db, `INSERT INTO billing_scheme_components (school_id, scheme_id, code, name, kind, amount_idr) VALUES ($1, $2, 'SPP', 'SPP', 'monthly', 1000000)`,
		schoolID, schemeID)
	mustExec(t, db, `INSERT INTO fee_component_accounts (school_id, component_code, name, revenue_acc, receivable_acc) VALUES ($1, 'SPP', 'SPP', '4-100', '1-300')`, schoolID)

	maker, checker := genUUID(), genUUID()
	fm := apiCaller{t: t, r: r, token: signToken(t, maker, schoolID, common.RoleFinanceManager)}
	internal := apiCaller{t: t, r: r}
	netOf := func(month int) int64 {
		var net int64
		_ = db.QueryRow(`SELECT net_idr FROM student_bills WHERE school_id=$1 AND student_user_id=$2 AND period_month=$3`, schoolID, student, month).Scan(&net)
		return net
	}
	gen := func(month int) {
		if code, env := fm.do(http.MethodPost, "/admin/bills/generate", map[string]any{"scheme_id": schemeID, "period_year": 2026,
			"period_month": month, "student_user_ids": []string{student}}); code != http.StatusOK {
			t.Fatalf("generate %d: %d %v", month, code, env)
		}
	}

	for _, bad := range []map[string]any{
		{"name": "X", "kind": "percent", "value": 10001, "scope": "school", "reason": "r"},
		{"name": "X", "kind": "percent", "value": 5000, "scope": "student", "reason": "r"},
		{"name": "X", "kind": "percent", "value": 5000, "scope": "school", "reason": "r", "valid_from": "2026-09-01", "valid_to": "2026-08-01"},
		{"name": "X", "kind": "percent", "value": 5000, "scope": "school", "reason": ""},
	} {
		if code, _ := fm.do(http.MethodPost, "/admin/discounts", bad); code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid %v accepted: %d", bad, code)
		}
	}
	code, env := fm.do(http.MethodPost, "/admin/discounts", map[string]any{"name": "Beasiswa Prestasi", "kind": "percent", "value": 5000,
		"scope": "student", "student_user_id": student, "scholarship": true, "valid_from": "2026-08-01", "valid_to": "2026-08-31", "reason": "SK Yayasan 7"})
	if code != http.StatusAccepted || dataOf(env)["status"] != "pending_approval" {
		t.Fatalf("create: %d %v", code, env)
	}
	id := dataOf(env)["id"].(string)
	if appr := core.lastApproval(t); appr.RequestType != "discount_award" || appr.MakerUserID != maker {
		t.Fatalf("approval %+v", appr)
	}
	// Pending program is not applied.
	gen(8)
	if n := netOf(8); n != 1000000 {
		t.Fatalf("pending discount applied: %d", n)
	}
	cb := func(by, decision string) (int, map[string]any) {
		return internal.do(http.MethodPost, "/internal/discounts/"+id+"/callback",
			map[string]any{"approval_id": "appr-" + id, "decision": decision, "decided_by": by})
	}
	if code, _ := cb(maker, "approved"); code != http.StatusUnprocessableEntity {
		t.Fatalf("self approval: %d", code)
	}
	if code, env := cb(checker, "approved"); code != http.StatusOK || dataOf(env)["status"] != "active" {
		t.Fatalf("approve: %d %v", code, env)
	}
	if code, env := cb(checker, "rejected"); code != http.StatusOK || dataOf(env)["status"] != "active" {
		t.Fatalf("replay must not change: %d %v", code, env)
	}
	mustExec(t, db, `DELETE FROM bill_discount_steps WHERE bill_id IN (SELECT id FROM student_bills WHERE school_id=$1)`, schoolID)
	mustExec(t, db, `DELETE FROM bill_items WHERE bill_id IN (SELECT id FROM student_bills WHERE school_id=$1)`, schoolID)
	mustExec(t, db, `DELETE FROM bill_installments WHERE school_id=$1`, schoolID)
	mustExec(t, db, `DELETE FROM student_bills WHERE school_id=$1`, schoolID)
	gen(8)
	gen(9)
	if n := netOf(8); n != 500000 {
		t.Fatalf("august inside period: %d", n)
	}
	if n := netOf(9); n != 1000000 {
		t.Fatalf("september outside period: %d", n)
	}
	if code, _ := fm.do(http.MethodPost, "/admin/discounts/"+id+"/deactivate", map[string]any{"reason": ""}); code != http.StatusUnprocessableEntity {
		t.Fatalf("deactivate without reason: %d", code)
	}
	if code, env := fm.do(http.MethodPost, "/admin/discounts/"+id+"/deactivate", map[string]any{"reason": "lulus"}); code != http.StatusOK {
		t.Fatalf("deactivate: %d %v", code, env)
	}
	if code, _ := fm.do(http.MethodPost, "/admin/discounts/"+id+"/deactivate", map[string]any{"reason": "lagi"}); code != http.StatusConflict {
		t.Fatalf("second deactivate: %d", code)
	}
	code, env = fm.do(http.MethodGet, "/admin/discounts", nil)
	items, _ := env["data"].([]any)
	if code != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["status"] != "inactive" || items[0].(map[string]any)["valid_from"] != "2026-08-01" {
		t.Fatalf("list: %d %v", code, env)
	}
	var audits int
	_ = db.QueryRow(`SELECT count(*) FROM billing_audit_events WHERE aggregate_id=$1`, id).Scan(&audits)
	if audits < 3 {
		t.Fatalf("audit events %d", audits)
	}
}
