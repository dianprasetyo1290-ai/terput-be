package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"skci/core/internal/model"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrConflict       = errors.New("conflict")
	ErrIdempotentHit  = errors.New("idempotency hit")
	ErrForbidden      = errors.New("forbidden")
	ErrInvalidRequest = errors.New("invalid request")
)

// Store defines persistence operations for the core service.
type Store interface {
	// Approval Flows
	ListFlows(ctx context.Context, schoolID string) ([]model.ApprovalFlow, error)
	GetFlow(ctx context.Context, schoolID, id string) (*model.ApprovalFlow, error)
	GetFlowByRequestType(ctx context.Context, schoolID, requestType string) (*model.ApprovalFlow, error)
	CreateFlow(ctx context.Context, schoolID, createdBy string, flow model.ApprovalFlow) (*model.ApprovalFlow, error)
	UpdateFlow(ctx context.Context, schoolID, id string, flow model.ApprovalFlow) (*model.ApprovalFlow, error)
	DeleteFlow(ctx context.Context, schoolID, id string) error

	// Approval Requests & Steps
	CreateRequest(ctx context.Context, req model.ApprovalRequest, steps []model.ApprovalStep) (*model.ApprovalRequest, error)
	GetRequestByID(ctx context.Context, schoolID, id string) (*model.ApprovalRequest, error)
	GetRequestByIdempotency(ctx context.Context, schoolID, key string) (*model.ApprovalRequest, error)
	GetRequestBySource(ctx context.Context, schoolID, sourceService, sourceID string) (*model.ApprovalRequest, error)
	ListInbox(ctx context.Context, f InboxFilter, page, perPage int) ([]model.ApprovalRequest, int, error)
	ListMine(ctx context.Context, schoolID, userID string, page, perPage int) ([]model.ApprovalRequest, int, error)
	DecideStep(ctx context.Context, schoolID, reqID string, seq int, decision, decidedBy string, delegateOf *string, comment string) (*model.ApprovalRequest, error)
	CancelRequest(ctx context.Context, reqID string) (*model.ApprovalRequest, error)
	MarkCallbackDone(ctx context.Context, reqID string) error
	MarkCallbackFailed(ctx context.Context, reqID string, maxAttempts int) (attempts int, exhausted bool, err error)
	ListPendingCallbacks(ctx context.Context, maxAttempts int) ([]model.ApprovalRequest, error)
	ListExpiredSteps(ctx context.Context) ([]model.ApprovalStep, error)
	EscalateStep(ctx context.Context, stepID string) (*model.ApprovalStep, error)

	// Approval Delegations
	ListDelegations(ctx context.Context, schoolID, userID string) ([]model.ApprovalDelegation, error)
	CreateDelegation(ctx context.Context, delegation model.ApprovalDelegation) (*model.ApprovalDelegation, error)
	RevokeDelegation(ctx context.Context, schoolID, id, userID string) error
	GetActiveDelegationsForUser(ctx context.Context, schoolID, userID string) ([]model.ActiveDelegation, error)

	// Integration Exceptions
	UpsertException(ctx context.Context, exc model.IntegrationException) (*model.IntegrationException, bool, error)
	GetExceptionByID(ctx context.Context, schoolID, id string) (*model.IntegrationException, error)
	ListExceptions(ctx context.Context, f ExceptionFilter, page, perPage int) ([]model.IntegrationException, int, error)
	AssignException(ctx context.Context, schoolID, id, assigneeUserID string) error
	ResolveException(ctx context.Context, schoolID, id, resolvedBy, reason string) error
	ClaimExceptionForRetry(ctx context.Context, schoolID, id string, staleAfter time.Duration) error
	ReopenException(ctx context.Context, schoolID, id, lastError string) error

	// Central Audit Trail
	InsertAudit(ctx context.Context, event model.AuditEvent) error
	ListAudit(ctx context.Context, f AuditFilter, page, perPage int) ([]model.AuditEvent, int, error)

	// Parent Service Requests (Menu 81)
	ListServiceRequestCategories(ctx context.Context, schoolID string, activeOnly bool) ([]model.ServiceRequestCategory, error)
	GetServiceRequestCategory(ctx context.Context, schoolID, id string) (*model.ServiceRequestCategory, error)
	CreateServiceRequestCategory(ctx context.Context, cat model.ServiceRequestCategory) (*model.ServiceRequestCategory, error)
	UpdateServiceRequestCategory(ctx context.Context, schoolID, id string, cat model.ServiceRequestCategory) (*model.ServiceRequestCategory, error)
	DeleteServiceRequestCategory(ctx context.Context, schoolID, id string) error

	CreateServiceRequest(ctx context.Context, req model.ServiceRequest) (*model.ServiceRequest, error)
	GetServiceRequest(ctx context.Context, schoolID, id string) (*model.ServiceRequest, error)
	ListParentServiceRequests(ctx context.Context, schoolID, requesterUserID string) ([]model.ServiceRequest, error)
	ListHandlerServiceRequests(ctx context.Context, schoolID string, roles []string, status string) ([]model.ServiceRequest, error)
	UpdateServiceRequestStatus(ctx context.Context, schoolID, id, newStatus, note, actorUserID string) (*model.ServiceRequest, error)
	UpdateServiceRequestResult(ctx context.Context, schoolID, id string, resultAttachments []byte) (*model.ServiceRequest, error)
	ListServiceRequestEvents(ctx context.Context, schoolID, requestID string) ([]model.ServiceRequestEvent, error)
	CanUserHandleServiceRequests(ctx context.Context, schoolID string, roles []string) ([]string, error)
}

// PGStore implements Store over Postgres.
type PGStore struct {
	DB *sql.DB
}

func NewPGStore(db *sql.DB) *PGStore {
	return &PGStore{DB: db}
}
