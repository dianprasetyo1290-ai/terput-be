package handler

// Demo access (SPA /demo): the presenter of a demo picks a module, then the
// person to demo as, and lands on the login page with the e-mail filled in.
// The page lists the active demo accounts (name and e-mail only -- never a
// password). Demo accounts are the users whose e-mail domain is one of
// DEMO_ACCESS_DOMAINS; with that unset (every school's live system) the
// endpoint answers 404, so the page and its login link never show there.

import (
	"net/http"
	"strings"

	"skci/identity/internal/common"
)

type demoAccount struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ParseDemoDomains reads a comma-separated DEMO_ACCESS_DOMAINS value
// ("teratai.sch.id, sekoci.local") into lower-cased domains without "@".
func ParseDemoDomains(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(p)), "@"); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// DemoAccess lists the active demo accounts: GET /public/demo-access.
func (a *App) DemoAccess(w http.ResponseWriter, r *http.Request) {
	if len(a.DemoDomains) == 0 {
		common.WriteError(w, http.StatusNotFound, "Akses demo tidak tersedia.")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT email, name FROM users
		WHERE deleted_at IS NULL AND anonymized_at IS NULL
		  AND (deactivate_at IS NULL OR deactivate_at > now())
		  AND lower(split_part(email, '@', 2)) = ANY(string_to_array($1, ','))
		  AND EXISTS (SELECT 1 FROM user_school_roles usr WHERE usr.user_id = users.id AND usr.status = 'active')
		ORDER BY email`, strings.Join(a.DemoDomains, ","))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat akun demo.")
		return
	}
	defer rows.Close()
	var out []demoAccount
	for rows.Next() {
		var d demoAccount
		if err := rows.Scan(&d.Email, &d.Name); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat akun demo.")
			return
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat akun demo.")
		return
	}
	if len(out) == 0 {
		common.WriteError(w, http.StatusNotFound, "Akses demo tidak tersedia.")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"accounts": out}, nil)
}
