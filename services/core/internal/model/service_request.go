package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidStatusTransition = errors.New("invalid status transition")
	ErrRejectionNoteRequired    = errors.New("rejection note is required")
	ErrCategoryNotFound         = errors.New("service request category not found")
	ErrCategoryInactive         = errors.New("service request category is inactive")
	ErrStudentRequired          = errors.New("student selection is required for this category")
	ErrUnauthorizedHandler      = errors.New("user is not authorized to handle this category")
)

type ServiceRequestCategory struct {
	ID              string     `json:"id"`
	SchoolID        string     `json:"school_id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	HandlerRole     string     `json:"handler_role"`
	SLAHours        *int       `json:"sla_hours,omitempty"`
	RequiresStudent bool       `json:"requires_student"`
	Active          bool       `json:"active"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Attachment struct {
	Name     string `json:"name"`
	Key      string `json:"key"`
	URL      string `json:"url,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type ServiceRequest struct {
	ID                string          `json:"id"`
	SchoolID          string          `json:"school_id"`
	CategoryID        string          `json:"category_id"`
	CategoryName      string          `json:"category_name,omitempty"`
	HandlerRole       string          `json:"handler_role,omitempty"`
	RequesterUserID   string          `json:"requester_user_id"`
	RequesterName     string          `json:"requester_name,omitempty"`
	StudentUserID     *string         `json:"student_user_id,omitempty"`
	StudentName       string          `json:"student_name,omitempty"`
	Subject           string          `json:"subject"`
	Description       string          `json:"description"`
	Attachments       json.RawMessage `json:"attachments"`
	Status            string          `json:"status"`
	AssigneeUserID    *string         `json:"assignee_user_id,omitempty"`
	ResultAttachments json.RawMessage `json:"result_attachments"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type ServiceRequestEvent struct {
	ID          string    `json:"id"`
	SchoolID    string    `json:"school_id"`
	RequestID   string    `json:"request_id"`
	FromStatus  *string   `json:"from_status,omitempty"`
	ToStatus    string    `json:"to_status"`
	Note        string    `json:"note"`
	ActorUserID *string   `json:"actor_user_id,omitempty"`
	ActorName   string    `json:"actor_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Valid transitions:
// submitted -> accepted, rejected
// accepted -> in_progress, rejected
// in_progress -> done, rejected
func ValidateStatusTransition(fromStatus, toStatus, note string) error {
	from := strings.ToLower(strings.TrimSpace(fromStatus))
	to := strings.ToLower(strings.TrimSpace(toStatus))

	if to == "rejected" && strings.TrimSpace(note) == "" {
		return ErrRejectionNoteRequired
	}

	switch from {
	case "submitted":
		if to == "accepted" || to == "rejected" {
			return nil
		}
	case "accepted":
		if to == "in_progress" || to == "rejected" {
			return nil
		}
	case "in_progress":
		if to == "done" || to == "rejected" {
			return nil
		}
	}

	return fmt.Errorf("%w: cannot transition from '%s' to '%s'", ErrInvalidStatusTransition, fromStatus, toStatus)
}
