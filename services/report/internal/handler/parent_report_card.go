package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/report/internal/common"
	"skci/report/internal/model"
)

// parentRole is the JWT active_role the Portal Orang Tua SPA presents. Every
// /parent/... route is restricted to it -- staff/admin reach the same rapor
// data through their own existing endpoints, and letting a guru token ride
// the guardian endpoints would bypass the linked-student check entirely.
const parentRole = "orang_tua"

// RequireGuardianStudent is the authorization gate in front of every
// /parent/children/{studentUserId}/... route. It re-derives the guardian and
// school from the caller's own JWT (never from a query param, so a wali
// cannot claim someone else's identity), then asks identity-service whether
// that guardian is linked to the {studentUserId} named in the path:
//
//   - linked (identity 200)          -> proceed
//   - not linked (identity 404)      -> 403, never 200-with-empty-data
//   - identity unreachable / answer  -> 503, fail closed
//   - GuardianAuth not configured    -> 503, fail closed
//
// The check runs on every request (the /parent/ routes opt out of the
// response cache in router.go precisely so a revoked link takes effect
// immediately rather than being masked by a cached 200), which is what stops
// a wali viewing a student they are not linked to -- including their own
// child's classmate, by swapping the studentUserId in the URL.
func (h *Handler) RequireGuardianStudent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := common.FromCtx(r)
		if claims == nil {
			common.WriteError(w, http.StatusUnauthorized, "missing token")
			return
		}
		if claims.ActiveRole != parentRole {
			common.WriteError(w, http.StatusForbidden, "only orang_tua may use the parent portal")
			return
		}

		guardianUserID := claims.UserID
		schoolID := claims.SchoolID
		studentUserID := chi.URLParam(r, "studentUserId")
		if guardianUserID == "" || schoolID == "" || studentUserID == "" {
			common.WriteError(w, http.StatusForbidden, "guardian, student, and school must all be resolvable from the token and path")
			return
		}

		if h.d.GuardianAuth == nil {
			common.WriteError(w, http.StatusServiceUnavailable, "guardian authorization is not configured")
			return
		}
		linked, err := h.d.GuardianAuth.GuardianStudentAccess(r.Context(), guardianUserID, studentUserID, schoolID)
		if err != nil {
			common.WriteError(w, http.StatusServiceUnavailable, "guardian authorization unavailable")
			return
		}
		if !linked {
			common.WriteError(w, http.StatusForbidden, "not linked to this student")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ListParentReportCards: GET /parent/children/{studentUserId}/report-cards
// (optional ?academic_year_id= and ?term_id=). Read-only list of the
// student's report cards for the linked wali -- the rapor equivalent of the
// siswa's own GET /students/{id}/report-cards, reached through the
// guardian-scoped URL instead.
func (h *Handler) ListParentReportCards(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	studentUserID := chi.URLParam(r, "studentUserId")
	termID := r.URL.Query().Get("term_id")
	academicYearID := r.URL.Query().Get("academic_year_id")

	cards, err := h.d.Store.ListReportCardsByStudent(r.Context(), studentUserID, termID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report cards")
		return
	}

	out := make([]model.ReportCard, 0, len(cards))
	for _, rc := range cards {
		// Defense in depth on top of the guardian gate: even a linked wali
		// only ever sees rows belonging to the token's own school.
		if claims != nil && rc.SchoolID != claims.SchoolID {
			continue
		}
		if academicYearID != "" && rc.AcademicYearID != academicYearID {
			continue
		}
		subjects, err := h.d.Store.ListSubjects(r.Context(), rc.ID)
		if err == nil {
			bands, _ := h.d.Store.GetGradeBands(r.Context(), rc.SchoolID)
			applyGradeLabels(subjects, bands)
			rc.Subjects = subjects
		}
		out = append(out, rc)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// GetParentReportCard: GET
// /parent/children/{studentUserId}/report-cards/{reportCardId}. The single
// rapor read a wali's Portal Orang Tua opens. Read-only: no method on this
// file writes, and the route is GET-only. Returns 404 (not 403) when the
// card belongs to a different student than the path claims, so a wali cannot
// use this to probe whether some other student's report-card id exists.
func (h *Handler) GetParentReportCard(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	studentUserID := chi.URLParam(r, "studentUserId")
	reportCardID := chi.URLParam(r, "reportCardId")

	rc, err := h.d.Store.GetReportCard(r.Context(), reportCardID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report card")
		return
	}
	// Scoping: the card must belong to the student named in the path (whom
	// the gate above already confirmed the wali is linked to) and to the
	// caller's school. Either mismatch is indistinguishable 404.
	if rc == nil || rc.StudentID != studentUserID {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	if claims != nil && rc.SchoolID != claims.SchoolID {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}

	subjects, err := h.d.Store.ListSubjects(r.Context(), rc.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch subjects")
		return
	}
	bands, _ := h.d.Store.GetGradeBands(r.Context(), rc.SchoolID)
	applyGradeLabels(subjects, bands)
	h.enrichSubjectGroups(r.Context(), rc.ClassroomID, rc.TermID, subjects)
	rc.Subjects = subjects
	rc.School = h.fetchSchoolInfo(r.Context(), rc.SchoolID)
	rc.Fase = h.resolveFase(r.Context(), rc.ClassroomID)
	common.WriteJSON(w, http.StatusOK, rc, nil)
}
