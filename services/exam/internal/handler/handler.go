package handler

import (
	"net/http"
	"time"

	"skci/exam/internal/common"
	"skci/exam/internal/httpclient"
	"skci/exam/internal/store"
)

// DefaultParticipantTokenTTL is the fallback lifetime of an applicant
// participant token when neither the request nor the config sets one, kept
// identical to config.participantTTL's own fallback.
const DefaultParticipantTokenTTL = 15 * time.Minute

type Handler struct {
	Store             *store.Store
	Internal          *httpclient.Client
	ParticipantSecret string
	// ParticipantTokenTTL is the default token lifetime for
	// POST /internal/admission/exams/{id}/participants/access.
	ParticipantTokenTTL time.Duration
}

// New builds the handler. participantSecret is the HMAC key used to sign (and
// verify) admission applicant CBT tokens — config.ParticipantSecret, sourced
// from EXAM_PARTICIPANT_SECRET (see internal/config). It is required and may
// never be empty: an empty key would make every /participant/** route accept a
// self-minted HS256 token, so this fails loudly instead of starting with one.
// main.go already guarantees a non-empty value via config.Validate.
func New(s *store.Store, ic *httpclient.Client, participantSecret string, participantTTL time.Duration) *Handler {
	if participantSecret == "" {
		panic("handler.New: participant token secret must not be empty (set EXAM_PARTICIPANT_SECRET)")
	}
	if participantTTL <= 0 {
		participantTTL = DefaultParticipantTokenTTL
	}
	return &Handler{Store: s, Internal: ic, ParticipantSecret: participantSecret, ParticipantTokenTTL: participantTTL}
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
