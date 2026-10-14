package handler

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"skci/notification/internal/client"
	"skci/notification/internal/common"
	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

// adminTemplateChannels are the channels an event template can target from
// the admin menu (push carries the in-app text).
var adminTemplateChannels = map[string]bool{
	model.ChannelInApp: true, model.ChannelEmail: true, model.ChannelWhatsApp: true,
}

// requireTemplateAdmin gates the BUG-088 admin template menu to school admins
// and superadmins and resolves the tenant.
func requireTemplateAdmin(w http.ResponseWriter, r *http.Request) (*common.Claims, string, bool) {
	claims := common.FromCtx(r)
	if claims.ActiveRole != roleAdmin && claims.ActiveRole != roleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Hanya admin yang dapat mengelola template notifikasi")
		return nil, "", false
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return nil, "", false
	}
	return claims, schoolID, true
}

// templateVariables is the GET /notification-templates/variables response.
type templateVariables struct {
	Common []string            `json:"common"`
	Events map[string][]string `json:"events"`
	Keys   []string            `json:"keys"`
}

// AdminTemplateVariables handles GET /notification-templates/variables: the
// placeholders each event key accepts, so the editor can offer them.
func (h *Handler) AdminTemplateVariables(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := requireTemplateAdmin(w, r); !ok {
		return
	}
	keys := make([]string, 0, len(model.EventTemplateVars))
	events := make(map[string][]string, len(model.EventTemplateVars))
	for k := range model.EventTemplateVars {
		keys = append(keys, k)
		events[k] = model.AllowedTemplateVars(k)
	}
	sort.Strings(keys)
	common.WriteJSON(w, http.StatusOK, templateVariables{Common: model.CommonTemplateVars, Events: events, Keys: keys}, nil)
}

// AdminListTemplates handles GET /notification-templates?key=: every version,
// newest first per key+channel (the first row of a pair is the one in use).
func (h *Handler) AdminListTemplates(w http.ResponseWriter, r *http.Request) {
	_, schoolID, ok := requireTemplateAdmin(w, r)
	if !ok {
		return
	}
	items, err := h.Store.ListNotificationTemplates(r.Context(), schoolID, strings.TrimSpace(r.URL.Query().Get("key")))
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat template notifikasi")
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// AdminCreateTemplate handles POST /notification-templates. Every save is a
// new version; the newest version of a key+channel is what gets sent, and
// older versions stay as history. Unknown {{variables}} are rejected.
func (h *Handler) AdminCreateTemplate(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := requireTemplateAdmin(w, r)
	if !ok {
		return
	}
	var p templatePayload
	if !common.Decode(w, r, &p) {
		return
	}
	key := strings.TrimSpace(p.Key)
	switch {
	case key == "":
		common.WriteError(w, http.StatusBadRequest, "jenis kejadian (key) wajib diisi")
		return
	case !adminTemplateChannels[p.Channel]:
		common.WriteError(w, http.StatusBadRequest, "kanal harus in_app, email, atau whatsapp")
		return
	case strings.TrimSpace(p.Body) == "":
		common.WriteError(w, http.StatusBadRequest, "isi template wajib diisi")
		return
	case p.Channel == model.ChannelEmail && strings.TrimSpace(p.Subject) == "":
		common.WriteError(w, http.StatusBadRequest, "subjek wajib diisi untuk template email")
		return
	}
	if err := model.ValidateTemplate(key, p.Subject, p.Body); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	t, err := h.Store.CreateNotificationTemplate(r.Context(), store.TemplateInput{
		SchoolID: schoolID, Key: key, Channel: p.Channel, Subject: p.Subject, Body: p.Body,
	})
	if errors.Is(err, store.ErrConflict) {
		common.WriteError(w, http.StatusConflict, "versi template baru saja dibuat oleh pengguna lain, muat ulang lalu coba lagi")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan template notifikasi")
		return
	}
	h.audit(r.Context(), client.AuditEvent{
		SchoolID: schoolID, EntityType: "notification_template", EntityID: t.ID, Action: "create",
		ActorUserID: claims.UserID, ActorRole: claims.ActiveRole, After: t,
	})
	common.WriteJSONMsg(w, http.StatusCreated, t, nil, "Template notifikasi disimpan sebagai versi baru")
}

// AdminDeleteTemplate handles DELETE /notification-templates/{id}; the
// previous version (if any) becomes the one in use again.
func (h *Handler) AdminDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := requireTemplateAdmin(w, r)
	if !ok {
		return
	}
	id := param(r, "id")
	before, err := h.Store.GetNotificationTemplate(r.Context(), id, schoolID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "template tidak ditemukan")
		return
	}
	if err == nil {
		err = h.Store.DeleteNotificationTemplate(r.Context(), id, schoolID)
	}
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "template tidak ditemukan")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal menghapus template notifikasi")
		return
	}
	h.audit(r.Context(), client.AuditEvent{
		SchoolID: schoolID, EntityType: "notification_template", EntityID: id, Action: "delete",
		ActorUserID: claims.UserID, ActorRole: claims.ActiveRole, Before: before,
	})
	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"id": id}, nil, "Template notifikasi dihapus")
}
