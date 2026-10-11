package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

type transitionInput struct {
	Reason  string `json:"reason"`
	Version int    `json:"version"`
}

type offerResponseInput struct {
	Response string `json:"response"`
}

// httpError carries an HTTP status alongside a message so the transition
// helpers can bubble a precise error up to the handler.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

func writeErr(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		common.WriteError(w, he.status, he.msg)
		return
	}
	common.WriteError(w, http.StatusInternalServerError, err.Error())
}

// SubmitApplication is POST /guardian/applications/{id}/submit. It requires the
// version from the client (optimistic lock) and the domain guards: the
// registration window must be open and required documents present.
func (a *App) SubmitApplication(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	var in transitionInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Version < 1 {
		common.WriteError(w, http.StatusBadRequest, "version is required")
		return
	}
	applicationID := param(r, "id")

	var windowOpen bool
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM applications a
		   JOIN admission_periods p ON p.id=a.period_id
		   JOIN admission_waves w ON w.id=a.wave_id
		   WHERE a.id=$1 AND a.school_id=$2 AND p.status='open'
		     AND now() BETWEEN w.registration_start AND w.registration_end)`,
		applicationID, c.SchoolID).Scan(&windowOpen)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !windowOpen {
		common.WriteError(w, http.StatusUnprocessableEntity, "registration window is closed")
		return
	}

	var missing int
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT count(*) FROM document_requirements dr
		   JOIN applications a ON a.period_id=dr.period_id
		   WHERE a.id=$1 AND dr.required AND dr.deleted_at IS NULL
		     AND NOT EXISTS (
		       SELECT 1 FROM application_documents d
		       WHERE d.application_id=a.id AND d.requirement_id=dr.id
		     )`, applicationID).Scan(&missing)
	if err != nil {
		writeErr(w, err)
		return
	}
	if missing > 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "required documents are not complete")
		return
	}

	result, err := a.transition(r.Context(), c.SchoolID, applicationID, "guardian", c.GuardianID, "",
		model.StatusSubmitted, "", in.Version, c.GuardianID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, result, nil)
}

// RequestCorrection is POST /admin/applications/{id}/request_correction. A
// correction does not delete the previous state; it appends an event and a
// reason so the guardian can see exactly what to fix.
func (a *App) RequestCorrection(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in transitionInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Version < 1 || in.Reason == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "reason and version are required")
		return
	}
	applicationID := param(r, "id")

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var (
		status  string
		version int
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,version FROM applications WHERE id=$1 AND school_id=$2 FOR UPDATE`, applicationID, schoolID).
		Scan(&status, &version)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if version != in.Version {
		common.WriteError(w, http.StatusConflict, "stale application version")
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE applications SET version=version+1,updated_at=now() WHERE id=$1`, applicationID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO application_state_events(school_id,application_id,from_status,to_status,actor_type,actor_id,reason,version)
		 VALUES($1,$2,$3,$3,'admin',$4,$5,$6)`,
		schoolID, applicationID, status, claims.UserID, in.Reason, version+1); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "application", applicationID, "correction_requested", "admin", claims.UserID, "",
		in.Reason, map[string]any{"status": status}, map[string]any{"status": status})
	a.writeAdminApplication(w, r, schoolID, applicationID)
}

// MarkEligible is POST /admin/applications/{id}/mark_eligible.
func (a *App) MarkEligible(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in transitionInput
	if !common.Decode(w, r, &in) {
		return
	}
	applicationID := param(r, "id")

	var (
		status  string
		version int
	)
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT status,version FROM applications WHERE id=$1 AND school_id=$2`, applicationID, schoolID).Scan(&status, &version)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if in.Version < 1 {
		in.Version = version
	}

	// From "submitted" the committee first opens the document review stage;
	// mark_eligible then advances that stage to eligible_for_test.
	if model.ApplicationStatus(status) == model.StatusSubmitted {
		if _, err = a.transition(r.Context(), schoolID, applicationID, "admin", claims.UserID, "",
			model.StatusDocumentReview, "", in.Version, ""); err != nil {
			writeErr(w, err)
			return
		}
		in.Version++
	}
	if _, err := a.transition(r.Context(), schoolID, applicationID, "admin", claims.UserID, "",
		model.StatusEligibleForTest, in.Reason, in.Version, ""); err != nil {
		writeErr(w, err)
		return
	}
	a.writeAdminApplication(w, r, schoolID, applicationID)
}

// RejectApplication is POST /admin/applications/{id}/reject.
func (a *App) RejectApplication(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in transitionInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Reason == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "reason is required")
		return
	}
	applicationID := param(r, "id")
	if _, err := a.transition(r.Context(), schoolID, applicationID, "admin", claims.UserID, "",
		model.StatusRejected, in.Reason, in.Version, ""); err != nil {
		writeErr(w, err)
		return
	}
	a.writeAdminApplication(w, r, schoolID, applicationID)
}

// RespondDecision is POST /guardian/applications/{id}/decision/respond.
func (a *App) RespondDecision(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	var in offerResponseInput
	if !common.Decode(w, r, &in) {
		return
	}
	response := in.Response
	if response != "accepted" && response != "declined" {
		common.WriteError(w, http.StatusUnprocessableEntity, "response must be accepted or declined")
		return
	}
	applicationID := param(r, "id")

	var (
		status  string
		version int
	)
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT status,version FROM applications WHERE id=$1 AND school_id=$2`, applicationID, c.SchoolID).
		Scan(&status, &version)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if model.ApplicationStatus(status) != model.StatusOffered {
		common.WriteError(w, http.StatusUnprocessableEntity, "no open offer to respond to")
		return
	}

	target := model.StatusReregistration
	reason := "offer accepted"
	if response == "declined" {
		target = model.StatusWithdrawn
		reason = "offer declined"
	}
	if _, err := a.transition(r.Context(), c.SchoolID, applicationID, "guardian", c.GuardianID, "",
		target, reason, version, c.GuardianID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		`UPDATE decisions SET decision=$1 WHERE id=(SELECT id FROM decisions WHERE application_id=$2 AND school_id=$3 ORDER BY version DESC LIMIT 1)`,
		decisionWord(response), applicationID, c.SchoolID); err != nil {
		writeErr(w, err)
		return
	}
	result, err := a.decisionView(r, c.SchoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, result, nil)
}

func decisionWord(response string) string {
	if response == "accepted" {
		return "offered"
	}
	return "waitlisted"
}

// transition performs one validated application status change inside a row
// lock, writing the state event and the audit trail atomically.
func (a *App) transition(ctx context.Context, schoolID, applicationID, actorType, actorID, actorName string, to model.ApplicationStatus, reason string, version int, guardianID string) (map[string]any, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	query := `SELECT a.status,a.version FROM applications a`
	args := []any{applicationID, schoolID}
	if actorType == "guardian" {
		query += ` JOIN applicants ap ON ap.id=a.applicant_id WHERE a.id=$1 AND a.school_id=$2 AND ap.guardian_id=$3 FOR UPDATE`
		args = append(args, guardianID)
	} else {
		query += ` WHERE a.id=$1 AND a.school_id=$2 FOR UPDATE`
	}

	var (
		from    string
		current int
	)
	err = tx.QueryRowContext(ctx, query, args...).Scan(&from, &current)
	if err == sql.ErrNoRows {
		return nil, &httpError{http.StatusNotFound, "application not found"}
	}
	if err != nil {
		return nil, err
	}
	if current != version {
		return nil, &httpError{http.StatusConflict, "stale application version"}
	}
	if actorType == "guardian" && !model.GuardianCanTransition(model.ApplicationStatus(from), to) {
		return nil, &httpError{http.StatusForbidden, "guardian cannot perform this transition"}
	}
	if err := model.ValidateTransition(model.ApplicationStatus(from), to, reason); err != nil {
		return nil, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}

	snapshot := ""
	if to == model.StatusSubmitted {
		snapshot = `,submitted_at=now(),submitted_snapshot=jsonb_build_object('answers',answers,'version',version)`
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE applications SET status=$1,version=version+1,updated_at=now()`+snapshot+` WHERE id=$2`,
		to, applicationID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO application_state_events(school_id,application_id,from_status,to_status,actor_type,actor_id,reason,version)
		 VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8)`,
		schoolID, applicationID, from, to, actorType, actorID, reason, current+1); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,before_data,after_data,reason)
		 VALUES($1,'application',$2,'state_transition',$3,$4,$5,jsonb_build_object('status',$6),jsonb_build_object('status',$7),NULLIF($8,''))`,
		schoolID, applicationID, actorType, actorID, actorName, from, to, reason); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"id": applicationID, "status": to, "version": current + 1}, nil
}
