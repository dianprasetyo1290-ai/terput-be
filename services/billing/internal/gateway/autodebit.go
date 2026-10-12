package gateway

import (
	"sort"
	"time"
)

// Mandate statuses (autodebit_mandates.status).
const (
	MandatePending   = "pending_activation"
	MandateActive    = "active"
	MandateCancelled = "cancelled"
	MandateFailed    = "failed"
)

// Attempt statuses (autodebit_attempts.status) and skip reasons.
const (
	AttemptScheduled  = "scheduled"
	AttemptProcessing = "processing"
	AttemptSuccess    = "success"
	AttemptFailed     = "failed"
	AttemptSkipped    = "skipped"

	SkipMaxAmount        = "max_amount_exceeded"
	SkipMandateCancelled = "mandate_cancelled"
	SkipBillSettled      = "bill_settled"
)

// Mandate is the autodebit view of a mandate.
type Mandate struct {
	ID            string
	StudentUserID string
	Status        string
	SchemeIDs     []string
	MaxAmountIDR  int64 // 0 = no limit
	CancelledAt   *time.Time
}

// DueBill is an open bill a mandate might cover. SchemeID is "" for ad-hoc
// bills, which are never debited automatically.
type DueBill struct {
	ID             string
	StudentUserID  string
	SchemeID       string
	DueDate        time.Time
	OutstandingIDR int64
}

// Settings is a school's autodebit_settings row.
type Settings struct {
	MaxRetries        int
	RetryIntervalDays int // 0 = not configured
	RunHour           int // -1 = not configured
}

// Ready reports whether the school configured everything the job needs.
func (s Settings) Ready() bool { return s.RetryIntervalDays > 0 && s.RunHour >= 0 && s.RunHour <= 23 }

// PlannedAttempt is a first attempt DueDebits wants created.
type PlannedAttempt struct {
	MandateID  string
	BillID     string
	AttemptNo  int
	AmountIDR  int64
	Status     string // scheduled | skipped
	SkipReason string
}

// CanDebit reports whether a mandate may be debited at the given moment
// (FR-151-003, BR-PAY-04): only active mandates, and never at or after the
// moment it was cancelled.
func CanDebit(m Mandate, at time.Time) bool {
	if m.Status != MandateActive {
		return false
	}
	return m.CancelledAt == nil || at.Before(*m.CancelledAt)
}

func dateOnly(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
}

// DueDebits plans the first debit attempt for every open scheme bill that
// is due on or before today, belongs to a debitable mandate covering its
// scheme and has no attempt yet (FR-151-002). A bill above the mandate's
// limit is planned as skipped so the parent can be told. existing holds the
// bill ids that already have an attempt. One mandate per bill.
func DueDebits(mandates []Mandate, bills []DueBill, existing map[string]bool, today time.Time, s Settings) []PlannedAttempt {
	if !s.Ready() {
		return nil
	}
	ms := append([]Mandate(nil), mandates...)
	sort.Slice(ms, func(i, j int) bool { return ms[i].ID < ms[j].ID })
	bs := append([]DueBill(nil), bills...)
	sort.Slice(bs, func(i, j int) bool {
		if !bs[i].DueDate.Equal(bs[j].DueDate) {
			return bs[i].DueDate.Before(bs[j].DueDate)
		}
		return bs[i].ID < bs[j].ID
	})
	day := dateOnly(today)
	var out []PlannedAttempt
	for _, b := range bs {
		if b.SchemeID == "" || b.OutstandingIDR <= 0 || existing[b.ID] || dateOnly(b.DueDate).After(day) {
			continue
		}
		for _, m := range ms {
			if m.StudentUserID != b.StudentUserID || !HasCapability(m.SchemeIDs, b.SchemeID) || !CanDebit(m, today) {
				continue
			}
			p := PlannedAttempt{MandateID: m.ID, BillID: b.ID, AttemptNo: 1, AmountIDR: b.OutstandingIDR, Status: AttemptScheduled}
			if m.MaxAmountIDR > 0 && b.OutstandingIDR > m.MaxAmountIDR {
				p.Status, p.SkipReason = AttemptSkipped, SkipMaxAmount
			}
			out = append(out, p)
			break
		}
	}
	return out
}

// NextRetry decides what follows a failed attempt (FR-151-002, AC-151-01):
// another attempt retry_interval_days later while 1 + max_retries attempts
// have not been used, otherwise stop (ok=false) and tell the parent to pay
// manually.
func NextRetry(attemptNo int, failedOn time.Time, s Settings) (next int, on time.Time, ok bool) {
	if s.RetryIntervalDays <= 0 || attemptNo >= 1+s.MaxRetries {
		return 0, time.Time{}, false
	}
	return attemptNo + 1, dateOnly(failedOn).AddDate(0, 0, s.RetryIntervalDays), true
}

// RetailExpiry is when a retail pay code stops being valid (FR-151-004).
func RetailExpiry(validMinutes int, now time.Time) time.Time {
	return now.Add(time.Duration(validMinutes) * time.Minute)
}

// RetailExpired reports whether a pending retail code is past its validity.
func RetailExpired(expiresAt, now time.Time) bool { return !now.Before(expiresAt) }
