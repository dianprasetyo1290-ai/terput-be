package handler

import (
	"net/http"

	"skci/identity/internal/common"
)

type schoolStudentCount struct {
	SchoolID   string `json:"school_id"`
	SchoolName string `json:"school_name"`
	Students   int    `json:"students"`
	Teachers   int    `json:"teachers"`
}

type dashboardResponse struct {
	TotalSchools  int                  `json:"total_schools"`
	TotalStudents int                  `json:"total_students"`
	PerSchool     []schoolStudentCount `json:"per_school"`
}

// Dashboard: total sekolah terdaftar, per-sekolah jumlah murid & guru
// (realtime, computed per school_id so counts never mix across schools),
// total siswa keseluruhan. Superadmin sees every school; kepala_yayasan sees
// only the schools of its own school's foundation.
func (a *App) Dashboard(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin, common.RoleKepalaYayasan) {
		return
	}
	// "" = all schools (superadmin); otherwise the caller's school id, widened
	// to its foundation's schools in the WHERE below.
	scope := ""
	if c := common.FromCtx(r); c != nil && c.ActiveRole != common.RoleSuperadmin {
		scope = c.SchoolID
	}

	var resp dashboardResponse
	rows, err := a.DB.Query(`
		SELECT s.id, s.name,
		       count(usr.id) FILTER (WHERE usr.role = 'siswa' AND usr.status = 'active'),
		       count(usr.id) FILTER (WHERE usr.role = 'guru' AND usr.status = 'active')
		FROM schools s
		LEFT JOIN user_school_roles usr ON usr.school_id = s.id
		WHERE s.deleted_at IS NULL
		  AND ($1 = '' OR s.id::text = $1 OR s.foundation_id = (SELECT foundation_id FROM schools WHERE id::text = $1))
		GROUP BY s.id, s.name
		ORDER BY s.name`, scope)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat dasbor")
		return
	}
	defer rows.Close()
	resp.PerSchool = []schoolStudentCount{}
	for rows.Next() {
		var c schoolStudentCount
		if err := rows.Scan(&c.SchoolID, &c.SchoolName, &c.Students, &c.Teachers); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat dasbor")
			return
		}
		resp.PerSchool = append(resp.PerSchool, c)
		resp.TotalStudents += c.Students
	}
	resp.TotalSchools = len(resp.PerSchool)

	common.WriteJSON(w, http.StatusOK, resp, nil)
}
