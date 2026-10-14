package handler

import (
	"net/http"

	"skci/notification/internal/common"
	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

// preferencePayload is one channel switch in a PUT body. Both the object form
// ({"channel":"email","enabled":true}) and the list form
// ({"preferences":[...]}) are accepted.
type preferencePayload struct {
	Channel     string             `json:"channel"`
	Enabled     *bool              `json:"enabled"`
	Preferences []preferenceChange `json:"preferences"`
}

type preferenceChange struct {
	Channel string `json:"channel"`
	Enabled *bool  `json:"enabled"`
}

// GetPreferences handles GET /notification-preferences: the caller's effective
// switches — stored rows merged with the defaults for untouched channels.
func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "sesi tidak valid, silakan masuk kembali")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	stored, err := h.Store.ListNotificationPreferences(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat preferensi")
		return
	}
	common.WriteJSON(w, http.StatusOK, mergePreferences(stored), nil)
}

// PutPreferences handles PUT /notification-preferences. Unknown channels are
// rejected (400) rather than silently stored.
func (h *Handler) PutPreferences(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "sesi tidak valid, silakan masuk kembali")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}

	var payload preferencePayload
	if !common.Decode(w, r, &payload) {
		return
	}

	changes := payload.Preferences
	if payload.Channel != "" {
		changes = append(changes, preferenceChange{Channel: payload.Channel, Enabled: payload.Enabled})
	}
	if len(changes) == 0 {
		common.WriteError(w, http.StatusBadRequest, "minimal satu preferensi kanal wajib diisi")
		return
	}

	seen := map[string]bool{}
	inputs := make([]store.PreferenceInput, 0, len(changes))
	for _, c := range changes {
		if !model.ValidChannels[c.Channel] {
			common.WriteError(w, http.StatusBadRequest, "kanal tidak dikenal: "+c.Channel+" (pilih in_app, email, atau whatsapp)")
			return
		}
		if seen[c.Channel] {
			continue
		}
		seen[c.Channel] = true
		enabled := false
		if c.Enabled != nil {
			enabled = *c.Enabled
		}
		inputs = append(inputs, store.PreferenceInput{Channel: c.Channel, Enabled: enabled})
	}

	if _, err := h.Store.UpsertNotificationPreferences(r.Context(), schoolID, claims.UserID, inputs); err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan preferensi")
		return
	}

	stored, err := h.Store.ListNotificationPreferences(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat preferensi")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, mergePreferences(stored), nil, "Preferensi notifikasi disimpan")
}

// mergePreferences returns all three channels in a stable order, using the
// stored switch when present and the model default otherwise.
func mergePreferences(stored []model.NotificationPreference) []model.NotificationPreference {
	byChannel := make(map[string]model.NotificationPreference, len(stored))
	for _, p := range stored {
		byChannel[p.Channel] = p
	}
	out := make([]model.NotificationPreference, 0, len(model.ValidChannels))
	for _, ch := range []string{model.ChannelInApp, model.ChannelEmail, model.ChannelWhatsApp} {
		if p, ok := byChannel[ch]; ok {
			out = append(out, p)
			continue
		}
		out = append(out, model.NotificationPreference{Channel: ch, Enabled: model.DefaultPreference(ch)})
	}
	return out
}
