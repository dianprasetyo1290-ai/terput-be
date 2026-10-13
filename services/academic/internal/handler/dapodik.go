package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"skci/academic/internal/common"
)

type DapodikRombelItem struct {
	ClassroomID           string  `json:"classroom_id"`
	Name                  string  `json:"name"`
	GradeLevel            int     `json:"grade_level"`
	HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id"`
	StudentCount          int     `json:"student_count"`
}

type DapodikRombelResponse struct {
	Rombel          []DapodikRombelItem `json:"rombel"`
	StudentRombelMap map[string]string   `json:"student_rombel_map"` // student_user_id -> classroom_name
}

// InternalDapodikRombel serves GET /internal/dapodik/rombel?school_id=&academic_year_id=
// Required for T23 Dapodik sync (FR-145-001..004).
func (h *Handler) InternalDapodikRombel(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id parameter is required")
		return
	}
	academicYearID := strings.TrimSpace(r.URL.Query().Get("academic_year_id"))

	// 1. Query rombel/classrooms
	query := `
		SELECT c.id, c.name, c.grade_level, c.homeroom_teacher_user_id,
		       (SELECT count(*) FROM class_enrollments ce WHERE ce.classroom_id = c.id AND ce.status = 'active') as student_count
		FROM classrooms c
		WHERE c.school_id = $1
		  AND ($2 = '' OR c.academic_year_id::text = $2)
		  AND c.deleted_at IS NULL
		ORDER BY c.grade_level, c.name ASC
	`
	rows, err := h.Store.DB.QueryContext(r.Context(), query, schoolID, academicYearID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to query classrooms: "+err.Error())
		return
	}
	defer rows.Close()

	var rombelList []DapodikRombelItem
	for rows.Next() {
		var item DapodikRombelItem
		var teacher sql.NullString
		if err := rows.Scan(&item.ClassroomID, &item.Name, &item.GradeLevel, &teacher, &item.StudentCount); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to scan classroom row")
			return
		}
		if teacher.Valid {
			item.HomeroomTeacherUserID = &teacher.String
		}
		rombelList = append(rombelList, item)
	}
	if rombelList == nil {
		rombelList = []DapodikRombelItem{}
	}

	// 2. Query student_user_id -> classroom mapping
	mapQuery := `
		SELECT ce.student_user_id, c.name
		FROM class_enrollments ce
		JOIN classrooms c ON c.id = ce.classroom_id
		WHERE c.school_id = $1
		  AND ($2 = '' OR c.academic_year_id::text = $2)
		  AND ce.status = 'active'
		  AND c.deleted_at IS NULL
	`
	mapRows, err := h.Store.DB.QueryContext(r.Context(), mapQuery, schoolID, academicYearID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to query student enrollments: "+err.Error())
		return
	}
	defer mapRows.Close()

	studentMap := make(map[string]string)
	for mapRows.Next() {
		var studentID, classroomName string
		if err := mapRows.Scan(&studentID, &classroomName); err == nil {
			studentMap[studentID] = classroomName
		}
	}

	common.WriteJSON(w, http.StatusOK, DapodikRombelResponse{
		Rombel:          rombelList,
		StudentRombelMap: studentMap,
	}, nil)
}
