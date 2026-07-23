package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
)

type examTypeReq struct {
	AcademicYearID string `json:"academic_year_id"`
	Name           string `json:"name"`
	MappingRapor   string `json:"mapping_rapor"`
}

func (h *Handler) CreateExamType(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	var req examTypeReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.MappingRapor == "" || req.AcademicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "academic_year_id, name and mapping_rapor are required")
		return
	}
	t := &model.ExamType{
		SchoolID:       common.EffectiveSchoolID(r, c),
		AcademicYearID: req.AcademicYearID,
		Name:           req.Name,
		MappingRapor:   req.MappingRapor,
		CreatedBy:      c.UserID,
	}
	if err := h.Store.CreateExamType(r.Context(), t); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, t, nil)
}

func (h *Handler) ListExamTypes(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r, c)
	academicYearID := r.URL.Query().Get("academic_year_id")
	if academicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "academic_year_id query param is required (see README Assumptions: exam-service does not resolve 'current' year itself)")
		return
	}
	page, perPage := common.Pagination(r)
	list, total, err := h.Store.ListExamTypes(r.Context(), schoolID, academicYearID, page, perPage)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, common.Meta{Page: page, PerPage: perPage, Total: total})
}

func (h *Handler) GetExamType(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	t, err := h.Store.GetExamType(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if t == nil {
		common.WriteError(w, http.StatusNotFound, "exam type not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

func (h *Handler) UpdateExamType(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	id := chi.URLParam(r, "id")
	var req examTypeReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.Store.UpdateExamType(r.Context(), id, common.EffectiveSchoolID(r, c), req.Name, req.MappingRapor); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) DeleteExamType(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteExamType(r.Context(), id, common.EffectiveSchoolID(r, c)); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, nil, nil)
}
