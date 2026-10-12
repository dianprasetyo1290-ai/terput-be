package handler

import (
	"net/http"
	"strings"

	"skci/academic/internal/common"
)

// enrollmentStatuses is the class_enrollments.status CHECK set.
var enrollmentStatuses = map[string]bool{"active": true, "graduated": true, "transferred_out": true, "withdrawn": true}

// InternalListEnrollments handles
//
//	GET /internal/enrollments?school_id=&status=&student_user_id=
//
// for sibling services only (X-Internal-Key): notification's alumni segment
// (status=graduated) and admission's referral eligibility (T13). school_id is
// mandatory because an internal caller carries no JWT school.
func (h *Handler) InternalListEnrollments(w http.ResponseWriter, r *http.Request) {
	if !common.IsInternal(r) {
		common.WriteError(w, http.StatusForbidden, "internal callers only")
		return
	}
	q := r.URL.Query()
	schoolID := strings.TrimSpace(q.Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	status := strings.TrimSpace(q.Get("status"))
	if status != "" && !enrollmentStatuses[status] {
		common.WriteError(w, http.StatusBadRequest, "unknown status")
		return
	}
	rows, err := h.Store.ListEnrollmentsByStatus(r.Context(), schoolID, status, strings.TrimSpace(q.Get("student_user_id")))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list enrollments")
		return
	}
	common.WriteJSON(w, http.StatusOK, rows, nil)
}
