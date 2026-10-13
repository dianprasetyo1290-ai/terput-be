package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"skci/identity/internal/common"
)

// InternalMetricFacts handles GET /internal/metric-facts for identity metrics:
// active_students, active_teachers
func (a *App) InternalMetricFacts(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	metric := strings.TrimSpace(r.URL.Query().Get("metric"))

	var query string
	switch metric {
	case "active_students":
		query = `SELECT count(usr.id) FROM user_school_roles usr
		         WHERE usr.school_id::text = $1 AND usr.role = 'siswa' AND usr.status = 'active'`
	case "active_teachers":
		query = `SELECT count(usr.id) FROM user_school_roles usr
		         WHERE usr.school_id::text = $1 AND usr.role = 'guru' AND usr.status = 'active'`
	default:
		common.WriteError(w, http.StatusBadRequest, "Unknown identity metric: "+metric)
		return
	}

	var count int64
	err := a.DB.QueryRow(query, schoolID).Scan(&count)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to query metric facts")
		return
	}

	resp := map[string]interface{}{
		"metric": metric,
		"unit":   "count",
		"value":  count,
		"as_of":  time.Now(),
	}

	common.WriteJSON(w, http.StatusOK, resp, nil)
}

// InternalFoundationSchools handles GET /internal/schools/{id}/foundation-schools
// Returns active schools in the same foundation as {id}
func (a *App) InternalFoundationSchools(w http.ResponseWriter, r *http.Request) {
	schoolID := chi.URLParam(r, "id")
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school id is required")
		return
	}

	rows, err := a.DB.Query(`
		SELECT s.id::text, s.name, COALESCE(s.level, '')
		FROM schools s
		WHERE s.deleted_at IS NULL
		  AND s.foundation_id = (SELECT foundation_id FROM schools WHERE id::text = $1 AND deleted_at IS NULL)
		ORDER BY s.name ASC`, schoolID)
	if err != nil {
		if err == sql.ErrNoRows {
			common.WriteJSON(w, http.StatusOK, []any{}, nil)
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "failed to query foundation schools")
		return
	}
	defer rows.Close()

	type schoolInfo struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Level string `json:"level"`
	}

	schools := []schoolInfo{}
	for rows.Next() {
		var s schoolInfo
		if err := rows.Scan(&s.ID, &s.Name, &s.Level); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to scan school info")
			return
		}
		schools = append(schools, s)
	}

	common.WriteJSON(w, http.StatusOK, schools, nil)
}
