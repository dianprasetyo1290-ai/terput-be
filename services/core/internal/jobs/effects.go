package jobs

import (
	"context"
	"fmt"
	"log"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

// Side effects shared by the HTTP handlers (right after a transition) and the
// periodic runner (retries, escalation): source callbacks and notifications.

// CallbackPayload is the body POSTed to a source's callback_url (T1 §3 contract).
func CallbackPayload(req *model.ApprovalRequest) map[string]any {
	decidedBy, comment := model.FinalDecision(req)
	return map[string]any{
		"approval_id":  req.ID,
		"source_id":    req.SourceID,
		"request_type": req.RequestType,
		"decision":     req.Status,
		"decided_by":   decidedBy,
		"comment":      comment,
		"decided_at":   req.DecidedAt,
	}
}

// SendCallback makes one delivery attempt with Idempotency-Key approval:<id>.
func SendCallback(ctx context.Context, cl *client.Client, req *model.ApprovalRequest) error {
	if cl == nil {
		return client.ErrNotConfigured
	}
	return cl.SendCallback(ctx, req.CallbackURL, "approval:"+req.ID, CallbackPayload(req))
}

// DeliverCallback attempts delivery and records the outcome. When attempts are
// exhausted the request goes to the exception queue (kind=callback, critical).
func DeliverCallback(ctx context.Context, st store.Store, cl *client.Client, req *model.ApprovalRequest, maxAttempts int) {
	sendErr := SendCallback(ctx, cl, req)
	if sendErr == nil {
		if err := st.MarkCallbackDone(ctx, req.ID); err != nil {
			log.Printf("callback %s: mark done: %v", req.ID, err)
		}
		return
	}
	attempts, exhausted, err := st.MarkCallbackFailed(ctx, req.ID, maxAttempts)
	if err != nil {
		log.Printf("callback %s: mark failed: %v", req.ID, err)
		return
	}
	if !exhausted {
		return
	}
	msg := fmt.Sprintf("callback ke %s gagal setelah %d percobaan: %v", req.SourceService, attempts, sendErr)
	_ = st.InsertAudit(ctx, model.AuditEvent{
		SchoolID: &req.SchoolID, Service: "core", Module: "approval", EntityType: "approval_request",
		EntityID: req.ID, Action: "callback_failed", Reason: &msg, CorrelationID: &req.CorrelationID,
	})
	exc, isNew, err := st.UpsertException(ctx, model.IntegrationException{
		SchoolID:      req.SchoolID,
		SourceService: "core",
		Kind:          model.ExceptionKindCallback,
		Severity:      model.SeverityCritical,
		CorrelationID: req.CorrelationID,
		ErrorMessage:  msg,
		MaxAttempts:   maxAttempts,
		// retry_url stays NULL: a callback retry is redelivered by core itself
		// from approval_id (see handler.RetryException).
		PayloadSummary: map[string]any{
			"approval_id":    req.ID,
			"source_service": req.SourceService,
			"source_id":      req.SourceID,
			"request_type":   req.RequestType,
			"decision":       req.Status,
		},
		IdempotencyKey: "callback:" + req.ID,
	})
	if err != nil {
		log.Printf("callback %s: queue exception: %v", req.ID, err)
		return
	}
	if isNew {
		NotifyCriticalException(ctx, cl, exc)
	}
}

// StepRecipients resolves who acts on a step: its named user (also the
// resolved direct supervisor), else every holder of its role in the school.
func StepRecipients(ctx context.Context, cl *client.Client, schoolID string, st model.ApprovalStep) []string {
	if st.ApproverUserID != nil {
		return []string{*st.ApproverUserID}
	}
	if st.ApproverRole == nil || cl == nil {
		return nil
	}
	ids, err := cl.UsersByRole(ctx, schoolID, *st.ApproverRole)
	if err != nil {
		log.Printf("resolve recipients for role %s: %v", *st.ApproverRole, err)
	}
	return ids
}

func approvalData(req *model.ApprovalRequest) map[string]any {
	return map[string]any{"approval_id": req.ID, "source_service": req.SourceService, "source_id": req.SourceID}
}

// NotifyStep tells a step's approvers that a request waits on them (FR-07-002).
func NotifyStep(ctx context.Context, cl *client.Client, req *model.ApprovalRequest, st model.ApprovalStep, title, body string) {
	if cl == nil {
		return
	}
	err := cl.NotifyUsers(ctx, client.Notice{
		SchoolID: req.SchoolID,
		KeyBase:  fmt.Sprintf("approval:%s:%d:%s", req.ID, st.Seq, st.Status),
		Type:     "approval", Title: title, Body: body, Data: approvalData(req),
	}, StepRecipients(ctx, cl, req.SchoolID, st))
	if err != nil {
		log.Printf("notify step %s/%d: %v", req.ID, st.Seq, err)
	}
}

// NotifyMaker tells the maker the final outcome.
func NotifyMaker(ctx context.Context, cl *client.Client, req *model.ApprovalRequest) {
	if cl == nil {
		return
	}
	title, body := "Pengajuan Disetujui", fmt.Sprintf("Pengajuan %q telah disetujui.", req.Title)
	if req.Status == model.RequestStatusRejected {
		_, comment := model.FinalDecision(req)
		title, body = "Pengajuan Ditolak", fmt.Sprintf("Pengajuan %q ditolak: %s", req.Title, comment)
	}
	err := cl.NotifyUsers(ctx, client.Notice{
		SchoolID: req.SchoolID,
		KeyBase:  fmt.Sprintf("approval:%s:final:%s", req.ID, req.Status),
		Type:     "approval", Title: title, Body: body, Data: approvalData(req),
	}, []string{req.MakerUserID})
	if err != nil {
		log.Printf("notify maker %s: %v", req.ID, err)
	}
}

// AfterTransition runs the side effects of a decision: next approver
// notification, or (when final) callback + maker notification.
func AfterTransition(ctx context.Context, st store.Store, cl *client.Client, req *model.ApprovalRequest, maxAttempts int) {
	if next := model.CurrentStep(req); next != nil {
		NotifyStep(ctx, cl, req, *next, "Menunggu Persetujuan", fmt.Sprintf("Pengajuan %q menunggu persetujuan Anda.", req.Title))
		return
	}
	if req.Status == model.RequestStatusApproved || req.Status == model.RequestStatusRejected {
		DeliverCallback(ctx, st, cl, req, maxAttempts)
		NotifyMaker(ctx, cl, req)
	}
}

// NotifyCriticalException alerts the school's admins about a new critical
// exception (FR-80-006).
func NotifyCriticalException(ctx context.Context, cl *client.Client, exc *model.IntegrationException) {
	if cl == nil || exc == nil || exc.Severity != model.SeverityCritical {
		return
	}
	admins, err := cl.UsersByRole(ctx, exc.SchoolID, common.RoleAdmin)
	if err != nil {
		log.Printf("resolve admins for exception %s: %v", exc.ID, err)
		return
	}
	err = cl.NotifyUsers(ctx, client.Notice{
		SchoolID: exc.SchoolID,
		KeyBase:  "exception:" + exc.ID + ":critical",
		Type:     "integration_exception",
		Title:    "Galat Integrasi Kritis",
		Body:     fmt.Sprintf("%s (%s): %s", exc.SourceService, exc.Kind, exc.ErrorMessage),
		Data:     map[string]any{"exception_id": exc.ID, "correlation_id": exc.CorrelationID},
	}, admins)
	if err != nil {
		log.Printf("notify critical exception %s: %v", exc.ID, err)
	}
}
