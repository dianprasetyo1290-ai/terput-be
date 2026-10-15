package inventory

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
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

	// Apply base + facility + inventory migrations, plus procurement: the
	// stock balance list derives avg_unit_cost_idr from PO/GR lines.
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

	cleanup := func() {
		// Clean tables
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
			DROP TABLE IF EXISTS facility_costs CASCADE;
			DROP TABLE IF EXISTS work_order_photos CASCADE;
			DROP TABLE IF EXISTS work_orders CASCADE;
			DROP TABLE IF EXISTS maintenance_schedules CASCADE;
			DROP TABLE IF EXISTS room_fixed_schedules CASCADE;
			DROP TABLE IF EXISTS room_facility_items CASCADE;
			DROP TABLE IF EXISTS room_bookings CASCADE;
			DROP TABLE IF EXISTS rooms CASCADE;
			DROP TABLE IF EXISTS floors CASCADE;
			DROP TABLE IF EXISTS buildings CASCADE;
			DROP TABLE IF EXISTS doc_sequences CASCADE;
			DROP TABLE IF EXISTS audit_events CASCADE;
		`)
		db.Close()
	}
	return db, cleanup
}

func TestInventoryMovementsAndCountsE2E(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	schoolID := "11111111-1111-1111-1111-111111111111"
	store := NewStore(db, time.UTC)
	handler := NewHandler(store, nil, nil, "http://localhost:8093")

	// 1. Create Category
	cat, err := store.CreateCategory(ctx, ItemCategory{
		SchoolID: schoolID,
		Code:     "ATK",
		Name:     "Alat Tulis Kantor",
		Tracking: "stock",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	// 2. Create Item with min_stock = 20
	item, err := store.CreateItem(ctx, Item{
		SchoolID:   schoolID,
		Code:       "KTS-A4",
		Name:       "Kertas HVS A4",
		CategoryID: cat.ID,
		Unit:       "rim",
		MinStock:   20,
		Status:     "active",
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	// 3. Create Warehouse
	wh, err := store.CreateWarehouse(ctx, Warehouse{
		SchoolID: schoolID,
		Code:     "GUD-01",
		Name:     "Gudang Utama",
		Status:   "active",
	})
	if err != nil {
		t.Fatalf("create warehouse: %v", err)
	}

	// 4. Initial stock in: 25 rim
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	bal, crossed, err := store.Move(ctx, tx, MoveInput{
		SchoolID:       schoolID,
		WarehouseID:    wh.ID,
		ItemID:         item.ID,
		Direction:      "in",
		Qty:            25,
		Kind:           "receipt",
		RefType:        "po",
		RefID:          schoolID,
		RefNo:          "PO-001",
		CreatedBy:      schoolID,
		IdempotencyKey: "move:1",
	})
	if err != nil {
		t.Fatalf("initial move in: %v", err)
	}
	_ = tx.Commit()
	if bal != 25 {
		t.Fatalf("expected balance 25, got %d", bal)
	}
	if crossed {
		t.Errorf("did not expect crossed below min on in")
	}

	// 5. Issue 10 rim (25 -> 15): CrossedBelowMin should be true (AC-102-02)
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	bal, crossed, err = store.Move(ctx, tx, MoveInput{
		SchoolID:       schoolID,
		WarehouseID:    wh.ID,
		ItemID:         item.ID,
		Direction:      "out",
		Qty:            10,
		Kind:           "issue",
		RefType:        "request",
		RefID:          schoolID,
		RefNo:          "REQ-001",
		CreatedBy:      schoolID,
		IdempotencyKey: "move:2",
	})
	if err != nil {
		t.Fatalf("move out 10: %v", err)
	}
	_ = tx.Commit()
	if bal != 15 {
		t.Fatalf("expected balance 15, got %d", bal)
	}
	if !crossed {
		t.Errorf("AC-102-02: expected crossed below min (25 -> 15 with min 20)")
	}

	// 6. AC-102-01: Try to issue 20 rim when balance is 15 -> must fail with ErrInsufficientStock
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.Move(ctx, tx, MoveInput{
		SchoolID:       schoolID,
		WarehouseID:    wh.ID,
		ItemID:         item.ID,
		Direction:      "out",
		Qty:            20,
		Kind:           "issue",
		RefType:        "request",
		RefID:          schoolID,
		RefNo:          "REQ-002",
		CreatedBy:      schoolID,
		IdempotencyKey: "move:3",
	})
	_ = tx.Rollback()
	if err == nil {
		t.Fatalf("AC-102-01: expected error issuing 20 when stock is 15")
	}

	// Verify balance remains 15
	balances, err := store.ListBalances(ctx, schoolID, wh.ID, item.ID, false)
	if err != nil || len(balances) == 0 || balances[0].Qty != 15 {
		t.Fatalf("expected balance to remain 15, got %v (err %v)", balances, err)
	}

	// 7. Stock Opname: Start Count
	docNo, err := store.NextDocNumber(ctx, schoolID, "SO", 2026, "SO-{yyyy}-{seq:4}")
	if err != nil {
		t.Fatal(err)
	}
	count, err := store.CreateStockCount(ctx, schoolID, wh.ID, schoolID, docNo)
	if err != nil {
		t.Fatalf("create stock count: %v", err)
	}
	if count.Status != "counting" {
		t.Fatalf("expected status counting, got %s", count.Status)
	}

	// While count is active, movements must be rejected with ErrCountingActive
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.Move(ctx, tx, MoveInput{
		SchoolID:       schoolID,
		WarehouseID:    wh.ID,
		ItemID:         item.ID,
		Direction:      "in",
		Qty:            5,
		Kind:           "receipt",
		RefType:        "po",
		RefID:          schoolID,
		RefNo:          "PO-002",
		CreatedBy:      schoolID,
		IdempotencyKey: "move:4",
	})
	_ = tx.Rollback()
	if err != ErrCountingActive {
		t.Fatalf("expected ErrCountingActive while count is active, got %v", err)
	}

	// 8. Update Counted Qty to 13 (variance -2)
	counted13 := int64(13)
	err = store.UpdateCountLines(ctx, schoolID, count.ID, []StockCountLine{
		{ItemID: item.ID, CountedQty: &counted13},
	})
	if err != nil {
		t.Fatalf("update count lines: %v", err)
	}

	// 9. Submit count
	if err := store.SubmitStockCount(ctx, schoolID, count.ID); err != nil {
		t.Fatalf("submit count: %v", err)
	}

	// 10. AC-103-01: Stock does NOT change before approval callback
	balances, _ = store.ListBalances(ctx, schoolID, wh.ID, item.ID, false)
	if balances[0].Qty != 15 {
		t.Fatalf("stock must not change before approval, expected 15, got %d", balances[0].Qty)
	}

	// 11. Approval callback from Core
	err = handler.ApplyApprovalDecision(ctx, ApprovalDecision{
		ApprovalID:  "app-1",
		SourceID:    count.ID,
		RequestType: "stock_adjustment",
		Decision:    "approved",
		DecidedBy:   "user-mgr",
	})
	if err != nil {
		t.Fatalf("apply approval decision: %v", err)
	}

	// Balance should now be adjusted to 13 (-2 applied)
	balances, _ = store.ListBalances(ctx, schoolID, wh.ID, item.ID, false)
	if balances[0].Qty != 13 {
		t.Fatalf("AC-103-01: expected balance 13 after adjustment, got %d", balances[0].Qty)
	}

	// Callback sent a 2nd time must be idempotent (balance remains 13, not 11)
	err = handler.ApplyApprovalDecision(ctx, ApprovalDecision{
		ApprovalID:  "app-1",
		SourceID:    count.ID,
		RequestType: "stock_adjustment",
		Decision:    "approved",
		DecidedBy:   "user-mgr",
	})
	if err != nil {
		t.Fatalf("re-apply approval decision: %v", err)
	}
	balances, _ = store.ListBalances(ctx, schoolID, wh.ID, item.ID, false)
	if balances[0].Qty != 13 {
		t.Fatalf("idempotency check failed: expected balance 13, got %d", balances[0].Qty)
	}

	// 12. Check Stock Card Report
	card, err := store.GetStockCard(ctx, schoolID, wh.ID, item.ID, time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("get stock card: %v", err)
	}
	if card.ClosingBalance != 13 {
		t.Fatalf("expected stock card closing balance 13, got %d", card.ClosingBalance)
	}
	if len(card.Entries) < 3 {
		t.Fatalf("expected at least 3 movement entries, got %d", len(card.Entries))
	}
}
