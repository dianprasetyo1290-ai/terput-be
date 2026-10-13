package inventory

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrInsufficientStock = errors.New("Stok tidak mencukupi")
	ErrInvalidQuantity   = errors.New("Kuantitas harus lebih besar dari 0")
	ErrInvalidDirection  = errors.New("Arah mutasi tidak valid")
)

// ApplyMovement calculates stock balance after in/out movement.
// AC-102-01: saldo 5, out 8 -> ErrInsufficientStock "Stok tidak mencukupi (tersedia 5)";
// saldo 5, out 5 -> 0.
func ApplyMovement(balance int64, direction string, qty int64) (int64, error) {
	if qty <= 0 {
		return balance, ErrInvalidQuantity
	}
	switch direction {
	case "in":
		return balance + qty, nil
	case "out":
		if balance < qty {
			return balance, fmt.Errorf("Stok tidak mencukupi (tersedia %d)", balance)
		}
		return balance - qty, nil
	default:
		return balance, ErrInvalidDirection
	}
}

// CrossedBelowMin reports if a movement caused stock to fall below minimum threshold.
// AC-102-02: min 20, 25 -> 15 -> true; 15 -> 10 -> false; min 0 -> false.
func CrossedBelowMin(before, after, min int64) bool {
	if min <= 0 {
		return false
	}
	return before >= min && after < min
}

// CountVariance calculates the difference between counted and frozen system quantity.
// AC-103-01: 12 vs 10 -> -2. If countedQty is nil, returns nil.
func CountVariance(systemQty int64, countedQty *int64) *int64 {
	if countedQty == nil {
		return nil
	}
	variance := *countedQty - systemQty
	return &variance
}

// FormatDocNumber formats a document number based on pattern, year, and sequence.
// Example: "PO-{yyyy}-{seq:4}", 2026, 12 -> "PO-2026-0012".
func FormatDocNumber(format string, year int, seq int) string {
	if format == "" {
		return fmt.Sprintf("DOC-%04d-%04d", year, seq)
	}
	res := strings.ReplaceAll(format, "{yyyy}", fmt.Sprintf("%04d", year))
	res = strings.ReplaceAll(res, "{yy}", fmt.Sprintf("%02d", year%100))

	// Search for {seq:N} or {seq}
	for {
		start := strings.Index(res, "{seq")
		if start == -1 {
			break
		}
		end := strings.Index(res[start:], "}")
		if end == -1 {
			break
		}
		end += start
		tag := res[start : end+1]

		pad := 4
		if strings.HasPrefix(tag, "{seq:") {
			numPart := strings.TrimSuffix(strings.TrimPrefix(tag, "{seq:"), "}")
			if p, err := strconv.Atoi(numPart); err == nil && p > 0 {
				pad = p
			}
		}
		formattedSeq := fmt.Sprintf("%0*d", pad, seq)
		res = strings.Replace(res, tag, formattedSeq, 1)
	}
	return res
}
