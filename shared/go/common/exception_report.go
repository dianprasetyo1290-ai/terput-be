package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Exception kinds accepted by core's integration exception queue (menu 80).
const (
	ExceptionKindWebhook        = "webhook"
	ExceptionKindJournalPosting = "journal_posting"
	ExceptionKindNotification   = "notification"
	ExceptionKindDeadLetter     = "dead_letter"
	ExceptionKindImport         = "import"
	ExceptionKindCallback       = "callback"
)

// Exception severities. Core also accepts aliases (warning/low → normal,
// high/error → critical) but reporters should send one of these.
const (
	ExceptionSeverityNormal   = "normal"
	ExceptionSeverityCritical = "critical"
)

// DefaultExceptionMaxAttempts is used when a report leaves MaxAttempts at 0.
const DefaultExceptionMaxAttempts = 3

// ExceptionReport is core's POST /internal/exceptions contract (FR-80-001,
// BUG-113). Reporting the same IdempotencyKey again bumps the existing item
// instead of creating a duplicate.
type ExceptionReport struct {
	SchoolID       string         `json:"school_id"`
	SourceService  string         `json:"source_service"`
	Kind           string         `json:"kind"`
	Severity       string         `json:"severity"`
	CorrelationID  string         `json:"correlation_id"`
	ErrorMessage   string         `json:"error_message"`
	PayloadSummary map[string]any `json:"payload_summary"`
	RetryURL       *string        `json:"retry_url,omitempty"`
	MaxAttempts    int            `json:"max_attempts"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// ErrReporterNotConfigured is returned when CORE_URL or INTERNAL_API_KEY is
// missing, so callers can log it instead of pretending the report landed.
var ErrReporterNotConfigured = errors.New("exception reporter: core url or internal key not configured")

// ImportFailureKey is the idempotency key for a failed import job: retries of
// the same job update one queue item.
func ImportFailureKey(service, jobID string) string {
	return "import:" + service + ":" + jobID
}

// DeadLetterKey is the idempotency key for a message that exhausted its
// retries on a topic/queue.
func DeadLetterKey(service, topic, messageID string) string {
	return "dead_letter:" + service + ":" + topic + ":" + messageID
}

// ExceptionReporter posts failures to core's exception queue.
type ExceptionReporter struct {
	CoreURL     string
	InternalKey string
	Service     string
	HTTP        *http.Client
}

// NewExceptionReporter builds a reporter for service with a 10s timeout and
// correlation propagation.
func NewExceptionReporter(coreURL, internalKey, service string) *ExceptionReporter {
	return &ExceptionReporter{
		CoreURL:     strings.TrimRight(coreURL, "/"),
		InternalKey: internalKey,
		Service:     service,
		HTTP:        &http.Client{Timeout: 10 * time.Second, Transport: CorrelationTransport{}},
	}
}

// Configured reports whether the reporter can reach core.
func (r *ExceptionReporter) Configured() bool {
	return r != nil && r.CoreURL != "" && r.InternalKey != ""
}

// normalizeExceptionReport fills defaults so core's validation passes.
func normalizeExceptionReport(ctx context.Context, service string, e ExceptionReport) (ExceptionReport, error) {
	if e.SourceService == "" {
		e.SourceService = service
	}
	if e.SourceService == "" || e.Kind == "" || e.SchoolID == "" || e.IdempotencyKey == "" || strings.TrimSpace(e.ErrorMessage) == "" {
		return e, errors.New("exception report: school_id, source_service, kind, error_message and idempotency_key are required")
	}
	if e.Severity == "" {
		e.Severity = ExceptionSeverityNormal
	}
	if e.CorrelationID == "" {
		e.CorrelationID = CorrelationID(ctx)
	}
	if e.CorrelationID == "" {
		e.CorrelationID = NewCorrelationID()
	}
	if e.PayloadSummary == nil {
		e.PayloadSummary = map[string]any{}
	}
	if e.MaxAttempts <= 0 {
		e.MaxAttempts = DefaultExceptionMaxAttempts
	}
	return e, nil
}

// Report sends e to core. A non-2xx answer is returned as an error.
func (r *ExceptionReporter) Report(ctx context.Context, e ExceptionReport) error {
	if !r.Configured() {
		return ErrReporterNotConfigured
	}
	e, err := normalizeExceptionReport(ctx, r.Service, e)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.CoreURL+"/internal/exceptions", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Internal-Key", r.InternalKey)
	req.Header.Set("Idempotency-Key", e.IdempotencyKey)
	req.Header.Set(CorrelationHeader, e.CorrelationID)
	client := r.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("exception report: core answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// ReportImportFailure queues a failed import job (kind import).
func (r *ExceptionReporter) ReportImportFailure(ctx context.Context, schoolID, jobID, message string, summary map[string]any) error {
	if summary == nil {
		summary = map[string]any{}
	}
	summary["job_id"] = jobID
	return r.Report(ctx, ExceptionReport{
		SchoolID: schoolID, Kind: ExceptionKindImport, Severity: ExceptionSeverityNormal,
		ErrorMessage: message, PayloadSummary: summary,
		IdempotencyKey: ImportFailureKey(r.Service, jobID),
	})
}

// ReportDeadLetter queues a message that exhausted its retries (kind
// dead_letter, critical).
func (r *ExceptionReporter) ReportDeadLetter(ctx context.Context, schoolID, topic, messageID, message string, attempts int, summary map[string]any) error {
	if summary == nil {
		summary = map[string]any{}
	}
	summary["topic"] = topic
	summary["message_id"] = messageID
	summary["attempts"] = attempts
	return r.Report(ctx, ExceptionReport{
		SchoolID: schoolID, Kind: ExceptionKindDeadLetter, Severity: ExceptionSeverityCritical,
		ErrorMessage: message, PayloadSummary: summary, MaxAttempts: attempts,
		IdempotencyKey: DeadLetterKey(r.Service, topic, messageID),
	})
}
