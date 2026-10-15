package qa

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"skci/quality/internal/common"
)

// StandardReport is one standard's weighted score on an evaluation.
type StandardReport struct {
	StandardID string `json:"standard_id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	WeightBP   int    `json:"weight_bp"`
	ScoreMilli *int64 `json:"score_milli"`
	Scored     int    `json:"scored"`
	Indicators int    `json:"indicators"`
	BelowCount int    `json:"below_target"`
}

// EvaluationScore is the computed result of one self evaluation.
type EvaluationScore struct {
	TotalMilli *int64           `json:"total_score_milli"`
	Standards  []StandardReport `json:"standards"`
	Scored     int              `json:"scored"`
	Indicators int              `json:"indicators"`
}

// scoreEvaluation applies StandardScore / InstrumentScore to the item
// scores recorded in the evaluation. Unscored indicators are left out (and
// counted), never treated as 0.
func scoreEvaluation(inst *QAInstrument, se *QASelfEvaluation) EvaluationScore {
	byInd := map[string]QASelfEvalItem{}
	for _, it := range se.Items {
		byInd[it.IndicatorID] = it
	}
	var out EvaluationScore
	var stdScores []StandardScoreItem
	for _, st := range inst.Standards {
		rep := StandardReport{StandardID: st.ID, Code: st.Code, Name: st.Name, WeightBP: st.WeightBP}
		var items []ItemScore
		for _, it := range st.Items {
			for _, ind := range it.Indicators {
				rep.Indicators++
				ei, ok := byInd[ind.ID]
				if !ok || ei.Score == nil {
					continue
				}
				rep.Scored++
				if ei.BelowTarget {
					rep.BelowCount++
				}
				items = append(items, ItemScore{Score: *ei.Score, WeightBP: ind.WeightBP})
			}
		}
		if v, err := StandardScore(items); err == nil {
			rep.ScoreMilli = &v
			stdScores = append(stdScores, StandardScoreItem{StandardScoreMilli: v, WeightBP: st.WeightBP})
		}
		out.Scored += rep.Scored
		out.Indicators += rep.Indicators
		out.Standards = append(out.Standards, rep)
	}
	if v, err := InstrumentScore(stdScores); err == nil {
		out.TotalMilli = &v
	}
	if out.Standards == nil {
		out.Standards = []StandardReport{}
	}
	return out
}

// latestEvaluation: the newest finalized evaluation of the instrument (and
// academic year when given), else the newest of any status.
func (s *Store) latestEvaluation(ctx context.Context, schoolID, instrumentID, academicYearID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text FROM qa_self_evaluations
		WHERE school_id::text = $1 AND instrument_id::text = $2 AND ($3 = '' OR academic_year_id::text = $3)
		ORDER BY (status = 'finalized') DESC, period_to DESC, created_at DESC LIMIT 1`,
		schoolID, instrumentID, academicYearID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// GetAccreditationReport: GET /qa/reports/accreditation?instrument_id=
// [&academic_year_id=]. Scores come from the latest (preferably finalized)
// self evaluation of the instrument.
func (h *Handler) GetAccreditationReport(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	instID := r.URL.Query().Get("instrument_id")
	ayID := r.URL.Query().Get("academic_year_id")
	if instID == "" {
		common.WriteError(w, http.StatusBadRequest, "instrument_id wajib diisi")
		return
	}
	inst, err := h.store.GetInstrument(r.Context(), schoolID, instID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat instrumen")
		return
	}
	if inst == nil {
		common.WriteError(w, http.StatusNotFound, "instrumen tidak ditemukan")
		return
	}
	resp := map[string]any{
		"instrument_id":      inst.ID,
		"instrument_name":    inst.Name,
		"instrument_version": inst.VersionLabel,
		"score_min":          inst.ScoreMin,
		"score_max":          inst.ScoreMax,
		"evaluation":         nil,
		"generated_at":       time.Now(),
	}
	evalID, err := h.store.latestEvaluation(r.Context(), schoolID, instID, ayID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat evaluasi diri")
		return
	}
	if evalID == "" {
		resp["score"] = scoreEvaluation(inst, &QASelfEvaluation{})
		common.WriteJSON(w, http.StatusOK, resp)
		return
	}
	se, err := h.store.GetSelfEvaluation(r.Context(), schoolID, evalID)
	if err != nil || se == nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat evaluasi diri")
		return
	}
	resp["evaluation"] = map[string]any{
		"id": se.ID, "period_label": se.PeriodLabel, "status": se.Status, "academic_year_id": se.AcademicYearID,
		"finalized_at": se.FinalizedAt, "updated_at": se.UpdatedAt,
	}
	resp["score"] = scoreEvaluation(inst, se)
	if bp, err := h.store.GetCompleteness(r.Context(), schoolID, instID, se.AcademicYearID); err == nil {
		resp["completeness_bp"] = bp
	}
	common.WriteJSON(w, http.StatusOK, resp)
}

// GetComparisonReport: GET /qa/reports/comparison?instrument_id= (kepala
// yayasan/superadmin). For each school of the foundation, the latest
// finalized evaluation of an instrument with the same code is scored.
func (h *Handler) GetComparisonReport(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	instID := r.URL.Query().Get("instrument_id")
	if instID == "" {
		common.WriteError(w, http.StatusBadRequest, "instrument_id wajib diisi")
		return
	}
	inst, err := h.store.GetInstrument(r.Context(), schoolID, instID)
	if err != nil || inst == nil {
		common.WriteError(w, http.StatusNotFound, "instrumen tidak ditemukan")
		return
	}
	if h.clients == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "layanan identitas tidak tersedia")
		return
	}
	schools, err := h.clients.GetFoundationSchools(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusServiceUnavailable, "daftar sekolah yayasan tidak dapat dimuat")
		return
	}
	type row struct {
		SchoolID       string  `json:"school_id"`
		SchoolName     string  `json:"school_name"`
		IsCurrent      bool    `json:"is_current"`
		EvaluationID   *string `json:"evaluation_id"`
		PeriodLabel    *string `json:"period_label"`
		ScoreMilli     *int64  `json:"score_milli"`
		CompletenessBP *int64  `json:"completeness_bp"`
	}
	out := []row{}
	for _, sc := range schools {
		rw := row{SchoolID: sc.ID, SchoolName: sc.Name, IsCurrent: sc.ID == schoolID}
		var evalID, evalInst, ay, label string
		err := h.store.pool.QueryRow(r.Context(), `
			SELECT se.id::text, se.instrument_id::text, se.academic_year_id::text, se.period_label
			FROM qa_self_evaluations se JOIN qa_instruments i ON i.id = se.instrument_id
			WHERE se.school_id::text = $1 AND i.code = $2 AND se.status = 'finalized'
			ORDER BY se.period_to DESC, se.finalized_at DESC NULLS LAST LIMIT 1`, sc.ID, inst.Code).Scan(&evalID, &evalInst, &ay, &label)
		if err == nil {
			oInst, _ := h.store.GetInstrument(r.Context(), sc.ID, evalInst)
			se, _ := h.store.GetSelfEvaluation(r.Context(), sc.ID, evalID)
			if oInst != nil && se != nil {
				score := scoreEvaluation(oInst, se)
				rw.EvaluationID, rw.PeriodLabel, rw.ScoreMilli = &evalID, &label, score.TotalMilli
				if bp, err := h.store.GetCompleteness(r.Context(), sc.ID, evalInst, ay); err == nil {
					rw.CompletenessBP = &bp
				}
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			common.WriteError(w, http.StatusInternalServerError, "gagal memuat evaluasi sekolah")
			return
		}
		out = append(out, rw)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"instrument_code": inst.Code, "schools": out, "generated_at": time.Now()})
}
