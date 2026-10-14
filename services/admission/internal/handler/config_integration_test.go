package handler

import (
	"context"
	"strings"
	"testing"
	"time"
)

// BUG-110/031/162: eligibility and quota are checked when an application is
// created and registration numbers are sequential per wave.
func TestNewApplicationChecksAndNumbers(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 1)
	mustExec(t, a.DB, `UPDATE admission_tracks SET eligibility_config='{"min_age_years":6,"age_reference_date":"2027-07-01"}' WHERE id=$1`, s.track)
	young := mustID(t, a.DB, `INSERT INTO applicants(school_id,guardian_id,full_name,birth_date) VALUES($1,$2,'Adik','2022-01-01') RETURNING id`, s.school, s.guardian)
	old := mustID(t, a.DB, `INSERT INTO applicants(school_id,guardian_id,full_name,birth_date) VALUES($1,$2,'Kakak','2020-01-01') RETURNING id`, s.school, s.guardian)

	check := func(applicant string) ([]string, error) {
		tx, err := a.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		return a.checkNewApplication(ctx, tx, s.school, createApplicationInput{ApplicantID: applicant, PeriodID: s.period, WaveID: s.wave, TrackID: s.track})
	}
	if _, err := check(young); err == nil || !strings.Contains(err.Error(), "minimal 6 tahun") {
		t.Fatalf("under-age applicant accepted: %v", err)
	}
	warnings, err := check(old)
	if err != nil || len(warnings) != 1 {
		t.Fatalf("eligible applicant: %v %v", warnings, err)
	}
	seedApplication(t, a.DB, s, s.guardian, "submitted") // takes the only seat
	if _, err := check(old); err == nil || !strings.Contains(err.Error(), "Kuota") {
		t.Fatalf("full track accepted: %v", err)
	}

	var numbers []string
	for i := 0; i < 2; i++ {
		tx, _ := a.DB.BeginTx(ctx, nil)
		n, err := nextRegistrationNumber(ctx, tx, s.school, s.wave)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, n)
	}
	if numbers[0] != "PPDB-2027-G01-0001" || numbers[1] != "PPDB-2027-G01-0002" {
		t.Fatalf("numbers %v", numbers)
	}
}

// BUG-030: the funnel counts every stage an application reached.
func TestDashboardFunnel(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 10)
	seedApplication(t, a.DB, s, s.guardian, "draft")
	for _, path := range [][]string{
		{"submitted"},
		{"submitted", "document_review", "eligible_for_test", "tested", "ranked", "offered"},
		{"submitted", "document_review", "eligible_for_test", "tested", "ranked", "offered", "reregistration"},
	} {
		app := seedApplication(t, a.DB, s, s.guardian, path[len(path)-1])
		for i, st := range path {
			mustExec(t, a.DB, `INSERT INTO application_state_events(school_id,application_id,to_status,actor_type,actor_id,version) VALUES($1,$2,$3,'system','t',$4)`,
				s.school, app, st, i+1)
		}
	}
	c, err := a.dashboardFunnel(ctx, dashboardFilter{school: s.school, period: s.period})
	if err != nil {
		t.Fatal(err)
	}
	if c.Registered != 4 || c.Submitted != 3 || c.Verified != 2 || c.Accepted != 2 || c.Reregistered != 1 {
		t.Fatalf("funnel %+v", c)
	}
	trend, err := a.dashboardTrend(ctx, dashboardFilter{school: s.school, period: s.period}, time.Now())
	if err != nil || len(trend) != 30 || trend[29]["count"].(int) != 4 {
		t.Fatalf("trend %v %v", trend, err)
	}
	if _, err := a.dashboardMoney(ctx, dashboardFilter{school: s.school, period: s.period}); err != nil {
		t.Fatal(err)
	}
	for _, dim := range []string{"wave", "track"} {
		if list, err := a.dashboardBreakdown(ctx, dashboardFilter{school: s.school, period: s.period}, dim); err != nil || len(list) != 1 {
			t.Fatalf("%s breakdown %v %v", dim, list, err)
		}
	}
}
