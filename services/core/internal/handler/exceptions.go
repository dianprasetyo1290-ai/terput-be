package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/common"
	"skci/core/internal/jobs"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

// exceptionActor loads an item the caller may act on (retry/assign/resolve):
// auditors are read-only, other roles only touch the kinds FR-80-005 gives
// them, and payroll officers only hr items. Writes the error and returns nil
// otherwise.
func (h *Handler) exceptionActor(w http.ResponseWriter, r *http.Request) (*common.Claims, string, *model.IntegrationException) {
	claims, schoolID, ok := tenant(w, r)
	if !ok {
		return nil, "", nil
	}
	if claims.ActiveRole == common.RoleAuditor {
		common.WriteError(w, http.StatusForbidden, "auditor hanya memiliki izin baca")
		return nil, "", nil
	}
	allowed := model.ExceptionKindsFor(claims.ActiveRole)
	if len(allowed) == 0 {
		common.WriteError(w, http.StatusForbidden, "peran Anda tidak memiliki akses ke antrean galat integrasi")
		return nil, "", nil
	}
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "item antrean galat tidak ditemukan")
		return nil, "", nil
	}
	exc, err := h.Store.GetExceptionByID(r.Context(), schoolID, id)
	if err != nil {
		writeStoreErr(w, err, "item antrean galat tidak ditemukan")
		return nil, "", nil
	}
	if !slices.Contains(allowed, exc.Kind) || (claims.ActiveRole == common.RolePayrollOfficer && exc.SourceService != "hr") {
		common.WriteError(w, http.StatusForbidden, "peran Anda tidak berwenang menangani galat jenis ini")
		return nil, "", nil
	}
	return claims, schoolID, exc
}

func (h *Handler) ListExceptions(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}

	allowed := model.ExceptionKindsFor(claims.ActiveRole)
	if len(allowed) == 0 {
		common.WriteError(w, http.StatusForbidden, "peran Anda tidak memiliki akses ke antrean galat integrasi")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return
	}

	page, perPage := common.Page(r)
	kindFilter := r.URL.Query().Get("kind")
	statusFilter := r.URL.Query().Get("status")
	severityFilter := r.URL.Query().Get("severity")

	// If kindFilter is set, verify caller has permission for this kind
	if kindFilter != "" && !slices.Contains(allowed, kindFilter) {
		common.WriteError(w, http.StatusForbidden, "peran Anda tidak diizinkan melihat jenis galat ini")
		return
	}

	filter := store.ExceptionFilter{
		SchoolID: schoolID,
		Kinds:    allowed,
		Kind:     kindFilter,
		Status:   statusFilter,
		Severity: severityFilter,
	}
	if claims.ActiveRole == common.RolePayrollOfficer {
		filter.SourceService = "hr"
	}

	exceptions, total, err := h.Store.ListExceptions(r.Context(), filter, page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat daftar galat: "+err.Error())
		return
	}

	// Defense in depth: Mask sensitive payload on read (AC-80-03)
	for i := range exceptions {
		exceptions[i].PayloadSummary = model.MaskSensitive(exceptions[i].PayloadSummary)
	}

	meta := map[string]any{
		"page":     page,
		"per_page": perPage,
		"total":    total,
	}
	common.WriteJSON(w, http.StatusOK, exceptions, meta)
}

// RetryException re-runs the failed integration (FR-80-003). The item is
// claimed first (status 'retrying', BUG-167) so two concurrent clicks never
// call the downstream twice: the second gets 409.
func (h *Handler) RetryException(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, exc := h.exceptionActor(w, r)
	if exc == nil {
		return
	}
	retry, msg := h.retryAction(exc)
	if msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	lock := time.Duration(h.Config.RetryLockSeconds) * time.Second
	if err := h.Store.ClaimExceptionForRetry(r.Context(), schoolID, exc.ID, lock); err != nil {
		writeStoreErr(w, err, "item antrean galat tidak ditemukan")
		return
	}

	if err := retry(r.Context()); err != nil {
		if rerr := h.Store.ReopenException(r.Context(), schoolID, exc.ID, "retry gagal: "+err.Error()); rerr != nil {
			log.Printf("reopen exception %s: %v", exc.ID, rerr)
		}
		h.audit(r, model.AuditEvent{SchoolID: &schoolID, Module: "exception_queue", EntityType: "integration_exception",
			EntityID: exc.ID, Action: "exception_retry_failed", Reason: ptr(err.Error()), CorrelationID: &exc.CorrelationID})
		common.WriteError(w, http.StatusBadGateway, "retry gagal: "+err.Error())
		return
	}
	if err := h.Store.ResolveException(r.Context(), schoolID, exc.ID, claims.GetUserID(), "retry berhasil"); err != nil {
		log.Printf("resolve exception %s after retry: %v", exc.ID, err)
	}
	h.audit(r, model.AuditEvent{SchoolID: &schoolID, Module: "exception_queue", EntityType: "integration_exception",
		EntityID: exc.ID, Action: "exception_retried", Reason: ptr("retry berhasil downstream"), CorrelationID: &exc.CorrelationID})
	common.WriteJSON(w, http.StatusOK, map[string]any{"message": "retry berhasil", "resolved": true}, nil)
}

// retryAction picks how an item is retried: its retry_url, or for a failed
// approval callback (no retry_url) core redelivers the callback itself.
func (h *Handler) retryAction(exc *model.IntegrationException) (func(ctx context.Context) error, string) {
	if exc.RetryURL != nil && *exc.RetryURL != "" {
		return func(ctx context.Context) error { return h.Client.CallRetry(ctx, *exc.RetryURL) }, ""
	}
	if approvalID, _ := exc.PayloadSummary["approval_id"].(string); exc.Kind == model.ExceptionKindCallback && isUUID(approvalID) {
		return func(ctx context.Context) error {
			req, err := h.Store.GetRequestByID(ctx, exc.SchoolID, approvalID)
			if err != nil {
				return err
			}
			if err := jobs.SendCallback(ctx, h.Client, req); err != nil {
				return err
			}
			return h.Store.MarkCallbackDone(ctx, req.ID)
		}, ""
	}
	return nil, "item ini tidak memiliki endpoint retry yang dapat dipanggil"
}

type assignRequest struct {
	UserID string `json:"user_id"`
}

func (h *Handler) AssignException(w http.ResponseWriter, r *http.Request) {
	_, schoolID, exc := h.exceptionActor(w, r)
	if exc == nil {
		return
	}
	var req assignRequest
	if !common.Decode(w, r, &req) {
		return
	}
	if !isUUID(strings.TrimSpace(req.UserID)) {
		common.WriteError(w, http.StatusBadRequest, "user_id wajib diisi (UUID)")
		return
	}
	if err := h.Store.AssignException(r.Context(), schoolID, exc.ID, req.UserID); err != nil {
		writeStoreErr(w, err, "item antrean galat tidak ditemukan")
		return
	}
	h.audit(r, model.AuditEvent{SchoolID: &schoolID, Module: "exception_queue", EntityType: "integration_exception",
		EntityID: exc.ID, Action: "exception_assigned", Reason: ptr(fmt.Sprintf("ditugaskan ke %s", req.UserID)),
		BeforeData: map[string]any{"assignee_user_id": exc.AssigneeUserID}, AfterData: map[string]any{"assignee_user_id": req.UserID},
		CorrelationID: &exc.CorrelationID})
	h.bg(func(ctx context.Context) { jobs.NotifyAssignee(ctx, h.Client, exc, req.UserID) })
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "item berhasil ditugaskan"}, nil)
}

type resolveRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) ResolveException(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, exc := h.exceptionActor(w, r)
	if exc == nil {
		return
	}
	var req resolveRequest
	if !common.Decode(w, r, &req) {
		return
	}
	// Mandatory reason check (AC-80-02)
	if strings.TrimSpace(req.Reason) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "alasan penyelesaian wajib diisi")
		return
	}
	if err := h.Store.ResolveException(r.Context(), schoolID, exc.ID, claims.GetUserID(), req.Reason); err != nil {
		writeStoreErr(w, err, "item antrean galat tidak ditemukan")
		return
	}
	h.audit(r, model.AuditEvent{SchoolID: &schoolID, Module: "exception_queue", EntityType: "integration_exception",
		EntityID: exc.ID, Action: "exception_resolved", Reason: ptr(req.Reason),
		BeforeData: map[string]any{"status": exc.Status}, AfterData: map[string]any{"status": model.ExceptionStatusResolved},
		CorrelationID: &exc.CorrelationID})
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "item berhasil ditandai selesai"}, nil)
}
