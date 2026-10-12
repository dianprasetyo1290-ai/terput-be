package sensitive

import (
	"database/sql"
	"errors"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/store"
)

// Handler serves the generic /sensitive/* endpoints (kebijakan akses + log
// akses NFR-OPS-03), reused by T20 for the counseling module.
type Handler struct {
	DB     *sql.DB
	Client *client.Client
	// ModuleFeatures maps a sensitive module to its feature key; a module is
	// only selectable while its feature is on for the school. Registered in main.
	ModuleFeatures map[string]string
}

func (h *Handler) moduleEnabled(w http.ResponseWriter, r *http.Request, schoolID, module string) bool {
	feature, ok := h.ModuleFeatures[module]
	if !ok || !slices.Contains(Modules, module) {
		common.WriteError(w, http.StatusBadRequest, "Modul tidak dikenal")
		return false
	}
	on, err := h.Client.FeatureEnabled(r.Context(), schoolID, feature)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "Gagal memverifikasi modul sekolah")
		return false
	}
	if !on {
		common.WriteError(w, http.StatusForbidden, "Modul belum diaktifkan untuk sekolah ini")
		return false
	}
	return true
}

// ListPolicies: GET /sensitive/access-policies?module=
func (h *Handler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	_, school := common.RequireRole(w, r, common.RoleAdmin, common.RoleSuperadmin)
	if school == "" {
		return
	}
	module := r.URL.Query().Get("module")
	if !h.moduleEnabled(w, r, school, module) {
		return
	}
	list, err := ListPolicies(r.Context(), h.DB, school, module)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kebijakan akses")
		return
	}
	common.WriteSuccess(w, map[string]any{"policies": list, "grantable_roles": GrantableRoles}, nil)
}

// GrantPolicy: POST /sensitive/access-policies {module, role}
func (h *Handler) GrantPolicy(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RoleAdmin, common.RoleSuperadmin)
	if school == "" {
		return
	}
	var body struct {
		Module string `json:"module"`
		Role   string `json:"role"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	if !slices.Contains(GrantableRoles, body.Role) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Peran tidak dapat diberi akses data sensitif")
		return
	}
	if !h.moduleEnabled(w, r, school, body.Module) {
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan kebijakan")
		return
	}
	defer tx.Rollback()
	p, err := GrantPolicy(r.Context(), tx, school, body.Module, body.Role, c.UserID)
	if errors.Is(err, ErrPolicyExists) {
		common.WriteError(w, http.StatusConflict, "Peran ini sudah memiliki akses aktif")
		return
	}
	if err == nil {
		err = store.Audit(r.Context(), tx, store.AuditEntry{SchoolID: school, Module: body.Module, EntityType: "confidential_access_policy",
			EntityID: p.ID, Action: "grant", ActorUserID: &c.UserID, ActorRole: &c.ActiveRole,
			After: map[string]any{"module": p.Module, "role": p.Role}})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan kebijakan")
		return
	}
	common.WriteJSON(w, http.StatusCreated, p, nil)
}

// RevokePolicy: DELETE /sensitive/access-policies/{id}
func (h *Handler) RevokePolicy(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RoleAdmin, common.RoleSuperadmin)
	if school == "" {
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencabut kebijakan")
		return
	}
	defer tx.Rollback()
	p, err := RevokePolicy(r.Context(), tx, school, chi.URLParam(r, "id"), c.UserID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Kebijakan tidak ditemukan atau sudah dicabut")
		return
	}
	if err == nil {
		err = store.Audit(r.Context(), tx, store.AuditEntry{SchoolID: school, Module: p.Module, EntityType: "confidential_access_policy",
			EntityID: p.ID, Action: "revoke", ActorUserID: &c.UserID, ActorRole: &c.ActiveRole,
			Before: map[string]any{"module": p.Module, "role": p.Role}})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencabut kebijakan")
		return
	}
	common.WriteSuccess(w, p, nil)
}

// ListLogs: GET /sensitive/access-logs?module=&student_id=&actor=&from=&to=
func (h *Handler) ListLogs(w http.ResponseWriter, r *http.Request) {
	_, school := common.RequireRole(w, r, common.RoleAdmin, common.RoleSuperadmin, common.RoleAuditor)
	if school == "" {
		return
	}
	q := r.URL.Query()
	page, perPage := common.Page(r)
	list, total, err := ListLogs(r.Context(), h.DB, LogFilter{SchoolID: school, Module: q.Get("module"),
		StudentID: q.Get("student_id"), ActorUserID: q.Get("actor"), From: q.Get("from"), To: q.Get("to")}, page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat log akses")
		return
	}
	ids := []string{}
	for _, l := range list {
		ids = append(ids, l.ActorUserID)
		if l.StudentID != nil {
			ids = append(ids, *l.StudentID)
		}
	}
	if names, err := h.Client.UserNames(r.Context(), school, ids); err == nil {
		for i := range list {
			list[i].ActorName = names[list[i].ActorUserID]
			if list[i].StudentID != nil {
				list[i].StudentName = names[*list[i].StudentID]
			}
		}
	}
	common.WriteSuccess(w, list, common.Meta{Page: page, PerPage: perPage, Total: total})
}
