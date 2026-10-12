package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"time"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/config"
	"skci/core/internal/model"
	"skci/core/internal/servicedesk"
	"skci/core/internal/store"
)

type Handler struct {
	Store  store.Store
	Config config.Config
	Client *client.Client
	wg     sync.WaitGroup
	// ServiceDesk is the nil-safe BR-SD-01 hook (T11): menu 81 requests
	// become tickets when the school runs the service desk.
	ServiceDesk *servicedesk.Service
}

func New(st store.Store, cfg config.Config, cl *client.Client) *Handler {
	return &Handler{Store: st, Config: cfg, Client: cl}
}

// bg runs a side effect (callback, notification) after the response, on its
// own context: r.Context() is cancelled as soon as the handler returns.
func (h *Handler) bg(fn func(ctx context.Context)) {
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		fn(ctx)
	}()
}

// Wait blocks until background side effects finish (tests, graceful shutdown).
func (h *Handler) Wait() { h.wg.Wait() }

// ClientIP extracts the bare client IP (middleware.RealIP has already applied
// X-Forwarded-For / X-Real-IP to RemoteAddr).
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// HashIP stores a SHA-256 of the IP, never the IP itself. Empty in, nil out.
func HashIP(ip string) *string {
	if ip == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(ip))
	s := hex.EncodeToString(sum[:])
	return &s
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidRe.MatchString(s) }

func ptr[T any](v T) *T { return &v }

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// writeStoreErr maps store sentinel errors to HTTP statuses.
func writeStoreErr(w http.ResponseWriter, err error, notFound string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		common.WriteError(w, http.StatusNotFound, notFound)
	case errors.Is(err, store.ErrConflict):
		common.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrInvalidRequest):
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		log.Printf("store error: %v", err)
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}

// tenant returns the caller's claims and school, writing 401/400 on failure.
func tenant(w http.ResponseWriter, r *http.Request) (*common.Claims, string, bool) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return nil, "", false
	}
	schoolID := common.TenantSchoolID(r)
	if !isUUID(schoolID) {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return nil, "", false
	}
	return claims, schoolID, true
}

func isAdmin(c *common.Claims) bool {
	return c.ActiveRole == common.RoleAdmin || c.ActiveRole == common.RoleSuperadmin
}

func userInfo(c *common.Claims) model.UserInfo {
	return model.UserInfo{UserID: c.GetUserID(), ActiveRole: c.ActiveRole, Roles: c.Roles}
}

// delegationsFor loads the caller's active incoming delegations with each
// delegator's roles in this school (from identity), so role-based steps can
// be decided on their behalf. A delegator whose roles cannot be resolved still
// counts for user/direct_supervisor steps.
func (h *Handler) delegationsFor(ctx context.Context, schoolID, userID string) []model.ActiveDelegation {
	dels, err := h.Store.GetActiveDelegationsForUser(ctx, schoolID, userID)
	if err != nil {
		log.Printf("load delegations for %s: %v", userID, err)
		return nil
	}
	for i := range dels {
		if h.Client == nil {
			break
		}
		roles, err := h.Client.UserRoles(ctx, schoolID, dels[i].FromUserID)
		if err != nil {
			log.Printf("resolve roles of delegator %s: %v", dels[i].FromUserID, err)
			continue
		}
		dels[i].FromUserRoles = roles
	}
	return dels
}

// audit writes a core-originated audit event stamped with the caller.
func (h *Handler) audit(r *http.Request, ev model.AuditEvent) {
	ev.Service = "core"
	if c := common.FromCtx(r); c != nil {
		ev.ActorUserID = strPtr(c.GetUserID())
		ev.ActorRole = strPtr(c.ActiveRole)
	}
	if ev.IPHash == nil {
		ev.IPHash = HashIP(ClientIP(r))
	}
	if err := h.Store.InsertAudit(r.Context(), ev); err != nil {
		log.Printf("audit %s/%s: %v", ev.Module, ev.Action, err)
		common.CaptureError(err)
	}
}

func inList(list []string, v string) bool { return slices.Contains(list, v) }
