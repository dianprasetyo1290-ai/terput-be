package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
)

// InternalMetricFacts handles GET /internal/metric-facts for admission metrics:
// ppdb_conversion, ppdb_applicants (+ group_by=wave, group_by=wave_day)
func (a *App) InternalMetricFacts(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	metric := strings.TrimSpace(r.URL.Query().Get("metric"))
	groupBy := strings.TrimSpace(r.URL.Query().Get("group_by"))

	ctx := r.Context()
	now := time.Now()

	// Query applications for the school
	query := `SELECT a.id, a.wave_id::text, a.status, a.created_at,
	                 COALESCE(w.name, '') as wave_name,
	                 w.registration_start
	          FROM applications a
	          LEFT JOIN admission_waves w ON w.id = a.wave_id
	          WHERE a.school_id = $1`

	rows, err := a.DB.QueryContext(ctx, query, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	type appRow struct {
		ID        string
		WaveID    string
		Status    string
		CreatedAt time.Time
		WaveName  string
		WaveStart *time.Time
	}

	var allApps []appRow
	var enrolledCount, totalCount int64
	waveCounts := make(map[string]int64)
	waveEnrolled := make(map[string]int64)
	waveDaysCumulative := make(map[int]int64)

	for rows.Next() {
		var (
			ar     appRow
			wStart *time.Time
		)
		if err := rows.Scan(&ar.ID, &ar.WaveID, &ar.Status, &ar.CreatedAt, &ar.WaveName, &wStart); err != nil {
			writeErr(w, err)
			return
		}
		ar.WaveStart = wStart
		allApps = append(allApps, ar)

		totalCount++
		waveCounts[ar.WaveName]++
		// Enrolled / accepted as converted
		if ar.Status == "enrolled" || ar.Status == "accepted" {
			enrolledCount++
			waveEnrolled[ar.WaveName]++
		}

		if ar.WaveStart != nil && !ar.CreatedAt.Before(*ar.WaveStart) {
			dayIdx := int(ar.CreatedAt.Sub(*ar.WaveStart).Hours() / 24)
			waveDaysCumulative[dayIdx]++
		}
	}

	resp := map[string]interface{}{
		"metric": metric,
		"as_of":  now,
	}

	type factRow struct {
		DimValue    string `json:"dim_value"`
		Numerator   *int64 `json:"numerator,omitempty"`
		Denominator *int64 `json:"denominator,omitempty"`
		Value       *int64 `json:"value,omitempty"`
	}

	switch metric {
	case "ppdb_applicants":
		resp["unit"] = "count"
		resp["value"] = totalCount

		if groupBy == "wave" {
			var fRows []factRow
			for wName, cnt := range waveCounts {
				cntCopy := cnt
				fRows = append(fRows, factRow{
					DimValue: wName,
					Value:    &cntCopy,
				})
			}
			resp["rows"] = fRows
		} else if groupBy == "wave_day" {
			var fRows []factRow
			var cum int64
			for d := 0; d <= 90; d++ {
				if added, ok := waveDaysCumulative[d]; ok {
					cum += added
					cumCopy := cum
					fRows = append(fRows, factRow{
						DimValue: fmt.Sprintf("%d", d),
						Value:    &cumCopy,
					})
				}
			}
			resp["rows"] = fRows
		}

	case "ppdb_conversion":
		resp["unit"] = "percent_bp"
		resp["numerator"] = enrolledCount
		resp["denominator"] = totalCount
		var rateBP int64 = 0
		if totalCount > 0 {
			rateBP = (enrolledCount * 10000) / totalCount
		}
		resp["value"] = rateBP

		if groupBy == "wave" {
			var fRows []factRow
			for wName, cnt := range waveCounts {
				enr := waveEnrolled[wName]
				var rVal int64 = 0
				if cnt > 0 {
					rVal = (enr * 10000) / cnt
				}
				enrCopy, cntCopy, rValCopy := enr, cnt, rVal
				fRows = append(fRows, factRow{
					DimValue:    wName,
					Numerator:   &enrCopy,
					Denominator: &cntCopy,
					Value:       &rValCopy,
				})
			}
			resp["rows"] = fRows
		}

	default:
		common.WriteError(w, http.StatusBadRequest, "Unknown admission metric: "+metric)
		return
	}

	common.WriteJSON(w, http.StatusOK, resp, nil)
}
