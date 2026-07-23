package handler

import (
	"net/http"

	"skci/identity/internal/common"
)

type schoolStudentCount struct {
	SchoolID   string `json:"school_id"`
	SchoolName string `json:"school_name"`
	Students   int    `json:"students"`
}

type dashboardResponse struct {
	TotalSchools  int                  `json:"total_schools"`
	TotalStudents int                  `json:"total_students"`
	PerSchool     []schoolStudentCount `json:"per_school"`
}

// Dashboard: superadmin-only. Total sekolah terdaftar, per-sekolah jumlah
// murid (realtime, computed per school_id so counts never mix across
// schools), total siswa keseluruhan.
func (a *App) Dashboard(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}

	var resp dashboardResponse
	if err := a.DB.QueryRow(`SELECT count(*) FROM schools WHERE deleted_at IS NULL`).Scan(&resp.TotalSchools); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to load dashboard")
		return
	}

	rows, err := a.DB.Query(`
		SELECT s.id, s.name, count(usr.id) FILTER (WHERE usr.role = 'siswa' AND usr.status = 'active')
		FROM schools s
		LEFT JOIN user_school_roles usr ON usr.school_id = s.id
		WHERE s.deleted_at IS NULL
		GROUP BY s.id, s.name
		ORDER BY s.name`)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to load dashboard")
		return
	}
	defer rows.Close()
	resp.PerSchool = []schoolStudentCount{}
	for rows.Next() {
		var c schoolStudentCount
		if err := rows.Scan(&c.SchoolID, &c.SchoolName, &c.Students); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to load dashboard")
			return
		}
		resp.PerSchool = append(resp.PerSchool, c)
		resp.TotalStudents += c.Students
	}

	common.WriteJSON(w, http.StatusOK, resp, nil)
}
