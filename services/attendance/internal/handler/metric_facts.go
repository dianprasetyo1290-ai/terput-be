package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/attendance/internal/common"
)

// InternalMetricFacts handles GET /internal/metric-facts for attendance metrics:
// attendance_rate (with late_counts_as_present query param)
func (h *Handler) InternalMetricFacts(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	metric := strings.TrimSpace(r.URL.Query().Get("metric"))
	if metric != "attendance_rate" {
		common.WriteError(w, http.StatusBadRequest, "Unknown attendance metric: "+metric)
		return
	}
	fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
	toStr := strings.TrimSpace(r.URL.Query().Get("to"))
	groupBy := strings.TrimSpace(r.URL.Query().Get("group_by"))

	lateCountsAsPresent := true
	if r.URL.Query().Get("late_counts_as_present") == "false" {
		lateCountsAsPresent = false
	}

	ctx := r.Context()
	now := time.Now()

	query := `SELECT to_char(sa.session_date, 'MM') as month_str,
	                 count(*) FILTER (WHERE sa.status = 'hadir') as hadir,
	                 count(*) FILTER (WHERE sa.status = 'terlambat') as terlambat,
	                 count(*) FILTER (WHERE sa.status = 'sakit') as sakit,
	                 count(*) FILTER (WHERE sa.status = 'izin') as izin,
	                 count(*) FILTER (WHERE sa.status = 'tanpa_keterangan') as alpa
	          FROM student_attendances sa
	          WHERE sa.school_id = $1`

	args := []interface{}{schoolID}
	if fromStr != "" {
		query += " AND sa.session_date >= $2::date"
		args = append(args, fromStr)
	}
	if toStr != "" {
		query += fmt.Sprintf(" AND sa.session_date <= $%d::date", len(args)+1)
		args = append(args, toStr)
	}
	query += " GROUP BY to_char(sa.session_date, 'MM') ORDER BY month_str ASC"

	rows, err := h.Store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Failed to query attendance facts: "+err.Error())
		return
	}
	defer rows.Close()

	type monthFact struct {
		Month     string
		Numerator int64
		Effective int64
	}

	var totalNum, totalEff int64
	var monthFacts []monthFact

	for rows.Next() {
		var (
			mStr                      string
			hadir, terlambat, s, i, a int64
		)
		if err := rows.Scan(&mStr, &hadir, &terlambat, &s, &i, &a); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Failed to scan facts: "+err.Error())
			return
		}

		eff := hadir + terlambat + s + i + a
		num := hadir
		if lateCountsAsPresent {
			num += terlambat
		}

		totalNum += num
		totalEff += eff
		monthFacts = append(monthFacts, monthFact{
			Month:     mStr,
			Numerator: num,
			Effective: eff,
		})
	}

	var rateBP int64 = 0
	if totalEff > 0 {
		rateBP = (totalNum * 10000) / totalEff
	}

	resp := map[string]interface{}{
		"metric":      "attendance_rate",
		"unit":        "percent_bp",
		"numerator":   totalNum,
		"denominator": totalEff,
		"value":       rateBP,
		"as_of":       now,
	}

	if groupBy == "month" {
		type factRow struct {
			DimValue    string `json:"dim_value"`
			Numerator   *int64 `json:"numerator,omitempty"`
			Denominator *int64 `json:"denominator,omitempty"`
			Value       *int64 `json:"value,omitempty"`
		}
		var fRows []factRow
		for _, mf := range monthFacts {
			var rVal int64 = 0
			if mf.Effective > 0 {
				rVal = (mf.Numerator * 10000) / mf.Effective
			}
			numCopy, effCopy, valCopy := mf.Numerator, mf.Effective, rVal
			fRows = append(fRows, factRow{
				DimValue:    mf.Month,
				Numerator:   &numCopy,
				Denominator: &effCopy,
				Value:       &valCopy,
			})
		}
		resp["rows"] = fRows
	}

	common.WriteJSON(w, http.StatusOK, resp, nil)
}

// InternalRiskFactsAbsence handles GET /internal/risk-facts/absence
// Returns [{student_user_id, effective_days, absent_days, unexcused_count, consecutive_absent_days}]
func (h *Handler) InternalRiskFactsAbsence(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
	toStr := strings.TrimSpace(r.URL.Query().Get("to"))

	ctx := r.Context()

	query := `SELECT sa.student_id::text, sa.session_date, sa.status
	          FROM student_attendances sa
	          WHERE sa.school_id = $1`

	args := []interface{}{schoolID}
	if fromStr != "" {
		query += " AND sa.session_date >= $2::date"
		args = append(args, fromStr)
	}
	if toStr != "" {
		query += fmt.Sprintf(" AND sa.session_date <= $%d::date", len(args)+1)
		args = append(args, toStr)
	}
	query += " ORDER BY sa.student_id, sa.session_date DESC"

	rows, err := h.Store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Failed to query absence facts: "+err.Error())
		return
	}
	defer rows.Close()

	type studentAbsence struct {
		StudentUserID         string `json:"student_user_id"`
		EffectiveDays         int    `json:"effective_days"`
		AbsentDays            int    `json:"absent_days"`
		UnexcusedCount        int    `json:"unexcused_count"`
		ConsecutiveAbsentDays int    `json:"consecutive_absent_days"`
	}

	factsMap := make(map[string]*studentAbsence)
	consecutiveTracking := make(map[string]bool)

	for rows.Next() {
		var (
			studentID string
			sDate     time.Time
			status    string
		)
		if err := rows.Scan(&studentID, &sDate, &status); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Failed to scan row: "+err.Error())
			return
		}

		sa := factsMap[studentID]
		if sa == nil {
			sa = &studentAbsence{StudentUserID: studentID}
			factsMap[studentID] = sa
			consecutiveTracking[studentID] = true
		}

		sa.EffectiveDays++
		isAbsent := (status == "sakit" || status == "izin" || status == "tanpa_keterangan")
		if isAbsent {
			sa.AbsentDays++
			if status == "tanpa_keterangan" {
				sa.UnexcusedCount++
			}
			if consecutiveTracking[studentID] {
				sa.ConsecutiveAbsentDays++
			}
		} else {
			// Streak broken
			consecutiveTracking[studentID] = false
		}
	}

	result := make([]studentAbsence, 0, len(factsMap))
	for _, sa := range factsMap {
		result = append(result, *sa)
	}

	common.WriteJSON(w, http.StatusOK, result, nil)
}
