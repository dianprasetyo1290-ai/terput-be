package inventory

import (
	"testing"
)

func TestApplyMovement(t *testing.T) {
	// AC-102-01: saldo 5, out 8 -> ErrInsufficientStock; saldo 5, out 5 -> 0; in 10 -> 15.
	after, err := ApplyMovement(5, "out", 8)
	if err == nil {
		t.Fatalf("expected error on out 8 from 5, got nil, after=%d", after)
	}
	if err.Error() != "Stok tidak mencukupi (tersedia 5)" {
		t.Errorf("unexpected error message: %v", err)
	}

	after, err = ApplyMovement(5, "out", 5)
	if err != nil || after != 0 {
		t.Fatalf("expected 0, got %d, err=%v", after, err)
	}

	after, err = ApplyMovement(5, "in", 10)
	if err != nil || after != 15 {
		t.Fatalf("expected 15, got %d, err=%v", after, err)
	}

	if _, err := ApplyMovement(5, "in", 0); err != ErrInvalidQuantity {
		t.Errorf("expected ErrInvalidQuantity on qty 0")
	}

	if _, err := ApplyMovement(5, "invalid", 2); err != ErrInvalidDirection {
		t.Errorf("expected ErrInvalidDirection on invalid direction")
	}
}

func TestCrossedBelowMin(t *testing.T) {
	// AC-102-02: min 20, 25 -> 15 -> true; 15 -> 10 -> false; min 0 -> false.
	if !CrossedBelowMin(25, 15, 20) {
		t.Errorf("expected true when crossing below 20 from 25 to 15")
	}
	if CrossedBelowMin(15, 10, 20) {
		t.Errorf("expected false when already below min")
	}
	if CrossedBelowMin(30, 25, 20) {
		t.Errorf("expected false when still above min")
	}
	if CrossedBelowMin(5, 0, 0) {
		t.Errorf("expected false when min is 0")
	}
}

func TestCountVariance(t *testing.T) {
	// AC-103-01: 12 vs 10 -> -2. Nil counted -> nil.
	counted := int64(10)
	v := CountVariance(12, &counted)
	if v == nil || *v != -2 {
		t.Fatalf("expected -2, got %v", v)
	}

	if CountVariance(12, nil) != nil {
		t.Errorf("expected nil variance for uncounted line")
	}
}

func TestFormatDocNumber(t *testing.T) {
	formatted := FormatDocNumber("PO-{yyyy}-{seq:4}", 2026, 12)
	if formatted != "PO-2026-0012" {
		t.Errorf("expected PO-2026-0012, got %s", formatted)
	}

	formatted = FormatDocNumber("SO-{yy}-{seq:3}", 2026, 7)
	if formatted != "SO-26-007" {
		t.Errorf("expected SO-26-007, got %s", formatted)
	}
}
