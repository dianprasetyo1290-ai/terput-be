package payable

import (
	"testing"
	"time"
)

func TestThreeWayMatch(t *testing.T) {
	// 1. Perfect Match
	linesOK := []MatchInputLine{
		{
			POLineID:         "line-1",
			Description:      "Kertas HVS A4",
			QtyInvoice:       10,
			UnitPriceInvoice: 50_000,
			QtyAcceptedGR:    10,
			UnitPricePO:      50_000,
		},
	}
	res := ThreeWayMatch(linesOK, 0)
	if !res.OK || res.MatchStatus != "matched_ok" {
		t.Errorf("expected matched_ok, got status=%s, ok=%v", res.MatchStatus, res.OK)
	}

	// 2. Qty Mismatch (Invoiced 12, GR accepted 10)
	linesQtyMismatch := []MatchInputLine{
		{
			POLineID:         "line-1",
			Description:      "Kertas HVS A4",
			QtyInvoice:       12,
			UnitPriceInvoice: 50_000,
			QtyAcceptedGR:    10,
			UnitPricePO:      50_000,
		},
	}
	res = ThreeWayMatch(linesQtyMismatch, 0)
	if res.OK || res.MatchStatus != "discrepancy" {
		t.Errorf("expected discrepancy on qty, got %v", res)
	}
	if res.Lines[0].DiscrepancyType != "qty_mismatch" {
		t.Errorf("expected discrepancy_type qty_mismatch, got %s", res.Lines[0].DiscrepancyType)
	}

	// 3. Price Mismatch with tolerance
	linesPriceDiff := []MatchInputLine{
		{
			POLineID:         "line-1",
			Description:      "Kertas HVS A4",
			QtyInvoice:       10,
			UnitPriceInvoice: 52_000,
			QtyAcceptedGR:    10,
			UnitPricePO:      50_000,
		},
	}
	// Without tolerance -> discrepancy
	res = ThreeWayMatch(linesPriceDiff, 0)
	if res.OK || res.Lines[0].DiscrepancyType != "price_mismatch" {
		t.Errorf("expected price_mismatch without tolerance")
	}
	// With tolerance 3000 -> passes
	resTol := ThreeWayMatch(linesPriceDiff, 3_000)
	if !resTol.OK {
		t.Errorf("expected pass with 3000 tolerance, got %v", resTol)
	}
}

func TestCanSchedule(t *testing.T) {
	invoiceTotal := int64(10_000_000)
	paid := int64(4_000_000)

	// Can schedule 6,000,000
	if !CanSchedule(invoiceTotal, paid, 6_000_000) {
		t.Errorf("expected CanSchedule true for remaining 6,000,000")
	}

	// Cannot schedule 6,000,001
	if CanSchedule(invoiceTotal, paid, 6_000_001) {
		t.Errorf("expected CanSchedule false for exceeding amount")
	}

	// Cannot schedule <= 0
	if CanSchedule(invoiceTotal, paid, 0) {
		t.Errorf("expected CanSchedule false for zero amount")
	}
}

func TestAgingBucket(t *testing.T) {
	asOf := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

	// Due in future
	dueFuture := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	b, days := AgingBucket(dueFuture, asOf)
	if b != "current" || days != 0 {
		t.Errorf("expected current, got %s, %d", b, days)
	}

	// Overdue 10 days
	due10 := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	b, days = AgingBucket(due10, asOf)
	if b != "1_30" || days != 10 {
		t.Errorf("expected 1_30, got %s, %d", b, days)
	}

	// Overdue 45 days
	due45 := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	b, days = AgingBucket(due45, asOf)
	if b != "31_60" || days != 45 {
		t.Errorf("expected 31_60, got %s, %d", b, days)
	}

	// Overdue 100 days
	due100 := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	b, days = AgingBucket(due100, asOf)
	if b != "90_plus" || days != 100 {
		t.Errorf("expected 90_plus, got %s, %d", b, days)
	}
}

func TestBuildInvoiceJournalBalance(t *testing.T) {
	subtotal := int64(10_000_000)
	vat := int64(1_100_000)
	wht := int64(200_000)
	total := subtotal + vat - wht // 10,900,000

	payload := BuildInvoiceJournal(
		"school-1", "INV-001", "2026-10-15", "vendor-1",
		subtotal, vat, wht, total,
		"2101", "1181", "5101", "2131",
	)

	lines := payload["lines"].([]map[string]any)
	var sumDebit, sumCredit int64
	for _, l := range lines {
		sumDebit += l["debit_idr"].(int64)
		sumCredit += l["credit_idr"].(int64)
	}

	if sumDebit != sumCredit {
		t.Fatalf("journal not balanced: debit=%d, credit=%d", sumDebit, sumCredit)
	}
	if sumDebit != 11_100_000 {
		t.Fatalf("expected sumDebit 11,100,000, got %d", sumDebit)
	}
}
