package health

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/config"
	"skci/studentlife/internal/sensitive"
	"skci/studentlife/internal/store"
)

// Handler serves /health/* and the health side of the approval callback.
type Handler struct {
	DB     *sql.DB
	Crypto *sensitive.Service
	Client *client.Client
	Cfg    config.Config
}

// errAccessLog marks a failure to write sensitive_access_logs (fail closed).
var errAccessLog = errors.New("access log write failed")

// level resolves the viewer's access level to one student's health data
// (BR-HLT-01): facts come from identity/academic, policies from the DB.
// Lookup errors fail closed (None).
func (h *Handler) level(ctx context.Context, c *common.Claims, school, studentID string) sensitive.Level {
	var f sensitive.Facts
	switch c.ActiveRole {
	case common.RoleParent:
		ok, err := h.Client.GuardianHasAccess(ctx, c.UserID, studentID, school)
		f.IsGuardian = err == nil && ok
	case common.RoleWalikelas:
		f.IsHomeroom = h.isHomeroomOfStudent(ctx, c.UserID, school, studentID)
	}
	policies, err := sensitive.ActivePolicyRoles(ctx, h.DB, school, sensitive.ModuleHealth)
	if err != nil {
		return sensitive.None
	}
	return sensitive.CanAccess(sensitive.ModuleHealth, c.ActiveRole, f, policies)
}

func (h *Handler) isHomeroomOfStudent(ctx context.Context, userID, school, studentID string) bool {
	clID, err := h.Client.StudentClassroomID(ctx, studentID, school)
	if err != nil || clID == "" {
		return false
	}
	return h.isHomeroomOf(ctx, userID, school, clID)
}

func (h *Handler) isHomeroomOf(ctx context.Context, userID, school, classroomID string) bool {
	cl, err := h.Client.GetClassroom(ctx, classroomID, school)
	return err == nil && cl.HomeroomTeacherUserID != nil && *cl.HomeroomTeacherUserID == userID
}

func (h *Handler) ipHash(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return h.Crypto.BlindIndex(ip, "sensitive_access_logs:ip")
}

// logAccess writes one sensitive_access_logs row (NFR-OPS-03).
func (h *Handler) logAccess(ctx context.Context, q store.Q, r *http.Request, c *common.Claims, school, entityType, entityID, studentID, action string) error {
	if err := sensitive.LogAccess(ctx, q, sensitive.AccessEntry{SchoolID: school, Module: sensitive.ModuleHealth,
		EntityType: entityType, EntityID: entityID, StudentID: studentID, ActorUserID: c.UserID, ActorRole: c.ActiveRole,
		Action: action, IPHash: h.ipHash(r)}); err != nil {
		return errAccessLog
	}
	return nil
}

// deny logs the refused access and answers 404 so the existence of health
// data is not leaked. A failed log write still denies (500).
func (h *Handler) deny(w http.ResponseWriter, r *http.Request, c *common.Claims, school, entityType, entityID, studentID string) {
	if err := h.logAccess(r.Context(), h.DB, r, c, school, entityType, entityID, studentID, sensitive.ActionDeny); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat akses data sensitif")
		return
	}
	common.WriteError(w, http.StatusNotFound, "Data tidak ditemukan")
}

// viewed logs a read and reports false (after writing 500) if logging failed.
func (h *Handler) viewed(w http.ResponseWriter, r *http.Request, c *common.Claims, school, entityType, entityID, studentID string) bool {
	if err := h.logAccess(r.Context(), h.DB, r, c, school, entityType, entityID, studentID, sensitive.ActionView); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat akses data sensitif")
		return false
	}
	return true
}

func (h *Handler) audit(ctx context.Context, q store.Q, c *common.Claims, school, entityType, entityID, action string, after map[string]any) error {
	return store.Audit(ctx, q, store.AuditEntry{SchoolID: school, Module: sensitive.ModuleHealth, EntityType: entityType,
		EntityID: entityID, Action: action, ActorUserID: &c.UserID, ActorRole: &c.ActiveRole, After: after})
}

func (h *Handler) names(ctx context.Context, school string, ids ...string) map[string]string {
	clean := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			clean = append(clean, id)
		}
	}
	names, err := h.Client.UserNames(ctx, school, clean)
	if err != nil {
		return map[string]string{}
	}
	return names
}

func writeValidation(w http.ResponseWriter, err error) bool {
	var ve ErrValidation
	if errors.As(err, &ve) {
		common.WriteError(w, http.StatusUnprocessableEntity, ve.Msg)
		return true
	}
	return false
}

func idempotencyKey(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("Idempotency-Key"))
}

// ListStudents: GET /health/students?q= (autocomplete siswa untuk petugas UKS).
func (h *Handler) ListStudents(w http.ResponseWriter, r *http.Request) {
	_, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	list, err := h.Client.SearchStudents(r.Context(), school, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "Gagal memuat daftar siswa")
		return
	}
	common.WriteSuccess(w, list, nil)
}

// ListClassrooms: GET /health/classrooms (pilihan sasaran program).
func (h *Handler) ListClassrooms(w http.ResponseWriter, r *http.Request) {
	_, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	list, err := h.Client.ListClassrooms(r.Context(), school)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "Gagal memuat daftar kelas")
		return
	}
	common.WriteSuccess(w, list, nil)
}
