package campaign

import (
	"errors"
	"net/http"
	"strings"

	"skci/notification/internal/common"
	"skci/notification/internal/provider"
)

type consentChange struct {
	Channel string `json:"channel"`
	OptIn   bool   `json:"opt_in"`
}

// consentView is one channel's effective consent; a missing row is reported
// as opted_in=false (OQ-43 default opt-out).
type consentView struct {
	Channel string `json:"channel"`
	OptedIn bool   `json:"opted_in"`
	Source  string `json:"source,omitempty"`
}

func mergeConsents(stored []Consent) []consentView {
	by := map[string]Consent{}
	for _, c := range stored {
		by[c.Channel] = c
	}
	out := make([]consentView, 0, 3)
	for _, ch := range []string{ChannelInApp, ChannelEmail, ChannelWhatsApp} {
		v := consentView{Channel: ch}
		if c, ok := by[ch]; ok {
			v.OptedIn = c.Status == ConsentOptedIn
			v.Source = c.Source
		}
		out = append(out, v)
	}
	return out
}

// GetMyConsents handles GET /marketing-consents (caller's own consent).
func (h *Handler) GetMyConsents(w http.ResponseWriter, r *http.Request) {
	school, c, ok := schoolOf(w, r)
	if !ok {
		return
	}
	stored, err := h.Svc.Store.ListConsents(r.Context(), school, UserSubject(c.UserID))
	if err != nil {
		writeStoreErr(w, err, "persetujuan")
		return
	}
	common.WriteJSON(w, http.StatusOK, mergeConsents(stored), nil)
}

// PutMyConsents handles PUT /marketing-consents {consents:[{channel,opt_in}]}.
func (h *Handler) PutMyConsents(w http.ResponseWriter, r *http.Request) {
	school, c, ok := schoolOf(w, r)
	if !ok {
		return
	}
	var p struct {
		Consents []consentChange `json:"consents"`
	}
	if !common.Decode(w, r, &p) {
		return
	}
	if len(p.Consents) == 0 {
		common.WriteError(w, http.StatusBadRequest, "minimal satu kanal wajib diisi")
		return
	}
	subject := UserSubject(c.UserID)
	for _, ch := range p.Consents {
		if !ValidChannels[ch.Channel] {
			common.WriteError(w, http.StatusBadRequest, "kanal tidak dikenal")
			return
		}
	}
	for _, ch := range p.Consents {
		status := ConsentOptedOut
		if ch.OptIn {
			status = ConsentOptedIn
		}
		if err := h.Svc.Store.SetConsent(r.Context(), school, subject, c.UserID, ch.Channel, status, SourcePreferences, c.UserID); err != nil {
			writeStoreErr(w, err, "persetujuan")
			return
		}
	}
	stored, err := h.Svc.Store.ListConsents(r.Context(), school, subject)
	if err != nil {
		writeStoreErr(w, err, "persetujuan")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, mergeConsents(stored), nil, "Preferensi info promosi disimpan")
}

// InternalSetConsent handles POST /internal/marketing-consents (admission's
// PPDB form checkbox, admin import).
func (h *Handler) InternalSetConsent(w http.ResponseWriter, r *http.Request) {
	var p struct {
		SchoolID   string `json:"school_id"`
		SubjectKey string `json:"subject_key"`
		UserID     string `json:"user_id"`
		Channel    string `json:"channel"`
		Status     string `json:"status"`
		Source     string `json:"source"`
	}
	if !common.Decode(w, r, &p) {
		return
	}
	validSubject := strings.HasPrefix(p.SubjectKey, "user:") || strings.HasPrefix(p.SubjectKey, "phone:") || strings.HasPrefix(p.SubjectKey, "email:")
	switch {
	case p.SchoolID == "" || !validSubject:
		common.WriteError(w, http.StatusBadRequest, "school_id dan subject_key (user:|phone:|email:) wajib diisi")
		return
	case !ValidChannels[p.Channel]:
		common.WriteError(w, http.StatusBadRequest, "kanal tidak dikenal")
		return
	case p.Status != ConsentOptedIn && p.Status != ConsentOptedOut:
		common.WriteError(w, http.StatusBadRequest, "status harus opted_in atau opted_out")
		return
	case !ValidConsentSources[p.Source] || p.Source == SourcePreferences:
		common.WriteError(w, http.StatusBadRequest, "sumber tidak dikenal")
		return
	}
	if err := h.Svc.Store.SetConsent(r.Context(), p.SchoolID, p.SubjectKey, p.UserID, p.Channel, p.Status, p.Source, ""); err != nil {
		writeStoreErr(w, err, "persetujuan")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"status": p.Status}, nil, "Persetujuan dicatat")
}

// PublicUnsubscribe handles GET /public/unsubscribe?token= (HMAC-signed link
// in every campaign email).
func (h *Handler) PublicUnsubscribe(w http.ResponseWriter, r *http.Request) {
	school, subject, channel, err := VerifyUnsubscribe(h.Svc.UnsubscribeSecret, r.URL.Query().Get("token"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "tautan berhenti berlangganan tidak valid")
		return
	}
	userID := ""
	if strings.HasPrefix(subject, "user:") {
		userID = strings.TrimPrefix(subject, "user:")
	}
	if err := h.Svc.Store.SetConsent(r.Context(), school, subject, userID, channel, ConsentOptedOut, SourceUnsubscribeLink, ""); err != nil {
		writeStoreErr(w, err, "persetujuan")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"channel": channel, "status": ConsentOptedOut}, nil,
		"Anda tidak akan lagi menerima info promosi melalui kanal ini")
}

// WhatsAppWebhook handles POST /webhooks/whatsapp: delivery/read/reply
// statuses and stop keywords. 404 while WhatsApp is disabled.
func (h *Handler) WhatsAppWebhook(w http.ResponseWriter, r *http.Request) {
	if h.Svc.WhatsApp == nil {
		common.WriteError(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	events, err := h.Svc.WhatsApp.ParseStatusWebhook(r)
	if err != nil {
		common.WriteError(w, http.StatusUnauthorized, "webhook tidak valid")
		return
	}
	applied := 0
	for _, e := range events {
		id, school, subject, err := h.Svc.Store.RecipientByProviderMessage(r.Context(), e.ProviderMessageID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			writeStoreErr(w, err, "webhook")
			return
		}
		if err := h.Svc.Store.ApplyWAStatus(r.Context(), id, e.Status, e.At, e.Error); err != nil {
			writeStoreErr(w, err, "webhook")
			return
		}
		if e.Status == provider.WAStatusReplied && h.Svc.WhatsApp.IsStopKeyword(e.Text) {
			userID := ""
			if strings.HasPrefix(subject, "user:") {
				userID = strings.TrimPrefix(subject, "user:")
			}
			if err := h.Svc.Store.SetConsent(r.Context(), school, subject, userID, ChannelWhatsApp, ConsentOptedOut, SourceWAKeyword, ""); err != nil {
				writeStoreErr(w, err, "webhook")
				return
			}
		}
		applied++
	}
	common.WriteJSON(w, http.StatusOK, map[string]int{"applied": applied}, nil)
}
