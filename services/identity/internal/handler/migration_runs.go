package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/rules"
)

// Migrasi data dari sistem lama (QA-IDN BUG-019/078).
//
// A migration run groups the imports of one cut-over attempt of a unit.
//   - mode=trial: every import attached to it runs its full commit inside a
//     transaction that is rolled back, so nothing reaches the live data;
//     only the batch and its per-category counts are recorded (BUG-078).
//   - mode=production: may only be opened from a signed-off trial run.
//   - Control totals per category (source = old system, target = computed
//     from SEKOCI or entered) must reconcile: sign-off is refused (409) while
//     any difference has no explanation, and the signer must be a different
//     person than the one who opened the run (BUG-019).
//
//	GET  /migration-runs                     POST /migration-runs {title, mode, trial_run_id?, notes?, school_id?}
//	GET  /migration-runs/{id}                PUT  /migration-runs/{id}/totals {items:[...]}
//	POST /migration-runs/{id}/recompute      POST /migration-runs/{id}/sign-off {note}
//	POST /migration-runs/{id}/discard {reason}

type migrationRun struct {
	ID            string           `json:"id"`
	SchoolID      string           `json:"school_id"`
	SchoolName    *string          `json:"school_name"`
	Title         string           `json:"title"`
	Mode          string           `json:"mode"`
	Status        string           `json:"status"`
	TrialRunID    *string          `json:"trial_run_id"`
	Notes         *string          `json:"notes"`
	CreatedBy     *string          `json:"created_by"`
	CreatedByName *string          `json:"created_by_name"`
	CreatedAt     time.Time        `json:"created_at"`
	SignedOffBy   *string          `json:"signed_off_by"`
	SignedOffName *string          `json:"signed_off_by_name"`
	SignedOffAt   *time.Time       `json:"signed_off_at"`
	SignOffNote   *string          `json:"sign_off_note"`
	DiscardedAt   *time.Time       `json:"discarded_at"`
	DiscardReason *string          `json:"discard_reason"`
	Totals        []migrationTotal `json:"totals,omitempty"`
	Batches       []importBatch    `json:"batches,omitempty"`
	Unexplained   int              `json:"unexplained_differences"`
}

type migrationTotal struct {
	ID           string    `json:"id"`
	Category     string    `json:"category"`
	Metric       string    `json:"metric"`
	Label        string    `json:"label"`
	SourceValue  *float64  `json:"source_value"`
	TargetValue  *float64  `json:"target_value"`
	TargetSource string    `json:"target_source"`
	Diff         float64   `json:"diff"`
	Explanation  *string   `json:"explanation"`
	UpdatedAt    time.Time `json:"updated_at"`
}

const runCols = `m.id, m.school_id, s.name, m.title, m.mode, m.status, m.trial_run_id::text, m.notes,
	m.created_by::text, cu.name, m.created_at, m.signed_off_by::text, su.name, m.signed_off_at, m.sign_off_note,
	m.discarded_at, m.discard_reason`

const runFrom = ` FROM migration_runs m
	LEFT JOIN schools s ON s.id=m.school_id
	LEFT JOIN users cu ON cu.id=m.created_by
	LEFT JOIN users su ON su.id=m.signed_off_by`

func scanRun(row interface{ Scan(...any) error }) (migrationRun, error) {
	var m migrationRun
	err := row.Scan(&m.ID, &m.SchoolID, &m.SchoolName, &m.Title, &m.Mode, &m.Status, &m.TrialRunID, &m.Notes,
		&m.CreatedBy, &m.CreatedByName, &m.CreatedAt, &m.SignedOffBy, &m.SignedOffName, &m.SignedOffAt, &m.SignOffNote,
		&m.DiscardedAt, &m.DiscardReason)
	return m, err
}

// Categories identity can count itself (target value "auto").
var autoCategories = map[string]string{
	"siswa":         "Siswa aktif",
	"guru":          "Guru",
	"orang_tua":     "Akun orang tua",
	"relasi_ortu":   "Relasi orang tua–siswa",
	"kelas":         "Kelas (dari impor)",
	"anggota_kelas": "Anggota kelas (dari impor)",
}

var reCategory = regexp.MustCompile(`^[a-z0-9_]{2,40}$`)

func (a *App) getRun(ctx context.Context, id string) (migrationRun, error) {
	return scanRun(a.DB.QueryRowContext(ctx, `SELECT `+runCols+runFrom+` WHERE m.id::text=$1`, id))
}

// loadRunForHandler loads a run in the caller's scope (404 outside it).
func (a *App) loadRunForHandler(w http.ResponseWriter, r *http.Request) (migrationRun, bool) {
	if !requireRole(w, r, common.RoleAdmin) {
		return migrationRun{}, false
	}
	claims := common.FromCtx(r)
	m, err := a.getRun(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && claims != nil && claims.ActiveRole != common.RoleSuperadmin && m.SchoolID != claims.SchoolID) {
		common.WriteError(w, http.StatusNotFound, "Migrasi tidak ditemukan")
		return m, false
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat migrasi")
		return m, false
	}
	return m, true
}

func (a *App) runTotals(ctx context.Context, runID string) ([]migrationTotal, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT id, category, metric, label, source_value::float8, target_value::float8,
			target_source, diff::float8, explanation, updated_at
		FROM migration_control_totals WHERE run_id=$1 ORDER BY category, metric`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []migrationTotal{}
	for rows.Next() {
		var t migrationTotal
		if err := rows.Scan(&t.ID, &t.Category, &t.Metric, &t.Label, &t.SourceValue, &t.TargetValue,
			&t.TargetSource, &t.Diff, &t.Explanation, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// unexplainedDifferences counts rows that block sign-off.
func unexplainedDifferences(totals []migrationTotal) int {
	n := 0
	for _, t := range totals {
		if t.SourceValue == nil || t.TargetValue == nil {
			n++
			continue
		}
		if t.Diff != 0 && (t.Explanation == nil || len([]rune(strings.TrimSpace(*t.Explanation))) < 10) {
			n++
		}
	}
	return n
}

// ListMigrationRuns: GET /migration-runs.
func (a *App) ListMigrationRuns(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	scope, _ := resolveSchoolScope(r)
	rows, err := a.DB.QueryContext(r.Context(), `SELECT `+runCols+runFrom+`
		WHERE ($1='' OR m.school_id::text=$1) ORDER BY m.created_at DESC LIMIT 200`, scope)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat daftar migrasi")
		return
	}
	defer rows.Close()
	out := []migrationRun{}
	for rows.Next() {
		m, err := scanRun(rows)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memuat daftar migrasi")
			return
		}
		out = append(out, m)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// CreateMigrationRun: POST /migration-runs.
func (a *App) CreateMigrationRun(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusForbidden, "Hanya untuk pengguna")
		return
	}
	var in struct {
		Title      string `json:"title"`
		Mode       string `json:"mode"`
		TrialRunID string `json:"trial_run_id"`
		Notes      string `json:"notes"`
		SchoolID   string `json:"school_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Isian tidak valid")
		return
	}
	school := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin {
		school = strings.TrimSpace(in.SchoolID)
	}
	errs := map[string]string{}
	in.Title = strings.TrimSpace(in.Title)
	if l := len([]rune(in.Title)); l < 3 || l > 150 {
		errs["title"] = "Judul 3–150 karakter"
	}
	if !rules.IsUUID(school) {
		errs["school_id"] = "Pilih unit"
	}
	switch in.Mode {
	case "trial":
		in.TrialRunID = ""
	case "production":
		if !rules.IsUUID(in.TrialRunID) {
			errs["trial_run_id"] = "Migrasi produksi harus merujuk migrasi percobaan yang sudah disahkan"
		}
	default:
		errs["mode"] = "Pilih mode: percobaan atau produksi"
	}
	if len(errs) > 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "Periksa kembali isian migrasi", errs)
		return
	}
	if in.Mode == "production" {
		trial, err := a.getRun(r.Context(), in.TrialRunID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && trial.SchoolID != school) {
			common.WriteValidationError(w, http.StatusBadRequest, "Migrasi percobaan tidak ditemukan di unit ini", map[string]string{"trial_run_id": "Tidak ditemukan"})
			return
		} else if err != nil {
			common.WriteDBError(w, err, "Gagal memeriksa migrasi percobaan")
			return
		}
		if trial.Mode != "trial" || trial.Status != "signed_off" {
			common.WriteJSONMsg(w, http.StatusConflict, map[string]any{"trial_status": trial.Status}, nil,
				"Migrasi produksi hanya dapat dibuka dari migrasi percobaan yang sudah disahkan (control total cocok)")
			return
		}
	}
	var id string
	err := a.DB.QueryRowContext(r.Context(), `INSERT INTO migration_runs (school_id, title, mode, trial_run_id, notes, created_by)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6) RETURNING id`,
		school, in.Title, in.Mode, nullIfEmpty(in.TrialRunID), strings.TrimSpace(in.Notes), nullUUID(claims.UserID)).Scan(&id)
	if err != nil {
		common.WriteDBError(w, err, "Gagal membuat migrasi")
		return
	}
	a.RecordSecurityAudit(&school, "migration_run_created", id, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"mode": in.Mode, "title": in.Title}, "", r.Header.Get("X-Correlation-ID"))
	m, err := a.getRun(r.Context(), id)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat migrasi")
		return
	}
	msg := "Migrasi percobaan dibuat. Impor yang dikaitkan tidak menyimpan data"
	if in.Mode == "production" {
		msg = "Migrasi produksi dibuat"
	}
	common.WriteJSONMsg(w, http.StatusCreated, m, nil, msg)
}

// GetMigrationRun: GET /migration-runs/{id} with totals and batches.
func (a *App) GetMigrationRun(w http.ResponseWriter, r *http.Request) {
	m, ok := a.loadRunForHandler(w, r)
	if !ok {
		return
	}
	var err error
	if m.Totals, err = a.runTotals(r.Context(), m.ID); err != nil {
		common.WriteDBError(w, err, "Gagal memuat control total")
		return
	}
	m.Unexplained = unexplainedDifferences(m.Totals)
	if m.Batches, err = a.listBatches(r.Context(), "", m.ID); err != nil {
		common.WriteDBError(w, err, "Gagal memuat impor")
		return
	}
	common.WriteJSON(w, http.StatusOK, m, map[string]any{"auto_categories": autoCategories})
}

type totalInput struct {
	Category    string   `json:"category"`
	Metric      string   `json:"metric"`
	Label       string   `json:"label"`
	SourceValue *float64 `json:"source_value"`
	TargetValue *float64 `json:"target_value"`
	Explanation *string  `json:"explanation"`
	Remove      bool     `json:"remove"`
}

func validateTotalInput(t *totalInput) string {
	t.Category = strings.ToLower(strings.TrimSpace(t.Category))
	if t.Metric == "" {
		t.Metric = "count"
	}
	if !reCategory.MatchString(t.Category) {
		return "Kategori 2–40 karakter: huruf kecil, angka, garis bawah"
	}
	if t.Metric != "count" && t.Metric != "sum_amount" {
		return "Metrik harus jumlah data (count) atau total nominal (sum_amount)"
	}
	if t.Remove {
		return ""
	}
	t.Label = strings.TrimSpace(t.Label)
	if t.Label == "" {
		if l, ok := autoCategories[t.Category]; ok {
			t.Label = l
		} else {
			return "Label kategori wajib diisi"
		}
	}
	for _, v := range []*float64{t.SourceValue, t.TargetValue} {
		if v != nil && *v < 0 {
			return "Nilai tidak boleh negatif"
		}
	}
	if t.Metric == "count" {
		for _, v := range []*float64{t.SourceValue, t.TargetValue} {
			if v != nil && *v != float64(int64(*v)) {
				return "Jumlah data harus bilangan bulat"
			}
		}
	}
	return ""
}

// UpsertMigrationTotals: PUT /migration-runs/{id}/totals.
func (a *App) UpsertMigrationTotals(w http.ResponseWriter, r *http.Request) {
	m, ok := a.loadRunForHandler(w, r)
	if !ok {
		return
	}
	if m.Status != "open" {
		common.WriteError(w, http.StatusConflict, "Migrasi sudah "+runStatusLabel(m.Status)+"; control total tidak dapat diubah")
		return
	}
	claims := common.FromCtx(r)
	var in struct {
		Items []totalInput `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.Items) == 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "Isi minimal satu baris control total", map[string]string{"items": "Wajib diisi"})
		return
	}
	for i := range in.Items {
		if msg := validateTotalInput(&in.Items[i]); msg != "" {
			common.WriteValidationError(w, http.StatusBadRequest, fmt.Sprintf("Baris %d: %s", i+1, msg), map[string]string{"items": msg})
			return
		}
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan control total")
		return
	}
	defer tx.Rollback()
	actor := ""
	if claims != nil {
		actor = claims.UserID
	}
	for _, t := range in.Items {
		if t.Remove {
			if _, err := tx.ExecContext(r.Context(), `DELETE FROM migration_control_totals WHERE run_id=$1 AND category=$2 AND metric=$3`, m.ID, t.Category, t.Metric); err != nil {
				common.WriteDBError(w, err, "Gagal menyimpan control total")
				return
			}
			continue
		}
		// A target typed by hand becomes "manual"; an omitted target keeps
		// the computed one.
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO migration_control_totals
				(run_id, category, metric, label, source_value, target_value, target_source, explanation, updated_by)
			VALUES ($1,$2,$3,$4,$5,$6, CASE WHEN $6::numeric IS NULL THEN 'auto' ELSE 'manual' END, NULLIF(btrim($7),''), $8)
			ON CONFLICT (run_id, category, metric) DO UPDATE SET
				label=EXCLUDED.label, source_value=EXCLUDED.source_value,
				target_value=COALESCE(EXCLUDED.target_value, migration_control_totals.target_value),
				target_source=CASE WHEN EXCLUDED.target_value IS NULL THEN migration_control_totals.target_source ELSE 'manual' END,
				explanation=EXCLUDED.explanation, updated_by=EXCLUDED.updated_by, updated_at=now()`,
			m.ID, t.Category, t.Metric, t.Label, t.SourceValue, t.TargetValue, deref(t.Explanation), nullUUID(actor)); err != nil {
			common.WriteDBError(w, err, "Gagal menyimpan control total")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan control total")
		return
	}
	if err := a.recomputeTargets(r.Context(), m); err != nil {
		common.WriteDBError(w, err, "Control total tersimpan tetapi nilai SEKOCI gagal dihitung")
		return
	}
	a.writeRunDetail(w, r, m.ID, "Control total disimpan")
}

// recomputeTargets fills target_value for auto rows: identity-owned
// categories from the live data (production) and every category from the
// run's import counts (trial, where nothing is stored).
func (a *App) recomputeTargets(ctx context.Context, m migrationRun) error {
	counts := map[string]float64{}
	rows, err := a.DB.QueryContext(ctx, `SELECT k, sum(v::numeric)::float8 FROM import_batches b, jsonb_each_text(b.category_counts) AS c(k, v)
		WHERE b.run_id=$1 AND b.rolled_back_at IS NULL GROUP BY k`, m.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k string
		var v float64
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return err
		}
		counts[k] = v
	}
	rows.Close()
	if m.Mode == "production" {
		live := map[string]string{
			"siswa":       `SELECT count(*) FROM user_school_roles WHERE school_id=$1 AND role='siswa' AND status='active'`,
			"guru":        `SELECT count(*) FROM user_school_roles WHERE school_id=$1 AND role='guru' AND status='active'`,
			"orang_tua":   `SELECT count(*) FROM user_school_roles WHERE school_id=$1 AND role='orang_tua' AND status='active'`,
			"relasi_ortu": `SELECT count(*) FROM guardian_links WHERE school_id=$1 AND status='active'`,
		}
		for cat, q := range live {
			var n float64
			if err := a.DB.QueryRowContext(ctx, q, m.SchoolID).Scan(&n); err != nil {
				return err
			}
			counts[cat] = n
		}
	}
	// Make sure every counted category has a row to reconcile against.
	for cat := range counts {
		label, ok := autoCategories[cat]
		if !ok { // informative counts (e.g. akun_ortu_baru) are not reconciled
			continue
		}
		if _, err := a.DB.ExecContext(ctx, `INSERT INTO migration_control_totals (run_id, category, metric, label, target_source)
			VALUES ($1,$2,'count',$3,'auto') ON CONFLICT DO NOTHING`, m.ID, cat, label); err != nil {
			return err
		}
	}
	for cat, v := range counts {
		if _, err := a.DB.ExecContext(ctx, `UPDATE migration_control_totals SET target_value=$3, updated_at=now()
			WHERE run_id=$1 AND category=$2 AND metric='count' AND target_source='auto'`, m.ID, cat, v); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) writeRunDetail(w http.ResponseWriter, r *http.Request, id, msg string) {
	m, err := a.getRun(r.Context(), id)
	if err == nil {
		m.Totals, err = a.runTotals(r.Context(), id)
	}
	if err == nil {
		m.Batches, err = a.listBatches(r.Context(), "", id)
	}
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat migrasi")
		return
	}
	m.Unexplained = unexplainedDifferences(m.Totals)
	common.WriteJSONMsg(w, http.StatusOK, m, map[string]any{"auto_categories": autoCategories}, msg)
}

// RecomputeMigrationRun: POST /migration-runs/{id}/recompute.
func (a *App) RecomputeMigrationRun(w http.ResponseWriter, r *http.Request) {
	m, ok := a.loadRunForHandler(w, r)
	if !ok {
		return
	}
	if m.Status != "open" {
		common.WriteError(w, http.StatusConflict, "Migrasi sudah "+runStatusLabel(m.Status))
		return
	}
	if err := a.recomputeTargets(r.Context(), m); err != nil {
		common.WriteDBError(w, err, "Gagal menghitung nilai SEKOCI")
		return
	}
	a.writeRunDetail(w, r, m.ID, "Nilai SEKOCI dihitung ulang")
}

func runStatusLabel(s string) string {
	switch s {
	case "signed_off":
		return "disahkan"
	case "discarded":
		return "dibatalkan"
	}
	return "terbuka"
}

// SignOffMigrationRun: POST /migration-runs/{id}/sign-off {note}.
func (a *App) SignOffMigrationRun(w http.ResponseWriter, r *http.Request) {
	m, ok := a.loadRunForHandler(w, r)
	if !ok {
		return
	}
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusForbidden, "Hanya untuk pengguna")
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if m.Status != "open" {
		common.WriteError(w, http.StatusConflict, "Migrasi sudah "+runStatusLabel(m.Status))
		return
	}
	if m.CreatedBy != nil && *m.CreatedBy == claims.UserID {
		common.WriteError(w, http.StatusConflict, "Pengesahan harus dilakukan oleh orang lain selain pembuat migrasi (maker-checker)")
		return
	}
	if err := a.recomputeTargets(r.Context(), m); err != nil {
		common.WriteDBError(w, err, "Gagal menghitung nilai SEKOCI")
		return
	}
	totals, err := a.runTotals(r.Context(), m.ID)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat control total")
		return
	}
	if len(totals) == 0 {
		common.WriteError(w, http.StatusConflict, "Belum ada control total. Isi jumlah data dari sistem lama per kategori")
		return
	}
	if n := unexplainedDifferences(totals); n > 0 {
		common.WriteJSONMsg(w, http.StatusConflict, map[string]any{"unexplained_differences": n, "totals": totals}, nil,
			fmt.Sprintf("%d kategori belum cocok: isi nilai sistem lama dan beri penjelasan (min. 10 karakter) untuk setiap selisih", n))
		return
	}
	res, err := a.DB.ExecContext(r.Context(), `UPDATE migration_runs SET status='signed_off', signed_off_by=$2, signed_off_at=now(),
		sign_off_note=NULLIF(btrim($3),'') WHERE id=$1 AND status='open'`, m.ID, claims.UserID, body.Note)
	if err != nil {
		common.WriteDBError(w, err, "Gagal mengesahkan migrasi")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusConflict, "Status migrasi berubah. Muat ulang halaman")
		return
	}
	sid := m.SchoolID
	a.RecordSecurityAudit(&sid, "migration_run_signed_off", m.ID, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"mode": m.Mode, "totals": totals}, body.Note, r.Header.Get("X-Correlation-ID"))
	msg := "Migrasi disahkan"
	if m.Mode == "trial" {
		msg = "Migrasi percobaan disahkan. Migrasi produksi sekarang dapat dibuka"
	}
	a.writeRunDetail(w, r, m.ID, msg)
}

// DiscardMigrationRun: POST /migration-runs/{id}/discard {reason}.
func (a *App) DiscardMigrationRun(w http.ResponseWriter, r *http.Request) {
	m, ok := a.loadRunForHandler(w, r)
	if !ok {
		return
	}
	claims := common.FromCtx(r)
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Reason = strings.TrimSpace(body.Reason)
	if len([]rune(body.Reason)) < 10 {
		common.WriteValidationError(w, http.StatusBadRequest, "Alasan minimal 10 karakter", map[string]string{"reason": "Minimal 10 karakter"})
		return
	}
	if m.Status != "open" {
		common.WriteError(w, http.StatusConflict, "Migrasi sudah "+runStatusLabel(m.Status))
		return
	}
	actor := ""
	if claims != nil {
		actor = claims.UserID
	}
	if _, err := a.DB.ExecContext(r.Context(), `UPDATE migration_runs SET status='discarded', discarded_by=$2, discarded_at=now(), discard_reason=$3
		WHERE id=$1 AND status='open'`, m.ID, nullUUID(actor), body.Reason); err != nil {
		common.WriteDBError(w, err, "Gagal membatalkan migrasi")
		return
	}
	a.writeRunDetail(w, r, m.ID, "Migrasi dibatalkan. Data yang sudah diimpor ke produksi tetap ada; batalkan per impor bila perlu")
}

// runForImport validates an optional run_id on an import (open, same unit).
func (a *App) runForImport(ctx context.Context, runID, schoolID string) (*migrationRun, string) {
	if runID == "" {
		return nil, ""
	}
	if !rules.IsUUID(runID) {
		return nil, "Migrasi tidak valid"
	}
	m, err := a.getRun(ctx, runID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && m.SchoolID != schoolID) {
		return nil, "Migrasi tidak ditemukan di unit ini"
	} else if err != nil {
		common.CaptureError(err)
		return nil, "Gagal memeriksa migrasi"
	}
	if m.Status != "open" {
		return nil, "Migrasi sudah " + runStatusLabel(m.Status) + "; buat migrasi baru"
	}
	return &m, ""
}
