package bi

import (
	"testing"
	"time"

	"skci/insight/internal/common"
)

func TestResolveScope(t *testing.T) {
	sdID := "school-sd-uuid"
	smpID := "school-smp-uuid"
	smaID := "school-sma-uuid"
	otherID := "school-other-foundation-uuid"

	foundationSchools := []string{sdID, smpID, smaID}

	// 1. Superadmin -> gets requested
	saClaims := ScopeClaims{UserID: "sa-1", SchoolID: sdID, ActiveRole: common.RoleSuperadmin}
	scoped, err := ResolveScope(saClaims, nil, foundationSchools, []string{sdID, otherID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scoped) != 2 {
		t.Errorf("superadmin expected 2 schools, got %v", scoped)
	}

	// 2. Kepala Yayasan -> gets all foundation schools or clipped requested
	kyClaims := ScopeClaims{UserID: "ky-1", SchoolID: sdID, ActiveRole: common.RoleKepalaYayasan}
	scopedKY, err := ResolveScope(kyClaims, nil, foundationSchools, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopedKY) != 3 {
		t.Errorf("kepala_yayasan expected 3 schools, got %v", scopedKY)
	}

	// Kepala yayasan requesting a school outside foundation -> clipped!
	scopedKYClipped, err := ResolveScope(kyClaims, nil, foundationSchools, []string{sdID, otherID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopedKYClipped) != 1 || scopedKYClipped[0] != sdID {
		t.Errorf("kepala_yayasan outside foundation should be clipped to [SD], got %v", scopedKYClipped)
	}

	// 3. AC-152-01: Kepala Sekolah SD opens foundation dashboard with requested [SD, SMP] -> CLIPPED to [SD]
	ksClaims := ScopeClaims{UserID: "ks-sd", SchoolID: sdID, ActiveRole: common.RoleKepalaSekolah}
	scopedKS, err := ResolveScope(ksClaims, nil, foundationSchools, []string{sdID, smpID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopedKS) != 1 || scopedKS[0] != sdID {
		t.Errorf("AC-152-01: kepala_sekolah SD must only see [SD], got %v", scopedKS)
	}

	// 4. BI Analyst with grant for SMP -> sees [SD, SMP]
	grants := []AnalyticsScopeGrant{
		{UserID: "bia-1", SchoolID: sdID, GrantedSchoolID: smpID, RevokedAt: nil},
	}
	biaClaims := ScopeClaims{UserID: "bia-1", SchoolID: sdID, ActiveRole: common.RoleBiAnalyst}
	scopedBIA, err := ResolveScope(biaClaims, grants, foundationSchools, []string{sdID, smpID, smaID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopedBIA) != 2 || !contains(scopedBIA, sdID) || !contains(scopedBIA, smpID) {
		t.Errorf("bi_analyst with grant should see SD and SMP, got %v", scopedBIA)
	}

	// BI Analyst without grant for SMP -> only sees SD
	scopedBIANoGrant, err := ResolveScope(biaClaims, nil, foundationSchools, []string{sdID, smpID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopedBIANoGrant) != 1 || scopedBIANoGrant[0] != sdID {
		t.Errorf("bi_analyst without grant should be clipped to [SD], got %v", scopedBIANoGrant)
	}
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func TestNextRun(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")

	// Base time: 2026-01-15 08:00:00
	baseTime := time.Date(2026, time.January, 15, 8, 0, 0, 0, loc)

	// Daily at 07:00 -> should run tomorrow 2026-01-16 07:00
	dailySched := ScheduledReport{
		Frequency: "daily",
		SendHour:  7,
	}
	nextDaily := NextRun(dailySched, baseTime)
	if nextDaily.Format("2006-01-02 15:04") != "2026-01-16 07:00" {
		t.Errorf("expected 2026-01-16 07:00, got %s", nextDaily.Format("2006-01-02 15:04"))
	}

	// Monthly with month_day = 31 in January -> should run 2026-01-31 08:00
	monthDay31 := 31
	monthlySched := ScheduledReport{
		Frequency: "monthly",
		MonthDay:  &monthDay31,
		SendHour:  8,
	}
	nextMonthlyJan := NextRun(monthlySched, baseTime)
	if nextMonthlyJan.Format("2006-01-02 15:04") != "2026-01-31 08:00" {
		t.Errorf("expected 2026-01-31 08:00, got %s", nextMonthlyJan.Format("2006-01-02 15:04"))
	}

	// Monthly with month_day = 31 starting on 2026-02-01 (non-leap year Feb has 28 days)
	febBase := time.Date(2026, time.February, 1, 9, 0, 0, 0, loc)
	nextMonthlyFeb := NextRun(monthlySched, febBase)
	// Must adjust 31 to 28 in February!
	if nextMonthlyFeb.Format("2006-01-02 15:04") != "2026-02-28 08:00" {
		t.Errorf("expected 2026-02-28 08:00 for month_day=31 in Feb, got %s", nextMonthlyFeb.Format("2006-01-02 15:04"))
	}
}
