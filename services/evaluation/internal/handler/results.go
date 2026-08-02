package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"skci/evaluation/internal/common"
	"skci/evaluation/internal/domain"
	"skci/evaluation/internal/model"
)

type categoryResult struct {
	FormCode             string   `json:"form_code"`
	Priority             int      `json:"priority"`
	FormTitle            string   `json:"form_title"`
	EvaluatorType        string   `json:"evaluator_type"`
	Weight               float64  `json:"weight"`
	SubmittedCount       int      `json:"submitted_count"`
	ExpectedCount        *int     `json:"expected_count"` // null when unknown (e.g. siswa roster unreachable)
	ExpectedNote         string   `json:"expected_note,omitempty"`
	CategoryAvgScore     *float64 `json:"category_avg_score"` // avg of submitted tasks' raw_score (1-4), null if none submitted
	WeightedContribution float64  `json:"weighted_contribution"`
	// Category is the same 4-tier label as PerformanceCategory (RINGKASAN
	// sheet's per-row "Kategori / Category" column), computed from this row's
	// own avg score rather than the overall final score. Empty until at
	// least one evaluator in this category has submitted.
	Category string `json:"category"`
	Complete bool   `json:"complete"`
}

type periodResults struct {
	PeriodID            string           `json:"period_id"`
	PeriodTitle         string           `json:"period_title"`
	TargetTeacherID     string           `json:"target_teacher_id"`
	Categories          []categoryResult `json:"categories"`
	FinalScore          float64          `json:"final_score"`
	IsComplete          bool             `json:"is_complete"`
	PromotifLabel       string           `json:"promotif_label"`
	PerformanceCategory string           `json:"performance_category"`
}

// computeResults is the shared scoring engine used by both /results and /report.
func (h *Handler) computeResults(ctx context.Context, p *model.Period, targetTeacherID string) (*periodResults, error) {
	forms, err := h.Forms.ListForms(ctx, p.SchoolID)
	if err != nil {
		return nil, err
	}
	tasks, err := h.Tasks.ListForPeriodTarget(ctx, p.ID, targetTeacherID)
	if err != nil {
		return nil, err
	}
	tasksByForm := map[string][]*model.EvalTask{}
	for _, t := range tasks {
		tasksByForm[t.FormID] = append(tasksByForm[t.FormID], t)
	}

	res := &periodResults{PeriodID: p.ID, PeriodTitle: p.Title, TargetTeacherID: targetTeacherID}
	overallComplete := true

	for _, f := range forms {
		if model.IsUpwardEvalType(f.EvaluatorType) {
			// Upward-feedback track (guru rates a leader) is a separate,
			// unblended track -- see leadership-results below -- and must
			// not appear in a guru's own weighted-score categories.
			continue
		}
		formTasks := tasksByForm[f.ID]
		submitted := 0
		var sum float64
		for _, t := range formTasks {
			if t.Status == model.TaskStatusSubmitted {
				submitted++
				if t.RawScore != nil {
					sum += *t.RawScore
				}
			}
		}

		expected, note := h.expectedCountForCategory(p, f, len(formTasks))
		cr := categoryResult{
			FormCode: f.Code, Priority: f.Priority, FormTitle: f.Title, EvaluatorType: f.EvaluatorType, Weight: f.Weight,
			SubmittedCount: submitted, ExpectedCount: expected, ExpectedNote: note,
		}
		if submitted > 0 {
			avg := sum / float64(submitted)
			cr.CategoryAvgScore = &avg
			cr.WeightedContribution = avg * f.Weight
			cr.Category = domain.PerformanceCategory(avg)
			res.FinalScore += cr.WeightedContribution
		}
		if expected != nil {
			cr.Complete = submitted >= *expected && *expected > 0
			if *expected > 0 && submitted < *expected {
				overallComplete = false
			}
		} else if note != "" {
			// unknown expected count (e.g. siswa roster unreachable) -> can't
			// assert completeness for this category
			overallComplete = false
		}
		res.Categories = append(res.Categories, cr)
	}

	res.IsComplete = overallComplete
	res.PromotifLabel = domain.FinalScoreLabel(res.FinalScore)
	res.PerformanceCategory = domain.PerformanceCategory(res.FinalScore)
	return res, nil
}

// expectedCountForCategory figures out how many evaluators are expected to
// submit for a given form/category. kepala_sekolah/waka_kurikulum/
// waka_kesiswaan/self/peer are eagerly generated so len(existing tasks) IS
// the expected count. siswa tasks are lazy (see period.go regenerateTasks
// doc comment), so expected count needs a best-effort roster lookup from
// academic-service; if that's unreachable we report expected=nil with a note
// rather than guessing.
func (h *Handler) expectedCountForCategory(p *model.Period, f *model.EvalForm, existingTaskCount int) (*int, string) {
	if f.EvaluatorType != model.EvalTypeSiswa {
		n := existingTaskCount
		return &n, ""
	}
	if len(p.StudentClassroomIDs) == 0 {
		zero := 0
		return &zero, ""
	}
	total := 0
	reachable := false
	for _, classroomID := range p.StudentClassroomIDs {
		students := h.Clients.GetClassroomStudents(classroomID)
		if students != nil {
			reachable = true
		}
		total += len(students)
	}
	if !reachable {
		return nil, "expected siswa count unknown: academic-service classroom roster unreachable"
	}
	return &total, ""
}

// GET /periods/{id}/results?guru_id=
func (h *Handler) PeriodResults(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	periodID := chi.URLParam(r, "id")
	schoolID := effectiveSchoolID(r, claims)
	guruID := r.URL.Query().Get("guru_id")
	if guruID == "" {
		common.WriteError(w, http.StatusBadRequest, "guru_id query param is required")
		return
	}
	p, err := h.Periods.GetByID(r.Context(), periodID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	res, err := h.computeResults(r.Context(), p, guruID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to compute results: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, res, nil)
}

type reportPayload struct {
	Title               string           `json:"title"`
	NamaGuru            string           `json:"nama_guru"`
	NUPTK               string           `json:"nuptk"`
	TahunAjaran         string           `json:"tahun_ajaran"` // resolved label (e.g. "2025/2026"), not the raw academic_year_id
	Semester            string           `json:"semester"`
	PeriodeEvaluasi     string           `json:"periode_evaluasi"` // publish_start_date - publish_end_date, formatted
	MataPelajaran       []string         `json:"mata_pelajaran"`
	KelasYangDiampu     []string         `json:"kelas_yang_diampu"`
	LeadAssessor        string           `json:"penilaian_utama"`
	Categories          []categoryResult `json:"categories"`
	FinalScore          float64          `json:"final_score"`
	IsComplete          bool             `json:"is_complete"`
	PromotifLabel       string           `json:"promotif_label"`
	PerformanceCategory string           `json:"performance_category"`
}

// GET /periods/{id}/guru/{guru_id}/report
func (h *Handler) GuruReport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	periodID := chi.URLParam(r, "id")
	guruID := chi.URLParam(r, "guru_id")
	schoolID := effectiveSchoolID(r, claims)

	p, err := h.Periods.GetByID(r.Context(), periodID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	res, err := h.computeResults(r.Context(), p, guruID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to compute results: "+err.Error())
		return
	}

	schoolName := h.Clients.GetSchool(p.SchoolID).Name
	user := h.Clients.GetUser(guruID)

	// classroom names and subject names shown as two separate fields on the
	// print template (RINGKASAN sheet's "Kelas yang Diampu" and "Mata
	// Pelajaran" columns are distinct), deduped since a teacher can teach the
	// same subject across multiple classrooms or vice versa.
	seenClass := map[string]bool{}
	seenSubj := map[string]bool{}
	kelas := []string{}
	mapel := []string{}
	for _, ta := range h.Clients.GetTeachingAssignments(guruID) {
		if !seenClass[ta.ClassroomName] {
			seenClass[ta.ClassroomName] = true
			kelas = append(kelas, ta.ClassroomName)
		}
		if !seenSubj[ta.SubjectName] {
			seenSubj[ta.SubjectName] = true
			mapel = append(mapel, ta.SubjectName)
		}
	}

	tahunAjaran := p.AcademicYearID
	for _, ay := range h.Clients.GetAcademicYears(p.SchoolID) {
		if ay.ID == p.AcademicYearID {
			tahunAjaran = ay.Label
			break
		}
	}

	periodeEvaluasi := p.PublishStartDate
	if p.PublishEndDate != "" {
		periodeEvaluasi += " – " + p.PublishEndDate
	}

	leadAssessor := p.LeadAssessorName
	if leadAssessor == "" && p.CreatedBy != nil {
		leadAssessor = h.Clients.GetUser(*p.CreatedBy).Name
	}
	if leadAssessor == "" {
		leadAssessor = "Admin Sekolah"
	}

	rep := reportPayload{
		Title:               "Rapor Kinerja Guru (" + p.Title + ") — " + schoolName,
		NamaGuru:            user.Name,
		NUPTK:               user.NUPTK,
		TahunAjaran:         tahunAjaran,
		Semester:            p.Semester,
		PeriodeEvaluasi:     periodeEvaluasi,
		MataPelajaran:       mapel,
		KelasYangDiampu:     kelas,
		LeadAssessor:        leadAssessor,
		Categories:          res.Categories,
		FinalScore:          res.FinalScore,
		IsComplete:          res.IsComplete,
		PromotifLabel:       res.PromotifLabel,
		PerformanceCategory: res.PerformanceCategory,
	}
	if rep.NUPTK == "" {
		rep.NUPTK = "-"
	}
	common.WriteJSON(w, http.StatusOK, rep, nil)
}

// ---------------- Upward-feedback track (guru rates a leader) ----------------
//
// Deliberately separate from computeResults above: not blended into any
// guru's weighted score (no room in the 100% self/siswa/kepsek/waka/peer
// split, and it's the opposite rating direction), just a simple per-leader
// average of what their guru rated them.

type leadershipCategoryResult struct {
	EvaluatorType  string   `json:"evaluator_type"`
	FormCode       string   `json:"form_code"`
	FormTitle      string   `json:"form_title"`
	LeaderUserID   string   `json:"leader_user_id"`
	SubmittedCount int      `json:"submitted_count"`
	ExpectedCount  int      `json:"expected_count"` // = number of guru in-scope for the period (each owes one upward eval per assigned leader role)
	AvgScore       *float64 `json:"avg_score"`      // simple average of submitted tasks' raw_score (1-4), null if none submitted
	Complete       bool     `json:"complete"`
}

type leadershipResults struct {
	PeriodID    string                     `json:"period_id"`
	PeriodTitle string                     `json:"period_title"`
	Categories  []leadershipCategoryResult `json:"categories"`
}

// computeLeadershipResults returns one row per leadership role currently
// assigned to the period (kepala_sekolah/waka_kurikulum/waka_kesiswaan,
// skipping any unassigned role), aggregating every guru's upward-eval task
// targeting that role-holder.
func (h *Handler) computeLeadershipResults(ctx context.Context, p *model.Period) (*leadershipResults, error) {
	res := &leadershipResults{PeriodID: p.ID, PeriodTitle: p.Title}
	roles := []struct {
		evalType string
		leaderID *string
	}{
		{model.EvalTypeGuruToKepsek, p.HeadmasterUserID},
		{model.EvalTypeGuruToWakaKurikulum, p.CurriculumVPUserID},
		{model.EvalTypeGuruToWakaKesiswaan, p.StudentAffairsVPUserID},
	}
	for _, role := range roles {
		if role.leaderID == nil {
			continue
		}
		form, err := h.Forms.GetFormByEvaluatorType(ctx, p.SchoolID, role.evalType)
		if err != nil {
			continue // form not bootstrapped yet for this school -- nothing to report
		}
		// ListForPeriodTarget is keyed by target_teacher_user_id only, so
		// filter to this role's own form -- the leader's user id could in
		// principle also appear as a target under a different form (e.g.
		// they're also a target guru in the primary track).
		tasks, err := h.Tasks.ListForPeriodTarget(ctx, p.ID, *role.leaderID)
		if err != nil {
			return nil, err
		}
		submitted := 0
		var sum float64
		for _, t := range tasks {
			if t.FormID != form.ID {
				continue
			}
			if t.Status == model.TaskStatusSubmitted {
				submitted++
				if t.RawScore != nil {
					sum += *t.RawScore
				}
			}
		}
		cr := leadershipCategoryResult{
			EvaluatorType: role.evalType, FormCode: form.Code, FormTitle: form.Title, LeaderUserID: *role.leaderID,
			SubmittedCount: submitted, ExpectedCount: len(p.TargetTeacherIDs),
		}
		if submitted > 0 {
			avg := sum / float64(submitted)
			cr.AvgScore = &avg
		}
		cr.Complete = cr.ExpectedCount > 0 && submitted >= cr.ExpectedCount
		res.Categories = append(res.Categories, cr)
	}
	return res, nil
}

// GET /periods/{id}/leadership-results
func (h *Handler) LeadershipResults(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	periodID := chi.URLParam(r, "id")
	schoolID := effectiveSchoolID(r, claims)
	p, err := h.Periods.GetByID(r.Context(), periodID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	res, err := h.computeLeadershipResults(r.Context(), p)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to compute leadership results: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, res, nil)
}
