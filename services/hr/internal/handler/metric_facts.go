package handler

import (
	"net/http"
	"strings"
	"time"

	"skci/hr/internal/common"
)

// InternalMetricFacts handles GET /internal/metric-facts for hr metrics:
// active_staff
func (a *App) InternalMetricFacts(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	metric := strings.TrimSpace(r.URL.Query().Get("metric"))
	if metric != "active_staff" {
		common.WriteError(w, http.StatusBadRequest, "Unknown hr metric: "+metric)
		return
	}

	query := `SELECT count(*) FROM employees WHERE school_id = $1 AND status = 'active' AND deleted_at IS NULL`
	var count int64
	err := a.DB.QueryRowContext(r.Context(), query, schoolID).Scan(&count)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to query hr metric facts")
		return
	}

	resp := map[string]interface{}{
		"metric": "active_staff",
		"unit":   "count",
		"value":  count,
		"as_of":  time.Now(),
	}

	common.WriteJSON(w, http.StatusOK, resp, nil)
}
