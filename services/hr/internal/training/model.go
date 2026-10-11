package training

import (
	"encoding/json"
	"time"
)

type Category struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Competency struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Course struct {
	ID              string       `json:"id"`
	SchoolID        string       `json:"school_id"`
	Title           string       `json:"title"`
	Objective       string       `json:"objective"`
	DurationMinutes int          `json:"duration_minutes"`
	Provider        string       `json:"provider"`
	Kind            string       `json:"kind"` // online, offline
	CategoryID      *string      `json:"category_id,omitempty"`
	CategoryName    string       `json:"category_name,omitempty"`
	CostIDR         int64        `json:"cost_idr"`
	Quota           *int         `json:"quota,omitempty"`
	PassingScore    *int         `json:"passing_score,omitempty"`
	Status          string       `json:"status"` // draft, published, archived
	Competencies    []Competency `json:"competencies,omitempty"`
	MaterialsCount  int          `json:"materials_count,omitempty"`
	SessionsCount   int          `json:"sessions_count,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

type CourseMaterial struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	CourseID  string    `json:"course_id"`
	Sort      int       `json:"sort"`
	Kind      string    `json:"kind"` // file, link, video
	Title     string    `json:"title"`
	ObjectKey *string   `json:"object_key,omitempty"`
	URL       *string   `json:"url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CourseSession struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	CourseID  string    `json:"course_id"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Location  string    `json:"location"`
	Quota     int       `json:"quota"`
	Enrolled  int       `json:"enrolled,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type QuizQuestion struct {
	ID           string          `json:"id"`
	SchoolID     string          `json:"school_id"`
	CourseID     string          `json:"course_id"`
	Sort         int             `json:"sort"`
	Prompt       string          `json:"prompt"`
	Options      json.RawMessage `json:"options"`
	CorrectIndex int             `json:"correct_index,omitempty"` // Omitted in ESS response
	CreatedAt    time.Time       `json:"created_at"`
}

type MandatoryTraining struct {
	ID           string    `json:"id"`
	SchoolID     string    `json:"school_id"`
	PositionID   string    `json:"position_id"`
	PositionName string    `json:"position_name,omitempty"`
	CourseID     string    `json:"course_id"`
	CourseTitle  string    `json:"course_title,omitempty"`
	DueDays      int       `json:"due_days"`
	CreatedAt    time.Time `json:"created_at"`
}

type Assignment struct {
	ID               string     `json:"id"`
	SchoolID         string     `json:"school_id"`
	EmployeeID       string     `json:"employee_id"`
	EmployeeName     string     `json:"employee_name,omitempty"`
	CourseID         string     `json:"course_id"`
	CourseTitle      string     `json:"course_title,omitempty"`
	SessionID        *string    `json:"session_id,omitempty"`
	Source           string     `json:"source"` // manual, position, unit, onboarding, self
	DueDate          *time.Time `json:"due_date,omitempty"`
	Status           string     `json:"status"` // pending_approval, not_started, in_progress, completed, rejected, cancelled
	DisplayStatus    string     `json:"display_status,omitempty"`
	ApprovalID       *string    `json:"approval_id,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	Score            *int       `json:"score,omitempty"`
	CertificateDocID *string    `json:"certificate_doc_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type QuizAttempt struct {
	ID           string          `json:"id"`
	SchoolID     string          `json:"school_id"`
	AssignmentID string          `json:"assignment_id"`
	Answers      json.RawMessage `json:"answers"`
	Score        int             `json:"score"`
	Passed       bool            `json:"passed"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Onboarding
type OnboardingTemplate struct {
	ID           string                   `json:"id"`
	SchoolID     string                   `json:"school_id"`
	Name         string                   `json:"name"`
	PositionID   *string                  `json:"position_id,omitempty"`
	DepartmentID *string                  `json:"department_id,omitempty"`
	Tasks        []OnboardingTemplateTask `json:"tasks,omitempty"`
	CreatedAt    time.Time                `json:"created_at"`
	UpdatedAt    time.Time                `json:"updated_at"`
}

type OnboardingTemplateTask struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	TemplateID    string    `json:"template_id"`
	Sort          int       `json:"sort"`
	Title         string    `json:"title"`
	Category      string    `json:"category"` // dokumen, akun, perangkat, orientasi, pelatihan
	OwnerKind     string    `json:"owner_kind"`
	OwnerRole     *string   `json:"owner_role,omitempty"`
	DueOffsetDays int       `json:"due_offset_days"`
	CreatedAt     time.Time `json:"created_at"`
}

type OnboardingPlan struct {
	ID           string           `json:"id"`
	SchoolID     string           `json:"school_id"`
	EmployeeID   string           `json:"employee_id"`
	EmployeeName string           `json:"employee_name,omitempty"`
	TemplateID   string           `json:"template_id"`
	TemplateName string           `json:"template_name,omitempty"`
	StartDate    time.Time        `json:"start_date"`
	Status       string           `json:"status"` // active, completed
	Tasks        []OnboardingTask `json:"tasks,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

type OnboardingTask struct {
	ID          string     `json:"id"`
	SchoolID    string     `json:"school_id"`
	PlanID      string     `json:"plan_id"`
	Title       string     `json:"title"`
	Category    string     `json:"category"`
	OwnerUserID *string    `json:"owner_user_id,omitempty"`
	OwnerRole   *string    `json:"owner_role,omitempty"`
	DueDate     time.Time  `json:"due_date"`
	DoneAt      *time.Time `json:"done_at,omitempty"`
	DoneBy      *string    `json:"done_by,omitempty"`
	IsOverdue   bool       `json:"is_overdue"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
