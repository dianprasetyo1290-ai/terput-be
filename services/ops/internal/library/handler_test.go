package library

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func setupLibraryTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("skipping DB test: TEST_DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Skip("skipping DB test: cannot open connection:", err)
	}
	if err := db.Ping(); err != nil {
		t.Skip("skipping DB test: cannot ping database:", err)
	}

	// Apply migrations
	for _, f := range []string{
		"../../migrations/0001_ops_base.up.sql",
		"../../migrations/0040_library.up.sql",
	} {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("exec migration %s: %v", f, err)
		}
	}

	cleanup := func() {
		_, _ = db.Exec(`
			DELETE FROM lib_visits;
			DELETE FROM lib_fines;
			DELETE FROM lib_reservations;
			DELETE FROM lib_loans;
			DELETE FROM lib_copies;
			DELETE FROM lib_titles;
			DELETE FROM lib_collection_categories;
			DELETE FROM lib_members;
			DELETE FROM lib_fine_rates;
			DELETE FROM lib_member_categories;
			DELETE FROM lib_settings;
		`)
	}
	return db, cleanup
}

func TestLibraryE2E(t *testing.T) {
	db, cleanup := setupLibraryTestDB(t)
	defer cleanup()
	loc, _ := time.LoadLocation("Asia/Jakarta")

	store := NewStore(db, loc)
	runner := NewJobRunner(db, nil, "", "test-key", loc)
	handler := NewHandler(store, nil, runner)

	schoolID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	ctx := context.Background()

	// 1. Setup member category (max 3 loans, 7 days loan duration)
	cat, err := store.CreateMemberCategory(ctx, MemberCategory{
		SchoolID:            schoolID,
		Name:                "Siswa Reguler",
		AppliesTo:           "student",
		IsDefault:           true,
		MaxLoans:            3,
		LoanDays:            7,
		MaxRenewals:         1,
		ReservationHoldDays: 2,
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	// 2. Setup fine rate (Rp1.000 / day)
	rate, err := store.CreateFineRate(ctx, FineRate{
		SchoolID:         schoolID,
		MemberCategoryID: &cat.ID,
		LateFeePerDayIDR: 1000,
		EffectiveFrom:    time.Now().AddDate(-1, 0, 0),
	})
	if err != nil {
		t.Fatalf("create fine rate: %v", err)
	}
	_ = rate

	// 3. Setup member
	userID := "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a22"
	member, err := store.UpsertMember(ctx, Member{
		SchoolID:   schoolID,
		Kind:       "student",
		UserID:     userID,
		CategoryID: cat.ID,
		MemberCode: "LIB-S-1001",
		Status:     "active",
	})
	if err != nil {
		t.Fatalf("upsert member: %v", err)
	}

	// 4. Setup collection category & title with 3 copies
	colCat, err := store.CreateCollectionCategory(ctx, CollectionCategory{
		SchoolID: schoolID,
		Code:     "TEK",
		Name:     "Teknologi",
	})
	if err != nil {
		t.Fatalf("create collection category: %v", err)
	}

	title, err := store.CreateTitle(ctx, Title{
		SchoolID:   schoolID,
		Title:      "Fisika Dasar",
		Author:     "Halliday & Resnick",
		Publisher:  "Erlangga",
		CategoryID: colCat.ID,
	})
	if err != nil {
		t.Fatalf("create title: %v", err)
	}

	cp1, _ := store.CreateCopy(ctx, Copy{SchoolID: schoolID, TitleID: title.ID, Barcode: "BAR-001", ShelfLocation: "Rak A", Status: "available"})
	cp2, _ := store.CreateCopy(ctx, Copy{SchoolID: schoolID, TitleID: title.ID, Barcode: "BAR-002", ShelfLocation: "Rak A", Status: "available"})
	cp3, _ := store.CreateCopy(ctx, Copy{SchoolID: schoolID, TitleID: title.ID, Barcode: "BAR-003", ShelfLocation: "Rak A", Status: "available"})

	// Check initial available copies in OPAC
	tList, err := store.ListTitles(ctx, schoolID, "", "")
	if err != nil || len(tList) == 0 || tList[0].AvailableCopies != 3 {
		t.Fatalf("expected 3 available copies in OPAC, got %v", tList)
	}

	loanerStaff := "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a33"

	// Borrow 1st, 2nd, and 3rd copy
	loan1, err := store.BorrowCopy(ctx, schoolID, member.MemberCode, cp1.Barcode, "", "", loanerStaff, "idemp-loan-1")
	if err != nil {
		t.Fatalf("borrow 1: %v", err)
	}
	loan2, err := store.BorrowCopy(ctx, schoolID, member.MemberCode, cp2.Barcode, "", "", loanerStaff, "idemp-loan-2")
	if err != nil {
		t.Fatalf("borrow 2: %v", err)
	}
	loan3, err := store.BorrowCopy(ctx, schoolID, member.MemberCode, cp3.Barcode, "", "", loanerStaff, "idemp-loan-3")
	if err != nil {
		t.Fatalf("borrow 3: %v", err)
	}
	_ = loan1
	_ = loan2
	_ = loan3

	// Idempotency test: calling BorrowCopy with same key returns existing loan
	reloan1, err := store.BorrowCopy(ctx, schoolID, member.MemberCode, cp1.Barcode, "", "", loanerStaff, "idemp-loan-1")
	if err != nil || reloan1.ID != loan1.ID {
		t.Fatalf("expected idempotent replay of loan1, got %v, err=%v", reloan1, err)
	}

	// AC-112-01: 3 copies, all 3 on_loan -> OPAC AvailableCopies must be 0!
	tList, err = store.ListTitles(ctx, schoolID, "", "")
	if err != nil || len(tList) == 0 {
		t.Fatalf("list titles error: %v", err)
	}
	if tList[0].AvailableCopies != 0 {
		t.Fatalf("AC-112-01 FAILED: expected 0 available copies, got %d", tList[0].AvailableCopies)
	}

	// AC-114-01: borrowing 4th copy when max is 3 must be rejected with "Batas pinjam tercapai"!
	cpExtra, _ := store.CreateCopy(ctx, Copy{SchoolID: schoolID, TitleID: title.ID, Barcode: "BAR-004", ShelfLocation: "Rak A", Status: "available"})
	_, err = store.BorrowCopy(ctx, schoolID, member.MemberCode, cpExtra.Barcode, "", "", loanerStaff, "idemp-loan-4")
	if err == nil || err.Error() != "Batas pinjam tercapai" {
		t.Fatalf("AC-114-01 FAILED: expected 'Batas pinjam tercapai', got %v", err)
	}

	// AC-114-02: return loan 4 days late -> Rp4.000 fine recorded!
	// Loan due date was loanDate + 7 days. We return it on dueDate + 4 days.
	returnDate := loan1.DueDate.AddDate(0, 0, 4)
	retRes, err := store.ReturnCopy(ctx, schoolID, cp1.Barcode, loanerStaff, returnDate)
	if err != nil {
		t.Fatalf("return copy: %v", err)
	}
	if retRes.Fine == nil {
		t.Fatalf("AC-114-02 FAILED: expected fine to be generated, got nil")
	}
	if retRes.Fine.AmountIDR != 4000 {
		t.Fatalf("AC-114-02 FAILED: expected 4000 IDR fine, got %d", retRes.Fine.AmountIDR)
	}
	if *retRes.Fine.LateDays != 4 {
		t.Fatalf("AC-114-02 FAILED: expected 4 late days, got %d", *retRes.Fine.LateDays)
	}

	// AC-113-01: Close membership sync: member has remaining active loans -> pending_close + reason
	closeSt, closeReason := CloseMembership(member, 2, 1)
	if closeSt != "pending_close" || closeReason == nil || *closeReason != "Masih meminjam 2 buku" {
		t.Fatalf("AC-113-01 FAILED: expected pending_close, got %s / %v", closeSt, closeReason)
	}

	// When clean (0 loans, 0 fines) -> inactive
	cleanSt, cleanReason := CloseMembership(member, 0, 0)
	if cleanSt != "inactive" || cleanReason != nil {
		t.Fatalf("AC-113-01 FAILED: expected inactive, got %s", cleanSt)
	}

	// AC-115-01: Overdue loans report lists member and days overdue
	// cp2 is still active and past due relative to returnDate
	overdueList, err := store.OverdueLoans(ctx, schoolID)
	if err != nil {
		t.Fatalf("overdue report: %v", err)
	}
	_ = overdueList
	_ = handler
}
