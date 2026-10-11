package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"skci/admission/internal/common"
)

// ---------------------------------------------------------------------------
// Guardian dashboard
// ---------------------------------------------------------------------------

// GuardianDashboard is GET /guardian/dashboard.
func (a *App) GuardianDashboard(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)

	var name string
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT name FROM guardians WHERE id=$1 AND school_id=$2`, c.GuardianID, c.SchoolID).Scan(&name); err != nil {
		common.WriteError(w, http.StatusNotFound, "guardian not found")
		return
	}
	contact, contactType := a.primaryContact(r, c.SchoolID, c.GuardianID)
	applicants, err := a.listApplicants(r, c.SchoolID, c.GuardianID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to load applicants")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"guardian": map[string]any{
			"id":           c.GuardianID,
			"name":         name,
			"contact":      contact,
			"contact_type": contactType,
		},
		"applicants": applicants,
	}, nil)
}

func (a *App) primaryContact(r *http.Request, schoolID, guardianID string) (string, string) {
	var contact, kind string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT normalized_value,kind FROM guardian_contacts WHERE school_id=$1 AND guardian_id=$2 ORDER BY verified_at DESC LIMIT 1`,
		schoolID, guardianID).Scan(&contact, &kind)
	if err != nil {
		return "", ""
	}
	return contact, kind
}

// ---------------------------------------------------------------------------
// Applicants
// ---------------------------------------------------------------------------

// applicantInput mirrors the frontend ApplicantInput plus the read-only fields
// the SPA round-trips on edit (id).
type applicantInput struct {
	ID                   string `json:"id"`
	FullName             string `json:"full_name"`
	Nickname             string `json:"nickname"`
	NISN                 string `json:"nisn"`
	NIK                  string `json:"nik"`
	BirthPlace           string `json:"birth_place"`
	BirthDate            string `json:"birth_date"`
	Gender               string `json:"gender"`
	PreviousSchool       string `json:"previous_school"`
	Address              string `json:"address"`
	GuardianRelationship string `json:"guardian_relationship"`
}

// ListApplicants is GET /guardian/applicants.
func (a *App) ListApplicants(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicants, err := a.listApplicants(r, c.SchoolID, c.GuardianID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list applicants")
		return
	}
	common.WriteJSON(w, http.StatusOK, applicants, nil)
}

func (a *App) listApplicants(r *http.Request, schoolID, guardianID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT ap.id,ap.full_name,COALESCE(ap.nickname,''),COALESCE(ap.nisn,''),COALESCE(ap.nik,''),
		        COALESCE(to_char(ap.birth_date,'YYYY-MM-DD'),''),COALESCE(ap.birth_place,''),COALESCE(ap.gender,''),
		        COALESCE(ap.previous_school,''),COALESCE(ap.address,''),COALESCE(ap.guardian_relationship,''),
		        ap.updated_at,
		        COALESCE(app.status,''),COALESCE(app.registration_number,''),COALESCE(app.id::text,'')
		 FROM applicants ap
		 LEFT JOIN LATERAL (
		     SELECT a.id,a.status,a.registration_number FROM applications a
		     WHERE a.applicant_id=ap.id AND a.school_id=ap.school_id
		     ORDER BY a.created_at DESC LIMIT 1
		 ) app ON true
		 WHERE ap.school_id=$1 AND ap.guardian_id=$2 AND ap.deleted_at IS NULL
		 ORDER BY ap.created_at`, schoolID, guardianID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, err := scanApplicantRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanApplicantRow(row rowScanner) (map[string]any, error) {
	var (
		id, fullName, nickname, nisn, nik, birthDate, birthPlace, gender string
		previousSchool, address, relationship                            string
		updatedAt                                                        time.Time
		status, registrationNumber, applicationID                        string
	)
	if err := row.Scan(&id, &fullName, &nickname, &nisn, &nik, &birthDate, &birthPlace, &gender,
		&previousSchool, &address, &relationship, &updatedAt, &status, &registrationNumber, &applicationID); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id":                    id,
		"full_name":             fullName,
		"nickname":              nickname,
		"nisn":                  nisn,
		"nik":                   nik,
		"birth_date":            birthDate,
		"birth_place":           birthPlace,
		"gender":                gender,
		"previous_school":       previousSchool,
		"address":               address,
		"guardian_relationship": relationship,
		"updated_at":            updatedAt,
	}
	if status != "" {
		m["status"] = status
		m["registration_number"] = registrationNumber
		m["application_id"] = applicationID
	}
	return m, nil
}

// GetApplicant is GET /guardian/applicants/{id}.
func (a *App) GetApplicant(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	row := a.DB.QueryRowContext(r.Context(),
		`SELECT ap.id,ap.full_name,COALESCE(ap.nickname,''),COALESCE(ap.nisn,''),COALESCE(ap.nik,''),
		        COALESCE(to_char(ap.birth_date,'YYYY-MM-DD'),''),COALESCE(ap.birth_place,''),COALESCE(ap.gender,''),
		        COALESCE(ap.previous_school,''),COALESCE(ap.address,''),COALESCE(ap.guardian_relationship,''),
		        ap.updated_at,
		        COALESCE(app.status,''),COALESCE(app.registration_number,''),COALESCE(app.id::text,'')
		 FROM applicants ap
		 LEFT JOIN LATERAL (
		     SELECT a.id,a.status,a.registration_number FROM applications a
		     WHERE a.applicant_id=ap.id AND a.school_id=ap.school_id
		     ORDER BY a.created_at DESC LIMIT 1
		 ) app ON true
		 WHERE ap.id=$1 AND ap.guardian_id=$2 AND ap.school_id=$3 AND ap.deleted_at IS NULL`,
		param(r, "id"), c.GuardianID, c.SchoolID)
	m, err := scanApplicantRow(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "applicant not found")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to load applicant")
		return
	}
	common.WriteJSON(w, http.StatusOK, m, nil)
}

// CreateApplicant is POST /guardian/applicants.
func (a *App) CreateApplicant(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	var in applicantInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.FullName == "" || in.BirthDate == "" {
		common.WriteError(w, http.StatusBadRequest, "full_name and birth_date are required")
		return
	}
	var id string
	err := a.DB.QueryRowContext(r.Context(),
		`INSERT INTO applicants(school_id,guardian_id,full_name,nickname,nisn,nik,birth_date,birth_place,gender,previous_school,address,guardian_relationship,source)
		 VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,'')::date,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),'guardian')
		 RETURNING id`,
		c.SchoolID, c.GuardianID, in.FullName, in.Nickname, in.NISN, in.NIK, in.BirthDate, in.BirthPlace,
		in.Gender, in.PreviousSchool, in.Address, in.GuardianRelationship).Scan(&id)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to create applicant")
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateApplicant is PUT /guardian/applicants/{id}.
func (a *App) UpdateApplicant(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	var in applicantInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.FullName == "" || in.BirthDate == "" {
		common.WriteError(w, http.StatusBadRequest, "full_name and birth_date are required")
		return
	}
	id := param(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE applicants SET full_name=$1,nickname=NULLIF($2,''),nisn=NULLIF($3,''),nik=NULLIF($4,''),
		        birth_date=NULLIF($5,'')::date,birth_place=NULLIF($6,''),gender=NULLIF($7,''),
		        previous_school=NULLIF($8,''),address=NULLIF($9,''),guardian_relationship=NULLIF($10,''),updated_at=now()
		 WHERE id=$11 AND guardian_id=$12 AND school_id=$13 AND deleted_at IS NULL`,
		in.FullName, in.Nickname, in.NISN, in.NIK, in.BirthDate, in.BirthPlace, in.Gender,
		in.PreviousSchool, in.Address, in.GuardianRelationship, id, c.GuardianID, c.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to update applicant")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "applicant not found")
		return
	}
	resp := map[string]any{"id": id}
	var applicationID string
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT id FROM applications WHERE applicant_id=$1 AND school_id=$2 ORDER BY created_at DESC LIMIT 1`, id, c.SchoolID).
		Scan(&applicationID)
	if applicationID != "" {
		resp["application_id"] = applicationID
	}
	common.WriteJSON(w, http.StatusOK, resp, nil)
}

// DeleteApplicant is DELETE /guardian/applicants/{id}. Applicants that already
// have a non-draft application are locked.
func (a *App) DeleteApplicant(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	id := param(r, "id")

	var status string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT status FROM applications WHERE applicant_id=$1 AND school_id=$2 ORDER BY created_at DESC LIMIT 1`, id, c.SchoolID).
		Scan(&status)
	if err == nil && status != "draft" {
		common.WriteError(w, http.StatusConflict, "applicant already has an active application")
		return
	}
	if err != nil && err != sql.ErrNoRows {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete applicant")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE applicants SET deleted_at=now(),updated_at=now() WHERE id=$1 AND guardian_id=$2 AND school_id=$3 AND deleted_at IS NULL`,
		id, c.GuardianID, c.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete applicant")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "applicant not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true}, nil)
}

// ---------------------------------------------------------------------------
// Applications (guardian)
// ---------------------------------------------------------------------------

type createApplicationInput struct {
	ApplicantID string `json:"applicant_id"`
	PeriodID    string `json:"period_id"`
	WaveID      string `json:"wave_id"`
	TrackID     string `json:"track_id"`
}

// CreateApplication is POST /guardian/applications.
func (a *App) CreateApplication(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	var in createApplicationInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.ApplicantID == "" || in.PeriodID == "" || in.WaveID == "" || in.TrackID == "" {
		common.WriteError(w, http.StatusBadRequest, "applicant_id, period_id, wave_id and track_id are required")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create application")
		return
	}
	defer func() { _ = tx.Rollback() }()

	var owned bool
	if err = tx.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM applicants WHERE id=$1 AND guardian_id=$2 AND school_id=$3 AND deleted_at IS NULL)`,
		in.ApplicantID, c.GuardianID, c.SchoolID).Scan(&owned); err != nil || !owned {
		common.WriteError(w, http.StatusNotFound, "applicant not found")
		return
	}

	var id, registrationNumber string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO applications(school_id,applicant_id,period_id,wave_id,track_id,registration_number)
		 VALUES($1,$2,$3,$4,$5,'PPDB-'||to_char(now(),'YYYY')||'-'||upper(substr(replace(gen_random_uuid()::text,'-',''),1,8)))
		 RETURNING id,registration_number`,
		c.SchoolID, in.ApplicantID, in.PeriodID, in.WaveID, in.TrackID).Scan(&id, &registrationNumber)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "an application for this period already exists")
			return
		}
		common.WriteError(w, http.StatusBadRequest, "invalid period, wave or track")
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO application_state_events(school_id,application_id,to_status,actor_type,actor_id,version)
		 VALUES($1,$2,'draft','guardian',$3,1)`, c.SchoolID, id, c.GuardianID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create application")
		return
	}
	if err = tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create application")
		return
	}
	a.audit(r.Context(), c.SchoolID, "application", id, "application_created", "guardian", c.GuardianID, "", "", nil,
		map[string]any{"status": "draft", "registration_number": registrationNumber})

	common.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":                  id,
		"status":              "draft",
		"version":             1,
		"registration_number": registrationNumber,
	}, nil)
}

// ApplicationDetail is GET /guardian/applications/{id}.
func (a *App) ApplicationDetail(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	result, err := a.applicationDetail(r, c.SchoolID, param(r, "id"), "guardian", c.GuardianID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to load application")
		return
	}
	common.WriteJSON(w, http.StatusOK, result, nil)
}

// applicationDetail loads an application plus its applicant and timeline.
// When scope is "guardian" the applicant must belong to guardianID.
func (a *App) applicationDetail(r *http.Request, schoolID, applicationID, scope, ownerID string) (map[string]any, error) {
	query := `SELECT a.id,a.applicant_id,a.period_id,a.wave_id,a.track_id,a.registration_number,a.status,a.version,
	                 a.submitted_at,a.answers,
	                 ap.full_name,COALESCE(ap.nickname,''),COALESCE(ap.nisn,''),COALESCE(ap.nik,''),
	                 COALESCE(to_char(ap.birth_date,'YYYY-MM-DD'),''),COALESCE(ap.birth_place,''),COALESCE(ap.gender,''),
	                 COALESCE(ap.previous_school,''),COALESCE(ap.address,''),COALESCE(ap.guardian_relationship,'')
	          FROM applications a JOIN applicants ap ON ap.id=a.applicant_id
	          WHERE a.id=$1 AND a.school_id=$2`
	args := []any{applicationID, schoolID}
	if scope == "guardian" {
		query += ` AND ap.guardian_id=$3`
		args = append(args, ownerID)
	}

	var (
		id, applicantID, periodID, waveID, trackID, registrationNumber, status string
		version                                                                int
		submittedAt                                                            sql.NullTime
		answers                                                                []byte
		fullName, nickname, nisn, nik, birthDate, birthPlace                   string
		gender, previousSchool, address, relationship                          string
	)
	err := a.DB.QueryRowContext(r.Context(), query, args...).Scan(
		&id, &applicantID, &periodID, &waveID, &trackID, &registrationNumber, &status, &version,
		&submittedAt, &answers,
		&fullName, &nickname, &nisn, &nik, &birthDate, &birthPlace, &gender, &previousSchool, &address, &relationship)
	if err != nil {
		return nil, err
	}

	applicant := map[string]any{
		"id":                    applicantID,
		"full_name":             fullName,
		"nickname":              nickname,
		"nisn":                  nisn,
		"nik":                   nik,
		"birth_date":            birthDate,
		"birth_place":           birthPlace,
		"gender":                gender,
		"previous_school":       previousSchool,
		"address":               address,
		"guardian_relationship": relationship,
	}
	result := map[string]any{
		"id":                  id,
		"applicant_id":        applicantID,
		"applicant":           applicant,
		"period_id":           periodID,
		"wave_id":             waveID,
		"track_id":            trackID,
		"registration_number": registrationNumber,
		"status":              status,
		"version":             version,
		"answers":             json.RawMessage(answers),
	}
	if submittedAt.Valid {
		result["submitted_at"] = submittedAt.Time
	}
	timeline, err := a.applicationTimeline(r, schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	result["timeline"] = timeline
	return result, nil
}

func (a *App) applicationTimeline(r *http.Request, schoolID, applicationID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,to_status,COALESCE(reason,''),created_at FROM application_state_events
		 WHERE school_id=$1 AND application_id=$2 ORDER BY id`, schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id         int64
			status     string
			reason     string
			occurredAt time.Time
		)
		if err := rows.Scan(&id, &status, &reason, &occurredAt); err != nil {
			return nil, err
		}
		event := map[string]any{
			"id":          id,
			"status":      status,
			"title":       statusTitle(status),
			"occurred_at": occurredAt,
		}
		if reason != "" {
			event["description"] = reason
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func statusTitle(status string) string {
	switch status {
	case "draft":
		return "Draf dibuat"
	case "submitted":
		return "Pendaftaran dikirim"
	case "document_review":
		return "Verifikasi berkas"
	case "eligible_for_test":
		return "Siap mengikuti tes"
	case "tested":
		return "Tes selesai"
	case "ranked":
		return "Masuk peringkat seleksi"
	case "offered":
		return "Ditawarkan penerimaan"
	case "waitlisted":
		return "Daftar tunggu"
	case "rejected":
		return "Ditolak"
	case "reregistration":
		return "Daftar ulang"
	case "converted":
		return "Menjadi siswa"
	case "withdrawn":
		return "Dibatalkan"
	default:
		return status
	}
}
