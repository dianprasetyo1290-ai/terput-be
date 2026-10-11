package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
)

// roleOrangTua is the identity active_role the Portal Orang Tua (parent SPA)
// carries in its JWT. The three rules for every handler in this file:
//
//  1. JWT-authenticated AS a wali (requireRole(roleOrangTua)), then narrowed
//     to ONE child by identity-service's guardian<->student link check.
//  2. Read-only — this file registers no POST/PUT/PATCH/DELETE, and the store
//     methods it calls are pure SELECTs, so a wali can never mutate an exam.
//  3. Academic students only — the store filters audience='academic' AND
//     participant_type='student', so a PPDB/admission applicant (audience
//     'admission', user_id NULL) can never surface through here.
const roleOrangTua = "orang_tua"

// requireGuardianAccess authorizes the caller to read studentUserID's data.
//
// guardianUserId is the JWT subject and school_id the JWT's school — exactly
// the pair identity-service keys guardian_links by (see identity migration
// 0013_parent_portal). Any failure is a denial:
//
//   - caller not active_role=orang_tua           -> 403 (requireRole)
//   - identity says 404 / not linked              -> 403
//   - identity unreachable or not configured      -> 503 (fail CLOSED)
//
// The 503 (not an allow) on an unreachable identity is the whole point: a
// guardian endpoint must never turn "couldn't check" into "let them read".
func (h *Handler) requireGuardianAccess(w http.ResponseWriter, r *http.Request, studentUserID string) bool {
	c := common.FromCtx(r)
	if !requireRole(w, c, roleOrangTua) {
		return false
	}
	if studentUserID == "" {
		common.WriteError(w, http.StatusBadRequest, "studentUserId is required")
		return false
	}
	linked, err := h.Internal.GuardianStudentAccess(c.SchoolID, c.UserID, studentUserID)
	if err != nil {
		common.WriteError(w, http.StatusServiceUnavailable, "guardian access check is unavailable")
		return false
	}
	if !linked {
		// Same 403 whether the student does not exist or simply is not this
		// guardian's child: the answer must not reveal which.
		common.WriteError(w, http.StatusForbidden, "student is not linked to this guardian")
		return false
	}
	return true
}

// ParentChildExams: GET /parent/children/{studentUserId}/exams — the linked
// student's academic exams (schedule + their own attempt status), for the
// Portal Orang Tua. Best-effort classroom resolution mirrors MyExams; when it
// fails the store still returns exams the student already has an attempt on.
func (h *Handler) ParentChildExams(w http.ResponseWriter, r *http.Request) {
	studentUserID := chi.URLParam(r, "studentUserId")
	if !h.requireGuardianAccess(w, r, studentUserID) {
		return
	}
	c := common.FromCtx(r)
	classroomID := h.Internal.StudentClassroomID(studentUserID, c.SchoolID, "")
	rows, err := h.Store.ListChildExams(r.Context(), c.SchoolID, studentUserID, classroomID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, rows, nil)
}

// ParentChildExamResult: GET /parent/children/{studentUserId}/exams/{examId} —
// that ONE student's result on one academic exam. Read-only and sanitized:
// store.StudentExamAttemptView carries status/score/timestamps only, never a
// token id, question order, or answer row.
func (h *Handler) ParentChildExamResult(w http.ResponseWriter, r *http.Request) {
	studentUserID := chi.URLParam(r, "studentUserId")
	if !h.requireGuardianAccess(w, r, studentUserID) {
		return
	}
	c := common.FromCtx(r)
	examID := chi.URLParam(r, "examId")
	row, err := h.Store.GetChildExamResult(r.Context(), c.SchoolID, studentUserID, examID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if row == nil {
		common.WriteError(w, http.StatusNotFound, "exam result not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, row, nil)
}
