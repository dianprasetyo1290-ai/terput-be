package jobs

import (
	"context"
	"errors"
	"fmt"
	"log"

	"skci/core/internal/client"
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
// The maker is left out: BR-GEN-09 forbids them to decide (BUG-111).
func StepRecipients(ctx context.Context, cl *client.Client, req *model.ApprovalRequest, st model.ApprovalStep) ([]string, error) {
	var ids []string
	switch {
	case st.ApproverUserID != nil:
		ids = []string{*st.ApproverUserID}
	case st.ApproverRole != nil && cl != nil:
		var err error
		if ids, err = cl.UsersByRole(ctx, req.SchoolID, *st.ApproverRole); err != nil {
			return nil, fmt.Errorf("resolve recipients for role %s: %w", *st.ApproverRole, err)
		}
	}
	out := ids[:0:0]
	for _, id := range ids {
		if id != req.MakerUserID {
			out = append(out, id)
		}
	}
	return out, nil
}

func approvalData(req *model.ApprovalRequest) map[string]any {
	return map[string]any{"approval_id": req.ID, "source_service": req.SourceService, "source_id": req.SourceID,
		"correlation_id": req.CorrelationID, "url": "/approval/" + req.ID}
}

// reportNotifyFailure makes an undelivered approval notification visible in
// the exception queue (FR-80-001, BUG-028/113) instead of a log line only.
// The same key reported again bumps attempts on the existing item.
func reportNotifyFailure(ctx context.Context, st store.Store, req *model.ApprovalRequest, keyBase string, cause error) {
	log.Printf("notify %s: %v", keyBase, cause)
	if st == nil {
		return
	}
	_, _, err := st.UpsertException(ctx, model.IntegrationException{
		SchoolID:      req.SchoolID,
		SourceService: "core",
		Kind:          model.ExceptionKindNotification,
		Severity:      model.SeverityNormal,
		CorrelationID: req.CorrelationID,
		ErrorMessage:  "notifikasi approval gagal dikirim: " + cause.Error(),
		MaxAttempts:   1,
		PayloadSummary: map[string]any{
			"approval_id": req.ID, "request_type": req.RequestType, "notification_key": keyBase,
		},
		IdempotencyKey: "notify:" + keyBase,
	})
	if err != nil {
		log.Printf("notify %s: queue exception: %v", keyBase, err)
	}
}

// NotifyStep tells a step's approvers that a request waits on them (FR-07-002).
func NotifyStep(ctx context.Context, st store.Store, cl *client.Client, req *model.ApprovalRequest, step model.ApprovalStep, title, body string) {
	if cl == nil {
		return
	}
	keyBase := fmt.Sprintf("approval:%s:%d:%s", req.ID, step.Seq, step.Status)
	recipients, err := StepRecipients(ctx, cl, req, step)
	if err == nil && len(recipients) == 0 {
		err = errors.New("tidak ada penerima untuk tahap ini")
	}
	if err == nil {
		err = cl.NotifyUsers(ctx, client.Notice{
			SchoolID: req.SchoolID, KeyBase: keyBase,
			Type: "approval", Title: title, Body: body, Data: approvalData(req),
		}, recipients)
	}
	if err != nil {
		reportNotifyFailure(ctx, st, req, keyBase, err)
	}
}

// NotifyMaker tells the maker the final outcome.
func NotifyMaker(ctx context.Context, st store.Store, cl *client.Client, req *model.ApprovalRequest) {
	if cl == nil {
		return
	}
	title, body := "Pengajuan Disetujui", fmt.Sprintf("Pengajuan %q telah disetujui.", req.Title)
	if req.Status == model.RequestStatusRejected {
		_, comment := model.FinalDecision(req)
		title, body = "Pengajuan Ditolak", fmt.Sprintf("Pengajuan %q ditolak: %s", req.Title, comment)
	}
	keyBase := fmt.Sprintf("approval:%s:final:%s", req.ID, req.Status)
	err := cl.NotifyUsers(ctx, client.Notice{
		SchoolID: req.SchoolID, KeyBase: keyBase,
		Type: "approval", Title: title, Body: body, Data: approvalData(req),
	}, []string{req.MakerUserID})
	if err != nil {
		reportNotifyFailure(ctx, st, req, keyBase, err)
	}
}

// AfterTransition runs the side effects of a decision: next approver
// notification, or (when final) callback + maker notification.
func AfterTransition(ctx context.Context, st store.Store, cl *client.Client, req *model.ApprovalRequest, maxAttempts int) {
	if next := model.CurrentStep(req); next != nil {
		NotifyStep(ctx, st, cl, req, *next, "Menunggu Persetujuan", fmt.Sprintf("Pengajuan %q menunggu persetujuan Anda.", req.Title))
		return
	}
	if req.Status == model.RequestStatusApproved || req.Status == model.RequestStatusRejected {
		DeliverCallback(ctx, st, cl, req, maxAttempts)
		NotifyMaker(ctx, st, cl, req)
	}
}

// NotifyCriticalException alerts the people responsible for a new critical
// exception (FR-80-006): every holder of a role allowed to act on its kind
// (model.ExceptionResponsibleRoles), so a journal-posting failure reaches
// Accounting and a webhook failure the Finance Admin.
func NotifyCriticalException(ctx context.Context, cl *client.Client, exc *model.IntegrationException) {
	if cl == nil || exc == nil || exc.Severity != model.SeverityCritical {
		return
	}
	var recipients []string
	for _, role := range model.ExceptionResponsibleRoles(exc.Kind) {
		ids, err := cl.UsersByRole(ctx, exc.SchoolID, role)
		if err != nil {
			log.Printf("resolve %s for exception %s: %v", role, exc.ID, err)
			continue
		}
		recipients = append(recipients, ids...)
	}
	if len(recipients) == 0 {
		log.Printf("critical exception %s (%s): no responsible user to notify", exc.ID, exc.Kind)
		return
	}
	err := cl.NotifyUsers(ctx, client.Notice{
		SchoolID: exc.SchoolID,
		KeyBase:  "exception:" + exc.ID + ":critical",
		Type:     "integration_exception",
		Title:    "Galat Integrasi Kritis",
		Body:     fmt.Sprintf("%s (%s): %s", exc.SourceService, exc.Kind, exc.ErrorMessage),
		Data:     map[string]any{"exception_id": exc.ID, "correlation_id": exc.CorrelationID, "url": "/integrasi/antrean-galat"},
	}, recipients)
	if err != nil {
		log.Printf("notify critical exception %s: %v", exc.ID, err)
	}
}

// NotifyAssignee tells the officer an exception was assigned to them (FR-80-004).
func NotifyAssignee(ctx context.Context, cl *client.Client, exc *model.IntegrationException, assigneeUserID string) {
	if cl == nil || exc == nil {
		return
	}
	err := cl.NotifyUsers(ctx, client.Notice{
		SchoolID: exc.SchoolID,
		KeyBase:  "exception:" + exc.ID + ":assigned:" + exc.UpdatedAt.Format("20060102150405.000"),
		Type:     "integration_exception",
		Title:    "Galat Integrasi Ditugaskan kepada Anda",
		Body:     fmt.Sprintf("%s (%s): %s", exc.SourceService, exc.Kind, exc.ErrorMessage),
		Data:     map[string]any{"exception_id": exc.ID, "correlation_id": exc.CorrelationID, "url": "/integrasi/antrean-galat"},
	}, []string{assigneeUserID})
	if err != nil {
		log.Printf("notify assignee of exception %s: %v", exc.ID, err)
	}
}
