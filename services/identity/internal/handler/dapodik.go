package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"skci/identity/internal/common"
)

// DapodikStudent represents a student row returned for Dapodik comparison.
type DapodikStudent struct {
	UserID     string  `json:"user_id"`
	Name       string  `json:"name"`
	NISN       *string `json:"nisn"`
	BirthDate  *string `json:"birth_date"`
	BirthPlace *string `json:"birth_place"`
	Religion   *string `json:"religion"`
}

// InternalDapodikStudents serves GET /internal/dapodik/students?school_id=
// Required for T23 Dapodik sync (FR-145-001..004).
func (a *App) InternalDapodikStudents(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id parameter is required")
		return
	}

	query := `
		SELECT u.id, u.name, u.nisn, u.birth_date, u.birth_place, u.religion
		FROM users u
		JOIN user_schools us ON us.user_id = u.id
		WHERE us.school_id = $1
		  AND us.role = 'siswa'
		  AND (u.deactivate_at IS NULL OR u.deactivate_at > NOW())
		ORDER BY u.name ASC
	`
	rows, err := a.DB.QueryContext(r.Context(), query, schoolID)
	if err != nil {
		common.WriteDBError(w, err, "failed to query students")
		return
	}
	defer rows.Close()

	var students []DapodikStudent
	for rows.Next() {
		var s DapodikStudent
		var nisn, bDate, bPlace, rel sql.NullString
		if err := rows.Scan(&s.UserID, &s.Name, &nisn, &bDate, &bPlace, &rel); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to scan student row")
			return
		}
		if nisn.Valid {
			s.NISN = &nisn.String
		}
		if bDate.Valid {
			s.BirthDate = &bDate.String
		}
		if bPlace.Valid {
			s.BirthPlace = &bPlace.String
		}
		if rel.Valid {
			s.Religion = &rel.String
		}
		students = append(students, s)
	}
	if students == nil {
		students = []DapodikStudent{}
	}

	common.WriteJSON(w, http.StatusOK, students, nil)
}
