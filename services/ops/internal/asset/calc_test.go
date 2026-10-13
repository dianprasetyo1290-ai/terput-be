package asset

import (
	"testing"
	"time"

	"skci/ops/internal/procurement"
)

func TestCandidatesFromReceipt(t *testing.T) {
	ev := procurement.GoodsReceivedEvent{
		SchoolID:       "sch-001",
		GoodsReceiptID: "gr-123",
		GRNumber:       "GR-2026-001",
		Lines: []procurement.GoodsReceivedLine{
			{
				GRLineID:     "grl-001",
				ItemName:     "Laptop Asus Vivobook",
				QtyAccepted:  2,
				UnitPriceIDR: 12000000,
				ReceivedOn:   time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
				POID:         "po-001",
				VendorID:     "vnd-001",
			},
		},
	}

	candidates := CandidatesFromReceipt(ev)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}

	for i, c := range candidates {
		if c.SchoolID != "sch-001" {
			t.Errorf("cand %d: expected school sch-001, got %s", i, c.SchoolID)
		}
		if c.AcquisitionCostIDR != 12000000 {
			t.Errorf("cand %d: expected cost 12000000, got %d", i, c.AcquisitionCostIDR)
		}
		if *c.UnitSeq != i+1 {
			t.Errorf("cand %d: expected unit_seq %d, got %d", i, i+1, *c.UnitSeq)
		}
		if c.Status != "candidate" {
			t.Errorf("cand %d: expected status candidate, got %s", i, c.Status)
		}
		if len(c.QRToken) != 32 {
			t.Errorf("cand %d: expected 32-char hex QR token, got len %d", i, len(c.QRToken))
		}
	}
}

func TestCanActivate(t *testing.T) {
	roomID := "room-01"
	custodianID := "user-01"
	life := 48
	depStart := "acquisition_month"
	assetAcc := "1-1310"
	accumAcc := "1-1319"
	depExpAcc := "5-1310"

	validCategory := &Category{
		ID:                "cat-01",
		UsefulLifeMonths:  &life,
		DepStart:          &depStart,
		AssetAccCode:      &assetAcc,
		AccumDepAccCode:   &accumAcc,
		DepExpenseAccCode: &depExpAcc,
	}

	// 1. Missing room & custodian
	a1 := Asset{Source: "goods_receipt"}
	errs := CanActivate(a1, validCategory)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %v", errs)
	}

	// 2. Incomplete category (nil)
	a2 := Asset{RoomID: &roomID, CustodianUserID: &custodianID, Source: "goods_receipt"}
	errs = CanActivate(a2, nil)
	if len(errs) != 1 || errs[0] != "kategori aset belum lengkap" {
		t.Fatalf("expected category error, got %v", errs)
	}

	// 3. Category missing GL account
	catIncomplete := &Category{
		UsefulLifeMonths: &life,
	}
	errs = CanActivate(a2, catIncomplete)
	if len(errs) != 1 || errs[0] != "kategori aset belum lengkap" {
		t.Fatalf("expected category error, got %v", errs)
	}

	// 4. Source manual without offset_acc_code
	aManual := Asset{RoomID: &roomID, CustodianUserID: &custodianID, Source: "manual"}
	errs = CanActivate(aManual, validCategory)
	if len(errs) != 1 || errs[0] != "akun lawan wajib untuk perolehan manual" {
		t.Fatalf("expected offset acc error, got %v", errs)
	}

	// 5. Complete
	offsetAcc := "3-1000"
	aManualValid := Asset{RoomID: &roomID, CustodianUserID: &custodianID, Source: "manual", OffsetAccCode: &offsetAcc}
	errs = CanActivate(aManualValid, validCategory)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %v", errs)
	}
}

func TestAcquisitionPayload(t *testing.T) {
	life := 36
	depStart := "next_month"
	assetAcc := "1-1310"
	accumAcc := "1-1319"
	depExpAcc := "5-1310"
	acquiredOn := "2026-10-01"

	cat := Category{
		UsefulLifeMonths:  &life,
		DepStart:          &depStart,
		AssetAccCode:      &assetAcc,
		AccumDepAccCode:   &accumAcc,
		DepExpenseAccCode: &depExpAcc,
	}

	// Goods receipt -> post_journal false
	aGR := Asset{
		ID:                 "ast-001",
		SchoolID:           "sch-001",
		Code:               "LPT-2026-0001",
		Name:               "Laptop",
		AcquisitionCostIDR: 15000000,
		ResidualIDR:        0,
		AcquiredOn:         &acquiredOn,
		Source:             "goods_receipt",
	}

	p1 := AcquisitionPayload(aGR, cat, Settings{})
	if p1["post_journal"] != false {
		t.Errorf("expected post_journal false for goods_receipt, got %v", p1["post_journal"])
	}
	if p1["reference"] != "asset:acq:ast-001" {
		t.Errorf("expected reference asset:acq:ast-001, got %v", p1["reference"])
	}

	// Manual -> post_journal true
	offset := "3-1000"
	aManual := Asset{
		ID:                 "ast-002",
		SchoolID:           "sch-001",
		Code:               "LPT-2026-0002",
		Name:               "Laptop Hibah",
		AcquisitionCostIDR: 10000000,
		AcquiredOn:         &acquiredOn,
		Source:             "manual",
		OffsetAccCode:      &offset,
	}

	p2 := AcquisitionPayload(aManual, cat, Settings{})
	if p2["post_journal"] != true {
		t.Errorf("expected post_journal true for manual, got %v", p2["post_journal"])
	}
	if p2["offset_acc_code"] != "3-1000" {
		t.Errorf("expected offset_acc_code 3-1000, got %v", p2["offset_acc_code"])
	}
}

func TestMovementKind(t *testing.T) {
	b := MovementState{RoomID: "r1", CustodianUserID: "u1", Condition: "baik", SchoolID: "s1"}

	// Room change
	k1 := MovementKind(b, MovementState{RoomID: "r2", CustodianUserID: "u1", Condition: "baik", SchoolID: "s1"})
	if len(k1) != 1 || k1[0] != "location" {
		t.Errorf("expected location, got %v", k1)
	}

	// Inter-unit change
	k2 := MovementKind(b, MovementState{RoomID: "r1", CustodianUserID: "u1", Condition: "baik", SchoolID: "s2"})
	if len(k2) != 1 || k2[0] != "unit" {
		t.Errorf("expected unit, got %v", k2)
	}
}

func TestCanDisposeAndApplyDecision(t *testing.T) {
	aActive := Asset{Status: "active"}
	aCandidate := Asset{Status: "candidate"}

	if err := CanDispose(aCandidate, false); err == nil {
		t.Errorf("expected error disposing candidate asset")
	}
	if err := CanDispose(aActive, true); err == nil {
		t.Errorf("expected error disposing asset with open disposal")
	}
	if err := CanDispose(aActive, false); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Apply decision
	res1, action1 := ApplyDisposalDecision(aActive, "pending")
	if res1.Status != "active" || action1 != "" {
		t.Errorf("expected status active, got %s (action %s)", res1.Status, action1)
	}

	res2, action2 := ApplyDisposalDecision(aActive, "approved")
	if res2.Status != "disposed" || action2 != "disposal" {
		t.Errorf("expected status disposed with action disposal, got %s (action %s)", res2.Status, action2)
	}
}

func TestFormatAssetCode(t *testing.T) {
	cases := []struct {
		format string
		cat    string
		year   int
		seq    int
		exp    string
	}{
		{"{cat}-{yyyy}-{seq:4}", "LPT", 2026, 7, "LPT-2026-0007"},
		{"{cat}/{yyyy}/{seq:6}", "MBL", 2025, 42, "MBL/2025/000042"},
		{"AST-{seq:3}", "XYZ", 2026, 5, "AST-005"},
	}

	for _, c := range cases {
		out := FormatAssetCode(c.format, c.cat, c.year, c.seq)
		if out != c.exp {
			t.Errorf("format %s: expected %s, got %s", c.format, c.exp, out)
		}
	}
}

func TestScanView(t *testing.T) {
	ast := Asset{
		ID:                 "ast-001",
		Code:               "LPT-2026-0001",
		Name:               "Laptop Admin",
		Status:             "active",
		AcquisitionCostIDR: 15000000,
	}
	bv := &BookValue{
		BookValueIDR:   12000000,
		AccumulatedIDR: 3000000,
	}

	// Financial role
	v1 := ScanView(ast, bv, "asset_officer")
	if v1["cost_idr"] != int64(15000000) || v1["book_value_idr"] != int64(12000000) {
		t.Errorf("financial role missing cost or book value: %v", v1)
	}

	// Regular teacher role
	v2 := ScanView(ast, bv, "guru")
	if _, ok := v2["cost_idr"]; ok {
		t.Errorf("guru role should not see cost_idr")
	}
	if _, ok := v2["book_value_idr"]; ok {
		t.Errorf("guru role should not see book_value_idr")
	}
	if v2["code"] != "LPT-2026-0001" {
		t.Errorf("guru role missing code")
	}
}
