package model

import (
	"errors"
	"strings"
	"time"
)

// MaxReportDays bounds a finance report range so a single request cannot
// scan years of ledger rows.
const MaxReportDays = 366

// ReportRange is a closed date range [From, To] in the school's time zone.
// Start and End are the matching instants, End being exclusive (00:00 of
// the day after To).
type ReportRange struct {
	From, To   time.Time
	Start, End time.Time
}

// ParseReportRange validates the from/to query parameters (YYYY-MM-DD).
// ok is false when neither was given; one without the other, a bad date, a
// reversed range or a range longer than MaxReportDays is an error.
func ParseReportRange(from, to string, loc *time.Location) (ReportRange, bool, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" && to == "" {
		return ReportRange{}, false, nil
	}
	if from == "" || to == "" {
		return ReportRange{}, false, errors.New("from dan to harus diisi bersamaan (YYYY-MM-DD)")
	}
	f, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return ReportRange{}, false, errors.New("from harus berformat YYYY-MM-DD")
	}
	t, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil {
		return ReportRange{}, false, errors.New("to harus berformat YYYY-MM-DD")
	}
	if t.Before(f) {
		return ReportRange{}, false, errors.New("rentang tanggal terbalik: from harus sebelum atau sama dengan to")
	}
	if CalendarDaysBetween(f, t, loc)+1 > MaxReportDays {
		return ReportRange{}, false, errors.New("rentang laporan maksimal 366 hari")
	}
	return ReportRange{From: f, To: t, Start: f, End: t.AddDate(0, 0, 1)}, true, nil
}

// PeriodRange is the range of a whole year (month 0) or a single month.
func PeriodRange(year, month int, loc *time.Location) ReportRange {
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(1, 0, 0)
	if month >= 1 && month <= 12 {
		start = time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 1, 0)
	}
	return ReportRange{From: start, To: end.AddDate(0, 0, -1), Start: start, End: end}
}

// Months lists the (year, month) pairs the range touches, oldest first.
func (r ReportRange) Months() [][2]int {
	var out [][2]int
	y, m := r.From.Year(), int(r.From.Month())
	for {
		out = append(out, [2]int{y, m})
		if y == r.To.Year() && m == int(r.To.Month()) {
			return out
		}
		m++
		if m > 12 {
			m, y = 1, y+1
		}
	}
}

// Aging buckets (days past due at the as-of date).
const (
	AgingCurrent = "current"
	Aging1to30   = "d1_30"
	Aging31to60  = "d31_60"
	Aging61to90  = "d61_90"
	AgingOver90  = "d90_plus"
)

// AgingBuckets is the display order of the buckets.
var AgingBuckets = []string{AgingCurrent, Aging1to30, Aging31to60, Aging61to90, AgingOver90}

// AgingBucket classifies an open balance by the whole days since its due
// date at asOf. A bill without a due date, or not yet due, is current.
func AgingBucket(due *time.Time, asOf time.Time) string {
	if due == nil || !asOf.After(*due) {
		return AgingCurrent
	}
	switch days := int(asOf.Sub(*due).Hours() / 24); {
	case days <= 30:
		return Aging1to30
	case days <= 60:
		return Aging31to60
	case days <= 90:
		return Aging61to90
	default:
		return AgingOver90
	}
}

// AgingBucketSQL mirrors AgingBucket for a due_at column against the
// as-of instant bound to placeholder ph. Keep both in step.
func AgingBucketSQL(dueCol, ph string) string {
	return `CASE WHEN ` + dueCol + ` IS NULL OR ` + dueCol + ` >= ` + ph + ` THEN 'current'
	             WHEN ` + ph + ` - ` + dueCol + ` < interval '31 days' THEN 'd1_30'
	             WHEN ` + ph + ` - ` + dueCol + ` < interval '61 days' THEN 'd31_60'
	             WHEN ` + ph + ` - ` + dueCol + ` < interval '91 days' THEN 'd61_90'
	             ELSE 'd90_plus' END`
}

// MonthEnd is one point of a monthly aging trend: the balance as of At
// (exclusive), labelled with its calendar month.
type MonthEnd struct {
	Year, Month int
	At          time.Time
}

// TrendPoints returns the last n month-end cut-offs up to asOf, oldest
// first. The current month is measured at asOf itself.
func TrendPoints(asOf time.Time, n int, loc *time.Location) []MonthEnd {
	local := asOf.In(loc)
	first := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	out := make([]MonthEnd, 0, n)
	for i := n - 1; i >= 0; i-- {
		m := first.AddDate(0, -i, 0)
		at := m.AddDate(0, 1, 0)
		if i == 0 {
			at = asOf
		}
		out = append(out, MonthEnd{Year: m.Year(), Month: int(m.Month()), At: at})
	}
	return out
}

// SplitByWeights divides amount over the weights in proportion, handing the
// rounding remainder to the largest fractional parts (ties: earliest), so
// the parts always add up to amount. All-zero or empty weights split evenly.
func SplitByWeights(amount int64, weights []int64) []int64 {
	n := len(weights)
	if n == 0 {
		return nil
	}
	w := make([]int64, n)
	var total int64
	for i, x := range weights {
		if x > 0 {
			w[i] = x
			total += x
		}
	}
	if total == 0 {
		for i := range w {
			w[i] = 1
		}
		total = int64(n)
	}
	out := make([]int64, n)
	rem := make([]int64, n)
	var used int64
	for i := range w {
		out[i] = amount * w[i] / total
		rem[i] = amount * w[i] % total
		used += out[i]
	}
	for left := amount - used; left > 0; left-- {
		best := 0
		for i := 1; i < n; i++ {
			if rem[i] > rem[best] {
				best = i
			}
		}
		out[best]++
		rem[best] = -1
	}
	return out
}
