package budget

import (
	"testing"
)

func TestAbsorptionBP(t *testing.T) {
	tests := []struct {
		name     string
		actual   int64
		planned  int64
		expected int
	}{
		{"zero planned, zero actual", 0, 0, 0},
		{"zero planned, positive actual", 1000, 0, 10000},
		{"half absorption", 50_000_000, 100_000_000, 5000},
		{"exact absorption", 100_000_000, 100_000_000, 10000},
		{"over absorption", 125_000_000, 100_000_000, 12500},
		{"fractional percentage", 33_333_333, 100_000_000, 3333},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AbsorptionBP(tt.actual, tt.planned)
			if got != tt.expected {
				t.Errorf("AbsorptionBP(%d, %d) = %d; want %d", tt.actual, tt.planned, got, tt.expected)
			}
		})
	}
}

func TestRemaining(t *testing.T) {
	rem := Remaining(100_000_000, 40_000_000, 20_000_000)
	if rem != 40_000_000 {
		t.Errorf("Remaining = %d; want 40000000", rem)
	}

	negRem := Remaining(100_000_000, 80_000_000, 30_000_000)
	if negRem != -10_000_000 {
		t.Errorf("Negative Remaining = %d; want -10000000", negRem)
	}
}

func TestCheckLineBudget(t *testing.T) {
	planned := int64(100_000_000)
	toleranceBP := 500 // 5% = 5,000,000 allowance

	// 1. Within budget
	over, policy := CheckLineBudget(planned, 50_000_000, 10_000_000, 20_000_000, toleranceBP, "hard_stop")
	if over || policy != "none" {
		t.Errorf("expected within budget, got over=%v, policy=%s", over, policy)
	}

	// 2. Exceeded planned, within tolerance (total 103,000,000 <= 105,000,000)
	over, policy = CheckLineBudget(planned, 50_000_000, 30_000_000, 23_000_000, toleranceBP, "hard_stop")
	if !over || policy != "warn" {
		t.Errorf("expected over with warn within tolerance, got over=%v, policy=%s", over, policy)
	}

	// 3. Exceeded tolerance with hard_stop (total 110,000,000 > 105,000,000)
	over, policy = CheckLineBudget(planned, 50_000_000, 30_000_000, 30_000_000, toleranceBP, "hard_stop")
	if !over || policy != "reject" {
		t.Errorf("expected reject with hard_stop, got over=%v, policy=%s", over, policy)
	}

	// 4. Exceeded tolerance with approval_required
	over, policy = CheckLineBudget(planned, 50_000_000, 30_000_000, 30_000_000, toleranceBP, "approval_required")
	if !over || policy != "extra_approval" {
		t.Errorf("expected extra_approval, got over=%v, policy=%s", over, policy)
	}

	// 5. Exceeded tolerance with warn
	over, policy = CheckLineBudget(planned, 50_000_000, 30_000_000, 30_000_000, toleranceBP, "warn")
	if !over || policy != "warn" {
		t.Errorf("expected warn, got over=%v, policy=%s", over, policy)
	}
}

func TestCrossedWarn(t *testing.T) {
	planned := int64(100_000_000)
	warnBP := 8000 // 80%

	if CrossedWarn(79_000_000, planned, warnBP) {
		t.Errorf("79%% should not cross 80%% threshold")
	}

	if !CrossedWarn(80_000_000, planned, warnBP) {
		t.Errorf("80%% should cross 80%% threshold")
	}

	if !CrossedWarn(85_000_000, planned, warnBP) {
		t.Errorf("85%% should cross 80%% threshold")
	}
}
