package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"skci/billing/internal/client"
)

// submitApproval sends a maker-checker request to core's Approval Center.
// Nothing is ever auto-approved: without core, or when core refuses (no flow
// configured, maker not allowed, ...), the caller gets an error to surface.
func (a *App) submitApproval(ctx context.Context, req client.ApprovalRequest) (client.ApprovalResponse, error) {
	if a.Core == nil || !a.Core.Configured() || a.Config.Gateway.BillingInternalURL == "" {
		return client.ApprovalResponse{}, &httpError{http.StatusServiceUnavailable,
			"Approval Center belum dikonfigurasi (CORE_URL / BILLING_INTERNAL_URL); pengajuan tidak disimpan"}
	}
	resp, err := a.Core.RequestApproval(ctx, req)
	if err == nil {
		return resp, nil
	}
	log.Printf("approval %s %s: %v", req.RequestType, req.SourceID, err)
	var he *client.HTTPError
	if errors.As(err, &he) && he.Status >= 400 && he.Status < 500 {
		return resp, &httpError{http.StatusUnprocessableEntity, "Approval Center menolak pengajuan: " + coreMessage(he.Body) +
			". Periksa menu Alur Approval untuk jenis " + req.RequestType + "."}
	}
	return resp, &httpError{http.StatusBadGateway, "Approval Center tidak dapat dihubungi; pengajuan tidak disimpan"}
}

// coreMessage extracts the envelope message from a core error body.
func coreMessage(body string) string {
	var env struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &env) == nil && strings.TrimSpace(env.Message) != "" {
		return env.Message
	}
	return strings.TrimSpace(body)
}

// auditBoth writes the local billing audit event and mirrors it to core's
// central Audit Trail (best effort: the local row is the record of truth).
func (a *App) auditBoth(ctx context.Context, schoolID, module, entityType, entityID, action, actorRole, actorID, reason string, before, after any) {
	a.audit(ctx, schoolID, entityType, entityID, action, actorRole, actorID, "", reason, before, after)
	if a.Core != nil && a.Core.Configured() {
		if err := a.Core.Audit(ctx, client.AuditEvent{
			SchoolID: schoolID, Module: module, EntityType: entityType, EntityID: entityID, Action: action,
			ActorUserID: actorID, ActorRole: actorRole, Before: before, After: after, Reason: reason,
		}); err != nil {
			log.Printf("central audit %s %s: %v", action, entityID, err)
		}
	}
}
