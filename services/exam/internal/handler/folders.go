package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
)

type folderReq struct {
	AcademicYearID string  `json:"academic_year_id"`
	Name           string  `json:"name"`
	Description    *string `json:"description"`
}

func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	var req folderReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.AcademicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "academic_year_id and name are required")
		return
	}
	f := &model.QuestionFolder{
		SchoolID:       common.EffectiveSchoolID(r, c),
		AcademicYearID: req.AcademicYearID,
		Name:           req.Name,
		Description:    req.Description,
		CreatedBy:      c.UserID,
	}
	if err := h.Store.CreateFolder(r.Context(), f); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, f, nil)
}

func (h *Handler) ListFolders(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	academicYearID := r.URL.Query().Get("academic_year_id")
	if academicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "academic_year_id query param is required")
		return
	}
	page, perPage := common.Pagination(r)
	list, total, err := h.Store.ListFolders(r.Context(), common.EffectiveSchoolID(r, c), academicYearID, page, perPage)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, common.Meta{Page: page, PerPage: perPage, Total: total})
}

func (h *Handler) GetFolder(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	f, err := h.Store.GetFolder(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if f == nil {
		common.WriteError(w, http.StatusNotFound, "folder not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, f, nil)
}

func (h *Handler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	id := chi.URLParam(r, "id")
	var req folderReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.Store.UpdateFolder(r.Context(), id, common.EffectiveSchoolID(r, c), req.Name, req.Description); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteFolder(r.Context(), id, common.EffectiveSchoolID(r, c)); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, nil, nil)
}
