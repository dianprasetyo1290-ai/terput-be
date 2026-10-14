package handler

import (
	"testing"
	"time"
)

func dp(s string) *time.Time {
	t, _ := time.Parse(dateFmt, s)
	return &t
}

func TestValidateTermDates(t *testing.T) {
	ys, ye := dp("2027-07-01"), dp("2028-06-30")
	sem1 := termSibling{Order: 1, StartDate: dp("2027-07-12"), EndDate: dp("2027-12-20")}
	sem2 := termSibling{Order: 2, StartDate: dp("2028-01-05"), EndDate: dp("2028-06-20")}
	cases := []struct {
		name       string
		order      int
		start, end *time.Time
		ys, ye     *time.Time
		sib        []termSibling
		want       []string
	}{
		{"valid semester 1", 1, dp("2027-07-12"), dp("2027-12-20"), ys, ye, []termSibling{sem2}, nil},
		{"end before start (BUG-071)", 1, dp("2026-10-14"), dp("2026-01-01"), ys, ye, nil, []string{"end_date"}},
		{"same day", 1, dp("2027-08-01"), dp("2027-08-01"), ys, ye, nil, []string{"end_date"}},
		{"outside year (BUG-072)", 2, dp("2028-01-05"), dp("2028-10-31"), ys, ye, []termSibling{sem1}, []string{"end_date"}},
		{"start before year", 1, dp("2027-06-01"), dp("2027-12-20"), ys, ye, nil, []string{"start_date"}},
		{"legacy year without range", 1, dp("2026-07-01"), dp("2026-12-20"), nil, nil, nil, nil},
		{"overlaps semester 1", 2, dp("2027-12-01"), dp("2028-06-20"), ys, ye, []termSibling{sem1}, []string{"start_date"}},
		{"overlaps semester 2", 1, dp("2027-07-12"), dp("2028-01-10"), ys, ye, []termSibling{sem2}, []string{"end_date"}},
		{"only end given", 1, nil, dp("2027-12-20"), ys, ye, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := validateTermDates(c.order, c.start, c.end, c.ys, c.ye, c.sib)
			if len(errs) != len(c.want) {
				t.Fatalf("got %v", errs)
			}
			for _, f := range c.want {
				if errs[f] == "" {
					t.Fatalf("missing %s: %v", f, errs)
				}
			}
		})
	}
}
