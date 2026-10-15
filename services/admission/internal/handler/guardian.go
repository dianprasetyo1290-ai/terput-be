package handler

import (
	"context"
	"database/sql"
	"log"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
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
		common.WriteError(w, http.StatusNotFound, "akun orang tua tidak ditemukan")
		return
	}
	contact, contactType := a.primaryContact(r, c.SchoolID, c.GuardianID)
	applicants, err := a.listApplicants(r, c.SchoolID, c.GuardianID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "daftar calon siswa gagal dimuat")
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
	ID                   string         `json:"id"`
	FullName             string         `json:"full_name"`
	Nickname             string         `json:"nickname"`
	NISN                 string         `json:"nisn"`
	NIK                  string         `json:"nik"`
	BirthPlace           string         `json:"birth_place"`
	BirthDate            string         `json:"birth_date"`
	Gender               string         `json:"gender"`
	PreviousSchool       string         `json:"previous_school"`
	Address              string         `json:"address"`
	GuardianRelationship string         `json:"guardian_relationship"`
	CustomFields         map[string]any `json:"custom_fields"`
}

// validate normalises and checks the applicant biodata (BUG-103/121).
func (in *applicantInput) validate() error {
	in.FullName = strings.TrimSpace(in.FullName)
	in.NIK = strings.TrimSpace(in.NIK)
	if in.FullName == "" || in.BirthDate == "" {
		return &httpError{http.StatusUnprocessableEntity, "nama lengkap dan tanggal lahir wajib diisi"}
	}
	birth, err := time.Parse("2006-01-02", in.BirthDate)
	if err != nil || birth.After(time.Now()) {
		return &httpError{http.StatusUnprocessableEntity, "tanggal lahir tidak valid"}
	}
	if err := model.ValidateNIK(in.NIK); err != nil {
		return &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	return nil
}

// ListApplicants is GET /guardian/applicants.
func (a *App) ListApplicants(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicants, err := a.listApplicants(r, c.SchoolID, c.GuardianID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "daftar calon siswa gagal dimuat")
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
		        COALESCE(app.status,''),COALESCE(app.registration_number,''),COALESCE(app.id::text,''),ap.custom_fields
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
		// Lists never carry the full NIK; the guardian reads it on the
		// applicant detail (GET /guardian/applicants/{id}) to edit it.
		m["nik"] = model.MaskNIK(m["nik"].(string))
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
		custom                                                           []byte
	)
	if err := row.Scan(&id, &fullName, &nickname, &nisn, &nik, &birthDate, &birthPlace, &gender,
		&previousSchool, &address, &relationship, &updatedAt, &status, &registrationNumber, &applicationID, &custom); err != nil {
		return nil, err
	}
	customFields := map[string]any{}
	if len(custom) > 0 {
		if err := json.Unmarshal(custom, &customFields); err != nil {
			return nil, err
		}
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
		"custom_fields":         customFields,
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
		        COALESCE(app.status,''),COALESCE(app.registration_number,''),COALESCE(app.id::text,''),ap.custom_fields
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
		common.WriteError(w, http.StatusNotFound, "calon siswa tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "data calon siswa gagal dimuat")
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
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	custom, err := normalizeApplicantCustom(r.Context(), a.DB, c.SchoolID, in.CustomFields)
	if err != nil {
		writeErr(w, err)
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "data calon siswa gagal disimpan")
		return
	}
	defer func() { _ = tx.Rollback() }()
	// Idempotent per guardian: a retry (e.g. after the application step failed)
	// with the same NISN or NIK reuses the existing applicant instead of
	// creating a duplicate. The advisory lock serialises concurrent submits.
	if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtext('applicant:'||$1))`, c.GuardianID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "data calon siswa gagal disimpan")
		return
	}
	var id, applicationID, appStatus string
	err = tx.QueryRowContext(r.Context(),
		`SELECT ap.id,COALESCE(a.id::text,''),COALESCE(a.status,'')
		 FROM applicants ap
		 LEFT JOIN LATERAL (SELECT id,status FROM applications WHERE applicant_id=ap.id AND school_id=ap.school_id ORDER BY created_at DESC LIMIT 1) a ON true
		 WHERE ap.school_id=$1 AND ap.guardian_id=$2 AND ap.deleted_at IS NULL
		   AND (($3<>'' AND ap.nisn=$3) OR ($4<>'' AND ap.nik=$4))
		 ORDER BY ap.created_at LIMIT 1`,
		c.SchoolID, c.GuardianID, in.NISN, in.NIK).Scan(&id, &applicationID, &appStatus)
	status := http.StatusCreated
	switch {
	case err == sql.ErrNoRows:
		err = tx.QueryRowContext(r.Context(),
			`INSERT INTO applicants(school_id,guardian_id,full_name,nickname,nisn,nik,birth_date,birth_place,gender,previous_school,address,guardian_relationship,source,custom_fields)
			 VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,'')::date,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),'guardian',$13)
			 RETURNING id`,
			c.SchoolID, c.GuardianID, in.FullName, in.Nickname, in.NISN, in.NIK, in.BirthDate, in.BirthPlace,
			in.Gender, in.PreviousSchool, in.Address, in.GuardianRelationship, jsonRaw(custom)).Scan(&id)
	case err == nil:
		status = http.StatusOK
		// Biodata behind a submitted application stays as submitted.
		if applicationID == "" || appStatus == "draft" {
			_, err = tx.ExecContext(r.Context(),
				`UPDATE applicants SET full_name=$1,nickname=NULLIF($2,''),nisn=NULLIF($3,''),nik=NULLIF($4,''),
				        birth_date=NULLIF($5,'')::date,birth_place=NULLIF($6,''),gender=NULLIF($7,''),
				        previous_school=NULLIF($8,''),address=NULLIF($9,''),guardian_relationship=NULLIF($10,''),
				        custom_fields=$12,updated_at=now()
				 WHERE id=$11`,
				in.FullName, in.Nickname, in.NISN, in.NIK, in.BirthDate, in.BirthPlace, in.Gender,
				in.PreviousSchool, in.Address, in.GuardianRelationship, id, jsonRaw(custom))
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "data calon siswa gagal disimpan")
		return
	}
	resp := map[string]any{"id": id}
	if applicationID != "" {
		resp["application_id"] = applicationID
	}
	common.WriteJSON(w, status, resp, nil)
}

// UpdateApplicant is PUT /guardian/applicants/{id}.
func (a *App) UpdateApplicant(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	var in applicantInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	id := param(r, "id")
	custom, err := normalizeApplicantCustom(r.Context(), a.DB, c.SchoolID, in.CustomFields)
	if err != nil {
		writeErr(w, err)
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE applicants SET full_name=$1,nickname=NULLIF($2,''),nisn=NULLIF($3,''),nik=NULLIF($4,''),
		        birth_date=NULLIF($5,'')::date,birth_place=NULLIF($6,''),gender=NULLIF($7,''),
		        previous_school=NULLIF($8,''),address=NULLIF($9,''),guardian_relationship=NULLIF($10,''),
		        custom_fields=$14,updated_at=now()
		 WHERE id=$11 AND guardian_id=$12 AND school_id=$13 AND deleted_at IS NULL`,
		in.FullName, in.Nickname, in.NISN, in.NIK, in.BirthDate, in.BirthPlace, in.Gender,
		in.PreviousSchool, in.Address, in.GuardianRelationship, id, c.GuardianID, c.SchoolID, jsonRaw(custom))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "data calon siswa gagal diperbarui")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "calon siswa tidak ditemukan")
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
		common.WriteError(w, http.StatusConflict, "calon siswa sudah memiliki pendaftaran aktif")
		return
	}
	if err != nil && err != sql.ErrNoRows {
		common.WriteError(w, http.StatusInternalServerError, "data calon siswa gagal dihapus")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE applicants SET deleted_at=now(),updated_at=now() WHERE id=$1 AND guardian_id=$2 AND school_id=$3 AND deleted_at IS NULL`,
		id, c.GuardianID, c.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "data calon siswa gagal dihapus")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "calon siswa tidak ditemukan")
		return
	}
	// Soft delete leaves no trace in the admin audit log otherwise.
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,before_data)
		 SELECT ap.school_id,'applicant',ap.id::text,'applicant_deleted','guardian',$2,COALESCE(g.name,''),
		        jsonb_build_object('full_name',ap.full_name,'nisn',ap.nisn,'source',ap.source)
		 FROM applicants ap LEFT JOIN guardians g ON g.id=ap.guardian_id WHERE ap.id=$1`,
		id, c.GuardianID); err != nil {
		log.Printf("audit applicant_deleted %s: %v", id, err)
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
		common.WriteError(w, http.StatusBadRequest, "calon siswa, periode, gelombang, dan jalur wajib dipilih")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "pendaftaran gagal dibuat")
		return
	}
	defer func() { _ = tx.Rollback() }()

	var owned bool
	if err = tx.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM applicants WHERE id=$1 AND guardian_id=$2 AND school_id=$3 AND deleted_at IS NULL)`,
		in.ApplicantID, c.GuardianID, c.SchoolID).Scan(&owned); err != nil || !owned {
		common.WriteError(w, http.StatusNotFound, "calon siswa tidak ditemukan")
		return
	}

	warnings, err := a.checkNewApplication(r.Context(), tx, c.SchoolID, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	registrationNumber, err := nextRegistrationNumber(r.Context(), tx, c.SchoolID, in.WaveID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var id string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO applications(school_id,applicant_id,period_id,wave_id,track_id,registration_number)
		 VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		c.SchoolID, in.ApplicantID, in.PeriodID, in.WaveID, in.TrackID, registrationNumber).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "calon siswa ini sudah terdaftar pada periode ini")
			return
		}
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO application_state_events(school_id,application_id,to_status,actor_type,actor_id,version)
		 VALUES($1,$2,'draft','guardian',$3,1)`, c.SchoolID, id, c.GuardianID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "pendaftaran gagal dibuat")
		return
	}
	if err = tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "pendaftaran gagal dibuat")
		return
	}
	a.audit(r.Context(), c.SchoolID, "application", id, "application_created", "guardian", c.GuardianID, "", "", nil,
		map[string]any{"status": "draft", "registration_number": registrationNumber})

	common.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":                  id,
		"status":              "draft",
		"version":             1,
		"registration_number": registrationNumber,
		"warnings":            nonNil(warnings),
	}, nil)
}

// checkNewApplication verifies that period, wave and track belong together
// and to the school, that registration is open, that the track still has a
// seat (BUG-031; enforced again under lock at submit) and that the applicant
// meets the track's eligibility rules (BUG-110).
func (a *App) checkNewApplication(ctx context.Context, tx *sql.Tx, schoolID string, in createApplicationInput) ([]string, error) {
	var (
		open, trackActive bool
		quota, active     int
		eligibility       []byte
		birth             sql.NullTime
	)
	err := tx.QueryRowContext(ctx,
		`SELECT p.status='open' AND now() BETWEEN w.registration_start AND w.registration_end,t.is_active,t.quota,t.eligibility_config,
		        (SELECT count(*) FROM applications x WHERE x.track_id=t.id AND x.status NOT IN('draft','withdrawn','rejected','dropped')),
		        (SELECT birth_date FROM applicants WHERE id=$5)
		 FROM admission_periods p
		 JOIN admission_waves w ON w.period_id=p.id AND w.id=$3 AND w.deleted_at IS NULL
		 JOIN admission_tracks t ON t.period_id=p.id AND t.id=$4 AND t.deleted_at IS NULL
		 WHERE p.id=$2 AND p.school_id=$1`, schoolID, in.PeriodID, in.WaveID, in.TrackID, in.ApplicantID).
		Scan(&open, &trackActive, &quota, &eligibility, &active, &birth)
	if err == sql.ErrNoRows {
		return nil, &httpError{http.StatusUnprocessableEntity, "periode, gelombang, atau jalur tidak valid"}
	}
	if err != nil {
		return nil, err
	}
	if !open {
		return nil, &httpError{http.StatusUnprocessableEntity, "masa pendaftaran gelombang ini belum dibuka atau sudah ditutup"}
	}
	if !trackActive {
		return nil, &httpError{http.StatusUnprocessableEntity, "jalur ini tidak menerima pendaftaran"}
	}
	if model.CheckQuota(quota, active) != nil {
		return nil, &httpError{http.StatusUnprocessableEntity, "Kuota jalur ini sudah penuh; silakan pilih jalur lain."}
	}
	rules, err := model.ParseEligibility(eligibility)
	if err != nil {
		return nil, err
	}
	if issues := rules.CheckEligibility(nullTimePtr(birth), in.WaveID, time.Now()); len(issues) > 0 {
		return nil, &httpError{http.StatusUnprocessableEntity, "Tidak memenuhi syarat jalur: " + strings.Join(issues, "; ") + "."}
	}
	var warnings []string
	if remaining := model.VacantSeats(quota, active); remaining <= 3 {
		warnings = append(warnings, fmt.Sprintf("Sisa kuota jalur ini %d kursi; kirim pendaftaran segera.", remaining))
	}
	return warnings, nil
}

// nextRegistrationNumber issues "PPDB-<tahun>-G<urutan gelombang>-<nomor>",
// sequential per wave (BUG-162). The counter row is locked by the upsert so
// concurrent registrations never share a number.
func nextRegistrationNumber(ctx context.Context, tx *sql.Tx, schoolID, waveID string) (string, error) {
	var (
		prefix string
		value  int
	)
	err := tx.QueryRowContext(ctx,
		`INSERT INTO registration_counters(wave_id,school_id,prefix,last_value)
		 SELECT w.id,w.school_id,
		        'PPDB-'||left(p.academic_year,4)||'-G'||lpad((SELECT count(*) FROM admission_waves w2
		            JOIN admission_periods p2 ON p2.id=w2.period_id
		            WHERE w2.school_id=w.school_id AND left(p2.academic_year,4)=left(p.academic_year,4)
		              AND (w2.created_at,w2.id)<=(w.created_at,w.id))::text,2,'0'),1
		 FROM admission_waves w JOIN admission_periods p ON p.id=w.period_id
		 WHERE w.id=$1 AND w.school_id=$2
		 ON CONFLICT (wave_id) DO UPDATE SET last_value=registration_counters.last_value+1
		 RETURNING prefix,last_value`, waveID, schoolID).Scan(&prefix, &value)
	if err != nil {
		return "", err
	}
	return model.RegistrationNumber(prefix, value), nil
}

// ApplicationDetail is GET /guardian/applications/{id}.
func (a *App) ApplicationDetail(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	result, err := a.applicationDetail(r, c.SchoolID, param(r, "id"), "guardian", c.GuardianID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "pendaftaran tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "pendaftaran gagal dimuat")
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
	                 COALESCE(ap.previous_school,''),COALESCE(ap.address,''),COALESCE(ap.guardian_relationship,''),ap.custom_fields
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
		custom                                                                 []byte
	)
	err := a.DB.QueryRowContext(r.Context(), query, args...).Scan(
		&id, &applicantID, &periodID, &waveID, &trackID, &registrationNumber, &status, &version,
		&submittedAt, &answers,
		&fullName, &nickname, &nisn, &nik, &birthDate, &birthPlace, &gender, &previousSchool, &address, &relationship, &custom)
	if err != nil {
		return nil, err
	}
	customFields := map[string]any{}
	if len(custom) > 0 {
		if err := json.Unmarshal(custom, &customFields); err != nil {
			return nil, err
		}
	}
	// form_answers pairs each configured custom field of the period with the
	// applicant's answer so staff and guardians see labelled values.
	fields, err := formFields(r.Context(), a.DB, schoolID, periodID, "", false)
	if err != nil {
		return nil, err
	}
	formAnswers := []map[string]any{}
	for _, f := range fields {
		if f.Builtin {
			continue
		}
		if v, ok := customFields[f.Key]; ok {
			formAnswers = append(formAnswers, map[string]any{"key": f.Key, "label": f.Label, "type": f.Type, "value": v})
		}
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
		"custom_fields":         customFields,
	}
	if scope != "guardian" {
		// Staff never see the full NIK (BUG-121).
		applicant["nik"] = model.MaskNIK(nik)
	}
	result := map[string]any{
		"id":                  id,
		"applicant_id":        applicantID,
		"applicant":           applicant,
		"form_answers":        formAnswers,
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
	refunds, err := applicationRefunds(r.Context(), a.DB, schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	result["refunds"] = refunds
	var dropReason string
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(drop_reason,'') FROM applications WHERE id=$1 AND school_id=$2`, applicationID, schoolID).Scan(&dropReason); err != nil {
		return nil, err
	}
	if dropReason != "" {
		result["drop_reason"] = dropReason
	}
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
