package procurement

import (
	"testing"
	"time"
)

func TestPRTotal(t *testing.T) {
	lines := []PRLineCalc{
		{Qty: 10, EstUnitPriceIDR: 50000},
		{Qty: 5, EstUnitPriceIDR: 200000},
	}
	total := PRTotal(lines)
	if total != 1500000 {
		t.Fatalf("expected 1500000, got %d", total)
	}
}

func TestApprovalFlags(t *testing.T) {
	// Hook nil -> []
	flags, err := ApprovalFlags(nil)
	if err != nil || len(flags) != 0 {
		t.Fatalf("expected empty flags on nil result, got %v, err=%v", flags, err)
	}

	// AC-104-01: sisa 2.000.000, PR 2.500.000, policy "extra_approval" -> ["over_budget"]
	resExtra := &BudgetCheckResult{
		Policy: "extra_approval",
		Over:   true,
	}
	flags, err = ApprovalFlags(resExtra)
	if err != nil || len(flags) != 1 || flags[0] != "over_budget" {
		t.Fatalf("expected ['over_budget'], got %v, err=%v", flags, err)
	}

	// policy "reject" -> ErrOverBudget
	resReject := &BudgetCheckResult{
		Policy: "reject",
		Over:   true,
	}
	flags, err = ApprovalFlags(resReject)
	if err != ErrOverBudget {
		t.Fatalf("expected ErrOverBudget on reject policy, got err=%v", err)
	}

	// Not over -> no flag
	resOk := &BudgetCheckResult{
		Policy: "reject",
		Over:   false,
	}
	flags, err = ApprovalFlags(resOk)
	if err != nil || len(flags) != 0 {
		t.Fatalf("expected no flag when not over budget, got %v", flags)
	}
}

func TestPOReceiptStatus(t *testing.T) {
	// AC-105-01: 10 ordered, receive 6 -> partially_received, remaining 4
	lines := []POLineReceiptCalc{
		{POLineID: "line-1", QtyOrdered: 10, QtyAcceptedNet: 6},
	}
	status, remaining := POReceiptStatus(lines)
	if status != "partially_received" || remaining["line-1"] != 4 {
		t.Fatalf("expected partially_received remaining 4, got %s, rem=%v", status, remaining)
	}

	// 10/10 -> received
	lines[0].QtyAcceptedNet = 10
	status, remaining = POReceiptStatus(lines)
	if status != "received" || remaining["line-1"] != 0 {
		t.Fatalf("expected received remaining 0, got %s, rem=%v", status, remaining)
	}

	// return 2 from 10 (net 8) -> back to partially_received, remaining 2
	lines[0].QtyAcceptedNet = 8
	status, remaining = POReceiptStatus(lines)
	if status != "partially_received" || remaining["line-1"] != 2 {
		t.Fatalf("expected partially_received remaining 2, got %s, rem=%v", status, remaining)
	}
}

func TestSplitGoodsReceipt(t *testing.T) {
	// AC-105-02: 2 laptop 'asset' -> 2 units in assetLines, without stock movements;
	// kursi 'stock' accepted 6, rejected 2 -> stockMoves 'in' 6, rejected does not enter stock.
	receiptLines := []ReceiptLineInput{
		{POLineID: "p1", ItemID: "item-laptop", QtyReceived: 2, QtyAccepted: 2, QtyRejected: 0},
		{POLineID: "p2", ItemID: "item-chair", QtyReceived: 8, QtyAccepted: 6, QtyRejected: 2},
		{POLineID: "p3", ItemID: "item-service", QtyReceived: 1, QtyAccepted: 1, QtyRejected: 0},
	}
	tracking := map[string]string{
		"item-laptop":  "asset",
		"item-chair":   "stock",
		"item-service": "none",
	}

	stockMoves, assetLines, noneLines := SplitGoodsReceipt(receiptLines, tracking)
	if len(assetLines) != 1 || assetLines[0].QtyAccepted != 2 {
		t.Fatalf("expected 1 asset line with qty 2, got %v", assetLines)
	}
	if len(stockMoves) != 1 || stockMoves[0].QtyAccepted != 6 {
		t.Fatalf("expected 1 stock move with qty 6 (rejected 2 ignored), got %v", stockMoves)
	}
	if len(noneLines) != 1 || noneLines[0].QtyAccepted != 1 {
		t.Fatalf("expected 1 none line, got %v", noneLines)
	}
}

func TestVendorDuplicates(t *testing.T) {
	bidx1 := "npwp-hash-1"
	bidx2 := "npwp-hash-2"
	existing := []ExistingVendorInfo{
		{VendorID: "v1", NPWPBidx: &bidx1, AccountBidx: []string{"acc-hash-1", "acc-hash-2"}},
		{VendorID: "v2", NPWPBidx: &bidx2, AccountBidx: []string{"acc-hash-3"}},
	}

	dups := VendorDuplicates("npwp-hash-1", "", existing)
	if len(dups) != 1 || dups[0] != "v1" {
		t.Fatalf("expected duplicate v1 for npwp, got %v", dups)
	}

	dups = VendorDuplicates("", "acc-hash-2", existing)
	if len(dups) != 1 || dups[0] != "v1" {
		t.Fatalf("expected duplicate v1 for account, got %v", dups)
	}

	dups = VendorDuplicates("unknown", "unknown", existing)
	if len(dups) != 0 {
		t.Fatalf("expected no duplicates, got %v", dups)
	}
}

func TestOnTime(t *testing.T) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)

	if OnTime(nil, now) != nil {
		t.Errorf("expected nil when expected date is nil")
	}

	res := OnTime(&tomorrow, now)
	if res == nil || !*res {
		t.Errorf("expected on time when received before expected")
	}

	res = OnTime(&yesterday, now)
	if res == nil || *res {
		t.Errorf("expected not on time when received after expected")
	}
}
