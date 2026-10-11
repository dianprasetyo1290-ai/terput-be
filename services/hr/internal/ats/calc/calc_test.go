package calc

import (
	"testing"
	"time"
)

func TestExceedsHeadcount(t *testing.T) {
	// AC-88-01: vacant 1, requested 2 -> true
	if !ExceedsHeadcount(1, 2) {
		t.Errorf("expected ExceedsHeadcount(1, 2) to be true")
	}
	// vacant 2, requested 1 -> false
	if ExceedsHeadcount(2, 1) {
		t.Errorf("expected ExceedsHeadcount(2, 1) to be false")
	}
	// vacant 2, requested 2 -> false
	if ExceedsHeadcount(2, 2) {
		t.Errorf("expected ExceedsHeadcount(2, 2) to be false")
	}
}

func TestWeightedScore(t *testing.T) {
	// AC-91-01: 60%/40%, 80/90 -> 84
	weights := []int{6000, 4000}
	scores := []int{80, 90}
	got := WeightedScore(weights, scores)
	if got != 84 {
		t.Errorf("expected 84, got %d", got)
	}

	// 50%/50%, 75/85 -> 80
	if got := WeightedScore([]int{5000, 5000}, []int{75, 85}); got != 80 {
		t.Errorf("expected 80, got %d", got)
	}
}

func TestVisibleScores(t *testing.T) {
	// AC-91-02: belum submit -> hanya miliknya
	allScores := []PanelScoreItem{
		{PanelUserID: "user-1", CriterionID: "c1", Score: 85},
		{PanelUserID: "user-2", CriterionID: "c1", Score: 90},
	}

	// When user-1 has not submitted
	v1 := VisibleScores("user-1", false, allScores)
	if len(v1) != 1 || v1[0].PanelUserID != "user-1" {
		t.Errorf("expected only user-1 score, got %+v", v1)
	}

	// When user-1 has submitted
	v2 := VisibleScores("user-1", true, allScores)
	if len(v2) != 2 {
		t.Errorf("expected all 2 scores visible, got %+v", v2)
	}
}

func TestSalaryOutOfRange(t *testing.T) {
	// AC-92-01: 5.5 jt vs 4-5 jt -> true
	if !SalaryOutOfRange(5_500_000, 4_000_000, 5_000_000) {
		t.Errorf("expected SalaryOutOfRange to be true for 5.5m in 4m-5m")
	}
	// 4.5 jt in 4-5 jt -> false
	if SalaryOutOfRange(4_500_000, 4_000_000, 5_000_000) {
		t.Errorf("expected SalaryOutOfRange to be false for 4.5m in 4m-5m")
	}
	// 3.5 jt in 4-5 jt -> true
	if !SalaryOutOfRange(3_500_000, 4_000_000, 5_000_000) {
		t.Errorf("expected SalaryOutOfRange to be true for 3.5m in 4m-5m")
	}
}

func TestShouldClose(t *testing.T) {
	// AC-89-02: closes_on 31 Oct, 1 Nov -> close
	closesOn := time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC)
	nov1 := time.Date(2026, time.November, 1, 10, 0, 0, 0, time.UTC)
	oct25 := time.Date(2026, time.October, 25, 10, 0, 0, 0, time.UTC)

	if !ShouldClose(&closesOn, nov1, false) {
		t.Errorf("expected ShouldClose to be true on Nov 1")
	}
	if ShouldClose(&closesOn, oct25, false) {
		t.Errorf("expected ShouldClose to be false on Oct 25")
	}
	// filled true -> always close
	if !ShouldClose(&closesOn, oct25, true) {
		t.Errorf("expected ShouldClose to be true if filled is true")
	}
}

func TestShouldAnonymize(t *testing.T) {
	// AC-90-02: ditolak, tanpa talent pool, > 6 bln -> true
	lastActivity := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	now7MonthsLater := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)
	now3MonthsLater := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)

	// Rejected, no talent pool, 7 months later -> true
	if !ShouldAnonymize("rejected", false, lastActivity, 6, now7MonthsLater) {
		t.Errorf("expected ShouldAnonymize to be true after 7 months")
	}
	// Rejected, with talent pool consent -> false
	if ShouldAnonymize("rejected", true, lastActivity, 6, now7MonthsLater) {
		t.Errorf("expected ShouldAnonymize to be false when candidate consented to talent pool")
	}
	// Rejected, but only 3 months later -> false
	if ShouldAnonymize("rejected", false, lastActivity, 6, now3MonthsLater) {
		t.Errorf("expected ShouldAnonymize to be false before retention cutoff")
	}
	// Hired -> never anonymize
	if ShouldAnonymize("hired", false, lastActivity, 6, now7MonthsLater) {
		t.Errorf("expected ShouldAnonymize to be false for hired candidate")
	}
}
