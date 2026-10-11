package handler

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/common"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

func (h *Handler) allowedExceptionKinds(role string) []string {
	switch role {
	case common.RoleSuperadmin, common.RoleAdmin, common.RoleAuditor:
		return []string{
			model.ExceptionKindWebhook,
			model.ExceptionKindJournalPosting,
			model.ExceptionKindNotification,
			model.ExceptionKindDeadLetter,
			model.ExceptionKindImport,
			model.ExceptionKindCallback,
		}
	case common.RoleFinanceManager:
		return []string{
			model.ExceptionKindWebhook,
			model.ExceptionKindJournalPosting,
		}
	case common.RolePayrollOfficer:
		return []string{
			model.ExceptionKindJournalPosting,
		}
	default:
		return nil
	}
}

func (h *Handler) ListExceptions(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}

	allowed := h.allowedExceptionKinds(claims.ActiveRole)
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

func (h *Handler) RetryException(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}

	if claims.ActiveRole == common.RoleAuditor {
		common.WriteError(w, http.StatusForbidden, "auditor hanya memiliki izin baca")
		return
	}
	allowed := h.allowedExceptionKinds(claims.ActiveRole)
	if len(allowed) == 0 {
		common.WriteError(w, http.StatusForbidden, "akses ditolak")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	exc, err := h.Store.GetExceptionByID(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "item antrean galat tidak ditemukan")
		return
	}

	if !slices.Contains(allowed, exc.Kind) {
		common.WriteError(w, http.StatusForbidden, "peran Anda tidak berwenang mencoba ulang galat jenis ini")
		return
	}

	if exc.RetryURL == nil || *exc.RetryURL == "" {
		common.WriteError(w, http.StatusBadRequest, "item ini tidak memiliki endpoint retry yang dapat dipanggil")
		return
	}

	// Call retry downstream
	err = h.Client.CallRetry(r.Context(), *exc.RetryURL)
	if err != nil {
		_ = h.Store.IncrementExceptionAttempt(r.Context(), schoolID, id)
		common.WriteError(w, http.StatusBadGateway, "retry gagal: "+err.Error())
		return
	}

	// Retry succeeded: mark resolved
	_ = h.Store.ResolveException(r.Context(), schoolID, id, claims.GetUserID(), "retry berhasil")

	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID:    &schoolID,
		Service:     "core",
		Module:      "exception_queue",
		EntityType:  "integration_exception",
		EntityID:    id,
		Action:      "exception_retried",
		ActorUserID: ptr(claims.GetUserID()),
		ActorRole:   ptr(claims.ActiveRole),
		Reason:      ptr("retry berhasil downstream"),
		IPHash:      HashIP(ClientIP(r)),
	})

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"message":  "retry berhasil",
		"resolved": true,
	}, nil)
}

type assignRequest struct {
	UserID string `json:"user_id"`
}

func (h *Handler) AssignException(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}
	if claims.ActiveRole == common.RoleAuditor {
		common.WriteError(w, http.StatusForbidden, "auditor hanya memiliki izin baca")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var req assignRequest
	if !common.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		common.WriteError(w, http.StatusBadRequest, "user_id wajib diisi")
		return
	}

	if err := h.Store.AssignException(r.Context(), schoolID, id, req.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menugaskan item: "+err.Error())
		return
	}

	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID:    &schoolID,
		Service:     "core",
		Module:      "exception_queue",
		EntityType:  "integration_exception",
		EntityID:    id,
		Action:      "exception_assigned",
		ActorUserID: ptr(claims.GetUserID()),
		ActorRole:   ptr(claims.ActiveRole),
		Reason:      ptr(fmt.Sprintf("ditugaskan ke %s", req.UserID)),
		IPHash:      HashIP(ClientIP(r)),
	})

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "item berhasil ditugaskan"}, nil)
}

type resolveRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) ResolveException(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}
	if claims.ActiveRole == common.RoleAuditor {
		common.WriteError(w, http.StatusForbidden, "auditor hanya memiliki izin baca")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var req resolveRequest
	if !common.Decode(w, r, &req) {
		return
	}

	// Mandatory reason check (AC-80-02)
	if strings.TrimSpace(req.Reason) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "alasan penyelesaian wajib diisi")
		return
	}

	if err := h.Store.ResolveException(r.Context(), schoolID, id, claims.GetUserID(), req.Reason); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menyelesaikan item: "+err.Error())
		return
	}

	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID:    &schoolID,
		Service:     "core",
		Module:      "exception_queue",
		EntityType:  "integration_exception",
		EntityID:    id,
		Action:      "exception_resolved",
		ActorUserID: ptr(claims.GetUserID()),
		ActorRole:   ptr(claims.ActiveRole),
		Reason:      ptr(req.Reason),
		IPHash:      HashIP(ClientIP(r)),
	})

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "item berhasil ditandai selesai"}, nil)
}
