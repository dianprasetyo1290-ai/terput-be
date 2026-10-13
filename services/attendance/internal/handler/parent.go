package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/attendance/internal/client"
	"skci/attendance/internal/common"
	"skci/attendance/internal/store"
)

// Portal Orang Tua (wali) read endpoints. SPA Portal Orang Tua calls these
// with a normal user JWT whose active_role is "orang_tua"; the JWT only tells
// us WHO is asking, never WHICH child they may see -- that link is re-verified
// live against identity-service on every request (see requireGuardianLink).

// parentAttendanceDay is one row of the child's daily list: which day, the
// status recorded for it, and the optional per-student note.
type parentAttendanceDay struct {
	SessionDate string  `json:"session_date"`
	Status      string  `json:"status"`
	Catatan     *string `json:"catatan,omitempty"`
	ClassroomID string  `json:"classroom_id"`
}

// parentAttendanceSummary is the hadir/sakit/izin/alfa recap for the window.
// "alfa" is the portal-facing name for the store's "tanpa_keterangan".
type parentAttendanceSummary struct {
	Hadir     int `json:"hadir"`
	Terlambat int `json:"terlambat"`
	Sakit     int `json:"sakit"`
	Izin      int `json:"izin"`
	Alfa      int `json:"alfa"`
}

type parentChildAttendanceResponse struct {
	StudentID      string                  `json:"student_id"`
	From           string                  `json:"from"`
	To             string                  `json:"to"`
	AcademicYearID string                  `json:"academic_year_id,omitempty"`
	Summary        parentAttendanceSummary `json:"summary"`
	Daily          []parentAttendanceDay   `json:"daily"`
}

// ParentChildAttendance backs
// GET /parent/children/{studentUserId}/attendance?from=&to=[&academic_year_id=]
// -- the Portal Orang Tua "presensi anak" screen. Read-only: it returns the
// linked child's hadir/sakit/izin/alfa recap plus the day-by-day list for the
// requested window. A wali may only ever read a child identity-service
// confirms they are linked to at the caller's own school; there is no write
// counterpart on this path.
func (h *Handler) ParentChildAttendance(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	if !requireRole(w, claims, "orang_tua") {
		return
	}

	studentUserID := chi.URLParam(r, "studentUserId")
	if studentUserID == "" {
		common.WriteError(w, http.StatusBadRequest, "studentUserId is required")
		return
	}
	if !h.requireGuardianLink(w, r, claims, studentUserID) {
		return
	}

	q := r.URL.Query()
	from, to, err := parseDateRange(q.Get("from"), q.Get("to"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if h.parentAttendance == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "attendance store unavailable")
		return
	}
	rows, err := h.parentAttendance.ListStudentAttendanceRange(r.Context(), store.ParentAttendanceFilter{
		StudentID:      studentUserID,
		SchoolID:       claims.SchoolID,
		AcademicYearID: q.Get("academic_year_id"),
		From:           from,
		To:             to,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var summary parentAttendanceSummary
	daily := make([]parentAttendanceDay, 0, len(rows))
	for _, row := range rows {
		switch row.Status {
		case "hadir":
			summary.Hadir++
		case "terlambat":
			summary.Terlambat++
		case "sakit":
			summary.Sakit++
		case "izin":
			summary.Izin++
		case "tanpa_keterangan":
			summary.Alfa++
		}
		daily = append(daily, parentAttendanceDay{
			SessionDate: row.SessionDate,
			Status:      row.Status,
			Catatan:     row.Catatan,
			ClassroomID: row.ClassroomID,
		})
	}

	common.WriteJSON(w, http.StatusOK, parentChildAttendanceResponse{
		StudentID:      studentUserID,
		From:           from,
		To:             to,
		AcademicYearID: q.Get("academic_year_id"),
		Summary:        summary,
		Daily:          daily,
	}, nil)
}

// requireGuardianLink verifies, live against identity-service, that the
// orang_tua caller (claims.sub) is a guardian of studentUserID at the
// caller's own school (claims.school_id). It fails CLOSED: a nil/unconfigured
// checker, a transport error, or an upstream 5xx all deny the request rather
// than allow it -- the only way through is a positive 200 from identity.
func (h *Handler) requireGuardianLink(w http.ResponseWriter, r *http.Request, claims *common.Claims, studentUserID string) bool {
	if h.guardianAccess == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "guardian authorization is not configured")
		return false
	}
	err := h.guardianAccess.CheckGuardianAccess(r.Context(), claims.UserID, studentUserID, claims.SchoolID)
	switch {
	case err == nil:
		return true
	case errors.Is(err, client.ErrGuardianNoAccess):
		common.WriteError(w, http.StatusForbidden, "forbidden: you are not a guardian of this student")
		return false
	default:
		common.WriteError(w, http.StatusServiceUnavailable, "guardian authorization check unavailable")
		return false
	}
}

const parentDateLayout = "2006-01-02"

// parseDateRange validates the from/to query params: both required, both
// YYYY-MM-DD, and from must not be after to. Normalized values are returned so
// the store filter and the echoed response agree.
func parseDateRange(from, to string) (string, string, error) {
	if from == "" || to == "" {
		return "", "", errors.New("from and to are required (YYYY-MM-DD)")
	}
	f, err := time.Parse(parentDateLayout, from)
	if err != nil {
		return "", "", errors.New("from must be a valid date (YYYY-MM-DD)")
	}
	t, err := time.Parse(parentDateLayout, to)
	if err != nil {
		return "", "", errors.New("to must be a valid date (YYYY-MM-DD)")
	}
	if f.After(t) {
		return "", "", errors.New("from must not be after to")
	}
	return f.Format(parentDateLayout), t.Format(parentDateLayout), nil
}
