package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/xuri/excelize/v2"

	"skci/identity/internal/common"
)

const orgSheetName = "Struktur Organisasi"

var orgTemplateHeaders = []string{"Jabatan", "Nama"}

// ExportOrganizationExcel: "Export" button on Organisasi Sekolah — same rows
// Organization() renders on screen (Jabatan | Nama, ordered the same way),
// as a downloadable .xlsx.
func (a *App) ExportOrganizationExcel(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	schoolID, filtered := resolveSchoolScope(r)
	if !filtered {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	rows, err := a.DB.Query(`
		SELECT p.name, u.name
		FROM position_assignments pa
		JOIN positions p ON p.id = pa.position_id AND p.deleted_at IS NULL
		JOIN users u ON u.id = pa.user_id AND u.deleted_at IS NULL
		WHERE pa.school_id = $1
		ORDER BY p.priority, p.name, u.name`, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to export organization")
		return
	}
	defer rows.Close()

	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", orgSheetName)
	f.SetSheetRow(orgSheetName, "A1", &orgTemplateHeaders)

	rowIdx := 2
	for rows.Next() {
		var position, name string
		if err := rows.Scan(&position, &name); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to export organization")
			return
		}
		f.SetSheetRow(orgSheetName, fmt.Sprintf("A%d", rowIdx), &[]string{position, name})
		rowIdx++
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=organisasi-sekolah.xlsx")
	_ = f.Write(w)
}

// DownloadOrganizationTemplate: "Download Template" button — headers + one
// example row, same 2-column shape ExportOrganizationExcel and
// ImportOrganization both use.
func (a *App) DownloadOrganizationTemplate(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", orgSheetName)
	f.SetSheetRow(orgSheetName, "A1", &orgTemplateHeaders)
	f.SetSheetRow(orgSheetName, "A2", &[]string{"Kepala Sekolah", "Nama Guru Contoh"})

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=template_organisasi_sekolah.xlsx")
	_ = f.Write(w)
}

type orgImportRowError struct {
	Row   int    `json:"row"`
	Issue string `json:"issue"`
}

type orgImportReport struct {
	Created int                 `json:"created"`
	Errors  []orgImportRowError `json:"errors"`
}

// ImportOrganization: "Import" button — bulk-assigns jabatan+guru pairs from
// an .xlsx following DownloadOrganizationTemplate's shape. Per the finding's
// own requirement, jabatan and guru must already be registered (this does
// NOT create positions or users) — unresolvable rows are reported back,
// resolvable ones are applied, matching AssignPosition's own behavior
// (including its jabatan-name -> user_school_roles mirroring) row by row.
func (a *App) ImportOrganization(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	schoolID := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin {
		if sid := r.URL.Query().Get("school_id"); sid != "" {
			schoolID = sid
		}
	}
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file is required (multipart field 'file')")
		return
	}
	defer file.Close()
	f, err := excelize.OpenReader(file)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid .xlsx file")
		return
	}
	defer f.Close()

	sheetName := orgSheetName
	if idx, _ := f.GetSheetIndex(sheetName); idx == -1 {
		sheetName = f.GetSheetName(0) // fall back to whatever the first sheet is called
	}
	rows, err := f.GetRows(sheetName)
	if err != nil || len(rows) < 2 {
		common.WriteError(w, http.StatusBadRequest, "sheet is empty or unreadable")
		return
	}

	report := orgImportReport{Errors: []orgImportRowError{}}
	for i, row := range rows[1:] {
		rowNum := i + 2 // 1-based, header excluded
		jabatan := strings.TrimSpace(cellAt(row, 0))
		guru := strings.TrimSpace(cellAt(row, 1))
		if jabatan == "" && guru == "" {
			continue
		}
		if jabatan == "" || guru == "" {
			report.Errors = append(report.Errors, orgImportRowError{Row: rowNum, Issue: "Jabatan dan Nama wajib diisi"})
			continue
		}

		var positionID, posName string
		err := a.DB.QueryRow(`SELECT id, name FROM positions WHERE school_id=$1 AND deleted_at IS NULL AND lower(name)=lower($2)`,
			schoolID, jabatan).Scan(&positionID, &posName)
		if err == sql.ErrNoRows {
			report.Errors = append(report.Errors, orgImportRowError{Row: rowNum, Issue: fmt.Sprintf("Jabatan %q belum terdaftar", jabatan)})
			continue
		} else if err != nil {
			report.Errors = append(report.Errors, orgImportRowError{Row: rowNum, Issue: "gagal memeriksa jabatan"})
			continue
		}

		var userID string
		err = a.DB.QueryRow(`SELECT u.id FROM users u
			JOIN user_school_roles usr ON usr.user_id = u.id
			WHERE u.deleted_at IS NULL AND usr.school_id=$1 AND usr.role='guru' AND lower(u.name)=lower($2)
			LIMIT 1`, schoolID, guru).Scan(&userID)
		if err == sql.ErrNoRows {
			report.Errors = append(report.Errors, orgImportRowError{Row: rowNum, Issue: fmt.Sprintf("Guru %q belum terdaftar di sekolah ini", guru)})
			continue
		} else if err != nil {
			report.Errors = append(report.Errors, orgImportRowError{Row: rowNum, Issue: "gagal memeriksa guru"})
			continue
		}

		tx, err := a.DB.Begin()
		if err != nil {
			report.Errors = append(report.Errors, orgImportRowError{Row: rowNum, Issue: "gagal menyimpan"})
			continue
		}
		if _, err := tx.Exec(`INSERT INTO position_assignments (position_id, user_id, school_id) VALUES ($1,$2,$3)
			ON CONFLICT (position_id, user_id) DO UPDATE SET created_at = position_assignments.created_at`,
			positionID, userID, schoolID); err != nil {
			tx.Rollback()
			report.Errors = append(report.Errors, orgImportRowError{Row: rowNum, Issue: "gagal menyimpan penugasan"})
			continue
		}
		if role, ok := common.JabatanOnlyRoleForPosition(posName); ok {
			if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)
				ON CONFLICT (user_id, school_id, role) DO NOTHING`, userID, schoolID, role); err != nil {
				tx.Rollback()
				report.Errors = append(report.Errors, orgImportRowError{Row: rowNum, Issue: "gagal menyimpan peran jabatan"})
				continue
			}
		}
		if err := tx.Commit(); err != nil {
			report.Errors = append(report.Errors, orgImportRowError{Row: rowNum, Issue: "gagal menyimpan"})
			continue
		}
		report.Created++
	}

	common.WriteJSON(w, http.StatusOK, report, nil)
}

func cellAt(row []string, i int) string {
	if i < len(row) {
		return row[i]
	}
	return ""
}
