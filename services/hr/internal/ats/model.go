package ats

import (
	"time"
)

type Requisition struct {
	ID                  string     `json:"id" db:"id"`
	SchoolID            string     `json:"school_id" db:"school_id"`
	PositionID          string     `json:"position_id" db:"position_id"`
	PositionName        string     `json:"position_name,omitempty" db:"position_name"`
	DepartmentID        *string    `json:"department_id,omitempty" db:"department_id"`
	Quantity            int        `json:"quantity" db:"quantity"`
	Reason              string     `json:"reason" db:"reason"`
	ReplacesEmployeeID  *string    `json:"replaces_employee_id,omitempty" db:"replaces_employee_id"`
	Qualifications      string     `json:"qualifications" db:"qualifications"`
	BudgetIDR           *int64     `json:"budget_idr,omitempty" db:"budget_idr"`
	ExceedsHeadcount    bool       `json:"exceeds_headcount" db:"exceeds_headcount"`
	Status              string     `json:"status" db:"status"`
	FilledCount         int        `json:"filled_count" db:"filled_count"`
	ApprovalID          *string    `json:"approval_id,omitempty" db:"approval_id"`
	RequestedBy         string     `json:"requested_by" db:"requested_by"`
	IdempotencyKey      string     `json:"idempotency_key" db:"idempotency_key"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

type JobPosting struct {
	ID             string     `json:"id" db:"id"`
	SchoolID       string     `json:"school_id" db:"school_id"`
	RequisitionID  string     `json:"requisition_id" db:"requisition_id"`
	Slug           string     `json:"slug" db:"slug"`
	Title          string     `json:"title" db:"title"`
	Description    string     `json:"description" db:"description"`
	Qualifications string     `json:"qualifications" db:"qualifications"`
	EmploymentType string     `json:"employment_type" db:"employment_type"`
	ClosesOn       *string    `json:"closes_on,omitempty" db:"closes_on"`
	Status         string     `json:"status" db:"status"`
	ClosedReason   *string    `json:"closed_reason,omitempty" db:"closed_reason"`
	PublishedAt    *time.Time `json:"published_at,omitempty" db:"published_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type PipelineStage struct {
	ID                string     `json:"id" db:"id"`
	SchoolID          string     `json:"school_id" db:"school_id"`
	Code              string     `json:"code" db:"code"`
	Name              string     `json:"name" db:"name"`
	Sort              int        `json:"sort" db:"sort"`
	Kind              string     `json:"kind" db:"kind"`
	NotifyTemplateKey *string    `json:"notify_template_key,omitempty" db:"notify_template_key"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
}

type Candidate struct {
	ID                  string     `json:"id" db:"id"`
	SchoolID            string     `json:"school_id" db:"school_id"`
	FullName            string     `json:"full_name" db:"full_name"`
	Email               string     `json:"email" db:"email"`
	EmailNorm           string     `json:"email_norm" db:"email_norm"`
	ConsentProcessingAt time.Time  `json:"consent_processing_at" db:"consent_processing_at"`
	ConsentTalentPool   bool       `json:"consent_talent_pool" db:"consent_talent_pool"`
	AnonymizedAt        *time.Time `json:"anonymized_at,omitempty" db:"anonymized_at"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

type Application struct {
	ID              string     `json:"id" db:"id"`
	SchoolID        string     `json:"school_id" db:"school_id"`
	PostingID       string     `json:"posting_id" db:"posting_id"`
	PostingTitle    string     `json:"posting_title,omitempty" db:"posting_title"`
	CandidateID     string     `json:"candidate_id" db:"candidate_id"`
	CandidateName   string     `json:"candidate_name,omitempty" db:"candidate_name"`
	CandidateEmail  string     `json:"candidate_email,omitempty" db:"candidate_email"`
	StageID         string     `json:"stage_id" db:"stage_id"`
	StageName       string     `json:"stage_name,omitempty" db:"stage_name"`
	Status          string     `json:"status" db:"status"`
	CVObjectKey     string     `json:"cv_object_key" db:"cv_object_key"`
	Answers         string     `json:"answers" db:"answers"`
	Source          string     `json:"source" db:"source"`
	AccessTokenHash string     `json:"-" db:"access_token_hash"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type ScorecardTemplate struct {
	ID         string              `json:"id" db:"id"`
	SchoolID   string              `json:"school_id" db:"school_id"`
	PositionID *string             `json:"position_id,omitempty" db:"position_id"`
	Name       string              `json:"name" db:"name"`
	Criteria   []ScorecardCriterion `json:"criteria,omitempty"`
	CreatedAt  time.Time           `json:"created_at" db:"created_at"`
}

type ScorecardCriterion struct {
	ID         string `json:"id" db:"id"`
	SchoolID   string `json:"school_id" db:"school_id"`
	TemplateID string `json:"template_id" db:"template_id"`
	Name       string `json:"name" db:"name"`
	WeightBP   int    `json:"weight_bp" db:"weight_bp"`
}

type Interview struct {
	ID             string     `json:"id" db:"id"`
	SchoolID       string     `json:"school_id" db:"school_id"`
	ApplicationID  string     `json:"application_id" db:"application_id"`
	CandidateName  string     `json:"candidate_name,omitempty" db:"candidate_name"`
	PostingTitle   string     `json:"posting_title,omitempty" db:"posting_title"`
	Kind           string     `json:"kind" db:"kind"`
	StartsAt       time.Time  `json:"starts_at" db:"starts_at"`
	EndsAt         time.Time  `json:"ends_at" db:"ends_at"`
	LocationOrLink string     `json:"location_or_link" db:"location_or_link"`
	TemplateID     *string    `json:"template_id,omitempty" db:"template_id"`
	Status         string     `json:"status" db:"status"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}

type InterviewScore struct {
	ID             string     `json:"id" db:"id"`
	SchoolID       string     `json:"school_id" db:"school_id"`
	InterviewID    string     `json:"interview_id" db:"interview_id"`
	PanelUserID    string     `json:"panel_user_id" db:"panel_user_id"`
	CriterionID    string     `json:"criterion_id" db:"criterion_id"`
	CriterionName  string     `json:"criterion_name,omitempty" db:"criterion_name"`
	Score          int        `json:"score" db:"score"`
	Recommendation *string    `json:"recommendation,omitempty" db:"recommendation"`
	SubmittedAt    *time.Time `json:"submitted_at,omitempty" db:"submitted_at"`
}

type Offer struct {
	ID                string     `json:"id" db:"id"`
	SchoolID          string     `json:"school_id" db:"school_id"`
	ApplicationID     string     `json:"application_id" db:"application_id"`
	CandidateName     string     `json:"candidate_name,omitempty" db:"candidate_name"`
	PositionID        string     `json:"position_id" db:"position_id"`
	PositionName      string     `json:"position_name,omitempty" db:"position_name"`
	StartDate         string     `json:"start_date" db:"start_date"`
	EmploymentType    string     `json:"employment_type" db:"employment_type"`
	BaseSalaryIDR     int64      `json:"base_salary_idr"`
	OutOfRange        bool       `json:"out_of_range" db:"out_of_range"`
	Status            string     `json:"status" db:"status"`
	ApprovalID        *string    `json:"approval_id,omitempty" db:"approval_id"`
	DocumentObjectKey string     `json:"document_object_key" db:"document_object_key"`
	RespondedAt       *time.Time `json:"responded_at,omitempty" db:"responded_at"`
	ResponseNote      *string    `json:"response_note,omitempty" db:"response_note"`
	EmployeeID        *string    `json:"employee_id,omitempty" db:"employee_id"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

type AtsSettings struct {
	SchoolID          string `json:"school_id" db:"school_id"`
	RetentionMonths   int    `json:"retention_months" db:"retention_months"`
	CareerPageEnabled bool   `json:"career_page_enabled" db:"career_page_enabled"`
	CareerIntro       string `json:"career_intro" db:"career_intro"`
}
