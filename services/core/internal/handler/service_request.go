package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/common"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

type createCategoryRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	HandlerRole     string `json:"handler_role"`
	SLAHours        *int   `json:"sla_hours,omitempty"`
	RequiresStudent bool   `json:"requires_student"`
	Active          bool   `json:"active"`
}

type createServiceRequestInput struct {
	CategoryID    string          `json:"category_id"`
	StudentUserID *string         `json:"student_user_id,omitempty"`
	Subject       string          `json:"subject"`
	Description   string          `json:"description"`
	Attachments   json.RawMessage `json:"attachments,omitempty"`
}

type updateStatusInput struct {
	To   string `json:"to"`
	Note string `json:"note"`
}

type updateResultInput struct {
	ResultAttachments json.RawMessage `json:"result_attachments"`
}

// -----------------------------------------------------------------------------
// Category Handlers (Admin)
// -----------------------------------------------------------------------------

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return
	}

	activeOnly := r.URL.Query().Get("active") == "true"
	list, err := h.Store.ListServiceRequestCategories(r.Context(), schoolID, activeOnly)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat kategori layanan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	cat, err := h.Store.GetServiceRequestCategory(r.Context(), schoolID, id)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "kategori layanan tidak ditemukan")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat kategori: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, cat, nil)
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || (!claims.HasRole(common.RoleAdmin) && !claims.HasRole(common.RoleSuperadmin)) {
		common.WriteError(w, http.StatusForbidden, "hanya admin yang dapat mengelola kategori layanan")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	var req createCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.HandlerRole) == "" {
		common.WriteError(w, http.StatusBadRequest, "nama dan handler_role wajib diisi")
		return
	}

	cat := model.ServiceRequestCategory{
		SchoolID:        schoolID,
		Name:            strings.TrimSpace(req.Name),
		Description:     strings.TrimSpace(req.Description),
		HandlerRole:     strings.TrimSpace(req.HandlerRole),
		SLAHours:        req.SLAHours,
		RequiresStudent: req.RequiresStudent,
		Active:          req.Active,
	}

	created, err := h.Store.CreateServiceRequestCategory(r.Context(), cat)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat kategori: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || (!claims.HasRole(common.RoleAdmin) && !claims.HasRole(common.RoleSuperadmin)) {
		common.WriteError(w, http.StatusForbidden, "hanya admin yang dapat mengelola kategori layanan")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var req createCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}

	cat := model.ServiceRequestCategory{
		Name:            strings.TrimSpace(req.Name),
		Description:     strings.TrimSpace(req.Description),
		HandlerRole:     strings.TrimSpace(req.HandlerRole),
		SLAHours:        req.SLAHours,
		RequiresStudent: req.RequiresStudent,
		Active:          req.Active,
	}

	updated, err := h.Store.UpdateServiceRequestCategory(r.Context(), schoolID, id, cat)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "kategori tidak ditemukan")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memperbarui kategori: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || (!claims.HasRole(common.RoleAdmin) && !claims.HasRole(common.RoleSuperadmin)) {
		common.WriteError(w, http.StatusForbidden, "hanya admin yang dapat mengelola kategori layanan")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := h.Store.DeleteServiceRequestCategory(r.Context(), schoolID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "kategori tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "gagal menghapus kategori: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "kategori berhasil dihapus"}, nil)
}

// -----------------------------------------------------------------------------
// Parent Service Requests Handlers
// -----------------------------------------------------------------------------

func (h *Handler) CreateParentServiceRequest(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	var req createServiceRequestInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}

	if strings.TrimSpace(req.Subject) == "" || strings.TrimSpace(req.Description) == "" || strings.TrimSpace(req.CategoryID) == "" {
		common.WriteError(w, http.StatusBadRequest, "kategori, subjek, dan deskripsi wajib diisi")
		return
	}

	cat, err := h.Store.GetServiceRequestCategory(r.Context(), schoolID, req.CategoryID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusBadRequest, "kategori layanan tidak valid")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memvalidasi kategori: "+err.Error())
		return
	}

	if !cat.Active {
		common.WriteError(w, http.StatusBadRequest, "kategori layanan sedang non-aktif")
		return
	}

	if cat.RequiresStudent && (req.StudentUserID == nil || strings.TrimSpace(*req.StudentUserID) == "") {
		common.WriteError(w, http.StatusBadRequest, "siswa wajib dipilih untuk kategori layanan ini")
		return
	}

	sr := model.ServiceRequest{
		SchoolID:        schoolID,
		CategoryID:      req.CategoryID,
		RequesterUserID: claims.GetUserID(),
		StudentUserID:   req.StudentUserID,
		Subject:         strings.TrimSpace(req.Subject),
		Description:     strings.TrimSpace(req.Description),
		Attachments:     req.Attachments,
	}

	created, err := h.Store.CreateServiceRequest(r.Context(), sr)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat permintaan layanan: "+err.Error())
		return
	}
	if h.ServiceDesk != nil {
		h.ServiceDesk.FromServiceRequest(r.Context(), *created)
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) ListParentServiceRequests(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListParentServiceRequests(r.Context(), schoolID, claims.GetUserID())
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat daftar permintaan layanan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetServiceRequestDetail(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	sr, err := h.Store.GetServiceRequest(r.Context(), schoolID, id)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "permintaan layanan tidak ditemukan")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat permintaan layanan: "+err.Error())
		return
	}

	// Verify authorization: requester or handler/admin
	isRequester := sr.RequesterUserID == claims.GetUserID()
	isHandler := claims.HasRole(common.RoleAdmin) || claims.HasRole(common.RoleSuperadmin) || claims.HasRole(sr.HandlerRole)
	if !isRequester && !isHandler {
		common.WriteError(w, http.StatusForbidden, "tidak memiliki hak akses ke permintaan layanan ini")
		return
	}

	events, err := h.Store.ListServiceRequestEvents(r.Context(), schoolID, id)
	if err != nil {
		events = []model.ServiceRequestEvent{}
	}

	resp := map[string]interface{}{
		"request": sr,
		"events":  events,
	}
	common.WriteJSON(w, http.StatusOK, resp, nil)
}

// -----------------------------------------------------------------------------
// Handler Workspace Handlers
// -----------------------------------------------------------------------------

func (h *Handler) ListHandlerServiceRequests(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")

	roles := claims.Roles
	if len(roles) == 0 && claims.ActiveRole != "" {
		roles = []string{claims.ActiveRole}
	}

	list, err := h.Store.ListHandlerServiceRequests(r.Context(), schoolID, roles, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat permintaan layanan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) UpdateServiceRequestStatus(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if h.serviceDeskGuard(w, r, schoolID, id) {
		return
	}

	var input updateStatusInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}

	updated, err := h.Store.UpdateServiceRequestStatus(
		r.Context(), schoolID, id, input.To, input.Note, claims.GetUserID(),
	)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "permintaan layanan tidak ditemukan")
		return
	} else if errors.Is(err, model.ErrInvalidStatusTransition) || errors.Is(err, model.ErrRejectionNoteRequired) {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memperbarui status: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, updated, nil)
}

func (h *Handler) UpdateServiceRequestResult(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if h.serviceDeskGuard(w, r, schoolID, id) {
		return
	}

	var input updateResultInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}

	updated, err := h.Store.UpdateServiceRequestResult(r.Context(), schoolID, id, input.ResultAttachments)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "permintaan layanan tidak ditemukan")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memperbarui lampiran hasil: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, updated, nil)
}

func (h *Handler) CanUserHandle(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	roles := claims.Roles
	if len(roles) == 0 && claims.ActiveRole != "" {
		roles = []string{claims.ActiveRole}
	}

	if claims.HasRole(common.RoleAdmin) || claims.HasRole(common.RoleSuperadmin) {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"can_handle": true,
			"roles":      roles,
		}, nil)
		return
	}

	handled, err := h.Store.CanUserHandleServiceRequests(r.Context(), schoolID, roles)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memeriksa peran: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"can_handle": len(handled) > 0,
		"roles":      handled,
	}, nil)
}
