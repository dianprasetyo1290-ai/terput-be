package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/rules"
)

// BUG-119: data-subject rights under UU No. 27/2022 (PDP). A user files a
// request for their own data (an orang_tua also for a linked child, an
// admin on behalf of anyone in the unit); the unit admin handles it within
// 3 x 24 hours:
//   - access:     an export of the personal data identity holds
//   - correction: the requested field values are validated and applied
//   - deletion:   the account is anonymised (PII cleared, sign-in disabled);
//                 records other services keep by user id for legal
//                 retention (payments, grades) stay, without the PII.
//
//	POST /privacy/requests                         {request_type, subject_user_id?, details, corrections?}
//	GET  /privacy/requests/mine
//	GET  /privacy/requests?status=                 (admin: own unit)
//	POST /privacy/requests/{id}/start
//	POST /privacy/requests/{id}/complete           {resolution, confirm?}
//	POST /privacy/requests/{id}/reject             {resolution}
//	GET  /privacy/requests/{id}/export             (access requests, after completion)

type dataSubjectRequest struct {
	ID            string            `json:"id"`
	SchoolID      *string           `json:"school_id"`
	SubjectUserID string            `json:"subject_user_id"`
	SubjectName   string            `json:"subject_name"`
	RequestType   string            `json:"request_type"`
	Details       string            `json:"details"`
	Corrections   map[string]string `json:"corrections"`
	Status        string            `json:"status"`
	RequestedBy   *string           `json:"requested_by"`
	RequesterName *string           `json:"requester_name"`
	RequestedAt   time.Time         `json:"requested_at"`
	DueAt         time.Time         `json:"due_at"`
	Overdue       bool              `json:"overdue"`
	HandledBy     *string           `json:"handled_by"`
	HandledAt     *time.Time        `json:"handled_at"`
	Resolution    *string           `json:"resolution"`
	HasExport     bool              `json:"has_export"`
}

const dsrCols = `d.id, d.school_id::text, d.subject_user_id, su.name, d.request_type, d.details, d.corrections, d.status,
	d.requested_by::text, rq.name, d.requested_at, d.due_at, d.handled_by::text, d.handled_at, d.resolution, d.export_payload IS NOT NULL`

const dsrFrom = ` FROM data_subject_requests d
	JOIN users su ON su.id=d.subject_user_id
	LEFT JOIN users rq ON rq.id=d.requested_by`

func scanDSR(row interface{ Scan(...any) error }) (dataSubjectRequest, error) {
	var d dataSubjectRequest
	var corr []byte
	err := row.Scan(&d.ID, &d.SchoolID, &d.SubjectUserID, &d.SubjectName, &d.RequestType, &d.Details, &corr, &d.Status,
		&d.RequestedBy, &d.RequesterName, &d.RequestedAt, &d.DueAt, &d.HandledBy, &d.HandledAt, &d.Resolution, &d.HasExport)
	if len(corr) > 0 {
		_ = json.Unmarshal(corr, &d.Corrections)
	}
	d.Overdue = (d.Status == "submitted" || d.Status == "in_review") && time.Now().After(d.DueAt)
	return d, err
}

// correctableFields are the profile fields a correction request may change.
var correctableFields = map[string]string{
	"name": "Nama", "phone": "Nomor HP", "address": "Alamat", "birth_date": "Tanggal lahir",
	"birth_place": "Tempat lahir", "religion": "Agama", "nik": "NIK", "nisn": "NISN",
}

// validateCorrections checks the requested values (no DB lookups).
func validateCorrections(c map[string]string) map[string]string {
	errs := map[string]string{}
	if len(c) == 0 {
		errs["corrections"] = "Isi minimal satu data yang ingin dikoreksi"
		return errs
	}
	for k, v := range c {
		v = strings.TrimSpace(v)
		c[k] = v
		if _, ok := correctableFields[k]; !ok {
			errs[k] = "Data ini tidak dapat dikoreksi lewat permintaan"
			continue
		}
		switch k {
		case "name":
			if v == "" {
				errs[k] = "Nama tidak boleh kosong"
			}
		case "nik":
			if v != "" && !rules.ValidNIK(v) {
				errs[k] = "NIK harus 16 digit angka"
			}
		case "nisn":
			if v != "" && !rules.ValidNISN(v) {
				errs[k] = "NISN harus 10 digit angka"
			}
		case "birth_date":
			if v != "" {
				if _, ok := rules.ParseDate(v); !ok {
					errs[k] = "Format tanggal YYYY-MM-DD"
				}
			}
		}
	}
	return errs
}

func (a *App) canFileFor(ctx context.Context, claimsUserID, claimsRole, claimsSchool, subjectID string) (bool, error) {
	if subjectID == claimsUserID {
		return true, nil
	}
	switch claimsRole {
	case common.RoleSuperadmin:
		return true, nil
	case common.RoleAdmin:
		var ok bool
		err := a.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_school_roles WHERE user_id::text=$1 AND school_id::text=$2)`,
			subjectID, claimsSchool).Scan(&ok)
		return ok, err
	case common.RoleOrangTua:
		var ok bool
		err := a.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM guardian_links WHERE guardian_user_id::text=$1
			AND student_user_id::text=$2 AND status='active')`, claimsUserID, subjectID).Scan(&ok)
		return ok, err
	}
	return false, nil
}

// CreateDataSubjectRequest: POST /privacy/requests.
func (a *App) CreateDataSubjectRequest(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	var in struct {
		RequestType   string            `json:"request_type"`
		SubjectUserID string            `json:"subject_user_id"`
		Details       string            `json:"details"`
		Corrections   map[string]string `json:"corrections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Isian tidak valid")
		return
	}
	errs := map[string]string{}
	switch in.RequestType {
	case "access", "correction", "deletion":
	default:
		errs["request_type"] = "Pilih jenis permintaan: akses, koreksi, atau penghapusan"
	}
	in.Details = strings.TrimSpace(in.Details)
	if len([]rune(in.Details)) < 10 {
		errs["details"] = "Jelaskan permintaan minimal 10 karakter"
	}
	if in.SubjectUserID == "" {
		in.SubjectUserID = claims.UserID
	} else if !rules.IsUUID(in.SubjectUserID) {
		errs["subject_user_id"] = "Subjek data tidak valid"
	}
	var corrections any
	if in.RequestType == "correction" {
		for k, v := range validateCorrections(in.Corrections) {
			errs[k] = v
		}
		raw, _ := json.Marshal(in.Corrections)
		corrections = raw
	}
	if len(errs) > 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "Periksa kembali isian permintaan", errs)
		return
	}
	ok, err := a.canFileFor(r.Context(), claims.UserID, claims.ActiveRole, claims.SchoolID, in.SubjectUserID)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa subjek data")
		return
	}
	if !ok {
		common.WriteError(w, http.StatusForbidden, "Anda hanya dapat mengajukan permintaan untuk data Anda sendiri atau anak yang terhubung")
		return
	}
	// The request belongs to the subject's unit (the caller's unit when the
	// subject has a role there).
	var schoolID sql.NullString
	_ = a.DB.QueryRowContext(r.Context(), `SELECT school_id::text FROM user_school_roles
		WHERE user_id::text=$1 AND school_id IS NOT NULL
		ORDER BY (school_id::text = $2) DESC, created_at LIMIT 1`, in.SubjectUserID, claims.SchoolID).Scan(&schoolID)
	var id string
	err = a.DB.QueryRowContext(r.Context(), `INSERT INTO data_subject_requests (school_id, subject_user_id, request_type, details, corrections, requested_by)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		nullIfEmpty(schoolID.String), in.SubjectUserID, in.RequestType, in.Details, corrections, claims.UserID).Scan(&id)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "Masih ada permintaan sejenis yang sedang diproses untuk data ini")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan permintaan")
		return
	}
	var sid *string
	if schoolID.Valid {
		sid = &schoolID.String
	}
	a.RecordSecurityAudit(sid, "data_subject_request_created", in.SubjectUserID, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"request_id": id, "type": in.RequestType}, "", r.Header.Get("X-Correlation-ID"))
	d, err := a.getDSR(r.Context(), id)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat permintaan")
		return
	}
	common.WriteJSONMsg(w, http.StatusCreated, d, nil,
		"Permintaan diterima. Admin unit menanggapi paling lambat "+d.DueAt.In(jakarta()).Format("02-01-2006 15:04 WIB"))
}

func (a *App) getDSR(ctx context.Context, id string) (dataSubjectRequest, error) {
	return scanDSR(a.DB.QueryRowContext(ctx, `SELECT `+dsrCols+dsrFrom+` WHERE d.id::text=$1`, id))
}

func (a *App) listDSR(w http.ResponseWriter, r *http.Request, where string, args ...any) {
	rows, err := a.DB.QueryContext(r.Context(), `SELECT `+dsrCols+dsrFrom+` WHERE `+where+` ORDER BY d.requested_at DESC LIMIT 300`, args...)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat permintaan")
		return
	}
	defer rows.Close()
	out := []dataSubjectRequest{}
	for rows.Next() {
		d, err := scanDSR(rows)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memuat permintaan")
			return
		}
		out = append(out, d)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// MyDataSubjectRequests: GET /privacy/requests/mine.
func (a *App) MyDataSubjectRequests(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	a.listDSR(w, r, `(d.requested_by::text=$1 OR d.subject_user_id::text=$1)`, claims.UserID)
}

// ListDataSubjectRequests: GET /privacy/requests (admin: own unit).
func (a *App) ListDataSubjectRequests(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "privacy_requests", "view") {
		return
	}
	scope, _ := resolveSchoolScope(r)
	a.listDSR(w, r, `($1='' OR d.school_id::text=$1) AND ($2='' OR d.status=$2)`, scope, r.URL.Query().Get("status"))
}

// loadDSRForHandler loads the request and checks the admin's unit scope.
func (a *App) loadDSRForHandler(w http.ResponseWriter, r *http.Request) (dataSubjectRequest, bool) {
	if !a.permit(w, r, "privacy_requests", "approve") {
		return dataSubjectRequest{}, false
	}
	claims := common.FromCtx(r)
	d, err := a.getDSR(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && claims != nil && claims.ActiveRole != common.RoleSuperadmin &&
		(d.SchoolID == nil || *d.SchoolID != claims.SchoolID)) {
		common.WriteError(w, http.StatusNotFound, "Permintaan tidak ditemukan")
		return d, false
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat permintaan")
		return d, false
	}
	return d, true
}

// StartDataSubjectRequest: POST /privacy/requests/{id}/start.
func (a *App) StartDataSubjectRequest(w http.ResponseWriter, r *http.Request) {
	d, ok := a.loadDSRForHandler(w, r)
	if !ok {
		return
	}
	if d.Status != "submitted" {
		common.WriteError(w, http.StatusConflict, "Permintaan sudah diproses")
		return
	}
	claims := common.FromCtx(r)
	if _, err := a.DB.ExecContext(r.Context(), `UPDATE data_subject_requests SET status='in_review', handled_by=$2, updated_at=now() WHERE id=$1`,
		d.ID, nullUUID(claims.UserID)); err != nil {
		common.WriteDBError(w, err, "Gagal memperbarui permintaan")
		return
	}
	d, _ = a.getDSR(r.Context(), d.ID)
	common.WriteJSONMsg(w, http.StatusOK, d, nil, "Permintaan sedang ditinjau")
}

// RejectDataSubjectRequest: POST /privacy/requests/{id}/reject {resolution}.
func (a *App) RejectDataSubjectRequest(w http.ResponseWriter, r *http.Request) {
	d, ok := a.loadDSRForHandler(w, r)
	if !ok {
		return
	}
	var body struct {
		Resolution string `json:"resolution"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Resolution = strings.TrimSpace(body.Resolution)
	if len([]rune(body.Resolution)) < 10 {
		common.WriteValidationError(w, http.StatusBadRequest, "Alasan penolakan minimal 10 karakter", map[string]string{"resolution": "Minimal 10 karakter"})
		return
	}
	a.finishDSR(w, r, d, "rejected", body.Resolution, nil)
}

// CompleteDataSubjectRequest: POST /privacy/requests/{id}/complete {resolution, confirm}.
func (a *App) CompleteDataSubjectRequest(w http.ResponseWriter, r *http.Request) {
	d, ok := a.loadDSRForHandler(w, r)
	if !ok {
		return
	}
	var body struct {
		Resolution string `json:"resolution"`
		Confirm    bool   `json:"confirm"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Resolution = strings.TrimSpace(body.Resolution)
	if len([]rune(body.Resolution)) < 10 {
		common.WriteValidationError(w, http.StatusBadRequest, "Catatan penyelesaian minimal 10 karakter", map[string]string{"resolution": "Minimal 10 karakter"})
		return
	}
	if d.Status != "submitted" && d.Status != "in_review" {
		common.WriteError(w, http.StatusConflict, "Permintaan sudah selesai")
		return
	}
	claims := common.FromCtx(r)
	switch d.RequestType {
	case "access":
		export, err := a.personalDataExport(r.Context(), d.SubjectUserID)
		if err != nil {
			common.WriteDBError(w, err, "Gagal menyusun salinan data")
			return
		}
		a.finishDSR(w, r, d, "completed", body.Resolution, export)
	case "correction":
		if errs := validateCorrections(d.Corrections); len(errs) > 0 {
			common.WriteValidationError(w, http.StatusUnprocessableEntity, "Data koreksi tidak valid; tolak permintaan atau minta pengajuan ulang", errs)
			return
		}
		if msg, field := a.applyCorrections(r.Context(), d.SubjectUserID, d.Corrections); msg != "" {
			common.WriteValidationError(w, http.StatusUnprocessableEntity, msg, map[string]string{field: msg})
			return
		}
		a.finishDSR(w, r, d, "completed", body.Resolution, nil)
	case "deletion":
		if !body.Confirm {
			common.WriteJSONMsg(w, http.StatusConflict, map[string]any{"requires_confirmation": true}, nil,
				"Penghapusan menganonimkan akun "+d.SubjectName+" dan tidak dapat dibatalkan. Konfirmasi untuk melanjutkan")
			return
		}
		if d.SubjectUserID == claims.UserID {
			common.WriteError(w, http.StatusConflict, "Akun Anda sendiri harus dianonimkan oleh admin lain")
			return
		}
		if err := a.anonymizeUser(r.Context(), d.SubjectUserID); err != nil {
			common.WriteDBError(w, err, "Gagal menganonimkan akun")
			return
		}
		a.finishDSR(w, r, d, "completed", body.Resolution, nil)
	}
}

func (a *App) finishDSR(w http.ResponseWriter, r *http.Request, d dataSubjectRequest, status, resolution string, export map[string]any) {
	claims := common.FromCtx(r)
	var exp any
	if export != nil {
		raw, _ := json.Marshal(export)
		exp = raw
	}
	res, err := a.DB.ExecContext(r.Context(), `UPDATE data_subject_requests SET status=$2, resolution=$3, handled_by=$4, handled_at=now(),
			export_payload=COALESCE($5, export_payload), updated_at=now()
		WHERE id=$1 AND status IN ('submitted','in_review')`, d.ID, status, resolution, nullUUID(claims.UserID), exp)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memperbarui permintaan")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusConflict, "Permintaan sudah selesai")
		return
	}
	a.RecordSecurityAudit(d.SchoolID, "data_subject_request_"+status, d.SubjectUserID, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"request_id": d.ID, "type": d.RequestType}, resolution, r.Header.Get("X-Correlation-ID"))
	if d.RequestedBy != nil && d.SchoolID != nil {
		title := "Permintaan data pribadi selesai"
		if status == "rejected" {
			title = "Permintaan data pribadi ditolak"
		}
		go func(school, to string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = a.notifyUser(ctx, school, to, "privacy_request", title, resolution, "privacy-request:"+d.ID+":"+status,
				map[string]any{"request_id": d.ID})
		}(*d.SchoolID, *d.RequestedBy)
	}
	out, _ := a.getDSR(r.Context(), d.ID)
	msg := "Permintaan ditolak"
	if status == "completed" {
		msg = map[string]string{
			"access":     "Salinan data pribadi siap diunduh oleh pemohon",
			"correction": "Data pribadi telah dikoreksi",
			"deletion":   "Akun telah dianonimkan; data pribadi dihapus",
		}[d.RequestType]
	}
	common.WriteJSONMsg(w, http.StatusOK, out, nil, msg)
}

// personalDataExport collects what identity holds about a user.
func (a *App) personalDataExport(ctx context.Context, userID string) (map[string]any, error) {
	var profile struct {
		Name, Email                                     string
		Phone, Address, BirthPlace, Religion, NIK, NISN sql.NullString
		BirthDate                                       sql.NullString
		CreatedAt                                       time.Time
	}
	err := a.DB.QueryRowContext(ctx, `SELECT name, email, phone, address, birth_place, religion, nik, nisn,
			to_char(birth_date,'YYYY-MM-DD'), created_at FROM users WHERE id::text=$1`, userID).Scan(
		&profile.Name, &profile.Email, &profile.Phone, &profile.Address, &profile.BirthPlace, &profile.Religion,
		&profile.NIK, &profile.NISN, &profile.BirthDate, &profile.CreatedAt)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"generated_at": time.Now(),
		"profile": map[string]any{
			"name": profile.Name, "email": profile.Email, "phone": profile.Phone.String, "address": profile.Address.String,
			"birth_place": profile.BirthPlace.String, "birth_date": profile.BirthDate.String, "religion": profile.Religion.String,
			"nik": profile.NIK.String, "nisn": profile.NISN.String, "created_at": profile.CreatedAt,
		},
	}
	q := func(key, query string) error {
		rows, err := a.DB.QueryContext(ctx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		list := []map[string]any{}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
			m := map[string]any{}
			for i, c := range cols {
				if b, ok := vals[i].([]byte); ok {
					m[c] = string(b)
				} else {
					m[c] = vals[i]
				}
			}
			list = append(list, m)
		}
		out[key] = list
		return rows.Err()
	}
	steps := []struct{ key, query string }{
		{"roles", `SELECT s.name AS unit, r.role, r.status, r.student_status, r.created_at FROM user_school_roles r
			LEFT JOIN schools s ON s.id=r.school_id WHERE r.user_id::text=$1 ORDER BY r.created_at`},
		{"guardian_links", `SELECT g.relationship, g.status, gu.name AS guardian, st.name AS student, g.created_at
			FROM guardian_links g JOIN users gu ON gu.id=g.guardian_user_id JOIN users st ON st.id=g.student_user_id
			WHERE g.guardian_user_id::text=$1 OR g.student_user_id::text=$1`},
		{"login_history", `SELECT created_at, success, reason, ip FROM login_events WHERE user_id::text=$1 ORDER BY created_at DESC LIMIT 100`},
		{"student_status_history", `SELECT from_status, to_status, reason, changed_at FROM student_status_history WHERE user_id::text=$1 ORDER BY changed_at`},
	}
	for _, s := range steps {
		if err := q(s.key, s.query); err != nil {
			return nil, err
		}
	}
	out["note"] = "Data dari layanan lain (nilai, presensi, tagihan) dapat diminta melalui admin unit; layanan tersebut menyimpan data dengan ID pengguna yang sama."
	return out, nil
}

// applyCorrections writes validated correction values. Returns a readable
// message and field when the change is refused (e.g. duplicate NIK).
func (a *App) applyCorrections(ctx context.Context, userID string, c map[string]string) (string, string) {
	if v, ok := c["religion"]; ok && v != "" {
		label, found, err := a.referenceLabel(ctx, "agama", v, "")
		if err != nil {
			return "Gagal memeriksa data referensi agama", "religion"
		}
		if !found {
			return "Agama tidak ada di data referensi", "religion"
		}
		c["religion"] = label
	}
	sets := []string{}
	args := []any{userID}
	for _, k := range []string{"name", "phone", "address", "birth_date", "birth_place", "religion", "nik", "nisn"} {
		v, ok := c[k]
		if !ok {
			continue
		}
		args = append(args, nullIfEmpty(v))
		sets = append(sets, fmt.Sprintf("%s=$%d", k, len(args)))
	}
	if len(sets) == 0 {
		return "Tidak ada data yang dikoreksi", "corrections"
	}
	_, err := a.DB.ExecContext(ctx, `UPDATE users SET `+strings.Join(sets, ", ")+`, updated_at=now() WHERE id::text=$1 AND deleted_at IS NULL`, args...)
	if err != nil {
		status, msg, field := common.DBErrorStatus(err, "Gagal menyimpan koreksi")
		if status >= 500 {
			common.CaptureError(err)
		}
		if field == "" {
			field = "corrections"
		}
		return msg, field
	}
	return "", ""
}

// anonymizeUser removes the PII of an account and disables it. The row and
// its id stay so records other services keep for legal retention remain
// consistent.
func (a *App) anonymizeUser(ctx context.Context, userID string) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET name='Data dihapus (UU PDP)', email='pdp-deleted-' || id::text || '@invalid.local',
			phone=NULL, photo=NULL, birth_date=NULL, birth_place=NULL, address=NULL, religion=NULL, nik=NULL, nisn=NULL,
			extra=NULL, default_password=NULL, mfa_secret_enc=NULL, mfa_enabled_at=NULL,
			deleted_at=COALESCE(deleted_at, now()), anonymized_at=now(), tokens_valid_after=now(), updated_at=now()
		WHERE id::text=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_school_roles SET status='inactive', deactivated_at=COALESCE(deactivated_at, now()) WHERE user_id::text=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE guardian_links SET status='inactive', updated_at=now() WHERE guardian_user_id::text=$1 OR student_user_id::text=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_sessions SET revoked_at=now(), revoked_reason='anonymized' WHERE user_id::text=$1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE login_events SET email='(dihapus)', ip=NULL, user_agent=NULL WHERE user_id::text=$1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// DataSubjectRequestExport: GET /privacy/requests/{id}/export — the subject,
// the requester or a unit admin downloads the JSON copy.
func (a *App) DataSubjectRequestExport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	var subject, school sql.NullString
	var requester sql.NullString
	var payload []byte
	err := a.DB.QueryRowContext(r.Context(), `SELECT subject_user_id::text, requested_by::text, school_id::text, export_payload
		FROM data_subject_requests WHERE id::text=$1 AND request_type='access' AND status='completed'`, chi.URLParam(r, "id")).
		Scan(&subject, &requester, &school, &payload)
	allowed := err == nil && (subject.String == claims.UserID || requester.String == claims.UserID ||
		claims.ActiveRole == common.RoleSuperadmin || (claims.ActiveRole == common.RoleAdmin && school.String == claims.SchoolID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !allowed) || (err == nil && len(payload) == 0) {
		common.WriteError(w, http.StatusNotFound, "Salinan data tidak tersedia")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat salinan data")
		return
	}
	a.RecordSecurityAudit(nullStr(school.String), "data_exported", subject.String, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"filter": map[string]string{"request_id": chi.URLParam(r, "id")}, "format": "json"}, "PDP access request", r.Header.Get("X-Correlation-ID"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="data-pribadi.json"`)
	_, _ = w.Write(payload)
}
