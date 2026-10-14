package handler

import (
	"context"
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
		AND ($6='' OR a.track_id::text=$6)
		AND ($7='' OR $7=CASE WHEN COALESCE(bill.billed,0)<=0 THEN 'unbilled'
		                      WHEN COALESCE(bill.outstanding,0)<=0 THEN 'paid'
		                      WHEN bill.outstanding>=bill.billed THEN 'unpaid' ELSE 'partial' END)`
	paymentStatus := q.Get("payment_status")

	var total int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT count(*) `+listApplicationsFrom+` WHERE `+where,
		schoolID, status, search, periodID, waveID, trackID, paymentStatus).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		listApplicationsSQL+` WHERE `+where+` ORDER BY a.created_at DESC LIMIT $8 OFFSET $9`,
		schoolID, status, search, periodID, waveID, trackID, paymentStatus, perPage, common.Offset(page, perPage))
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
	       COALESCE(bill.billed,0),COALESCE(bill.outstanding,0),
	       COALESCE(w.name,''),COALESCE(t.name,'')
	` + listApplicationsFrom

const listApplicationsFrom = `FROM applications a
	JOIN applicants ap ON ap.id=a.applicant_id
	LEFT JOIN admission_waves w ON w.id=a.wave_id
	LEFT JOIN admission_tracks t ON t.id=a.track_id
	LEFT JOIN LATERAL (
	    -- Per stage: the latest non-cancelled charge minus that stage's
	    -- verified payments. Reversed payments are no longer 'verified'.
	    SELECT sum(c.net_idr) AS billed,
	           sum(GREATEST(c.net_idr-COALESCE((SELECT sum(pa.amount_idr) FROM payment_allocations pa
	                 JOIN payment_submissions ps ON ps.id=pa.payment_id
	                 JOIN charges c2 ON c2.id=ps.charge_id
	                 WHERE ps.application_id=a.id AND ps.status='verified' AND c2.stage=c.stage),0),0)) AS outstanding
	    FROM (SELECT DISTINCT ON (stage) stage,net_idr,cancelled_at FROM charges
	          WHERE application_id=a.id ORDER BY stage,revision DESC) c
	    WHERE c.cancelled_at IS NULL
	) bill ON true`

func scanAdminApplication(row rowScanner) (map[string]any, error) {
	var (
		id, applicantID, periodID, waveID, trackID, registrationNumber, status string
		version                                                                int
		submittedAt                                                            sql.NullTime
		applicantName, nisn                                                    string
		documentsCount, documentsVerified                                      int
		billed, outstanding                                                    int64
		waveName, trackName                                                    string
	)
	if err := row.Scan(&id, &applicantID, &periodID, &waveID, &trackID, &registrationNumber, &status, &version,
		&submittedAt, &applicantName, &nisn, &documentsCount, &documentsVerified, &billed, &outstanding, &waveName, &trackName); err != nil {
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
		"billed":              billed,
		"payment_status":      model.PaymentStatusOf(billed, outstanding),
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
	guardian, err := a.applicationGuardian(r.Context(), schoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	result["guardian"] = guardian
	activity, err := a.applicationActivity(r.Context(), schoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	result["activity"] = activity
	discounts, err := a.specialDiscounts(r.Context(), schoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	result["special_discounts"] = discounts
	schedules, err := a.selectionSchedules(r.Context(), a.DB, schoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	result["schedules"] = schedules
	common.WriteJSON(w, http.StatusOK, result, nil)
}

// applicationGuardian returns the parent/guardian with verified contacts
// (BUG-103).
func (a *App) applicationGuardian(ctx context.Context, schoolID, applicationID string) (map[string]any, error) {
	var id, name string
	if err := a.DB.QueryRowContext(ctx,
		`SELECT g.id,g.name FROM applications a JOIN applicants ap ON ap.id=a.applicant_id JOIN guardians g ON g.id=ap.guardian_id
		 WHERE a.id=$1 AND a.school_id=$2`, applicationID, schoolID).Scan(&id, &name); err != nil {
		return nil, err
	}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT kind,normalized_value FROM guardian_contacts WHERE guardian_id=$1 AND school_id=$2 ORDER BY kind,created_at`, id, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{"id": id, "name": name, "email": "", "phone": ""}
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return nil, err
		}
		if out[kind] == "" {
			out[kind] = value
		}
	}
	return out, rows.Err()
}

// applicationActivity is the application's audit trail: every review,
// payment, decision and status change with actor and time (BUG-102).
func (a *App) applicationActivity(ctx context.Context, schoolID, applicationID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT action,actor_type,COALESCE(actor_name,''),COALESCE(reason,''),created_at,after_data FROM audit_events
		 WHERE school_id=$1 AND aggregate_type='application' AND aggregate_id=$2 ORDER BY created_at DESC,id DESC LIMIT 200`,
		schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			action, actorType, actorName, reason string
			at                                   time.Time
			after                                []byte
		)
		if err := rows.Scan(&action, &actorType, &actorName, &reason, &at, &after); err != nil {
			return nil, err
		}
		item := map[string]any{"action": action, "actor_type": actorType, "actor_name": model.ActorLabel(actorType, actorName), "created_at": at}
		if reason != "" {
			item["reason"] = reason
		}
		if len(after) > 0 {
			item["detail"] = json.RawMessage(after)
		}
		out = append(out, item)
	}
	return out, rows.Err()
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
	// Promo policy (BR-PPDB-02) and the form-payment grace bound.
	DiscountMode        string `json:"discount_mode"`
	MaxDiscountIDR      *int64 `json:"max_discount_idr"`
	MaxPaymentGraceDays *int   `json:"max_payment_grace_days"`
	// Refund policy for dropped applicants (BUG-040): share of the paid
	// amount returned, in basis points, minus a flat administration fee.
	RefundPolicyBP    *int   `json:"refund_policy_bp"`
	RefundAdminFeeIDR *int64 `json:"refund_admin_fee_idr"`
}

func (in periodInput) validatePolicy() error {
	if in.DiscountMode != "" && in.DiscountMode != "stack" && in.DiscountMode != "max" {
		return &httpError{http.StatusUnprocessableEntity, "mode promo harus stack (bertumpuk) atau max (ambil terbesar)"}
	}
	if in.MaxDiscountIDR != nil && *in.MaxDiscountIDR < 0 {
		return &httpError{http.StatusUnprocessableEntity, "batas potongan tidak boleh negatif"}
	}
	if in.MaxPaymentGraceDays != nil && *in.MaxPaymentGraceDays < 0 {
		return &httpError{http.StatusUnprocessableEntity, "batas hari bayar formulir tidak boleh negatif"}
	}
	if in.RefundPolicyBP != nil && (*in.RefundPolicyBP < 0 || *in.RefundPolicyBP > 10000) {
		return &httpError{http.StatusUnprocessableEntity, "persentase pengembalian dana harus 0-100%"}
	}
	if in.RefundAdminFeeIDR != nil && *in.RefundAdminFeeIDR < 0 {
		return &httpError{http.StatusUnprocessableEntity, "biaya administrasi pengembalian tidak boleh negatif"}
	}
	return nil
}

func intOrNil[T int | int64](v *T) any {
	if v == nil {
		return nil
	}
	return *v
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
		        COALESCE(registration_close_at,(SELECT max(registration_end) FROM admission_waves w WHERE w.period_id=admission_periods.id AND w.deleted_at IS NULL)),
		        discount_mode,max_discount_idr,max_payment_grace_days,refund_policy_bp,refund_admin_fee_idr
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
			discountMode                     string
			maxDiscount                      int64
			graceDays, refundBP              int
			refundFee                        int64
		)
		if err := rows.Scan(&id, &name, &year, &status, &timezone, &open, &close, &discountMode, &maxDiscount, &graceDays,
			&refundBP, &refundFee); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{"id": id, "name": name, "academic_year": year, "status": status, "timezone": timezone,
			"discount_mode": discountMode, "max_discount_idr": maxDiscount, "max_payment_grace_days": graceDays,
			"refund_policy_bp": refundBP, "refund_admin_fee_idr": refundFee}
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
		common.WriteError(w, http.StatusUnprocessableEntity, "nama periode dan tahun ajaran wajib diisi")
		return
	}
	if in.Timezone == "" {
		in.Timezone = "Asia/Jakarta"
	}
	if err := in.validatePolicy(); err != nil {
		writeErr(w, err)
		return
	}
	open, err := parseTime(in.RegistrationOpenAt)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "tanggal buka pendaftaran tidak valid")
		return
	}
	closeAt, err := parseTime(in.RegistrationCloseAt)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "tanggal tutup pendaftaran tidak valid")
		return
	}
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO admission_periods(school_id,name,academic_year,timezone,status,registration_open_at,registration_close_at,
		                               discount_mode,max_discount_idr,max_payment_grace_days,refund_policy_bp,refund_admin_fee_idr)
		 VALUES($1,$2,$3,$4,$5,$6,$7,COALESCE(NULLIF($8,''),'stack'),COALESCE($9::bigint,0),COALESCE($10::int,30),
		        COALESCE($11::int,10000),COALESCE($12::bigint,0)) RETURNING id`,
		schoolID, in.Name, in.AcademicYear, in.Timezone, normalizePeriodStatus(in.Status), nullTime(open), nullTime(closeAt),
		in.DiscountMode, intOrNil(in.MaxDiscountIDR), intOrNil(in.MaxPaymentGraceDays),
		intOrNil(in.RefundPolicyBP), intOrNil(in.RefundAdminFeeIDR)).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "periode dengan nama dan tahun ajaran ini sudah ada")
			return
		}
		writeErr(w, err)
		return
	}
	if c := common.Admin(r); c != nil {
		a.audit(r.Context(), schoolID, "period", id, "period_created", "admin", c.UserID, c.Name, "", nil, in)
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
		common.WriteError(w, http.StatusUnprocessableEntity, "tanggal buka pendaftaran tidak valid")
		return
	}
	closeAt, err := parseTime(in.RegistrationCloseAt)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "tanggal tutup pendaftaran tidak valid")
		return
	}
	if err := in.validatePolicy(); err != nil {
		writeErr(w, err)
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE admission_periods SET name=COALESCE(NULLIF($1,''),name),academic_year=COALESCE(NULLIF($2,''),academic_year),
		        status=$3,timezone=COALESCE(NULLIF($4,''),timezone),
		        registration_open_at=COALESCE($5,registration_open_at),registration_close_at=COALESCE($6,registration_close_at),
		        discount_mode=COALESCE(NULLIF($9,''),discount_mode),max_discount_idr=COALESCE($10::bigint,max_discount_idr),
		        max_payment_grace_days=COALESCE($11::int,max_payment_grace_days),
		        refund_policy_bp=COALESCE($12::int,refund_policy_bp),refund_admin_fee_idr=COALESCE($13::bigint,refund_admin_fee_idr),
		        updated_at=now()
		 WHERE id=$7 AND school_id=$8`,
		in.Name, in.AcademicYear, normalizePeriodStatus(in.Status), in.Timezone, nullTime(open), nullTime(closeAt), param(r, "id"), schoolID,
		in.DiscountMode, intOrNil(in.MaxDiscountIDR), intOrNil(in.MaxPaymentGraceDays),
		intOrNil(in.RefundPolicyBP), intOrNil(in.RefundAdminFeeIDR))
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "periode tidak ditemukan")
		return
	}
	if c := common.Admin(r); c != nil {
		a.audit(r.Context(), schoolID, "period", param(r, "id"), "period_updated", "admin", c.UserID, c.Name, "", nil, in)
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
	TrackID       string   `json:"track_id"`
	Stage         string   `json:"stage"`
}

// normalize validates the requirement: only PDF/JPG/PNG may be allowed
// (BUG-100), the stage is registration or reregistration, the size 1..20 MB.
func (in *requirementInput) normalize() error {
	if len(in.AcceptedTypes) == 0 {
		in.AcceptedTypes = defaultMIMEs(nil)
	}
	if err := model.ValidateAllowedMIMEs(in.AcceptedTypes); err != nil {
		return &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	if in.Stage == "" {
		in.Stage = stageRegistration
	}
	if in.Stage != stageRegistration && in.Stage != stageReregistration {
		return &httpError{http.StatusUnprocessableEntity, "tahap dokumen harus registration atau reregistration"}
	}
	if in.MaxSizeMB == 0 {
		in.MaxSizeMB = 5
	}
	if in.MaxSizeMB < 1 || in.MaxSizeMB > 20 {
		return &httpError{http.StatusUnprocessableEntity, "ukuran maksimal berkas harus 1 sampai 20 MB"}
	}
	return nil
}

// ListRequirements is GET /admin/config/document-requirements.
func (a *App) ListRequirements(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,name,COALESCE(description,''),allowed_mimes,max_bytes,required,COALESCE(track_id::text,''),stage
		 FROM document_requirements WHERE school_id=$1 AND deleted_at IS NULL ORDER BY name`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name, description, trackID, stage string
			allowed                               textArray
			maxBytes                              int64
			required                              bool
		)
		if err := rows.Scan(&id, &name, &description, &allowed, &maxBytes, &required, &trackID, &stage); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{
			"id":             id,
			"name":           name,
			"description":    description,
			"accepted_types": allowed,
			"max_size_mb":    int(maxBytes / (1 << 20)),
			"required":       required,
			"stage":          stage,
		}
		if trackID != "" {
			item["track_id"] = trackID
		}
		out = append(out, item)
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
	if strings.TrimSpace(in.Name) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "nama dokumen wajib diisi")
		return
	}
	if err := in.normalize(); err != nil {
		writeErr(w, err)
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	allowed := in.AcceptedTypes
	maxBytes := int64(in.MaxSizeMB) * (1 << 20)
	required := in.Required == nil || *in.Required
	code := strings.ToUpper(truncateSlug(in.Name))
	if code == "" {
		code = "DOC"
	}
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO document_requirements(school_id,period_id,code,name,description,required,allowed_mimes,max_bytes,track_id,stage)
		 SELECT $1,p.id,$2,$3,NULLIF($4,''),$5,$6,$7,
		        (SELECT t.id FROM admission_tracks t WHERE t.id::text=NULLIF($9,'') AND t.period_id=p.id),$10
		 FROM admission_periods p WHERE p.id=$8 AND p.school_id=$1 RETURNING id`,
		schoolID, code, in.Name, in.Description, required, allowed, maxBytes, periodID, in.TrackID, in.Stage).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "dokumen dengan nama ini sudah ada pada periode tersebut")
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
	if err := in.normalize(); err != nil {
		writeErr(w, err)
		return
	}
	maxBytes := int64(in.MaxSizeMB) * (1 << 20)
	required := in.Required == nil || *in.Required
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE document_requirements dr SET name=COALESCE(NULLIF($1,''),name),description=NULLIF($2,''),
		        allowed_mimes=$3,max_bytes=$4,required=$5,stage=$8,
		        track_id=(SELECT t.id FROM admission_tracks t WHERE t.id::text=NULLIF($9,'') AND t.period_id=dr.period_id)
		 WHERE id=$6 AND school_id=$7 AND deleted_at IS NULL`,
		in.Name, in.Description, in.AcceptedTypes, maxBytes, required, param(r, "id"), schoolID, in.Stage, in.TrackID)
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
	applicationID := q.Get("application_id")
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
		   AND ($9='' OR (aggregate_type='application' AND aggregate_id=$9))
		 ORDER BY created_at DESC LIMIT $7 OFFSET $8`,
		schoolID, entityType, actor, from, to, search, perPage, common.Offset(page, perPage), applicationID)
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
