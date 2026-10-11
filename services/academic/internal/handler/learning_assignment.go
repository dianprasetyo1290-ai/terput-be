package handler

// Tugas & Pengumpulan (PRD v2.1 menus 58 siswa / 71 guru). See learning.go
// for the access model and file storage.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
)

type learningSubmission struct {
	ID            string   `json:"id"`
	StudentUserID string   `json:"student_user_id"`
	AnswerText    *string  `json:"answer_text"`
	FileName      *string  `json:"file_name"`
	FileSize      *int64   `json:"file_size"`
	SubmittedAt   string   `json:"submitted_at"`
	IsLate        bool     `json:"is_late"`
	Status        string   `json:"status"` // submitted | graded | resubmit_requested
	ResubmitCount int      `json:"resubmit_count"`
	Score         *float64 `json:"score"`
	Feedback      *string  `json:"feedback"`
	GradedAt      *string  `json:"graded_at"`
}

type learningAssignment struct {
	ID                 string  `json:"id"`
	ClassroomSubjectID string  `json:"classroom_subject_id"`
	ClassroomID        string  `json:"classroom_id"`
	ClassroomName      string  `json:"classroom_name"`
	SubjectID          string  `json:"subject_id"`
	SubjectName        string  `json:"subject_name"`
	TeacherUserID      string  `json:"teacher_user_id"`
	Title              string  `json:"title"`
	Instructions       *string `json:"instructions"`
	LinkURL            *string `json:"link_url"`
	DueAt              string  `json:"due_at"`
	MaxScore           float64 `json:"max_score"`
	FileName           *string `json:"file_name"`
	FileMIME           *string `json:"file_mime"`
	FileSize           *int64  `json:"file_size"`
	Status             string  `json:"status"`
	PublishedAt        *string `json:"published_at"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
	// Guru/admin view: roster progress.
	StudentCount   int `json:"student_count"`
	SubmittedCount int `json:"submitted_count"`
	GradedCount    int `json:"graded_count"`
	// Siswa view: the caller's own submission, if any.
	MySubmission *learningSubmission `json:"my_submission,omitempty"`
	fileKey      *string
}

const tsFmt = `'YYYY-MM-DD"T"HH24:MI:SS"Z"'`

// $1 = school_id, $2 = caller user id (for my_submission; '' for staff).
const assignmentSelect = `
	SELECT a.id, a.classroom_subject_id, cs.classroom_id, c.name, cs.subject_id, sub.name, a.teacher_user_id,
	       a.title, a.instructions, a.link_url, to_char(a.due_at AT TIME ZONE 'UTC', ` + tsFmt + `), a.max_score::float8,
	       a.file_name, a.file_mime, a.file_size, a.status,
	       to_char(a.published_at AT TIME ZONE 'UTC', ` + tsFmt + `),
	       to_char(a.created_at AT TIME ZONE 'UTC', ` + tsFmt + `),
	       to_char(a.updated_at AT TIME ZONE 'UTC', ` + tsFmt + `),
	       a.file_key,
	       (SELECT count(*) FROM class_enrollments e WHERE e.classroom_id = cs.classroom_id AND e.status = 'active'),
	       (SELECT count(*) FROM learning_submissions s WHERE s.assignment_id = a.id),
	       (SELECT count(*) FROM learning_submissions s WHERE s.assignment_id = a.id AND s.status = 'graded'),
	       ms.id, ms.student_user_id, ms.answer_text, ms.file_name, ms.file_size,
	       to_char(ms.submitted_at AT TIME ZONE 'UTC', ` + tsFmt + `), ms.is_late, ms.status, ms.resubmit_count,
	       ms.score::float8, ms.feedback, to_char(ms.graded_at AT TIME ZONE 'UTC', ` + tsFmt + `)
	FROM learning_assignments a
	JOIN classroom_subjects cs ON cs.id = a.classroom_subject_id
	JOIN classrooms c ON c.id = cs.classroom_id
	JOIN subjects sub ON sub.id = cs.subject_id
	LEFT JOIN learning_submissions ms ON ms.assignment_id = a.id AND ms.student_user_id::text = $2
	WHERE a.deleted_at IS NULL AND a.school_id::text = $1`

func scanAssignment(row interface{ Scan(...any) error }) (*learningAssignment, error) {
	var a learningAssignment
	var sID, sStudent, sSubmittedAt, sStatus *string
	var sLate *bool
	var sCount *int
	var s learningSubmission
	err := row.Scan(&a.ID, &a.ClassroomSubjectID, &a.ClassroomID, &a.ClassroomName, &a.SubjectID, &a.SubjectName,
		&a.TeacherUserID, &a.Title, &a.Instructions, &a.LinkURL, &a.DueAt, &a.MaxScore,
		&a.FileName, &a.FileMIME, &a.FileSize, &a.Status, &a.PublishedAt, &a.CreatedAt, &a.UpdatedAt, &a.fileKey,
		&a.StudentCount, &a.SubmittedCount, &a.GradedCount,
		&sID, &sStudent, &s.AnswerText, &s.FileName, &s.FileSize, &sSubmittedAt, &sLate, &sStatus, &sCount,
		&s.Score, &s.Feedback, &s.GradedAt)
	if err != nil {
		return nil, err
	}
	if sID != nil {
		s.ID, s.StudentUserID, s.SubmittedAt, s.IsLate, s.Status, s.ResubmitCount = *sID, *sStudent, *sSubmittedAt, *sLate, *sStatus, *sCount
		a.MySubmission = &s
	}
	return &a, nil
}

func (h *Handler) getAssignment(ctx context.Context, schoolID, id, callerID string) (*learningAssignment, error) {
	a, err := scanAssignment(h.Store.DB.QueryRowContext(ctx, assignmentSelect+` AND a.id::text = $3`, schoolID, callerID, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func siswaCallerID(c *common.Claims) string {
	if c != nil && c.ActiveRole == common.RoleSiswa {
		return c.UserID
	}
	return ""
}

// ListLearningAssignments: GET /learning/assignments?classroom_subject_id=&academic_year_id=
func (h *Handler) ListLearningAssignments(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	where, args, ok := h.learningListScope(w, r, schoolID, "a", []any{schoolID, siswaCallerID(common.FromCtx(r))})
	if !ok {
		return
	}
	rows, err := h.Store.DB.QueryContext(r.Context(), assignmentSelect+where+` ORDER BY a.due_at DESC`, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []learningAssignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, *a)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func parseDueAt(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func parseMaxScore(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 100, true
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil && v > 0 && v <= 1000
}

// CreateLearningAssignment: POST /learning/assignments (multipart:
// classroom_subject_id, title, instructions, link_url, due_at RFC3339,
// max_score, status, file).
func (h *Handler) CreateLearningAssignment(w http.ResponseWriter, r *http.Request) {
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
	due, okDue := parseDueAt(r.FormValue("due_at"))
	maxScore, okMax := parseMaxScore(r.FormValue("max_score"))
	if title == "" || !okStatus || !validLink(link) || !okDue || !okMax {
		common.WriteError(w, http.StatusBadRequest, "judul & tenggat wajib diisi, nilai maksimal 1-1000, tautan harus http(s)")
		return
	}
	file, err := saveLearningUpload(r, "file", schoolID, "assignments")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
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
		INSERT INTO learning_assignments (school_id, academic_year_id, classroom_subject_id, teacher_user_id, title, instructions, link_url,
		       due_at, max_score, file_key, file_name, file_mime, file_size, status, published_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14, CASE WHEN $14 = 'published' THEN now() END)
		RETURNING id`,
		cs.SchoolID, cs.AcademicYearID, cs.ID, teacher, title, nullIfEmpty(r.FormValue("instructions")), link,
		due, maxScore, fk, fn, fm, fs, status).Scan(&id)
	if err != nil {
		removeLearningFile(fk)
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a, _ := h.getAssignment(r.Context(), schoolID, id, "")
	common.WriteJSON(w, http.StatusCreated, a, nil)
}

func (h *Handler) loadManagedAssignment(w http.ResponseWriter, r *http.Request, schoolID, id string) (*learningAssignment, bool) {
	a, err := h.getAssignment(r.Context(), schoolID, id, "")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if a == nil {
		common.WriteError(w, http.StatusNotFound, "tugas tidak ditemukan")
		return nil, false
	}
	cs, err := h.getLearningCS(r.Context(), schoolID, a.ClassroomSubjectID)
	if err != nil || cs == nil || !canManageCS(common.FromCtx(r), cs) {
		common.WriteError(w, http.StatusForbidden, "anda bukan guru pengampu tugas ini")
		return nil, false
	}
	return a, true
}

// UpdateLearningAssignment: PUT /learning/assignments/{id} (same fields as
// create, plus remove_file=1).
func (h *Handler) UpdateLearningAssignment(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok || !parseLearningForm(w, r) {
		return
	}
	a, ok := h.loadManagedAssignment(w, r, schoolID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	link := nullIfEmpty(r.FormValue("link_url"))
	status, okStatus := formStatus(r, a.Status)
	due, okDue := parseDueAt(r.FormValue("due_at"))
	maxScore, okMax := parseMaxScore(r.FormValue("max_score"))
	if title == "" || !okStatus || !validLink(link) || !okDue || !okMax {
		common.WriteError(w, http.StatusBadRequest, "judul & tenggat wajib diisi, nilai maksimal 1-1000, tautan harus http(s)")
		return
	}
	file, err := saveLearningUpload(r, "file", schoolID, "assignments")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	fk, fn, fm, fs := a.fileKey, a.FileName, a.FileMIME, a.FileSize
	oldKey, replaced := a.fileKey, false
	if file != nil {
		fk, fn, fm, fs, replaced = &file.Key, &file.Name, &file.MIME, &file.Size, true
	} else if r.FormValue("remove_file") == "1" {
		fk, fn, fm, fs, replaced = nil, nil, nil, nil, true
	}
	_, err = h.Store.DB.ExecContext(r.Context(), `
		UPDATE learning_assignments SET title=$2, instructions=$3, link_url=$4, due_at=$5, max_score=$6,
		       file_key=$7, file_name=$8, file_mime=$9, file_size=$10,
		       status=$11, published_at = CASE WHEN $11 = 'published' THEN COALESCE(published_at, now()) END, updated_at=now()
		WHERE id::text=$1`,
		a.ID, title, nullIfEmpty(r.FormValue("instructions")), link, due, maxScore, fk, fn, fm, fs, status)
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
	out, _ := h.getAssignment(r.Context(), schoolID, a.ID, "")
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// DeleteLearningAssignment: DELETE /learning/assignments/{id}. Soft delete;
// submissions stay in the database (graded work is not destroyed).
func (h *Handler) DeleteLearningAssignment(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	a, ok := h.loadManagedAssignment(w, r, schoolID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if _, err := h.Store.DB.ExecContext(r.Context(), `UPDATE learning_assignments SET deleted_at=now() WHERE id::text=$1`, a.ID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// DownloadLearningAssignmentFile: GET /learning/assignments/{id}/file
func (h *Handler) DownloadLearningAssignmentFile(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var csID, status, ayID string
	var key, name, mt *string
	err := h.Store.DB.QueryRowContext(r.Context(), `
		SELECT classroom_subject_id, status, academic_year_id, file_key, file_name, file_mime
		FROM learning_assignments WHERE id::text=$1 AND school_id::text=$2 AND deleted_at IS NULL`,
		chi.URLParam(r, "id"), schoolID).Scan(&csID, &status, &ayID, &key, &name, &mt)
	if err != nil || !h.canReadCSItem(w, r, schoolID, csID, status, ayID) {
		common.WriteError(w, http.StatusNotFound, "tugas tidak ditemukan")
		return
	}
	serveLearningFile(w, key, name, mt)
}

// submissionRosterRow: one enrolled student of the classroom, with their
// submission if they handed one in.
type submissionRosterRow struct {
	StudentUserID string              `json:"student_user_id"`
	Submission    *learningSubmission `json:"submission"`
}

// ListAssignmentSubmissions: GET /learning/assignments/{id}/submissions
// (guru pengampu / admin). Returns the full class roster so "belum
// mengumpulkan" is visible, plus submissions of students no longer enrolled.
func (h *Handler) ListAssignmentSubmissions(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	a, err := h.getAssignment(r.Context(), schoolID, chi.URLParam(r, "id"), "")
	if err != nil || a == nil {
		common.WriteError(w, http.StatusNotFound, "tugas tidak ditemukan")
		return
	}
	c := common.FromCtx(r)
	if !isLearningStaffReader(c) {
		cs, err := h.getLearningCS(r.Context(), schoolID, a.ClassroomSubjectID)
		if err != nil || cs == nil || !canManageCS(c, cs) {
			common.WriteError(w, http.StatusForbidden, "anda bukan guru pengampu tugas ini")
			return
		}
	}
	rows, err := h.Store.DB.QueryContext(r.Context(), `
		WITH roster AS (
			SELECT student_user_id FROM class_enrollments WHERE classroom_id::text = $2 AND status = 'active'
			UNION
			SELECT student_user_id FROM learning_submissions WHERE assignment_id::text = $1
		)
		SELECT ro.student_user_id, s.id, s.answer_text, s.file_name, s.file_size,
		       to_char(s.submitted_at AT TIME ZONE 'UTC', `+tsFmt+`), s.is_late, s.status, s.resubmit_count,
		       s.score::float8, s.feedback, to_char(s.graded_at AT TIME ZONE 'UTC', `+tsFmt+`)
		FROM roster ro
		LEFT JOIN learning_submissions s ON s.assignment_id::text = $1 AND s.student_user_id = ro.student_user_id
		ORDER BY s.submitted_at NULLS LAST`, a.ID, a.ClassroomID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []submissionRosterRow{}
	for rows.Next() {
		var row submissionRosterRow
		var s learningSubmission
		var sID, sAt, sStatus *string
		var sLate *bool
		var sCount *int
		if err := rows.Scan(&row.StudentUserID, &sID, &s.AnswerText, &s.FileName, &s.FileSize, &sAt, &sLate, &sStatus, &sCount,
			&s.Score, &s.Feedback, &s.GradedAt); err != nil {
			common.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if sID != nil {
			s.ID, s.StudentUserID, s.SubmittedAt, s.IsLate, s.Status, s.ResubmitCount = *sID, row.StudentUserID, *sAt, *sLate, *sStatus, *sCount
			row.Submission = &s
		}
		out = append(out, row)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// SubmitAssignment: POST /learning/assignments/{id}/submission (siswa;
// multipart answer_text + file). Accepted after the deadline too, flagged
// is_late (AC-58-01). A graded submission is locked unless the guru asked
// for a resubmission.
func (h *Handler) SubmitAssignment(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	c := common.FromCtx(r)
	if c.ActiveRole != common.RoleSiswa {
		common.WriteError(w, http.StatusForbidden, "hanya siswa yang dapat mengumpulkan tugas")
		return
	}
	if !parseLearningForm(w, r) {
		return
	}
	a, err := h.getAssignment(r.Context(), schoolID, chi.URLParam(r, "id"), c.UserID)
	if err != nil || a == nil || a.Status != "published" {
		common.WriteError(w, http.StatusNotFound, "tugas tidak ditemukan")
		return
	}
	ayID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	ids, err := h.studentCSIDs(r.Context(), schoolID, c.UserID, ayID)
	if err != nil || !ids[a.ClassroomSubjectID] {
		common.WriteError(w, http.StatusNotFound, "tugas tidak ditemukan")
		return
	}
	if a.MySubmission != nil && a.MySubmission.Status == "graded" {
		common.WriteError(w, http.StatusConflict, "tugas sudah dinilai; pengumpulan ulang hanya jika diminta guru")
		return
	}
	answer := nullIfEmpty(r.FormValue("answer_text"))
	file, err := saveLearningUpload(r, "file", schoolID, "submissions")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if file == nil && answer == nil {
		common.WriteError(w, http.StatusBadRequest, "isi jawaban atau unggah file")
		return
	}
	due, _ := time.Parse(time.RFC3339, a.DueAt)
	late := time.Now().After(due)
	var fk, fn, fm *string
	var fs *int64
	if file != nil {
		fk, fn, fm, fs = &file.Key, &file.Name, &file.MIME, &file.Size
	}
	var oldKey *string
	_ = h.Store.DB.QueryRowContext(r.Context(), `SELECT file_key FROM learning_submissions WHERE assignment_id::text=$1 AND student_user_id::text=$2`, a.ID, c.UserID).Scan(&oldKey)
	_, err = h.Store.DB.ExecContext(r.Context(), `
		INSERT INTO learning_submissions (school_id, assignment_id, student_user_id, answer_text, file_key, file_name, file_mime, file_size, is_late)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (assignment_id, student_user_id) DO UPDATE SET
		  answer_text=EXCLUDED.answer_text, file_key=EXCLUDED.file_key, file_name=EXCLUDED.file_name,
		  file_mime=EXCLUDED.file_mime, file_size=EXCLUDED.file_size, is_late=EXCLUDED.is_late,
		  submitted_at=now(), status='submitted', resubmit_count=learning_submissions.resubmit_count+1,
		  score=NULL, graded_at=NULL, graded_by=NULL, updated_at=now()
		WHERE learning_submissions.status <> 'graded'`,
		schoolID, a.ID, c.UserID, answer, fk, fn, fm, fs, late)
	if err != nil {
		removeLearningFile(fk)
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if oldKey != nil && (fk == nil || *oldKey != *fk) {
		removeLearningFile(oldKey)
	}
	out, _ := h.getAssignment(r.Context(), schoolID, a.ID, c.UserID)
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// DownloadSubmissionFile: GET /learning/submissions/{id}/file (the owning
// siswa, the guru pengampu, or staff readers).
func (h *Handler) DownloadSubmissionFile(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var studentID, csID string
	var key, name, mt *string
	err := h.Store.DB.QueryRowContext(r.Context(), `
		SELECT s.student_user_id, a.classroom_subject_id, s.file_key, s.file_name, s.file_mime
		FROM learning_submissions s JOIN learning_assignments a ON a.id = s.assignment_id
		WHERE s.id::text=$1 AND s.school_id::text=$2`, chi.URLParam(r, "id"), schoolID).
		Scan(&studentID, &csID, &key, &name, &mt)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "pengumpulan tidak ditemukan")
		return
	}
	c := common.FromCtx(r)
	allowed := isLearningStaffReader(c) || (c.ActiveRole == common.RoleSiswa && c.UserID == studentID)
	if !allowed && c.ActiveRole != common.RoleSiswa {
		cs, err := h.getLearningCS(r.Context(), schoolID, csID)
		allowed = err == nil && cs != nil && canManageCS(c, cs)
	}
	if !allowed {
		common.WriteError(w, http.StatusNotFound, "pengumpulan tidak ditemukan")
		return
	}
	serveLearningFile(w, key, name, mt)
}

type gradeSubmissionRequest struct {
	Score           *float64 `json:"score"`
	Feedback        *string  `json:"feedback"`
	RequestResubmit bool     `json:"request_resubmit"`
}

// GradeSubmission: POST /learning/submissions/{id}/grade (guru pengampu /
// admin). Score must be within 0..max_score; request_resubmit reopens the
// submission for the siswa instead of grading it.
func (h *Handler) GradeSubmission(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var body gradeSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var assignmentID string
	err := h.Store.DB.QueryRowContext(r.Context(), `SELECT assignment_id FROM learning_submissions WHERE id::text=$1 AND school_id::text=$2`,
		chi.URLParam(r, "id"), schoolID).Scan(&assignmentID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "pengumpulan tidak ditemukan")
		return
	}
	a, ok := h.loadManagedAssignment(w, r, schoolID, assignmentID)
	if !ok {
		return
	}
	feedback := (*string)(nil)
	if body.Feedback != nil {
		feedback = nullIfEmpty(*body.Feedback)
	}
	c := common.FromCtx(r)
	if body.RequestResubmit {
		_, err = h.Store.DB.ExecContext(r.Context(), `
			UPDATE learning_submissions SET status='resubmit_requested', score=NULL, feedback=$2, graded_at=now(), graded_by=$3, updated_at=now()
			WHERE id::text=$1`, chi.URLParam(r, "id"), feedback, c.UserID)
	} else {
		if body.Score == nil || *body.Score < 0 || *body.Score > a.MaxScore {
			common.WriteError(w, http.StatusBadRequest, "nilai harus di antara 0 dan "+strconv.FormatFloat(a.MaxScore, 'f', -1, 64))
			return
		}
		_, err = h.Store.DB.ExecContext(r.Context(), `
			UPDATE learning_submissions SET status='graded', score=$2, feedback=$3, graded_at=now(), graded_by=$4, updated_at=now()
			WHERE id::text=$1`, chi.URLParam(r, "id"), *body.Score, feedback, c.UserID)
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}
