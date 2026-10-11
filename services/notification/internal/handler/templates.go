package handler

import (
	"errors"
	"net/http"
	"strings"

	"skci/notification/internal/common"
	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

// templatePayload is the create/update body for a notification template.
type templatePayload struct {
	SchoolID string `json:"school_id"`
	Key      string `json:"key"`
	Version  int    `json:"version"`
	Channel  string `json:"channel"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

// templateSchoolID resolves the tenant for an internal template call: explicit
// ?school_id=, else the body's school_id.
func templateSchoolID(r *http.Request, bodySchoolID string) string {
	if s := strings.TrimSpace(r.URL.Query().Get("school_id")); s != "" {
		return s
	}
	return strings.TrimSpace(bodySchoolID)
}

// InternalListTemplates handles GET /internal/notification-templates?school_id=&key=.
func (h *Handler) InternalListTemplates(w http.ResponseWriter, r *http.Request) {
	schoolID := templateSchoolID(r, "")
	if !requireSchool(w, schoolID) {
		return
	}
	items, err := h.Store.ListNotificationTemplates(r.Context(), schoolID, r.URL.Query().Get("key"))
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to list templates")
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// InternalGetTemplate handles GET /internal/notification-templates/{id}?school_id=.
func (h *Handler) InternalGetTemplate(w http.ResponseWriter, r *http.Request) {
	schoolID := templateSchoolID(r, "")
	if !requireSchool(w, schoolID) {
		return
	}
	t, err := h.Store.GetNotificationTemplate(r.Context(), param(r, "id"), schoolID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "template not found")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load template")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

// InternalCreateTemplate handles POST /internal/notification-templates.
// Version <= 0 means "next version for this key+channel".
func (h *Handler) InternalCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var payload templatePayload
	if !common.Decode(w, r, &payload) {
		return
	}
	schoolID := templateSchoolID(r, payload.SchoolID)
	if !requireSchool(w, schoolID) {
		return
	}
	if !validTemplatePayload(w, payload) {
		return
	}
	t, err := h.Store.CreateNotificationTemplate(r.Context(), store.TemplateInput{
		SchoolID: schoolID,
		Key:      strings.TrimSpace(payload.Key),
		Version:  payload.Version,
		Channel:  payload.Channel,
		Subject:  payload.Subject,
		Body:     payload.Body,
	})
	if errors.Is(err, store.ErrConflict) {
		common.WriteError(w, http.StatusConflict, "template version already exists for this key and channel")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to create template")
		return
	}
	common.WriteJSONMsg(w, http.StatusCreated, t, nil, "Template notifikasi dibuat")
}

// InternalUpdateTemplate handles PUT /internal/notification-templates/{id}.
func (h *Handler) InternalUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	var payload templatePayload
	if !common.Decode(w, r, &payload) {
		return
	}
	schoolID := templateSchoolID(r, payload.SchoolID)
	if !requireSchool(w, schoolID) {
		return
	}
	if strings.TrimSpace(payload.Body) == "" {
		common.WriteError(w, http.StatusBadRequest, "body is required")
		return
	}
	t, err := h.Store.UpdateNotificationTemplate(r.Context(), param(r, "id"), schoolID, store.TemplateInput{
		Subject: payload.Subject,
		Body:    payload.Body,
	})
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "template not found")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to update template")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, t, nil, "Template notifikasi diperbarui")
}

// InternalDeleteTemplate handles DELETE /internal/notification-templates/{id}.
func (h *Handler) InternalDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	schoolID := templateSchoolID(r, "")
	if !requireSchool(w, schoolID) {
		return
	}
	err := h.Store.DeleteNotificationTemplate(r.Context(), param(r, "id"), schoolID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "template not found")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to delete template")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"id": param(r, "id")}, nil, "Template notifikasi dihapus")
}

func validTemplatePayload(w http.ResponseWriter, p templatePayload) bool {
	if strings.TrimSpace(p.Key) == "" {
		common.WriteError(w, http.StatusBadRequest, "key is required")
		return false
	}
	if p.Channel == "" {
		common.WriteError(w, http.StatusBadRequest, "channel is required")
		return false
	}
	if !model.ValidChannels[p.Channel] {
		common.WriteError(w, http.StatusBadRequest, "unknown channel "+p.Channel+" (want in_app|email|whatsapp)")
		return false
	}
	if strings.TrimSpace(p.Body) == "" {
		common.WriteError(w, http.StatusBadRequest, "body is required")
		return false
	}
	return true
}
