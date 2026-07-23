package handler

import (
	"net/http"

	"skci/exam/internal/common"
	"skci/exam/internal/httpclient"
	"skci/exam/internal/store"
)

type Handler struct {
	Store    *store.Store
	Internal *httpclient.Client
}

func New(s *store.Store, ic *httpclient.Client) *Handler {
	return &Handler{Store: s, Internal: ic}
}

// requireRole writes 403 and returns false unless the caller's active_role is
// one of allowed.
func requireRole(w http.ResponseWriter, c *common.Claims, allowed ...string) bool {
	if c == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing claims")
		return false
	}
	if common.Contains(allowed, c.ActiveRole) {
		return true
	}
	common.WriteError(w, http.StatusForbidden, "role "+c.ActiveRole+" is not permitted to perform this action")
	return false
}

func writeStoreErr(w http.ResponseWriter, err error) {
	common.WriteError(w, http.StatusInternalServerError, err.Error())
}
