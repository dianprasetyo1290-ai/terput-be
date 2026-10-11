package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"skci/admission/internal/client"
	"skci/admission/internal/common"
	"skci/admission/internal/conversion"
)

// ---------------------------------------------------------------------------
// Identity adapters
//
// The payloads carry real guardian/applicant data and stable, deterministic
// external references (the admission guardian id / applicant id, and the
// application registration number as the conversion ref). Identity's internal
// endpoints are upserts keyed on those refs, so replaying a step is safe.
// ---------------------------------------------------------------------------

type identityAdapter struct{ a *App }

func (x identityAdapter) UpsertGuardian(ctx context.Context, r conversion.Record) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := x.a.Identity.Post(ctx, "/internal/guardians/upsert", r.Key+":guardian", map[string]any{
		"school_id":      r.SchoolID,
		"source":         conversion.Source,
		"external_ref":   r.GuardianRef,
		"conversion_ref": r.ConversionRef,
		"name":           r.GuardianName,
		"email":          r.GuardianEmail,
		"phone":          r.GuardianPhone,
	}, &out)
	return out.ID, err
}

func (x identityAdapter) UpsertStudent(ctx context.Context, r conversion.Record) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := x.a.Identity.Post(ctx, "/internal/students/create-or-link", r.Key+":student", map[string]any{
		"school_id":      r.SchoolID,
		"source":         conversion.Source,
		"external_ref":   r.StudentRef,
		"conversion_ref": r.ConversionRef,
		"name":           r.ApplicantName,
		"email":          r.ApplicantEmail,
		"phone":          r.ApplicantPhone,
		"nisn":           r.ApplicantNISN,
	}, &out)
	return out.ID, err
}

func (x identityAdapter) LinkGuardian(ctx context.Context, r conversion.Record) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := x.a.Identity.Post(ctx, "/internal/guardian-links", r.Key+":link", map[string]any{
		"school_id":        r.SchoolID,
		"source":           conversion.Source,
		"conversion_ref":   r.ConversionRef,
		"guardian_user_id": r.GuardianID,
		"student_user_id":  r.StudentID,
		"relationship":     r.Relationship,
		"is_primary":       true,
	}, &out)
	return out.ID, err
}

// academicAdapter provisions the optional enrollment. A 404 means academic has
// not shipped the endpoint yet, which the saga treats as a skipped step rather
// than a failure.
type academicAdapter struct{ a *App }

func (x academicAdapter) Enroll(ctx context.Context, r conversion.Record) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := x.a.Academic.Post(ctx, "/internal/admission/enrollments/upsert", r.Key+":enrollment", map[string]any{
		"school_id":      r.SchoolID,
		"student_id":     r.StudentID,
		"application_id": r.ApplicationID,
		"conversion_ref": r.ConversionRef,
	}, &out)
	if client.NotFound(err) {
		return "", conversion.ErrNotAvailable
	}
	return out.ID, err
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

type conversionInput struct {
	ApplicationID string `json:"application_id"`
	Enroll        bool   `json:"enroll"`
}

// StartConversion is POST /admin/conversions. (The SPA only calls the list and
// retry endpoints; this start endpoint mirrors the retry so operations can kick
// off a conversion for a ready application.)
func (a *App) StartConversion(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in conversionInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.ApplicationID == "" {
		common.WriteError(w, http.StatusBadRequest, "application_id is required")
		return
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	record, err := a.runConversion(r, schoolID, in.ApplicationID, idempotencyKey, in.Enroll)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, conversionRecordView(record), nil)
}

// RetryConversion is POST /admin/conversions/{id}/retry.
func (a *App) RetryConversion(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	conversionID := param(r, "id")

	var applicationID string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT application_id FROM student_conversions WHERE id=$1 AND school_id=$2`, conversionID, schoolID).Scan(&applicationID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "conversion not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	record, err := a.runConversion(r, schoolID, applicationID, r.Header.Get("Idempotency-Key"), true)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, conversionRecordView(record), nil)
}

// ListConversions is GET /admin/conversions.
func (a *App) ListConversions(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")
	page, perPage := common.Page(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT sc.id,sc.application_id,ap.full_name,a.registration_number,sc.status,sc.current_step,
		        COALESCE(sc.last_error,''),sc.attempts,sc.updated_at
		 FROM student_conversions sc
		 JOIN applications a ON a.id=sc.application_id
		 JOIN applicants ap ON ap.id=a.applicant_id
		 WHERE sc.school_id=$1 AND ($2='' OR sc.status=$2)
		 ORDER BY sc.updated_at DESC LIMIT $3 OFFSET $4`,
		schoolID, status, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id, applicationID, applicantName, registrationNumber, convStatus, currentStep, lastError string
			attempts                                                                                 int
			updatedAt                                                                                any
		)
		if err := rows.Scan(&id, &applicationID, &applicantName, &registrationNumber, &convStatus, &currentStep, &lastError, &attempts, &updatedAt); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{
			"id":                  id,
			"application_id":      applicationID,
			"applicant_name":      applicantName,
			"registration_number": registrationNumber,
			"status":              convStatus,
			"attempts":            attempts,
			"updated_at":          updatedAt,
		}
		if currentStep != "" {
			item["current_step"] = currentStep
		}
		if lastError != "" {
			item["error"] = lastError
		}
		items = append(items, item)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": items}, nil)
}

// runConversion upserts the persisted conversion row, loads the real payload
// and drives the saga, resuming from the last completed step.
func (a *App) runConversion(r *http.Request, schoolID, applicationID, idempotencyKey string, enroll bool) (conversion.Record, error) {
	if idempotencyKey == "" {
		idempotencyKey = "conversion:" + applicationID
	}
	record, err := a.loadConversionRecord(r, schoolID, applicationID, idempotencyKey)
	if err != nil {
		return conversion.Record{}, err
	}
	persist := func(ctx context.Context, rec conversion.Record) error {
		return a.persistConversion(ctx, rec)
	}
	record, err = conversion.Run(r.Context(), record, identityAdapter{a}, academicAdapter{a}, persist, enroll)
	if err != nil {
		// The failed state is already persisted by the saga; surface a 502 so the
		// admin UI shows the conversion as retryable.
		return record, &httpError{http.StatusBadGateway, "conversion paused and can be retried: " + err.Error()}
	}
	if record.Status == conversion.StatusCompleted {
		if _, err = a.DB.ExecContext(r.Context(),
			`UPDATE applications SET status='converted',version=version+1,updated_at=now()
			 WHERE id=$1 AND school_id=$2 AND status='reregistration'`, applicationID, schoolID); err != nil {
			return record, err
		}
		a.audit(r.Context(), schoolID, "conversion", record.ID, "conversion_completed", "admin", "", "", "", nil,
			map[string]any{"status": record.Status, "student_user_id": record.StudentID})
	}
	return record, nil
}

// loadConversionRecord upserts the conversion row (guarding conversion
// readiness on first creation) and hydrates the saga record from real data.
func (a *App) loadConversionRecord(r *http.Request, schoolID, applicationID, idempotencyKey string) (conversion.Record, error) {
	var record conversion.Record
	var (
		guardianStep, studentStep, linkStep, enrollmentStep []byte
	)
	err := a.DB.QueryRowContext(r.Context(),
		`INSERT INTO student_conversions(school_id,application_id,idempotency_key)
		 SELECT $1,a.id,$2 FROM applications a
		 WHERE a.id=$3 AND a.school_id=$1 AND a.status IN('reregistration','converted')
		   AND EXISTS(SELECT 1 FROM decisions d WHERE d.application_id=a.id AND d.decision='offered')
		   AND NOT EXISTS(SELECT 1 FROM application_reregistration_items ari
		                  JOIN reregistration_items ri ON ri.id=ari.item_id
		                  WHERE ari.application_id=a.id AND ri.required AND ari.completed_at IS NULL)
		 ON CONFLICT(application_id) DO UPDATE SET attempts=student_conversions.attempts+1,updated_at=now()
		 RETURNING id,school_id,application_id,idempotency_key,status,current_step,COALESCE(last_error,''),
		           COALESCE(identity_guardian_id,''),COALESCE(student_user_id,''),COALESCE(guardian_link_id,''),
		           COALESCE(enrollment_id,''),guardian_step,student_step,link_step,enrollment_step,attempts`,
		schoolID, idempotencyKey, applicationID).Scan(
		&record.ID, &record.SchoolID, &record.ApplicationID, &record.Key, &record.Status, &record.CurrentStep, &record.LastError,
		&record.GuardianID, &record.StudentID, &record.LinkID, &record.EnrollmentID,
		&guardianStep, &studentStep, &linkStep, &enrollmentStep, new(int))
	if err == sql.ErrNoRows {
		return record, &httpError{http.StatusUnprocessableEntity, "application is not conversion-ready"}
	}
	if err != nil {
		return record, err
	}

	record.Steps = map[string]*conversion.StepState{
		conversion.StepGuardian:   decodeStep(guardianStep),
		conversion.StepStudent:    decodeStep(studentStep),
		conversion.StepLink:       decodeStep(linkStep),
		conversion.StepEnrollment: decodeStep(enrollmentStep),
	}
	record.ConversionRef = "app:" + applicationID
	record.GuardianRef = "guardian:" + record.GuardianID
	record.StudentRef = "applicant:" + record.ApplicationID

	if err := a.hydrateConversionPayload(r, &record); err != nil {
		return record, err
	}
	return record, nil
}

func (a *App) hydrateConversionPayload(r *http.Request, record *conversion.Record) error {
	var (
		applicantID, applicantName, nisn, relationship string
		guardianID, guardianName                       string
	)
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT ap.id,ap.full_name,COALESCE(ap.nisn,''),COALESCE(ap.guardian_relationship,''),
		        g.id,COALESCE(g.name,'')
		 FROM applications app
		 JOIN applicants ap ON ap.id=app.applicant_id
		 JOIN guardians g ON g.id=ap.guardian_id
		 WHERE app.id=$1 AND app.school_id=$2`, record.ApplicationID, record.SchoolID).
		Scan(&applicantID, &applicantName, &nisn, &relationship, &guardianID, &guardianName)
	if err != nil {
		return err
	}
	record.GuardianRef = "guardian:" + guardianID
	record.StudentRef = "applicant:" + applicantID
	record.GuardianName = guardianName
	record.ApplicantName = applicantName
	record.ApplicantNISN = nisn
	record.Relationship = relationship
	if record.Relationship == "" {
		record.Relationship = "orang_tua"
	}

	// Guardian contact: prefer a verified email, fall back to a synthesized,
	// deterministic address when only a phone is on file (identity requires an
	// email on its user upsert).
	var email, phone string
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT kind,normalized_value FROM guardian_contacts WHERE school_id=$1 AND guardian_id=$2 ORDER BY verified_at DESC`,
		record.SchoolID, guardianID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return err
		}
		switch kind {
		case "email":
			if email == "" {
				email = value
			}
		case "phone":
			if phone == "" {
				phone = value
			}
		}
	}
	record.GuardianPhone = phone
	if email != "" {
		record.GuardianEmail = email
	} else if phone != "" {
		record.GuardianEmail = sanitizeLocalPart(phone) + "@wali.ppdb.teratai.local"
	} else {
		record.GuardianEmail = sanitizeLocalPart(guardianID) + "@wali.ppdb.teratai.local"
	}

	// Applicants have no email of their own; identity needs one for the student
	// user, so derive a stable per-applicant address.
	record.ApplicantEmail = sanitizeLocalPart(applicantID) + "@siswa.ppdb.teratai.local"
	return nil
}

func sanitizeLocalPart(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "user"
	}
	return b.String()
}

func decodeStep(raw []byte) *conversion.StepState {
	step := &conversion.StepState{Status: conversion.StepPending}
	if len(raw) == 0 {
		return step
	}
	if err := json.Unmarshal(raw, step); err != nil {
		step.Status = conversion.StepPending
	}
	if step.Status == "" {
		step.Status = conversion.StepPending
	}
	return step
}

func (a *App) persistConversion(ctx context.Context, rec conversion.Record) error {
	guardianStep, _ := json.Marshal(rec.Step(conversion.StepGuardian))
	studentStep, _ := json.Marshal(rec.Step(conversion.StepStudent))
	linkStep, _ := json.Marshal(rec.Step(conversion.StepLink))
	enrollmentStep, _ := json.Marshal(rec.Step(conversion.StepEnrollment))
	_, err := a.DB.ExecContext(ctx,
		`UPDATE student_conversions SET status=$1,current_step=$2,last_error=NULLIF($3,''),
		        identity_guardian_id=NULLIF($4,''),student_user_id=NULLIF($5,''),guardian_link_id=NULLIF($6,''),
		        enrollment_id=NULLIF($7,''),guardian_step=$8,student_step=$9,link_step=$10,enrollment_step=$11,updated_at=now()
		 WHERE id=$12`,
		rec.Status, rec.CurrentStep, rec.LastError, rec.GuardianID, rec.StudentID, rec.LinkID, rec.EnrollmentID,
		guardianStep, studentStep, linkStep, enrollmentStep, rec.ID)
	return err
}

// conversionRecordView renders the persisted conversion for API responses.
func conversionRecordView(rec conversion.Record) map[string]any {
	return map[string]any{
		"id":               rec.ID,
		"application_id":   rec.ApplicationID,
		"status":           rec.Status,
		"guardian_user_id": rec.GuardianID,
		"student_user_id":  rec.StudentID,
		"guardian_link_id": rec.LinkID,
		"enrollment_id":    rec.EnrollmentID,
		"current_step":     rec.CurrentStep,
		"error":            rec.LastError,
		"steps":            rec.Steps,
	}
}
