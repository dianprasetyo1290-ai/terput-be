package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/config"
	"skci/billing/internal/model"
)

func TestSettlementBatchStatus(t *testing.T) {
	cases := []struct {
		name    string
		matched []bool
		exc     int
		want    string
	}{
		{"all items matched", []bool{true, true}, 0, "matched"},
		{"empty batch, no difference", nil, 0, "matched"},
		{"one item unmatched", []bool{true, false}, 0, "mismatch"},
		{"provider has a difference", []bool{true}, 1, "mismatch"},
	}
	for _, tc := range cases {
		if got := model.SettlementBatchStatus(tc.matched, tc.exc); got != tc.want {
			t.Errorf("%s: %s want %s", tc.name, got, tc.want)
		}
	}
}

// TestGatewayExpiryAndSettlementJobs covers the coordinator follow-ups:
// overdue pending gateway transactions are expired by a job (idempotent,
// audited), and the daily reconciliation job sets settlement batch status
// and alerts finance on a mismatch or a missing settlement (BUG-136).
func TestGatewayExpiryAndSettlementJobs(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	core := &fakeCore{}
	srv := httptest.NewServer(core.handler())
	defer srv.Close()
	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(srv.URL, "k")
	ctx := context.Background()
	now := time.Now()

	school, payer, student, method := genUUID(), genUUID(), genUUID(), genUUID()
	prov := "qa" + genUUID()[:6]
	mustExec(t, db, `INSERT INTO payment_methods (id, school_id, code, kind, provider_code, label, enabled, fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort)
		VALUES ($1,$2,'VA_QA','va','009','VA QA',true,'flat',0,0,'parent',60,1)`, method, school)
	insTx := func(status, provider, ref string, expires time.Time, paid *time.Time) string {
		id := genUUID()
		mustExec(t, db, `INSERT INTO gateway_transactions (id, school_id, tx_no, payer_user_id, student_user_id, method_id, bills_amount_idr, fee_idr, total_idr,
			fee_bearer, status, provider_ref, expires_at, paid_at, idempotency_key, provider)
			VALUES ($1,$2,$3,$4,$5,$6,100000,0,100000,'parent',$7,$8,$9,$10,$11,$12)`,
			id, school, "TX-"+id[:8], payer, student, method, status, ref, expires, paid, "idem-"+id, provider)
		return id
	}
	txStatus := func(id string) string {
		var s string
		_ = db.QueryRow(`SELECT status FROM gateway_transactions WHERE id=$1`, id).Scan(&s)
		return s
	}

	// --- expiry ---
	overdue := insTx("pending", prov, "R-OLD", now.Add(-time.Hour), nil)
	fresh := insTx("pending", prov, "R-NEW", now.Add(time.Hour), nil)
	for run := 0; run < 2; run++ {
		// The job sweeps every school in the shared test database; rows
		// left by other tests may fail (e.g. a purpose without handler
		// here) without blocking ours, which is what is asserted below.
		if err := app.runGatewayExpiry(ctx, now); err != nil {
			t.Logf("expiry run %d (other rows): %v", run, err)
		}
	}
	if txStatus(overdue) != "expired" || txStatus(fresh) != "pending" {
		t.Fatalf("expiry: overdue=%s fresh=%s", txStatus(overdue), txStatus(fresh))
	}
	var audits int
	_ = db.QueryRow(`SELECT count(*) FROM billing_audit_events WHERE aggregate_id=$1 AND action='gateway_tx_expired'`, overdue).Scan(&audits)
	if audits != 1 {
		t.Fatalf("expiry audits = %d, want 1 (idempotent)", audits)
	}

	// --- daily settlement reconciliation ---
	loc := app.location()
	yesterday := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day()-1, 10, 0, 0, 0, loc)
	day := yesterday.Format("2006-01-02")
	insTx("success", prov, "R-1", yesterday, &yesterday)
	okBatch, badBatch := genUUID(), genUUID()
	prov2 := prov + "b"
	mustExec(t, db, `INSERT INTO settlement_batches (id, school_id, provider, channel, settled_on, gross_idr, net_idr, bank_account)
		VALUES ($1,$2,$3,'va',$4,100000,100000,'BNI'), ($5,$2,$6,'va',$4,50000,50000,'BNI')`, okBatch, school, prov, day, badBatch, prov2)
	mustExec(t, db, `INSERT INTO settlement_items (school_id, batch_id, provider_ref, amount_idr) VALUES ($1,$2,'R-1',100000), ($1,$3,'R-X',50000)`,
		school, okBatch, badBatch)
	// A success three days ago whose settlement never arrived.
	old := now.In(loc).AddDate(0, 0, -3)
	prov3 := prov + "c"
	insTx("success", prov3, "R-3", old, &old)

	for run := 0; run < 2; run++ {
		if err := app.runSettlementReconciliation(ctx, now); err != nil {
			t.Logf("recon run %d (other rows): %v", run, err)
		}
	}
	batchStatus := func(id string) string {
		var s string
		_ = db.QueryRow(`SELECT status FROM settlement_batches WHERE id=$1`, id).Scan(&s)
		return s
	}
	if batchStatus(okBatch) != "matched" || batchStatus(badBatch) != "mismatch" {
		t.Fatalf("batches: ok=%s bad=%s", batchStatus(okBatch), batchStatus(badBatch))
	}
	core.mu.Lock()
	kinds := map[string]int{}
	for _, e := range core.excs {
		if e.SchoolID == school {
			kinds[e.CorrelationID]++
		}
	}
	core.mu.Unlock()
	if kinds["recon_mismatch:"+badBatch] != 1 || kinds["settlement_missing:"+prov3+":"+old.Format("2006-01-02")] != 1 || kinds["recon_mismatch:"+okBatch] != 0 {
		t.Fatalf("alerts = %v", kinds)
	}
	var batchAudits int
	_ = db.QueryRow(`SELECT count(*) FROM billing_audit_events WHERE aggregate_id IN ($1,$2) AND action='settlement_batch_reconciled'`, okBatch, badBatch).Scan(&batchAudits)
	if batchAudits != 2 {
		t.Fatalf("batch audits = %d", batchAudits)
	}
}
