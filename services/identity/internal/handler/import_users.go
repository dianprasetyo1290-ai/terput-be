package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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
	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind == "" {
		kind = userimport.KindUsers
	}
	if kind != userimport.KindUsers && kind != userimport.KindOrangTua && kind != userimport.KindKelas {
		common.WriteValidationError(w, http.StatusBadRequest, "Jenis impor tidak dikenal", map[string]string{"kind": "Pilih: users, orang_tua, atau kelas"})
		return
	}
	run, runMsg := a.runForImport(r.Context(), strings.TrimSpace(r.FormValue("run_id")), schoolID)
	if runMsg != "" {
		common.WriteValidationError(w, http.StatusBadRequest, runMsg, map[string]string{"run_id": runMsg})
		return
	}
	if kind != userimport.KindUsers {
		a.importKind(w, r, kindImport{kind: kind, schoolID: schoolID, fileName: hdr.Filename, format: format,
			mapping: mapping, table: table, run: run, actor: claims.UserID, actorRole: claims.ActiveRole})
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
	if valid, rowErrs, err = a.rejectMigratedRefs(r.Context(), schoolID, valid, rowErrs); err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa data impor")
		return
	}
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

	batchID, err := a.commitImport(r, batchMeta{SchoolID: schoolID, Kind: userimport.KindUsers, FileName: hdr.Filename, Format: format,
		Actor: claims.UserID, Total: len(valid) + len(rowErrs), Created: len(valid), Skipped: len(rowErrs), Run: run}, valid)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "Sebagian email baru saja didaftarkan oleh proses lain. Ulangi pratinjau")
			return
		}
		a.reportImportFailure(r, userimport.KindUsers, schoolID, hdr.Filename, "Impor pengguna gagal disimpan: "+err.Error(),
			map[string]any{"rows": len(valid)})
		common.WriteDBError(w, err, "Gagal menyimpan data impor")
		return
	}
	out.BatchID, out.Created, out.Skipped = batchID, len(valid), len(rowErrs)
	if run != nil && run.Mode == "trial" {
		common.WriteJSONMsg(w, http.StatusOK, out, nil, fmt.Sprintf("Impor percobaan: %d pengguna akan dibuat, %d baris dilewati. Tidak ada data yang disimpan", out.Created, out.Skipped))
		return
	}
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

// commitImport saves the rows in one transaction. In a trial run the same
// statements run and are rolled back; only the batch summary is kept.
func (a *App) commitImport(r *http.Request, m batchMeta, rows []userimport.Row) (string, error) {
	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	m.Counts = map[string]int{}
	for _, row := range rows {
		m.Counts[row.Role]++
		if m.isTrial() && row.Role == common.RoleSiswa && row.SourceRef != "" {
			m.TrialRefs = append(m.TrialRefs, "ref:"+row.SourceRef)
		}
	}
	batchID, err := insertBatch(ctx, tx, m)
	if err != nil {
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
			userID, m.SchoolID, row.Role, studentStatus); err != nil {
			return "", err
		}
		if _, err := tx.Exec(`INSERT INTO import_batch_items (batch_id, row_number, user_id, role, email, source_ref, entity_type, entity_id)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),'user',$7)`, batchID, row.Number, userID, row.Role, row.Email, row.SourceRef, userID); err != nil {
			return "", err
		}
		if err := mapSourceID(ctx, tx, m.SchoolID, row.Role, row.SourceRef, userID, batchID); err != nil {
			return "", err
		}
	}
	if m.isTrial() {
		if err := tx.Rollback(); err != nil {
			return "", err
		}
		return insertBatch(ctx, a.DB, m)
	}
	return batchID, tx.Commit()
}

// rejectMigratedRefs turns rows whose ID Sumber was already migrated into
// row errors, so importing the same legacy file twice cannot duplicate
// people (BUG-077).
func (a *App) rejectMigratedRefs(ctx context.Context, schoolID string, rows []userimport.Row, errs []userimport.RowError) ([]userimport.Row, []userimport.RowError, error) {
	var keys []string
	for _, r := range rows {
		if r.SourceRef != "" {
			keys = append(keys, r.Role+"|"+r.SourceRef)
		}
	}
	if len(keys) == 0 {
		return rows, errs, nil
	}
	q, err := a.DB.QueryContext(ctx, `SELECT entity_type||'|'||source_ref FROM source_id_map
		WHERE school_id::text=$1 AND entity_type||'|'||source_ref = ANY($2::text[])`, schoolID, stringArray(keys))
	if err != nil {
		return nil, nil, err
	}
	defer q.Close()
	done := map[string]bool{}
	for q.Next() {
		var k string
		if err := q.Scan(&k); err != nil {
			return nil, nil, err
		}
		done[k] = true
	}
	if err := q.Err(); err != nil {
		return nil, nil, err
	}
	var keep []userimport.Row
	for _, r := range rows {
		if r.SourceRef != "" && done[r.Role+"|"+r.SourceRef] {
			errs = append(errs, userimport.RowError{Row: r.Number, Issue: fmt.Sprintf("ID Sumber %q sudah pernah dimigrasikan", r.SourceRef)})
			continue
		}
		keep = append(keep, r)
	}
	sort.Slice(errs, func(i, j int) bool { return errs[i].Row < errs[j].Row })
	return keep, errs, nil
}

type importBatch struct {
	ID             string         `json:"id"`
	SchoolID       *string        `json:"school_id"`
	Kind           string         `json:"kind"`
	FileName       *string        `json:"file_name"`
	FileFormat     string         `json:"file_format"`
	TotalRows      int            `json:"total_rows"`
	CreatedRows    int            `json:"created_rows"`
	SkippedRows    int            `json:"skipped_rows"`
	CreatedBy      *string        `json:"created_by"`
	CreatedAt      time.Time      `json:"created_at"`
	RolledBackAt   *time.Time     `json:"rolled_back_at"`
	RollbackReason *string        `json:"rollback_reason"`
	RunID          *string        `json:"run_id"`
	Trial          bool           `json:"trial"`
	CategoryCounts map[string]int `json:"category_counts"`
}

// importBatchScope: admin sees its own unit, superadmin all or ?school_id=.
func importBatchScope(r *http.Request) string {
	c := common.FromCtx(r)
	if c.ActiveRole == common.RoleSuperadmin {
		return r.URL.Query().Get("school_id")
	}
	return c.SchoolID
}

// listBatches returns recent batches of a unit ("" = all) or of one run.
func (a *App) listBatches(ctx context.Context, scope, runID string) ([]importBatch, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT id, school_id::text, kind, file_name, file_format, total_rows, created_rows, skipped_rows,
			created_by::text, created_at, rolled_back_at, rollback_reason, run_id::text, trial, category_counts
		FROM import_batches
		WHERE ($1 = '' OR school_id::text = $1) AND ($2 = '' OR run_id::text = $2)
		ORDER BY created_at DESC LIMIT 100`, scope, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []importBatch{}
	for rows.Next() {
		var b importBatch
		var counts []byte
		if err := rows.Scan(&b.ID, &b.SchoolID, &b.Kind, &b.FileName, &b.FileFormat, &b.TotalRows, &b.CreatedRows, &b.SkippedRows,
			&b.CreatedBy, &b.CreatedAt, &b.RolledBackAt, &b.RollbackReason, &b.RunID, &b.Trial, &counts); err != nil {
			return nil, err
		}
		b.CategoryCounts = map[string]int{}
		_ = json.Unmarshal(counts, &b.CategoryCounts)
		out = append(out, b)
	}
	return out, rows.Err()
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
	out, err := a.listBatches(r.Context(), importBatchScope(r), "")
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat riwayat impor")
		return
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
	var kind string
	var trial bool
	err = tx.QueryRow(`SELECT school_id::text, rolled_back_at, kind, trial FROM import_batches WHERE id::text=$1 FOR UPDATE`, id).Scan(&schoolID, &rolledBack, &kind, &trial)
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
	if trial {
		common.WriteError(w, http.StatusConflict, "Impor percobaan tidak menyimpan data, jadi tidak ada yang perlu dibatalkan")
		return
	}
	if kind == userimport.KindKelas {
		common.WriteError(w, http.StatusConflict, "Impor kelas tidak dapat dibatalkan otomatis. Ubah anggota kelas melalui menu Kelas")
		return
	}
	// Parent imports: drop the links made by this batch first, so only links
	// made afterwards (by hand) block removing the accounts.
	var linksRemoved int64
	if kind == userimport.KindOrangTua {
		res, err := tx.Exec(`DELETE FROM guardian_links WHERE id::text IN (
			SELECT entity_id FROM import_batch_items WHERE batch_id::text=$1 AND entity_type='guardian_link')`, id)
		if err != nil {
			common.WriteDBError(w, err, "Gagal membatalkan impor")
			return
		}
		linksRemoved, _ = res.RowsAffected()
		// Existing accounts that only received the orang_tua role lose it
		// when they have no other active child in this unit.
		if _, err := tx.Exec(`DELETE FROM user_school_roles x USING import_batch_items i, import_batches b
			WHERE i.batch_id::text=$1 AND i.entity_type='user' AND NOT i.created_account AND b.id=i.batch_id
			  AND x.user_id=i.user_id AND x.school_id=b.school_id AND x.role='orang_tua'
			  AND NOT EXISTS (SELECT 1 FROM guardian_links g WHERE g.guardian_user_id=i.user_id AND g.school_id=b.school_id AND g.status='active')`, id); err != nil {
			common.WriteDBError(w, err, "Gagal membatalkan impor")
			return
		}
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
		WHERE i.batch_id::text=$1 AND i.entity_type='user' AND i.created_account ORDER BY i.row_number`, id)
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
	if _, err := tx.Exec(`DELETE FROM source_id_map WHERE batch_id::text=$1`, id); err != nil {
		common.WriteDBError(w, err, "Gagal membatalkan impor")
		return
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
		map[string]any{"removed": len(userIDs), "links_removed": linksRemoved, "kind": kind}, req.Reason, r.Header.Get("X-Correlation-ID"))
	msg := fmt.Sprintf("Impor dibatalkan; %d akun dihapus", len(userIDs))
	if kind == userimport.KindOrangTua {
		msg = fmt.Sprintf("Impor dibatalkan; %d relasi dan %d akun orang tua baru dihapus", linksRemoved, len(userIDs))
	}
	common.WriteJSONMsg(w, http.StatusOK, map[string]any{"batch_id": id, "removed": len(userIDs), "links_removed": linksRemoved}, nil, msg)
}
