package handler

import (
	"time"
)

// termSibling is another semester of the same academic year.
type termSibling struct {
	Order     int
	StartDate *time.Time
	EndDate   *time.Time
}

const dateFmt = "2006-01-02"

// validateTermDates is the pure rule behind UpdateTerm (QA-IDN BUG-071/072):
//   - end after start;
//   - inside the academic year's range when identity knows it;
//   - Semester n must not overlap Semester n-1 / n+1.
//
// Returns field -> Indonesian message.
func validateTermDates(order int, start, end, yearStart, yearEnd *time.Time, siblings []termSibling) map[string]string {
	errs := map[string]string{}
	if start != nil && end != nil && !end.After(*start) {
		errs["end_date"] = "Tanggal selesai harus setelah tanggal mulai"
		return errs
	}
	if yearStart != nil && yearEnd != nil {
		rng := " (" + yearStart.Format(dateFmt) + " s.d. " + yearEnd.Format(dateFmt) + ")"
		if start != nil && (start.Before(*yearStart) || start.After(*yearEnd)) {
			errs["start_date"] = "Tanggal mulai semester di luar rentang tahun ajaran" + rng
		}
		if end != nil && (end.After(*yearEnd) || end.Before(*yearStart)) {
			errs["end_date"] = "Tanggal selesai semester di luar rentang tahun ajaran" + rng
		}
	}
	for _, s := range siblings {
		switch {
		case s.Order < order && s.EndDate != nil && start != nil && !start.After(*s.EndDate):
			errs["start_date"] = "Tanggal mulai harus setelah semester sebelumnya selesai (" + s.EndDate.Format(dateFmt) + ")"
		case s.Order > order && s.StartDate != nil && end != nil && !end.Before(*s.StartDate):
			errs["end_date"] = "Tanggal selesai harus sebelum semester berikutnya dimulai (" + s.StartDate.Format(dateFmt) + ")"
		}
	}
	return errs
}

func parseDatePtr(s *string) (*time.Time, bool) {
	if s == nil || *s == "" {
		return nil, true
	}
	t, err := time.Parse(dateFmt, *s)
	if err != nil {
		return nil, false
	}
	return &t, true
}

// dateOnly drops the clock/zone of a DATE scanned as time.Time.
func dateOnly(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return &d
}
