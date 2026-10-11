package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
	"skci/exam/internal/store"
)

// classroomSubjectPair is one checked "{Subject} - {Class}" row in Buat
// Ujian Baru's Mata Pelajaran picker (item 3): a guru can tick several,
// fanning out to one exam per pair in a single CreateExam call (see
// ClassroomAssignments below) since the exams table only carries a single
// classroom_id/subject_id per row.
type classroomSubjectPair struct {
	ClassroomID string `json:"classroom_id"`
	SubjectID   string `json:"subject_id"`
}

type examReq struct {
	AcademicYearID string   `json:"academic_year_id"`
	TermID         string   `json:"term_id"`       // Semester/Tahun Ajaran
	ExamTypeID     string   `json:"exam_type_id"`  // Tipe Ujian
	Title          string   `json:"title"`         // Judul Ujian
	Description    *string  `json:"description"`   // Deskripsi/catatan (optional)
	ClassroomID    string   `json:"classroom_id"`  // Mata Pelajaran: kelas (single-classroom path)
	ClassroomIDs   []string `json:"classroom_ids"` // for multi select update
	SubjectID      string   `json:"subject_id"`    // Mata Pelajaran: mapel (single-classroom path)
	// ClassroomAssignments: when set (guru's multi-select Mata Pelajaran
	// checkboxes), ClassroomID/SubjectID above are ignored and one exam is
	// created per pair instead — see CreateExam.
	ClassroomAssignments []classroomSubjectPair `json:"classroom_assignments,omitempty"`
	TeacherID            string                 `json:"teacher_id"`  // admin assigns explicitly; guru defaults to self
	StartAt              string                 `json:"start_at"`    // RFC3339, Tanggal Mulai
	EndAt                string                 `json:"end_at"`      // RFC3339, Tanggal Selesai
	IsShuffled           bool                   `json:"is_shuffled"` // Acak Soal
	FolderID             *string                `json:"folder_id"`   // Bank Soal folder this exam was created from, if any
}

// validateExamReq enforces the spec's "ALL mandatory" header fields, the
// no-past-date rule, and (for guru) that they can only assign themselves as
// teacher. See README Assumptions re: trusting classroom_id/subject_id
// straight from the JWT+payload instead of round-tripping academic-service.
// skipClassroomSubject is set by CreateExam's multi-classroom path (item 3),
// where classroom_id/subject_id come from ClassroomAssignments instead.
func (h *Handler) validateExamReq(req examReq, c *common.Claims, skipClassroomSubject bool) (string, bool, time.Time, time.Time) {
	if req.AcademicYearID == "" || req.TermID == "" || req.ExamTypeID == "" || req.Title == "" ||
		(!skipClassroomSubject && ((req.ClassroomID == "" && len(req.ClassroomIDs) == 0) || req.SubjectID == "")) {
		return "academic_year_id, term_id, exam_type_id, title, classroom_id and subject_id are all mandatory", false, time.Time{}, time.Time{}
	}
	start, err := time.Parse(time.RFC3339, req.StartAt)
	if err != nil {
		return "start_at must be RFC3339", false, time.Time{}, time.Time{}
	}
	end, err := time.Parse(time.RFC3339, req.EndAt)
	if err != nil {
		return "end_at must be RFC3339", false, time.Time{}, time.Time{}
	}
	now := time.Now()
	// start_at can be in the past as long as end_at is in the future
	if end.Before(now) {
		return "end_at must not be in the past", false, time.Time{}, time.Time{}
	}
	if end.Before(start) {
		return "end_at must not be before start_at", false, time.Time{}, time.Time{}
	}
	if c.ActiveRole == "guru" && req.TeacherID != "" && req.TeacherID != c.UserID {
		return "guru may only create exams for themselves as teacher_id", false, time.Time{}, time.Time{}
	}
	return "", true, start, end
}

func (h *Handler) CreateExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru", "pengawas") {
		return
	}
	var req examReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	teacherID := req.TeacherID
	if c.ActiveRole == "guru" || teacherID == "" {
		teacherID = c.UserID
	}
	req.TeacherID = teacherID

	// Item 3: guru's multi-select Mata Pelajaran checkboxes. An exam is
	// fundamentally scoped to ONE subject (one shared question set), so
	// pairs are grouped by subject_id — same-subject pairs across several
	// classrooms (e.g. "Agama Islam" ticked for both 1-A and 1-B) become a
	// SINGLE exam related to all those classrooms via exam_classrooms
	// (migration 0005), not one duplicate exam per classroom. A genuinely
	// different subject_id still starts its own exam, since two subjects
	// can't share one question set.
	if len(req.ClassroomAssignments) > 0 {
		for _, pair := range req.ClassroomAssignments {
			if pair.ClassroomID == "" || pair.SubjectID == "" {
				common.WriteError(w, http.StatusBadRequest, "each classroom_assignments entry requires classroom_id and subject_id")
				return
			}
		}
		msg, ok, start, end := h.validateExamReq(req, c, true)
		if !ok {
			common.WriteError(w, http.StatusBadRequest, msg)
			return
		}
		schoolID := common.EffectiveSchoolID(r, c)

		bySubject := make(map[string][]string) // subject_id -> classroom_ids, insertion order
		var subjectOrder []string
		for _, pair := range req.ClassroomAssignments {
			if _, ok := bySubject[pair.SubjectID]; !ok {
				subjectOrder = append(subjectOrder, pair.SubjectID)
			}
			bySubject[pair.SubjectID] = append(bySubject[pair.SubjectID], pair.ClassroomID)
		}

		created := make([]*model.Exam, 0, len(subjectOrder))
		for _, subjectID := range subjectOrder {
			classroomIDs := bySubject[subjectID]
			e := &model.Exam{
				SchoolID:       schoolID,
				AcademicYearID: req.AcademicYearID,
				TermID:         req.TermID,
				ExamTypeID:     req.ExamTypeID,
				Title:          req.Title,
				Description:    req.Description,
				ClassroomID:    classroomIDs[0],
				ClassroomIDs:   classroomIDs,
				SubjectID:      subjectID,
				TeacherID:      teacherID,
				StartAt:        start,
				EndAt:          end,
				IsShuffled:     req.IsShuffled,
				CreatedBy:      c.UserID,
				FolderID:       req.FolderID,
			}
			if err := h.Store.CreateExam(r.Context(), e); err != nil {
				writeStoreErr(w, err)
				return
			}
			created = append(created, e)
		}
		common.WriteJSON(w, http.StatusCreated, created, nil)
		return
	}

	msg, ok, start, end := h.validateExamReq(req, c, false)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	e := &model.Exam{
		SchoolID:       common.EffectiveSchoolID(r, c),
		AcademicYearID: req.AcademicYearID,
		TermID:         req.TermID,
		ExamTypeID:     req.ExamTypeID,
		Title:          req.Title,
		Description:    req.Description,
		ClassroomID:    req.ClassroomID,
		ClassroomIDs:   req.ClassroomIDs,
		SubjectID:      req.SubjectID,
		TeacherID:      teacherID,
		StartAt:        start,
		EndAt:          end,
		IsShuffled:     req.IsShuffled,
		CreatedBy:      c.UserID,
		FolderID:       req.FolderID,
	}
	if err := h.Store.CreateExam(r.Context(), e); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, e, nil)
}

func (h *Handler) ListExams(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	f := store.ExamFilter{
		SchoolID:       common.EffectiveSchoolID(r, c),
		AcademicYearID: r.URL.Query().Get("academic_year_id"),
		TermID:         r.URL.Query().Get("term_id"),
		ClassroomID:    r.URL.Query().Get("classroom_id"),
		FolderID:       r.URL.Query().Get("folder_id"),
		Title:          r.URL.Query().Get("title"),
	}
	if c != nil && (r.URL.Query().Get("mine") == "true" || c.ActiveRole == "guru") {
		f.TeacherID = c.UserID
	}
	if v := r.URL.Query().Get("is_published"); v != "" {
		b := v == "true"
		f.IsPublished = &b
	}
	page, perPage := common.Pagination(r)
	list, total, err := h.Store.ListExams(r.Context(), f, page, perPage)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, common.Meta{Page: page, PerPage: perPage, Total: total})
}

func (h *Handler) GetExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	e, err := h.Store.GetExam(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if e == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	questions, err := h.Store.ListExamQuestions(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	// has_attempts: Bug Report SKCIXGPM 0.1 -- ExamDetailView uses this to
	// decide whether question-editing controls stay visible on a published
	// exam (locked again once any student has entered, see HasAnyAttempt).
	hasAttempts, err := h.Store.HasAnyAttempt(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"exam": e, "questions": questions, "has_attempts": hasAttempts}, nil)
}

// changedBesidesSchedule names the non-schedule fields an update would alter,
// so a published-exam edit can be refused with the reason instead of a bare
// "not editable" (Yadika13 item 2). Empty result = schedule-only edit.
// Description is treated as schedule-adjacent: it carries no state a running
// attempt depends on, and guru use it to note the new time.
func changedBesidesSchedule(existing *model.Exam, req examReq) string {
	var changed []string
	cmp := func(name, was, now string) {
		if now != "" && now != was {
			changed = append(changed, name)
		}
	}
	cmp("Judul", existing.Title, req.Title)
	cmp("Semester/Term", existing.TermID, req.TermID)
	cmp("Tipe Ujian", existing.ExamTypeID, req.ExamTypeID)
	cmp("Mata Pelajaran", existing.SubjectID, req.SubjectID)
	cmp("Guru", existing.TeacherID, req.TeacherID)
	if req.IsShuffled != existing.IsShuffled {
		changed = append(changed, "Acak Soal")
	}
	// Kelas: compare as sets — the request may send the same classrooms in a
	// different order, which is not a change.
	want := req.ClassroomIDs
	if len(want) == 0 && req.ClassroomID != "" {
		want = []string{req.ClassroomID}
	}
	if len(want) > 0 {
		have := map[string]bool{}
		for _, id := range existing.ClassroomIDs {
			have[id] = true
		}
		if len(want) != len(existing.ClassroomIDs) {
			changed = append(changed, "Kelas")
		} else {
			for _, id := range want {
				if !have[id] {
					changed = append(changed, "Kelas")
					break
				}
			}
		}
	}
	return strings.Join(changed, ", ")
}

func (h *Handler) UpdateExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru", "pengawas") {
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.Store.GetExam(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if existing == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && existing.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only edit your own exams")
		return
	}
	var req examReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.TeacherID == "" {
		req.TeacherID = existing.TeacherID
	}
	msg, ok, start, end := h.validateExamReq(req, c, false)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	// Yadika13 item 2: a published exam -- including one running right now --
	// stays editable, but only its schedule. Everything else is frozen
	// because students may already hold a shuffled question_order built from
	// the current question set and class list. Say so explicitly rather than
	// silently dropping the other fields, so the guru knows what was saved.
	if existing.IsPublished {
		if changed := changedBesidesSchedule(existing, req); changed != "" {
			common.WriteError(w, http.StatusConflict,
				"ujian sudah diterbitkan: hanya Tanggal Mulai/Selesai yang bisa diubah (perubahan ditolak pada: "+changed+")")
			return
		}
		if err := h.Store.UpdateExamSchedule(r.Context(), id, existing.SchoolID, start, end); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				common.WriteError(w, http.StatusNotFound, "exam not found")
				return
			}
			writeStoreErr(w, err)
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
		return
	}

	e := &model.Exam{
		ID: id, SchoolID: existing.SchoolID,
		TermID: req.TermID, ExamTypeID: req.ExamTypeID, Title: req.Title, Description: req.Description,
		ClassroomID: req.ClassroomID, ClassroomIDs: req.ClassroomIDs, SubjectID: req.SubjectID, TeacherID: req.TeacherID,
		StartAt: start, EndAt: end, IsShuffled: req.IsShuffled,
	}
	if err := h.Store.UpdateExam(r.Context(), e); err != nil {
		if errors.Is(err, store.ErrNotEditable) {
			common.WriteError(w, http.StatusConflict, "exam is published and no longer editable")
			return
		}
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) DeleteExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru", "pengawas") {
		return
	}
	id := chi.URLParam(r, "id")
	// Item 3 (V4): Hapus is newly wired up on the frontend (Kelola Ujian /
	// Bank Soal folder rows) — match the same guru-may-only-touch-their-own
	// ownership check already enforced on Update/Publish/Duplicate above.
	existing, err := h.Store.GetExam(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if existing == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && existing.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only delete your own exams")
		return
	}
	if err := h.Store.DeleteExam(r.Context(), id, common.EffectiveSchoolID(r, c)); err != nil {
		if errors.Is(err, store.ErrHasAttempts) {
			common.WriteError(w, http.StatusConflict, "exam already has student attempts and cannot be deleted")
			return
		}
		if errors.Is(err, store.ErrNotEditable) {
			common.WriteError(w, http.StatusNotFound, "exam not found")
			return
		}
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, nil, nil)
}

type addExamQuestionReq struct {
	QuestionID string  `json:"question_id"`
	Poin       float64 `json:"poin"` // optional override of the bank-soal question's raw poin for this exam
}

func (h *Handler) AddExamQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	// Bug Report SKCIXGPM 0.1: guru can keep adding questions to a published
	// exam ("input soal menyicil") -- only locked once a student has
	// actually entered it (see HasAnyAttempt's doc comment for why).
	if exam.IsPublished {
		hasAttempt, err := h.Store.HasAnyAttempt(r.Context(), examID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		if hasAttempt {
			common.WriteError(w, http.StatusConflict, "cannot modify questions: siswa sudah mulai mengerjakan ujian ini")
			return
		}
	}
	var req addExamQuestionReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	q, err := h.Store.GetQuestion(r.Context(), req.QuestionID, exam.SchoolID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if q == nil {
		common.WriteError(w, http.StatusNotFound, "question not found")
		return
	}
	poin := req.Poin
	if poin <= 0 {
		poin = q.Poin
	}
	eq, err := h.Store.AddExamQuestion(r.Context(), examID, req.QuestionID, poin)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam.IsPublished {
		if err := h.Store.RenormalizePublishedPoints(r.Context(), examID); err != nil {
			common.WriteJSON(w, http.StatusCreated, eq, map[string]interface{}{
				"warning": "soal ditambahkan, tapi gagal menyesuaikan ulang poin otomatis: " + err.Error(),
			})
			return
		}
	}
	common.WriteJSON(w, http.StatusCreated, eq, nil)
}

func (h *Handler) RemoveExamQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if exam.IsPublished {
		hasAttempt, err := h.Store.HasAnyAttempt(r.Context(), examID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		if hasAttempt {
			common.WriteError(w, http.StatusConflict, "cannot modify questions: siswa sudah mulai mengerjakan ujian ini")
			return
		}
	}
	eqID := chi.URLParam(r, "eqId")
	if err := h.Store.RemoveExamQuestion(r.Context(), examID, eqID); err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam.IsPublished {
		if err := h.Store.RenormalizePublishedPoints(r.Context(), examID); err != nil {
			common.WriteJSON(w, http.StatusOK, nil, map[string]interface{}{
				"warning": "soal dihapus, tapi gagal menyesuaikan ulang poin otomatis: " + err.Error(),
			})
			return
		}
	}
	common.WriteJSON(w, http.StatusOK, nil, nil)
}

type updateExamQuestionPointsReq struct {
	Points float64 `json:"points"`
}

// UpdateExamQuestionPoints syncs this exam's own point snapshot for a
// question (exam_questions.points) when the guru edits a soal's poin from
// inside an exam's question list -- editing the question alone only ever
// touched the bank-wide questions.poin, leaving this exam's copy (what's
// actually shown/summed for it) stale.
func (h *Handler) UpdateExamQuestionPoints(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if exam.IsPublished {
		hasAttempt, err := h.Store.HasAnyAttempt(r.Context(), examID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		if hasAttempt {
			common.WriteError(w, http.StatusConflict, "cannot modify questions: siswa sudah mulai mengerjakan ujian ini")
			return
		}
	}
	var req updateExamQuestionPointsReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.Points <= 0 {
		common.WriteError(w, http.StatusBadRequest, "points harus lebih dari 0")
		return
	}
	eqID := chi.URLParam(r, "eqId")
	if err := h.Store.UpdateExamQuestionPoints(r.Context(), examID, eqID, req.Points); err != nil {
		if err == sql.ErrNoRows {
			common.WriteError(w, http.StatusNotFound, "soal tidak ditemukan di ujian ini")
			return
		}
		writeStoreErr(w, err)
		return
	}
	if exam.IsPublished {
		if err := h.Store.RenormalizePublishedPoints(r.Context(), examID); err != nil {
			common.WriteJSON(w, http.StatusOK, nil, map[string]interface{}{
				"warning": "poin diperbarui, tapi gagal menyesuaikan ulang poin otomatis: " + err.Error(),
			})
			return
		}
	}
	common.WriteJSON(w, http.StatusOK, nil, nil)
}

// PublishExam normalizes points to sum to 100 (see README "Score
// normalization") then flips the exam to published/live.
func (h *Handler) PublishExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru", "pengawas") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only publish your own exams")
		return
	}
	if err := h.Store.NormalizeAndPublish(r.Context(), examID, exam.SchoolID); err != nil {
		if errors.Is(err, store.ErrNotEditable) {
			common.WriteError(w, http.StatusConflict, "exam not found or already published")
			return
		}
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, _ := h.Store.GetExam(r.Context(), examID, exam.SchoolID)
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// UnpublishExam: POST /exams/{id}/unpublish — reverts a published exam back
// to draft. Sekoci revamp V1.docx: allowed only while the exam's own
// scheduled window hasn't started yet (once it has, students may already be
// mid-attempt, so unpublishing would pull the exam out from under them).
func (h *Handler) UnpublishExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru", "pengawas") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only unpublish your own exams")
		return
	}
	if !time.Now().Before(exam.StartAt) {
		common.WriteError(w, http.StatusConflict, "tidak dapat membatalkan publikasi: waktu ujian sudah berlangsung atau berakhir")
		return
	}
	if err := h.Store.UnpublishExam(r.Context(), examID, exam.SchoolID); err != nil {
		if errors.Is(err, store.ErrNotEditable) {
			common.WriteError(w, http.StatusConflict, "exam not found or belum dipublish")
			return
		}
		writeStoreErr(w, err)
		return
	}
	updated, _ := h.Store.GetExam(r.Context(), examID, exam.SchoolID)
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// NormalizeDraftPoints: POST /exams/{id}/normalize-points — the "Simpan
// sebagai Draft" action. Re-distributes 100 points evenly across the exam's
// current questions (see Store.NormalizeDraftPoints) WITHOUT publishing, so
// a guru can save a draft, add/remove questions, and save again with points
// always summing to exactly 100.
func (h *Handler) NormalizeDraftPoints(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only save your own exams as draft")
		return
	}
	if err := h.Store.NormalizeDraftPoints(r.Context(), examID, exam.SchoolID); err != nil {
		if errors.Is(err, store.ErrNotEditable) {
			common.WriteError(w, http.StatusConflict, "exam not found or already published")
			return
		}
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, _ := h.Store.GetExam(r.Context(), examID, exam.SchoolID)
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// DuplicateExam creates a fresh, unpublished copy of an exam (settings +
// question associations) for reuse. Tokens/attempts are never copied — see
// Store.DuplicateExam.
func (h *Handler) DuplicateExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru", "pengawas") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only duplicate your own exams")
		return
	}
	dup, err := h.Store.DuplicateExam(r.Context(), examID, exam.SchoolID, c.UserID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, dup, nil)
}
