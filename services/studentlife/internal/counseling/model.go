package counseling

import (
	"errors"
	"strings"
	"time"
)

// Status constants
const (
	CaseStatusOpen       = "open"
	CaseStatusMonitoring = "monitoring"
	CaseStatusClosed     = "closed"

	SessionKindIndividual = "individual"
	SessionKindGroup      = "group"

	SessionStatusScheduled = "scheduled"
	SessionStatusHeld      = "held"
	SessionStatusCancelled = "cancelled"
	SessionStatusNoShow    = "no_show"

	ReferralTargetInternal = "internal"
	ReferralTargetExternal = "external"

	ReferralStatusAwaitingConsent = "awaiting_consent"
	ReferralStatusActive          = "active"
	ReferralStatusCompleted       = "completed"
	ReferralStatusDeclined        = "declined"
	ReferralStatusCancelled       = "cancelled"

	HomeroomStatusInCare = "dalam_pendampingan"
	HomeroomStatusNoInfo = "tidak_ada_informasi"

	TaskStatusDone    = "selesai"
	TaskStatusLate    = "terlambat"
	TaskStatusPending = "belum"

	ConsentSubjectReferral = "referral"
)

// Category models counseling_categories row.
type Category struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Name      string     `json:"name"`
	Active    bool       `json:"active"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Case models counseling_cases row.
type Case struct {
	ID                 string     `json:"id"`
	SchoolID           string     `json:"school_id"`
	StudentID          string     `json:"student_id"`
	StudentName        string     `json:"student_name,omitempty"`
	StudentNISN        string     `json:"student_nisn,omitempty"`
	StudentClassroom   string     `json:"student_classroom,omitempty"`
	CategoryID         string     `json:"category_id"`
	CategoryName       string     `json:"category_name,omitempty"`
	OpenedBy           string     `json:"opened_by"`
	OpenedByName       string     `json:"opened_by_name,omitempty"`
	Status             string     `json:"status"`
	DiscloseToHomeroom bool       `json:"disclose_to_homeroom"`
	OpenedAt           time.Time  `json:"opened_at"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
	CloseReasonEnc     []byte     `json:"-"`
	CloseReason        string     `json:"close_reason,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	Members            []Member   `json:"members,omitempty"`
}

// Member models counseling_case_members row.
type Member struct {
	CaseID     string     `json:"case_id"`
	UserID     string     `json:"user_id"`
	UserName   string     `json:"user_name,omitempty"`
	MemberRole string     `json:"member_role"`
	AddedBy    string     `json:"added_by"`
	AddedAt    time.Time  `json:"added_at"`
	RemovedAt  *time.Time `json:"removed_at,omitempty"`
}

// Session models counseling_sessions row.
type Session struct {
	ID                 string     `json:"id"`
	SchoolID           string     `json:"school_id"`
	CaseID             *string    `json:"case_id,omitempty"`
	Kind               string     `json:"kind"`
	CategoryID         string     `json:"category_id"`
	CategoryName       string     `json:"category_name,omitempty"`
	CounselorUserID    string     `json:"counselor_user_id"`
	CounselorName      string     `json:"counselor_name,omitempty"`
	ScheduledAt        time.Time  `json:"scheduled_at"`
	HeldAt             *time.Time `json:"held_at,omitempty"`
	Status             string     `json:"status"`
	NotesEnc           []byte     `json:"-"`
	Notes              string     `json:"notes,omitempty"`
	FollowUpPlanEnc    []byte     `json:"-"`
	FollowUpPlan       string     `json:"follow_up_plan,omitempty"`
	ReminderSentAt     *time.Time `json:"reminder_sent_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	StudentIDs         []string   `json:"student_ids,omitempty"`
	StudentNames       []string   `json:"student_names,omitempty"`
}

// Referral models referrals row.
type Referral struct {
	ID                string     `json:"id"`
	SchoolID          string     `json:"school_id"`
	CaseID            string     `json:"case_id"`
	StudentID         string     `json:"student_id"`
	StudentName       string     `json:"student_name,omitempty"`
	TargetKind        string     `json:"target_kind"`
	TargetNameEnc     []byte     `json:"-"`
	TargetName        string     `json:"target_name,omitempty"`
	TargetContactEnc  []byte     `json:"-"`
	TargetContact     string     `json:"target_contact,omitempty"`
	ReasonEnc         []byte     `json:"-"`
	Reason            string     `json:"reason,omitempty"`
	ParentSummaryEnc  []byte     `json:"-"`
	ParentSummary     string     `json:"parent_summary,omitempty"`
	Status            string     `json:"status"`
	ConsentID         *string    `json:"consent_id,omitempty"`
	ConsentStatus     string     `json:"consent_status,omitempty"`
	OutcomeEnc        []byte     `json:"-"`
	Outcome           string     `json:"outcome,omitempty"`
	CreatedBy         string     `json:"created_by"`
	CreatedByName     string     `json:"created_by_name,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Task models case_tasks row.
type Task struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	CaseID         string     `json:"case_id"`
	Title          string     `json:"title"`
	AssigneeUserID string     `json:"assignee_user_id"`
	AssigneeName   string     `json:"assignee_name,omitempty"`
	DueDate        string     `json:"due_date"`
	DoneAt         *time.Time `json:"done_at,omitempty"`
	DoneBy         *string    `json:"done_by,omitempty"`
	NoteEnc        []byte     `json:"-"`
	Note           string     `json:"note,omitempty"`
	DisplayStatus  string     `json:"display_status,omitempty"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	StudentName    string     `json:"student_name,omitempty"`
}

// Settings models counseling_settings row.
type Settings struct {
	SchoolID           string    `json:"school_id"`
	ReminderDaysBefore int       `json:"reminder_days_before"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// HomeroomStatus returns whether student is in counseling care as visible to homeroom (AC-130-01).
// Returns "dalam_pendampingan" if any active case (open/monitoring) has disclose_to_homeroom=true.
// Otherwise returns "tidak_ada_informasi" (never leaks case existence if not disclosed).
func HomeroomStatus(cases []Case) string {
	for _, c := range cases {
		if (c.Status == CaseStatusOpen || c.Status == CaseStatusMonitoring) && c.DiscloseToHomeroom {
			return HomeroomStatusInCare
		}
	}
	return HomeroomStatusNoInfo
}

// ValidateSession validates session parameters (FR-130-001).
func ValidateSession(kind string, studentIDs []string, status string, heldAt *time.Time) error {
	switch kind {
	case SessionKindGroup:
		if len(studentIDs) < 2 {
			return errors.New("sesi kelompok membutuhkan minimal 2 siswa")
		}
	case SessionKindIndividual:
		if len(studentIDs) != 1 {
			return errors.New("sesi individual harus tepat 1 siswa")
		}
	default:
		return errors.New("jenis sesi tidak valid")
	}

	if status == SessionStatusHeld && heldAt == nil {
		return errors.New("sesi yang sudah terlaksana wajib memiliki waktu pelaksanaan (held_at)")
	}
	return nil
}

// ReminderDue checks if a session reminder notification is due (FR-130-003).
// If reminderDaysBefore == 0, due on the day of the session.
// If reminderDaysBefore > 0, due H-days before the session.
func ReminderDue(scheduledAt time.Time, reminderDaysBefore int, reminderSentAt *time.Time, status string, today time.Time, loc *time.Location) bool {
	if status != SessionStatusScheduled || reminderSentAt != nil {
		return false
	}
	if loc == nil {
		loc = time.UTC
	}
	sTime := scheduledAt.In(loc)
	tDate := today.In(loc)

	scheduledDate := time.Date(sTime.Year(), sTime.Month(), sTime.Day(), 0, 0, 0, 0, loc)
	currentDate := time.Date(tDate.Year(), tDate.Month(), tDate.Day(), 0, 0, 0, 0, loc)

	targetDate := scheduledDate.AddDate(0, 0, -reminderDaysBefore)
	return targetDate.Equal(currentDate)
}

// ReferralInitialStatus determines initial status of a referral based on consent (AC-132-01).
func ReferralInitialStatus(consentStatus string) string {
	switch consentStatus {
	case "granted":
		return ReferralStatusActive
	case "declined":
		return ReferralStatusDeclined
	default:
		return ReferralStatusAwaitingConsent
	}
}

// CanClose checks if a case can be closed (FR-132-003).
// Can close if all case tasks are done, OR an explicit close reason is provided.
func CanClose(tasks []Task, closeReason string) bool {
	if strings.TrimSpace(closeReason) != "" {
		return true
	}
	for _, t := range tasks {
		if t.DoneAt == nil {
			return false
		}
	}
	return true
}

// TaskDisplayStatus computes task display status (FR-132-002).
func TaskDisplayStatus(t Task, todayDate string) string {
	if t.DoneAt != nil {
		return TaskStatusDone
	}
	if t.DueDate != "" && t.DueDate < todayDate {
		return TaskStatusLate
	}
	return TaskStatusPending
}
