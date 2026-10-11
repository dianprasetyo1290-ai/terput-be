// Package model holds the pure billing domain logic: the student-bill and
// payment state machines, the ordered discount/scholarship engine (borrowed
// from admission's PPDB discount engine), the outstanding-balance invariant,
// and the idempotent generation planner. Nothing here touches the database or
// the network, so every rule is exercised by fast, deterministic unit tests.
//
// All money is integer Indonesian rupiah (IDR). Floating point is never used.
package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Bill lifecycle
// ---------------------------------------------------------------------------

// BillStatus is the lifecycle of a student bill.
type BillStatus string

const (
	BillUnpaid  BillStatus = "unpaid"
	BillPartial BillStatus = "partial"
	BillPaid    BillStatus = "paid"
	BillVoid    BillStatus = "void"
)

// Valid reports whether s is a known bill status.
func (s BillStatus) Valid() bool {
	switch s {
	case BillUnpaid, BillPartial, BillPaid, BillVoid:
		return true
	default:
		return false
	}
}

// DeriveBillStatus maps a snapshot net amount and the settled amount to the
// bill status. A zero net bill with no payment is unpaid; a void bill is
// handled by the caller (never derived).
func DeriveBillStatus(netIDR, paidIDR int64) BillStatus {
	switch {
	case paidIDR <= 0:
		return BillUnpaid
	case paidIDR >= netIDR:
		return BillPaid
	default:
		return BillPartial
	}
}

// ---------------------------------------------------------------------------
// Payment state machine
// ---------------------------------------------------------------------------

// PaymentStatus is the lifecycle of a transfer-proof submission.
type PaymentStatus string

const (
	PaymentPending  PaymentStatus = "pending"
	PaymentVerified PaymentStatus = "verified"
	PaymentRejected PaymentStatus = "rejected"
)

// paymentTransitions is the single source of truth for allowed status changes.
// A verified payment is immutable: it is never rejected, edited or deleted —
// corrections only happen through a reasoned reversal entry.
var paymentTransitions = map[PaymentStatus]map[PaymentStatus]bool{
	PaymentPending: {PaymentVerified: true, PaymentRejected: true},
}

// ValidatePaymentTransition enforces the payment state machine.
func ValidatePaymentTransition(from, to PaymentStatus) error {
	if from == to {
		return fmt.Errorf("payment is already %s", from)
	}
	if !paymentTransitions[from][to] {
		return fmt.Errorf("invalid payment transition %s -> %s", from, to)
	}
	return nil
}

// PaymentAction maps the admin review action names to a payment status.
func PaymentAction(action string) (PaymentStatus, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "verify", "verified", "approve", "approved":
		return PaymentVerified, nil
	case "reject", "rejected":
		return PaymentRejected, nil
	default:
		return "", fmt.Errorf("unknown payment action %q", action)
	}
}

// Allocation is a partial or full settlement of a payment against one bill
// line (an empty BillItemID settles the bill as a whole).
type Allocation struct {
	BillItemID string `json:"bill_item_id"`
	AmountIDR  int64  `json:"amount_idr"`
}

// ValidateAllocations enforces the invariant that a verified payment is fully
// allocated: every line is positive and the lines sum exactly to the payment.
func ValidateAllocations(paymentAmount int64, allocations []Allocation) error {
	if paymentAmount <= 0 {
		return errors.New("payment amount must be positive")
	}
	if len(allocations) == 0 {
		return errors.New("at least one allocation is required to verify a payment")
	}
	var sum int64
	for _, a := range allocations {
		if a.AmountIDR <= 0 {
			return errors.New("allocation amount must be positive")
		}
		sum += a.AmountIDR
	}
	if sum != paymentAmount {
		return fmt.Errorf("allocations total %d does not match payment amount %d", sum, paymentAmount)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Outstanding balance
// ---------------------------------------------------------------------------

// Outstanding computes the remaining balance of a bill:
//
//	outstanding = snapshot net - verified allocations + reversals
//
// and floors at zero (an overpayment never shows as negative debt). A reversal
// restores exactly the amount it reverses, which makes
// "verify then reverse" preserve the original balance.
func Outstanding(netIDR int64, verified, reversals []int64) (int64, error) {
	if netIDR < 0 {
		return 0, errors.New("bill net amount must not be negative")
	}
	out := netIDR
	for _, v := range verified {
		if v < 0 {
			return 0, errors.New("verified amount must not be negative")
		}
		out -= v
	}
	for _, v := range reversals {
		if v < 0 {
			return 0, errors.New("reversal amount must not be negative")
		}
		out += v
	}
	if out < 0 {
		out = 0
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Discount / scholarship engine
// ---------------------------------------------------------------------------

// DiscountKind is the discount mode. Percent values are expressed in basis
// points (10000 == 100%) so no floating point arithmetic is ever used.
type DiscountKind string

const (
	DiscountPercent DiscountKind = "percent"
	DiscountNominal DiscountKind = "nominal"
	percentScale                 = 10000
)

// Valid reports whether k is a known discount kind.
func (k DiscountKind) Valid() bool { return k == DiscountPercent || k == DiscountNominal }

// DiscountCandidate is one scholarship/discount considered while computing a
// bill. Value is basis points for percent rules and rupiah for nominal rules.
type DiscountCandidate struct {
	ID        string
	Source    string // e.g. "scholarship", "sibling", "yayasan"
	Label     string
	Kind      DiscountKind
	Value     int64
	Cap       int64 // 0 means "no per-rule cap"
	Priority  int
	CreatedAt time.Time
}

// DiscountStep is the immutable audit snapshot for one applied discount.
type DiscountStep struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	Label      string `json:"label"`
	Priority   int    `json:"priority"`
	BaseBefore int64  `json:"base_before"`
	Requested  int64  `json:"requested"`
	Applied    int64  `json:"applied"`
	BaseAfter  int64  `json:"base_after"`
}

// ApplyDiscounts stacks every candidate onto base in a deterministic order:
// ascending priority, then ascending created_at, then id as a stable
// tie-break. Each step records the running base, so the returned slice is a
// complete ordered audit snapshot. The floor is zero: a discount can never
// make a bill negative.
func ApplyDiscounts(base int64, in []DiscountCandidate) (int64, []DiscountStep, error) {
	if base < 0 {
		return 0, nil, errors.New("base amount must not be negative")
	}
	ordered := append([]DiscountCandidate(nil), in...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority < ordered[j].Priority
		}
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})

	out := base
	steps := make([]DiscountStep, 0, len(ordered))
	for _, d := range ordered {
		if d.Value < 0 {
			return 0, nil, fmt.Errorf("discount %s has a negative value", d.ID)
		}
		var requested int64
		switch d.Kind {
		case DiscountNominal:
			requested = d.Value
		case DiscountPercent:
			if d.Value > percentScale {
				return 0, nil, fmt.Errorf("discount %s exceeds 100%%", d.ID)
			}
			requested = out * d.Value / percentScale
		default:
			return 0, nil, fmt.Errorf("discount %s has an invalid kind %q", d.ID, d.Kind)
		}
		applied := requested
		if d.Cap > 0 && applied > d.Cap {
			applied = d.Cap
		}
		if applied > out {
			applied = out
		}
		before := out
		out -= applied
		label := d.Label
		if label == "" {
			label = d.Source
		}
		steps = append(steps, DiscountStep{
			ID:         d.ID,
			Source:     d.Source,
			Label:      label,
			Priority:   d.Priority,
			BaseBefore: before,
			Requested:  requested,
			Applied:    applied,
			BaseAfter:  out,
		})
	}
	return out, steps, nil
}

// ---------------------------------------------------------------------------
// Billing periods and idempotent generation
// ---------------------------------------------------------------------------

// BillKey uniquely identifies a generated bill: one bill per student, scheme
// and billing period. This is exactly the student_bills unique constraint and
// is what makes generation idempotent.
type BillKey struct {
	StudentUserID string
	Year          int
	Month         int
	SchemeID      string
}

// BillCandidate is a bill the caller wants generated. Its Key is the identity
// used for de-duplication against existing rows and within the same batch.
type BillCandidate struct {
	Key BillKey
}

// PlanGeneration returns the candidates that still need inserting: it drops
// any requested bill whose key already exists and collapses duplicates inside
// the batch. Running generate twice with the same period therefore yields an
// empty plan the second time.
func PlanGeneration(existing []BillKey, candidates []BillCandidate) ([]BillCandidate, error) {
	seen := make(map[BillKey]bool, len(existing)+len(candidates))
	for _, k := range existing {
		seen[normalizeKey(k)] = true
	}
	out := make([]BillCandidate, 0, len(candidates))
	for _, c := range candidates {
		k := normalizeKey(c.Key)
		if err := validateKey(k); err != nil {
			return nil, err
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, BillCandidate{Key: k})
	}
	return out, nil
}

func normalizeKey(k BillKey) BillKey {
	return BillKey{
		StudentUserID: strings.TrimSpace(k.StudentUserID),
		Year:          k.Year,
		Month:         k.Month,
		SchemeID:      strings.TrimSpace(k.SchemeID),
	}
}

func validateKey(k BillKey) error {
	if k.StudentUserID == "" {
		return errors.New("student_user_id is required")
	}
	if k.SchemeID == "" {
		return errors.New("scheme_id is required")
	}
	if k.Month < 1 || k.Month > 12 {
		return fmt.Errorf("period_month %d is out of range", k.Month)
	}
	if k.Year < 2000 || k.Year > 2100 {
		return fmt.Errorf("period_year %d is out of range", k.Year)
	}
	return nil
}

// DueDate returns the due date for a monthly charge: day clamped to the last
// day of the month (e.g. day 31 in February becomes the 28th/29th).
func DueDate(year, month, day int) time.Time {
	if day < 1 {
		day = 1
	}
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// BillItem is one line of a generated bill (SPP bulanan, uang gedung, seragam,
// kegiatan). Amounts are non-negative rupiah.
type BillItem struct {
	Name          string
	ComponentCode string
	AmountIDR     int64
}

// TotalBase sums a scheme's line items after validating them.
func TotalBase(items []BillItem) (int64, error) {
	var total int64
	for _, it := range items {
		if it.AmountIDR < 0 {
			return 0, fmt.Errorf("item %q has a negative amount", it.Name)
		}
		total += it.AmountIDR
	}
	return total, nil
}

// ---------------------------------------------------------------------------
// Refund (PRD menu 25)
// ---------------------------------------------------------------------------

// RefundSource is a verified payment and how much of it is still unreversed.
type RefundSource struct {
	PaymentID string
	Remaining int64
}

// RefundPart is the reversal a processed refund writes against one payment.
type RefundPart struct {
	PaymentID string
	AmountIDR int64
}

// SpreadRefund splits a refund across verified payments in the order given
// (callers pass newest first), never taking more than a payment's unreversed
// remainder. It fails if the payments cannot cover the amount.
func SpreadRefund(amount int64, sources []RefundSource) ([]RefundPart, error) {
	if amount <= 0 {
		return nil, errors.New("refund amount must be positive")
	}
	left := amount
	var out []RefundPart
	for _, s := range sources {
		if left == 0 {
			break
		}
		if s.Remaining <= 0 {
			continue
		}
		take := min(s.Remaining, left)
		out = append(out, RefundPart{PaymentID: s.PaymentID, AmountIDR: take})
		left -= take
	}
	if left > 0 {
		return nil, fmt.Errorf("refund exceeds the paid balance by %d", left)
	}
	return out, nil
}
