package handler

import (
	"net/http"
	"regexp"
	"strings"

	"skci/academic/internal/common"
)

var reUserUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// parseUserIDs validates a comma-separated list of 1..50 UUIDs.
func parseUserIDs(raw string) ([]string, bool) {
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !reUserUUID.MatchString(p) {
			return nil, false
		}
		out = append(out, strings.ToLower(p))
	}
	return out, len(out) > 0 && len(out) <= 50
}

// InternalUserClassrooms: GET /internal/user-classrooms?school_id=&user_ids=a,b
//
// Classrooms the given users belong to in the unit: active student
// enrollment, wali kelas, or guru pengampu of an active subject. Used by
// identity to show class-targeted announcements (QA-IDN BUG-089); a parent's
// call passes the children's ids. Internal key only.
func (h *Handler) InternalUserClassrooms(w http.ResponseWriter, r *http.Request) {
	if !common.IsInternal(r) {
		common.WriteError(w, http.StatusForbidden, "internal callers only")
		return
	}
	q := r.URL.Query()
	schoolID := strings.TrimSpace(q.Get("school_id"))
	if !reUserUUID.MatchString(schoolID) {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	ids, ok := parseUserIDs(q.Get("user_ids"))
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "user_ids must be 1-50 UUIDs")
		return
	}
	arr := "{" + strings.Join(ids, ",") + "}"
	rows, err := h.Store.DB.QueryContext(r.Context(), `
		SELECT DISTINCT c.id::text FROM classrooms c
		WHERE c.school_id = $1::uuid AND c.deleted_at IS NULL AND (
			c.homeroom_teacher_user_id = ANY($2::uuid[])
			OR EXISTS (SELECT 1 FROM class_enrollments e WHERE e.classroom_id = c.id
				AND e.status = 'active' AND e.student_user_id = ANY($2::uuid[]))
			OR EXISTS (SELECT 1 FROM classroom_subjects s WHERE s.classroom_id = c.id
				AND s.deleted_at IS NULL AND s.is_active AND s.teacher_user_id = ANY($2::uuid[])))
		ORDER BY 1`, schoolID, arr)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list classrooms")
		return
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list classrooms")
			return
		}
		out = append(out, id)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
