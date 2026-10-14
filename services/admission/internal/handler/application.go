package handler

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

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

// writeErr maps an error to the response envelope. Domain errors keep their
// status; database constraint failures become a generic Indonesian 409/422 and
// anything else a generic 500 — raw SQL text is only ever logged, never
// returned to the client (BUG-099).
func writeErr(w http.ResponseWriter, err error) {
	status, msg := errorResponse(err)
	if status >= http.StatusInternalServerError {
		log.Printf("admission: internal error: %v", err)
	}
	common.WriteError(w, status, msg)
}

func errorResponse(err error) (int, string) {
	var he *httpError
	if errors.As(err, &he) {
		return he.status, he.msg
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return http.StatusConflict, "data yang sama sudah ada"
		case "23514":
			return http.StatusUnprocessableEntity, "nilai tidak valid: melanggar aturan validasi data"
		case "23502":
			return http.StatusUnprocessableEntity, "data wajib belum diisi"
		case "23503":
			return http.StatusUnprocessableEntity, "data rujukan tidak ditemukan"
		case "22P02", "22007", "22008", "22003":
			return http.StatusUnprocessableEntity, "format data tidak valid"
		}
	}
	return http.StatusInternalServerError, "terjadi kesalahan pada server, silakan coba lagi"
}

// SubmitApplication is POST /guardian/applications/{id}/submit. It requires the
// version from the client (optimistic lock) and the domain guards: the
// registration window must be open, required documents present and the track
// quota not exhausted. The track row is locked so two guardians cannot both
// take the last seat (BUG-031). The form fee charge is issued in the same
// transaction (BR-PPDB-06).
func (a *App) SubmitApplication(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	var in transitionInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Version < 1 {
		common.WriteError(w, http.StatusUnprocessableEntity, "version wajib dikirim")
		return
	}
	applicationID := param(r, "id")
	if err := a.submit(r.Context(), c.SchoolID, c.GuardianID, applicationID, in.Version); err != nil {
		writeErr(w, err)
		return
	}
	detail, err := a.applicationDetail(r, c.SchoolID, applicationID, "guardian", c.GuardianID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, detail, nil)
}

func (a *App) submit(ctx context.Context, schoolID, guardianID, applicationID string, version int) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		trackID    string
		windowOpen bool
	)
	err = tx.QueryRowContext(ctx,
		`SELECT a.track_id,(p.status='open' AND now() BETWEEN w.registration_start AND w.registration_end)
		 FROM applications a
		 JOIN applicants ap ON ap.id=a.applicant_id
		 JOIN admission_periods p ON p.id=a.period_id
		 JOIN admission_waves w ON w.id=a.wave_id
		 WHERE a.id=$1 AND a.school_id=$2 AND ap.guardian_id=$3`,
		applicationID, schoolID, guardianID).Scan(&trackID, &windowOpen)
	if err == sql.ErrNoRows {
		return &httpError{http.StatusNotFound, "pendaftaran tidak ditemukan"}
	}
	if err != nil {
		return err
	}
	if !windowOpen {
		return &httpError{http.StatusUnprocessableEntity, "masa pendaftaran gelombang ini sudah ditutup"}
	}

	var missing []string
	rows, err := tx.QueryContext(ctx,
		`SELECT dr.name FROM document_requirements dr
		   JOIN applications a ON a.period_id=dr.period_id AND (dr.track_id IS NULL OR dr.track_id=a.track_id)
		   WHERE a.id=$1 AND dr.required AND dr.deleted_at IS NULL AND dr.stage='registration'
		     AND NOT EXISTS (SELECT 1 FROM application_documents d WHERE d.application_id=a.id AND d.requirement_id=dr.id)
		   ORDER BY dr.name`, applicationID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		missing = append(missing, name)
	}
	rows.Close()
	if len(missing) > 0 {
		return &httpError{http.StatusUnprocessableEntity, "dokumen wajib belum lengkap: " + strings.Join(missing, ", ")}
	}

	// Seat lock: serialise submissions per track, then count every
	// application that still competes for a seat.
	var quota, active int
	if err = tx.QueryRowContext(ctx,
		`SELECT quota FROM admission_tracks WHERE id=$1 AND school_id=$2 FOR UPDATE`, trackID, schoolID).Scan(&quota); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx,
		`SELECT count(*) FROM applications WHERE school_id=$1 AND track_id=$2 AND id<>$3
		   AND status NOT IN('draft','withdrawn','rejected','dropped')`, schoolID, trackID, applicationID).Scan(&active); err != nil {
		return err
	}
	if err = model.CheckQuota(quota, active); err != nil {
		return &httpError{http.StatusUnprocessableEntity, "Kuota jalur ini sudah penuh; pendaftaran tidak dapat dikirim."}
	}

	if _, err = a.transitionTx(ctx, tx, schoolID, applicationID, "guardian", guardianID, "",
		model.StatusSubmitted, "", version, guardianID); err != nil {
		return err
	}
	chargeID, err := a.issueChargeTx(ctx, tx, schoolID, applicationID, stageRegistration, issueOptions{})
	if err != nil {
		return err
	}
	var net int64
	var due sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT net_idr,due_at FROM charges WHERE id=$1`, chargeID).Scan(&net, &due); err != nil {
		return err
	}
	body := "Pendaftaran telah dikirim."
	if net > 0 {
		body += " Silakan bayar biaya formulir sebesar " + formatIDR(net)
		if due.Valid {
			body += " paling lambat " + formatDateWIB(due.Time)
		}
		body += "; pendaftaran gugur bila tidak dibayar sampai batas waktu."
	}
	if err = a.notify(ctx, tx, schoolID, applicationID, "application_submitted", "Pendaftaran terkirim", body,
		"submitted:"+applicationID, map[string]any{"charge_id": chargeID}); err != nil {
		return err
	}
	// A zero-fee form (or one already settled) goes straight to review.
	if settled, _, _, err := stageSettled(ctx, tx, schoolID, applicationID, stageRegistration); err != nil {
		return err
	} else if settled {
		if err = a.afterPaymentVerified(ctx, tx, schoolID, applicationID, stageRegistration); err != nil {
			return err
		}
	}
	return tx.Commit()
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

	// The test stage needs the form fee settled and every required document
	// accepted (BUG-033, BUG-107); the issues are listed so the committee
	// knows what is still missing.
	issues, err := eligibilityIssues(r.Context(), a.DB, schoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(issues) > 0 {
		writeErr(w, issuesError("Belum dapat dinyatakan siap tes:", issues))
		return
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	// From "submitted" the committee first opens the document review stage;
	// mark_eligible then advances that stage to eligible_for_test.
	if model.ApplicationStatus(status) == model.StatusSubmitted {
		if _, err = a.transitionTx(r.Context(), tx, schoolID, applicationID, "admin", claims.UserID, claims.Name,
			model.StatusDocumentReview, "", in.Version, ""); err != nil {
			writeErr(w, err)
			return
		}
		in.Version++
	}
	if _, err = a.transitionTx(r.Context(), tx, schoolID, applicationID, "admin", claims.UserID, claims.Name,
		model.StatusEligibleForTest, in.Reason, in.Version, ""); err != nil {
		writeErr(w, err)
		return
	}
	if err = a.notify(r.Context(), tx, schoolID, applicationID, "eligible_for_test", "Berkas lengkap, siap mengikuti tes",
		"Berkas pendaftaran telah diverifikasi dan dinyatakan lengkap. Silakan pantau jadwal tes di portal PPDB.",
		"eligible:"+applicationID, nil); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
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
	out, err := a.transitionTx(ctx, tx, schoolID, applicationID, actorType, actorID, actorName, to, reason, version, guardianID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// transitionTx is transition inside a caller-owned transaction. version <= 0
// skips the optimistic-lock check (system jobs act on the current row).
func (a *App) transitionTx(ctx context.Context, tx *sql.Tx, schoolID, applicationID, actorType, actorID, actorName string, to model.ApplicationStatus, reason string, version int, guardianID string) (map[string]any, error) {
	query := `SELECT a.status,a.version FROM applications a`
	args := []any{applicationID, schoolID}
	if actorType == "guardian" {
		query += ` JOIN applicants ap ON ap.id=a.applicant_id WHERE a.id=$1 AND a.school_id=$2 AND ap.guardian_id=$3 FOR UPDATE OF a`
		args = append(args, guardianID)
	} else {
		query += ` WHERE a.id=$1 AND a.school_id=$2 FOR UPDATE`
	}

	var (
		from    string
		current int
	)
	err := tx.QueryRowContext(ctx, query, args...).Scan(&from, &current)
	if err == sql.ErrNoRows {
		return nil, &httpError{http.StatusNotFound, "application not found"}
	}
	if err != nil {
		return nil, err
	}
	if version > 0 && current != version {
		return nil, &httpError{http.StatusConflict, "data pendaftaran sudah berubah, muat ulang halaman"}
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
	if to == model.StatusDropped {
		snapshot = `,dropped_at=now(),drop_reason=NULLIF($3,'')`
	}
	updateArgs := []any{to, applicationID}
	if to == model.StatusDropped {
		updateArgs = append(updateArgs, reason)
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE applications SET status=$1,version=version+1,updated_at=now()`+snapshot+` WHERE id=$2`,
		updateArgs...); err != nil {
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
		 VALUES($1,'application',$2,'state_transition',$3,$4,$5,jsonb_build_object('status',$6::text),jsonb_build_object('status',$7::text),NULLIF($8,''))`,
		schoolID, applicationID, actorType, actorID, actorName, from, to, reason); err != nil {
		return nil, err
	}
	return map[string]any{"id": applicationID, "status": to, "version": current + 1, "from": from}, nil
}
