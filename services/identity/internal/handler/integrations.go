package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"skci/identity/internal/common"
)

// Calls from identity to core (Approval Center) and notification. Both are
// authenticated with X-Internal-Key. Contracts: core T1 §3
// (POST /internal/approvals, callback {approval_id, source_id, request_type,
// decision, decided_by, comment, decided_at}) and notification
// POST /internal/notifications.

var errNotConfigured = errors.New("layanan tujuan belum dikonfigurasi")

// upstreamError carries the HTTP status and the readable message the other
// service returned, so the UI can show it instead of a generic failure.
type upstreamError struct {
	Status  int
	Message string
}

func (e *upstreamError) Error() string {
	return fmt.Sprintf("status %d: %s", e.Status, e.Message)
}

// selfURL is identity's own internal base URL as core reaches it (callbacks).
func (a *App) selfURL() string {
	if a.SelfURL != "" {
		return strings.TrimRight(a.SelfURL, "/")
	}
	if v := os.Getenv("IDENTITY_INTERNAL_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://identity:8081"
}

type approvalSubmission struct {
	SchoolID       string         `json:"school_id"`
	RequestType    string         `json:"request_type"`
	SourceService  string         `json:"source_service"`
	SourceID       string         `json:"source_id"`
	Title          string         `json:"title"`
	Summary        map[string]any `json:"summary"`
	MakerUserID    string         `json:"maker_user_id"`
	SubjectUserID  *string        `json:"subject_user_id,omitempty"`
	CallbackURL    string         `json:"callback_url"`
	CorrelationID  string         `json:"correlation_id"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// submitApproval creates an approval request in core and returns its id.
func (a *App) submitApproval(ctx context.Context, in approvalSubmission) (string, error) {
	if a.CoreURL == "" {
		return "", errNotConfigured
	}
	var out struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := a.postInternal(ctx, strings.TrimRight(a.CoreURL, "/")+"/internal/approvals", in, in.IdempotencyKey, &out); err != nil {
		return "", err
	}
	if out.Data.ID == "" {
		return "", &upstreamError{Status: http.StatusBadGateway, Message: "Approval Center tidak mengembalikan nomor permintaan"}
	}
	return out.Data.ID, nil
}

// cancelApproval withdraws a pending approval in core (best effort).
func (a *App) cancelApproval(ctx context.Context, approvalID, actorUserID, reason string) error {
	if a.CoreURL == "" || approvalID == "" {
		return errNotConfigured
	}
	return a.postInternal(ctx, strings.TrimRight(a.CoreURL, "/")+"/internal/approvals/"+approvalID+"/cancel",
		map[string]string{"actor_user_id": actorUserID, "reason": reason}, "", nil)
}

// notifyUser sends one notification (in-app plus the user's channels; the
// notification service applies preferences and mandatory types).
func (a *App) notifyUser(ctx context.Context, schoolID, userID, typ, title, body, idemKey string, data map[string]any) error {
	if a.NotificationURL == "" {
		return errNotConfigured
	}
	payload := map[string]any{
		"school_id": schoolID,
		"user_id":   userID,
		"type":      typ,
		"title":     title,
		"body":      body,
		"channels":  []string{"in_app", "email"},
	}
	if data != nil {
		payload["data"] = data
	}
	return a.postInternal(ctx, strings.TrimRight(a.NotificationURL, "/")+"/internal/notifications", payload, idemKey, nil)
}

func (a *App) postInternal(ctx context.Context, url string, payload any, idemKey string, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", a.InternalKey)
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	if cid := correlationFromCtx(ctx); cid != "" {
		req.Header.Set("X-Correlation-ID", cid)
	}
	resp, err := a.httpClient().Do(req)
	if err != nil {
		common.CaptureError(err)
		return &upstreamError{Status: http.StatusServiceUnavailable, Message: "layanan tujuan tidak dapat dihubungi"}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var env struct {
			Message string `json:"message"`
			Errors  struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		_ = json.Unmarshal(raw, &env)
		msg := env.Message
		if msg == "" {
			msg = env.Errors.Message
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return &upstreamError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (a *App) httpClient() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return &http.Client{Timeout: 8 * time.Second, Transport: common.CorrelationTransport{}}
}

type ctxKeyCorrelation struct{}

func withCorrelation(ctx context.Context, r *http.Request) context.Context {
	if cid := r.Header.Get(common.CorrelationHeader); cid != "" {
		return context.WithValue(ctx, ctxKeyCorrelation{}, cid)
	}
	return ctx
}

// correlationFromCtx: the id withCorrelation stored, else the one
// common.CorrelationMiddleware put on the request context.
func correlationFromCtx(ctx context.Context) string {
	if v, _ := ctx.Value(ctxKeyCorrelation{}).(string); v != "" {
		return v
	}
	return common.CorrelationID(ctx)
}

// upstreamMessage turns an integration error into a readable sentence.
func upstreamMessage(service string, err error) string {
	var ue *upstreamError
	switch {
	case errors.Is(err, errNotConfigured):
		return service + " belum dikonfigurasi pada server"
	case errors.As(err, &ue):
		return service + ": " + ue.Message
	default:
		return service + " tidak dapat dihubungi"
	}
}
