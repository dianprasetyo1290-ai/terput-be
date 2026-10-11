package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// ---------------------------------------------------------------------------
// Admin dashboard
// ---------------------------------------------------------------------------

// AdminDashboard is GET /admin/dashboard.
func (a *App) AdminDashboard(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)

	summary := map[string]any{
		"totals":              map[string]int{},
		"recent_applications": []map[string]any{},
		"tasks":               []map[string]any{},
	}

	var (
		periodID, periodName, periodStatus, periodYear string
		open, close                                    sql.NullTime
	)
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id,name,status,academic_year,
		        COALESCE(registration_open_at,(SELECT min(registration_start) FROM admission_waves w WHERE w.period_id=admission_periods.id AND w.deleted_at IS NULL)),
		        COALESCE(registration_close_at,(SELECT max(registration_end) FROM admission_waves w WHERE w.period_id=admission_periods.id AND w.deleted_at IS NULL))
		 FROM admission_periods WHERE school_id=$1 AND deleted_at IS NULL AND status='open' ORDER BY created_at DESC LIMIT 1`,
		schoolID).Scan(&periodID, &periodName, &periodStatus, &periodYear, &open, &close)
	switch {
	case err == nil:
		period := map[string]any{
			"id":            periodID,
			"name":          periodName,
			"status":        periodStatus,
			"academic_year": periodYear,
		}
		if open.Valid {
			period["registration_open_at"] = open.Time
		}
		if close.Valid {
			period["registration_close_at"] = close.Time
		}
		summary["active_period"] = period
	case err != sql.ErrNoRows:
		writeErr(w, err)
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT status,count(*) FROM applications WHERE school_id=$1 GROUP BY status`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	totals := map[string]int{}
	total := 0
	for rows.Next() {
		var (
			status string
			count  int
		)
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		totals[status] = count
		total += count
	}
	rows.Close()
	totals["total"] = total
	summary["totals"] = totals

	recent, err := a.recentApplications(r, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	summary["recent_applications"] = recent

	tasks, err := a.dashboardTasks(r, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	summary["tasks"] = tasks

	common.WriteJSON(w, http.StatusOK, summary, nil)
}

func (a *App) recentApplications(r *http.Request, schoolID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT a.id,a.registration_number,a.status,ap.full_name
		 FROM applications a JOIN applicants ap ON ap.id=a.applicant_id
		 WHERE a.school_id=$1 ORDER BY a.created_at DESC LIMIT 5`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, registrationNumber, status, applicantName string
		if err := rows.Scan(&id, &registrationNumber, &status, &applicantName); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":                  id,
			"registration_number": registrationNumber,
			"status":              status,
			"applicant":           map[string]any{"full_name": applicantName},
		})
	}
	return out, rows.Err()
}

func (a *App) dashboardTasks(r *http.Request, schoolID string) ([]map[string]any, error) {
	specs := []struct {
		key, title, description, route, query string
	}{
		{"document", "Verifikasi berkas", "Dokumen menunggu review", "/ppdb/admin/berkas",
			`SELECT count(*) FROM application_documents d WHERE d.school_id=$1 AND NOT EXISTS (SELECT 1 FROM document_reviews rv WHERE rv.document_id=d.id)`},
		{"payment", "Review pembayaran", "Bukti transfer menunggu verifikasi", "/ppdb/admin/pembayaran",
			`SELECT count(*) FROM payment_submissions WHERE school_id=$1 AND status IN('submitted','pending_review')`},
		{"decision", "Keputusan seleksi", "Peserta menunggu keputusan", "/ppdb/admin/seleksi",
			`SELECT count(*) FROM applications WHERE school_id=$1 AND status IN('tested','ranked')`},
		{"conversion", "Konversi siswa", "Konversi gagal atau menunggu", "/ppdb/admin/konversi",
			`SELECT count(*) FROM student_conversions WHERE school_id=$1 AND status IN('pending','failed')`},
	}
	out := []map[string]any{}
	for _, spec := range specs {
		var count int
		if err := a.DB.QueryRowContext(r.Context(), spec.query, schoolID).Scan(&count); err != nil {
			return nil, err
		}
		if count == 0 {
			continue
		}
		out = append(out, map[string]any{
			"id":          spec.key,
			"type":        spec.key,
			"title":       spec.title,
			"description": spec.description,
			"route":       spec.route,
			"count":       count,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Admin applications
// ---------------------------------------------------------------------------

// ListApplications is GET /admin/applications.
func (a *App) ListApplications(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	q := r.URL.Query()
	status := q.Get("status")
	search := strings.TrimSpace(q.Get("search"))
	periodID := q.Get("period_id")
	waveID := q.Get("wave_id")
	trackID := q.Get("track_id")
	page, perPage := common.Page(r)

	where := `a.school_id=$1
		AND ($2='' OR a.status=$2)
		AND ($3='' OR ap.full_name ILIKE '%'||$3||'%' OR COALESCE(ap.nisn,'') ILIKE '%'||$3||'%' OR a.registration_number ILIKE '%'||$3||'%')
		AND ($4='' OR a.period_id::text=$4)
		AND ($5='' OR a.wave_id::text=$5)
		AND ($6='' OR a.track_id::text=$6)`

	var total int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT count(*) FROM applications a JOIN applicants ap ON ap.id=a.applicant_id WHERE `+where,
		schoolID, status, search, periodID, waveID, trackID).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		listApplicationsSQL+` WHERE `+where+` ORDER BY a.created_at DESC LIMIT $7 OFFSET $8`,
		schoolID, status, search, periodID, waveID, trackID, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		item, err := scanAdminApplication(rows)
		if err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, item)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total}, nil)
}

const listApplicationsSQL = `SELECT a.id,a.applicant_id,a.period_id,a.wave_id,a.track_id,a.registration_number,a.status,a.version,a.submitted_at,
	       ap.full_name,COALESCE(ap.nisn,''),
	       (SELECT count(*) FROM application_documents d WHERE d.application_id=a.id),
	       (SELECT count(*) FROM application_documents d WHERE d.application_id=a.id AND EXISTS
	           (SELECT 1 FROM document_reviews rv WHERE rv.document_id=d.id AND rv.status='accepted')),
	       COALESCE((SELECT net_idr FROM charges c WHERE c.application_id=a.id ORDER BY revision DESC LIMIT 1),0)
	       - COALESCE((SELECT sum(pa.amount_idr) FROM payment_allocations pa
	                    JOIN payment_submissions ps ON ps.id=pa.payment_id
	                    WHERE ps.application_id=a.id AND ps.status='verified'),0)
	       + COALESCE((SELECT sum(pr.amount_idr) FROM payment_reversals pr
	                    JOIN payment_submissions ps ON ps.id=pr.payment_id
	                    WHERE ps.application_id=a.id),0),
	       COALESCE(w.name,''),COALESCE(t.name,'')
	FROM applications a
	JOIN applicants ap ON ap.id=a.applicant_id
	LEFT JOIN admission_waves w ON w.id=a.wave_id
	LEFT JOIN admission_tracks t ON t.id=a.track_id`

func scanAdminApplication(row rowScanner) (map[string]any, error) {
	var (
		id, applicantID, periodID, waveID, trackID, registrationNumber, status string
		version                                                                int
		submittedAt                                                            sql.NullTime
		applicantName, nisn                                                    string
		documentsCount, documentsVerified                                      int
		outstanding                                                            int64
		waveName, trackName                                                    string
	)
	if err := row.Scan(&id, &applicantID, &periodID, &waveID, &trackID, &registrationNumber, &status, &version,
		&submittedAt, &applicantName, &nisn, &documentsCount, &documentsVerified, &outstanding, &waveName, &trackName); err != nil {
		return nil, err
	}
	item := map[string]any{
		"id":                  id,
		"applicant_id":        applicantID,
		"period_id":           periodID,
		"wave_id":             waveID,
		"track_id":            trackID,
		"registration_number": registrationNumber,
		"status":              status,
		"version":             version,
		"documents_count":     documentsCount,
		"documents_verified":  documentsVerified,
		"outstanding":         outstanding,
		"applicant": map[string]any{
			"id":        applicantID,
			"full_name": applicantName,
			"nisn":      nisn,
		},
	}
	if submittedAt.Valid {
		item["submitted_at"] = submittedAt.Time
	}
	if waveName != "" {
		item["wave"] = map[string]any{"id": waveID, "name": waveName}
	}
	if trackName != "" {
		item["track"] = map[string]any{"id": trackID, "name": trackName}
	}
	return item, nil
}

// AdminApplicationDetail is GET /admin/applications/{id}.
func (a *App) AdminApplicationDetail(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	a.writeAdminApplication(w, r, common.EffectiveSchoolID(r), param(r, "id"))
}

// writeAdminApplication renders {application, documents, charge, payments}.
func (a *App) writeAdminApplication(w http.ResponseWriter, r *http.Request, schoolID, applicationID string) {
	row := a.DB.QueryRowContext(r.Context(), listApplicationsSQL+` WHERE a.id=$1 AND a.school_id=$2`, applicationID, schoolID)
	application, err := scanAdminApplication(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	detail, err := a.applicationDetail(r, schoolID, applicationID, "admin", "")
	if err != nil {
		writeErr(w, err)
		return
	}
	if applicant, ok := detail["applicant"]; ok {
		application["applicant"] = applicant
	}
	documents, err := a.applicationDocuments(r, schoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	result := map[string]any{
		"application": application,
		"documents":   documents,
	}
	var chargeID string
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT id FROM charges WHERE application_id=$1 AND school_id=$2 ORDER BY revision DESC LIMIT 1`, applicationID, schoolID).
		Scan(&chargeID); err == nil {
		if view, err := a.chargeView(r.Context(), schoolID, applicationID, chargeID); err == nil {
			result["charge"] = view
		}
	} else if err != sql.ErrNoRows {
		writeErr(w, err)
		return
	}
	payments, err := a.paymentsList(r, schoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	result["payments"] = payments
	common.WriteJSON(w, http.StatusOK, result, nil)
}

func (a *App) paymentsList(r *http.Request, schoolID, applicationID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,amount_idr,COALESCE(transfer_at,created_at),COALESCE(bank_name,''),COALESCE(account_name,''),
		        COALESCE(reference,''),COALESCE(file_name,''),status,COALESCE(review_note,''),created_at
		 FROM payment_submissions WHERE application_id=$1 AND school_id=$2 ORDER BY created_at DESC`,
		applicationID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, bank, account, reference, fileName, status, reviewNote string
			amount                                                     int64
			transferAt, createdAt                                      time.Time
		)
		if err := rows.Scan(&id, &amount, &transferAt, &bank, &account, &reference, &fileName, &status, &reviewNote, &createdAt); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id":             id,
			"application_id": applicationID,
			"amount":         amount,
			"transfer_at":    transferAt,
			"bank_name":      bank,
			"account_name":   account,
			"reference":      reference,
			"file_name":      fileName,
			"status":         status,
			"created_at":     createdAt,
		}
		if reviewNote != "" {
			item["review_note"] = reviewNote
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Config: periods / waves / tracks / requirements
// ---------------------------------------------------------------------------

type periodInput struct {
	Name                string `json:"name"`
	AcademicYear        string `json:"academic_year"`
	Status              string `json:"status"`
	Timezone            string `json:"timezone"`
	RegistrationOpenAt  string `json:"registration_open_at"`
	RegistrationCloseAt string `json:"registration_close_at"`
}

// ListPeriods is GET /admin/config/periods.
func (a *App) ListPeriods(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,name,academic_year,status,timezone,
		        COALESCE(registration_open_at,(SELECT min(registration_start) FROM admission_waves w WHERE w.period_id=admission_periods.id AND w.deleted_at IS NULL)),
		        COALESCE(registration_close_at,(SELECT max(registration_end) FROM admission_waves w WHERE w.period_id=admission_periods.id AND w.deleted_at IS NULL))
		 FROM admission_periods WHERE school_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name, year, status, timezone string
			open, close                      sql.NullTime
		)
		if err := rows.Scan(&id, &name, &year, &status, &timezone, &open, &close); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{"id": id, "name": name, "academic_year": year, "status": status, "timezone": timezone}
		if open.Valid {
			item["registration_open_at"] = open.Time
		}
		if close.Valid {
			item["registration_close_at"] = close.Time
		}
		out = append(out, item)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// CreatePeriod is POST /admin/config/periods.
func (a *App) CreatePeriod(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in periodInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Name == "" || in.AcademicYear == "" {
		common.WriteError(w, http.StatusBadRequest, "name and academic_year are required")
		return
	}
	if in.Timezone == "" {
		in.Timezone = "Asia/Jakarta"
	}
	open, err := parseTime(in.RegistrationOpenAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "registration_open_at is invalid")
		return
	}
	closeAt, err := parseTime(in.RegistrationCloseAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "registration_close_at is invalid")
		return
	}
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO admission_periods(school_id,name,academic_year,timezone,status,registration_open_at,registration_close_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		schoolID, in.Name, in.AcademicYear, in.Timezone, normalizePeriodStatus(in.Status), nullTime(open), nullTime(closeAt)).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "a period with this name and academic year already exists")
			return
		}
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdatePeriod is PUT /admin/config/periods/{id}.
func (a *App) UpdatePeriod(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in periodInput
	if !common.Decode(w, r, &in) {
		return
	}
	open, err := parseTime(in.RegistrationOpenAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "registration_open_at is invalid")
		return
	}
	closeAt, err := parseTime(in.RegistrationCloseAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "registration_close_at is invalid")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE admission_periods SET name=COALESCE(NULLIF($1,''),name),academic_year=COALESCE(NULLIF($2,''),academic_year),
		        status=$3,timezone=COALESCE(NULLIF($4,''),timezone),
		        registration_open_at=COALESCE($5,registration_open_at),registration_close_at=COALESCE($6,registration_close_at),updated_at=now()
		 WHERE id=$7 AND school_id=$8`,
		in.Name, in.AcademicYear, normalizePeriodStatus(in.Status), in.Timezone, nullTime(open), nullTime(closeAt), param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "period not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}

type waveInput struct {
	PeriodID            string `json:"period_id"`
	Name                string `json:"name"`
	RegistrationOpenAt  string `json:"registration_open_at"`
	RegistrationCloseAt string `json:"registration_close_at"`
	TestAt              string `json:"test_at"`
	AnnouncementAt      string `json:"announcement_at"`
}

// ListWaves is GET /admin/config/waves.
func (a *App) ListWaves(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	periodID := r.URL.Query().Get("period_id")
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,period_id,name,registration_start,registration_end,test_start,announcement_at
		 FROM admission_waves WHERE school_id=$1 AND deleted_at IS NULL AND ($2='' OR period_id::text=$2)
		 ORDER BY registration_start`, schoolID, periodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, period, name   string
			start, end         time.Time
			testAt, announceAt sql.NullTime
		)
		if err := rows.Scan(&id, &period, &name, &start, &end, &testAt, &announceAt); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{
			"id":                    id,
			"period_id":             period,
			"name":                  name,
			"registration_open_at":  start,
			"registration_close_at": end,
			"status":                waveStatus(time.Now(), start, end),
		}
		if testAt.Valid {
			item["test_at"] = testAt.Time
		}
		if announceAt.Valid {
			item["announcement_at"] = announceAt.Time
		}
		out = append(out, item)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// CreateWave is POST /admin/config/waves.
func (a *App) CreateWave(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in waveInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	start, err := parseTime(in.RegistrationOpenAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "registration_open_at is invalid")
		return
	}
	end, err := parseTime(in.RegistrationCloseAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "registration_close_at is invalid")
		return
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		common.WriteError(w, http.StatusUnprocessableEntity, "registration window is invalid")
		return
	}
	testAt, _ := parseTime(in.TestAt)
	announceAt, _ := parseTime(in.AnnouncementAt)
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO admission_waves(school_id,period_id,name,registration_start,registration_end,test_start,announcement_at)
		 SELECT $1,p.id,$2,$3,$4,$5,$6 FROM admission_periods p WHERE p.id=$7 AND p.school_id=$1 RETURNING id`,
		schoolID, in.Name, start, end, nullTime(testAt), nullTime(announceAt), periodID).Scan(&id)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateWave is PUT /admin/config/waves/{id}.
func (a *App) UpdateWave(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in waveInput
	if !common.Decode(w, r, &in) {
		return
	}
	start, err := parseTime(in.RegistrationOpenAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "registration_open_at is invalid")
		return
	}
	end, err := parseTime(in.RegistrationCloseAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "registration_close_at is invalid")
		return
	}
	testAt, _ := parseTime(in.TestAt)
	announceAt, _ := parseTime(in.AnnouncementAt)
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE admission_waves SET name=COALESCE(NULLIF($1,''),name),
		        registration_start=COALESCE($2,registration_start),registration_end=COALESCE($3,registration_end),
		        test_start=COALESCE($4,test_start),announcement_at=COALESCE($5,announcement_at),updated_at=now()
		 WHERE id=$6 AND school_id=$7`,
		in.Name, nullTime(start), nullTime(end), nullTime(testAt), nullTime(announceAt), param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "wave not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}

type trackInput struct {
	PeriodID    string `json:"period_id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Quota       int    `json:"quota"`
	IsActive    *bool  `json:"is_active"`
}

// ListTracks is GET /admin/config/tracks.
func (a *App) ListTracks(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	periodID := r.URL.Query().Get("period_id")
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT t.id,t.period_id,t.name,COALESCE(t.description,''),t.quota,t.is_active,
		        (SELECT count(*) FROM applications a WHERE a.track_id=t.id)
		 FROM admission_tracks t WHERE t.school_id=$1 AND t.deleted_at IS NULL AND ($2='' OR t.period_id::text=$2)
		 ORDER BY t.name`, schoolID, periodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, period, name, description string
			quota, applicants             int
			isActive                      bool
		)
		if err := rows.Scan(&id, &period, &name, &description, &quota, &isActive, &applicants); err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, map[string]any{
			"id":              id,
			"period_id":       period,
			"name":            name,
			"description":     description,
			"quota":           quota,
			"applicant_count": applicants,
			"is_active":       isActive,
			"requirements":    []string{},
		})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// CreateTrack is POST /admin/config/tracks.
func (a *App) CreateTrack(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in trackInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	code := in.Code
	if code == "" {
		code = strings.ToUpper(truncateSlug(in.Name))
	}
	if code == "" {
		code = "TRACK"
	}
	isActive := in.IsActive == nil || *in.IsActive
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO admission_tracks(school_id,period_id,code,name,description,quota,is_active)
		 SELECT $1,p.id,$2,$3,NULLIF($4,''),$5,$6 FROM admission_periods p WHERE p.id=$7 AND p.school_id=$1 RETURNING id`,
		schoolID, code, in.Name, in.Description, in.Quota, isActive, periodID).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "a track with this code already exists for the period")
			return
		}
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateTrack is PUT /admin/config/tracks/{id}.
func (a *App) UpdateTrack(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in trackInput
	if !common.Decode(w, r, &in) {
		return
	}
	isActive := in.IsActive == nil || *in.IsActive
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE admission_tracks SET name=COALESCE(NULLIF($1,''),name),description=NULLIF($2,''),
		        quota=$3,is_active=$4,updated_at=now()
		 WHERE id=$5 AND school_id=$6`,
		in.Name, in.Description, in.Quota, isActive, param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "track not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}

type requirementInput struct {
	PeriodID      string   `json:"period_id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	AcceptedTypes []string `json:"accepted_types"`
	MaxSizeMB     int      `json:"max_size_mb"`
	Required      *bool    `json:"required"`
}

// ListRequirements is GET /admin/config/document-requirements.
func (a *App) ListRequirements(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,name,COALESCE(description,''),allowed_mimes,max_bytes,required
		 FROM document_requirements WHERE school_id=$1 AND deleted_at IS NULL ORDER BY name`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name, description string
			allowed               textArray
			maxBytes              int64
			required              bool
		)
		if err := rows.Scan(&id, &name, &description, &allowed, &maxBytes, &required); err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, map[string]any{
			"id":             id,
			"name":           name,
			"description":    description,
			"accepted_types": allowed,
			"max_size_mb":    int(maxBytes / (1 << 20)),
			"required":       required,
		})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// CreateRequirement is POST /admin/config/document-requirements.
func (a *App) CreateRequirement(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in requirementInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	allowed := defaultMIMEs(in.AcceptedTypes)
	maxBytes := int64(maxInt(in.MaxSizeMB, 5)) * (1 << 20)
	required := in.Required == nil || *in.Required
	code := strings.ToUpper(truncateSlug(in.Name))
	if code == "" {
		code = "DOC"
	}
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO document_requirements(school_id,period_id,code,name,description,required,allowed_mimes,max_bytes)
		 SELECT $1,p.id,$2,$3,NULLIF($4,''),$5,$6,$7 FROM admission_periods p WHERE p.id=$8 AND p.school_id=$1 RETURNING id`,
		schoolID, code, in.Name, in.Description, required, allowed, maxBytes, periodID).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "a requirement with this name already exists for the period")
			return
		}
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateRequirement is PUT /admin/config/document-requirements/{id}.
func (a *App) UpdateRequirement(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in requirementInput
	if !common.Decode(w, r, &in) {
		return
	}
	allowed := defaultMIMEs(in.AcceptedTypes)
	maxBytes := int64(maxInt(in.MaxSizeMB, 5)) * (1 << 20)
	required := in.Required == nil || *in.Required
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE document_requirements SET name=COALESCE(NULLIF($1,''),name),description=NULLIF($2,''),
		        allowed_mimes=$3,max_bytes=$4,required=$5
		 WHERE id=$6 AND school_id=$7 AND deleted_at IS NULL`,
		in.Name, in.Description, allowed, maxBytes, required, param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "requirement not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}

// ---------------------------------------------------------------------------
// Audit
// ---------------------------------------------------------------------------

// ListAuditEvents is GET /admin/audit-events.
func (a *App) ListAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	q := r.URL.Query()
	entityType := q.Get("entity_type")
	actor := q.Get("actor")
	from := q.Get("from")
	to := q.Get("to")
	search := strings.TrimSpace(q.Get("search"))
	page, perPage := common.Page(r)

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,action,actor_name,actor_type,aggregate_type,aggregate_id,COALESCE(reason,''),COALESCE(correlation_id,''),created_at,before_data,after_data
		 FROM audit_events
		 WHERE school_id=$1
		   AND ($2='' OR aggregate_type=$2)
		   AND ($3='' OR actor_id=$3 OR actor_name ILIKE '%'||$3||'%')
		   AND ($4='' OR created_at >= $4::date)
		   AND ($5='' OR created_at < ($5::date + interval '1 day'))
		   AND ($6='' OR action ILIKE '%'||$6||'%' OR aggregate_id ILIKE '%'||$6||'%')
		 ORDER BY created_at DESC LIMIT $7 OFFSET $8`,
		schoolID, entityType, actor, from, to, search, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id                                                                    int64
			action, actorName, actorType, entityTypeOut, entityID, reason, corrID string
			createdAt                                                             time.Time
			before, after                                                         []byte
		)
		if err := rows.Scan(&id, &action, &actorName, &actorType, &entityTypeOut, &entityID, &reason, &corrID, &createdAt, &before, &after); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{
			"id":          id,
			"action":      action,
			"actor_name":  actorName,
			"actor_type":  actorType,
			"entity_type": entityTypeOut,
			"entity_id":   entityID,
			"created_at":  createdAt,
		}
		if reason != "" {
			item["reason"] = reason
		}
		if corrID != "" {
			item["correlation_id"] = corrID
		}
		if len(before) > 0 {
			item["before"] = json.RawMessage(before)
		}
		if len(after) > 0 {
			item["after"] = json.RawMessage(after)
		}
		items = append(items, item)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": items}, nil)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func normalizePeriodStatus(status string) string {
	switch strings.ToLower(status) {
	case "open", "closed", "archived", "draft":
		return strings.ToLower(status)
	default:
		return "draft"
	}
}

func truncateSlug(name string) string {
	slug := model.Slug(name)
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return slug
}

func defaultMIMEs(types []string) []string {
	if len(types) == 0 {
		return []string{"application/pdf", "image/jpeg", "image/png"}
	}
	return types
}
