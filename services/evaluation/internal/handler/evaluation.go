package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"skci/evaluation/internal/common"
	"skci/evaluation/internal/model"
)

type answerInput struct {
	ItemID string `json:"item_id"`
	Score  int    `json:"score"`
	Notes  string `json:"notes"`
}

type submitEvaluationRequest struct {
	EvaluatorType   string        `json:"evaluator_type"`    // self|peer|siswa|kepala_sekolah|waka_kurikulum|waka_kesiswaan|guru_to_kepsek|guru_to_waka_kurikulum|guru_to_waka_kesiswaan
	TargetTeacherID string        `json:"target_teacher_id"` // for guru_to_* upward types, this is the LEADER being rated
	ClassroomID     string        `json:"classroom_id"`      // required only when evaluator_type=siswa
	Answers         []answerInput `json:"answers"`
}

// authorizeEvaluation checks the caller is actually permitted to submit this
// evaluator_type -> target_teacher_id pairing under this period's config.
// This is the server-side enforcement point ("scoped to the caller's
// identity + role-category" from the spec).
func authorizeEvaluation(claims *common.Claims, p *model.Period, req submitEvaluationRequest) error {
	// Client hierarchy rule (2026-08): a guru may never rate an atasan --
	// not waka kurikulum, not waka kesiswaan, not kepala sekolah -- full
	// stop, no "leadership feedback" carve-out. The guru_to_kepsek/
	// guru_to_waka_kurikulum/guru_to_waka_kesiswaan upward-feedback types
	// are exactly that (guru rating a leader in their leadership capacity),
	// so they're blocked here regardless of period config. This also
	// resolves the "rated as guru, never as kepala sekolah/waka" exception
	// for free: with this blocked, the ONLY way a kepsek/waka gets rated at
	// all is by being a target guru (target_teacher_ids/peer_evaluatee_ids)
	// -- i.e. always in their guru capacity, never their jabatan capacity,
	// since there is no other rating pathway left that targets the jabatan.
	if model.IsUpwardEvalType(req.EvaluatorType) {
		return fmt.Errorf("guru tidak dapat menilai atasan (kepala sekolah/waka kurikulum/waka kesiswaan)")
	}
	// For peer and the upward-feedback (guru_to_*) types, target_teacher_id
	// isn't a member of the "J" target-guru list (it's the peer evaluatee or
	// the leader being rated respectively) -- those cases validate their own
	// target below instead.
	if !containsStr(p.TargetTeacherIDs, req.TargetTeacherID) && req.EvaluatorType != model.EvalTypePeer && !model.IsUpwardEvalType(req.EvaluatorType) {
		return fmt.Errorf("target_teacher_id is not in this period's target guru list")
	}
	// A guru holding a structural position (kepsek/waka_*) is still just
	// themselves for every OTHER category: they cannot rate their own
	// leadership performance ("kepala_sekolah"/"waka_*"), nor can they submit
	// an upward-feedback rating targeting themselves-as-leader
	// ("guru_to_*"), nor peer-evaluate themselves. The only category allowed
	// when evaluator == target is self-assessment. Checked once here so
	// every submission path (not just eager task generation in
	// regenerateTasks) is covered.
	if req.EvaluatorType != model.EvalTypeSelf && claims.UserID == req.TargetTeacherID {
		return fmt.Errorf("cannot submit a %s evaluation targeting yourself; self-evaluation must use the self-assessment form", req.EvaluatorType)
	}
	switch req.EvaluatorType {
	case model.EvalTypeSelf:
		if !hasRole(claims, "guru") {
			return fmt.Errorf("only guru can submit self-assessment")
		}
		if claims.UserID != req.TargetTeacherID {
			return fmt.Errorf("self-assessment target_teacher_id must be the caller's own id")
		}
	case model.EvalTypePeer:
		if !containsStr(p.PeerEvaluatorIDs, claims.UserID) {
			return fmt.Errorf("caller is not assigned as a peer evaluator ('yang menilai') for this period")
		}
		if !containsStr(p.PeerEvaluateeIDs, req.TargetTeacherID) {
			return fmt.Errorf("target_teacher_id is not in this period's 'guru yang ingin dinilai' list")
		}
		// self-pair already rejected by the general check above
	case model.EvalTypeKepalaSekolah:
		if p.HeadmasterUserID == nil || *p.HeadmasterUserID != claims.UserID {
			return fmt.Errorf("caller is not the Kepala Sekolah assigned to this period")
		}
	case model.EvalTypeWakaKurikulum:
		if p.CurriculumVPUserID == nil || *p.CurriculumVPUserID != claims.UserID {
			return fmt.Errorf("caller is not the Waka Kurikulum assigned to this period")
		}
	case model.EvalTypeWakaKesiswaan:
		if p.StudentAffairsVPUserID == nil || *p.StudentAffairsVPUserID != claims.UserID {
			return fmt.Errorf("caller is not the Waka Kesiswaan assigned to this period")
		}
	// model.EvalTypeGuruToKepsek/GuruToWakaKurikulum/GuruToWakaKesiswaan
	// (upward-feedback: guru rating a leader) are rejected unconditionally
	// above -- see the IsUpwardEvalType check at the top of this function --
	// so they're intentionally absent from this switch.
	case model.EvalTypeSiswa:
		if !hasRole(claims, "siswa") {
			return fmt.Errorf("only siswa can submit this evaluation type")
		}
		if req.ClassroomID == "" {
			return fmt.Errorf("classroom_id is required for siswa evaluations")
		}
		if !containsStr(p.StudentClassroomIDs, req.ClassroomID) {
			return fmt.Errorf("classroom_id is not assigned to this period")
		}
	default:
		return fmt.Errorf("unknown evaluator_type %q", req.EvaluatorType)
	}
	return nil
}

// POST /periods/{id}/evaluations
func (h *Handler) SubmitEvaluation(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	periodID := chi.URLParam(r, "id")
	schoolID := effectiveSchoolID(r, claims)

	p, err := h.Periods.GetByID(r.Context(), periodID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	attachStatus(p)
	if p.Status != model.StatusOngoing {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("period is not open for evaluation (status: %s)", p.Status))
		return
	}

	var req submitEvaluationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := authorizeEvaluation(claims, p, req); err != nil {
		common.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	form, err := h.Forms.GetFormByEvaluatorType(r.Context(), schoolID, req.EvaluatorType)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "evaluation form not configured for this school/category: "+err.Error())
		return
	}
	items, err := h.Forms.ListItems(r.Context(), form.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to load form items: "+err.Error())
		return
	}
	itemWeights := map[string]float64{}
	itemMax := map[string]int{}
	for _, it := range items {
		itemWeights[it.ID] = it.Weight
		itemMax[it.ID] = it.MaxScore
	}

	answered := map[string]int{}
	notes := map[string]string{}
	for _, a := range req.Answers {
		maxScore, ok := itemMax[a.ItemID]
		if !ok {
			common.WriteError(w, http.StatusBadRequest, "item_id "+a.ItemID+" does not belong to this evaluation form")
			return
		}
		if a.Score < 1 || a.Score > maxScore {
			common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("score for item %s must be between 1 and %d", a.ItemID, maxScore))
			return
		}
		answered[a.ItemID] = a.Score
		notes[a.ItemID] = a.Notes
	}
	var missing []string
	for _, it := range items {
		if _, ok := answered[it.ID]; !ok {
			missing = append(missing, it.Code)
		}
	}
	if len(missing) > 0 {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("missing answers for items: %v", missing))
		return
	}

	var classroomID *string
	if req.EvaluatorType == model.EvalTypeSiswa {
		classroomID = &req.ClassroomID
	}
	task, err := h.Tasks.UpsertTask(r.Context(), &model.EvalTask{
		PeriodID: p.ID, FormID: form.ID, SchoolID: schoolID, AcademicYearID: p.AcademicYearID,
		TargetTeacherUserID: req.TargetTeacherID, EvaluatorUserID: claims.UserID, EvaluatorType: req.EvaluatorType,
		ClassroomID: classroomID,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create/find evaluation task: "+err.Error())
		return
	}

	rawScore, err := h.Tasks.SubmitAnswers(r.Context(), task.ID, schoolID, answered, notes, itemWeights)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to submit answers: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"task_id":           task.ID,
		"form":              form.Code,
		"evaluator_type":    req.EvaluatorType,
		"target_teacher_id": req.TargetTeacherID,
		"raw_score":         rawScore,
		"status":            "submitted",
	}, nil)
}

// GET /periods/{id}/my-tasks - the caller's own assignment list for this period (bonus convenience endpoint).
func (h *Handler) MyTasks(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	periodID := chi.URLParam(r, "id")
	tasks, err := h.Tasks.ListForEvaluator(r.Context(), periodID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list tasks: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, filterSelfTargetingTasks(tasks), nil)
}

// filterSelfTargetingTasks is defense-in-depth for a guru's own task list:
// regenerateTasks never creates, and SubmitEvaluation's authorizeEvaluation
// never accepts, a non-self task where evaluator == target (see the guard
// there), but this strips any such row anyway before it reaches the UI --
// e.g. leftover rows from before that guard existed.
func filterSelfTargetingTasks(tasks []*model.EvalTask) []*model.EvalTask {
	out := tasks[:0]
	for _, t := range tasks {
		if t.EvaluatorType != model.EvalTypeSelf && t.EvaluatorUserID == t.TargetTeacherUserID {
			continue
		}
		out = append(out, t)
	}
	return out
}
