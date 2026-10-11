package model

import (
	"testing"
	"time"
)

func TestWorkingDays(t *testing.T) {
	// 2026-10-01 (Thu) to 2026-10-07 (Wed):
	// Thu 1, Fri 2, Sat 3 (off), Sun 4 (off), Mon 5, Tue 6, Wed 7.
	// Total Mon-Fri = 5 days.
	// If 2026-10-02 is a holiday, total = 4 days.
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)

	holidays := map[string]bool{
		"2026-10-02": true,
	}

	days, err := WorkingDays(start, end, []int16{1, 2, 3, 4, 5}, holidays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if days != 4.0 {
		t.Fatalf("got %.1f days, want 4.0", days)
	}

	// Weekend-spanning range without holidays: Fri 2 .. Mon 5 -> 2 days
	days, _ = WorkingDays(time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), []int16{1, 2, 3, 4, 5}, nil)
	if days != 2 {
		t.Fatalf("weekend span got %.1f, want 2", days)
	}

	// No schedule configured -> error, never a silent Mon-Fri default
	if _, err = WorkingDays(start, end, nil, nil); err == nil {
		t.Fatal("expected error without schedule")
	}

	// Test start > end error
	_, err = WorkingDays(end, start, []int16{1, 2, 3, 4, 5}, holidays)
	if err == nil {
		t.Fatal("expected error when start > end")
	}
}

func TestCanTakeLeave(t *testing.T) {
	// AC-86-01: saldo 2, minta 3 -> ditolak, pesan "Saldo cuti tidak mencukupi (sisa 2 hari)"
	balance := LeaveBalance{
		Entitled: 2,
		Carried:  0,
		Used:     0,
	}

	ok, msg := CanTakeLeave(balance, 3, true)
	if ok {
		t.Fatal("expected leave request to be rejected")
	}
	wantMsg := "Saldo cuti tidak mencukupi (sisa 2 hari)"
	if msg != wantMsg {
		t.Fatalf("got msg %q, want %q", msg, wantMsg)
	}

	// Half-day remaining keeps its fraction in the message
	if _, m := CanTakeLeave(LeaveBalance{Entitled: 1.5}, 2, true); m != "Saldo cuti tidak mencukupi (sisa 1.5 hari)" {
		t.Fatalf("got %q", m)
	}

	// Asking 2 days should pass
	ok, msg = CanTakeLeave(balance, 2, true)
	if !ok || msg != "" {
		t.Fatalf("expected leave request of 2 days to pass, got ok=%v, msg=%s", ok, msg)
	}

	// No quota leave should always pass
	ok, msg = CanTakeLeave(balance, 10, false)
	if !ok {
		t.Fatal("expected leave without quota to pass")
	}
}

func TestApplyLeaveDecision(t *testing.T) {
	// AC-86-02: 12 -> 10, cancel -> 12
	balance := LeaveBalance{
		Entitled: 12,
		Carried:  0,
		Used:     0,
	}

	// Approve 2 days
	ApplyLeaveDecision(&balance, 2, true)
	if balance.Used != 2 || balance.Remaining != 10 {
		t.Fatalf("after approve: used=%.1f remaining=%.1f, want used=2 remaining=10", balance.Used, balance.Remaining)
	}

	// Cancel 2 days
	ApplyLeaveDecision(&balance, 2, false)
	if balance.Used != 0 || balance.Remaining != 12 {
		t.Fatalf("after cancel: used=%.1f remaining=%.1f, want used=0 remaining=12", balance.Used, balance.Remaining)
	}
}

func TestAttendanceStatus(t *testing.T) {
	sched := WorkSchedule{
		StartTime:            "07:30",
		LateToleranceMinutes: 15, // until 07:45
	}
	loc := time.FixedZone("WIB", 7*3600)

	// Exactly on time (07:30) -> hadir
	t1 := time.Date(2026, 10, 1, 7, 30, 0, 0, loc)
	if got := AttendanceStatus(t1, sched, loc); got != "hadir" {
		t.Fatalf("at 07:30 got %s, want hadir", got)
	}

	// Exactly at tolerance boundary (07:45) -> hadir
	t2 := time.Date(2026, 10, 1, 7, 45, 0, 0, loc)
	if got := AttendanceStatus(t2, sched, loc); got != "hadir" {
		t.Fatalf("at 07:45 got %s, want hadir", got)
	}

	// 1 second past tolerance (07:45:01) -> terlambat
	t3 := time.Date(2026, 10, 1, 7, 45, 1, 0, loc)
	if got := AttendanceStatus(t3, sched, loc); got != "terlambat" {
		t.Fatalf("at 07:45:01 got %s, want terlambat", got)
	}
}

func TestMaskEmployee(t *testing.T) {
	// AC-83-04: Kepala Sekolah / Unit -> data sensitif tersamar
	nik := "3201012345678901"
	npwp := "0912345678901234"
	rek := "1234567890"
	salary := int64(7500000)
	emp := Employee{
		ID:                   "e1",
		FullName:             "Sinta Dewi",
		NIK:                  &nik,
		NPWP:                 &npwp,
		BankAccount:          &rek,
		CurrentBaseSalaryIDR: &salary,
	}

	// Kepala sekolah viewing
	masked := MaskEmployee(emp, []string{"kepala_sekolah"}, "user-other")
	if masked.NIK == nil || *masked.NIK != "••••8901" {
		t.Fatalf("masked NIK got %v, want ••••8901", masked.NIK)
	}
	if masked.NPWP == nil || *masked.NPWP != "••••1234" {
		t.Fatalf("masked NPWP got %v, want ••••1234", masked.NPWP)
	}
	if masked.BankAccount == nil || *masked.BankAccount != "••••7890" {
		t.Fatalf("masked BankAccount got %v, want ••••7890", masked.BankAccount)
	}
	if masked.CurrentBaseSalaryIDR != nil {
		t.Fatalf("salary should be masked (nil), got %v", *masked.CurrentBaseSalaryIDR)
	}

	// Superadmin/admin are not in BR-HR-03's full-view list
	if m := MaskEmployee(emp, []string{"admin", "superadmin"}, "user-x"); *m.NIK == nik {
		t.Fatal("admin must see masked data")
	}

	// Owner sees own data in full
	owner := "user-owner"
	emp.UserID = &owner
	if m := MaskEmployee(emp, []string{"pegawai"}, owner); *m.NIK != nik {
		t.Fatal("owner should see own data unmasked")
	}

	// HR Admin viewing -> unmasked
	unmasked := MaskEmployee(emp, []string{"hr_admin"}, "user-hr")
	if *unmasked.NIK != nik || *unmasked.CurrentBaseSalaryIDR != salary {
		t.Fatal("hr_admin should see unmasked data")
	}
}

func TestDuplicateCheck(t *testing.T) {
	bidx1 := "hash-nik-1"
	email1 := "sinta@teratai.sch.id"
	candidates := []Employee{
		{ID: "e1", EmployeeNo: "EMP001", FullName: "Sinta", NIKBidx: &bidx1, Email: &email1},
	}

	// Duplicate NIK
	matches := DuplicateCheck(candidates, bidx1, "other@teratai.sch.id")
	if len(matches) != 1 || matches[0].MatchReason != "NIK sudah terdaftar" {
		t.Fatalf("expected duplicate NIK match, got %+v", matches)
	}

	// Duplicate Email (case insensitive)
	matches = DuplicateCheck(candidates, "other-hash", "SINTA@TERATAI.SCH.ID")
	if len(matches) != 1 || matches[0].MatchReason != "Email sudah terdaftar" {
		t.Fatalf("expected duplicate Email match, got %+v", matches)
	}

	// No duplicate
	matches = DuplicateCheck(candidates, "other-hash", "unique@teratai.sch.id")
	if len(matches) != 0 {
		t.Fatalf("expected no duplicates, got %+v", matches)
	}
}

func TestDocumentCompleteness(t *testing.T) {
	// AC-85-02: pegawai kontrak wajib memiliki KTP, ijazah, dan kontrak, tetapi kontrak belum diunggah -> belum lengkap: kontrak
	today := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	required := []RequiredDocument{
		{DocumentTypeID: "t-ktp", DocumentTypeCode: "KTP", DocumentTypeName: "KTP"},
		{DocumentTypeID: "t-ijazah", DocumentTypeCode: "IJAZAH", DocumentTypeName: "Ijazah"},
		{DocumentTypeID: "t-kontrak", DocumentTypeCode: "KONTRAK", DocumentTypeName: "Kontrak"},
	}

	docs := []EmployeeDocument{
		{DocumentTypeID: "t-ktp", Version: 1},
		{DocumentTypeID: "t-ijazah", Version: 1},
	}

	complete, missing := DocumentCompleteness(required, docs, today)
	if complete {
		t.Fatal("expected completeness to be false")
	}
	if len(missing) != 1 || missing[0] != "kontrak" {
		t.Fatalf("expected missing: [kontrak], got %v", missing)
	}

	// With expired contract
	exp := "2026-09-01"
	docs = append(docs, EmployeeDocument{DocumentTypeID: "t-kontrak", Version: 1, ExpiresOn: &exp})
	complete, missing = DocumentCompleteness(required, docs, today)
	if complete || len(missing) != 1 || missing[0] != "kontrak (kedaluwarsa)" {
		t.Fatalf("expected expired contract notice, got %v", missing)
	}
}

func TestAccrue(t *testing.T) {
	// 12 days annual quota, 5 months worked -> 5.0
	accrued := Accrue(12, 5)
	if accrued != 5.0 {
		t.Fatalf("Accrue(12, 5) = %.1f, want 5.0", accrued)
	}

	// 15 days annual quota, 5 months worked: 15/12 * 5 = 6.25 -> rounded down to 6.0
	accrued = Accrue(15, 5)
	if accrued != 6.0 {
		t.Fatalf("Accrue(15, 5) = %.1f, want 6.0", accrued)
	}

	// 0 months worked
	if Accrue(12, 0) != 0 {
		t.Fatal("expected 0 for 0 months")
	}
}

func TestCarryOver(t *testing.T) {
	if CarryOver(8, 5) != 5 {
		t.Fatalf("CarryOver(8, 5) want 5, got %.1f", CarryOver(8, 5))
	}
	if CarryOver(3, 5) != 3 {
		t.Fatalf("CarryOver(3, 5) want 3, got %.1f", CarryOver(3, 5))
	}
	if CarryOver(-1, 5) != 0 {
		t.Fatalf("CarryOver(-1, 5) want 0, got %.1f", CarryOver(-1, 5))
	}
}

func TestDistanceM(t *testing.T) {
	officeLat, officeLng := -6.2251, 106.8097
	dSame := DistanceM(officeLat, officeLng, officeLat, officeLng)
	if dSame > 1.0 {
		t.Fatalf("same point distance should be ~0m, got %.2fm", dSame)
	}

	// ~150m away
	dNear := DistanceM(officeLat, officeLng, -6.2265, 106.8097)
	if dNear > 300.0 {
		t.Fatalf("expected < 300m, got %.2fm", dNear)
	}

	// Bandung (~120km away)
	dFar := DistanceM(officeLat, officeLng, -6.9219, 107.61)
	if dFar < 100000.0 {
		t.Fatalf("expected > 100km, got %.2fm", dFar)
	}
}
