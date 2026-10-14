package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// Per-applicant test / interview schedules (BUG-105). Setting or moving a
// schedule notifies the guardian; one schedule per kind per application.

type scheduleInput struct {
	Kind        string `json:"kind"`
	ScheduledAt string `json:"scheduled_at"`
	Location    string `json:"location"`
	Notes       string `json:"notes"`
}

func (a *App) selectionSchedules(ctx context.Context, q queryer, schoolID, applicationID string) ([]map[string]any, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id,kind,scheduled_at,location,notes,created_by_name,updated_at FROM selection_schedules
		 WHERE school_id=$1 AND application_id=$2 ORDER BY scheduled_at`, schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, kind, location, notes, by string
			at, updated                   time.Time
		)
		if err := rows.Scan(&id, &kind, &at, &location, &notes, &by, &updated); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "kind": kind, "label": model.ScheduleKindLabel(kind), "scheduled_at": at,
			"location": location, "notes": notes, "created_by_name": by, "updated_at": updated})
	}
	return out, rows.Err()
}

// ListSchedules is GET /admin/applications/{id}/schedules.
func (a *App) ListSchedules(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	out, err := a.selectionSchedules(r.Context(), a.DB, common.EffectiveSchoolID(r), param(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// GuardianSchedules is GET /guardian/applications/{id}/schedules.
func (a *App) GuardianSchedules(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	var owned bool
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM applications a JOIN applicants ap ON ap.id=a.applicant_id
		  WHERE a.id=$1 AND a.school_id=$2 AND ap.guardian_id=$3)`, param(r, "id"), c.SchoolID, c.GuardianID).Scan(&owned); err != nil {
		writeErr(w, err)
		return
	}
	if !owned {
		common.WriteError(w, http.StatusNotFound, "pendaftaran tidak ditemukan")
		return
	}
	out, err := a.selectionSchedules(r.Context(), a.DB, c.SchoolID, param(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// UpsertSchedule is POST /admin/applications/{id}/schedules.
func (a *App) UpsertSchedule(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	applicationID := param(r, "id")
	var in scheduleInput
	if !common.Decode(w, r, &in) {
		return
	}
	at, err := parseTime(in.ScheduledAt)
	if err != nil || at.IsZero() {
		common.WriteError(w, http.StatusUnprocessableEntity, "jadwal tidak valid")
		return
	}
	if err := model.ValidateSchedule(in.Kind, at, time.Now()); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	in.Location = strings.TrimSpace(in.Location)
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	err = tx.QueryRowContext(r.Context(),
		`SELECT status FROM applications WHERE id=$1 AND school_id=$2 FOR UPDATE`, applicationID, schoolID).Scan(&status)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "pendaftaran tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if !model.SchedulableStatus(status) {
		common.WriteError(w, http.StatusUnprocessableEntity, "jadwal tes/wawancara hanya untuk pendaftar yang sedang verifikasi atau siap tes")
		return
	}
	var id string
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO selection_schedules(school_id,application_id,kind,scheduled_at,location,notes,created_by,created_by_name)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (application_id,kind) DO UPDATE SET scheduled_at=EXCLUDED.scheduled_at,location=EXCLUDED.location,
		        notes=EXCLUDED.notes,created_by=EXCLUDED.created_by,created_by_name=EXCLUDED.created_by_name,updated_at=now()
		 RETURNING id`, schoolID, applicationID, in.Kind, at, in.Location, in.Notes, claims.UserID, claims.Name).Scan(&id); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data)
		 VALUES($1,'application',$2,'schedule_set','admin',$3,$4,$5)`, schoolID, applicationID, claims.UserID, claims.Name,
		jsonRaw(map[string]any{"schedule_id": id, "kind": in.Kind, "scheduled_at": at, "location": in.Location})); err != nil {
		writeErr(w, err)
		return
	}
	body := "Jadwal " + strings.ToLower(model.ScheduleKindLabel(in.Kind)) + " ananda: " + formatDateWIB(at)
	if in.Location != "" {
		body += ", lokasi " + in.Location
	}
	body += "."
	if strings.TrimSpace(in.Notes) != "" {
		body += " Catatan: " + strings.TrimSpace(in.Notes)
	}
	// The dedupe key includes the time, so a moved schedule notifies again.
	if err = a.notify(r.Context(), tx, schoolID, applicationID, "schedule", "Jadwal "+model.ScheduleKindLabel(in.Kind), body,
		"schedule:"+id+":"+at.UTC().Format(time.RFC3339), nil); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	out, err := a.selectionSchedules(r.Context(), a.DB, schoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
