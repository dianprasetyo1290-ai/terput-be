package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/userimport"
)

// Bulk user import ("Import Excel" on Data Pengguna / Siswa / Guru).
//
//	POST /users/import  multipart: file (.xlsx|.csv), role?, school_id? (superadmin),
//	                    mapping? (JSON {"name":"<header>",...}), dry_run?, skip_invalid?
//	  dry_run=true  -> 200 preview {total, valid, invalid, rows, errors}; nothing saved (BUG-018)
//	  commit        -> all-or-nothing: invalid rows give 422 unless skip_invalid=true,
//	                   then the valid rows are saved in ONE transaction and recorded
//	                   as an import batch (BUG-020/077)
//	GET  /users/import-batches                 -> recent batches of the unit
//	POST /users/import-batches/{id}/rollback   -> {reason}; removes accounts still untouched
//
// Format/headers/size are checked before any row (BUG-076/080/081/151).

const maxImportBytes = 10 << 20

type importPreview struct {
	Total   int                   `json:"total"`
	Valid   int                   `json:"valid"`
	Invalid int                   `json:"invalid"`
	Format  string                `json:"format"`
	Rows    []userimport.Row      `json:"rows"`
	Errors  []userimport.RowError `json:"errors"`
	DryRun  bool                  `json:"dry_run"`
	BatchID string                `json:"batch_id,omitempty"`
	Created int                   `json:"created"`
	Skipped int                   `json:"skipped"`
}

// importAllowedRoles: admin imports guru/siswa only; superadmin any role
// except superadmin.
func importAllowedRoles(activeRole string) map[string]bool {
	if activeRole == common.RoleSuperadmin {
		out := map[string]bool{}
		for _, r := range common.AllRoles {
			if r != common.RoleSuperadmin {
				out[r] = true
			}
		}
		return out
	}
	return map[string]bool{common.RoleGuru: true, common.RoleSiswa: true}
}

func formBool(r *http.Request, k string) bool {
	b, _ := strconv.ParseBool(strings.TrimSpace(r.FormValue(k)))
	return b
}

func (a *App) ImportUsers(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+1<<20)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Unggahan gagal dibaca. Ukuran file maksimal 10 MB")
		return
	}
	allowed := importAllowedRoles(claims.ActiveRole)
	role := strings.ToLower(strings.TrimSpace(r.FormValue("role")))
	if role != "" {
		if !common.IsValidRole(role) || role == common.RoleSuperadmin {
			common.WriteValidationError(w, http.StatusBadRequest, "Peran tidak valid", map[string]string{"role": "Peran tidak valid"})
			return
		}
		if !allowed[role] {
			common.WriteError(w, http.StatusForbidden, "Admin unit hanya dapat mengimpor akun guru atau siswa")
			return
		}
	}
	schoolID := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin {
		if sid := strings.TrimSpace(r.FormValue("school_id")); sid != "" {
			schoolID = sid
		}
	}
	if schoolID == "" {
		common.WriteValidationError(w, http.StatusBadRequest, "Pilih unit tujuan impor", map[string]string{"school_id": "Unit wajib dipilih"})
		return
	}
	var mapping map[string]string
	if m := strings.TrimSpace(r.FormValue("mapping")); m != "" {
		if err := json.Unmarshal([]byte(m), &mapping); err != nil {
			common.WriteValidationError(w, http.StatusBadRequest, "Pemetaan kolom tidak valid", map[string]string{"mapping": "Harus berupa JSON {\"name\":\"Judul Kolom\", ...}"})
			return
		}
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		common.WriteValidationError(w, http.StatusBadRequest, "File impor wajib diunggah", map[string]string{"file": "File wajib diunggah"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil || len(data) > maxImportBytes {
		common.WriteError(w, http.StatusBadRequest, "Ukuran file maksimal 10 MB")
		return
	}

	table, format, err := userimport.ReadTable(hdr.Filename, data)
	if err != nil {
		common.WriteValidationError(w, http.StatusBadRequest, err.Error(), map[string]string{"file": err.Error()})
		return
	}
	if len(table) == 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "File kosong. Gunakan template impor", map[string]string{"file": "File kosong"})
		return
	}
	cols, missing := userimport.MapColumns(table[0], mapping, role != "")
	if len(missing) > 0 {
		msg := "Kolom wajib tidak ditemukan: " + strings.Join(missing, ", ") + ". Sesuaikan judul kolom dengan template atau isi pemetaan kolom"
		common.WriteJSONMsg(w, http.StatusBadRequest, map[string]any{"missing_columns": missing, "headers": table[0]}, nil, msg)
		return
	}
	dataRows := table[1:]
	if n := userimport.DataRowCount(dataRows); n == 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "File tidak berisi data", map[string]string{"file": "Tidak ada baris data"})
		return
	} else if n > userimport.MaxRows {
		msg := fmt.Sprintf("File berisi %d baris data; maksimal %d baris per impor. Pecah file menjadi beberapa bagian", n, userimport.MaxRows)
		common.WriteValidationError(w, http.StatusBadRequest, msg, map[string]string{"file": msg})
		return
	}

	taken, err := a.existingEmails(r, dataRows, cols)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa data impor")
		return
	}
	valid, rowErrs := userimport.Validate(dataRows, cols, role, allowed, taken)
	out := importPreview{
		Total: len(valid) + len(rowErrs), Valid: len(valid), Invalid: len(rowErrs), Format: format,
		Rows: valid, Errors: rowErrs, DryRun: formBool(r, "dry_run"),
	}
	if out.Rows == nil {
		out.Rows = []userimport.Row{}
	}
	if out.Errors == nil {
		out.Errors = []userimport.RowError{}
	}
	if out.DryRun {
		common.WriteJSONMsg(w, http.StatusOK, out, nil, fmt.Sprintf("Pratinjau: %d baris valid, %d baris bermasalah. Belum ada data yang disimpan", out.Valid, out.Invalid))
		return
	}
	if len(rowErrs) > 0 && !formBool(r, "skip_invalid") {
		common.WriteJSONMsg(w, http.StatusUnprocessableEntity, out, nil,
			fmt.Sprintf("%d baris bermasalah; tidak ada data yang disimpan. Perbaiki file atau lanjutkan hanya baris valid", out.Invalid))
		return
	}
	if len(valid) == 0 {
		common.WriteJSONMsg(w, http.StatusUnprocessableEntity, out, nil, "Tidak ada baris valid untuk disimpan")
		return
	}

	batchID, err := a.commitImport(r, schoolID, hdr.Filename, format, valid, len(rowErrs), claims.UserID)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "Sebagian email baru saja didaftarkan oleh proses lain. Ulangi pratinjau")
			return
		}
		common.WriteDBError(w, err, "Gagal menyimpan data impor")
		return
	}
	out.BatchID, out.Created, out.Skipped = batchID, len(valid), len(rowErrs)
	sid := schoolID
	a.RecordSecurityAudit(&sid, "users_imported", batchID, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"file": hdr.Filename, "created": len(valid), "skipped": len(rowErrs)}, "bulk import", r.Header.Get("X-Correlation-ID"))
	common.WriteJSONMsg(w, http.StatusOK, out, nil, fmt.Sprintf("%d pengguna berhasil diimpor, %d baris dilewati", out.Created, out.Skipped))
}

// existingEmails returns the lower-cased emails of the file that already
// belong to an account (users.email is unique, deleted accounts included).
func (a *App) existingEmails(r *http.Request, rows [][]string, cols map[string]int) (map[string]bool, error) {
	idx, ok := cols[userimport.FieldEmail]
	taken := map[string]bool{}
	if !ok {
		return taken, nil
	}
	var emails []string
	for _, row := range rows {
		if idx < len(row) {
			if e := strings.ToLower(strings.TrimSpace(row[idx])); e != "" {
				emails = append(emails, e)
			}
		}
	}
	if len(emails) == 0 {
		return taken, nil
	}
	q, err := a.DB.QueryContext(r.Context(), `SELECT lower(email) FROM users WHERE lower(email) = ANY($1::text[])`, stringArray(emails))
	if err != nil {
		return nil, err
	}
	defer q.Close()
	for q.Next() {
		var e string
		if err := q.Scan(&e); err != nil {
			return nil, err
		}
		taken[e] = true
	}
	return taken, q.Err()
}

func (a *App) commitImport(r *http.Request, schoolID, fileName, format string, rows []userimport.Row, skipped int, actor string) (string, error) {
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var batchID string
	if err := tx.QueryRow(`INSERT INTO import_batches (school_id, kind, file_name, file_format, total_rows, created_rows, skipped_rows, created_by)
		VALUES ($1,'users',$2,$3,$4,$5,$6,$7) RETURNING id`,
		schoolID, fileName, format, len(rows)+skipped, len(rows), skipped, nullUUID(actor)).Scan(&batchID); err != nil {
		return "", err
	}
	for _, row := range rows {
		userID, _, err := createImportedUser(tx, row.Name, row.Email, row.Phone, "", "", "", "", nil)
		if err != nil {
			return "", err
		}
		var studentStatus any
		if row.Role == common.RoleSiswa {
			studentStatus = "aktif"
		}
		if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role, student_status) VALUES ($1,$2,$3,$4)`,
			userID, schoolID, row.Role, studentStatus); err != nil {
			return "", err
		}
		if _, err := tx.Exec(`INSERT INTO import_batch_items (batch_id, row_number, user_id, role, email, source_ref)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,''))`, batchID, row.Number, userID, row.Role, row.Email, row.SourceRef); err != nil {
			return "", err
		}
	}
	return batchID, tx.Commit()
}

type importBatch struct {
	ID             string     `json:"id"`
	SchoolID       *string    `json:"school_id"`
	FileName       *string    `json:"file_name"`
	FileFormat     string     `json:"file_format"`
	TotalRows      int        `json:"total_rows"`
	CreatedRows    int        `json:"created_rows"`
	SkippedRows    int        `json:"skipped_rows"`
	CreatedBy      *string    `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	RolledBackAt   *time.Time `json:"rolled_back_at"`
	RollbackReason *string    `json:"rollback_reason"`
}

// importBatchScope: admin sees its own unit, superadmin all or ?school_id=.
func importBatchScope(r *http.Request) string {
	c := common.FromCtx(r)
	if c.ActiveRole == common.RoleSuperadmin {
		return r.URL.Query().Get("school_id")
	}
	return c.SchoolID
}

// ListImportBatches: GET /users/import-batches.
func (a *App) ListImportBatches(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	if common.FromCtx(r) == nil { // internal callers have no unit scope
		common.WriteError(w, http.StatusForbidden, "Hanya untuk pengguna")
		return
	}
	scope := importBatchScope(r)
	rows, err := a.DB.QueryContext(r.Context(), `SELECT id, school_id::text, file_name, file_format, total_rows, created_rows, skipped_rows,
			created_by::text, created_at, rolled_back_at, rollback_reason
		FROM import_batches WHERE kind='users' AND ($1 = '' OR school_id::text = $1)
		ORDER BY created_at DESC LIMIT 100`, scope)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat riwayat impor")
		return
	}
	defer rows.Close()
	out := []importBatch{}
	for rows.Next() {
		var b importBatch
		if err := rows.Scan(&b.ID, &b.SchoolID, &b.FileName, &b.FileFormat, &b.TotalRows, &b.CreatedRows, &b.SkippedRows,
			&b.CreatedBy, &b.CreatedAt, &b.RolledBackAt, &b.RollbackReason); err != nil {
			common.WriteDBError(w, err, "Gagal memuat riwayat impor")
			return
		}
		out = append(out, b)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

type rollbackBlocked struct {
	Row    int    `json:"row"`
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

// RollbackImportBatch: POST /users/import-batches/{id}/rollback {reason}.
// All-or-nothing: if any imported account has been used (signed in, given
// another role, linked to a guardian, taken over by HR) nothing is undone
// and the blocking rows are listed, so the admin fixes those by hand.
// Otherwise the accounts are soft-deleted and their emails released so the
// corrected file can be imported again.
func (a *App) RollbackImportBatch(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) < 10 {
		common.WriteValidationError(w, http.StatusBadRequest, "Alasan pembatalan impor wajib diisi (minimal 10 karakter)", map[string]string{"reason": "Minimal 10 karakter"})
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membatalkan impor")
		return
	}
	defer tx.Rollback()
	var schoolID sql.NullString
	var rolledBack *time.Time
	err = tx.QueryRow(`SELECT school_id::text, rolled_back_at FROM import_batches WHERE id::text=$1 AND kind='users' FOR UPDATE`, id).Scan(&schoolID, &rolledBack)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && claims.ActiveRole != common.RoleSuperadmin && schoolID.String != claims.SchoolID) {
		common.WriteError(w, http.StatusNotFound, "Riwayat impor tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal membatalkan impor")
		return
	}
	if rolledBack != nil {
		common.WriteError(w, http.StatusConflict, "Impor ini sudah dibatalkan")
		return
	}
	rows, err := tx.Query(`SELECT i.row_number, i.user_id::text, i.email,
			CASE
			  WHEN u.deleted_at IS NOT NULL THEN 'akun sudah dihapus'
			  WHEN u.managed_by_hr THEN 'akun sudah dikelola HR'
			  WHEN EXISTS (SELECT 1 FROM login_events le WHERE le.user_id=u.id AND le.success) THEN 'akun sudah pernah dipakai masuk'
			  WHEN EXISTS (SELECT 1 FROM user_school_roles x WHERE x.user_id=u.id AND NOT (x.school_id IS NOT DISTINCT FROM b.school_id AND x.role=i.role)) THEN 'akun sudah memiliki peran lain'
			  WHEN EXISTS (SELECT 1 FROM guardian_links g WHERE g.student_user_id=u.id OR g.guardian_user_id=u.id) THEN 'akun sudah terhubung dengan wali/siswa'
			  ELSE ''
			END
		FROM import_batch_items i
		JOIN import_batches b ON b.id=i.batch_id
		JOIN users u ON u.id=i.user_id
		WHERE i.batch_id::text=$1 ORDER BY i.row_number`, id)
	if err != nil {
		common.WriteDBError(w, err, "Gagal membatalkan impor")
		return
	}
	var blocked []rollbackBlocked
	var userIDs []string
	for rows.Next() {
		var b rollbackBlocked
		if err := rows.Scan(&b.Row, &b.UserID, &b.Email, &b.Reason); err != nil {
			rows.Close()
			common.WriteDBError(w, err, "Gagal membatalkan impor")
			return
		}
		if b.Reason != "" {
			blocked = append(blocked, b)
		} else {
			userIDs = append(userIDs, b.UserID)
		}
	}
	rows.Close()
	if len(blocked) > 0 {
		common.WriteJSONMsg(w, http.StatusConflict, map[string]any{"blocked": blocked}, nil,
			fmt.Sprintf("%d akun hasil impor sudah dipakai sehingga impor tidak dapat dibatalkan otomatis. Tangani akun tersebut secara manual", len(blocked)))
		return
	}
	if len(userIDs) > 0 {
		if _, err := tx.Exec(`UPDATE users SET deleted_at=now(), updated_at=now(),
				email = 'import-rollback-' || id::text || '@invalid.local'
			WHERE id::text = ANY($1::text[]) AND deleted_at IS NULL`, stringArray(userIDs)); err != nil {
			common.WriteDBError(w, err, "Gagal membatalkan impor")
			return
		}
		if _, err := tx.Exec(`UPDATE user_school_roles SET status='inactive' WHERE user_id::text = ANY($1::text[])`, stringArray(userIDs)); err != nil {
			common.WriteDBError(w, err, "Gagal membatalkan impor")
			return
		}
	}
	if _, err := tx.Exec(`UPDATE import_batches SET rolled_back_at=now(), rolled_back_by=$2, rollback_reason=$3 WHERE id::text=$1`,
		id, nullUUID(claims.UserID), req.Reason); err != nil {
		common.WriteDBError(w, err, "Gagal membatalkan impor")
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membatalkan impor")
		return
	}
	var sid *string
	if schoolID.Valid {
		sid = &schoolID.String
	}
	a.RecordSecurityAudit(sid, "users_import_rolled_back", id, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"removed": len(userIDs)}, req.Reason, r.Header.Get("X-Correlation-ID"))
	common.WriteJSONMsg(w, http.StatusOK, map[string]any{"batch_id": id, "removed": len(userIDs)}, nil,
		fmt.Sprintf("Impor dibatalkan; %d akun dihapus", len(userIDs)))
}
