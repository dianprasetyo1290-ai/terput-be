package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"skci/identity/internal/client"
	"skci/identity/internal/common"
	"skci/identity/internal/security"
)

// App holds shared dependencies for all handlers.
type App struct {
	DB          *sql.DB
	JWTSecret   string
	InternalKey string
	Exam        *client.ExamClient
	Academic    *client.AcademicClient
	CoreURL     string
	// MFAKey encrypts TOTP secrets at rest (MFA_ENCRYPTION_KEY).
	MFAKey []byte
	// NotificationURL delivers password-reset emails (notification-service).
	NotificationURL string
	// PublicAppURL is the SPA base used to build the reset link.
	PublicAppURL string
	// MFAIssuer is the label authenticator apps show.
	MFAIssuer string
	// SelfURL is identity's internal base URL as other services reach it
	// (approval callbacks). Empty -> IDENTITY_INTERNAL_URL or http://identity:8081.
	SelfURL string
	// HTTPClient for calls to core/notification (nil -> 8 s default).
	HTTPClient *http.Client

	policyCache policyCache
}

func NewApp(db *sql.DB, jwtSecret, internalKey string) *App {
	return &App{
		DB:          db,
		JWTSecret:   jwtSecret,
		InternalKey: internalKey,
		Exam:        client.NewExamClient(),
		Academic:    client.NewAcademicClient(),
		CoreURL:     "http://localhost:8091",
		MFAKey:      security.DeriveKey(jwtSecret),
		MFAIssuer:   "SEKOCI",
	}
}

// RecordSecurityAudit dispatches an asynchronous security audit event to core service.
func (a *App) RecordSecurityAudit(schoolID *string, action, entityID string, actorUserID *string, actorRole *string, before, after any, reason string, correlationID string) {
	if a.CoreURL == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		payload := map[string]any{
			"school_id":      schoolID,
			"service":        "identity",
			"module":         "security",
			"entity_type":    "user",
			"entity_id":      entityID,
			"action":         action,
			"actor_user_id":  actorUserID,
			"actor_role":     actorRole,
			"before":         before,
			"after":          after,
			"reason":         reason,
			"correlation_id": correlationID,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.CoreURL, "/")+"/internal/audit", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Key", a.InternalKey)

		cl := &http.Client{Timeout: 2 * time.Second}
		resp, err := cl.Do(req)
		if err != nil {
			common.CaptureError(err)
			return
		}
		defer resp.Body.Close()
	}()
}

// requireRole rejects the request unless the caller's active_role is one of
// allowed, or the caller is superadmin (superadmin can act as any dashboard
// role's admin-level endpoints unless a handler says otherwise).
func requireRole(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	claims := common.FromCtx(r)
	if claims == nil {
		if common.IsInternal(r) {
			// Trusted sibling-service call authenticated via X-Internal-Key
			// (see internal_auth.go) — only ever true on the specific
			// read-only routes mounted under InternalOrJWTMiddleware in
			// main.go, so bypassing the role check here is safe.
			return true
		}
		common.WriteError(w, http.StatusUnauthorized, "Sesi tidak ditemukan. Silakan masuk kembali")
		return false
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		return true
	}
	for _, a := range allowed {
		if claims.ActiveRole == a {
			return true
		}
	}
	common.WriteError(w, http.StatusForbidden, "Akses ditolak: hanya untuk peran "+strings.Join(allowed, " atau "))
	return false
}

// resolveSchoolScope implements the multi-tenancy rule from CONVENTIONS.md:
// non-superadmin callers are hard-scoped to their own JWT school_id;
// superadmin may pass ?school_id= to filter, or omit it to see all schools.
// Returns (schoolID, isFiltered). isFiltered=false means "no school filter,
// show everything" (only ever true for superadmin).
func resolveSchoolScope(r *http.Request) (schoolID string, isFiltered bool) {
	claims := common.FromCtx(r)
	if claims == nil && common.IsInternal(r) {
		// Trusted sibling-service call: behave like an unfiltered
		// superadmin — an explicit ?school_id= filters, omitting it means
		// "no filter" (only ever reachable on the internal-or-jwt route
		// group in main.go).
		if q := r.URL.Query().Get("school_id"); q != "" {
			return q, true
		}
		return "", false
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		q := r.URL.Query().Get("school_id")
		if q == "" {
			return "", false
		}
		return q, true
	}
	return claims.SchoolID, true
}
