package activity

import (
	"encoding/json"
	"sort"
	"time"
)

// Status constants
const (
	EventStatusDraft           = "draft"
	EventStatusPendingApproval = "pending_approval"
	EventStatusApproved        = "approved"
	EventStatusRejected        = "rejected"
	EventStatusPublished       = "published"
	EventStatusCancelled       = "cancelled"
	EventStatusCompleted       = "completed"

	RegStatusPendingConsent = "pending_consent"
	RegStatusConfirmed      = "confirmed"
	RegStatusDeclined       = "declined"
	RegStatusCancelled      = "cancelled"

	EccMemberStatusActive     = "active"
	EccMemberStatusWaitlisted = "waitlisted"
	EccMemberStatusLeft       = "left"
	EccMemberStatusRejected   = "rejected"

	EccFullPolicyReject   = "reject"
	EccFullPolicyWaitlist = "waitlist"

	AchStatusPending  = "pending"
	AchStatusVerified = "verified"
	AchStatusRejected = "rejected"
)

type EventTarget struct {
	GradeLevels  []int    `json:"grade_levels,omitempty"`
	ClassroomIDs []string `json:"classroom_ids,omitempty"`
	IncludeStaff bool     `json:"include_staff,omitempty"`
}

type EventKind struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Name      string     `json:"name"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type Event struct {
	ID                    string      `json:"id"`
	SchoolID              string      `json:"school_id"`
	Name                  string      `json:"name"`
	KindID                string      `json:"kind_id"`
	KindName              string      `json:"kind_name,omitempty"`
	Description           *string     `json:"description,omitempty"`
	StartsAt              time.Time   `json:"starts_at"`
	EndsAt                time.Time   `json:"ends_at"`
	RoomID                *string     `json:"room_id,omitempty"`
	BookingID             *string     `json:"booking_id,omitempty"`
	LocationText          *string     `json:"location_text,omitempty"`
	Target                EventTarget `json:"target"`
	Quota                 *int        `json:"quota,omitempty"`
	FeeIDR                int64       `json:"fee_idr"`
	RequiresParentConsent bool        `json:"requires_parent_consent"`
	ConsentText           *string     `json:"consent_text,omitempty"`
	BudgetLineID          *string     `json:"budget_line_id,omitempty"`
	BudgetAmountIDR       *int64      `json:"budget_amount_idr,omitempty"`
	RegistrationOpensAt   *time.Time  `json:"registration_opens_at,omitempty"`
	RegistrationClosesAt  *time.Time  `json:"registration_closes_at,omitempty"`
	Status                string      `json:"status"`
	ApprovalID            *string     `json:"approval_id,omitempty"`
	PublishedAt           *time.Time  `json:"published_at,omitempty"`
	CreatedBy             string      `json:"created_by"`
	DeletedAt             *time.Time  `json:"deleted_at,omitempty"`
	IdempotencyKey        *string     `json:"idempotency_key,omitempty"`
	CreatedAt             time.Time   `json:"created_at"`
	UpdatedAt             time.Time   `json:"updated_at"`

	RegisteredCount int `json:"registered_count,omitempty"`
}

type EventRegistration struct {
	ID                  string     `json:"id"`
	SchoolID            string     `json:"school_id"`
	EventID             string     `json:"event_id"`
	StudentUserID       string     `json:"student_user_id"`
	RegisteredBy        string     `json:"registered_by"`
	RegisteredVia       string     `json:"registered_via"` // 'siswa' | 'orang_tua' | 'event_coordinator'
	Status              string     `json:"status"`         // 'pending_consent' | 'confirmed' | 'declined' | 'cancelled'
	ConsentID           *string    `json:"consent_id,omitempty"`
	ConsentTextSnapshot *string    `json:"consent_text_snapshot,omitempty"`
	BillID              *string    `json:"bill_id,omitempty"`
	AttendedAt          *time.Time `json:"attended_at,omitempty"`
	AttendanceBy        *string    `json:"attendance_by,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`

	StudentName  string `json:"student_name,omitempty"`
	StudentClass string `json:"student_class,omitempty"`
	EventName    string `json:"event_name,omitempty"`
}

type EccPeriod struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	AcademicYearID *string   `json:"academic_year_id,omitempty"`
	Name           string    `json:"name"`
	OpensAt        time.Time `json:"opens_at"`
	ClosesAt       time.Time `json:"closes_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Extracurricular struct {
	ID           string          `json:"id"`
	SchoolID     string          `json:"school_id"`
	Name         string          `json:"name"`
	Description  *string         `json:"description,omitempty"`
	Schedule     json.RawMessage `json:"schedule"` // [{weekday:1,start:"15:00",end:"17:00"}]
	RoomID       *string         `json:"room_id,omitempty"`
	LocationText *string         `json:"location_text,omitempty"`
	Quota        *int            `json:"quota,omitempty"`
	FeeIDR       int64           `json:"fee_idr"`
	FullPolicy   string          `json:"full_policy"` // 'reject' | 'waitlist'
	Active       bool            `json:"active"`
	DeletedAt    *time.Time      `json:"deleted_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`

	ActiveMemberCount     int `json:"active_member_count,omitempty"`
	WaitlistedMemberCount int `json:"waitlisted_member_count,omitempty"`
}

type EccMember struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	EccID         string     `json:"ecc_id"`
	PeriodID      string     `json:"period_id"`
	StudentUserID string     `json:"student_user_id"`
	Status        string     `json:"status"` // 'active' | 'waitlisted' | 'left' | 'rejected'
	WaitlistedAt  *time.Time `json:"waitlisted_at,omitempty"`
	JoinedAt      *time.Time `json:"joined_at,omitempty"`
	LeftAt        *time.Time `json:"left_at,omitempty"`
	BillID        *string    `json:"bill_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	StudentName  string `json:"student_name,omitempty"`
	StudentClass string `json:"student_class,omitempty"`
	EccName      string `json:"ecc_name,omitempty"`
}

type EccSession struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	EccID     string    `json:"ecc_id"`
	HeldOn    string    `json:"held_on"` // YYYY-MM-DD
	Note      *string   `json:"note,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type EccAssessment struct {
	ID          string    `json:"id"`
	SchoolID    string    `json:"school_id"`
	EccID       string    `json:"ecc_id"`
	PeriodID    string    `json:"period_id"`
	MemberID    string    `json:"member_id"`
	Predicate   string    `json:"predicate"`
	Description *string   `json:"description,omitempty"`
	AssessedBy  string    `json:"assessed_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Achievement struct {
	ID            string          `json:"id"`
	SchoolID      string          `json:"school_id"`
	StudentUserID string          `json:"student_user_id"`
	ActivityName  string          `json:"activity_name"`
	Organizer     *string         `json:"organizer,omitempty"`
	Level         string          `json:"level"` // 'sekolah'|'kecamatan'|'kabupaten_kota'|'provinsi'|'nasional'|'internasional'
	RankLabel     string          `json:"rank_label"`
	AchievedOn    string          `json:"achieved_on"` // YYYY-MM-DD
	Evidence      json.RawMessage `json:"evidence"`
	EventID       *string         `json:"event_id,omitempty"`
	EccID         *string         `json:"ecc_id,omitempty"`
	SubmittedBy   string          `json:"submitted_by"`
	SubmittedVia  string          `json:"submitted_via"` // 'orang_tua'|'siswa'|'pembina_ekskul'|'kesiswaan'
	Status        string          `json:"status"`        // 'pending'|'verified'|'rejected'
	VerifiedBy    *string         `json:"verified_by,omitempty"`
	VerifiedAt    *time.Time      `json:"verified_at,omitempty"`
	RejectReason  *string         `json:"reject_reason,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	DeletedAt     *time.Time      `json:"deleted_at,omitempty"`

	StudentName string `json:"student_name,omitempty"`
}

type BudgetCheckResult struct {
	AvailableIDR int64  `json:"available_idr"`
	OverBudget   bool   `json:"over_budget"`
	Policy       string `json:"policy"` // "extra_approval" | "reject"
}

// -------------------------------------------------------------
// Pure Business Functions (§3)
// -------------------------------------------------------------

// NeedsApproval returns true if event incurs fees or draws from a budget line (FR-133-003).
func NeedsApproval(event Event) bool {
	if event.FeeIDR > 0 {
		return true
	}
	if event.BudgetLineID != nil && *event.BudgetLineID != "" {
		return true
	}
	return false
}

// ApprovalFlags computes metadata flags for core approval engine.
func ApprovalFlags(event Event, budgetResult *BudgetCheckResult) map[string]any {
	hasFee := event.FeeIDR > 0
	usesBudget := event.BudgetLineID != nil && *event.BudgetLineID != ""
	overBudget := false
	if budgetResult != nil && budgetResult.OverBudget {
		overBudget = true
	}
	return map[string]any{
		"has_fee":     hasFee,
		"uses_budget": usesBudget,
		"over_budget": overBudget,
	}
}

// InTarget returns true if student matches event targeting rules (FR-134-001).
func InTarget(event Event, studentGrade int, classroomID string) bool {
	// If both grade levels and classrooms are empty, unrestricted target
	if len(event.Target.GradeLevels) == 0 && len(event.Target.ClassroomIDs) == 0 {
		return true
	}

	gradeMatch := false
	if len(event.Target.GradeLevels) > 0 {
		for _, g := range event.Target.GradeLevels {
			if g == studentGrade {
				gradeMatch = true
				break
			}
		}
	} else {
		gradeMatch = true
	}

	classMatch := false
	if len(event.Target.ClassroomIDs) > 0 {
		for _, c := range event.Target.ClassroomIDs {
			if c == classroomID {
				classMatch = true
				break
			}
		}
	} else {
		classMatch = true
	}

	return gradeMatch && classMatch
}

// EventSeatDecision evaluates seat availability based on quota and taken seats.
// taken = confirmed + pending_consent.
func EventSeatDecision(quota *int, taken int) (allowed bool, reason string) {
	if quota == nil {
		return true, ""
	}
	if taken >= *quota {
		return false, "Kuota penuh"
	}
	return true, ""
}

// RegistrationStatus determines initial registration status based on parent consent requirements (BR-EVT-01).
func RegistrationStatus(event Event, via string, consentGiven bool) string {
	if !event.RequiresParentConsent {
		return RegStatusConfirmed
	}
	if via == "orang_tua" && consentGiven {
		return RegStatusConfirmed
	}
	return RegStatusPendingConsent
}

// CanIssueEventBill verifies whether an ad-hoc bill should be generated for event registration (AC-134-01).
func CanIssueEventBill(reg EventRegistration, event Event) (canIssue bool, amount int64) {
	if event.FeeIDR <= 0 {
		return false, 0
	}
	if reg.Status == RegStatusConfirmed {
		return true, event.FeeIDR
	}
	return false, 0
}

// CanMarkAttendance determines if registration attendance can be recorded (AC-134-01, FR-134-004).
func CanMarkAttendance(reg EventRegistration, event Event, today time.Time) (canMark bool, reason string) {
	if reg.Status != RegStatusConfirmed {
		return false, "Hanya peserta dengan status terkonfirmasi yang dapat diabsen"
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.Local
	}

	eventDay := event.StartsAt.In(loc).Format("2006-01-02")
	todayDay := today.In(loc).Format("2006-01-02")

	if eventDay != todayDay {
		return false, "Absensi hanya dapat dilakukan pada tanggal pelaksanaan kegiatan"
	}
	return true, ""
}

// EccJoinDecision determines member status when joining an extracurricular (AC-135-01).
func EccJoinDecision(quota *int, activeCount int, policy string) string {
	if quota == nil {
		return EccMemberStatusActive
	}
	if activeCount >= *quota {
		if policy == EccFullPolicyWaitlist {
			return EccMemberStatusWaitlisted
		}
		return EccMemberStatusRejected
	}
	return EccMemberStatusActive
}

// PromoteWaitlist picks the oldest waitlisted member and marks them active.
func PromoteWaitlist(members []EccMember) (promotedID *string, remaining []EccMember) {
	var waitlisted []EccMember
	for _, m := range members {
		if m.Status == EccMemberStatusWaitlisted {
			waitlisted = append(waitlisted, m)
		}
	}
	if len(waitlisted) == 0 {
		return nil, members
	}

	// Sort by WaitlistedAt asc or CreatedAt asc
	sort.Slice(waitlisted, func(i, j int) bool {
		tI := waitlisted[i].CreatedAt
		if waitlisted[i].WaitlistedAt != nil {
			tI = *waitlisted[i].WaitlistedAt
		}
		tJ := waitlisted[j].CreatedAt
		if waitlisted[j].WaitlistedAt != nil {
			tJ = *waitlisted[j].WaitlistedAt
		}
		return tI.Before(tJ)
	})

	targetID := waitlisted[0].ID
	now := time.Now()
	var updated []EccMember
	for _, m := range members {
		if m.ID == targetID {
			m.Status = EccMemberStatusActive
			m.JoinedAt = &now
			promotedID = &targetID
		}
		updated = append(updated, m)
	}
	return promotedID, updated
}

// InPeriod verifies if current timestamp falls within the extracurricular registration period (FR-135-002).
func InPeriod(period EccPeriod, now time.Time) bool {
	return !now.Before(period.OpensAt) && !now.After(period.ClosesAt)
}

// VisibleAchievement checks if achievement is visible in portals and dashboards (AC-136-01).
func VisibleAchievement(a Achievement) bool {
	return a.Status == AchStatusVerified
}

// CanVerify enforces maker-checker restriction: verifier cannot be the submitter (BR-GEN-09).
func CanVerify(a Achievement, userID string) bool {
	if userID == "" {
		return false
	}
	return a.SubmittedBy != userID
}

type AchievementSummaryResult struct {
	Total          int                  `json:"total"`
	PerLevel       map[string]int       `json:"per_level"`
	PerMonth       map[string]int       `json:"per_month"`
	RecentVerified []AchievementPreview `json:"recent_verified"`
}

type AchievementPreview struct {
	ID           string `json:"id"`
	StudentName  string `json:"student_name"`
	ActivityName string `json:"activity_name"`
	Level        string `json:"level"`
	RankLabel    string `json:"rank_label"`
	AchievedOn   string `json:"achieved_on"`
}

// AchievementSummary generates statistics from verified achievements only (FR-136-003, AC-136-01).
func AchievementSummary(list []Achievement) AchievementSummaryResult {
	res := AchievementSummaryResult{
		Total:          0,
		PerLevel:       make(map[string]int),
		PerMonth:       make(map[string]int),
		RecentVerified: make([]AchievementPreview, 0),
	}

	var verified []Achievement
	for _, a := range list {
		if VisibleAchievement(a) {
			verified = append(verified, a)
		}
	}

	// Sort verified by achieved_on desc
	sort.Slice(verified, func(i, j int) bool {
		return verified[i].AchievedOn > verified[j].AchievedOn
	})

	res.Total = len(verified)
	for _, a := range verified {
		res.PerLevel[a.Level]++
		// parse month from YYYY-MM-DD
		month := "unknown"
		if len(a.AchievedOn) >= 7 {
			month = a.AchievedOn[:7]
		}
		res.PerMonth[month]++
	}

	limit := 5
	if len(verified) < limit {
		limit = len(verified)
	}
	for i := 0; i < limit; i++ {
		a := verified[i]
		res.RecentVerified = append(res.RecentVerified, AchievementPreview{
			ID:           a.ID,
			StudentName:  a.StudentName,
			ActivityName: a.ActivityName,
			Level:        a.Level,
			RankLabel:    a.RankLabel,
			AchievedOn:   a.AchievedOn,
		})
	}
	return res
}
