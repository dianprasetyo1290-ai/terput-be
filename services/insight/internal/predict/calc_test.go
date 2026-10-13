package predict

import (
	"testing"
)

func TestArrearsRiskScore_AC_154_01(t *testing.T) {
	params := DefaultArrearsRuleParams()

	// High risk student: 5 overdue bills, 95 days overdue, 100% outstanding ratio, 4 late payments
	factsHigh := StudentArrearsFacts{
		StudentUserID:      "s-1",
		OverdueBillCount:   5,
		MaxDaysOverdue:     95,
		OutstandingRatioBP: 10000,
		LatePaymentCount:   4,
	}

	score, level, factors := ArrearsRiskScore(factsHigh, params)
	if level != "high" {
		t.Errorf("expected level high, got %s (score: %d)", level, score)
	}
	if score < 70 {
		t.Errorf("expected score >= 70, got %d", score)
	}
	// AC-154-01: high score must have explanatory factors
	if len(factors) != 3 {
		t.Fatalf("expected 3 explanatory factors, got %d", len(factors))
	}
	for _, f := range factors {
		if f.Contribution <= 0 {
			t.Errorf("expected factor %s to have positive contribution, got %f", f.Key, f.Contribution)
		}
	}

	// Low risk student: 0 overdue bills, 0 days, 0 ratio, 0 late
	factsLow := StudentArrearsFacts{
		StudentUserID:      "s-2",
		OverdueBillCount:   0,
		MaxDaysOverdue:     0,
		OutstandingRatioBP: 0,
		LatePaymentCount:   0,
	}
	scoreLow, levelLow, _ := ArrearsRiskScore(factsLow, params)
	if levelLow != "low" || scoreLow != 0 {
		t.Errorf("expected score 0 level low, got %d %s", scoreLow, levelLow)
	}
}

func TestAbsenceRiskScore(t *testing.T) {
	params := DefaultAbsenceRuleParams()

	factsHigh := StudentAbsenceFacts{
		StudentUserID:         "s-1",
		AbsenceRateBP:         3000, // 30%
		ConsecutiveAbsentDays: 6,
		UnexcusedCount:        6,
		TrendDeltaBP:          2000,
	}

	score, level, factors := AbsenceRiskScore(factsHigh, params)
	if level != "high" {
		t.Errorf("expected high level, got %s (score: %d)", level, score)
	}
	if len(factors) != 3 {
		t.Errorf("expected 3 top factors, got %d", len(factors))
	}
}

func TestProjectApplicants_FR_154_002(t *testing.T) {
	// Prev curve: Day 0: 10, Day 1: 30, Day 2: 60, Day 3: 100 (total = 100)
	prevCurve := []int{10, 30, 60, 100}

	// Current wave at day 1 has 45 applicants.
	// Projected = 45 * 100 / 30 = 150
	proj, errStr := ProjectApplicants(45, 1, prevCurve)
	if errStr != "" || proj == nil {
		t.Fatalf("unexpected error: %s", errStr)
	}
	if *proj != 150 {
		t.Errorf("expected 150 projected, got %d", *proj)
	}

	// Empty curve -> returns nil + "data historis tidak cukup"
	projEmpty, errEmpty := ProjectApplicants(45, 1, nil)
	if projEmpty != nil || errEmpty != "data historis tidak cukup" {
		t.Errorf("expected nil and 'data historis tidak cukup', got %v, %s", projEmpty, errEmpty)
	}
}

func TestProjectCashflow(t *testing.T) {
	due := int64(100_000_000)
	rateBP := 8500 // 85%
	proj := ProjectCashflow(due, rateBP)
	if proj != 85_000_000 {
		t.Errorf("expected 85_000_000, got %d", proj)
	}
}

func TestPrecisionRecallMAPE_FR_154_004(t *testing.T) {
	flagged := []string{"s1", "s2", "s3", "s4"}
	actual := []string{"s2", "s3", "s5"}

	// True positives: s2, s3 (2 out of 4 flagged -> 5000 bp precision; 2 out of 3 actual -> 6666 bp recall)
	prec := Precision(flagged, actual)
	if prec == nil || *prec != 5000 {
		t.Errorf("expected precision 5000 bp, got %v", prec)
	}

	rec := Recall(flagged, actual)
	if rec == nil || *rec != 6666 {
		t.Errorf("expected recall 6666 bp, got %v", rec)
	}

	// Empty sets must return nil, NOT 0!
	if Precision(nil, actual) != nil {
		t.Errorf("expected nil precision for empty flagged set")
	}
	if Recall(flagged, nil) != nil {
		t.Errorf("expected nil recall for empty actual set")
	}

	// MAPE
	proj := []int64{100, 200}
	act := []int64{80, 220}
	// err1 = |100-80|/80 = 20/80 = 0.25 (2500 bp)
	// err2 = |200-220|/220 = 20/220 = 0.090909 (909 bp)
	// mean = (0.25 + 0.090909)/2 = 0.17045 -> 1705 bp
	mape := MAPE(proj, act)
	if mape == nil || *mape != 1705 {
		t.Errorf("expected MAPE 1705 bp, got %v", mape)
	}

	if MAPE(nil, nil) != nil {
		t.Errorf("expected nil MAPE for empty dataset")
	}
}
