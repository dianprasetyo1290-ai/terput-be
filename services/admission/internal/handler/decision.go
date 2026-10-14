package handler

import (
	"database/sql"
	"net/http"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// GetDecision is GET /guardian/applications/{id}/decision.
func (a *App) GetDecision(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	view, err := a.decisionView(r, c.SchoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, view, nil)
}

// decisionView renders the latest decision plus the ranking context for one
// application.
func (a *App) decisionView(r *http.Request, schoolID, applicationID string) (map[string]any, error) {
	var status string
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT status FROM applications WHERE id=$1 AND school_id=$2`, applicationID, schoolID).Scan(&status); err != nil {
		return nil, &httpError{http.StatusNotFound, "application not found"}
	}

	view := map[string]any{
		"application_id": applicationID,
		"status":         "pending",
	}

	var (
		decision  string
		reason    sql.NullString
		rank      sql.NullInt64
		published sql.NullTime
	)
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT d.decision,d.reason,re.rank,COALESCE(d.published_at,d.created_at)
		 FROM decisions d
		 LEFT JOIN ranking_entries re ON re.id=d.ranking_entry_id
		 WHERE d.application_id=$1 AND d.school_id=$2 AND d.status='published'
		 ORDER BY d.version DESC LIMIT 1`, applicationID, schoolID).
		Scan(&decision, &reason, &rank, &published)
	switch {
	case err == sql.ErrNoRows:
		// no decision yet
	case err != nil:
		return nil, err
	default:
		view["status"] = decisionAPI(decision)
		if reason.Valid && reason.String != "" {
			view["message"] = reason.String
		}
		if rank.Valid {
			view["rank"] = rank.Int64
		}
		if published.Valid {
			view["published_at"] = published.Time
		}
	}

	var quota int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT t.quota FROM applications a JOIN admission_tracks t ON t.id=a.track_id WHERE a.id=$1 AND a.school_id=$2`,
		applicationID, schoolID).Scan(&quota); err == nil {
		view["quota"] = quota
	}

	switch status {
	case string(model.StatusReregistration):
		view["response"] = "accepted"
	case string(model.StatusWithdrawn):
		view["response"] = "declined"
	}
	return view, nil
}

func decisionAPI(decision string) string {
	switch decision {
	case "offered", "accepted":
		return "offered"
	case "waitlisted":
		return "waitlisted"
	case "rejected":
		return "rejected"
	default:
		return "pending"
	}
}

// FinalizeDecision is POST /admin/selection/decisions/{applicationId}.
func (a *App) FinalizeDecision(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	applicationID := param(r, "applicationId")

	var in struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}

	var status string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT status FROM applications WHERE id=$1 AND school_id=$2`, applicationID, schoolID).Scan(&status)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "pendaftaran tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if model.ApplicationStatus(status) != model.StatusRanked {
		common.WriteError(w, http.StatusUnprocessableEntity, "pendaftar harus sudah diperingkat sebelum keputusan seleksi")
		return
	}

	dbDecision, err := decisionTarget(in.Decision)
	if err != nil {
		writeErr(w, err)
		return
	}

	// An override (decision other than the ranking recommendation) must carry a
	// reason and is recorded as such.
	var recommendation string
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT recommendation FROM ranking_entries WHERE application_id=$1 AND school_id=$2 ORDER BY ranking_run_id DESC LIMIT 1`,
		applicationID, schoolID).Scan(&recommendation)
	override := recommendation != "" && recommendationAPI(recommendation) != in.Decision
	if override && in.Reason == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "a reason is required when overriding the ranking recommendation")
		return
	}
	if in.Reason == "" {
		in.Reason = "decision published"
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var pendingApproval bool
	if err = tx.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM decisions WHERE application_id=$1 AND status IN('pending_approval','approved'))`,
		applicationID).Scan(&pendingApproval); err != nil {
		writeErr(w, err)
		return
	}
	if pendingApproval {
		common.WriteError(w, http.StatusConflict, "hasil seleksi pendaftar ini sedang diajukan atau sudah disetujui; tidak dapat diubah")
		return
	}
	var rankingEntryID sql.NullInt64
	_ = tx.QueryRowContext(r.Context(),
		`SELECT e.id FROM ranking_entries e JOIN ranking_runs rr ON rr.id=e.ranking_run_id
		 WHERE e.application_id=$1 AND e.school_id=$2 ORDER BY rr.version DESC LIMIT 1`,
		applicationID, schoolID).Scan(&rankingEntryID)
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE decisions SET status='superseded' WHERE application_id=$1 AND status='draft'`, applicationID); err != nil {
		writeErr(w, err)
		return
	}
	var decisionVersion int
	if err = tx.QueryRowContext(r.Context(),
		`SELECT COALESCE(max(version),0)+1 FROM decisions WHERE application_id=$1`, applicationID).Scan(&decisionVersion); err != nil {
		writeErr(w, err)
		return
	}
	// The decision is a draft: it reaches the guardian only after the
	// Selection Committee approves the wave result and the scheduled
	// announcement publishes it (BUG-034/036).
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO decisions(school_id,application_id,ranking_entry_id,decision,override,reason,actor_id,version,status,published_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,'draft',NULL)`,
		schoolID, applicationID, rankingEntryID, dbDecision, override, in.Reason, claims.UserID, decisionVersion); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data,reason)
		 VALUES($1,'application',$2,'decision_drafted','admin',$3,$4,$5,$6)`,
		schoolID, applicationID, claims.UserID, claims.Name, jsonRaw(map[string]any{"decision": dbDecision, "override": override, "version": decisionVersion}), in.Reason); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{
		"application_id": applicationID,
		"decision":       in.Decision,
		"override":       override,
		"version":        decisionVersion,
		"status":         "draft",
	}, nil)
}

// decisionTarget maps the API decision to the stored decision vocabulary.
func decisionTarget(decision string) (string, error) {
	switch decision {
	case "accepted", "offered":
		return "offered", nil
	case "waitlisted":
		return "waitlisted", nil
	case "rejected":
		return "rejected", nil
	default:
		return "", &httpError{http.StatusUnprocessableEntity, "keputusan harus accepted, waitlisted, atau rejected"}
	}
}

// conversionStatusAPI maps the saga status to the reregistration status the
// frontend understands.
func conversionStatusAPI(status string) string {
	switch status {
	case "pending":
		return "not_started"
	case "running":
		return "converting"
	case "completed":
		return "converted"
	case "failed":
		return "failed"
	default:
		return status
	}
}

// GetReregistration is GET /guardian/applications/{id}/reregistration.
func (a *App) GetReregistration(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}

	var (
		status   string
		periodID string
	)
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT status,period_id FROM applications WHERE id=$1 AND school_id=$2`, applicationID, c.SchoolID).
		Scan(&status, &periodID); err != nil {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}

	items, err := a.reregistrationItems(r, c.SchoolID, periodID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}

	view := map[string]any{
		"application_id": applicationID,
		"status":         reregistrationStatus(status),
		"items":          items,
	}

	var (
		conversionStatus sql.NullString
		lastError        sql.NullString
		studentID        sql.NullString
	)
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT status,last_error,student_user_id FROM student_conversions WHERE application_id=$1 AND school_id=$2`,
		applicationID, c.SchoolID).Scan(&conversionStatus, &lastError, &studentID)
	if err == nil {
		if conversionStatus.Valid {
			view["status"] = conversionStatusAPI(conversionStatus.String)
		}
		if lastError.Valid && lastError.String != "" {
			view["conversion_message"] = lastError.String
		}
		if studentID.Valid && studentID.String != "" {
			view["student_id"] = studentID.String
		}
	} else if err != sql.ErrNoRows {
		writeErr(w, err)
		return
	}

	common.WriteJSON(w, http.StatusOK, view, nil)
}

func (a *App) reregistrationItems(r *http.Request, schoolID, periodID, applicationID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT ri.id,ri.label,ri.required,ari.completed_at IS NOT NULL,COALESCE(ari.notes,'')
		 FROM reregistration_items ri
		 JOIN reregistration_checklists rc ON rc.id=ri.checklist_id
		 LEFT JOIN application_reregistration_items ari ON ari.item_id=ri.id AND ari.application_id=$3
		 WHERE rc.school_id=$1 AND rc.period_id=$2
		 ORDER BY ri.code`, schoolID, periodID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id, label, note string
			required, done  bool
		)
		if err := rows.Scan(&id, &label, &required, &done, &note); err != nil {
			return nil, err
		}
		status := "pending"
		if done {
			status = "completed"
		}
		item := map[string]any{
			"id":       id,
			"name":     label,
			"required": required,
			"status":   status,
		}
		if note != "" {
			item["note"] = note
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func reregistrationStatus(applicationStatus string) string {
	switch applicationStatus {
	case string(model.StatusConverted):
		return "converted"
	case string(model.StatusReregistration):
		return "in_progress"
	case string(model.StatusOffered):
		return "not_started"
	default:
		return "not_started"
	}
}
