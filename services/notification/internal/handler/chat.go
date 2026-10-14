package handler

import (
	"errors"
	"net/http"
	"strings"

	"skci/notification/internal/common"
	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

// createThreadPayload is the POST /chat/threads body. Participants may be given
// as objects (participants) or bare ids (participant_user_ids); the caller is
// always added as an owner.
type createThreadPayload struct {
	Subject            string               `json:"subject"`
	Participants       []participantPayload `json:"participants"`
	ParticipantUserIDs []string             `json:"participant_user_ids"`
}

type participantPayload struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// createMessagePayload is the POST /chat/threads/{id}/messages body.
type createMessagePayload struct {
	Body          string `json:"body"`
	AttachmentKey string `json:"attachment_key"`
}

// ListThreads handles GET /chat/threads — only threads where the caller is a
// participant, in the caller's school.
func (h *Handler) ListThreads(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "sesi tidak valid, silakan masuk kembali")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	page, perPage := common.Page(r)
	threads, total, err := h.Store.ListChatThreadsForUser(r.Context(), schoolID, claims.UserID, page, perPage)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat percakapan")
		return
	}
	common.WriteJSON(w, http.StatusOK, threads, common.NewPageMeta(page, perPage, total))
}

// CreateThread handles POST /chat/threads.
func (h *Handler) CreateThread(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "sesi tidak valid, silakan masuk kembali")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}

	var payload createThreadPayload
	if !common.Decode(w, r, &payload) {
		return
	}
	if strings.TrimSpace(payload.Subject) == "" {
		common.WriteError(w, http.StatusBadRequest, "subjek wajib diisi")
		return
	}

	participants := make([]store.ParticipantInput, 0, len(payload.Participants)+len(payload.ParticipantUserIDs))
	for _, p := range payload.Participants {
		if strings.TrimSpace(p.UserID) == "" {
			continue
		}
		participants = append(participants, store.ParticipantInput{UserID: p.UserID, Role: normalizeChatRole(p.Role)})
	}
	for _, id := range payload.ParticipantUserIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		participants = append(participants, store.ParticipantInput{UserID: id, Role: model.ChatRoleMember})
	}

	if msg, ok := h.checkGeneralThread(r.Context(), schoolID, claims, participants); !ok {
		common.WriteError(w, http.StatusForbidden, msg)
		return
	}

	thread, err := h.Store.CreateChatThread(r.Context(), store.CreateThreadInput{
		SchoolID:     schoolID,
		Subject:      strings.TrimSpace(payload.Subject),
		CreatedBy:    claims.UserID,
		Participants: participants,
	})
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat percakapan")
		return
	}
	common.WriteJSONMsg(w, http.StatusCreated, thread, nil, "Thread dibuat")
}

// ListMessages handles GET /chat/threads/{id}/messages. Only participants may
// read the thread.
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "sesi tidak valid, silakan masuk kembali")
		return
	}
	if _, ok := h.authorizeThread(w, r, claims, param(r, "id")); !ok {
		return
	}
	page, perPage := common.Page(r)
	messages, total, err := h.Store.ListChatMessages(r.Context(), param(r, "id"), page, perPage)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat pesan")
		return
	}
	common.WriteJSON(w, http.StatusOK, messages, common.NewPageMeta(page, perPage, total))
}

// CreateMessage handles POST /chat/threads/{id}/messages.
func (h *Handler) CreateMessage(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "sesi tidak valid, silakan masuk kembali")
		return
	}
	thread, ok := h.authorizeThread(w, r, claims, param(r, "id"))
	if !ok {
		return
	}
	if thread.Closed() {
		common.WriteError(w, http.StatusConflict, "percakapan sudah ditutup")
		return
	}

	var payload createMessagePayload
	if !common.Decode(w, r, &payload) {
		return
	}
	if strings.TrimSpace(payload.Body) == "" && strings.TrimSpace(payload.AttachmentKey) == "" {
		common.WriteError(w, http.StatusBadRequest, "isi pesan atau lampiran wajib diisi")
		return
	}

	key := strings.TrimSpace(payload.AttachmentKey)
	if key != "" && !strings.HasPrefix(key, "chat/"+thread.ID+"/") {
		// A key may only reference a file uploaded to this same thread.
		common.WriteError(w, http.StatusBadRequest, "lampiran bukan milik percakapan ini")
		return
	}
	h.postMessage(w, r, thread, store.CreateMessageInput{
		ThreadID:      thread.ID,
		SenderUserID:  claims.UserID,
		Body:          payload.Body,
		AttachmentKey: strptr(key),
	})
}

// MarkThreadRead handles POST /chat/threads/{id}/read. Idempotent: a second call
// records no new receipts.
func (h *Handler) MarkThreadRead(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "sesi tidak valid, silakan masuk kembali")
		return
	}
	thread, ok := h.authorizeThread(w, r, claims, param(r, "id"))
	if !ok {
		return
	}
	marked, err := h.Store.MarkChatThreadRead(r.Context(), thread.ID, claims.UserID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal menandai percakapan dibaca")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, map[string]int{"marked": len(marked)}, nil, "Pesan ditandai sudah dibaca")
}

// authorizeThread enforces the chat access rule: the thread must exist in the
// caller's school AND the caller must be a named participant. A thread from
// another school resolves to "not found"; a same-school thread the caller is
// not part of resolves to 403.
func (h *Handler) authorizeThread(w http.ResponseWriter, r *http.Request, claims *common.Claims, threadID string) (*model.ChatThread, bool) {
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return nil, false
	}
	thread, err := h.Store.GetChatThread(r.Context(), threadID, schoolID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "percakapan tidak ditemukan")
		return nil, false
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat percakapan")
		return nil, false
	}
	ok, err := h.Store.IsChatParticipant(r.Context(), threadID, claims.UserID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memverifikasi akses percakapan")
		return nil, false
	}
	if !ok {
		common.WriteError(w, http.StatusForbidden, "Anda bukan peserta percakapan ini")
		return nil, false
	}
	return thread, true
}

func normalizeChatRole(role string) string {
	if role == model.ChatRoleOwner {
		return model.ChatRoleOwner
	}
	return model.ChatRoleMember
}
