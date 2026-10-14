package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// enrollmentRoster is a ClassRoster that also answers StudentEnrollment.
type enrollmentRoster struct {
	stubRoster
	grades map[string]string // enrolled students -> grade level
}

func (e enrollmentRoster) StudentEnrollment(_ context.Context, _, student string) (bool, string, error) {
	g, ok := e.grades[student]
	return ok, g, nil
}

// TestGenerateBillsGuards covers BUG-050 (COA mapping), BUG-054 (only active
// students of the scheme grade) and BUG-131 (deposit settles the new bill).
func TestGenerateBillsGuards(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID, schemeID := genUUID(), genUUID()
	budi, ani, adminUser, smp := genUUID(), genUUID(), genUUID(), genUUID()
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day, grade_level)
		VALUES ($1, $2, $3, 'SPP SMA', 'active', true, 10, '10')`, schemeID, schoolID, genUUID())
	mustExec(t, db, `INSERT INTO billing_scheme_components (school_id, scheme_id, code, name, amount_idr)
		VALUES ($1, $2, 'SPP', 'SPP', 750000)`, schoolID, schemeID)
	mustExec(t, db, `INSERT INTO student_deposits (school_id, student_user_id, balance_idr) VALUES ($1, $2, 200000)`, schoolID, budi)

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Roster = enrollmentRoster{grades: map[string]string{budi: "10", ani: "10", smp: "7"}}
	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Post("/admin/bills/generate", app.GenerateBills)
	r.Delete("/admin/journal-accounts/components/{code}", app.DeleteComponentAccount)
	actor := genUUID()
	token := signToken(t, actor, schoolID, common.RoleFinanceManager)
	do := func(method, path string, body any) (int, []byte) {
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.Bytes()
	}
	body := map[string]any{"scheme_id": schemeID, "period_year": 2026, "period_month": 7,
		"student_user_ids": []string{budi, ani, adminUser, smp}}

	// No mapping: refused, nothing issued.
	code, raw := do(http.MethodPost, "/admin/bills/generate", body)
	if code != http.StatusUnprocessableEntity || !bytes.Contains(raw, []byte("mapping akun belum lengkap")) {
		t.Fatalf("unmapped generate: %d %s", code, raw)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM student_bills WHERE school_id=$1`, schoolID).Scan(&n)
	if n != 0 {
		t.Fatalf("bills issued without mapping: %d", n)
	}

	mustExec(t, db, `INSERT INTO fee_component_accounts (school_id, component_code, name, revenue_acc, receivable_acc) VALUES ($1, 'SPP', 'SPP', '4-100', '1-300')`, schoolID)
	code, raw = do(http.MethodPost, "/admin/bills/generate", body)
	if code != http.StatusOK {
		t.Fatalf("generate: %d %s", code, raw)
	}
	var res struct {
		Data generateResult `json:"data"`
	}
	_ = json.Unmarshal(raw, &res)
	if res.Data.Generated != 2 || len(res.Data.Rejected) != 2 || res.Data.DepositAppliedIDR != 200000 {
		t.Fatalf("result = %+v", res.Data)
	}
	rejected := map[string]bool{}
	for _, rj := range res.Data.Rejected {
		rejected[rj.StudentUserID] = true
	}
	if !rejected[adminUser] || !rejected[smp] {
		t.Fatalf("admin and SMP student must be rejected: %+v", res.Data.Rejected)
	}

	var status string
	var balance, movement int64
	_ = db.QueryRow(`SELECT status FROM student_bills WHERE school_id=$1 AND student_user_id=$2`, schoolID, budi).Scan(&status)
	_ = db.QueryRow(`SELECT balance_idr FROM student_deposits WHERE school_id=$1 AND student_user_id=$2`, schoolID, budi).Scan(&balance)
	_ = db.QueryRow(`SELECT amount_idr FROM deposit_movements WHERE school_id=$1 AND student_user_id=$2`, schoolID, budi).Scan(&movement)
	if status != "partial" || balance != 0 || movement != -200000 {
		t.Fatalf("deposit apply: status=%s balance=%d movement=%d", status, balance, movement)
	}
	var actorID string
	_ = db.QueryRow(`SELECT COALESCE(actor_id::text,'') FROM billing_audit_events WHERE school_id=$1 AND action='bills_generated'`, schoolID).Scan(&actorID)
	if actorID != actor {
		t.Errorf("audit actor = %q", actorID)
	}

	// The mapping is in use by an active scheme: delete is refused.
	if code, raw := do(http.MethodDelete, "/admin/journal-accounts/components/SPP", nil); code != http.StatusConflict {
		t.Fatalf("delete in-use mapping: %d %s", code, raw)
	}

	// No academic roster: refused rather than issuing to unverified users.
	app.Roster = nil
	body["period_month"] = 8
	if code, raw := do(http.MethodPost, "/admin/bills/generate", body); code != http.StatusServiceUnavailable {
		t.Fatalf("no roster: %d %s", code, raw)
	}
}
