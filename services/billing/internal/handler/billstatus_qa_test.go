package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// TestBillStatusFollowsLedger covers BUG-008/174: a stale stored 'paid' with
// nothing paid is shown as unpaid, still appears in arrears, and carries the
// overdue flag; the gateway recompute never revives a void bill.
func TestBillStatusFollowsLedger(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID, studentID, schemeID := genUUID(), genUUID(), genUUID()
	staleBill, voidBill := genUUID(), genUUID()
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, $3, 'SPP QA', 'active', true, 10)`, schemeID, schoolID, genUUID())
	for _, b := range []struct {
		id, status string
		month      int
	}{{staleBill, "paid", 7}, {voidBill, "void", 8}} {
		mustExec(t, db, `INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month,
			base_idr, discount_idr, net_idr, status, due_at)
			VALUES ($1, $2, $3, $4, 2026, $6, 750000, 0, 750000, $5, '2026-07-09T17:00:00Z')`,
			b.id, schoolID, studentID, schemeID, b.status, b.month)
	}

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Clock = func() time.Time { return time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC) }
	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Get("/admin/bills", app.ListBills)
	r.Get("/admin/arrears", app.ListArrears)
	token := signToken(t, genUUID(), schoolID, common.RoleFinanceManager)

	get := func(path string) []map[string]any {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d %s", path, rec.Code, rec.Body.String())
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return env.Data
	}

	byID := map[string]map[string]any{}
	for _, b := range get("/admin/bills?student_user_id=" + studentID) {
		byID[b["id"].(string)] = b
	}
	stale := byID[staleBill]
	if stale == nil || stale["status"] != "unpaid" || stale["paid_idr"].(float64) != 0 || stale["overdue"] != true {
		t.Fatalf("stale bill view = %v", stale)
	}
	if v := byID[voidBill]; v == nil || v["status"] != "void" || v["overdue"] != false {
		t.Fatalf("void bill view = %v", v)
	}
	arrears := get("/admin/arrears")
	if len(arrears) != 1 || arrears[0]["id"] != staleBill {
		t.Fatalf("arrears should list the stale bill only: %v", arrears)
	}

	ctx := context.Background()
	for _, id := range []string{staleBill, voidBill} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := recomputeBillStatusInTx(ctx, tx, id); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var s1, s2 string
	_ = db.QueryRow(`SELECT status FROM student_bills WHERE id=$1`, staleBill).Scan(&s1)
	_ = db.QueryRow(`SELECT status FROM student_bills WHERE id=$1`, voidBill).Scan(&s2)
	if s1 != "unpaid" || s2 != "void" {
		t.Fatalf("recompute: stale=%s void=%s", s1, s2)
	}
	tx, _ := db.BeginTx(ctx, nil)
	defer tx.Rollback()
	if err := recomputeBillStatusInTx(ctx, tx, genUUID()); err == nil {
		t.Fatal("unknown bill must return an error")
	}
}
