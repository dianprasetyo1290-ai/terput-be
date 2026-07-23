package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"skci/evaluation/internal/client"
	"skci/evaluation/internal/common"
	"skci/evaluation/internal/domain"
	"skci/evaluation/internal/model"
)

type periodRequest struct {
	Title                  string   `json:"title"`
	PublishStartDate       string   `json:"publish_start_date"`
	PublishEndDate         string   `json:"publish_end_date"`
	AcademicYearID         string   `json:"academic_year_id"`
	Semester               string   `json:"semester"`
	HeadmasterUserID       *string  `json:"headmaster_user_id"`
	CurriculumVPUserID     *string  `json:"curriculum_vp_user_id"`
	StudentAffairsVPUserID *string  `json:"student_affairs_vp_user_id"`
	TargetTeacherIDs       []string `json:"target_teacher_ids"`
	PeerEvaluateeIDs       []string `json:"peer_evaluatee_ids"`
	PeerEvaluatorIDs       []string `json:"peer_evaluator_ids"`
	StudentClassroomIDs    []string `json:"student_classroom_ids"`
	LeadAssessorName       string   `json:"lead_assessor_name"`
}

func attachStatus(p *model.Period) {
	p.Status = domain.PeriodStatus(time.Now(), p.Published, p.PublishStartDate, p.PublishEndDate)
}

// validatePeerCrossFilter enforces the CRITICAL business rule: a guru placed
// in "guru yang ingin dinilai" (K-a) cannot also appear in "yang menilai"
// (K-b) for the same period.
func validatePeerCrossFilter(evaluatees, evaluators []string) error {
	set := map[string]bool{}
	for _, id := range evaluatees {
		set[id] = true
	}
	var offenders []string
	for _, id := range evaluators {
		if set[id] {
			offenders = append(offenders, id)
		}
	}
	if len(offenders) > 0 {
		return fmt.Errorf("guru %v are in 'guru yang ingin dinilai' and cannot also be in 'yang menilai' for the same period", offenders)
	}
	return nil
}

// POST /periods - admin creates a period (status starts as Draft: published=false).
func (h *Handler) CreatePeriod(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "admin") && !hasRole(claims, "superadmin") {
		common.WriteError(w, http.StatusForbidden, "admin only")
		return
	}
	var req periodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Title == "" || req.PublishStartDate == "" || req.PublishEndDate == "" || req.AcademicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "title, publish_start_date, publish_end_date, academic_year_id are required")
		return
	}
	if err := validatePeerCrossFilter(req.PeerEvaluateeIDs, req.PeerEvaluatorIDs); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	p := &model.Period{
		SchoolID: schoolID, AcademicYearID: req.AcademicYearID, Title: req.Title, Semester: req.Semester,
		PublishStartDate: req.PublishStartDate, PublishEndDate: req.PublishEndDate, Published: false,
		HeadmasterUserID: req.HeadmasterUserID, CurriculumVPUserID: req.CurriculumVPUserID, StudentAffairsVPUserID: req.StudentAffairsVPUserID,
		TargetTeacherIDs: req.TargetTeacherIDs, PeerEvaluateeIDs: req.PeerEvaluateeIDs, PeerEvaluatorIDs: req.PeerEvaluatorIDs,
		StudentClassroomIDs: req.StudentClassroomIDs, LeadAssessorName: req.LeadAssessorName, CreatedBy: &claims.UserID,
	}
	if err := h.Periods.Create(r.Context(), p); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create period: "+err.Error())
		return
	}
	if err := h.regenerateTasks(r.Context(), p); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "period created but task generation failed: "+err.Error())
		return
	}
	attachStatus(p)
	common.WriteJSON(w, http.StatusCreated, p, nil)
}

// PUT /periods/{id} - normal update, modifies config without losing prior settings.
func (h *Handler) UpdatePeriod(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "admin") && !hasRole(claims, "superadmin") {
		common.WriteError(w, http.StatusForbidden, "admin only")
		return
	}
	id := chi.URLParam(r, "id")
	schoolID := effectiveSchoolID(r, claims)
	existing, err := h.Periods.GetByID(r.Context(), id, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	var req periodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validatePeerCrossFilter(req.PeerEvaluateeIDs, req.PeerEvaluatorIDs); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing.Title, existing.Semester = req.Title, req.Semester
	existing.PublishStartDate, existing.PublishEndDate = req.PublishStartDate, req.PublishEndDate
	existing.HeadmasterUserID, existing.CurriculumVPUserID, existing.StudentAffairsVPUserID = req.HeadmasterUserID, req.CurriculumVPUserID, req.StudentAffairsVPUserID
	existing.TargetTeacherIDs, existing.PeerEvaluateeIDs, existing.PeerEvaluatorIDs = req.TargetTeacherIDs, req.PeerEvaluateeIDs, req.PeerEvaluatorIDs
	existing.StudentClassroomIDs, existing.LeadAssessorName = req.StudentClassroomIDs, req.LeadAssessorName
	existing.UpdatedBy = &claims.UserID

	updated, err := h.Periods.Update(r.Context(), id, schoolID, existing)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update period: "+err.Error())
		return
	}
	if err := h.regenerateTasks(r.Context(), updated); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "period updated but task generation failed: "+err.Error())
		return
	}
	attachStatus(updated)
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// GET /periods/{id}
func (h *Handler) GetPeriod(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	schoolID := effectiveSchoolID(r, claims)
	p, err := h.Periods.GetByID(r.Context(), id, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	attachStatus(p)
	common.WriteJSON(w, http.StatusOK, p, nil)
}

// GET /periods - list header: Periode | Tanggal Awal | Tanggal Akhir | Status | Aksi
func (h *Handler) ListPeriods(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := effectiveSchoolID(r, claims)
	page, perPage := pagination(r)
	var ayID *string
	if v := r.URL.Query().Get("academic_year_id"); v != "" {
		ayID = &v
	}
	list, total, err := h.Periods.List(r.Context(), schoolID, ayID, page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list periods: "+err.Error())
		return
	}
	statusFilter := r.URL.Query().Get("status")
	out := list[:0]
	for _, p := range list {
		attachStatus(p)
		if statusFilter != "" && p.Status != statusFilter {
			continue
		}
		out = append(out, p)
	}
	common.WriteJSON(w, http.StatusOK, out, map[string]interface{}{"page": page, "per_page": perPage, "total": total})
}

// GET /periods/{id}/confirm-delete - returns the confirm-dialog payload; frontend renders it.
func (h *Handler) ConfirmDeletePeriod(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	schoolID := effectiveSchoolID(r, claims)
	p, err := h.Periods.GetByID(r.Context(), id, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"period_id": p.ID,
		"title":     p.Title,
		"message":   fmt.Sprintf("apakah anda yakin ingin menghapus %s", p.Title),
	}, nil)
}

// DELETE /periods/{id} - performs the actual deletion (frontend calls confirm-delete first).
func (h *Handler) DeletePeriod(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "admin") && !hasRole(claims, "superadmin") {
		common.WriteError(w, http.StatusForbidden, "admin only")
		return
	}
	id := chi.URLParam(r, "id")
	schoolID := effectiveSchoolID(r, claims)
	ok, err := h.Periods.Delete(r.Context(), id, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete period: "+err.Error())
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) setPublished(w http.ResponseWriter, r *http.Request, published bool) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "admin") && !hasRole(claims, "superadmin") {
		common.WriteError(w, http.StatusForbidden, "admin only")
		return
	}
	id := chi.URLParam(r, "id")
	schoolID := effectiveSchoolID(r, claims)
	p, err := h.Periods.SetPublished(r.Context(), id, schoolID, published)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	attachStatus(p)
	common.WriteJSON(w, http.StatusOK, p, nil)
}

// POST /periods/{id}/publish
func (h *Handler) PublishPeriod(w http.ResponseWriter, r *http.Request) { h.setPublished(w, r, true) }

// POST /periods/{id}/unpublish
func (h *Handler) UnpublishPeriod(w http.ResponseWriter, r *http.Request) {
	h.setPublished(w, r, false)
}

// GET /periods/{id}/available-peer-evaluators
// Excludes anyone already placed in the period's "guru yang ingin dinilai" (K-a)
// list from the selectable pool for "yang menilai" (K-b).
func (h *Handler) AvailablePeerEvaluators(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	schoolID := effectiveSchoolID(r, claims)
	p, err := h.Periods.GetByID(r.Context(), id, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	pool := h.Clients.GetGuruList(schoolID)
	source := "identity-service"
	if len(pool) == 0 {
		source = "fallback: period.target_teacher_ids (identity-service unreachable or returned empty)"
		for _, id := range p.TargetTeacherIDs {
			pool = append(pool, client.UserSummary{ID: id, Name: "(name via identity-service, unavailable)"})
		}
	}
	excluded := map[string]bool{}
	for _, id := range p.PeerEvaluateeIDs {
		excluded[id] = true
	}
	available := make([]client.UserSummary, 0, len(pool))
	for _, u := range pool {
		if !excluded[u.ID] {
			available = append(available, u)
		}
	}
	common.WriteJSON(w, http.StatusOK, available, map[string]interface{}{"source": source})
}

// regenerateTasks (re)creates pending tasks for the eagerly-known evaluator
// categories (kepala_sekolah, waka_kurikulum, waka_kesiswaan, self, peer).
// Siswa tasks are created lazily on first submission -- see evaluation.go --
// because the student roster per classroom is owned by academic-service and
// this call must stay idempotent/cheap on every period save. See README
// Assumptions for the full rationale.
func (h *Handler) regenerateTasks(ctx context.Context, p *model.Period) error {
	if err := h.Forms.BootstrapDefaults(ctx, p.SchoolID); err != nil {
		return err
	}
	forms, err := h.Forms.ListForms(ctx, p.SchoolID)
	if err != nil {
		return err
	}
	byType := map[string]*model.EvalForm{}
	for _, f := range forms {
		byType[f.EvaluatorType] = f
	}

	upsert := func(formType, target, evaluator string) error {
		f, ok := byType[formType]
		if !ok {
			return nil
		}
		_, err := h.Tasks.UpsertTask(ctx, &model.EvalTask{
			PeriodID: p.ID, FormID: f.ID, SchoolID: p.SchoolID, AcademicYearID: p.AcademicYearID,
			TargetTeacherUserID: target, EvaluatorUserID: evaluator, EvaluatorType: formType,
		})
		return err
	}

	for _, guruID := range p.TargetTeacherIDs {
		if p.HeadmasterUserID != nil {
			if err := upsert(model.EvalTypeKepalaSekolah, guruID, *p.HeadmasterUserID); err != nil {
				return err
			}
		}
		if p.CurriculumVPUserID != nil {
			if err := upsert(model.EvalTypeWakaKurikulum, guruID, *p.CurriculumVPUserID); err != nil {
				return err
			}
		}
		if p.StudentAffairsVPUserID != nil {
			if err := upsert(model.EvalTypeWakaKesiswaan, guruID, *p.StudentAffairsVPUserID); err != nil {
				return err
			}
		}
		if err := upsert(model.EvalTypeSelf, guruID, guruID); err != nil {
			return err
		}
	}
	for _, evaluatorID := range p.PeerEvaluatorIDs {
		for _, evaluateeID := range p.PeerEvaluateeIDs {
			if evaluatorID == evaluateeID {
				continue
			}
			if err := upsert(model.EvalTypePeer, evaluateeID, evaluatorID); err != nil {
				return err
			}
		}
	}
	return nil
}

// GET /forms - list the 6 evaluator-category forms + items for the school (bootstraps defaults if missing).
func (h *Handler) ListForms(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := effectiveSchoolID(r, claims)
	if err := h.Forms.BootstrapDefaults(r.Context(), schoolID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to bootstrap forms: "+err.Error())
		return
	}
	forms, err := h.Forms.ListForms(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list forms: "+err.Error())
		return
	}
	type formWithItems struct {
		*model.EvalForm
		Items []*model.EvalItem `json:"items"`
	}
	out := make([]formWithItems, 0, len(forms))
	for _, f := range forms {
		items, err := h.Forms.ListItems(r.Context(), f.ID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list items: "+err.Error())
			return
		}
		out = append(out, formWithItems{EvalForm: f, Items: items})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
