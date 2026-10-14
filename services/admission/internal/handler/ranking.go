package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// ---------------------------------------------------------------------------
// Selection formulas
// ---------------------------------------------------------------------------

// formulaInput is the admin-editable payload. Component weights arrive as
// percentages (0-100); they are converted to basis points on the way in.
type formulaInput struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Version      int    `json:"version"`
	Status       string `json:"status"`
	MinimumTotal int64  `json:"minimum_total"`
	Components   []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Source  string `json:"source"`
		Weight  int64  `json:"weight"`
		Minimum int64  `json:"minimum"`
	} `json:"components"`
	TieBreakers []string `json:"tie_breakers"`
}

func (in formulaInput) spec() (model.FormulaSpec, error) {
	components := make([]model.Component, 0, len(in.Components))
	for _, c := range in.Components {
		code := c.ID
		if code == "" {
			code = model.Slug(c.Name)
		}
		if code == "" {
			return model.FormulaSpec{}, &httpError{http.StatusUnprocessableEntity, "every component needs a name"}
		}
		components = append(components, model.Component{
			Code:    code,
			Name:    c.Name,
			Source:  c.Source,
			Weight:  c.Weight * 100,
			Minimum: c.Minimum,
		})
	}
	return model.FormulaSpec{
		Name:         in.Name,
		Status:       normalizeFormulaStatus(in.Status),
		MinimumTotal: in.MinimumTotal,
		Components:   components,
		TieBreakers:  in.TieBreakers,
	}, nil
}

// ListFormulas is GET /admin/selection/formulas.
func (a *App) ListFormulas(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,version,status,formula FROM selection_formula_versions WHERE school_id=$1 ORDER BY created_at DESC`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, status string
			version    int
			raw        []byte
		)
		if err := rows.Scan(&id, &version, &status, &raw); err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, formulaView(id, version, status, raw))
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func formulaView(id string, version int, status string, raw []byte) map[string]any {
	var spec model.FormulaSpec
	_ = json.Unmarshal(raw, &spec)
	components := []map[string]any{}
	for _, c := range spec.Components {
		components = append(components, map[string]any{
			"id":      c.Code,
			"name":    c.Name,
			"source":  c.Source,
			"weight":  c.Weight / 100,
			"minimum": c.Minimum,
		})
	}
	tieBreakers := spec.TieBreakers
	if tieBreakers == nil {
		tieBreakers = []string{"total_score desc", "cbt_score desc", "submitted_at asc", "registration_number asc"}
	}
	return map[string]any{
		"id":            id,
		"name":          spec.Name,
		"version":       version,
		"status":        status,
		"minimum_total": spec.MinimumTotal,
		"components":    components,
		"tie_breakers":  tieBreakers,
	}
}

// CreateFormula is POST /admin/selection/formulas.
func (a *App) CreateFormula(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in formulaInput
	if !common.Decode(w, r, &in) {
		return
	}
	spec, err := in.spec()
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = spec.Compile(); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	periodID, waveID, trackID, err := a.defaultScope(r, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	raw, _ := json.Marshal(spec)
	checksum := sha256.Sum256(raw)

	var version int
	if err = a.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(max(version),0)+1 FROM selection_formula_versions WHERE wave_id=$1 AND track_id=$2`, waveID, trackID).
		Scan(&version); err != nil {
		writeErr(w, err)
		return
	}
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO selection_formula_versions(school_id,period_id,wave_id,track_id,version,formula,status,checksum)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		schoolID, periodID, waveID, trackID, version, raw, spec.Status, hex.EncodeToString(checksum[:])).Scan(&id)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "version": version, "status": spec.Status}, nil)
}

// UpdateFormula is PUT /admin/selection/formulas/{id}.
func (a *App) UpdateFormula(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in formulaInput
	if !common.Decode(w, r, &in) {
		return
	}
	spec, err := in.spec()
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = spec.Compile(); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	raw, _ := json.Marshal(spec)
	checksum := sha256.Sum256(raw)
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE selection_formula_versions SET formula=$1,status=$2,checksum=$3 WHERE id=$4 AND school_id=$5`,
		raw, spec.Status, hex.EncodeToString(checksum[:]), param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "formula not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id"), "status": spec.Status}, nil)
}

// defaultScope resolves a period/wave/track for records the config forms do not
// scope explicitly.
func (a *App) defaultScope(r *http.Request, schoolID string) (periodID, waveID, trackID string, err error) {
	periodID, err = a.resolvePeriodID(r, schoolID, "")
	if err != nil {
		return
	}
	if err = a.DB.QueryRowContext(r.Context(),
		`SELECT id FROM admission_waves WHERE school_id=$1 AND period_id=$2 AND deleted_at IS NULL ORDER BY registration_start LIMIT 1`,
		schoolID, periodID).Scan(&waveID); err == sql.ErrNoRows {
		return "", "", "", &httpError{http.StatusUnprocessableEntity, "configure a wave for the period first"}
	} else if err != nil {
		return
	}
	if err = a.DB.QueryRowContext(r.Context(),
		`SELECT id FROM admission_tracks WHERE school_id=$1 AND period_id=$2 AND deleted_at IS NULL ORDER BY name LIMIT 1`,
		schoolID, periodID).Scan(&trackID); err == sql.ErrNoRows {
		return "", "", "", &httpError{http.StatusUnprocessableEntity, "configure a track for the period first"}
	}
	return
}

// ---------------------------------------------------------------------------
// Scores
// ---------------------------------------------------------------------------

// ListScores is GET /admin/selection/scores.
func (a *App) ListScores(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	applicationID := r.URL.Query().Get("application_id")
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT cs.application_id,sc.code,cs.score,cs.final
		 FROM component_scores cs JOIN selection_components sc ON sc.id=cs.component_id
		 WHERE cs.school_id=$1 AND ($2='' OR cs.application_id::text=$2)
		 ORDER BY cs.application_id,sc.code`, schoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			appID, code string
			score       int64
			final       bool
		)
		if err := rows.Scan(&appID, &code, &score, &final); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"application_id": appID,
			"component_code": code,
			"score":          score,
			"final":          final,
		})
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": items}, nil)
}

// UpsertScore is PUT /admin/selection/scores/{applicationId}.
func (a *App) UpsertScore(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	applicationID := param(r, "applicationId")
	var in struct {
		ComponentID   string `json:"component_id"`
		ComponentCode string `json:"component_code"`
		Score         int64  `json:"score"`
		Final         *bool  `json:"final"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	var periodID, status string
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT period_id,status FROM applications WHERE id=$1 AND school_id=$2`, applicationID, schoolID).Scan(&periodID, &status); err != nil {
		common.WriteError(w, http.StatusNotFound, "pendaftaran tidak ditemukan")
		return
	}
	switch model.ApplicationStatus(status) {
	case model.StatusEligibleForTest, model.StatusTested, model.StatusRanked:
	default:
		common.WriteError(w, http.StatusUnprocessableEntity, "nilai hanya dapat diisi untuk pendaftar yang siap tes atau sudah tes")
		return
	}
	componentID, _, err := a.ensureComponent(r, schoolID, periodID, in.ComponentID, in.ComponentCode)
	if err != nil {
		writeErr(w, err)
		return
	}
	var maxScore int64
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT max_score FROM selection_components WHERE id=$1`, componentID).Scan(&maxScore); err != nil {
		writeErr(w, err)
		return
	}
	if err := model.ValidateScore(in.Score, maxScore); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	final := in.Final == nil || *in.Final
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO component_scores(school_id,application_id,component_id,score,final,entered_by)
		 VALUES($1,$2,$3,$4,$5,$6)
		 ON CONFLICT(application_id,component_id) DO UPDATE SET score=EXCLUDED.score,final=EXCLUDED.final,entered_by=EXCLUDED.entered_by,updated_at=now()`,
		schoolID, applicationID, componentID, in.Score, final, claims.UserID); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "application", applicationID, "score_saved", "admin", claims.UserID, claims.Name, "",
		nil, map[string]any{"component_id": componentID, "score": in.Score, "final": final})
	common.WriteJSON(w, http.StatusOK, map[string]any{"application_id": applicationID, "score": in.Score, "final": final}, nil)
}

// ensureComponent returns the component id for a code, creating the component
// if a code was supplied but does not exist yet.
func (a *App) ensureComponent(r *http.Request, schoolID, periodID, componentID, code string) (string, string, error) {
	if componentID != "" {
		var resolvedCode string
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT code FROM selection_components WHERE id=$1 AND school_id=$2`, componentID, schoolID).Scan(&resolvedCode); err != nil {
			return "", "", &httpError{http.StatusNotFound, "component not found"}
		}
		return componentID, resolvedCode, nil
	}
	if code == "" {
		return "", "", &httpError{http.StatusUnprocessableEntity, "component_id or component_code is required"}
	}
	var id string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id FROM selection_components WHERE school_id=$1 AND period_id=$2 AND code=$3`, schoolID, periodID, code).Scan(&id)
	if err == nil {
		return id, code, nil
	}
	if err != sql.ErrNoRows {
		return "", "", err
	}
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO selection_components(school_id,period_id,code,name,source) VALUES($1,$2,$3,$4,'manual') RETURNING id`,
		schoolID, periodID, code, code).Scan(&id)
	return id, code, err
}

// ---------------------------------------------------------------------------
// Ranking
// ---------------------------------------------------------------------------

type rankingInput struct {
	PeriodID  string `json:"period_id"`
	WaveID    string `json:"wave_id"`
	TrackID   string `json:"track_id"`
	FormulaID string `json:"formula_id"`
}

// RankingPreview is POST /admin/selection/rankings/preview: compute without
// persisting, so admins can inspect a recommendation before locking it.
func (a *App) RankingPreview(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in rankingInput
	if !common.Decode(w, r, &in) {
		return
	}
	entries, checksum, quota, err := a.computeRanking(r, schoolID, in.PeriodID, in.WaveID, in.TrackID, in.FormulaID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"id":         "",
		"formula_id": in.FormulaID,
		"status":     "preview",
		"checksum":   checksum,
		"created_at": time.Now(),
		"quota":      quota,
		"entries":    entries,
	}, nil)
}

// RunRanking is POST /admin/selection/rankings: persist an immutable snapshot.
func (a *App) RunRanking(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in rankingInput
	if !common.Decode(w, r, &in) {
		return
	}
	entries, checksum, quota, err := a.computeRanking(r, schoolID, in.PeriodID, in.WaveID, in.TrackID, in.FormulaID)
	if err != nil {
		writeErr(w, err)
		return
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var version int
	if err = tx.QueryRowContext(r.Context(),
		`SELECT COALESCE(max(version),0)+1 FROM ranking_runs WHERE wave_id=$1 AND track_id=$2`, in.WaveID, in.TrackID).
		Scan(&version); err != nil {
		writeErr(w, err)
		return
	}
	var runID string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO ranking_runs(school_id,period_id,wave_id,track_id,formula_version_id,version,input_checksum,status,created_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,'final',$8) RETURNING id`,
		schoolID, in.PeriodID, in.WaveID, in.TrackID, in.FormulaID, version, checksum, claims.UserID).Scan(&runID)
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, entry := range entries {
		if _, err = tx.ExecContext(r.Context(),
			`INSERT INTO ranking_entries(school_id,ranking_run_id,application_id,rank,total_score,scores_snapshot,recommendation)
			 VALUES($1,$2,$3,$4,$5,'{}',$6)`,
			schoolID, runID, entry["application_id"], entry["rank"], entry["total_score"], recommendationDB(entry["recommendation"].(string))); err != nil {
			writeErr(w, err)
			return
		}
		if err = a.markRanked(r.Context(), tx, schoolID, entry["application_id"].(string)); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "ranking", runID, "ranking_published", "admin", claims.UserID, "", "",
		nil, map[string]any{"version": version, "entries": len(entries)})
	common.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":         runID,
		"formula_id": in.FormulaID,
		"status":     "final",
		"checksum":   checksum,
		"created_at": time.Now(),
		"quota":      quota,
		"entries":    entries,
	}, nil)
}

// computeRanking loads the final scores for a scope, applies the stored formula
// and returns ranked entries with a deterministic checksum.
func (a *App) computeRanking(r *http.Request, schoolID, periodID, waveID, trackID, formulaID string) ([]map[string]any, string, int, error) {
	if periodID == "" || waveID == "" || trackID == "" || formulaID == "" {
		return nil, "", 0, &httpError{http.StatusUnprocessableEntity, "period_id, wave_id, track_id and formula_id are required"}
	}
	var (
		raw                       []byte
		formulaWave, formulaTrack string
	)
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT formula,wave_id,track_id FROM selection_formula_versions WHERE id=$1 AND school_id=$2`, formulaID, schoolID).
		Scan(&raw, &formulaWave, &formulaTrack); err != nil {
		return nil, "", 0, &httpError{http.StatusNotFound, "formula tidak ditemukan"}
	}
	// BUG-161: a formula belongs to one wave + track.
	if formulaWave != waveID || formulaTrack != trackID {
		return nil, "", 0, &httpError{http.StatusUnprocessableEntity, "formula ini dibuat untuk gelombang/jalur lain"}
	}
	var spec model.FormulaSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, "", 0, err
	}
	formula, err := spec.Compile()
	if err != nil {
		return nil, "", 0, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT a.id,a.registration_number,a.submitted_at,ap.full_name,sc.code,cs.score
		 FROM applications a
		 JOIN applicants ap ON ap.id=a.applicant_id
		 JOIN component_scores cs ON cs.application_id=a.id AND cs.final
		 JOIN selection_components sc ON sc.id=cs.component_id
		 WHERE a.school_id=$1 AND a.period_id=$2 AND a.wave_id=$3 AND a.track_id=$4
		   AND a.status IN('eligible_for_test','tested','ranked')
		 ORDER BY a.id,sc.code`, schoolID, periodID, waveID, trackID)
	if err != nil {
		return nil, "", 0, err
	}
	defer rows.Close()

	inputs := map[string]*model.RankInput{}
	names := map[string]string{}
	for rows.Next() {
		var (
			id, registrationNumber, applicantName, code string
			submittedAt                                 time.Time
			score                                       int64
		)
		if err := rows.Scan(&id, &registrationNumber, &submittedAt, &applicantName, &code, &score); err != nil {
			return nil, "", 0, err
		}
		if inputs[id] == nil {
			inputs[id] = &model.RankInput{ApplicationID: id, RegistrationNumber: registrationNumber, SubmittedAt: submittedAt, Scores: map[string]int64{}}
			names[id] = applicantName
		}
		inputs[id].Scores[code] = score
	}
	if err := rows.Err(); err != nil {
		return nil, "", 0, err
	}

	list := make([]model.RankInput, 0, len(inputs))
	for _, in := range inputs {
		list = append(list, *in)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ApplicationID < list[j].ApplicationID })

	var quota int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT quota FROM admission_tracks WHERE id=$1 AND school_id=$2`, trackID, schoolID).Scan(&quota); err != nil {
		return nil, "", 0, &httpError{http.StatusNotFound, "track not found"}
	}

	ranked, err := model.Rank(list, formula, quota)
	if err != nil {
		return nil, "", 0, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	checksumInput := struct {
		Inputs  []model.RankInput `json:"inputs"`
		Formula model.Formula     `json:"formula"`
		Quota   int               `json:"quota"`
	}{list, formula, quota}
	encoded, _ := json.Marshal(checksumInput)
	sum := sha256.Sum256(encoded)

	entries := make([]map[string]any, 0, len(ranked))
	for _, entry := range ranked {
		entries = append(entries, map[string]any{
			"application_id":      entry.ApplicationID,
			"registration_number": inputs[entry.ApplicationID].RegistrationNumber,
			"applicant_name":      names[entry.ApplicationID],
			"total_score":         entry.Total,
			"rank":                entry.Rank,
			"recommendation":      recommendationAPI(entry.Recommendation),
		})
	}
	return entries, hex.EncodeToString(sum[:]), quota, nil
}

// recommendationDB maps the API recommendation to the stored vocabulary.
func recommendationDB(recommendation string) string {
	if recommendation == "waitlisted" {
		return "waitlisted"
	}
	return recommendation
}

// recommendationAPI maps the stored vocabulary to the API value.
func recommendationAPI(recommendation string) string {
	if recommendation == "waitlist" {
		return "waitlisted"
	}
	return recommendation
}

func normalizeFormulaStatus(status string) string {
	if strings.EqualFold(status, "published") {
		return "published"
	}
	return "draft"
}

// markRanked moves a scored applicant to "ranked" (via "tested" when needed),
// so decisions can be drafted for it.
func (a *App) markRanked(ctx context.Context, tx *sql.Tx, schoolID, applicationID string) error {
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM applications WHERE id=$1`, applicationID).Scan(&status); err != nil {
		return err
	}
	steps := map[string][]model.ApplicationStatus{
		string(model.StatusEligibleForTest): {model.StatusTested, model.StatusRanked},
		string(model.StatusTested):          {model.StatusRanked},
	}[status]
	for _, to := range steps {
		if _, err := a.transitionTx(ctx, tx, schoolID, applicationID, "system", "admission", "Sistem PPDB", to, "Hasil pemeringkatan", 0, ""); err != nil {
			return err
		}
	}
	return nil
}
