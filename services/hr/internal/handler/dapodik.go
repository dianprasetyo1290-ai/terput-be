package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"skci/hr/internal/common"
)

type DapodikPTKItem struct {
	EmployeeID       string  `json:"employee_id"`
	FullName         string  `json:"full_name"`
	Gender           string  `json:"gender"`
	BirthDate        string  `json:"birth_date"`
	Position         string  `json:"position"`
	EmploymentStatus string  `json:"employment_status"`
	NIKLast4         *string `json:"nik_last4"`
}

// InternalDapodikPTK serves GET /internal/dapodik/ptk?school_id=
// Returns active PTK data for Dapodik sync (FR-145-001..004, NFR-HR-01: no raw NIK).
func (a *App) InternalDapodikPTK(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.Error(w, http.StatusBadRequest, "school_id parameter is required")
		return
	}

	query := `
		SELECT e.id, e.full_name, e.gender, e.birth_date,
		       COALESCE(p.name, '') as position_name,
		       COALESCE(e.employment_type, '') as employment_status,
		       e.nik_enc
		FROM employees e
		LEFT JOIN employee_assignments ea ON ea.employee_id = e.id AND ea.is_primary = true
		LEFT JOIN positions p ON p.id = ea.position_id
		WHERE e.school_id = $1
		  AND e.deleted_at IS NULL
		  AND e.status = 'active'
		ORDER BY e.full_name ASC
	`
	rows, err := a.DB.QueryContext(r.Context(), query, schoolID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "failed to query employees: "+err.Error())
		return
	}
	defer rows.Close()

	var ptkList []DapodikPTKItem
	for rows.Next() {
		var item DapodikPTKItem
		var bDate timeNullString
		var nikEnc []byte
		if err := rows.Scan(&item.EmployeeID, &item.FullName, &item.Gender, &bDate, &item.Position, &item.EmploymentStatus, &nikEnc); err != nil {
			common.Error(w, http.StatusInternalServerError, "failed to scan employee row")
			return
		}
		if bDate.Valid {
			item.BirthDate = bDate.String
		}
		if len(nikEnc) > 0 {
			if decrypted, err := a.Crypto.Decrypt(nikEnc, schoolID); err == nil && len(decrypted) >= 4 {
				last4 := decrypted[len(decrypted)-4:]
				item.NIKLast4 = &last4
			}
		}
		ptkList = append(ptkList, item)
	}
	if ptkList == nil {
		ptkList = []DapodikPTKItem{}
	}

	common.JSON(w, http.StatusOK, ptkList)
}

type DapodikPTKMatchRequest struct {
	SchoolID string   `json:"school_id"`
	NIKs     []string `json:"niks"`
}

// InternalDapodikPTKMatch serves POST /internal/dapodik/ptk/match
// Body: {school_id, niks[]}, returns map of {nik_index -> employee_id}.
// Evaluated via HMAC BlindIndex; raw NIKs must not be logged.
func (a *App) InternalDapodikPTKMatch(w http.ResponseWriter, r *http.Request) {
	var req DapodikPTKMatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = strings.TrimSpace(req.SchoolID)
	if req.SchoolID == "" {
		common.Error(w, http.StatusBadRequest, "school_id is required")
		return
	}

	result := make(map[string]string)
	if len(req.NIKs) == 0 {
		common.JSON(w, http.StatusOK, result)
		return
	}

	// Compute blind indices for each input NIK
	type nikIdxPair struct {
		index int
		bidx  string
	}
	var pairs []nikIdxPair
	var bidxList []string
	for i, rawNIK := range req.NIKs {
		clean := strings.TrimSpace(rawNIK)
		if clean == "" {
			continue
		}
		bidx := a.Crypto.BlindIndex(clean, req.SchoolID)
		pairs = append(pairs, nikIdxPair{index: i, bidx: bidx})
		bidxList = append(bidxList, bidx)
	}

	if len(bidxList) == 0 {
		common.JSON(w, http.StatusOK, result)
		return
	}

	// Query employees matching any blind index in that school
	query := `
		SELECT id, nik_bidx
		FROM employees
		WHERE school_id = $1
		  AND nik_bidx = ANY($2)
		  AND deleted_at IS NULL
	`
	rows, err := a.DB.QueryContext(r.Context(), query, req.SchoolID, bidxList)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "failed to query matched employees: "+err.Error())
		return
	}
	defer rows.Close()

	bidxToEmpID := make(map[string]string)
	for rows.Next() {
		var empID, bidx string
		if err := rows.Scan(&empID, &bidx); err == nil {
			bidxToEmpID[bidx] = empID
		}
	}

	for _, p := range pairs {
		if empID, found := bidxToEmpID[p.bidx]; found {
			result[strconv.Itoa(p.index)] = empID
		}
	}

	common.JSON(w, http.StatusOK, result)
}

type timeNullString = sql.NullString
