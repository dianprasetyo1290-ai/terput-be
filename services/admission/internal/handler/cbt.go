package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"time"

	"skci/admission/internal/common"
)

type cbtScope struct {
	ApplicantID        string
	RegistrationNumber string
	ApplicantName      string
	TestStart          time.Time
	TestEnd            time.Time
	HasWindow          bool
}

func (a *App) cbtScope(r *http.Request, schoolID, applicationID string) (cbtScope, error) {
	var scope cbtScope
	var start, end sql.NullTime
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT a.applicant_id,a.registration_number,ap.full_name,w.test_start,w.test_end
		 FROM applications a
		 JOIN applicants ap ON ap.id=a.applicant_id
		 LEFT JOIN admission_waves w ON w.id=a.wave_id
		 WHERE a.id=$1 AND a.school_id=$2`, applicationID, schoolID).
		Scan(&scope.ApplicantID, &scope.RegistrationNumber, &scope.ApplicantName, &start, &end)
	if err == sql.ErrNoRows {
		return scope, &httpError{http.StatusNotFound, "pendaftaran tidak ditemukan"}
	}
	if err != nil {
		return scope, err
	}
	if start.Valid && end.Valid && end.Time.After(start.Time) {
		scope.TestStart = start.Time
		scope.TestEnd = end.Time
		scope.HasWindow = true
	}
	return scope, nil
}

// GetCBT is GET /guardian/applications/{id}/cbt.
func (a *App) GetCBT(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "pendaftaran tidak ditemukan")
		return
	}
	scope, err := a.cbtScope(r, c.SchoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}

	var (
		id, status  string
		score       sql.NullInt64
		scheduledAt sql.NullTime
	)
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT id,status,score,COALESCE(scheduled_at,now()) FROM admission_cbt_participants WHERE application_id=$1 AND school_id=$2`,
		applicationID, c.SchoolID).Scan(&id, &status, &score, &scheduledAt)
	hasParticipant := err == nil
	if err != nil && err != sql.ErrNoRows {
		writeErr(w, err)
		return
	}
	if !hasParticipant && !scope.HasWindow {
		common.WriteError(w, http.StatusNotFound, "jadwal CBT belum tersedia untuk pendaftaran ini")
		return
	}

	start, end := scope.TestStart, scope.TestEnd
	if hasParticipant && scheduledAt.Valid {
		start = scheduledAt.Time
	}
	view := map[string]any{
		"id":             id,
		"application_id": applicationID,
		"title":          "Tes Seleksi PPDB",
		"start_at":       start,
		"end_at":         end,
		"location":       "Online",
		"status":         cbtStatus(hasParticipant, status, start, end),
		"configured":     a.cbtConfigured(),
	}
	if !hasParticipant {
		view["id"] = applicationID
	}
	if score.Valid {
		view["score"] = score.Int64
	}
	common.WriteJSON(w, http.StatusOK, view, nil)
}

// cbtConfigured reports whether applicants can be sent to the exam service:
// both the internal API and the public entry URL must be set.
func (a *App) cbtConfigured() bool {
	return a.Exam.Configured() && a.Config.ExamPublicURL != ""
}

func cbtStatus(hasParticipant bool, status string, start, end time.Time) string {
	if hasParticipant && status != "" {
		return status
	}
	now := time.Now()
	switch {
	case !start.IsZero() && now.Before(start):
		return "scheduled"
	case !end.IsZero() && now.After(end):
		return "missed"
	default:
		return "available"
	}
}

// EnterCBT is POST /guardian/applications/{id}/cbt/enter. It provisions the
// applicant as an exam participant and issues a scoped access token.
func (a *App) EnterCBT(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "pendaftaran tidak ditemukan")
		return
	}
	if !a.cbtConfigured() {
		common.WriteError(w, http.StatusServiceUnavailable, "layanan ujian CBT belum dikonfigurasi; hubungi panitia PPDB")
		return
	}
	scope, err := a.cbtScope(r, c.SchoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}

	start, end := scope.TestStart, scope.TestEnd
	if !scope.HasWindow {
		start = time.Now()
		end = start.Add(2 * time.Hour)
	}

	var exam struct {
		ID string `json:"id"`
	}
	err = a.Exam.Post(r.Context(), "/internal/admission/exams", "admission-exam:"+scope.RegistrationNumber, map[string]any{
		"school_id":          c.SchoolID,
		"external_reference": scope.RegistrationNumber,
		"title":              "Tes Seleksi PPDB " + scope.RegistrationNumber,
		"start_at":           start.Format(time.RFC3339),
		"end_at":             end.Format(time.RFC3339),
		"admission_context":  map[string]any{"application_id": applicationID},
	}, &exam)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "ujian CBT gagal disiapkan, coba lagi beberapa saat")
		return
	}

	participantPayload := map[string]any{
		"school_id":          c.SchoolID,
		"applicant_id":       scope.ApplicantID,
		"external_reference": applicationID,
		"display_name":       scope.ApplicantName,
	}
	if err = a.Exam.Post(r.Context(), "/internal/admission/exams/"+exam.ID+"/participants", "admission-participant:"+applicationID, participantPayload, nil); err != nil {
		common.WriteError(w, http.StatusBadGateway, "peserta CBT gagal didaftarkan, coba lagi beberapa saat")
		return
	}

	var access struct {
		AccessToken string    `json:"access_token"`
		ExpiresAt   time.Time `json:"expires_at"`
	}
	if err = a.Exam.Post(r.Context(), "/internal/admission/exams/"+exam.ID+"/participants/access", "admission-access:"+applicationID, map[string]any{
		"school_id":    c.SchoolID,
		"applicant_id": scope.ApplicantID,
		"ttl_seconds":  3600,
	}, &access); err != nil {
		common.WriteError(w, http.StatusBadGateway, "akses CBT gagal diterbitkan, coba lagi beberapa saat")
		return
	}

	if err = a.upsertCBTParticipant(r.Context(), c.SchoolID, applicationID, exam.ID, scope.ApplicantID, "available", start); err != nil {
		writeErr(w, err)
		return
	}

	entryURL := a.Config.ExamPublicURL + "?token=" + url.QueryEscape(access.AccessToken)
	a.audit(r.Context(), c.SchoolID, "cbt", applicationID, "cbt_entered", "guardian", c.GuardianID, "", "", nil,
		map[string]any{"exam_id": exam.ID})
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"entry_url":    entryURL,
		"access_token": access.AccessToken,
		"expires_at":   access.ExpiresAt,
	}, nil)
}

func (a *App) upsertCBTParticipant(ctx context.Context, schoolID, applicationID, examID, participantRef, status string, scheduledAt time.Time) error {
	_, err := a.DB.ExecContext(ctx,
		`INSERT INTO admission_cbt_participants(school_id,application_id,exam_id,participant_ref,status,scheduled_at)
		 VALUES($1,$2,$3,$4,$5,$6)
		 ON CONFLICT(application_id) DO UPDATE SET exam_id=EXCLUDED.exam_id,participant_ref=EXCLUDED.participant_ref,
		     status=EXCLUDED.status,scheduled_at=EXCLUDED.scheduled_at,updated_at=now()`,
		schoolID, applicationID, examID, participantRef, status, nullTime(scheduledAt))
	return err
}
