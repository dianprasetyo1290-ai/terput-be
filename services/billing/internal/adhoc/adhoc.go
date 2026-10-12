// Package adhoc holds the pure rules of BILL-ADHOC (T12 §4.7): validating an
// ad-hoc bill raised by another module and re-pricing it after a proration
// or adjustment. No I/O; all money is integer rupiah.
package adhoc

import (
	"errors"
	"strings"
	"time"
)

// Source modules allowed to raise ad-hoc bills. The list mirrors the
// student_bills.source_module CHECK; it grows only through the contract
// owner's migration.
var SourceModules = map[string]bool{"library": true, "transport": true, "event": true, "extracurricular": true}

// Item is one ad-hoc bill line.
type Item struct {
	Name      string `json:"name"`
	AmountIDR int64  `json:"amount_idr"`
}

// CreateRequest is the body of POST /internal/bills/adhoc.
type CreateRequest struct {
	SchoolID      string `json:"school_id"`
	StudentUserID string `json:"student_user_id"`
	SourceModule  string `json:"source_module"`
	SourceRef     string `json:"source_ref"`
	Items         []Item `json:"items"`
	DueDate       string `json:"due_date"` // YYYY-MM-DD (Asia/Jakarta)
}

// ValidationError is a 422-class input problem.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// ValidateCreate checks an ad-hoc bill request and returns its total (sum of
// items) and due date.
func ValidateCreate(req CreateRequest) (int64, time.Time, error) {
	if strings.TrimSpace(req.SchoolID) == "" || strings.TrimSpace(req.StudentUserID) == "" {
		return 0, time.Time{}, &ValidationError{"school_id dan student_user_id wajib diisi"}
	}
	if !SourceModules[req.SourceModule] {
		return 0, time.Time{}, &ValidationError{"source_module tidak dikenal"}
	}
	if strings.TrimSpace(req.SourceRef) == "" {
		return 0, time.Time{}, &ValidationError{"source_ref wajib diisi"}
	}
	total, err := Total(req.Items)
	if err != nil {
		return 0, time.Time{}, err
	}
	due, err := time.Parse("2006-01-02", strings.TrimSpace(req.DueDate))
	if err != nil {
		return 0, time.Time{}, &ValidationError{"due_date wajib berformat YYYY-MM-DD"}
	}
	return total, due, nil
}

// Total sums items, rejecting an empty list, blank names and amounts <= 0.
func Total(items []Item) (int64, error) {
	if len(items) == 0 {
		return 0, &ValidationError{"item tagihan wajib diisi"}
	}
	var total int64
	for _, it := range items {
		if strings.TrimSpace(it.Name) == "" {
			return 0, &ValidationError{"nama item wajib diisi"}
		}
		if it.AmountIDR <= 0 {
			return 0, &ValidationError{"amount_idr item harus lebih dari 0"}
		}
		total += it.AmountIDR
	}
	return total, nil
}

// SameItems reports whether two item lists are identical (order matters).
func SameItems(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].Name) != strings.TrimSpace(b[i].Name) || a[i].AmountIDR != b[i].AmountIDR {
			return false
		}
	}
	return true
}

// Bill is the state Adjust needs.
type Bill struct {
	NetIDR int64
	Status string // unpaid | partial | paid | void
}

// Adjustment is the outcome of re-pricing a bill.
type Adjustment struct {
	NetIDR     int64  `json:"net_idr"`
	DeltaIDR   int64  `json:"delta_idr"`
	DepositIDR int64  `json:"deposit_idr"` // overpayment moved to student_deposits (BR-FIN-04)
	PaidIDR    int64  `json:"paid_idr"`    // paid amount kept on the bill
	Status     string `json:"status"`
}

var (
	ErrVoid        = errors.New("tagihan sudah dibatalkan")
	ErrNegativeNet = errors.New("nominal baru tidak boleh negatif")
	ErrNoChange    = errors.New("nominal baru sama dengan nominal lama")
)

// Adjust re-prices a bill to newNet given what has been paid. Paid money
// above the new net moves to the student's deposit; the status follows the
// remaining balance (a bill whose new net is 0 counts as settled).
func Adjust(b Bill, paid, newNet int64) (Adjustment, error) {
	if b.Status == "void" {
		return Adjustment{}, ErrVoid
	}
	if newNet < 0 {
		return Adjustment{}, ErrNegativeNet
	}
	if newNet == b.NetIDR {
		return Adjustment{}, ErrNoChange
	}
	if paid < 0 {
		paid = 0
	}
	out := Adjustment{NetIDR: newNet, DeltaIDR: newNet - b.NetIDR, PaidIDR: paid}
	if paid > newNet {
		out.DepositIDR = paid - newNet
		out.PaidIDR = newNet
	}
	switch {
	case out.PaidIDR >= newNet:
		out.Status = "paid"
	case out.PaidIDR > 0:
		out.Status = "partial"
	default:
		out.Status = "unpaid"
	}
	return out, nil
}
