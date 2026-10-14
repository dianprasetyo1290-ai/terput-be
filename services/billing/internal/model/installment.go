package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Component frequencies (FR-18-001): a monthly component is billed every
// period, an annual one once per student per academic year and a one-time
// one once per student for good (uang pangkal).
const (
	KindMonthly = "monthly"
	KindAnnual  = "annual"
	KindOneTime = "one_time"
)

// ValidKind reports whether k is a known component frequency.
func ValidKind(k string) bool {
	return k == KindMonthly || k == KindAnnual || k == KindOneTime
}

// PlannedComponent is a scheme component as generation sees it.
type PlannedComponent struct {
	Code, Name, Kind string
	AmountIDR        int64
	// InstallmentTerms/IntervalMonths are the default installment plan of
	// the component (BUG-126); 1 term = paid at once.
	InstallmentTerms, IntervalMonths int
}

// ComponentsForStudent picks the components one student is billed in a
// period: monthly always, annual unless already billed in the academic year,
// one-time unless ever billed. Codes are compared case-insensitively.
func ComponentsForStudent(comps []PlannedComponent, billedThisYear, billedEver map[string]bool) []PlannedComponent {
	var out []PlannedComponent
	for _, c := range comps {
		code := strings.ToUpper(strings.TrimSpace(c.Code))
		switch c.Kind {
		case KindAnnual:
			if billedThisYear[code] {
				continue
			}
		case KindOneTime, "":
			if billedEver[code] {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// Installment is one term of a bill (BUG-052).
type Installment struct {
	Seq       int       `json:"seq"`
	DueAt     time.Time `json:"due_at"`
	AmountIDR int64     `json:"amount_idr"`
}

// MaxInstallments bounds a bill's term count.
const MaxInstallments = 24

// AddMonthsClamped moves t by n calendar months keeping the day of month,
// clamped to the last day of the target month (31 Jan + 1 = 28/29 Feb).
func AddMonthsClamped(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	last := first.AddDate(0, 1, -1).Day()
	if d > last {
		d = last
	}
	return first.AddDate(0, 0, d-1)
}

// DefaultInstallments builds a bill's term schedule from its components'
// default plans: each component is split evenly over its terms, due every
// IntervalMonths from firstDue; parts falling on the same date form one
// term. The schedule is then scaled to net (the bill after discounts). It
// returns nil when the bill is paid at once (fewer than two terms).
func DefaultInstallments(net int64, firstDue time.Time, comps []PlannedComponent) []Installment {
	if net <= 0 {
		return nil
	}
	var dates []time.Time
	weights := map[time.Time]int64{}
	for _, c := range comps {
		terms := c.InstallmentTerms
		if terms < 1 {
			terms = 1
		}
		interval := c.IntervalMonths
		if interval < 1 {
			interval = 1
		}
		parts := SplitByWeights(c.AmountIDR, make([]int64, terms))
		for k, p := range parts {
			due := AddMonthsClamped(firstDue, k*interval)
			if _, ok := weights[due]; !ok {
				dates = append(dates, due)
			}
			weights[due] += p
		}
	}
	if len(dates) < 2 {
		return nil
	}
	sortTimes(dates)
	w := make([]int64, len(dates))
	for i, d := range dates {
		w[i] = weights[d]
	}
	return buildTerms(dates, SplitByWeights(net, w))
}

// RescaleInstallments keeps a schedule's dates and proportions for a new
// net amount (after an approved adjustment). Terms that become zero are
// dropped; fewer than two terms means the bill is paid at once (nil).
func RescaleInstallments(ins []Installment, net int64) []Installment {
	if net <= 0 || len(ins) < 2 {
		return nil
	}
	dates := make([]time.Time, len(ins))
	w := make([]int64, len(ins))
	for i, t := range ins {
		dates[i], w[i] = t.DueAt, t.AmountIDR
	}
	out := buildTerms(dates, SplitByWeights(net, w))
	if len(out) < 2 {
		return nil
	}
	return out
}

func buildTerms(dates []time.Time, amounts []int64) []Installment {
	var out []Installment
	for i, a := range amounts {
		if a > 0 {
			out = append(out, Installment{Seq: len(out) + 1, DueAt: dates[i], AmountIDR: a})
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

func sortTimes(ts []time.Time) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && ts[j].Before(ts[j-1]); j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}

// ValidateInstallments checks a manual schedule: 2..MaxInstallments terms,
// positive amounts summing exactly to the bill's net, due dates strictly
// increasing. Seq is renumbered 1..n.
func ValidateInstallments(net int64, ins []Installment) ([]Installment, error) {
	if len(ins) < 2 {
		return nil, errors.New("termin minimal 2; hapus jadwal termin untuk membayar sekaligus")
	}
	if len(ins) > MaxInstallments {
		return nil, fmt.Errorf("termin maksimal %d", MaxInstallments)
	}
	var sum int64
	out := make([]Installment, len(ins))
	for i, t := range ins {
		if t.AmountIDR <= 0 {
			return nil, fmt.Errorf("nominal termin %d harus lebih dari 0", i+1)
		}
		if t.DueAt.IsZero() {
			return nil, fmt.Errorf("jatuh tempo termin %d wajib diisi", i+1)
		}
		if i > 0 && !t.DueAt.After(ins[i-1].DueAt) {
			return nil, fmt.Errorf("jatuh tempo termin %d harus setelah termin %d", i+1, i)
		}
		sum += t.AmountIDR
		out[i] = Installment{Seq: i + 1, DueAt: t.DueAt, AmountIDR: t.AmountIDR}
	}
	if sum != net {
		return nil, fmt.Errorf("jumlah termin (%d) harus sama dengan nilai tagihan (%d)", sum, net)
	}
	return out, nil
}

// InstallmentState is a term with what has been paid against it.
type InstallmentState struct {
	Installment
	PaidIDR        int64  `json:"paid_idr"`
	OutstandingIDR int64  `json:"outstanding_idr"`
	Status         string `json:"status"` // paid | partial | unpaid
}

// InstallmentProgress applies the bill's paid amount to its terms in due
// order (oldest first).
func InstallmentProgress(ins []Installment, paid int64) []InstallmentState {
	out := make([]InstallmentState, len(ins))
	for i, t := range ins {
		use := min(max(paid, 0), t.AmountIDR)
		paid -= use
		s := InstallmentState{Installment: t, PaidIDR: use, OutstandingIDR: t.AmountIDR - use, Status: "unpaid"}
		switch {
		case use == t.AmountIDR:
			s.Status = "paid"
		case use > 0:
			s.Status = "partial"
		}
		out[i] = s
	}
	return out
}

// NextDue is the due date of the first term not fully paid, or nil when
// every term is paid (or there is no schedule).
func NextDue(ins []Installment, paid int64) *time.Time {
	for _, s := range InstallmentProgress(ins, paid) {
		if s.OutstandingIDR > 0 {
			d := s.DueAt
			return &d
		}
	}
	return nil
}

// MaxAnnualPeriods bounds one annual generation run (one academic year).
const MaxAnnualPeriods = 12

// AnnualPeriods lists the billing months from..to ("YYYY-MM", inclusive)
// for an annual run (BUG-128). An empty bound defaults to the scheme's
// validity window; the run is refused when neither gives a bound, when the
// range is reversed or longer than MaxAnnualPeriods months.
func AnnualPeriods(from, to string, effFrom, effTo *time.Time) ([][2]int, error) {
	parse := func(s string, def *time.Time, field string) (int, error) {
		s = strings.TrimSpace(s)
		if s == "" {
			if def == nil {
				return 0, fmt.Errorf("%s wajib diisi (skema tidak memiliki masa berlaku)", field)
			}
			return def.Year()*12 + int(def.Month()) - 1, nil
		}
		t, err := time.Parse("2006-01", s)
		if err != nil {
			return 0, fmt.Errorf("%s harus berformat YYYY-MM", field)
		}
		return t.Year()*12 + int(t.Month()) - 1, nil
	}
	f, err := parse(from, effFrom, "from")
	if err != nil {
		return nil, err
	}
	t, err := parse(to, effTo, "to")
	if err != nil {
		return nil, err
	}
	if t < f {
		return nil, errors.New("periode akhir tidak boleh sebelum periode awal")
	}
	if t-f+1 > MaxAnnualPeriods {
		return nil, fmt.Errorf("penerbitan tahunan maksimal %d bulan", MaxAnnualPeriods)
	}
	out := make([][2]int, 0, t-f+1)
	for p := f; p <= t; p++ {
		out = append(out, [2]int{p / 12, p%12 + 1})
	}
	return out, nil
}
