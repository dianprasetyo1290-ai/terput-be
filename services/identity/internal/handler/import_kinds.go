package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/lib/pq"
	"github.com/xuri/excelize/v2"

	"skci/identity/internal/common"
	"skci/identity/internal/userimport"
)

// Import of parents (with their relation to students) and of class
// membership from the old system (QA-IDN BUG-021), on the same endpoint as
// accounts: POST /users/import with kind=orang_tua|kelas. Students are found
// by "ID Sumber Siswa" (source_id_map, filled by earlier imports, BUG-077)
// or by NISN. Inside a trial migration run (BUG-078) nothing is stored.

type batchMeta struct {
	SchoolID, Kind, FileName, Format, Actor string
	Total, Created, Skipped                 int
	Run                                     *migrationRun
	Counts                                  map[string]int
	TrialRefs                               []string
}

func (m batchMeta) isTrial() bool { return m.Run != nil && m.Run.Mode == "trial" }

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func insertBatch(ctx context.Context, q rowQuerier, m batchMeta) (string, error) {
	counts, _ := json.Marshal(m.Counts)
	if m.Counts == nil {
		counts = []byte("{}")
	}
	var runID any
	if m.Run != nil {
		runID = m.Run.ID
	}
	var id string
	err := q.QueryRowContext(ctx, `INSERT INTO import_batches (school_id, kind, file_name, file_format, total_rows, created_rows, skipped_rows,
			created_by, run_id, trial, category_counts, trial_refs)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12::text[]) RETURNING id`,
		m.SchoolID, m.Kind, m.FileName, m.Format, m.Total, m.Created, m.Skipped, nullUUID(m.Actor), runID, m.isTrial(),
		string(counts), stringArray(m.TrialRefs)).Scan(&id)
	return id, err
}

func mapSourceID(ctx context.Context, tx *sql.Tx, schoolID, entity, ref, targetID, batchID string) error {
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO source_id_map (school_id, entity_type, source_ref, target_id, batch_id)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (school_id, entity_type, source_ref) DO NOTHING`, schoolID, entity, ref, targetID, batchID)
	return err
}

type kindImport struct {
	kind, schoolID, fileName, format string
	mapping                          map[string]string
	table                            [][]string
	run                              *migrationRun
	actor, actorRole                 string
}

type kindPreview struct {
	Kind    string                `json:"kind"`
	Total   int                   `json:"total"`
	Valid   int                   `json:"valid"`
	Invalid int                   `json:"invalid"`
	Format  string                `json:"format"`
	Rows    any                   `json:"rows"`
	Errors  []userimport.RowError `json:"errors"`
	DryRun  bool                  `json:"dry_run"`
	Trial   bool                  `json:"trial"`
	BatchID string                `json:"batch_id,omitempty"`
	Created int                   `json:"created"`
	Skipped int                   `json:"skipped"`
	Counts  map[string]int        `json:"category_counts,omitempty"`
}

type resolvedStudent struct {
	ID, Name, Email string
}

// studentResolver finds students of a unit by source id or NISN; in a trial
// run, students that a trial import of the same run would have created count
// as found (they have no id yet).
type studentResolver struct {
	byRef, byNISN map[string]resolvedStudent
	trialRefs     map[string]bool
}

func (a *App) newStudentResolver(ctx context.Context, schoolID string, refs, nisns []string, run *migrationRun) (*studentResolver, error) {
	sr := &studentResolver{byRef: map[string]resolvedStudent{}, byNISN: map[string]resolvedStudent{}, trialRefs: map[string]bool{}}
	if len(refs) > 0 {
		rows, err := a.DB.QueryContext(ctx, `SELECT m.source_ref, u.id::text, u.name, u.email FROM source_id_map m
			JOIN users u ON u.id::text=m.target_id AND u.deleted_at IS NULL
			JOIN user_school_roles x ON x.user_id=u.id AND x.school_id=m.school_id AND x.role='siswa'
			WHERE m.school_id::text=$1 AND m.entity_type='siswa' AND m.source_ref = ANY($2::text[])`, schoolID, stringArray(refs))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var ref string
			var s resolvedStudent
			if err := rows.Scan(&ref, &s.ID, &s.Name, &s.Email); err != nil {
				rows.Close()
				return nil, err
			}
			sr.byRef[ref] = s
		}
		rows.Close()
	}
	if len(nisns) > 0 {
		rows, err := a.DB.QueryContext(ctx, `SELECT u.nisn, u.id::text, u.name, u.email FROM users u
			JOIN user_school_roles x ON x.user_id=u.id AND x.school_id::text=$1 AND x.role='siswa'
			WHERE u.deleted_at IS NULL AND u.nisn = ANY($2::text[])`, schoolID, stringArray(nisns))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var nisn string
			var s resolvedStudent
			if err := rows.Scan(&nisn, &s.ID, &s.Name, &s.Email); err != nil {
				rows.Close()
				return nil, err
			}
			sr.byNISN[nisn] = s
		}
		rows.Close()
	}
	if run != nil && run.Mode == "trial" {
		var keys []string
		if err := a.DB.QueryRowContext(ctx, `SELECT COALESCE(array_agg(DISTINCT k), '{}') FROM import_batches b, unnest(b.trial_refs) k
			WHERE b.run_id=$1 AND b.trial`, run.ID).Scan(pq.Array(&keys)); err != nil {
			return nil, err
		}
		for _, k := range keys {
			sr.trialRefs[k] = true
		}
	}
	return sr, nil
}

// resolve returns the student, whether it exists only in this trial run,
// and a row issue when it cannot be found.
func (sr *studentResolver) resolve(ref, nisn string) (resolvedStudent, bool, string) {
	if ref != "" {
		if s, ok := sr.byRef[ref]; ok {
			return s, false, ""
		}
	}
	if nisn != "" {
		if s, ok := sr.byNISN[nisn]; ok {
			return s, false, ""
		}
	}
	if ref != "" && sr.trialRefs["ref:"+ref] {
		return resolvedStudent{}, true, ""
	}
	switch {
	case ref != "" && nisn != "":
		return resolvedStudent{}, false, fmt.Sprintf("Siswa dengan ID Sumber %q / NISN %s tidak ditemukan di unit ini. Impor siswa terlebih dahulu", ref, nisn)
	case ref != "":
		return resolvedStudent{}, false, fmt.Sprintf("Siswa dengan ID Sumber %q tidak ditemukan di unit ini. Impor siswa terlebih dahulu", ref)
	}
	return resolvedStudent{}, false, fmt.Sprintf("Siswa dengan NISN %s tidak ditemukan di unit ini", nisn)
}

func addRowError(errs []userimport.RowError, row int, issue string) []userimport.RowError {
	for i := range errs {
		if errs[i].Row == row {
			errs[i].Issue += "; " + issue
			return errs
		}
	}
	return append(errs, userimport.RowError{Row: row, Issue: issue})
}

func (a *App) importKind(w http.ResponseWriter, r *http.Request, in kindImport) {
	cols, missing := userimport.MapKindColumns(in.kind, in.table[0], in.mapping)
	if len(missing) > 0 {
		msg := "Kolom wajib tidak ditemukan: " + strings.Join(missing, ", ") + ". Sesuaikan judul kolom dengan template atau isi pemetaan kolom"
		common.WriteJSONMsg(w, http.StatusBadRequest, map[string]any{"missing_columns": missing, "headers": in.table[0]}, nil, msg)
		return
	}
	dataRows := in.table[1:]
	if n := userimport.DataRowCount(dataRows); n == 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "File tidak berisi data", map[string]string{"file": "Tidak ada baris data"})
		return
	} else if n > userimport.MaxRows {
		msg := fmt.Sprintf("File berisi %d baris data; maksimal %d baris per impor. Pecah file menjadi beberapa bagian", n, userimport.MaxRows)
		common.WriteValidationError(w, http.StatusBadRequest, msg, map[string]string{"file": msg})
		return
	}
	if in.kind == userimport.KindOrangTua {
		a.importGuardians(w, r, in, cols, dataRows)
		return
	}
	a.importClasses(w, r, in, cols, dataRows)
}

// answerPreviewOrRefuse handles dry_run and invalid rows the same way as
// account imports. It returns false when the response was written.
func answerPreviewOrRefuse(w http.ResponseWriter, r *http.Request, out *kindPreview, valid int) bool {
	if out.Errors == nil {
		out.Errors = []userimport.RowError{}
	}
	sort.Slice(out.Errors, func(i, j int) bool { return out.Errors[i].Row < out.Errors[j].Row })
	out.Valid, out.Invalid = valid, len(out.Errors)
	out.Total = out.Valid + out.Invalid
	out.DryRun = formBool(r, "dry_run")
	if out.DryRun {
		common.WriteJSONMsg(w, http.StatusOK, out, nil, fmt.Sprintf("Pratinjau: %d baris valid, %d baris bermasalah. Belum ada data yang disimpan", out.Valid, out.Invalid))
		return false
	}
	if out.Invalid > 0 && !formBool(r, "skip_invalid") {
		common.WriteJSONMsg(w, http.StatusUnprocessableEntity, out, nil,
			fmt.Sprintf("%d baris bermasalah; tidak ada data yang disimpan. Perbaiki file atau lanjutkan hanya baris valid", out.Invalid))
		return false
	}
	if out.Valid == 0 {
		common.WriteJSONMsg(w, http.StatusUnprocessableEntity, out, nil, "Tidak ada baris valid untuk disimpan")
		return false
	}
	return true
}

type guardianAccount struct {
	ID, Name   string
	Deleted    bool
	HasRole    bool
	Conflicted string
}

func (a *App) importGuardians(w http.ResponseWriter, r *http.Request, in kindImport, cols map[string]int, dataRows [][]string) {
	ctx := r.Context()
	rows, rowErrs := userimport.ValidateGuardianRows(dataRows, cols)
	var refs, nisns, emails []string
	for _, g := range rows {
		if g.StudentRef != "" {
			refs = append(refs, g.StudentRef)
		}
		if g.StudentNISN != "" {
			nisns = append(nisns, g.StudentNISN)
		}
		emails = append(emails, strings.ToLower(g.Email))
	}
	sr, err := a.newStudentResolver(ctx, in.schoolID, refs, nisns, in.run)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa data impor")
		return
	}
	accounts, err := a.guardianAccounts(ctx, in.schoolID, emails)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa data impor")
		return
	}
	var valid []userimport.GuardianRow
	for _, g := range rows {
		var issues []string
		stu, trialOnly, issue := sr.resolve(g.StudentRef, g.StudentNISN)
		if issue != "" {
			issues = append(issues, issue)
		}
		g.StudentUserID, g.StudentName = stu.ID, stu.Name
		if trialOnly {
			g.StudentName = "(siswa dari impor percobaan)"
		}
		if acc, ok := accounts[strings.ToLower(g.Email)]; ok {
			g.ExistingUser = true
			switch {
			case acc.Deleted:
				issues = append(issues, "Email milik akun yang sudah dihapus")
			case acc.Conflicted != "":
				issues = append(issues, acc.Conflicted)
			case stu.ID != "" && acc.ID == stu.ID:
				issues = append(issues, "Orang tua dan siswa tidak boleh akun yang sama")
			}
			if stu.ID != "" && !acc.Deleted {
				var exists bool
				if err := a.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM guardian_links WHERE school_id::text=$1 AND guardian_user_id::text=$2 AND student_user_id::text=$3)`,
					in.schoolID, acc.ID, stu.ID).Scan(&exists); err != nil {
					common.WriteDBError(w, err, "Gagal memeriksa data impor")
					return
				}
				if exists {
					issues = append(issues, "Relasi orang tua–siswa ini sudah terdaftar")
				}
			}
		}
		if len(issues) > 0 {
			rowErrs = addRowError(rowErrs, g.Number, strings.Join(issues, "; "))
			continue
		}
		valid = append(valid, g)
	}
	// A parent with one bad row is rejected entirely, so the account is not
	// created with only some of the children.
	bad := map[string]bool{}
	for _, e := range rowErrs {
		for _, g := range rows {
			if g.Number == e.Row {
				bad[strings.ToLower(g.Email)] = true
			}
		}
	}
	var keep []userimport.GuardianRow
	for _, g := range valid {
		if bad[strings.ToLower(g.Email)] {
			rowErrs = addRowError(rowErrs, g.Number, "Baris lain untuk orang tua yang sama bermasalah")
			continue
		}
		keep = append(keep, g)
	}
	valid = keep
	out := &kindPreview{Kind: in.kind, Format: in.format, Rows: valid, Errors: rowErrs, Trial: in.run != nil && in.run.Mode == "trial"}
	if valid == nil {
		out.Rows = []userimport.GuardianRow{}
	}
	if !answerPreviewOrRefuse(w, r, out, len(valid)) {
		return
	}
	m := batchMeta{SchoolID: in.schoolID, Kind: in.kind, FileName: in.fileName, Format: in.format, Actor: in.actor,
		Total: out.Total, Created: len(valid), Skipped: out.Invalid, Run: in.run}
	batchID, counts, err := a.commitGuardians(ctx, m, valid, accounts)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "Sebagian data baru saja diubah oleh proses lain. Ulangi pratinjau")
			return
		}
		common.WriteDBError(w, err, "Gagal menyimpan data impor")
		return
	}
	out.BatchID, out.Created, out.Skipped, out.Counts = batchID, len(valid), out.Invalid, counts
	if out.Trial {
		common.WriteJSONMsg(w, http.StatusOK, out, nil, fmt.Sprintf("Impor percobaan: %d relasi orang tua akan dibuat (%d akun baru). Tidak ada data yang disimpan",
			counts["relasi_ortu"], counts["akun_ortu_baru"]))
		return
	}
	sid := in.schoolID
	a.RecordSecurityAudit(&sid, "guardians_imported", batchID, &in.actor, &in.actorRole, nil,
		map[string]any{"file": in.fileName, "counts": counts}, "bulk import", r.Header.Get("X-Correlation-ID"))
	common.WriteJSONMsg(w, http.StatusOK, out, nil, fmt.Sprintf("%d relasi orang tua tersimpan (%d akun baru), %d baris dilewati",
		counts["relasi_ortu"], counts["akun_ortu_baru"], out.Skipped))
}

// guardianAccounts looks up the accounts behind the parent emails.
func (a *App) guardianAccounts(ctx context.Context, schoolID string, emails []string) (map[string]guardianAccount, error) {
	out := map[string]guardianAccount{}
	if len(emails) == 0 {
		return out, nil
	}
	rows, err := a.DB.QueryContext(ctx, `SELECT lower(u.email), u.id::text, u.name, u.deleted_at IS NOT NULL,
			EXISTS (SELECT 1 FROM user_school_roles x WHERE x.user_id=u.id AND x.school_id::text=$1 AND x.role='orang_tua')
		FROM users u WHERE lower(u.email) = ANY($2::text[])`, schoolID, stringArray(emails))
	if err != nil {
		return nil, err
	}
	var list []guardianAccount
	var keys []string
	for rows.Next() {
		var k string
		var g guardianAccount
		if err := rows.Scan(&k, &g.ID, &g.Name, &g.Deleted, &g.HasRole); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, g)
		keys = append(keys, k)
	}
	rows.Close()
	for i, g := range list {
		if !g.Deleted && !g.HasRole {
			conflicts, err := a.findRoleConflicts(ctx, g.ID, common.RoleOrangTua)
			if err != nil {
				return nil, err
			}
			if len(conflicts) > 0 {
				names := make([]string, 0, len(conflicts))
				for _, c := range conflicts {
					other := c.RoleA
					if other == common.RoleOrangTua {
						other = c.RoleB
					}
					names = append(names, other)
				}
				g.Conflicted = "Akun ini memegang peran yang tidak boleh dirangkap dengan orang tua (" + strings.Join(names, ", ") + ")"
			}
		}
		out[keys[i]] = g
	}
	return out, nil
}

func (a *App) commitGuardians(ctx context.Context, m batchMeta, rows []userimport.GuardianRow, accounts map[string]guardianAccount) (string, map[string]int, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()
	batchID, err := insertBatch(ctx, tx, m)
	if err != nil {
		return "", nil, err
	}
	counts := map[string]int{}
	ids := map[string]string{}
	for _, g := range rows {
		key := strings.ToLower(g.Email)
		gid, seen := ids[key]
		if !seen {
			created := false
			if acc, ok := accounts[key]; ok {
				gid = acc.ID
			} else {
				if gid, _, err = createImportedUser(tx, g.Name, g.Email, g.Phone, "", "", "", "", nil); err != nil {
					return "", nil, err
				}
				created = true
				counts["akun_ortu_baru"]++
			}
			res, err := tx.ExecContext(ctx, `INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,'orang_tua')
				ON CONFLICT (user_id, school_id, role) DO UPDATE SET status='active' WHERE user_school_roles.status <> 'active'`, gid, m.SchoolID)
			if err != nil {
				return "", nil, err
			}
			granted, _ := res.RowsAffected()
			if granted > 0 {
				counts["orang_tua"]++
			}
			if created || granted > 0 {
				if _, err := tx.ExecContext(ctx, `INSERT INTO import_batch_items (batch_id, row_number, user_id, role, email, source_ref, entity_type, entity_id, created_account)
					VALUES ($1,$2,$3,'orang_tua',$4,NULLIF($5,''),'user',$7,$6)`, batchID, g.Number, gid, g.Email, g.SourceRef, created, gid); err != nil {
					return "", nil, err
				}
			}
			if err := mapSourceID(ctx, tx, m.SchoolID, common.RoleOrangTua, g.SourceRef, gid, batchID); err != nil {
				return "", nil, err
			}
			ids[key] = gid
		}
		counts["relasi_ortu"]++
		if g.StudentUserID == "" { // trial: the student only exists in the trial run
			continue
		}
		var linkID string
		if err := tx.QueryRowContext(ctx, `INSERT INTO guardian_links (school_id, guardian_user_id, student_user_id, relationship, status, verified_at)
			VALUES ($1,$2,$3,$4,'active',now()) RETURNING id::text`, m.SchoolID, gid, g.StudentUserID, g.Relationship).Scan(&linkID); err != nil {
			return "", nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO import_batch_items (batch_id, row_number, user_id, role, email, source_ref, entity_type, entity_id, created_account)
			VALUES ($1,$2,$3,'orang_tua',$4,NULLIF($5,''),'guardian_link',$6,false)`, batchID, g.Number, gid, g.Email, g.SourceRef, linkID); err != nil {
			return "", nil, err
		}
	}
	m.Counts = counts
	if m.isTrial() {
		if err := tx.Rollback(); err != nil {
			return "", nil, err
		}
		id, err := insertBatch(ctx, a.DB, m)
		return id, counts, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_batches SET category_counts=$2::jsonb WHERE id=$1`, batchID, mustJSON(counts)); err != nil {
		return "", nil, err
	}
	return batchID, counts, tx.Commit()
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// resolveAcademicYear: the form's academic_year_id (of the unit) or the
// unit's current year.
func (a *App) resolveAcademicYear(ctx context.Context, schoolID, requested string) (string, string, error) {
	var id, label string
	var err error
	if requested != "" {
		err = a.DB.QueryRowContext(ctx, `SELECT id::text, label FROM academic_years WHERE id::text=$1 AND school_id::text=$2`, requested, schoolID).Scan(&id, &label)
	} else {
		err = a.DB.QueryRowContext(ctx, `SELECT y.id::text, y.label FROM academic_years y
			WHERE y.school_id::text=$1 AND (y.is_current OR y.id=(SELECT current_academic_year_id FROM schools WHERE id::text=$1))
			ORDER BY y.is_current DESC LIMIT 1`, schoolID).Scan(&id, &label)
	}
	return id, label, err
}

func (a *App) importClasses(w http.ResponseWriter, r *http.Request, in kindImport, cols map[string]int, dataRows [][]string) {
	ctx := r.Context()
	yearID, yearLabel, err := a.resolveAcademicYear(ctx, in.schoolID, strings.TrimSpace(r.FormValue("academic_year_id")))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteValidationError(w, http.StatusBadRequest, "Tahun ajaran tidak ditemukan. Pilih tahun ajaran atau tetapkan tahun ajaran aktif unit",
			map[string]string{"academic_year_id": "Wajib dipilih"})
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa tahun ajaran")
		return
	}
	rows, rowErrs := userimport.ValidateClassRows(dataRows, cols)
	var refs, nisns, walis []string
	for _, c := range rows {
		if c.StudentRef != "" {
			refs = append(refs, c.StudentRef)
		}
		if c.StudentNISN != "" {
			nisns = append(nisns, c.StudentNISN)
		}
		if c.HomeroomEmail != "" {
			walis = append(walis, strings.ToLower(c.HomeroomEmail))
		}
	}
	sr, err := a.newStudentResolver(ctx, in.schoolID, refs, nisns, in.run)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa data impor")
		return
	}
	teachers := map[string]string{}
	if len(walis) > 0 {
		q, err := a.DB.QueryContext(ctx, `SELECT lower(u.email), u.id::text FROM users u
			JOIN user_school_roles x ON x.user_id=u.id AND x.school_id::text=$1 AND x.role='guru' AND x.status='active'
			WHERE u.deleted_at IS NULL AND lower(u.email) = ANY($2::text[])`, in.schoolID, stringArray(walis))
		if err != nil {
			common.WriteDBError(w, err, "Gagal memeriksa data impor")
			return
		}
		for q.Next() {
			var e, id string
			if err := q.Scan(&e, &id); err != nil {
				q.Close()
				common.WriteDBError(w, err, "Gagal memeriksa data impor")
				return
			}
			teachers[e] = id
		}
		q.Close()
	}
	var valid []userimport.ClassRow
	for _, c := range rows {
		var issues []string
		stu, trialOnly, issue := sr.resolve(c.StudentRef, c.StudentNISN)
		if issue != "" {
			issues = append(issues, issue)
		}
		c.StudentUserID, c.StudentName = stu.ID, stu.Name
		if trialOnly {
			c.StudentName = "(siswa dari impor percobaan)"
		}
		if c.HomeroomEmail != "" {
			if id, ok := teachers[strings.ToLower(c.HomeroomEmail)]; ok {
				c.HomeroomUserID = id
			} else {
				issues = append(issues, "Wali kelas "+c.HomeroomEmail+" bukan guru aktif di unit ini")
			}
		}
		if len(issues) > 0 {
			rowErrs = addRowError(rowErrs, c.Number, strings.Join(issues, "; "))
			continue
		}
		valid = append(valid, c)
	}
	classes := map[string]bool{}
	for _, c := range valid {
		classes[strings.ToLower(c.ClassName)] = true
	}
	counts := map[string]int{"kelas": len(classes), "anggota_kelas": len(valid)}
	out := &kindPreview{Kind: in.kind, Format: in.format, Rows: valid, Errors: rowErrs, Trial: in.run != nil && in.run.Mode == "trial", Counts: counts}
	if valid == nil {
		out.Rows = []userimport.ClassRow{}
	}
	if !answerPreviewOrRefuse(w, r, out, len(valid)) {
		return
	}
	m := batchMeta{SchoolID: in.schoolID, Kind: in.kind, FileName: in.fileName, Format: in.format, Actor: in.actor,
		Total: out.Total, Created: len(valid), Skipped: out.Invalid, Run: in.run, Counts: counts}
	out.Created, out.Skipped = len(valid), out.Invalid
	if m.isTrial() {
		// Classes live in the academic service, which has no dry run: a trial
		// only validates and records the counts.
		if out.BatchID, err = insertBatch(ctx, a.DB, m); err != nil {
			common.WriteDBError(w, err, "Gagal mencatat impor percobaan")
			return
		}
		common.WriteJSONMsg(w, http.StatusOK, out, nil, fmt.Sprintf("Impor percobaan: %d siswa akan ditempatkan di %d kelas (%s). Tidak ada data yang disimpan",
			len(valid), len(classes), yearLabel))
		return
	}
	if a.Academic == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "Layanan akademik belum dikonfigurasi; impor kelas tidak dapat dijalankan")
		return
	}
	classIDs := map[string]string{}
	enrolled := 0
	for _, c := range valid {
		key := strings.ToLower(c.ClassName)
		cid, ok := classIDs[key]
		if !ok {
			var grade, wali *string
			if c.GradeLevel != "" {
				g := c.GradeLevel
				grade = &g
			}
			if c.HomeroomUserID != "" {
				h := c.HomeroomUserID
				wali = &h
			}
			if cid, err = a.Academic.FindOrCreateClassroom(in.schoolID, yearID, c.ClassName, grade, wali); err != nil {
				common.CaptureError(err)
				common.WriteJSONMsg(w, http.StatusBadGateway, map[string]any{"enrolled": enrolled, "failed_row": c.Number}, nil,
					fmt.Sprintf("Layanan akademik tidak dapat membuat kelas %q (baris %d). %d siswa sudah ditempatkan; ulangi impor file yang sama dengan aman — siswa yang sudah masuk tidak digandakan", c.ClassName, c.Number, enrolled))
				return
			}
			classIDs[key] = cid
		}
		if err := a.Academic.EnrollStudent(in.schoolID, cid, c.StudentUserID); err != nil {
			common.CaptureError(err)
			common.WriteJSONMsg(w, http.StatusBadGateway, map[string]any{"enrolled": enrolled, "failed_row": c.Number}, nil,
				fmt.Sprintf("Layanan akademik gagal menempatkan siswa baris %d. %d siswa sudah ditempatkan; ulangi impor file yang sama dengan aman — siswa yang sudah masuk tidak digandakan", c.Number, enrolled))
			return
		}
		enrolled++
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		common.WriteDBError(w, err, "Kelas tersimpan tetapi riwayat impor gagal dicatat")
		return
	}
	defer tx.Rollback()
	if out.BatchID, err = insertBatch(ctx, tx, m); err == nil {
		for _, c := range valid {
			if _, err = tx.ExecContext(ctx, `INSERT INTO import_batch_items (batch_id, row_number, user_id, role, email, source_ref, entity_type, entity_id, created_account)
				SELECT $1,$2,u.id,'siswa',u.email,NULLIF($4,''),'enrollment',$5,false FROM users u WHERE u.id::text=$3`,
				out.BatchID, c.Number, c.StudentUserID, c.StudentRef, classIDs[strings.ToLower(c.ClassName)]); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteDBError(w, err, "Kelas tersimpan tetapi riwayat impor gagal dicatat")
		return
	}
	sid := in.schoolID
	a.RecordSecurityAudit(&sid, "classes_imported", out.BatchID, &in.actor, &in.actorRole, nil,
		map[string]any{"file": in.fileName, "counts": counts, "academic_year": yearLabel}, "bulk import", r.Header.Get("X-Correlation-ID"))
	common.WriteJSONMsg(w, http.StatusOK, out, nil, fmt.Sprintf("%d siswa ditempatkan di %d kelas (%s), %d baris dilewati",
		enrolled, len(classes), yearLabel, out.Skipped))
}

// writeKindTemplate answers GET /users/import-template?kind=orang_tua|kelas.
func writeKindTemplate(w http.ResponseWriter, kind string) {
	header := userimport.KindTemplates[kind]
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Impor Orang Tua"
	example := []string{"Siti Aminah", "siti@example.com", "08123456789", "OT-001", "S-2024-001", "", "ibu"}
	if kind == userimport.KindKelas {
		sheet = "Impor Kelas"
		example = []string{"X-A", "10", "wali@example.com", "S-2024-001", ""}
	}
	_ = f.SetSheetName("Sheet1", sheet)
	_ = f.SetSheetRow(sheet, "A1", &header)
	_ = f.SetSheetRow(sheet, "A2", &example)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat template")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=template_impor_%s.xlsx", kind))
	_, _ = w.Write(buf.Bytes())
}
