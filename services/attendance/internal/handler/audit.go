package handler

import (
	"context"
	"log"
	"time"

	"skci/attendance/internal/client"
)

// RunAuditForwarder drains audit_outbox into core's POST /internal/audit
// every interval until ctx is done. Rows stay in the outbox (attempts++)
// when core is unreachable, so nothing is lost; forwarding is in id order.
func (h *Handler) RunAuditForwarder(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		h.ForwardAuditOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ForwardAuditOnce forwards up to one batch; returns how many were accepted.
func (h *Handler) ForwardAuditOnce(ctx context.Context) int {
	if h.Store == nil || h.Client == nil {
		return 0
	}
	rows, err := h.Store.PendingAudit(ctx, 100)
	if err != nil {
		log.Printf("audit forwarder: %v", err)
		return 0
	}
	n := 0
	for _, e := range rows {
		school := e.SchoolID
		err := h.Client.PostAudit(ctx, client.AuditEvent{
			SchoolID: school, Module: e.Module, EntityType: e.EntityType, EntityID: e.EntityID,
			Action: e.Action, ActorUserID: e.ActorUserID, ActorRole: e.ActorRole,
			Before: e.BeforeRaw, After: e.AfterRaw, Reason: e.Reason, CorrelationID: e.CorrelationID,
		})
		if err != nil {
			_ = h.Store.MarkAuditAttempt(ctx, e.ID)
			log.Printf("audit forwarder: row %d: %v", e.ID, err)
			return n // keep order; retry next tick
		}
		if err := h.Store.MarkAuditForwarded(ctx, e.ID); err != nil {
			log.Printf("audit forwarder: mark %d: %v", e.ID, err)
			return n
		}
		n++
	}
	return n
}
