package library

import (
	"testing"
	"time"
)

func TestAvailableCopies(t *testing.T) {
	// AC-112-01: 3 copies, all 3 on_loan -> 0 available
	copies := []Copy{
		{ID: "c1", Status: "on_loan"},
		{ID: "c2", Status: "on_loan"},
		{ID: "c3", Status: "on_loan"},
	}
	if got := AvailableCopies(copies); got != 0 {
		t.Fatalf("expected 0 available copies, got %d", got)
	}

	copies = append(copies, Copy{ID: "c4", Status: "available"})
	copies = append(copies, Copy{ID: "c5", Status: "on_hold"})
	copies = append(copies, Copy{ID: "c6", Status: "lost"})
	if got := AvailableCopies(copies); got != 1 {
		t.Fatalf("expected 1 available copy, got %d", got)
	}
}

func TestCanBorrow(t *testing.T) {
	cat := MemberCategory{MaxLoans: 3}
	activeMember := Member{Status: "active"}

	// AC-114-01: activeLoans == 3 -> reject
	ok, msg := CanBorrow(activeMember, cat, 3, false, false)
	if ok || msg != "Batas pinjam tercapai" {
		t.Fatalf("expected reject due to max loans, got ok=%v, msg=%s", ok, msg)
	}

	// ActiveLoans < 3 but has overdue -> reject (BR-LIB-01)
	ok, msg = CanBorrow(activeMember, cat, 1, true, false)
	if ok || msg != "Ada pinjaman yang terlambat dikembalikan" {
		t.Fatalf("expected reject due to overdue, got ok=%v, msg=%s", ok, msg)
	}

	// ActiveLoans < 3 but has open fine -> reject
	ok, msg = CanBorrow(activeMember, cat, 1, false, true)
	if ok || msg != "Ada denda perpustakaan yang belum diselesaikan" {
		t.Fatalf("expected reject due to fine, got ok=%v, msg=%s", ok, msg)
	}

	// Inactive member -> reject
	inactiveMember := Member{Status: "inactive"}
	ok, msg = CanBorrow(inactiveMember, cat, 0, false, false)
	if ok || msg != "Status keanggotaan tidak aktif" {
		t.Fatalf("expected reject due to inactive, got ok=%v, msg=%s", ok, msg)
	}

	// Valid borrow
	ok, msg = CanBorrow(activeMember, cat, 2, false, false)
	if !ok || msg != "" {
		t.Fatalf("expected allowed to borrow, got ok=%v, msg=%s", ok, msg)
	}
}

func TestDueDateAndLateDays(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	start := time.Date(2026, 10, 1, 14, 30, 0, 0, loc)

	due := DueDate(start, 7, loc)
	expectedDue := time.Date(2026, 10, 8, 0, 0, 0, 0, loc)
	if !due.Equal(expectedDue) {
		t.Fatalf("expected due date %v, got %v", expectedDue, due)
	}

	// Returned on time (2026-10-07)
	retOnTime := time.Date(2026, 10, 7, 10, 0, 0, 0, loc)
	if late := LateDays(due, retOnTime, loc); late != 0 {
		t.Fatalf("expected 0 late days, got %d", late)
	}

	// Returned exact day (2026-10-08)
	retSameDay := time.Date(2026, 10, 8, 16, 0, 0, 0, loc)
	if late := LateDays(due, retSameDay, loc); late != 0 {
		t.Fatalf("expected 0 late days, got %d", late)
	}

	// Returned 4 days late (2026-10-12)
	retLate := time.Date(2026, 10, 12, 9, 0, 0, 0, loc)
	if late := LateDays(due, retLate, loc); late != 4 {
		t.Fatalf("expected 4 late days, got %d", late)
	}
}

func TestLateFine(t *testing.T) {
	// AC-114-02: 4 days late * 1000 IDR -> 4000 IDR
	fine := LateFine(4, 1000)
	if fine != 4000 {
		t.Fatalf("expected 4000, got %d", fine)
	}

	if LateFine(0, 1000) != 0 {
		t.Fatalf("expected 0 for 0 days")
	}
}

func TestLostFine(t *testing.T) {
	lostFee := int64(50000)
	rateWithLost := &FineRate{LostFeeIDR: &lostFee}
	repPrice := int64(75000)
	title := Title{ReplacementPriceIDR: &repPrice}

	// Rate lost fee takes precedence
	fine, err := LostFine(rateWithLost, title)
	if err != nil || fine != 50000 {
		t.Fatalf("expected 50000 from rate, got %d, err=%v", fine, err)
	}

	// Fallback to title replacement price if rate lost fee is nil
	rateNil := &FineRate{}
	fine, err = LostFine(rateNil, title)
	if err != nil || fine != 75000 {
		t.Fatalf("expected 75000 from title, got %d, err=%v", fine, err)
	}

	// Both nil -> error
	titleNil := Title{}
	_, err = LostFine(rateNil, titleNil)
	if err == nil {
		t.Fatalf("expected error when neither lost fee nor replacement price set")
	}
}

func TestResolveRate(t *testing.T) {
	catID := "cat-student"
	otherCatID := "cat-teacher"

	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	rates := []FineRate{
		{
			ID:               "general",
			LateFeePerDayIDR: 1000,
			EffectiveFrom:    from,
			EffectiveTo:      &to,
		},
		{
			ID:               "cat_student",
			MemberCategoryID: &catID,
			LateFeePerDayIDR: 500,
			EffectiveFrom:    from,
			EffectiveTo:      &to,
		},
	}

	// Category rate matches student
	r, ok := ResolveRate(rates, &catID, now)
	if !ok || r.ID != "cat_student" || r.LateFeePerDayIDR != 500 {
		t.Fatalf("expected cat_student rate, got %+v", r)
	}

	// Other category falls back to general
	r, ok = ResolveRate(rates, &otherCatID, now)
	if !ok || r.ID != "general" || r.LateFeePerDayIDR != 1000 {
		t.Fatalf("expected general rate, got %+v", r)
	}

	// Date outside range
	futureDate := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	_, ok = ResolveRate(rates, &catID, futureDate)
	if ok {
		t.Fatalf("expected no rate matched for future date")
	}
}

func TestSettlement(t *testing.T) {
	// BR-LIB-02: policy='bill' only applies to students
	if s := Settlement("bill", "student"); s != "bill" {
		t.Fatalf("expected 'bill' for student, got %s", s)
	}
	if s := Settlement("bill", "employee"); s != "pay_at_library" {
		t.Fatalf("expected 'pay_at_library' for employee, got %s", s)
	}
	if s := Settlement("pay_at_library", "student"); s != "pay_at_library" {
		t.Fatalf("expected 'pay_at_library', got %s", s)
	}
}

func TestCanRenew(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	today := time.Date(2026, 10, 5, 10, 0, 0, 0, loc)
	cat := MemberCategory{MaxRenewals: 2}

	// RenewCount < max, not late, no reservation -> OK
	loan := Loan{
		RenewCount: 1,
		DueDate:    time.Date(2026, 10, 7, 0, 0, 0, 0, loc),
	}
	ok, msg := CanRenew(loan, cat, today, false, loc)
	if !ok || msg != "" {
		t.Fatalf("expected renew allowed, got ok=%v, msg=%s", ok, msg)
	}

	// Max renewals reached -> reject
	loan.RenewCount = 2
	ok, msg = CanRenew(loan, cat, today, false, loc)
	if ok || msg != "Batas perpanjangan tercapai" {
		t.Fatalf("expected reject max renewals, got ok=%v, msg=%s", ok, msg)
	}

	// Overdue -> reject
	loan.RenewCount = 1
	loan.DueDate = time.Date(2026, 10, 3, 0, 0, 0, 0, loc)
	ok, msg = CanRenew(loan, cat, today, false, loc)
	if ok || msg != "Pinjaman sudah melewati batas jatuh tempo" {
		t.Fatalf("expected reject overdue, got ok=%v, msg=%s", ok, msg)
	}

	// Has reservation waiting -> reject (FR-114-003)
	loan.DueDate = time.Date(2026, 10, 7, 0, 0, 0, 0, loc)
	ok, msg = CanRenew(loan, cat, today, true, loc)
	if ok || msg != "Buku memiliki antrean reservasi yang menunggu" {
		t.Fatalf("expected reject waiting reservation, got ok=%v, msg=%s", ok, msg)
	}
}

func TestCloseMembership(t *testing.T) {
	m := Member{}

	// AC-113-01: student leaves, still 1 active loan -> pending_close + reason
	status, reason := CloseMembership(m, 1, 0)
	if status != "pending_close" || reason == nil || *reason != "Masih meminjam 1 buku" {
		t.Fatalf("expected pending_close with loan reason, got status=%s, reason=%v", status, reason)
	}

	// No loans but 1 open fine -> pending_close
	status, reason = CloseMembership(m, 0, 2)
	if status != "pending_close" || reason == nil || *reason != "Masih memiliki 2 denda belum lunas" {
		t.Fatalf("expected pending_close with fine reason, got status=%s, reason=%v", status, reason)
	}

	// Clean -> inactive
	status, reason = CloseMembership(m, 0, 0)
	if status != "inactive" || reason != nil {
		t.Fatalf("expected inactive with nil reason, got status=%s, reason=%v", status, reason)
	}
}

func TestReminderKind(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	due := time.Date(2026, 10, 10, 0, 0, 0, 0, loc)
	loan := Loan{DueDate: due}

	// 2 days before due date -> due_soon
	twoDaysBefore := time.Date(2026, 10, 8, 11, 0, 0, 0, loc)
	if k := ReminderKind(loan, twoDaysBefore, 2, loc); k != "due_soon" {
		t.Fatalf("expected 'due_soon', got %s", k)
	}

	// 1 day after due date -> overdue
	oneDayAfter := time.Date(2026, 10, 11, 8, 0, 0, 0, loc)
	if k := ReminderKind(loan, oneDayAfter, 2, loc); k != "overdue" {
		t.Fatalf("expected 'overdue', got %s", k)
	}

	// Same day -> none
	sameDay := time.Date(2026, 10, 10, 14, 0, 0, 0, loc)
	if k := ReminderKind(loan, sameDay, 2, loc); k != "none" {
		t.Fatalf("expected 'none', got %s", k)
	}
}

func TestNextReservation(t *testing.T) {
	q := []Reservation{
		{ID: "r1", Status: "cancelled"},
		{ID: "r2", Status: "waiting"},
		{ID: "r3", Status: "waiting"},
	}

	next := NextReservation(q)
	if next == nil || next.ID != "r2" {
		t.Fatalf("expected r2, got %v", next)
	}

	empty := []Reservation{{ID: "r1", Status: "fulfilled"}}
	if NextReservation(empty) != nil {
		t.Fatalf("expected nil for empty queue")
	}
}
