package discipline

import (
	"testing"
)

func TestAccumulate_FR13102(t *testing.T) {
	year1 := "year-1"
	year2 := "year-2"

	records := []Record{
		{Kind: TypeKindViolation, Points: 15, Status: RecordStatusActive, AcademicYearID: &year1},
		{Kind: TypeKindViolation, Points: 20, Status: RecordStatusAppealed, AcademicYearID: &year1}, // appealed still counts
		{Kind: TypeKindViolation, Points: 30, Status: RecordStatusAnnulled, AcademicYearID: &year1}, // annulled is excluded
		{Kind: TypeKindMerit, Points: 10, Status: RecordStatusActive, AcademicYearID: &year1},      // merit separate
		{Kind: TypeKindViolation, Points: 25, Status: RecordStatusActive, AcademicYearID: &year2},  // other year
	}

	// Scope: Academic Year 1
	violation, merit := Accumulate(records, ScopeAcademicYear, &year1)
	if violation != 35 { // 15 + 20
		t.Errorf("expected violation points 35, got %d", violation)
	}
	if merit != 10 {
		t.Errorf("expected merit points 10, got %d", merit)
	}

	// Scope: All Time
	violationAll, meritAll := Accumulate(records, ScopeAllTime, nil)
	if violationAll != 60 { // 15 + 20 + 25
		t.Errorf("expected violation all-time 60, got %d", violationAll)
	}
	if meritAll != 10 {
		t.Errorf("expected merit all-time 10, got %d", meritAll)
	}
}

func TestCrossedThresholds_AC13101(t *testing.T) {
	th50 := Threshold{ID: "th-50", Points: 50, ActionName: "Peringatan I", EffectiveFrom: "2026-01-01"}
	th75 := Threshold{ID: "th-75", Points: 75, ActionName: "Panggilan Ortu I", EffectiveFrom: "2026-01-01"}
	th100 := Threshold{ID: "th-100", Points: 100, ActionName: "Skorsing", EffectiveFrom: "2026-01-01"}

	thresholds := []Threshold{th100, th50, th75} // unordered to test sort

	// Vector 1: before=45, after=55 -> crossed [50] (AC-131-01)
	res1 := CrossedThresholds(45, 55, thresholds, nil, "2026-10-15")
	if len(res1) != 1 || res1[0].ID != "th-50" {
		t.Fatalf("expected crossed [th-50], got %v", res1)
	}

	// Vector 2: 40 -> 100 with 50, 75, 100 -> all 3, sorted ascending
	res2 := CrossedThresholds(40, 100, thresholds, nil, "2026-10-15")
	if len(res2) != 3 {
		t.Fatalf("expected 3 thresholds, got %d", len(res2))
	}
	if res2[0].Points != 50 || res2[1].Points != 75 || res2[2].Points != 100 {
		t.Errorf("expected sorted [50, 75, 100], got %v, %v, %v", res2[0].Points, res2[1].Points, res2[2].Points)
	}

	// Vector 3: 50 -> 60 with 50 already triggered -> []
	triggered := map[string]bool{"th-50": true}
	res3 := CrossedThresholds(50, 60, thresholds, triggered, "2026-10-15")
	if len(res3) != 0 {
		t.Errorf("expected 0 thresholds when 50 already triggered, got %d", len(res3))
	}

	// Vector 4: effective date not matching
	futureDate := "2025-01-01" // before effectiveFrom 2026-01-01
	res4 := CrossedThresholds(45, 55, thresholds, nil, futureDate)
	if len(res4) != 0 {
		t.Errorf("expected 0 thresholds when date is before effective date, got %d", len(res4))
	}
}

func TestCanAppeal_FR13104(t *testing.T) {
	activeRecord := Record{Status: RecordStatusActive}
	appealedRecord := Record{Status: RecordStatusAppealed}
	annulledRecord := Record{Status: RecordStatusAnnulled}

	// Active without pending appeal -> true
	if !CanAppeal(activeRecord, 0) {
		t.Errorf("expected true for active record with 0 pending appeals")
	}

	// Active with 1 pending appeal -> false
	if CanAppeal(activeRecord, 1) {
		t.Errorf("expected false for active record with pending appeal")
	}

	// Already appealed or annulled -> false
	if CanAppeal(appealedRecord, 0) {
		t.Errorf("expected false for appealed record")
	}
	if CanAppeal(annulledRecord, 0) {
		t.Errorf("expected false for annulled record")
	}
}
