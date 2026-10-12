package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"skci/notification/internal/chat"
	"skci/notification/internal/client"
	"skci/notification/internal/common"
	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

// Role names (final per T9 §2).
const (
	roleGuru          = "guru"
	roleWalikelas     = "walikelas"
	roleOrangTua      = "orang_tua"
	roleKepalaSekolah = "kepala_sekolah"
	roleAdmin         = "admin"
	roleSuperadmin    = "superadmin"
)

// msgRelationUnverified is returned when a relation lookup fails (fail closed).
const msgRelationUnverified = "Relasi guru–orang tua tidak dapat diverifikasi saat ini; coba lagi nanti"

// attachmentMimes is the allowed attachment set (PDF/JPG/PNG).
var attachmentMimes = map[string]bool{"application/pdf": true, "image/jpeg": true, "image/png": true}

func isTeacher(role string) bool { return role == roleGuru || role == roleWalikelas }

func hasAny(roles []string, want ...string) bool {
	for _, r := range roles {
		for _, w := range want {
			if r == w {
				return true
			}
		}
	}
	return false
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Handler) loc() *time.Location {
	if h.Location != nil {
		return h.Location
	}
	return time.UTC
}

func (h *Handler) audit(ctx context.Context, e client.AuditEvent) {
	if h.Audit != nil {
		h.Audit(ctx, e)
	}
}

// teacherGuardianRelated reports whether the teacher teaches (or is wali
// kelas of) at least one active child of the guardian. Errors fail closed.
func (h *Handler) teacherGuardianRelated(ctx context.Context, schoolID, teacherID, guardianID string) (bool, error) {
	if h.Relations == nil {
		return false, errors.New("relations not configured")
	}
	students, err := h.Relations.TeacherStudentIDs(ctx, schoolID, teacherID)
	if err != nil {
		return false, err
	}
	children, err := h.Relations.GuardianChildren(ctx, schoolID, guardianID)
	if err != nil {
		return false, err
	}
	for _, c := range children {
		if students[c] {
			return true, nil
		}
	}
	return false, nil
}

// checkGeneralThread tightens the 0002 CreateThread (BR-COM-02, fail
// closed): a general thread pairing orang_tua with guru/walikelas must pass
// the same relation check as a teacher–parent thread. Admin ↔ parent
// threads are unchanged. Returns ("", true) when allowed.
func (h *Handler) checkGeneralThread(ctx context.Context, schoolID string, claims *common.Claims, participants []store.ParticipantInput) (string, bool) {
	caller := claims.ActiveRole
	if !isTeacher(caller) && caller != roleOrangTua {
		return "", true
	}
	for _, p := range participants {
		if p.UserID == claims.UserID {
			continue
		}
		if h.Relations == nil {
			return msgRelationUnverified, false
		}
		roles, err := h.Relations.UserRoles(ctx, schoolID, p.UserID)
		if err != nil {
			return msgRelationUnverified, false
		}
		var teacherID, guardianID string
		switch {
		case isTeacher(caller) && hasAny(roles, roleOrangTua):
			teacherID, guardianID = claims.UserID, p.UserID
		case caller == roleOrangTua && hasAny(roles, roleGuru, roleWalikelas):
			teacherID, guardianID = p.UserID, claims.UserID
		default:
			continue
		}
		ok, err := h.teacherGuardianRelated(ctx, schoolID, teacherID, guardianID)
		if err != nil {
			return msgRelationUnverified, false
		}
		if !ok {
			if isTeacher(caller) {
				return chat.MsgTeacherNotRelated, false
			}
			return chat.MsgPeerNotTeacher, false
		}
	}
	return "", true
}

type teacherParentPayload struct {
	StudentUserID  string `json:"student_user_id"`
	GuardianUserID string `json:"guardian_user_id"`
	TeacherUserID  string `json:"teacher_user_id"`
	Subject        string `json:"subject"`
}

// CreateTeacherParentThread handles POST /chat/teacher-parent/threads
// (FR-148-001, AC-148-01). Teachers name {student, guardian?}; parents name
// {student, teacher}. An open thread for the same trio is returned (200).
func (h *Handler) CreateTeacherParentThread(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	var p teacherParentPayload
	if !common.Decode(w, r, &p) {
		return
	}
	if strings.TrimSpace(p.StudentUserID) == "" {
		common.WriteError(w, http.StatusBadRequest, "student_user_id wajib diisi")
		return
	}
	if h.Relations == nil {
		common.WriteError(w, http.StatusForbidden, msgRelationUnverified)
		return
	}
	ctx := r.Context()
	var teacherID, guardianID string
	switch {
	case isTeacher(claims.ActiveRole):
		teacherID = claims.UserID
		guardianID = strings.TrimSpace(p.GuardianUserID)
		if guardianID == "" {
			guardians, err := h.Relations.StudentGuardians(ctx, schoolID, p.StudentUserID)
			if err != nil {
				common.WriteError(w, http.StatusForbidden, msgRelationUnverified)
				return
			}
			for _, g := range guardians {
				if guardianID == "" || g.IsPrimary {
					guardianID = g.GuardianUserID
				}
			}
			if guardianID == "" {
				common.WriteError(w, http.StatusUnprocessableEntity, "Siswa ini belum memiliki akun orang tua yang terhubung")
				return
			}
		}
	case claims.ActiveRole == roleOrangTua:
		guardianID = claims.UserID
		teacherID = strings.TrimSpace(p.TeacherUserID)
		if teacherID == "" {
			common.WriteError(w, http.StatusBadRequest, "teacher_user_id wajib diisi")
			return
		}
	default:
		common.WriteError(w, http.StatusForbidden, "Hanya guru, wali kelas, dan orang tua yang dapat memulai percakapan ini")
		return
	}

	students, err := h.Relations.TeacherStudentIDs(ctx, schoolID, teacherID)
	if err != nil {
		common.WriteError(w, http.StatusForbidden, msgRelationUnverified)
		return
	}
	guardianOK, err := h.Relations.GuardianHasAccess(ctx, schoolID, guardianID, p.StudentUserID)
	if err != nil {
		common.WriteError(w, http.StatusForbidden, msgRelationUnverified)
		return
	}
	if ok, msg := chat.CanStartTeacherParent(students, p.StudentUserID, guardianOK, isTeacher(claims.ActiveRole)); !ok {
		common.WriteError(w, http.StatusForbidden, msg)
		return
	}

	existing, err := h.Store.FindActiveTeacherParentThread(ctx, schoolID, teacherID, guardianID, p.StudentUserID)
	if err == nil {
		common.WriteJSONMsg(w, http.StatusOK, existing, nil, "Percakapan sudah ada")
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load thread")
		return
	}
	subject := strings.TrimSpace(p.Subject)
	if subject == "" {
		subject = "Percakapan guru–orang tua"
		if name := h.Relations.UserName(ctx, p.StudentUserID); name != "" {
			subject += ": " + name
		}
	}
	other := guardianID
	if claims.UserID == guardianID {
		other = teacherID
	}
	thread, err := h.Store.CreateChatThread(ctx, store.CreateThreadInput{
		SchoolID: schoolID, Subject: subject, CreatedBy: claims.UserID,
		Participants: []store.ParticipantInput{{UserID: other, Role: model.ChatRoleMember}},
		Kind:         model.ChatKindTeacherParent, StudentUserID: p.StudentUserID, TeacherUserID: teacherID, GuardianUserID: guardianID,
	})
	if err != nil {
		// A concurrent create of the same trio hits the partial UNIQUE index.
		if again, findErr := h.Store.FindActiveTeacherParentThread(ctx, schoolID, teacherID, guardianID, p.StudentUserID); findErr == nil {
			common.WriteJSONMsg(w, http.StatusOK, again, nil, "Percakapan sudah ada")
			return
		}
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to create thread")
		return
	}
	common.WriteJSONMsg(w, http.StatusCreated, thread, nil, "Percakapan dimulai")
}

type personView struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Label  string `json:"label,omitempty"`
}

// TeacherParentStudents handles GET /chat/teacher-parent/students: the
// students a teacher teaches, or a parent's own children.
func (h *Handler) TeacherParentStudents(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	if h.Relations == nil {
		common.WriteError(w, http.StatusBadGateway, msgRelationUnverified)
		return
	}
	var ids []string
	switch {
	case isTeacher(claims.ActiveRole):
		set, err := h.Relations.TeacherStudentIDs(r.Context(), schoolID, claims.UserID)
		if err != nil {
			common.WriteError(w, http.StatusBadGateway, msgRelationUnverified)
			return
		}
		for id := range set {
			ids = append(ids, id)
		}
	case claims.ActiveRole == roleOrangTua:
		children, err := h.Relations.GuardianChildren(r.Context(), schoolID, claims.UserID)
		if err != nil {
			common.WriteError(w, http.StatusBadGateway, msgRelationUnverified)
			return
		}
		ids = children
	default:
		common.WriteError(w, http.StatusForbidden, "Hanya guru, wali kelas, dan orang tua")
		return
	}
	out := make([]personView, 0, len(ids))
	for _, id := range ids {
		out = append(out, personView{UserID: id, Name: h.Relations.UserName(r.Context(), id)})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// TeacherParentContacts handles GET /chat/teacher-parent/contacts?student_user_id=:
// the teachers a parent may contact about a child, or the guardians a teacher
// may contact about a student they teach. Unrelated callers get 403.
func (h *Handler) TeacherParentContacts(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	studentID := strings.TrimSpace(r.URL.Query().Get("student_user_id"))
	if studentID == "" {
		common.WriteError(w, http.StatusBadRequest, "student_user_id wajib diisi")
		return
	}
	if h.Relations == nil {
		common.WriteError(w, http.StatusForbidden, msgRelationUnverified)
		return
	}
	ctx := r.Context()
	out := []personView{}
	switch {
	case isTeacher(claims.ActiveRole):
		set, err := h.Relations.TeacherStudentIDs(ctx, schoolID, claims.UserID)
		if err != nil {
			common.WriteError(w, http.StatusForbidden, msgRelationUnverified)
			return
		}
		if !set[studentID] {
			common.WriteError(w, http.StatusForbidden, chat.MsgTeacherNotRelated)
			return
		}
		guardians, err := h.Relations.StudentGuardians(ctx, schoolID, studentID)
		if err != nil {
			common.WriteError(w, http.StatusForbidden, msgRelationUnverified)
			return
		}
		for _, g := range guardians {
			out = append(out, personView{UserID: g.GuardianUserID, Name: h.Relations.UserName(ctx, g.GuardianUserID), Label: g.Relationship})
		}
	case claims.ActiveRole == roleOrangTua:
		ok, err := h.Relations.GuardianHasAccess(ctx, schoolID, claims.UserID, studentID)
		if err != nil {
			common.WriteError(w, http.StatusForbidden, msgRelationUnverified)
			return
		}
		if !ok {
			common.WriteError(w, http.StatusForbidden, chat.MsgGuardianNotRelated)
			return
		}
		teachers, err := h.Relations.StudentTeachers(ctx, schoolID, studentID)
		if err != nil {
			common.WriteError(w, http.StatusForbidden, msgRelationUnverified)
			return
		}
		for _, t := range teachers {
			out = append(out, personView{UserID: t.UserID, Name: h.Relations.UserName(ctx, t.UserID), Label: t.Label})
		}
	default:
		common.WriteError(w, http.StatusForbidden, "Hanya guru, wali kelas, dan orang tua")
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// postMessage stores a message, applies the service-hours rule and notifies
// the other participants. It writes the 201 response.
func (h *Handler) postMessage(w http.ResponseWriter, r *http.Request, thread *model.ChatThread, in store.CreateMessageInput) {
	ctx := r.Context()
	settings, err := h.Store.GetChatSettings(ctx, thread.SchoolID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load chat settings")
		return
	}
	outside, notice := chat.ServiceWindow(&chat.Settings{Start: settings.ServiceStart, End: settings.ServiceEnd, Days: settings.ServiceDays}, h.now(), h.loc())
	in.OutsideServiceHours = outside
	msg, err := h.Store.CreateChatMessage(ctx, in)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to post message")
		return
	}
	h.notifyParticipants(ctx, thread, msg)
	var meta map[string]any
	if notice != "" {
		meta = map[string]any{"service_notice": notice}
	}
	common.WriteJSONMsg(w, http.StatusCreated, msg, meta, "Pesan terkirim")
}

// notifyParticipants creates an in-app notification for every other
// participant (FR-148-002). Idempotent per (message, user).
func (h *Handler) notifyParticipants(ctx context.Context, thread *model.ChatThread, msg *model.ChatMessage) {
	preview := msg.Body
	if len([]rune(preview)) > 120 {
		preview = string([]rune(preview)[:120]) + "…"
	}
	if preview == "" && msg.AttachmentName != nil {
		preview = "Lampiran: " + *msg.AttachmentName
	}
	data, _ := json.Marshal(map[string]string{"thread_id": thread.ID, "message_id": msg.ID})
	for _, p := range thread.Participants {
		if p.UserID == msg.SenderUserID {
			continue
		}
		key := "chat:" + msg.ID + ":" + p.UserID
		if _, _, err := h.Store.CreateNotification(ctx, store.CreateNotificationInput{
			SchoolID: thread.SchoolID, UserID: p.UserID, Type: "chat.message", Title: "Pesan baru: " + thread.Subject,
			Body: preview, Data: data, IdempotencyKey: &key,
		}); err != nil {
			common.CaptureError(err)
		}
	}
}

// UploadAttachment handles POST /chat/threads/{id}/attachments (multipart:
// file + optional body). The file is stored privately and posted as a message.
func (h *Handler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	thread, ok := h.authorizeThread(w, r, claims, param(r, "id"))
	if !ok {
		return
	}
	if thread.Closed() {
		common.WriteError(w, http.StatusConflict, "thread is closed")
		return
	}
	if h.Attachments == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "penyimpanan lampiran belum dikonfigurasi")
		return
	}
	settings, err := h.Store.GetChatSettings(r.Context(), thread.SchoolID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load chat settings")
		return
	}
	max := settings.MaxAttachmentBytes
	r.Body = http.MaxBytesReader(w, r.Body, max+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		common.WriteError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("Ukuran lampiran melebihi batas %d MB", max>>20))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file wajib diunggah")
		return
	}
	defer file.Close()
	if header.Size > max {
		common.WriteError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("Ukuran lampiran melebihi batas %d MB", max>>20))
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime := http.DetectContentType(head[:n])
	if !attachmentMimes[mime] {
		common.WriteError(w, http.StatusUnprocessableEntity, "Jenis lampiran harus PDF, JPG, atau PNG")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		common.WriteError(w, http.StatusBadRequest, "file tidak dapat dibaca")
		return
	}
	key := "chat/" + thread.ID + "/" + newObjectID() + extFor(mime)
	if _, err := h.Attachments.Put(r.Context(), key, mime, file, header.Size); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "lampiran ditolak: "+err.Error())
		return
	}
	name := filepath.Base(header.Filename)
	size := header.Size
	h.postMessage(w, r, thread, store.CreateMessageInput{
		ThreadID: thread.ID, SenderUserID: claims.UserID, Body: r.FormValue("body"),
		AttachmentKey: &key, AttachmentName: &name, AttachmentMime: &mime, AttachmentSize: &size,
	})
}

func extFor(mime string) string {
	switch mime {
	case "application/pdf":
		return ".pdf"
	case "image/png":
		return ".png"
	}
	return ".jpg"
}

// DownloadAttachment handles GET /chat/messages/{id}/attachment (participants
// only; moderators reach attachments through a report).
func (h *Handler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	msg, err := h.Store.GetChatMessage(r.Context(), param(r, "id"), schoolID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "message not found")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load message")
		return
	}
	if _, ok := h.authorizeThread(w, r, claims, msg.ThreadID); !ok {
		return
	}
	h.streamAttachment(w, r, msg)
}

func (h *Handler) streamAttachment(w http.ResponseWriter, r *http.Request, msg *model.ChatMessage) {
	if msg.AttachmentKey == nil || h.Attachments == nil {
		common.WriteError(w, http.StatusNotFound, "attachment not found")
		return
	}
	f, err := h.Attachments.Open(r.Context(), *msg.AttachmentKey)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "attachment not found")
		return
	}
	defer f.Close()
	mime, name := "application/octet-stream", "lampiran"
	if msg.AttachmentMime != nil {
		mime = *msg.AttachmentMime
	}
	if msg.AttachmentName != nil {
		name = *msg.AttachmentName
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, f)
}

// ArchiveThread handles POST /chat/threads/{id}/archive (participants).
func (h *Handler) ArchiveThread(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	thread, ok := h.authorizeThread(w, r, claims, param(r, "id"))
	if !ok {
		return
	}
	if err := h.Store.ArchiveChatThread(r.Context(), thread.ID); err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to archive thread")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"id": thread.ID}, nil, "Percakapan diarsipkan; pesan tetap tersimpan")
}

// ReportMessage handles POST /chat/messages/{id}/report {reason}
// (participants) and notifies the school's kepala_sekolah (FR-148-004).
func (h *Handler) ReportMessage(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	var p struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &p) {
		return
	}
	if strings.TrimSpace(p.Reason) == "" {
		common.WriteError(w, http.StatusBadRequest, "alasan laporan wajib diisi")
		return
	}
	msg, err := h.Store.GetChatMessage(r.Context(), param(r, "id"), schoolID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "message not found")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load message")
		return
	}
	if _, ok := h.authorizeThread(w, r, claims, msg.ThreadID); !ok {
		return
	}
	report, err := h.Store.CreateChatReport(r.Context(), schoolID, msg.ID, claims.UserID, strings.TrimSpace(p.Reason))
	if errors.Is(err, store.ErrConflict) {
		common.WriteError(w, http.StatusConflict, "Anda sudah melaporkan pesan ini")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to report message")
		return
	}
	h.notifyModerators(r.Context(), schoolID, report)
	common.WriteJSONMsg(w, http.StatusCreated, report, nil, "Laporan dikirim ke Kepala Unit")
}

func (h *Handler) notifyModerators(ctx context.Context, schoolID string, report *model.ChatMessageReport) {
	if h.Relations == nil {
		return
	}
	heads, err := h.Relations.UsersByRole(ctx, schoolID, roleKepalaSekolah)
	if err != nil {
		common.CaptureError(err)
		return
	}
	data, _ := json.Marshal(map[string]string{"report_id": report.ID})
	for _, u := range heads {
		key := "chat-report:" + report.ID + ":" + u.ID
		if _, _, err := h.Store.CreateNotification(ctx, store.CreateNotificationInput{
			SchoolID: schoolID, UserID: u.ID, Type: "chat.report", Title: "Laporan pesan chat",
			Body: "Ada pesan yang dilaporkan dan perlu ditinjau.", Data: data, IdempotencyKey: &key,
		}); err != nil {
			common.CaptureError(err)
		}
	}
}

func isModerator(role string) bool {
	return role == roleKepalaSekolah || role == roleAdmin || role == roleSuperadmin
}

func requireModerator(w http.ResponseWriter, claims *common.Claims) bool {
	if !isModerator(claims.ActiveRole) {
		common.WriteError(w, http.StatusForbidden, "Hanya Kepala Unit/admin yang dapat memoderasi chat")
		return false
	}
	return true
}

// ListReports handles GET /chat/moderation/reports?status=.
func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireModerator(w, claims) {
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	page, perPage := common.Page(r)
	list, total, err := h.Store.ListChatReports(r.Context(), schoolID, r.URL.Query().Get("status"), page, perPage)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to list reports")
		return
	}
	common.WriteJSON(w, http.StatusOK, list, common.NewPageMeta(page, perPage, total))
}

// GetReport handles GET /chat/moderation/reports/{id}: the report plus the
// reported thread's messages. Moderators read only threads that have a
// report; every opening is audited.
func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireModerator(w, claims) {
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	report, err := h.Store.GetChatReport(r.Context(), param(r, "id"), schoolID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "laporan tidak ditemukan")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load report")
		return
	}
	thread, err := h.Store.GetChatThread(r.Context(), report.ThreadID, schoolID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load thread")
		return
	}
	messages, _, err := h.Store.ListChatMessages(r.Context(), thread.ID, 1, 200)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load messages")
		return
	}
	h.audit(r.Context(), client.AuditEvent{
		SchoolID: schoolID, EntityType: "chat_thread", EntityID: thread.ID, Action: "moderation_open",
		ActorUserID: claims.UserID, ActorRole: claims.ActiveRole, After: map[string]string{"report_id": report.ID},
	})
	common.WriteJSON(w, http.StatusOK, map[string]any{"report": report, "thread": thread, "messages": messages}, nil)
}

// ReviewReport handles POST /chat/moderation/reports/{id}/review {status, note}.
func (h *Handler) ReviewReport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireModerator(w, claims) {
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	var p struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if !common.Decode(w, r, &p) {
		return
	}
	if p.Status != model.ReportReviewed && p.Status != model.ReportDismissed {
		common.WriteError(w, http.StatusBadRequest, "status harus reviewed atau dismissed")
		return
	}
	report, err := h.Store.ReviewChatReport(r.Context(), param(r, "id"), schoolID, p.Status, strings.TrimSpace(p.Note), claims.UserID)
	if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusConflict, "laporan tidak ditemukan atau sudah ditinjau")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to review report")
		return
	}
	h.audit(r.Context(), client.AuditEvent{
		SchoolID: schoolID, EntityType: "chat_message_report", EntityID: report.ID, Action: "review",
		ActorUserID: claims.UserID, ActorRole: claims.ActiveRole, After: map[string]string{"status": p.Status},
	})
	common.WriteJSONMsg(w, http.StatusOK, report, nil, "Laporan ditinjau")
}

// GetChatSettings handles GET /chat-settings (admin) — the service-hours
// banner also needs it, so every signed-in user may read it.
func (h *Handler) GetChatSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	s, err := h.Store.GetChatSettings(r.Context(), schoolID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load chat settings")
		return
	}
	common.WriteJSON(w, http.StatusOK, s, nil)
}

// PutChatSettings handles PUT /chat-settings (admin only).
func (h *Handler) PutChatSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims.ActiveRole != roleAdmin && claims.ActiveRole != roleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Hanya admin yang dapat mengubah pengaturan chat")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	var p model.ChatSettings
	if !common.Decode(w, r, &p) {
		return
	}
	p.SchoolID = schoolID
	if (p.ServiceStart == nil) != (p.ServiceEnd == nil) {
		common.WriteError(w, http.StatusBadRequest, "jam mulai dan selesai harus diisi bersamaan")
		return
	}
	for _, v := range []*string{p.ServiceStart, p.ServiceEnd} {
		if v != nil {
			if _, err := time.Parse("15:04", *v); err != nil {
				common.WriteError(w, http.StatusBadRequest, "format jam harus HH:MM")
				return
			}
		}
	}
	for _, d := range p.ServiceDays {
		if d < 1 || d > 7 {
			common.WriteError(w, http.StatusBadRequest, "hari layanan 1 (Senin) s.d. 7 (Minggu)")
			return
		}
	}
	if p.MaxAttachmentBytes <= 0 {
		p.MaxAttachmentBytes = model.DefaultMaxAttachmentBytes
	}
	before, _ := h.Store.GetChatSettings(r.Context(), schoolID)
	if err := h.Store.UpsertChatSettings(r.Context(), p); err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to save chat settings")
		return
	}
	h.audit(r.Context(), client.AuditEvent{
		SchoolID: schoolID, EntityType: "chat_settings", EntityID: schoolID, Action: "update",
		ActorUserID: claims.UserID, ActorRole: claims.ActiveRole, Before: before, After: p,
	})
	common.WriteJSONMsg(w, http.StatusOK, p, nil, "Pengaturan chat disimpan")
}

func newObjectID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
