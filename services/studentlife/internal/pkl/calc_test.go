package pkl

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func parseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestMouCovering_AC13901(t *testing.T) {
	period := Period{
		StartsOn: parseDate("2026-01-01"),
		EndsOn:   parseDate("2026-06-30"),
	}

	// 1. Expired MoU ending 31 Dec 2025
	mousExpired := []PartnerMou{
		{
			MouNumber: "MOU/001",
			ValidFrom: parseDate("2025-01-01"),
			ValidTo:   parseDate("2025-12-31"),
		},
	}
	mou, err := MouCovering(mousExpired, period)
	if err == nil || mou != nil {
		t.Fatalf("expected error for expired MoU, got nil error")
	}
	expectedSub := "MoU mitra sudah tidak berlaku (berakhir 31 December 2025)"
	if err.Error() != expectedSub {
		t.Errorf("expected '%s', got '%s'", expectedSub, err.Error())
	}

	// 2. Active covering MoU ending 31 Dec 2026
	mousValid := []PartnerMou{
		{
			MouNumber: "MOU/002",
			ValidFrom: parseDate("2025-01-01"),
			ValidTo:   parseDate("2026-12-31"),
		},
	}
	mouValid, err := MouCovering(mousValid, period)
	if err != nil || mouValid == nil {
		t.Fatalf("expected valid MoU, got err: %v", err)
	}
	if mouValid.MouNumber != "MOU/002" {
		t.Errorf("expected MOU/002, got %s", mouValid.MouNumber)
	}
}

func TestCanPlace_AC14001(t *testing.T) {
	quota := &Quota{
		PartnerName: "PT Teknologi Teratai",
		MajorName:   "RPL",
		Quota:       4,
	}

	// 1. MoU error forwarded
	mouErr := errors.New("MoU expired")
	if err := CanPlace(quota, 2, mouErr); err != mouErr {
		t.Errorf("expected mouErr, got %v", err)
	}

	// 2. Quota not configured
	if err := CanPlace(nil, 0, nil); err == nil || err.Error() != "Kuota mitra untuk jurusan ini belum diatur" {
		t.Errorf("expected quota not configured error, got %v", err)
	}

	// 3. Quota full: 4/4
	if err := CanPlace(quota, 4, nil); err == nil {
		t.Errorf("expected quota full error, got nil")
	} else {
		expected := "Kuota mitra PT Teknologi Teratai jurusan RPL periode ini penuh (4/4)"
		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	}

	// 4. Quota available: 3/4
	if err := CanPlace(quota, 3, nil); err != nil {
		t.Errorf("expected allowed placement, got error: %v", err)
	}
}

func TestCountFilled(t *testing.T) {
	placements := []Placement{
		{Status: PlacementStatusPlanned},
		{Status: PlacementStatusActive},
		{Status: PlacementStatusCompleted},
		{Status: PlacementStatusCancelled},
	}
	if CountFilled(placements) != 3 {
		t.Errorf("expected 3 filled, got %d", CountFilled(placements))
	}
}

func TestCanRecordAttendance_FR140002(t *testing.T) {
	p := Placement{
		Status:   PlacementStatusActive,
		StartsOn: parseDate("2026-01-05"),
		EndsOn:   parseDate("2026-04-30"),
	}

	// Valid today
	if err := CanRecordAttendance(p, parseDate("2026-02-10")); err != nil {
		t.Errorf("expected allowed, got %v", err)
	}

	// Outside range
	if err := CanRecordAttendance(p, parseDate("2026-05-01")); err == nil {
		t.Errorf("expected error outside range, got nil")
	}

	// Inactive placement
	pInactive := p
	pInactive.Status = PlacementStatusCompleted
	if err := CanRecordAttendance(pInactive, parseDate("2026-02-10")); err == nil {
		t.Errorf("expected error for inactive placement, got nil")
	}
}

func TestAttendanceRecap_FR140003(t *testing.T) {
	rows := []Attendance{
		{Status: AttendanceStatusHadir, Confirmation: ConfirmationConfirmed},
		{Status: AttendanceStatusHadir, Confirmation: ConfirmationConfirmed},
		{Status: AttendanceStatusIzin, Confirmation: ConfirmationPending},
		{Status: AttendanceStatusSakit, Confirmation: ConfirmationRejected},
	}
	recap := AttendanceRecap(rows)
	if recap.Total != 4 || recap.Hadir != 2 || recap.Izin != 1 || recap.Sakit != 1 {
		t.Errorf("status counts mismatch: %+v", recap)
	}
	if recap.Confirmed != 2 || recap.Pending != 1 || recap.Rejected != 1 {
		t.Errorf("confirmation counts mismatch: %+v", recap)
	}
}

func TestValidateScheme_FR141002(t *testing.T) {
	scheme := Scheme{
		IndustryWeightBP: 7000,
		SchoolWeightBP:   3000,
	}

	components := []Component{
		{Assessor: AssessorIndustry, Name: "Keterampilan", WeightBP: 6000},
		{Assessor: AssessorIndustry, Name: "Kedisiplinan", WeightBP: 4000},
		{Assessor: AssessorSchool, Name: "Laporan", WeightBP: 10000},
	}

	if err := ValidateScheme(scheme, components); err != nil {
		t.Errorf("expected valid scheme, got %v", err)
	}

	// Invalid weights sum
	badScheme := Scheme{IndustryWeightBP: 5000, SchoolWeightBP: 4000}
	if err := ValidateScheme(badScheme, components); err == nil {
		t.Errorf("expected error when weights don't sum to 10000")
	}
}

func TestAssessorScore_FR141002(t *testing.T) {
	components := []Component{
		{ID: "c1", Assessor: AssessorIndustry, WeightBP: 6000},
		{ID: "c2", Assessor: AssessorIndustry, WeightBP: 4000},
	}

	// 1. Incomplete scores
	scoresIncomplete := []Score{
		{ComponentID: "c1", ScoreX100: 8000},
	}
	_, err := AssessorScore(components, scoresIncomplete, AssessorIndustry)
	if err == nil || err.Error() != "Penilaian industri belum lengkap" {
		t.Errorf("expected incomplete error, got %v", err)
	}

	// 2. Complete scores: 85 * 0.6 + 90 * 0.4 = 51 + 36 = 87
	scoresComplete := []Score{
		{ComponentID: "c1", ScoreX100: 8500},
		{ComponentID: "c2", ScoreX100: 9000},
	}
	score, err := AssessorScore(components, scoresComplete, AssessorIndustry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// (8500 * 6000 + 9000 * 4000 + 5000) / 10000 = (51000000 + 36000000 + 5000) / 10000 = 8700
	if score != 8700 {
		t.Errorf("expected 8700, got %d", score)
	}
}

func TestFinalScore_AC14101(t *testing.T) {
	scheme := Scheme{
		IndustryWeightBP: 7000, // 70%
		SchoolWeightBP:   3000, // 30%
	}

	// AC-141-01 test vector: industry 8500 (85), school 8000 (80)
	// (8500 * 7000 + 8000 * 3000 + 5000) / 10000 = (59500000 + 24000000 + 5000) / 10000 = 8350
	final := FinalScore(8500, 8000, scheme)
	if final != 8350 {
		t.Errorf("expected 8350, got %d", final)
	}

	// Round half up check:
	// industry 8550 * 7000 = 59850000; school 8550 * 3000 = 25650000 -> 8550
	if FinalScore(8550, 8550, scheme) != 8550 {
		t.Errorf("expected 8550")
	}
}

func TestFormatScore(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{8350, "83,5"},
		{8000, "80"},
		{8525, "85,25"},
		{9005, "90,05"},
		{10000, "100"},
	}
	for _, tc := range tests {
		got := FormatScore(tc.input)
		if got != tc.expected {
			t.Errorf("FormatScore(%d): expected '%s', got '%s'", tc.input, tc.expected, got)
		}
	}
}

func TestCanIssueCertificate_FR141003(t *testing.T) {
	pCompleted := Placement{Status: PlacementStatusCompleted}
	pActive := Placement{Status: PlacementStatusActive}

	now := time.Now()
	rFinalized := Result{FinalizedAt: &now}
	rNotFinalized := Result{FinalizedAt: nil}

	if !CanIssueCertificate(pCompleted, rFinalized) {
		t.Errorf("expected true for completed and finalized")
	}
	if CanIssueCertificate(pCompleted, rNotFinalized) {
		t.Errorf("expected false for unfinalized result")
	}
	if CanIssueCertificate(pActive, rFinalized) {
		t.Errorf("expected false for active placement")
	}
}

func TestCertificateNo(t *testing.T) {
	if got := CertificateNo("PKL-SMK", 2026, 1); got != "PKL-SMK/2026/0001" {
		t.Errorf("expected PKL-SMK/2026/0001, got %s", got)
	}
	if got := CertificateNo("", 2026, 42); got != "2026/0042" {
		t.Errorf("expected 2026/0042, got %s", got)
	}
}

func TestScope_FR139003(t *testing.T) {
	industryMentorUser := "mentor-ind-1"
	p := Placement{
		SchoolMentorUserID: "guru-1",
		StudentID:          "siswa-1",
	}

	// 1. Koordinator has access
	if !Scope("koordinator_pkl", "user-x", p, &industryMentorUser) {
		t.Errorf("expected coordinator access")
	}

	// 2. School mentor has access
	if !Scope("guru", "guru-1", p, &industryMentorUser) {
		t.Errorf("expected school mentor access")
	}

	// 3. Other guru has no access
	if Scope("guru", "guru-other", p, &industryMentorUser) {
		t.Errorf("expected no access for other guru")
	}

	// 4. Assigned industry mentor has access
	if !Scope("pembimbing_industri", "mentor-ind-1", p, &industryMentorUser) {
		t.Errorf("expected industry mentor access")
	}

	// 5. Other industry mentor has NO access (FR-139-003)
	if Scope("pembimbing_industri", "mentor-other", p, &industryMentorUser) {
		t.Errorf("expected no access for other industry mentor")
	}

	// 6. Student has access to own placement
	if !Scope("siswa", "siswa-1", p, &industryMentorUser) {
		t.Errorf("expected student access to own placement")
	}

	// 7. Other student has no access
	if Scope("siswa", "siswa-other", p, &industryMentorUser) {
		t.Errorf("expected no access for other student")
	}
}

func TestBuildCertificatePDF_FR141003(t *testing.T) {
	pdf := BuildCertificatePDF(CertificateDetails{
		CertificateNo: "PKL/2026/0001",
		SchoolName:    "SMK Teratai Putih Global",
		StudentName:   "Budi Pratama",
		StudentNIS:    "2026001",
		MajorName:     "Rekayasa Perangkat Lunak",
		PartnerName:   "PT Teknologi Masa Depan",
		FinalScoreStr: "83,5",
		StartsOn:      parseDate("2026-01-01"),
		EndsOn:        parseDate("2026-06-30"),
		IssuedDate:    parseDate("2026-07-01"),
		IssuedBy:      "koordinator-user",
	})

	if len(pdf) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}
	if string(pdf[:8]) != "%PDF-1.4" {
		t.Errorf("expected header %%PDF-1.4, got %s", string(pdf[:8]))
	}
	if !strings.Contains(string(pdf), "PKL/2026/0001") {
		t.Errorf("expected certificate number in PDF stream")
	}
	if !strings.Contains(string(pdf), "83,5") {
		t.Errorf("expected final score in PDF stream")
	}
}


func TestWorkDayUsesJakarta(t *testing.T) {
	// 2026-10-14 18:30 UTC is 2026-10-15 01:30 WIB.
	got := WorkDay(time.Date(2026, 10, 14, 18, 30, 0, 0, time.UTC))
	if got.Format("2006-01-02") != "2026-10-15" {
		t.Fatalf("WorkDay = %s; want 2026-10-15", got.Format("2006-01-02"))
	}
	got = WorkDay(time.Date(2026, 10, 15, 16, 59, 0, 0, time.UTC))
	if got.Format("2006-01-02") != "2026-10-15" {
		t.Fatalf("WorkDay = %s; want 2026-10-15", got.Format("2006-01-02"))
	}
}
