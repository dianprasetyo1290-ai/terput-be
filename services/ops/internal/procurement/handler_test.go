package procurement

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"skci/ops/internal/crypto"
)

func setupTestDB(t *testing.T) (*sql.DB, *crypto.Service, func()) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("skipping DB test: TEST_DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Skip("skipping DB test: cannot open connection:", err)
	}
	if err := db.Ping(); err != nil {
		t.Skip("skipping DB test: cannot ping database:", err)
	}

	for _, f := range []string{
		"../../migrations/0001_ops_base.up.sql",
		"../../migrations/0002_facility.up.sql",
		"../../migrations/0010_inventory.up.sql",
		"../../migrations/0011_procurement.up.sql",
	} {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("exec migration %s: %v", f, err)
		}
	}

	c, err := crypto.New([]byte("12345678901234567890123456789012"), "test-blind-key-123456")
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		_, _ = db.Exec(`
			DROP VIEW IF EXISTS pr_open_commitment_v CASCADE;
			DROP VIEW IF EXISTS po_line_match_v CASCADE;
			DROP TABLE IF EXISTS vendor_evaluations CASCADE;
			DROP TABLE IF EXISTS goods_returns CASCADE;
			DROP TABLE IF EXISTS goods_receipt_lines CASCADE;
			DROP TABLE IF EXISTS goods_receipts CASCADE;
			DROP TABLE IF EXISTS purchase_order_lines CASCADE;
			DROP TABLE IF EXISTS purchase_orders CASCADE;
			DROP TABLE IF EXISTS purchase_request_lines CASCADE;
			DROP TABLE IF EXISTS purchase_requests CASCADE;
			DROP TABLE IF EXISTS vendor_accounting_sync CASCADE;
			DROP TABLE IF EXISTS vendor_bank_accounts CASCADE;
			DROP TABLE IF EXISTS vendor_documents CASCADE;
			DROP TABLE IF EXISTS vendor_contacts CASCADE;
			DROP TABLE IF EXISTS vendors CASCADE;
			DROP TABLE IF EXISTS inventory_settings CASCADE;
			DROP TABLE IF EXISTS stock_count_lines CASCADE;
			DROP TABLE IF EXISTS stock_counts CASCADE;
			DROP TABLE IF EXISTS stock_movements CASCADE;
			DROP TABLE IF EXISTS stock_balances CASCADE;
			DROP TABLE IF EXISTS warehouses CASCADE;
			DROP TABLE IF EXISTS items CASCADE;
			DROP TABLE IF EXISTS item_categories CASCADE;
			DROP TABLE IF EXISTS doc_sequences CASCADE;
			DROP TABLE IF EXISTS audit_events CASCADE;
		`)
		db.Close()
	}
	return db, c, cleanup
}

func TestProcurementE2E(t *testing.T) {
	db, c, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	schoolID := "22222222-2222-2222-2222-222222222222"
	store := NewStore(db, c)
	handler := NewHandler(store, nil, nil, "http://localhost:8093")

	// 1. Create Items (one stock, one asset)
	var catStockID, catAssetID string
	_ = db.QueryRow(`INSERT INTO item_categories (school_id, code, name, tracking) VALUES ($1, 'CAT-STK', 'Barang Habis Pakai', 'stock') RETURNING id`, schoolID).Scan(&catStockID)
	_ = db.QueryRow(`INSERT INTO item_categories (school_id, code, name, tracking) VALUES ($1, 'CAT-AST', 'Aset Elektronik', 'asset') RETURNING id`, schoolID).Scan(&catAssetID)

	var itemChairID, itemLaptopID string
	_ = db.QueryRow(`INSERT INTO items (school_id, code, name, category_id, unit) VALUES ($1, 'ITM-CHR', 'Kursi Siswa', $2, 'unit') RETURNING id`, schoolID, catStockID).Scan(&itemChairID)
	_ = db.QueryRow(`INSERT INTO items (school_id, code, name, category_id, unit) VALUES ($1, 'ITM-LPT', 'Laptop Guru', $2, 'unit') RETURNING id`, schoolID, catAssetID).Scan(&itemLaptopID)

	// 2. Vendor Management & AC-106-01
	v, err := store.CreateVendor(ctx, Vendor{
		SchoolID:         schoolID,
		Code:             "V-001",
		Name:             "PT Sumber Makmur",
		NPWP:             "01.234.567.8-999.000",
		Categories:       []string{"furniture", "it"},
		PaymentTermsDays: intPtr(30),
		Status:           "active",
	})
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}

	testUserID := "33333333-3333-3333-3333-333333333333"

	// Add First Bank Account (simulated approved)
	b1, err := store.AddBankAccount(ctx, VendorBankAccount{
		SchoolID:      schoolID,
		VendorID:      v.ID,
		BankName:      "BCA",
		AccountHolder: "PT Sumber Makmur",
		RequestedBy:   testUserID,
	}, "1234567890")
	if err != nil {
		t.Fatalf("add bank account 1: %v", err)
	}
	if err := store.ActivateBankAccount(ctx, schoolID, b1.ID); err != nil {
		t.Fatalf("activate bank account 1: %v", err)
	}

	// Add Second Bank Account -> Should be pending_approval
	b2, err := store.AddBankAccount(ctx, VendorBankAccount{
		SchoolID:      schoolID,
		VendorID:      v.ID,
		BankName:      "Mandiri",
		AccountHolder: "PT Sumber Makmur",
		RequestedBy:   testUserID,
	}, "9876543210")
	if err != nil {
		t.Fatalf("add bank account 2: %v", err)
	}

	// AC-106-01: b1 remains active until b2 is approved
	vRefreshed, _ := store.GetVendor(ctx, schoolID, v.ID)
	var activeCount int
	for _, b := range vRefreshed.BankAccounts {
		if b.Status == "active" {
			activeCount++
			if b.ID != b1.ID {
				t.Fatalf("expected b1 to be active, got %s", b.ID)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active bank account, got %d", activeCount)
	}

	// Now approval callback arrives for b2
	err = handler.ApplyApprovalDecision(ctx, ApprovalDecision{
		ApprovalID:  "bbbbbbbb-0002-0000-0000-000000000002",
		SourceID:    b2.ID,
		RequestType: "vendor_bank_change",
		Decision:    "approved",
	})
	if err != nil {
		t.Fatalf("approve b2: %v", err)
	}

	// Now b2 should be active, and b1 superseded
	vRefreshed, _ = store.GetVendor(ctx, schoolID, v.ID)
	for _, b := range vRefreshed.BankAccounts {
		if b.ID == b2.ID && b.Status != "active" {
			t.Fatalf("expected b2 to be active, got %s", b.Status)
		}
		if b.ID == b1.ID && b.Status != "superseded" {
			t.Fatalf("expected b1 to be superseded, got %s", b.Status)
		}
	}

	// 3. Purchase Request & AC-104-01
	prDocNo, _ := store.NextDocNumber(ctx, schoolID, "PR", 2026, "PR-{yyyy}-{seq:4}")
	pr, err := store.CreatePR(ctx, PurchaseRequest{
		SchoolID:        schoolID,
		Number:          prDocNo,
		RequesterUserID: testUserID,
		Reason:          "Kebutuhan Lab Komputer",
	}, []PurchaseRequestLine{
		{ItemID: &itemLaptopID, Description: "Laptop Guru", Qty: 2, Unit: "unit", EstUnitPriceIDR: 10000000},
		{ItemID: &itemChairID, Description: "Kursi", Qty: 10, Unit: "unit", EstUnitPriceIDR: 250000},
	}, "pr-key-1")
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	if pr.TotalEstIDR != 22500000 {
		t.Fatalf("expected total 22500000, got %d", pr.TotalEstIDR)
	}

	// Test AC-104-01 BudgetCheck hook integration
	BudgetCheck = func(ctx context.Context, tx *sql.Tx, in BudgetCheckInput) (BudgetCheckResult, error) {
		return BudgetCheckResult{
			Policy: "extra_approval",
			Over:   true,
		}, nil
	}
	defer func() { BudgetCheck = nil }()
	bRes, _ := BudgetCheck(ctx, nil, BudgetCheckInput{})
	flags, _ := ApprovalFlags(&bRes)
	if len(flags) != 1 || flags[0] != "over_budget" {
		t.Fatalf("expected over_budget flag, got %v", flags)
	}

	// Submit PR with over budget flag
	err = store.SubmitPR(ctx, schoolID, pr.ID, "aaaaaaaa-0001-0000-0000-000000000001", true, map[string]any{"over": true})
	if err != nil {
		t.Fatalf("submit PR: %v", err)
	}

	// Core approves PR
	err = handler.ApplyApprovalDecision(ctx, ApprovalDecision{
		ApprovalID:  "aaaaaaaa-0001-0000-0000-000000000001",
		SourceID:    pr.ID,
		RequestType: "purchase_request",
		Decision:    "approved",
	})
	if err != nil {
		t.Fatalf("approve PR: %v", err)
	}

	prRefreshed, _ := store.GetPR(ctx, schoolID, pr.ID)
	if prRefreshed.Status != "approved" {
		t.Fatalf("expected PR status approved, got %s", prRefreshed.Status)
	}

	// 4. Create Purchase Order from PR
	var prLineLaptopID, prLineChairID string
	for _, l := range prRefreshed.Lines {
		if l.ItemID != nil && *l.ItemID == itemLaptopID {
			prLineLaptopID = l.ID
		}
		if l.ItemID != nil && *l.ItemID == itemChairID {
			prLineChairID = l.ID
		}
	}

	poDocNo, _ := store.NextDocNumber(ctx, schoolID, "PO", 2026, "PO-{yyyy}-{seq:4}")
	po, err := store.CreatePO(ctx, PurchaseOrder{
		SchoolID:         schoolID,
		Number:           poDocNo,
		VendorID:         v.ID,
		PaymentTermsDays: 30,
	}, []PurchaseOrderLine{
		{PRLineID: &prLineLaptopID, ItemID: &itemLaptopID, Description: "Laptop Guru", Qty: 2, UnitPriceIDR: 9500000},
		{PRLineID: &prLineChairID, ItemID: &itemChairID, Description: "Kursi Siswa", Qty: 10, UnitPriceIDR: 250000},
	}, "po-key-1")
	if err != nil {
		t.Fatalf("create PO: %v", err)
	}
	if po.TotalIDR != 21500000 {
		t.Fatalf("expected PO total 21500000, got %d", po.TotalIDR)
	}

	// PR should now be 'ordered'
	prRefreshed, _ = store.GetPR(ctx, schoolID, pr.ID)
	if prRefreshed.Status != "ordered" {
		t.Fatalf("expected PR status ordered after PO creation, got %s", prRefreshed.Status)
	}

	// Submit and Approve PO
	_ = store.SubmitPO(ctx, schoolID, po.ID, "cccccccc-0001-0000-0000-000000000001")
	err = handler.ApplyApprovalDecision(ctx, ApprovalDecision{
		ApprovalID:  "cccccccc-0001-0000-0000-000000000001",
		SourceID:    po.ID,
		RequestType: "purchase_order",
		Decision:    "approved",
	})
	if err != nil {
		t.Fatalf("approve PO: %v", err)
	}

	poRefreshed, _ := store.GetPO(ctx, schoolID, po.ID)
	var poLineLaptopID, poLineChairID string
	for _, l := range poRefreshed.Lines {
		if l.ItemID != nil && *l.ItemID == itemLaptopID {
			poLineLaptopID = l.ID
		}
		if l.ItemID != nil && *l.ItemID == itemChairID {
			poLineChairID = l.ID
		}
	}

	// 5. Test Asset Hook Failure Rollbacks Receipt (AC-105-02)
	OnGoodsReceived = func(ctx context.Context, tx *sql.Tx, ev GoodsReceivedEvent) error {
		return errors.New("simulated asset service failure")
	}
	_, err = store.CreateReceipt(ctx, GoodsReceipt{
		SchoolID:   schoolID,
		Number:     "GR-FAIL",
		POID:       po.ID,
		ReceivedOn: time.Now(),
		ReceivedBy: testUserID,
	}, []GoodsReceiptLine{
		{POLineID: poLineLaptopID, QtyReceived: 1, QtyAccepted: 1, QtyRejected: 0},
	}, "gr-key-fail")
	if err == nil {
		t.Fatalf("expected failure when asset hook returns error")
	}

	// Now set asset hook to succeed
	assetHookCalled := false
	var receivedAssetUnits int64
	OnGoodsReceived = func(ctx context.Context, tx *sql.Tx, ev GoodsReceivedEvent) error {
		assetHookCalled = true
		for _, l := range ev.Lines {
			receivedAssetUnits += l.QtyAccepted
		}
		return nil
	}
	defer func() { OnGoodsReceived = nil }()

	// Receive Partial: 6 chairs accepted, 2 chairs rejected, 2 laptops accepted
	grDocNo, _ := store.NextDocNumber(ctx, schoolID, "GR", 2026, "GR-{yyyy}-{seq:4}")
	gr, err := store.CreateReceipt(ctx, GoodsReceipt{
		SchoolID:   schoolID,
		Number:     grDocNo,
		POID:       po.ID,
		ReceivedOn: time.Now(),
		ReceivedBy: testUserID,
	}, []GoodsReceiptLine{
		{POLineID: poLineLaptopID, QtyReceived: 2, QtyAccepted: 2, QtyRejected: 0},
		{POLineID: poLineChairID, QtyReceived: 8, QtyAccepted: 6, QtyRejected: 2, RejectReason: strPtr("Patah")},
	}, "gr-key-1")
	if err != nil {
		t.Fatalf("create goods receipt: %v", err)
	}

	// AC-105-02: Asset hook was called with 2 units
	if !assetHookCalled || receivedAssetUnits != 2 {
		t.Fatalf("AC-105-02: expected asset hook called with 2 units, got %v / %d", assetHookCalled, receivedAssetUnits)
	}

	// AC-105-01: PO status is 'partially_received' (chairs remaining: 10 - 6 = 4)
	poRefreshed, _ = store.GetPO(ctx, schoolID, po.ID)
	if poRefreshed.Status != "partially_received" {
		t.Fatalf("AC-105-01: expected PO status partially_received, got %s", poRefreshed.Status)
	}
	for _, l := range poRefreshed.Lines {
		if l.ID == poLineChairID && l.QtyRemaining != 4 {
			t.Fatalf("expected remaining chairs 4, got %d", l.QtyRemaining)
		}
	}

	// 6. Test Excess Receipt Prevention (receiving 5 more chairs when only 4 remain) -> must fail with ErrExcessReceipt
	_, err = store.CreateReceipt(ctx, GoodsReceipt{
		SchoolID:   schoolID,
		Number:     "GR-EXCESS",
		POID:       po.ID,
		ReceivedOn: time.Now(),
		ReceivedBy: testUserID,
	}, []GoodsReceiptLine{
		{POLineID: poLineChairID, QtyReceived: 5, QtyAccepted: 5, QtyRejected: 0},
	}, "gr-key-excess")
	if err != ErrExcessReceipt {
		t.Fatalf("expected ErrExcessReceipt when exceeding remaining PO qty, got %v", err)
	}

	// 8. Goods Return: Return 2 chairs from GR
	var chairGRLineID string
	for _, gl := range gr.Lines {
		if gl.POLineID == poLineChairID {
			chairGRLineID = gl.ID
		}
	}
	ret, err := store.CreateReturn(ctx, GoodsReturn{
		SchoolID:  schoolID,
		GRLineID:  chairGRLineID,
		Qty:       2,
		Reason:    "Cacat produksi",
		CreatedBy: testUserID,
	})
	if err != nil {
		t.Fatalf("create goods return: %v", err)
	}
	if ret.ID == "" {
		t.Fatalf("expected non-empty return ID")
	}

	// Net accepted chairs is now 6 - 2 = 4, remaining is 10 - 4 = 6
	poRefreshed, _ = store.GetPO(ctx, schoolID, po.ID)
	for _, l := range poRefreshed.Lines {
		if l.ID == poLineChairID && l.QtyRemaining != 6 {
			t.Fatalf("expected remaining chairs 6 after return, got %d", l.QtyRemaining)
		}
	}
}

func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }
