package library

import (
	"errors"
	"fmt"
	"time"
)

// --- Domain Models ---

type Settings struct {
	SchoolID           string    `json:"school_id"`
	FinePolicy         string    `json:"fine_policy"` // 'pay_at_library' | 'bill'
	ReminderDaysBefore int       `json:"reminder_days_before"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type MemberCategory struct {
	ID                  string     `json:"id"`
	SchoolID            string     `json:"school_id"`
	Name                string     `json:"name"`
	AppliesTo           string     `json:"applies_to"` // 'student' | 'employee'
	IsDefault           bool       `json:"is_default"`
	MaxLoans            int        `json:"max_loans"`
	LoanDays            int        `json:"loan_days"`
	MaxRenewals         int        `json:"max_renewals"`
	ReservationHoldDays int        `json:"reservation_hold_days"`
	DeletedAt           *time.Time `json:"deleted_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type FineRate struct {
	ID                string     `json:"id"`
	SchoolID          string     `json:"school_id"`
	MemberCategoryID  *string    `json:"member_category_id,omitempty"`
	LateFeePerDayIDR  int64      `json:"late_fee_per_day_idr"`
	LostFeeIDR        *int64     `json:"lost_fee_idr,omitempty"`
	EffectiveFrom     time.Time  `json:"effective_from"`
	EffectiveTo       *time.Time `json:"effective_to,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Member struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	Kind          string     `json:"kind"` // 'student' | 'employee'
	UserID        string     `json:"user_id"`
	CategoryID    string     `json:"category_id"`
	CategoryName  string     `json:"category_name,omitempty"`
	MemberCode    string     `json:"member_code"`
	Status        string     `json:"status"` // 'active' | 'pending_close' | 'inactive'
	PendingReason *string    `json:"pending_reason,omitempty"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
	SyncedAt      time.Time  `json:"synced_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Enriched fields
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
}

type CollectionCategory struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type Title struct {
	ID                  string     `json:"id"`
	SchoolID            string     `json:"school_id"`
	Title               string     `json:"title"`
	Author              string     `json:"author"`
	Publisher           string     `json:"publisher"`
	ISBN                *string    `json:"isbn,omitempty"`
	PublishYear         *int       `json:"publish_year,omitempty"`
	CategoryID          string     `json:"category_id"`
	CategoryName        string     `json:"category_name,omitempty"`
	ReplacementPriceIDR *int64     `json:"replacement_price_idr,omitempty"`
	DeletedAt           *time.Time `json:"deleted_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`

	// OPAC / Summary
	AvailableCopies int     `json:"available_copies,omitempty"`
	TotalCopies     int     `json:"total_copies,omitempty"`
	Copies          []Copy  `json:"copies,omitempty"`
}

type Copy struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	TitleID       string     `json:"title_id"`
	Barcode       string     `json:"barcode"`
	ShelfLocation string     `json:"shelf_location"`
	Status        string     `json:"status"` // 'available','on_loan','on_hold','lost','damaged','withdrawn'
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	TitleName string `json:"title_name,omitempty"`
}

type Loan struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	CopyID         string     `json:"copy_id"`
	MemberID       string     `json:"member_id"`
	ClassroomID    *string    `json:"classroom_id,omitempty"`
	ClassroomName  *string    `json:"classroom_name,omitempty"`
	LoanedAt       time.Time  `json:"loaned_at"`
	DueDate        time.Time  `json:"due_date"`
	ReturnedAt     *time.Time `json:"returned_at,omitempty"`
	RenewCount     int        `json:"renew_count"`
	Status         string     `json:"status"` // 'active' | 'returned' | 'lost'
	LoanedBy       string     `json:"loaned_by"`
	ReturnedBy     *string    `json:"returned_by,omitempty"`
	IdempotencyKey string     `json:"idempotency_key"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	// Joined fields
	TitleID     string `json:"title_id,omitempty"`
	TitleName   string `json:"title_name,omitempty"`
	CopyBarcode string `json:"copy_barcode,omitempty"`
	MemberCode  string `json:"member_code,omitempty"`
	MemberName  string `json:"member_name,omitempty"`
}

type Reservation struct {
	ID         string     `json:"id"`
	SchoolID   string     `json:"school_id"`
	TitleID    string     `json:"title_id"`
	MemberID   string     `json:"member_id"`
	Status     string     `json:"status"` // 'waiting','ready','fulfilled','cancelled','expired'
	HeldCopyID *string    `json:"held_copy_id,omitempty"`
	ReadyUntil *time.Time `json:"ready_until,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`

	TitleName  string `json:"title_name,omitempty"`
	MemberCode string `json:"member_code,omitempty"`
	MemberName string `json:"member_name,omitempty"`
}

type Fine struct {
	ID           string     `json:"id"`
	SchoolID     string     `json:"school_id"`
	LoanID       string     `json:"loan_id"`
	MemberID     string     `json:"member_id"`
	Kind         string     `json:"kind"` // 'late' | 'lost'
	LateDays     *int       `json:"late_days,omitempty"`
	AmountIDR    int64      `json:"amount_idr"`
	RateSnapshot any        `json:"rate_snapshot"`
	Settlement   string     `json:"settlement"` // 'pay_at_library' | 'bill'
	Status       string     `json:"status"`     // 'open','paid','billed','waived','void'
	BillID       *string    `json:"bill_id,omitempty"`
	PaidAt       *time.Time `json:"paid_at,omitempty"`
	ReceivedBy   *string    `json:"received_by,omitempty"`
	WaivedBy     *string    `json:"waived_by,omitempty"`
	WaiveReason  *string    `json:"waive_reason,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	// Joined
	MemberCode string `json:"member_code,omitempty"`
	MemberName string `json:"member_name,omitempty"`
	TitleName  string `json:"title_name,omitempty"`
}

type Visit struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	MemberID  string    `json:"member_id"`
	VisitedOn time.Time `json:"visited_on"`
	VisitedAt time.Time `json:"visited_at"`

	MemberCode string `json:"member_code,omitempty"`
	MemberName string `json:"member_name,omitempty"`
}

// --- Pure Calculation Functions (Section 3) ---

// AvailableCopies calculates number of available copies (AC-112-01).
// 'on_loan', 'on_hold', 'lost', 'damaged', 'withdrawn' are not available.
func AvailableCopies(copies []Copy) int {
	count := 0
	for _, c := range copies {
		if c.DeletedAt == nil && c.Status == "available" {
			count++
		}
	}
	return count
}

// CanBorrow checks if a member can borrow another copy (AC-114-01).
func CanBorrow(member Member, category MemberCategory, activeLoans int, hasOverdue bool, hasOpenLibraryFine bool) (bool, string) {
	if member.Status == "pending_close" || member.Status == "inactive" {
		return false, "Status keanggotaan tidak aktif"
	}
	if hasOverdue {
		return false, "Ada pinjaman yang terlambat dikembalikan"
	}
	if hasOpenLibraryFine {
		return false, "Ada denda perpustakaan yang belum diselesaikan"
	}
	if activeLoans >= category.MaxLoans {
		return false, "Batas pinjam tercapai"
	}
	return true, ""
}

// DueDate calculates due date based on loanDate and loanDays.
func DueDate(loanDate time.Time, loanDays int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	inLoc := loanDate.In(loc)
	year, month, day := inLoc.Date()
	midnight := time.Date(year, month, day, 0, 0, 0, 0, loc)
	return midnight.AddDate(0, 0, loanDays)
}

// LateDays calculates the number of calendar days a return is late.
func LateDays(dueDate, returnedOn time.Time, loc *time.Location) int {
	if loc == nil {
		loc = time.UTC
	}
	dYear, dMonth, dDay := dueDate.In(loc).Date()
	dMidnight := time.Date(dYear, dMonth, dDay, 0, 0, 0, 0, loc)

	rYear, rMonth, rDay := returnedOn.In(loc).Date()
	rMidnight := time.Date(rYear, rMonth, rDay, 0, 0, 0, 0, loc)

	diff := rMidnight.Sub(dMidnight)
	days := int(diff.Hours() / 24)
	if days <= 0 {
		return 0
	}
	return days
}

// LateFine calculates fine for late return (AC-114-02).
func LateFine(days int, ratePerDay int64) int64 {
	if days <= 0 || ratePerDay <= 0 {
		return 0
	}
	return int64(days) * ratePerDay
}

// LostFine calculates fine for lost copy.
func LostFine(rate *FineRate, title Title) (int64, error) {
	if rate != nil && rate.LostFeeIDR != nil && *rate.LostFeeIDR > 0 {
		return *rate.LostFeeIDR, nil
	}
	if title.ReplacementPriceIDR != nil && *title.ReplacementPriceIDR > 0 {
		return *title.ReplacementPriceIDR, nil
	}
	return 0, errors.New("Tarif kehilangan belum diatur")
}

// ResolveRate resolves applicable fine rate on a given date.
// Category-specific rate overrides general rate.
func ResolveRate(rates []FineRate, memberCategoryID *string, onDate time.Time) (FineRate, bool) {
	var generalRate *FineRate
	var categoryRate *FineRate

	onDateNorm := onDate.Truncate(24 * time.Hour)

	for i := range rates {
		r := &rates[i]
		fromNorm := r.EffectiveFrom.Truncate(24 * time.Hour)
		if onDateNorm.Before(fromNorm) {
			continue
		}
		if r.EffectiveTo != nil {
			toNorm := r.EffectiveTo.Truncate(24 * time.Hour)
			if onDateNorm.After(toNorm) {
				continue
			}
		}

		if r.MemberCategoryID != nil && memberCategoryID != nil && *r.MemberCategoryID == *memberCategoryID {
			categoryRate = r
		} else if r.MemberCategoryID == nil {
			generalRate = r
		}
	}

	if categoryRate != nil {
		return *categoryRate, true
	}
	if generalRate != nil {
		return *generalRate, true
	}
	return FineRate{}, false
}

// Settlement determines settlement method ('bill' or 'pay_at_library') (BR-LIB-02).
// 'bill' is only allowed for 'student'. Employee members are always 'pay_at_library'.
func Settlement(policy, memberKind string) string {
	if policy == "bill" && memberKind == "student" {
		return "bill"
	}
	return "pay_at_library"
}

// CanRenew checks if a loan can be renewed (FR-114-003).
func CanRenew(loan Loan, category MemberCategory, today time.Time, titleHasWaitingReservation bool, loc *time.Location) (bool, string) {
	if loan.RenewCount >= category.MaxRenewals {
		return false, "Batas perpanjangan tercapai"
	}
	if LateDays(loan.DueDate, today, loc) > 0 {
		return false, "Pinjaman sudah melewati batas jatuh tempo"
	}
	if titleHasWaitingReservation {
		return false, "Buku memiliki antrean reservasi yang menunggu"
	}
	return true, ""
}

// CloseMembership determines whether a membership can be set to 'inactive' or must be 'pending_close' (AC-113-01).
func CloseMembership(member Member, activeLoans int, openFines int) (string, *string) {
	if activeLoans > 0 {
		reason := fmt.Sprintf("Masih meminjam %d buku", activeLoans)
		return "pending_close", &reason
	}
	if openFines > 0 {
		reason := fmt.Sprintf("Masih memiliki %d denda belum lunas", openFines)
		return "pending_close", &reason
	}
	return "inactive", nil
}

// ReminderKind determines if a loan reminder should be triggered.
func ReminderKind(loan Loan, today time.Time, daysBefore int, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	dYear, dMonth, dDay := loan.DueDate.In(loc).Date()
	dueMidnight := time.Date(dYear, dMonth, dDay, 0, 0, 0, 0, loc)

	tYear, tMonth, tDay := today.In(loc).Date()
	todayMidnight := time.Date(tYear, tMonth, tDay, 0, 0, 0, 0, loc)

	if todayMidnight.AddDate(0, 0, daysBefore).Equal(dueMidnight) {
		return "due_soon"
	}

	// First day of overdue: today == due + 1 day
	if todayMidnight.Equal(dueMidnight.AddDate(0, 0, 1)) {
		return "overdue"
	}

	return "none"
}

// NextReservation gets the oldest waiting reservation FIFO.
func NextReservation(queue []Reservation) *Reservation {
	for i := range queue {
		if queue[i].Status == "waiting" {
			return &queue[i]
		}
	}
	return nil
}
