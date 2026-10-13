package metrics

import (
	"testing"
	"time"
)

func TestResolvePeriod(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	// Test date: 2026-10-15
	today := time.Date(2026, time.October, 15, 10, 0, 0, 0, loc)

	// this_month -> 2026-10-01 to 2026-10-31
	start, end, err := ResolvePeriod(PeriodSpec{Kind: "relative", Value: "this_month"}, today, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start.Format("2006-01-02") != "2026-10-01" || end.Format("2006-01-02") != "2026-10-31" {
		t.Errorf("expected 2026-10-01..2026-10-31, got %s..%s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}

	// current_academic_year -> 2026-07-01 to 2027-06-30
	start, end, err = ResolvePeriod(PeriodSpec{Kind: "relative", Value: "current_academic_year"}, today, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start.Format("2006-01-02") != "2026-07-01" || end.Format("2006-01-02") != "2027-06-30" {
		t.Errorf("expected 2026-07-01..2027-06-30, got %s..%s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}

	// last_12_months -> 2025-10-01 to 2026-09-30
	start, end, err = ResolvePeriod(PeriodSpec{Kind: "relative", Value: "last_12_months"}, today, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start.Format("2006-01-02") != "2025-10-01" || end.Format("2006-01-02") != "2026-09-30" {
		t.Errorf("expected 2025-10-01..2026-09-30, got %s..%s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}

	// explicit
	start, end, err = ResolvePeriod(PeriodSpec{Kind: "explicit", From: "2026-01-01", To: "2026-03-31"}, today, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start.Format("2006-01-02") != "2026-01-01" || end.Format("2006-01-02") != "2026-03-31" {
		t.Errorf("expected 2026-01-01..2026-03-31, got %s..%s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}
}

func TestIsStale(t *testing.T) {
	now := time.Now()
	refreshMin := 60
	def := MetricDef{
		RefreshMinutes: &refreshMin,
	}

	// Snapshot 30 mins ago -> not stale
	snapRecent := &MetricSnapshot{
		ComputedAt: now.Add(-30 * time.Minute),
	}
	if IsStale(snapRecent, def, now, 60) {
		t.Errorf("expected not stale for 30m old snapshot with 60m threshold")
	}

	// Snapshot 90 mins ago -> stale
	snapOld := &MetricSnapshot{
		ComputedAt: now.Add(-90 * time.Minute),
	}
	if !IsStale(snapOld, def, now, 60) {
		t.Errorf("expected stale for 90m old snapshot with 60m threshold")
	}

	// nil snapshot -> stale
	if !IsStale(nil, def, now, 60) {
		t.Errorf("expected nil snapshot to be stale")
	}
}

func TestAggregate(t *testing.T) {
	// Vector AC-52-01: tagihan 100.000.000, teralokasi 85.000.000 -> 8500 bp
	collectionRateDef := MetricDef{
		Key:      "collection_rate",
		Unit:     UnitPercentBP,
		Additive: false,
	}

	rows := []AggregateRow{
		{
			SchoolID:    "school-sd",
			Numerator:   40_000_000, // teralokasi SD
			Denominator: 50_000_000, // tagihan SD
			Value:       8000,       // 80%
		},
		{
			SchoolID:    "school-smp",
			Numerator:   45_000_000, // teralokasi SMP
			Denominator: 50_000_000, // tagihan SMP
			Value:       9000,       // 90%
		},
	}

	perUnit, consolidated := Aggregate(collectionRateDef, rows)

	if len(perUnit) != 2 {
		t.Fatalf("expected 2 per unit results, got %d", len(perUnit))
	}
	if consolidated.Numerator != 85_000_000 {
		t.Errorf("expected total numerator 85_000_000, got %d", consolidated.Numerator)
	}
	if consolidated.Denominator != 100_000_000 {
		t.Errorf("expected total denominator 100_000_000, got %d", consolidated.Denominator)
	}
	// Total: 85_000_000 / 100_000_000 * 10000 = 8500 bp
	if consolidated.Value != 8500 {
		t.Errorf("expected consolidated value 8500 bp, got %d", consolidated.Value)
	}

	// Test Additive (outstanding_idr)
	outstandingDef := MetricDef{
		Key:      "outstanding_idr",
		Unit:     UnitIDR,
		Additive: true,
	}
	outRows := []AggregateRow{
		{SchoolID: "school-sd", Value: 10_000_000},
		{SchoolID: "school-smp", Value: 5_000_000},
	}
	_, consOut := Aggregate(outstandingDef, outRows)
	if consOut.Value != 15_000_000 {
		t.Errorf("expected additive sum 15_000_000, got %d", consOut.Value)
	}
}
