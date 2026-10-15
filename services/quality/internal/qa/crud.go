package qa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"skci/quality/internal/common"
)

// Structural edits (standards/items/indicators) and the evidence,
// self-evaluation and action-plan endpoints the web calls. Every query is
// scoped by school_id and, for tree nodes, by the instrument in the URL, so a
// node of another instrument or school is a 404.

var (
	ErrNotFound         = errors.New("data tidak ditemukan")
	ErrInstrumentInUse  = errors.New("instrumen sudah dipakai evaluasi diri yang diajukan/final; buat versi baru untuk mengubah struktur")
	ErrEvidenceLocked   = errors.New("bukti terkunci oleh evaluasi diri yang sudah final")
	ErrDuplicateVersion = errors.New("label versi sudah dipakai untuk instrumen ini")
)

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

// instrumentEditable: the instrument exists in the school and no self
// evaluation beyond draft references it (BR-QA: finalized results must keep
// the structure they were scored against).
func (s *Store) instrumentEditable(ctx context.Context, schoolID, instrumentID string) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM qa_instruments WHERE id::text = $1 AND school_id::text = $2 AND deleted_at IS NULL)`,
		instrumentID, schoolID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	var used bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM qa_self_evaluations WHERE instrument_id::text = $1 AND status <> 'draft')`, instrumentID).Scan(&used); err != nil {
		return err
	}
	if used {
		return ErrInstrumentInUse
	}
	return nil
}

func (s *Store) standardInInstrument(ctx context.Context, schoolID, instrumentID, stdID string) error {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM qa_standards WHERE id::text = $1 AND instrument_id::text = $2 AND school_id::text = $3)`,
		stdID, instrumentID, schoolID).Scan(&ok)
	if err == nil && !ok {
		return ErrNotFound
	}
	return err
}

func (s *Store) itemInInstrument(ctx context.Context, schoolID, instrumentID, itemID string) error {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM qa_items it JOIN qa_standards st ON st.id = it.standard_id
		WHERE it.id::text = $1 AND st.instrument_id::text = $2 AND it.school_id::text = $3)`,
		itemID, instrumentID, schoolID).Scan(&ok)
	if err == nil && !ok {
		return ErrNotFound
	}
	return err
}

func (s *Store) indicatorInInstrument(ctx context.Context, schoolID, instrumentID, indID string) error {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM qa_indicators ind JOIN qa_items it ON it.id = ind.item_id JOIN qa_standards st ON st.id = it.standard_id
		WHERE ind.id::text = $1 AND st.instrument_id::text = $2 AND ind.school_id::text = $3 AND ind.deleted_at IS NULL)`,
		indID, instrumentID, schoolID).Scan(&ok)
	if err == nil && !ok {
		return ErrNotFound
	}
	return err
}

// NewVersion copies an instrument with its whole tree into a new draft
// version (FR-141: "Versi baru").
func (s *Store) NewVersion(ctx context.Context, schoolID, id, versionLabel string) (*QAInstrument, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var newID string
	err = tx.QueryRow(ctx, `
		INSERT INTO qa_instruments (school_id, code, name, issuer, version_label, score_min, score_max, status, effective_from)
		SELECT school_id, code, name, issuer, $3::text, score_min, score_max, 'draft', NULL
		FROM qa_instruments WHERE id::text = $1 AND school_id::text = $2 AND deleted_at IS NULL
		RETURNING id`, id, schoolID, versionLabel).Scan(&newID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if isUnique(err) {
		return nil, ErrDuplicateVersion
	}
	if err != nil {
		return nil, err
	}
	// Standards, then items, then indicators, mapping old ids to new ones.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE IF NOT EXISTS qa_copy_map (old_id uuid PRIMARY KEY, new_id uuid NOT NULL) ON COMMIT DROP`); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		WITH src AS (SELECT id, gen_random_uuid() AS nid FROM qa_standards WHERE instrument_id::text = $1),
		ins AS (
			INSERT INTO qa_standards (id, school_id, instrument_id, sort, code, name, weight_bp)
			SELECT src.nid, s.school_id, $2::uuid, s.sort, s.code, s.name, s.weight_bp
			FROM qa_standards s JOIN src ON src.id = s.id
			RETURNING id
		)
		INSERT INTO qa_copy_map SELECT id, nid FROM src`, id, newID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		WITH src AS (
			SELECT it.id, gen_random_uuid() AS nid, m.new_id AS std
			FROM qa_items it JOIN qa_copy_map m ON m.old_id = it.standard_id
		),
		ins AS (
			INSERT INTO qa_items (id, school_id, standard_id, sort, code, statement)
			SELECT src.nid, it.school_id, src.std, it.sort, it.code, it.statement
			FROM qa_items it JOIN src ON src.id = it.id
			RETURNING id
		)
		INSERT INTO qa_copy_map SELECT id, nid FROM src`); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO qa_indicators (school_id, item_id, sort, code, name, weight_bp, unit, direction, source_kind, metric_key, rubric)
		SELECT ind.school_id, m.new_id, ind.sort, ind.code, ind.name, ind.weight_bp, ind.unit, ind.direction, ind.source_kind, ind.metric_key, ind.rubric
		FROM qa_indicators ind JOIN qa_copy_map m ON m.old_id = ind.item_id
		WHERE ind.deleted_at IS NULL`); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetInstrument(ctx, schoolID, newID)
}

func (s *Store) UpdateStandard(ctx context.Context, std QAStandard) error {
	_, err := s.pool.Exec(ctx, `UPDATE qa_standards SET sort = $3, code = $4, name = $5, weight_bp = $6, updated_at = now() WHERE id::text = $1 AND school_id::text = $2`,
		std.ID, std.SchoolID, std.Sort, std.Code, std.Name, std.WeightBP)
	return err
}

func (s *Store) UpdateItem(ctx context.Context, it QAItem) error {
	_, err := s.pool.Exec(ctx, `UPDATE qa_items SET sort = $3, code = $4, statement = $5, updated_at = now() WHERE id::text = $1 AND school_id::text = $2`,
		it.ID, it.SchoolID, it.Sort, it.Code, it.Statement)
	return err
}

func (s *Store) UpdateIndicator(ctx context.Context, ind QAIndicator) error {
	rb, _ := json.Marshal(ind.Rubric)
	if ind.Rubric == nil {
		rb = []byte("[]")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE qa_indicators SET sort = $3, code = $4, name = $5, weight_bp = $6, unit = $7, direction = $8,
		       source_kind = $9, metric_key = $10, rubric = $11, updated_at = now()
		WHERE id::text = $1 AND school_id::text = $2 AND deleted_at IS NULL`,
		ind.ID, ind.SchoolID, ind.Sort, ind.Code, ind.Name, ind.WeightBP, ind.Unit, ind.Direction, ind.SourceKind, ind.MetricKey, rb)
	return err
}

// Deletes: a standard/item cascades to its children; an indicator is soft
// deleted (self-evaluation items keep their reference).
func (s *Store) DeleteStandard(ctx context.Context, schoolID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM qa_standards WHERE id::text = $1 AND school_id::text = $2`, id, schoolID)
	return err
}

func (s *Store) DeleteItem(ctx context.Context, schoolID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM qa_items WHERE id::text = $1 AND school_id::text = $2`, id, schoolID)
	return err
}

func (s *Store) DeleteIndicator(ctx context.Context, schoolID, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE qa_indicators SET deleted_at = now(), updated_at = now() WHERE id::text = $1 AND school_id::text = $2`, id, schoolID)
	return err
}

func (s *Store) getEvidence(ctx context.Context, schoolID, id string) (*QAEvidence, bool, error) {
	var ev QAEvidence
	var refRaw []byte
	var locked bool
	err := s.pool.QueryRow(ctx, `
		SELECT e.id, e.school_id, e.kind, e.title, e.object_key, e.mime, e.size_bytes, e.checksum, e.report_ref, e.url,
		       e.version, e.uploaded_by, e.status,
		       EXISTS (SELECT 1 FROM qa_evidence_links l WHERE l.evidence_id = e.id AND l.locked_at IS NOT NULL)
		FROM qa_evidence e WHERE e.id::text = $1 AND e.school_id::text = $2 AND e.deleted_at IS NULL`, id, schoolID).
		Scan(&ev.ID, &ev.SchoolID, &ev.Kind, &ev.Title, &ev.ObjectKey, &ev.Mime, &ev.SizeBytes, &ev.Checksum, &refRaw, &ev.URL,
			&ev.Version, &ev.UploadedBy, &ev.Status, &locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrNotFound
	}
	if err != nil {
		return nil, false, err
	}
	if len(refRaw) > 0 {
		_ = json.Unmarshal(refRaw, &ev.ReportRef)
	}
	return &ev, locked, nil
}

func (s *Store) DeleteEvidence(ctx context.Context, schoolID, id string) error {
	_, locked, err := s.getEvidence(ctx, schoolID, id)
	if err != nil {
		return err
	}
	if locked {
		return ErrEvidenceLocked
	}
	_, err = s.pool.Exec(ctx, `UPDATE qa_evidence SET deleted_at = now(), updated_at = now() WHERE id::text = $1 AND school_id::text = $2`, id, schoolID)
	return err
}

// NewEvidenceVersion stores a new version that supersedes the old one: the
// links move to the new row (pending verification again) and the old row is
// soft-deleted but kept for history.
func (s *Store) NewEvidenceVersion(ctx context.Context, old *QAEvidence, nv QAEvidence) (*QAEvidence, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var refBytes []byte
	if nv.ReportRef != nil {
		refBytes, _ = json.Marshal(nv.ReportRef)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO qa_evidence (school_id, kind, title, object_key, mime, size_bytes, checksum, report_ref, url, version, supersedes_id, uploaded_by, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::uuid, $12::uuid, 'pending')
		RETURNING id, created_at, updated_at`,
		old.SchoolID, old.Kind, old.Title, nv.ObjectKey, nv.Mime, nv.SizeBytes, nv.Checksum, refBytes, nv.URL, old.Version+1, old.ID, nv.UploadedBy).
		Scan(&nv.ID, &nv.CreatedAt, &nv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE qa_evidence_links SET evidence_id = $2::uuid WHERE evidence_id::text = $1`, old.ID, nv.ID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE qa_evidence SET deleted_at = now(), updated_at = now() WHERE id::text = $1`, old.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	nv.SchoolID, nv.Kind, nv.Title, nv.Version, nv.Status = old.SchoolID, old.Kind, old.Title, old.Version+1, "pending"
	nv.SupersedesID = &old.ID
	return &nv, nil
}

// ------------------- handlers -------------------

func (h *Handler) writeStoreErr(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, ErrNotFound):
		common.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInstrumentInUse), errors.Is(err, ErrEvidenceLocked), errors.Is(err, ErrDuplicateVersion), errors.Is(err, ErrEvaluationLocked):
		common.WriteError(w, http.StatusConflict, err.Error())
	case isUnique(err):
		common.WriteError(w, http.StatusConflict, "kode sudah dipakai")
	default:
		log.Printf("quality: %s: %v", fallback, err)
		common.WriteError(w, http.StatusInternalServerError, fallback)
	}
}

func (h *Handler) NewInstrumentVersion(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)
	var req struct {
		VersionLabel string `json:"version_label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.VersionLabel) == "" {
		common.WriteError(w, http.StatusBadRequest, "label versi wajib diisi")
		return
	}
	inst, err := h.store.NewVersion(r.Context(), schoolID, chi.URLParam(r, "id"), strings.TrimSpace(req.VersionLabel))
	if err != nil {
		h.writeStoreErr(w, err, "gagal membuat versi baru")
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_instrument", inst.ID, "new_version", claims.UserID, claims.ActiveRole, nil, map[string]string{"from": chi.URLParam(r, "id"), "version_label": inst.VersionLabel}, "Versi baru instrumen")
	common.WriteJSON(w, http.StatusCreated, inst)
}

func validInstrument(in *QAInstrument) string {
	in.Code, in.Name, in.VersionLabel = strings.TrimSpace(in.Code), strings.TrimSpace(in.Name), strings.TrimSpace(in.VersionLabel)
	if in.Code == "" || in.Name == "" || in.VersionLabel == "" || strings.TrimSpace(in.Issuer) == "" {
		return "kode, nama, penerbit, dan label versi instrumen wajib diisi"
	}
	if in.ScoreMin >= in.ScoreMax {
		return "skor maksimum harus lebih besar dari skor minimum"
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	if in.Status != "draft" && in.Status != "active" && in.Status != "retired" {
		return "status instrumen harus draft, active, atau retired"
	}
	if in.EffectiveFrom != nil && *in.EffectiveFrom != "" {
		if _, err := time.Parse("2006-01-02", *in.EffectiveFrom); err != nil {
			return "tanggal berlaku harus berformat YYYY-MM-DD"
		}
	}
	return ""
}

func validStandard(s *QAStandard) string {
	s.Code, s.Name = strings.TrimSpace(s.Code), strings.TrimSpace(s.Name)
	if s.Code == "" || s.Name == "" {
		return "kode dan nama standar wajib diisi"
	}
	if s.WeightBP < 0 || s.WeightBP > 10000 {
		return "bobot standar harus 0–10000 bp"
	}
	return ""
}

func validItem(it *QAItem) string {
	it.Code, it.Statement = strings.TrimSpace(it.Code), strings.TrimSpace(it.Statement)
	if it.Code == "" || it.Statement == "" {
		return "kode dan pernyataan butir wajib diisi"
	}
	return ""
}

func validIndicator(ind *QAIndicator) string {
	ind.Code, ind.Name = strings.TrimSpace(ind.Code), strings.TrimSpace(ind.Name)
	if ind.Code == "" || ind.Name == "" {
		return "kode dan nama indikator wajib diisi"
	}
	if ind.WeightBP < 0 || ind.WeightBP > 10000 {
		return "bobot indikator harus 0–10000 bp"
	}
	switch ind.Unit {
	case "percent_bp", "count", "ratio_milli", "idr", "days", "score":
	default:
		return "satuan indikator tidak dikenal"
	}
	if ind.Direction != "higher_better" && ind.Direction != "lower_better" {
		return "arah indikator harus higher_better atau lower_better"
	}
	switch ind.SourceKind {
	case "manual":
		ind.MetricKey = nil
	case "metric":
		if ind.MetricKey == nil || strings.TrimSpace(*ind.MetricKey) == "" {
			return "metric_key wajib diisi jika sumber data metrik"
		}
	default:
		return "sumber data harus manual atau metric"
	}
	if ind.Rubric == nil {
		ind.Rubric = []RubricTier{}
	}
	return ""
}

// structural runs the common guard for every tree mutation.
func (h *Handler) structural(w http.ResponseWriter, r *http.Request) (schoolID, instID string, ok bool) {
	schoolID = h.effectiveSchoolID(r)
	instID = chi.URLParam(r, "id")
	if err := h.store.instrumentEditable(r.Context(), schoolID, instID); err != nil {
		h.writeStoreErr(w, err, "gagal memeriksa instrumen")
		return "", "", false
	}
	return schoolID, instID, true
}

func (h *Handler) UpdateStandard(w http.ResponseWriter, r *http.Request) {
	schoolID, instID, ok := h.structural(w, r)
	if !ok {
		return
	}
	var req QAStandard
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if msg := validStandard(&req); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	req.ID, req.SchoolID, req.InstrumentID = chi.URLParam(r, "stdId"), schoolID, instID
	if err := h.store.standardInInstrument(r.Context(), schoolID, instID, req.ID); err != nil {
		h.writeStoreErr(w, err, "gagal memuat standar")
		return
	}
	if err := h.store.UpdateStandard(r.Context(), req); err != nil {
		h.writeStoreErr(w, err, "gagal memperbarui standar")
		return
	}
	common.WriteJSON(w, http.StatusOK, req)
}

func (h *Handler) DeleteStandard(w http.ResponseWriter, r *http.Request) {
	schoolID, instID, ok := h.structural(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "stdId")
	if err := h.store.standardInInstrument(r.Context(), schoolID, instID, id); err != nil {
		h.writeStoreErr(w, err, "gagal memuat standar")
		return
	}
	if err := h.store.DeleteStandard(r.Context(), schoolID, id); err != nil {
		h.writeStoreErr(w, err, "standar tidak dapat dihapus karena indikatornya sudah dinilai")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Standar dihapus"})
}

// CreateInstrumentItem: POST /qa/instruments/{id}/items {standard_id,...}.
func (h *Handler) CreateInstrumentItem(w http.ResponseWriter, r *http.Request) {
	schoolID, instID, ok := h.structural(w, r)
	if !ok {
		return
	}
	var req QAItem
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if msg := validItem(&req); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	if err := h.store.standardInInstrument(r.Context(), schoolID, instID, req.StandardID); err != nil {
		h.writeStoreErr(w, err, "gagal memuat standar")
		return
	}
	req.SchoolID = schoolID
	it, err := h.store.CreateItem(r.Context(), req)
	if err != nil {
		h.writeStoreErr(w, err, "gagal membuat butir")
		return
	}
	common.WriteJSON(w, http.StatusCreated, it)
}

func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	schoolID, instID, ok := h.structural(w, r)
	if !ok {
		return
	}
	var req QAItem
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if msg := validItem(&req); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	req.ID, req.SchoolID = chi.URLParam(r, "itemId"), schoolID
	if err := h.store.itemInInstrument(r.Context(), schoolID, instID, req.ID); err != nil {
		h.writeStoreErr(w, err, "gagal memuat butir")
		return
	}
	if err := h.store.UpdateItem(r.Context(), req); err != nil {
		h.writeStoreErr(w, err, "gagal memperbarui butir")
		return
	}
	common.WriteJSON(w, http.StatusOK, req)
}

func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	schoolID, instID, ok := h.structural(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "itemId")
	if err := h.store.itemInInstrument(r.Context(), schoolID, instID, id); err != nil {
		h.writeStoreErr(w, err, "gagal memuat butir")
		return
	}
	if err := h.store.DeleteItem(r.Context(), schoolID, id); err != nil {
		h.writeStoreErr(w, err, "butir tidak dapat dihapus karena indikatornya sudah dinilai")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Butir dihapus"})
}

// CreateInstrumentIndicator: POST /qa/instruments/{id}/indicators {item_id,...}.
func (h *Handler) CreateInstrumentIndicator(w http.ResponseWriter, r *http.Request) {
	schoolID, instID, ok := h.structural(w, r)
	if !ok {
		return
	}
	var req QAIndicator
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if msg := validIndicator(&req); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	if err := h.store.itemInInstrument(r.Context(), schoolID, instID, req.ItemID); err != nil {
		h.writeStoreErr(w, err, "gagal memuat butir")
		return
	}
	req.SchoolID = schoolID
	ind, err := h.store.CreateIndicator(r.Context(), req)
	if err != nil {
		h.writeStoreErr(w, err, "gagal membuat indikator")
		return
	}
	common.WriteJSON(w, http.StatusCreated, ind)
}

func (h *Handler) UpdateIndicator(w http.ResponseWriter, r *http.Request) {
	schoolID, instID, ok := h.structural(w, r)
	if !ok {
		return
	}
	var req QAIndicator
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if msg := validIndicator(&req); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	req.ID, req.SchoolID = chi.URLParam(r, "indId"), schoolID
	if err := h.store.indicatorInInstrument(r.Context(), schoolID, instID, req.ID); err != nil {
		h.writeStoreErr(w, err, "gagal memuat indikator")
		return
	}
	if err := h.store.UpdateIndicator(r.Context(), req); err != nil {
		h.writeStoreErr(w, err, "gagal memperbarui indikator")
		return
	}
	common.WriteJSON(w, http.StatusOK, req)
}

func (h *Handler) DeleteIndicator(w http.ResponseWriter, r *http.Request) {
	schoolID, instID, ok := h.structural(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "indId")
	if err := h.store.indicatorInInstrument(r.Context(), schoolID, instID, id); err != nil {
		h.writeStoreErr(w, err, "gagal memuat indikator")
		return
	}
	if err := h.store.DeleteIndicator(r.Context(), schoolID, id); err != nil {
		h.writeStoreErr(w, err, "gagal menghapus indikator")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Indikator dihapus"})
}

// canManageEvidence: qa_coordinator/admin/superadmin manage all evidence;
// anyone else only what they uploaded (T23 §5 "owner … miliknya").
func canManageEvidence(c *common.Claims, ev *QAEvidence) bool {
	if c == nil {
		return false
	}
	switch c.ActiveRole {
	case common.RoleSuperadmin, common.RoleAdmin, common.RoleQACoordinator:
		return true
	}
	return ev.UploadedBy == c.UserID
}

func (h *Handler) DeleteEvidence(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	ev, _, err := h.store.getEvidence(r.Context(), schoolID, id)
	if err != nil {
		h.writeStoreErr(w, err, "gagal memuat bukti")
		return
	}
	if !canManageEvidence(claims, ev) {
		common.WriteError(w, http.StatusForbidden, "hanya pengunggah atau koordinator mutu yang dapat menghapus bukti ini")
		return
	}
	if err := h.store.DeleteEvidence(r.Context(), schoolID, id); err != nil {
		h.writeStoreErr(w, err, "gagal menghapus bukti")
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_evidence", id, "delete", claims.UserID, claims.ActiveRole, ev, nil, "Bukti dihapus")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Bukti dihapus"})
}

func (h *Handler) DownloadEvidence(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	ev, _, err := h.store.getEvidence(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err, "gagal memuat bukti")
		return
	}
	if ev.Kind != "file" || ev.ObjectKey == nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "bukti ini berupa tautan, bukan berkas")
		return
	}
	if h.storage == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "penyimpanan berkas belum dikonfigurasi")
		return
	}
	f, err := h.storage.Open(*ev.ObjectKey)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "berkas bukti tidak ditemukan di penyimpanan")
		return
	}
	defer f.Close()
	mime := "application/octet-stream"
	if ev.Mime != nil && *ev.Mime != "" {
		mime = *ev.Mime
	}
	name := filepath.Base(*ev.ObjectKey)
	if i := strings.Index(name, "_"); i >= 0 {
		name = name[i+1:]
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(name, `"`, "")))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, f)
}

// NewEvidenceVersion: POST /qa/evidence/{id}/new-version (multipart file,
// or JSON {url} / {report_ref} matching the original kind).
func (h *Handler) NewEvidenceVersion(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)
	old, locked, err := h.store.getEvidence(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err, "gagal memuat bukti")
		return
	}
	if !canManageEvidence(claims, old) {
		common.WriteError(w, http.StatusForbidden, "hanya pengunggah atau koordinator mutu yang dapat memperbarui bukti ini")
		return
	}
	if locked {
		common.WriteError(w, http.StatusConflict, ErrEvidenceLocked.Error())
		return
	}
	nv := QAEvidence{UploadedBy: claims.UserID}
	if old.Kind == "file" {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			common.WriteError(w, http.StatusBadRequest, "berkas wajib diunggah")
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "berkas wajib diunggah")
			return
		}
		defer file.Close()
		if h.storage == nil {
			common.WriteError(w, http.StatusServiceUnavailable, "penyimpanan berkas belum dikonfigurasi")
			return
		}
		mime := header.Header.Get("Content-Type")
		key := fmt.Sprintf("schools/%s/evidence/%d_%s", schoolID, time.Now().UnixNano(), safeFileName(header.Filename))
		sum, size, err := h.storage.Save(key, file)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan berkas")
			return
		}
		nv.ObjectKey, nv.Mime, nv.Checksum, nv.SizeBytes = &key, &mime, &sum, &size
	} else {
		var body struct {
			URL       *string                `json:"url"`
			ReportRef map[string]interface{} `json:"report_ref"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
			return
		}
		if old.Kind == "link" && (body.URL == nil || strings.TrimSpace(*body.URL) == "") {
			common.WriteError(w, http.StatusBadRequest, "url wajib diisi")
			return
		}
		if old.Kind == "sekoci_report" && body.ReportRef == nil {
			common.WriteError(w, http.StatusBadRequest, "report_ref wajib diisi")
			return
		}
		if old.Kind == "link" {
			nv.URL = body.URL
		} else {
			nv.ReportRef = body.ReportRef
		}
	}
	saved, err := h.store.NewEvidenceVersion(r.Context(), old, nv)
	if err != nil {
		h.writeStoreErr(w, err, "gagal menyimpan versi bukti")
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_evidence", saved.ID, "new_version", claims.UserID, claims.ActiveRole, old, saved, "Versi baru bukti")
	common.WriteJSON(w, http.StatusCreated, saved)
}

// safeFileName strips directories so an upload name can never escape the
// school's evidence folder.
func safeFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == "/" || name == ".." || name == "" {
		return "berkas"
	}
	return name
}

// RefreshMetrics: POST /qa/self-evaluations/{id}/refresh-metrics. Metric
// indicators take their value from insight for the evaluation period; an
// unavailable metric leaves the item untouched (never filled with 0,
// AC-142-01) and is reported in the message.
func (h *Handler) RefreshMetrics(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	se, err := h.store.GetSelfEvaluation(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat evaluasi diri")
		return
	}
	if se == nil {
		common.WriteError(w, http.StatusNotFound, "evaluasi diri tidak ditemukan")
		return
	}
	if IsLocked(se.Status) {
		common.WriteError(w, http.StatusConflict, ErrEvaluationLocked.Error())
		return
	}
	inst, err := h.store.GetInstrument(r.Context(), schoolID, se.InstrumentID)
	if err != nil || inst == nil {
		common.WriteError(w, http.StatusNotFound, "instrumen tidak ditemukan")
		return
	}
	inds := map[string]QAIndicator{}
	for _, st := range inst.Standards {
		for _, it := range st.Items {
			for _, ind := range it.Indicators {
				inds[ind.ID] = ind
			}
		}
	}
	var failed []string
	updated := 0
	for _, item := range se.Items {
		ind, ok := inds[item.IndicatorID]
		if !ok || ind.SourceKind != "metric" || ind.MetricKey == nil {
			continue
		}
		if h.clients == nil {
			failed = append(failed, ind.Code)
			continue
		}
		res, err := h.clients.GetMetricValue(r.Context(), schoolID, *ind.MetricKey, se.PeriodFrom, se.PeriodTo, "")
		if err != nil || res == nil {
			failed = append(failed, ind.Code)
			continue
		}
		val := res.Value
		score := ScoreFromRubric(val, ind.Rubric, inst.ScoreMin)
		below := false
		if item.TargetValue != nil {
			below = BelowTarget(&val, *item.TargetValue, ind.Direction)
		}
		if _, err := h.store.pool.Exec(r.Context(), `
			UPDATE qa_self_eval_items
			SET actual_value = $2, score = $3, below_target = $4, value_source = 'metric',
			    metric_as_of = $5, metric_fetched_at = now(), updated_at = now()
			WHERE id::text = $1`, item.ID, val, score, below, res.AsOf); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan nilai metrik")
			return
		}
		updated++
	}
	fresh, _ := h.store.GetSelfEvaluation(r.Context(), schoolID, id)
	_ = h.store.AttachStandardScores(r.Context(), schoolID, fresh)
	msg := fmt.Sprintf("%d indikator metrik diperbarui", updated)
	if len(failed) > 0 {
		msg += "; sumber metrik tidak tersedia untuk: " + strings.Join(failed, ", ")
	}
	w.Header().Set("X-Refresh-Message", msg)
	common.WriteJSON(w, http.StatusOK, fresh)
}

// SelfEvaluationReport: GET /qa/reports/self-evaluation/{id}[?format=xlsx].
func (h *Handler) SelfEvaluationReport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	se, err := h.store.GetSelfEvaluation(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat evaluasi diri")
		return
	}
	if se == nil {
		common.WriteError(w, http.StatusNotFound, "evaluasi diri tidak ditemukan")
		return
	}
	inst, err := h.store.GetInstrument(r.Context(), schoolID, se.InstrumentID)
	if err != nil || inst == nil {
		common.WriteError(w, http.StatusNotFound, "instrumen tidak ditemukan")
		return
	}
	byInd := map[string]QASelfEvalItem{}
	for _, it := range se.Items {
		byInd[it.IndicatorID] = it
	}
	type row struct {
		Standard, Item, Code, Name string
		WeightBP                   int
		Target, Actual             *int64
		Score                      *int
		Below                      bool
		Note                       string
		Plans                      int
	}
	var rows []row
	for _, st := range inst.Standards {
		for _, it := range st.Items {
			for _, ind := range it.Indicators {
				ei := byInd[ind.ID]
				note := ""
				if ei.Note != nil {
					note = *ei.Note
				}
				rows = append(rows, row{st.Code + " " + st.Name, it.Code, ind.Code, ind.Name, ind.WeightBP, ei.TargetValue, ei.ActualValue, ei.Score, ei.BelowTarget, note, len(ei.ActionPlans)})
			}
		}
	}
	if r.URL.Query().Get("format") != "xlsx" {
		common.WriteJSON(w, http.StatusOK, map[string]any{"evaluation": se, "instrument": inst.Name, "rows": rows})
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	sheet := "EDS"
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetCellValue(sheet, "A1", fmt.Sprintf("Evaluasi Diri %s – %s (%s s/d %s), status %s", inst.Name, se.PeriodLabel, se.PeriodFrom, se.PeriodTo, se.Status))
	headers := []string{"Standar", "Butir", "Kode Indikator", "Indikator", "Bobot (bp)", "Target", "Capaian", "Skor", "Di bawah target", "Catatan", "Jumlah RTL"}
	for i, hd := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = f.SetCellValue(sheet, cell, hd)
	}
	for ri, rw := range rows {
		vals := []any{rw.Standard, rw.Item, rw.Code, rw.Name, rw.WeightBP, nilOr(rw.Target), nilOr(rw.Actual), nilOrInt(rw.Score), map[bool]string{true: "Ya", false: "Tidak"}[rw.Below], rw.Note, rw.Plans}
		for ci, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(ci+1, ri+4)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_self_evaluation", se.ID, "export", claims.UserID, claims.ActiveRole, nil, map[string]string{"format": "xlsx"}, "Ekspor EDS")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="EDS_%s.xlsx"`, strings.ReplaceAll(se.PeriodLabel, `"`, "")))
	_ = f.Write(w)
}

func nilOr(v *int64) any {
	if v == nil {
		return ""
	}
	return *v
}

func nilOrInt(v *int) any {
	if v == nil {
		return ""
	}
	return *v
}

// ------------------- Action plans: PUT / DELETE -------------------

func isQAManager(c *common.Claims) bool {
	if c == nil {
		return false
	}
	switch c.ActiveRole {
	case common.RoleSuperadmin, common.RoleAdmin, common.RoleQACoordinator, common.RoleKepalaSekolah:
		return true
	}
	return false
}

func (s *Store) actionPlanOwner(ctx context.Context, schoolID, id string) (string, error) {
	var owner string
	err := s.pool.QueryRow(ctx, `SELECT owner_user_id::text FROM qa_action_plans WHERE id::text = $1 AND school_id::text = $2 AND deleted_at IS NULL`, id, schoolID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return owner, err
}

func (h *Handler) UpdateActionPlan(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	owner, err := h.store.actionPlanOwner(r.Context(), schoolID, id)
	if err != nil {
		h.writeStoreErr(w, err, "gagal memuat RTL")
		return
	}
	if !isQAManager(claims) && owner != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "RTL ini bukan milik Anda")
		return
	}
	var req struct {
		Action      *string `json:"action"`
		OwnerUserID *string `json:"owner_user_id"`
		DueDate     *string `json:"due_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if req.OwnerUserID != nil && !isQAManager(claims) {
		common.WriteError(w, http.StatusForbidden, "hanya koordinator mutu yang dapat mengganti penanggung jawab")
		return
	}
	if req.DueDate != nil {
		if _, err := time.Parse("2006-01-02", *req.DueDate); err != nil {
			common.WriteError(w, http.StatusBadRequest, "tenggat harus berformat YYYY-MM-DD")
			return
		}
	}
	if _, err := h.store.pool.Exec(r.Context(), `
		UPDATE qa_action_plans
		SET action = COALESCE($3, action), owner_user_id = COALESCE($4::uuid, owner_user_id),
		    due_date = COALESCE($5::date, due_date), updated_at = now()
		WHERE id::text = $1 AND school_id::text = $2`, id, schoolID, req.Action, req.OwnerUserID, req.DueDate); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memperbarui RTL")
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_action_plan", id, "update", claims.UserID, claims.ActiveRole, nil, req, "RTL diperbarui")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "RTL diperbarui"})
}

func (h *Handler) DeleteActionPlan(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if _, err := h.store.actionPlanOwner(r.Context(), schoolID, id); err != nil {
		h.writeStoreErr(w, err, "gagal memuat RTL")
		return
	}
	if !isQAManager(claims) {
		common.WriteError(w, http.StatusForbidden, "hanya koordinator mutu yang dapat menghapus RTL")
		return
	}
	if _, err := h.store.pool.Exec(r.Context(), `UPDATE qa_action_plans SET deleted_at = now(), updated_at = now() WHERE id::text = $1 AND school_id::text = $2`, id, schoolID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menghapus RTL")
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_action_plan", id, "delete", claims.UserID, claims.ActiveRole, nil, nil, "RTL dihapus")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "RTL dihapus"})
}
