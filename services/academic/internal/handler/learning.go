package handler

// Learning module (PRD v2.1 menus 57/70 Materi, 58/71 Tugas). Everything is
// keyed by a classroom_subjects row: the guru pengampu of that row (or an
// admin of the school) manages it; a siswa sees only PUBLISHED items of the
// classroom_subjects that ActiveSubjectsForStudent resolves for them this
// academic year. Files are written under STORAGE_ROOT (same private-volume
// approach as billing/admission) -- only metadata reaches the database.
//
// These routes are registered OUTSIDE the Redis GET cache (see main.go): a
// siswa who just submitted must see "Dikumpulkan" on the next list call, and
// file downloads must never be copied into Redis.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
)

const learningMaxUpload = 20 << 20 // 20 MB per file

var learningAllowedExt = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".ppt": true, ".pptx": true,
	".xls": true, ".xlsx": true, ".txt": true, ".zip": true,
	".jpg": true, ".jpeg": true, ".png": true, ".mp4": true,
}

func learningStorageRoot() string {
	if v := os.Getenv("STORAGE_ROOT"); v != "" {
		return v
	}
	return "./data/private"
}

type learningFile struct {
	Key, Name, MIME string
	Size            int64
}

// saveLearningUpload stores the multipart field `field` if present. Returns
// (nil, nil) when the request carries no file for that field.
func saveLearningUpload(r *http.Request, field, schoolID, kind string) (*learningFile, error) {
	f, hdr, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("file upload tidak valid")
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if !learningAllowedExt[ext] {
		return nil, errors.New("tipe file tidak didukung (pdf, office, gambar, zip, txt, mp4)")
	}
	if hdr.Size <= 0 || hdr.Size > learningMaxUpload {
		return nil, errors.New("ukuran file maksimal 20 MB")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	key := filepath.Join(schoolID, "learning", kind, hex.EncodeToString(b)+ext)
	p := filepath.Join(learningStorageRoot(), key)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	out, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	n, copyErr := io.Copy(out, io.LimitReader(f, learningMaxUpload+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || n > learningMaxUpload {
		_ = os.Remove(p)
		return nil, errors.New("gagal menyimpan file")
	}
	mt := mime.TypeByExtension(ext)
	if mt == "" {
		mt = "application/octet-stream"
	}
	return &learningFile{Key: key, Name: filepath.Base(hdr.Filename), MIME: mt, Size: n}, nil
}

func removeLearningFile(key *string) {
	if key == nil || *key == "" || strings.Contains(*key, "..") {
		return
	}
	_ = os.Remove(filepath.Join(learningStorageRoot(), *key))
}

func serveLearningFile(w http.ResponseWriter, key, name, mt *string) {
	if key == nil || *key == "" || strings.Contains(*key, "..") {
		common.WriteError(w, http.StatusNotFound, "file tidak ada")
		return
	}
	f, err := os.Open(filepath.Join(learningStorageRoot(), *key))
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "file tidak ditemukan")
		return
	}
	defer f.Close()
	ct, fn := "application/octet-stream", "lampiran"
	if mt != nil && *mt != "" {
		ct = *mt
	}
	if name != nil && *name != "" {
		fn = *name
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fn}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, f)
}

// learningCS is the classroom_subjects row a material/assignment belongs to.
type learningCS struct {
	ID, SchoolID, AcademicYearID, ClassroomID string
	TeacherUserID                             *string
}

func (h *Handler) getLearningCS(ctx context.Context, schoolID, id string) (*learningCS, error) {
	var cs learningCS
	err := h.Store.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, classroom_id, teacher_user_id
		FROM classroom_subjects
		WHERE id::text = $1 AND school_id::text = $2 AND deleted_at IS NULL`, id, schoolID).
		Scan(&cs.ID, &cs.SchoolID, &cs.AcademicYearID, &cs.ClassroomID, &cs.TeacherUserID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &cs, err
}

func isLearningAdmin(c *common.Claims) bool {
	return c != nil && (c.ActiveRole == common.RoleAdmin || c.ActiveRole == common.RoleSuperadmin)
}

// isLearningStaffReader: roles that may read every item of the school.
func isLearningStaffReader(c *common.Claims) bool {
	return isLearningAdmin(c) || (c != nil && c.ActiveRole == "kepala_sekolah")
}

// canManageCS: admin of the school, or the guru pengampu of this exact row.
func canManageCS(c *common.Claims, cs *learningCS) bool {
	if isLearningAdmin(c) {
		return true
	}
	return c != nil && c.ActiveRole == common.RoleGuru && cs.TeacherUserID != nil && *cs.TeacherUserID == c.UserID
}

// studentCSIDs returns the classroom_subjects ids visible to a siswa this
// academic year (their classroom, active, elective-resolved).
func (h *Handler) studentCSIDs(ctx context.Context, schoolID, studentID, ayID string) (map[string]bool, error) {
	items, err := h.Store.ActiveSubjectsForStudent(ctx, schoolID, studentID, ayID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(items))
	for _, it := range items {
		out[it.ID] = true
	}
	return out, nil
}

func setToSlice(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func formStatus(r *http.Request, fallback string) (string, bool) {
	s := strings.TrimSpace(r.FormValue("status"))
	if s == "" {
		return fallback, true
	}
	return s, s == "draft" || s == "published"
}

func nullIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func validLink(s *string) bool {
	if s == nil {
		return true
	}
	return strings.HasPrefix(*s, "http://") || strings.HasPrefix(*s, "https://")
}

// ---------------------------------------------------------------------------
// Materi
// ---------------------------------------------------------------------------

type learningMaterial struct {
	ID                 string  `json:"id"`
	ClassroomSubjectID string  `json:"classroom_subject_id"`
	ClassroomID        string  `json:"classroom_id"`
	ClassroomName      string  `json:"classroom_name"`
	SubjectID          string  `json:"subject_id"`
	SubjectName        string  `json:"subject_name"`
	TeacherUserID      string  `json:"teacher_user_id"`
	Title              string  `json:"title"`
	Description        *string `json:"description"`
	LinkURL            *string `json:"link_url"`
	FileName           *string `json:"file_name"`
	FileMIME           *string `json:"file_mime"`
	FileSize           *int64  `json:"file_size"`
	Status             string  `json:"status"`
	PublishedAt        *string `json:"published_at"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
	fileKey            *string
}

const materialSelect = `
	SELECT m.id, m.classroom_subject_id, cs.classroom_id, c.name, cs.subject_id, sub.name, m.teacher_user_id,
	       m.title, m.description, m.link_url, m.file_name, m.file_mime, m.file_size, m.status,
	       to_char(m.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	       to_char(m.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	       to_char(m.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	       m.file_key
	FROM learning_materials m
	JOIN classroom_subjects cs ON cs.id = m.classroom_subject_id
	JOIN classrooms c ON c.id = cs.classroom_id
	JOIN subjects sub ON sub.id = cs.subject_id
	WHERE m.deleted_at IS NULL AND m.school_id::text = $1`

func scanMaterial(row interface{ Scan(...any) error }) (*learningMaterial, error) {
	var m learningMaterial
	err := row.Scan(&m.ID, &m.ClassroomSubjectID, &m.ClassroomID, &m.ClassroomName, &m.SubjectID, &m.SubjectName,
		&m.TeacherUserID, &m.Title, &m.Description, &m.LinkURL, &m.FileName, &m.FileMIME, &m.FileSize, &m.Status,
		&m.PublishedAt, &m.CreatedAt, &m.UpdatedAt, &m.fileKey)
	return &m, err
}

func (h *Handler) getMaterial(ctx context.Context, schoolID, id string) (*learningMaterial, error) {
	m, err := scanMaterial(h.Store.DB.QueryRowContext(ctx, materialSelect+` AND m.id::text = $2`, schoolID, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// learningListScope builds the role-specific WHERE fragment shared by the
// material and assignment lists. alias is the item table alias ("m"/"a").
// Returns ok=false after writing the error response.
func (h *Handler) learningListScope(w http.ResponseWriter, r *http.Request, schoolID, alias string, args []any) (string, []any, bool) {
	c := common.FromCtx(r)
	ayID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return "", nil, false
	}
	args = append(args, ayID)
	where := " AND " + alias + ".academic_year_id::text = $" + strconv.Itoa(len(args))
	if cs := r.URL.Query().Get("classroom_subject_id"); cs != "" {
		args = append(args, cs)
		where += " AND " + alias + ".classroom_subject_id::text = $" + strconv.Itoa(len(args))
	}
	switch {
	case isLearningStaffReader(c):
	case c.ActiveRole == common.RoleGuru:
		args = append(args, c.UserID)
		where += " AND cs.teacher_user_id::text = $" + strconv.Itoa(len(args))
	case c.ActiveRole == common.RoleSiswa:
		ids, err := h.studentCSIDs(r.Context(), schoolID, c.UserID, ayID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, err.Error())
			return "", nil, false
		}
		args = append(args, setToSlice(ids))
		where += " AND " + alias + ".status = 'published' AND " + alias + ".classroom_subject_id::text = ANY($" + strconv.Itoa(len(args)) + ")"
	default:
		common.WriteError(w, http.StatusForbidden, "forbidden")
		return "", nil, false
	}
	return where, args, true
}

// ListLearningMaterials: GET /learning/materials?classroom_subject_id=&academic_year_id=
func (h *Handler) ListLearningMaterials(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	where, args, ok := h.learningListScope(w, r, schoolID, "m", []any{schoolID})
	if !ok {
		return
	}
	rows, err := h.Store.DB.QueryContext(r.Context(), materialSelect+where+` ORDER BY COALESCE(m.published_at, m.updated_at) DESC`, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []learningMaterial{}
	for rows.Next() {
		m, err := scanMaterial(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, *m)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func parseLearningForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, learningMaxUpload+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "form multipart tidak valid atau file melebihi 20 MB")
		return false
	}
	return true
}

// CreateLearningMaterial: POST /learning/materials (multipart:
// classroom_subject_id, title, description, link_url, status, file).
func (h *Handler) CreateLearningMaterial(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok || !parseLearningForm(w, r) {
		return
	}
	c := common.FromCtx(r)
	cs, err := h.getLearningCS(r.Context(), schoolID, r.FormValue("classroom_subject_id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cs == nil {
		common.WriteError(w, http.StatusBadRequest, "kelas/mata pelajaran tidak ditemukan")
		return
	}
	if !canManageCS(c, cs) {
		common.WriteError(w, http.StatusForbidden, "anda bukan guru pengampu kelas/mata pelajaran ini")
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	link := nullIfEmpty(r.FormValue("link_url"))
	status, okStatus := formStatus(r, "draft")
	if title == "" || !okStatus || !validLink(link) {
		common.WriteError(w, http.StatusBadRequest, "judul wajib diisi, status draft/published, tautan harus http(s)")
		return
	}
	file, err := saveLearningUpload(r, "file", schoolID, "materials")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if file == nil && link == nil {
		common.WriteError(w, http.StatusBadRequest, "materi harus berisi file atau tautan")
		return
	}
	teacher := c.UserID
	if cs.TeacherUserID != nil {
		teacher = *cs.TeacherUserID
	}
	var fk, fn, fm *string
	var fs *int64
	if file != nil {
		fk, fn, fm, fs = &file.Key, &file.Name, &file.MIME, &file.Size
	}
	var id string
	err = h.Store.DB.QueryRowContext(r.Context(), `
		INSERT INTO learning_materials (school_id, academic_year_id, classroom_subject_id, teacher_user_id, title, description, link_url,
		       file_key, file_name, file_mime, file_size, status, published_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, CASE WHEN $12 = 'published' THEN now() END)
		RETURNING id`,
		cs.SchoolID, cs.AcademicYearID, cs.ID, teacher, title, nullIfEmpty(r.FormValue("description")), link,
		fk, fn, fm, fs, status).Scan(&id)
	if err != nil {
		removeLearningFile(fk)
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	m, _ := h.getMaterial(r.Context(), schoolID, id)
	common.WriteJSON(w, http.StatusCreated, m, nil)
}

// loadManagedMaterial fetches a material and checks the caller manages it.
func (h *Handler) loadManagedMaterial(w http.ResponseWriter, r *http.Request, schoolID string) (*learningMaterial, bool) {
	m, err := h.getMaterial(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if m == nil {
		common.WriteError(w, http.StatusNotFound, "materi tidak ditemukan")
		return nil, false
	}
	cs, err := h.getLearningCS(r.Context(), schoolID, m.ClassroomSubjectID)
	if err != nil || cs == nil || !canManageCS(common.FromCtx(r), cs) {
		common.WriteError(w, http.StatusForbidden, "anda bukan guru pengampu materi ini")
		return nil, false
	}
	return m, true
}

// UpdateLearningMaterial: PUT /learning/materials/{id} (multipart: title,
// description, link_url, status, remove_file=1, file).
func (h *Handler) UpdateLearningMaterial(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok || !parseLearningForm(w, r) {
		return
	}
	m, ok := h.loadManagedMaterial(w, r, schoolID)
	if !ok {
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	link := nullIfEmpty(r.FormValue("link_url"))
	status, okStatus := formStatus(r, m.Status)
	if title == "" || !okStatus || !validLink(link) {
		common.WriteError(w, http.StatusBadRequest, "judul wajib diisi, status draft/published, tautan harus http(s)")
		return
	}
	file, err := saveLearningUpload(r, "file", schoolID, "materials")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	fk, fn, fm, fs := m.fileKey, m.FileName, m.FileMIME, m.FileSize
	oldKey := m.fileKey
	replaced := false
	if file != nil {
		fk, fn, fm, fs = &file.Key, &file.Name, &file.MIME, &file.Size
		replaced = true
	} else if r.FormValue("remove_file") == "1" {
		fk, fn, fm, fs = nil, nil, nil, nil
		replaced = true
	}
	if fk == nil && link == nil {
		if file != nil {
			removeLearningFile(&file.Key)
		}
		common.WriteError(w, http.StatusBadRequest, "materi harus berisi file atau tautan")
		return
	}
	_, err = h.Store.DB.ExecContext(r.Context(), `
		UPDATE learning_materials SET title=$2, description=$3, link_url=$4, file_key=$5, file_name=$6, file_mime=$7, file_size=$8,
		       status=$9, published_at = CASE WHEN $9 = 'published' THEN COALESCE(published_at, now()) END, updated_at=now()
		WHERE id::text=$1`,
		m.ID, title, nullIfEmpty(r.FormValue("description")), link, fk, fn, fm, fs, status)
	if err != nil {
		if file != nil {
			removeLearningFile(&file.Key)
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if replaced {
		removeLearningFile(oldKey)
	}
	out, _ := h.getMaterial(r.Context(), schoolID, m.ID)
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// DeleteLearningMaterial: DELETE /learning/materials/{id} (soft delete; the
// file is removed since nothing can reach it any more).
func (h *Handler) DeleteLearningMaterial(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	m, ok := h.loadManagedMaterial(w, r, schoolID)
	if !ok {
		return
	}
	if _, err := h.Store.DB.ExecContext(r.Context(), `UPDATE learning_materials SET deleted_at=now(), file_key=NULL WHERE id::text=$1`, m.ID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	removeLearningFile(m.fileKey)
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// canReadCSItem: staff readers, the managing guru, or a siswa for whom the
// item is published and the classroom_subject is theirs this year.
func (h *Handler) canReadCSItem(w http.ResponseWriter, r *http.Request, schoolID, csID, status, ayID string) bool {
	c := common.FromCtx(r)
	if isLearningStaffReader(c) {
		return true
	}
	if c.ActiveRole == common.RoleSiswa {
		if status != "published" {
			return false
		}
		ids, err := h.studentCSIDs(r.Context(), schoolID, c.UserID, ayID)
		return err == nil && ids[csID]
	}
	cs, err := h.getLearningCS(r.Context(), schoolID, csID)
	return err == nil && cs != nil && canManageCS(c, cs)
}

// DownloadLearningMaterialFile: GET /learning/materials/{id}/file
func (h *Handler) DownloadLearningMaterialFile(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var csID, status, ayID string
	var key, name, mt *string
	err := h.Store.DB.QueryRowContext(r.Context(), `
		SELECT classroom_subject_id, status, academic_year_id, file_key, file_name, file_mime
		FROM learning_materials WHERE id::text=$1 AND school_id::text=$2 AND deleted_at IS NULL`,
		chi.URLParam(r, "id"), schoolID).Scan(&csID, &status, &ayID, &key, &name, &mt)
	if err != nil || !h.canReadCSItem(w, r, schoolID, csID, status, ayID) {
		common.WriteError(w, http.StatusNotFound, "materi tidak ditemukan")
		return
	}
	serveLearningFile(w, key, name, mt)
}
