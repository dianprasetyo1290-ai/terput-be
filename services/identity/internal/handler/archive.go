package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/userimport"
)

// Arsip historis read-only (QA-IDN BUG-079): data of the old system that is
// not migrated into live modules (old report cards, old payments, letters…)
// is imported per category as searchable read-only records. Rows can never
// be edited (DB trigger); a wrong file is corrected by importing a new
// category version, never by changing history.
//
//	POST /archive/import   multipart: file, category, title_column?, school_id? (superadmin), run_id?
//	GET  /archive/categories
//	GET  /archive?category=&q=&page=&per_page=
//	GET  /archive/{id}

const maxArchiveRows = 5000

var archiveViewRoles = []string{common.RoleAdmin, common.RoleKepalaSekolah, common.RoleKepalaYayasan}

type archiveRecord struct {
	ID         string          `json:"id"`
	SchoolID   string          `json:"school_id"`
	RunID      *string         `json:"run_id"`
	Category   string          `json:"category"`
	SourceRef  *string         `json:"source_ref"`
	Title      string          `json:"title"`
	Payload    json.RawMessage `json:"payload"`
	FileName   *string         `json:"file_name"`
	ImportedAt time.Time       `json:"imported_at"`
}

var titleHeaders = []string{"judul", "nama", "nama siswa", "title", "uraian", "keterangan"}

// archiveRows turns a table into (title, source_ref, payload) records.
// titleColumn picks the title; otherwise a known header or the first column.
func archiveRows(table [][]string, titleColumn string) ([]archiveRow, string) {
	if len(table) < 2 {
		return nil, "File tidak berisi data"
	}
	header := make([]string, len(table[0]))
	titleIdx, refIdx := -1, -1
	seen := map[string]bool{}
	for i, h := range table[0] {
		h = strings.TrimSpace(h)
		if h == "" {
			h = fmt.Sprintf("Kolom %d", i+1)
		}
		if seen[strings.ToLower(h)] {
			return nil, fmt.Sprintf("Judul kolom %q muncul lebih dari sekali", h)
		}
		seen[strings.ToLower(h)] = true
		header[i] = h
		low := strings.ToLower(h)
		if titleColumn != "" && strings.EqualFold(h, strings.TrimSpace(titleColumn)) {
			titleIdx = i
		}
		if refIdx < 0 && (low == "id sumber" || low == "id lama" || low == "kode lama") {
			refIdx = i
		}
	}
	if titleColumn != "" && titleIdx < 0 {
		return nil, fmt.Sprintf("Kolom judul %q tidak ditemukan", titleColumn)
	}
	if titleIdx < 0 {
		for _, want := range titleHeaders {
			for i, h := range header {
				if strings.ToLower(h) == want {
					titleIdx = i
					break
				}
			}
			if titleIdx >= 0 {
				break
			}
		}
	}
	if titleIdx < 0 {
		titleIdx = 0
	}
	var out []archiveRow
	for n, row := range table[1:] {
		payload := map[string]string{}
		empty := true
		for i, h := range header {
			v := ""
			if i < len(row) {
				v = strings.TrimSpace(row[i])
			}
			if v != "" {
				empty = false
			}
			payload[h] = v
		}
		if empty {
			continue
		}
		ar := archiveRow{Row: n + 2, Payload: payload}
		if titleIdx < len(row) {
			ar.Title = strings.TrimSpace(row[titleIdx])
		}
		if ar.Title == "" {
			ar.Title = fmt.Sprintf("Baris %d", n+2)
		}
		if refIdx >= 0 && refIdx < len(row) {
			ar.SourceRef = strings.TrimSpace(row[refIdx])
		}
		out = append(out, ar)
	}
	if len(out) == 0 {
		return nil, "File tidak berisi data"
	}
	if len(out) > maxArchiveRows {
		return nil, fmt.Sprintf("File berisi %d baris; maksimal %d baris per unggahan arsip", len(out), maxArchiveRows)
	}
	return out, ""
}

type archiveRow struct {
	Row       int
	Title     string
	SourceRef string
	Payload   map[string]string
}

// ImportArchive: POST /archive/import.
func (a *App) ImportArchive(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusForbidden, "Hanya untuk pengguna")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+1<<20)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Unggahan gagal dibaca. Ukuran file maksimal 10 MB")
		return
	}
	school := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin {
		school = strings.TrimSpace(r.FormValue("school_id"))
	}
	category := strings.ToLower(strings.TrimSpace(r.FormValue("category")))
	errs := map[string]string{}
	if school == "" {
		errs["school_id"] = "Pilih unit"
	}
	if !reCategory.MatchString(category) {
		errs["category"] = "Kategori 2–40 karakter: huruf kecil, angka, garis bawah (mis. rapor_2019)"
	}
	if len(errs) > 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "Periksa kembali isian arsip", errs)
		return
	}
	run, runMsg := a.runForImport(r.Context(), strings.TrimSpace(r.FormValue("run_id")), school)
	if runMsg != "" {
		common.WriteValidationError(w, http.StatusBadRequest, runMsg, map[string]string{"run_id": runMsg})
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		common.WriteValidationError(w, http.StatusBadRequest, "File arsip wajib diunggah", map[string]string{"file": "File wajib diunggah"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil || len(data) > maxImportBytes {
		common.WriteError(w, http.StatusBadRequest, "Ukuran file maksimal 10 MB")
		return
	}
	table, _, err := userimport.ReadTable(hdr.Filename, data)
	if err != nil {
		common.WriteValidationError(w, http.StatusBadRequest, err.Error(), map[string]string{"file": err.Error()})
		return
	}
	rows, msg := archiveRows(table, r.FormValue("title_column"))
	if msg != "" {
		common.WriteValidationError(w, http.StatusBadRequest, msg, map[string]string{"file": msg})
		return
	}
	if run != nil && run.Mode == "trial" {
		common.WriteJSONMsg(w, http.StatusOK, map[string]any{"category": category, "rows": len(rows), "trial": true}, nil,
			fmt.Sprintf("Impor percobaan: %d baris arsip terbaca untuk kategori %s. Tidak ada data yang disimpan", len(rows), category))
		return
	}
	var runID any
	if run != nil {
		runID = run.ID
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan arsip")
		return
	}
	defer tx.Rollback()
	for _, row := range rows {
		payload, _ := json.Marshal(row.Payload)
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO archive_records (school_id, run_id, category, source_ref, title, payload, file_name, imported_by)
			VALUES ($1,$2,$3,NULLIF($4,''),$5,$6::jsonb,$7,$8)`, school, runID, category, row.SourceRef, row.Title, string(payload), hdr.Filename, nullUUID(claims.UserID)); err != nil {
			common.WriteDBError(w, err, "Gagal menyimpan arsip")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan arsip")
		return
	}
	a.RecordSecurityAudit(&school, "archive_imported", category, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"file": hdr.Filename, "rows": len(rows)}, "", r.Header.Get("X-Correlation-ID"))
	common.WriteJSONMsg(w, http.StatusCreated, map[string]any{"category": category, "rows": len(rows)}, nil,
		fmt.Sprintf("%d baris arsip kategori %s tersimpan (hanya-baca)", len(rows), category))
}

func archiveScope(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !requireRole(w, r, archiveViewRoles...) {
		return "", false
	}
	if common.FromCtx(r) == nil {
		common.WriteError(w, http.StatusForbidden, "Hanya untuk pengguna")
		return "", false
	}
	scope, _ := resolveSchoolScope(r)
	return scope, true
}

// ListArchiveCategories: GET /archive/categories.
func (a *App) ListArchiveCategories(w http.ResponseWriter, r *http.Request) {
	scope, ok := archiveScope(w, r)
	if !ok {
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT category, count(*), max(imported_at) FROM archive_records
		WHERE ($1='' OR school_id::text=$1) GROUP BY category ORDER BY category`, scope)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat kategori arsip")
		return
	}
	defer rows.Close()
	type cat struct {
		Category     string    `json:"category"`
		Count        int       `json:"count"`
		LastImported time.Time `json:"last_imported_at"`
	}
	out := []cat{}
	for rows.Next() {
		var c cat
		if err := rows.Scan(&c.Category, &c.Count, &c.LastImported); err != nil {
			common.WriteDBError(w, err, "Gagal memuat kategori arsip")
			return
		}
		out = append(out, c)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

const archiveCols = `id, school_id, run_id::text, category, source_ref, title, payload, file_name, imported_at`

func scanArchive(row interface{ Scan(...any) error }) (archiveRecord, error) {
	var a archiveRecord
	var payload []byte
	err := row.Scan(&a.ID, &a.SchoolID, &a.RunID, &a.Category, &a.SourceRef, &a.Title, &payload, &a.FileName, &a.ImportedAt)
	a.Payload = payload
	return a, err
}

// SearchArchive: GET /archive?category=&q=.
func (a *App) SearchArchive(w http.ResponseWriter, r *http.Request) {
	scope, ok := archiveScope(w, r)
	if !ok {
		return
	}
	page, per := common.ParsePage(r)
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) > 100 {
		common.WriteValidationError(w, http.StatusBadRequest, "Kata kunci maksimal 100 karakter", map[string]string{"q": "Maksimal 100 karakter"})
		return
	}
	// Full-text match on words, plus a substring match on the title so
	// partial names and numbers are found too.
	where := ` WHERE ($1='' OR school_id::text=$1) AND ($2='' OR category=$2)
		AND ($3='' OR search @@ plainto_tsquery('simple', $3) OR title ILIKE '%' || $3 || '%')`
	var total int
	if err := a.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM archive_records`+where, scope, category, q).Scan(&total); err != nil {
		common.WriteDBError(w, err, "Gagal mencari arsip")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT `+archiveCols+` FROM archive_records`+where+`
		ORDER BY category, title, id LIMIT $4 OFFSET $5`, scope, category, q, per, (page-1)*per)
	if err != nil {
		common.WriteDBError(w, err, "Gagal mencari arsip")
		return
	}
	defer rows.Close()
	out := []archiveRecord{}
	for rows.Next() {
		rec, err := scanArchive(rows)
		if err != nil {
			common.WriteDBError(w, err, "Gagal mencari arsip")
			return
		}
		out = append(out, rec)
	}
	common.WriteJSON(w, http.StatusOK, out, &common.Meta{Page: page, PerPage: per, Total: total})
}

// GetArchiveRecord: GET /archive/{id}.
func (a *App) GetArchiveRecord(w http.ResponseWriter, r *http.Request) {
	scope, ok := archiveScope(w, r)
	if !ok {
		return
	}
	rec, err := scanArchive(a.DB.QueryRowContext(r.Context(), `SELECT `+archiveCols+` FROM archive_records
		WHERE id::text=$1 AND ($2='' OR school_id::text=$2)`, chi.URLParam(r, "id"), scope))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "Arsip tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat arsip")
		return
	}
	common.WriteJSON(w, http.StatusOK, rec, nil)
}
