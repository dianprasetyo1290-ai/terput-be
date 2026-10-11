package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/attendance/internal/common"
	"skci/attendance/internal/store"
)

// PRD menu 48 (orang tua submits izin/sakit for a linked child) and menu 42
// (wali kelas / admin approves or rejects). Approval applies BR-ATT-02 --
// see store.ReviewLeaveRequest and store.ApprovedLeaveTypes.

const (
	leaveMaxAttachment = 2 << 20 // 2 MB
	leaveMaxDays       = 31
)

var leaveAttachmentTypes = map[string]bool{"image/jpeg": true, "image/png": true, "application/pdf": true}

// CreateLeaveRequest: POST /parent/children/{studentUserId}/leave-requests
// multipart/form-data: type (izin|sakit), start_date, end_date, reason,
// attachment (optional; jpg/png/pdf <= 2 MB).
func (h *Handler) CreateLeaveRequest(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "orang_tua") {
		return
	}
	studentID := chi.URLParam(r, "studentUserId")
	if !h.requireGuardianLink(w, r, claims, studentID) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, leaveMaxAttachment+64<<10)
	if err := r.ParseMultipartForm(leaveMaxAttachment); err != nil {
		common.WriteError(w, http.StatusBadRequest, "form tidak valid atau lampiran melebihi 2 MB")
		return
	}
	typ := r.FormValue("type")
	start, end := r.FormValue("start_date"), r.FormValue("end_date")
	if end == "" {
		end = start
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if typ != "izin" && typ != "sakit" {
		common.WriteError(w, http.StatusBadRequest, "jenis harus izin atau sakit")
		return
	}
	if reason == "" {
		common.WriteError(w, http.StatusBadRequest, "alasan wajib diisi")
		return
	}
	start, end, err := parseDateRange(start, end)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	s, _ := time.Parse(parentDateLayout, start)
	e, _ := time.Parse(parentDateLayout, end)
	if e.Sub(s) >= leaveMaxDays*24*time.Hour {
		common.WriteError(w, http.StatusBadRequest, "rentang izin maksimal 31 hari")
		return
	}

	in := store.CreateLeaveInput{
		SchoolID: claims.SchoolID, StudentID: studentID, RequestedBy: claims.UserID,
		Type: typ, StartDate: start, EndDate: end, Reason: reason,
	}
	if f, fh, err := r.FormFile("attachment"); err == nil {
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, leaveMaxAttachment+1))
		if err != nil || len(data) > leaveMaxAttachment {
			common.WriteError(w, http.StatusBadRequest, "lampiran melebihi 2 MB")
			return
		}
		ct := http.DetectContentType(data)
		if !leaveAttachmentTypes[ct] {
			common.WriteError(w, http.StatusBadRequest, "lampiran harus JPG, PNG, atau PDF")
			return
		}
		in.Attachment, in.AttachmentType, in.AttachmentName = data, ct, fh.Filename
	}

	// Snapshot the student's current classroom so the right wali kelas sees
	// it. Not enrolled / academic down => admin-only queue, never a failure.
	var homeroom string
	if cid, err := h.Client.GetStudentClassroomID(r.Context(), studentID, claims.SchoolID); err == nil && cid != "" {
		in.ClassroomID = cid
		if c, err := h.Client.GetClassroom(r.Context(), cid); err == nil && c != nil {
			in.AcademicYearID = c.AcademicYearID
			if c.HomeroomTeacherUserID != nil {
				homeroom = *c.HomeroomTeacherUserID
			}
		}
	}

	l, err := h.Store.CreateLeaveRequest(r.Context(), in)
	if errors.Is(err, store.ErrLeaveOverlap) {
		common.WriteError(w, http.StatusConflict, "sudah ada pengajuan izin pada tanggal tersebut")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan pengajuan: "+err.Error())
		return
	}
	if homeroom != "" {
		h.notify(r, l.SchoolID, homeroom, "attendance.leave.submitted", "Pengajuan "+l.Type+" baru",
			"Orang tua mengajukan "+l.Type+" "+l.StartDate+" s.d. "+l.EndDate+": "+l.Reason, "leave-"+l.ID+"-submitted")
	}
	common.WriteJSONMsg(w, http.StatusCreated, l, nil, "Pengajuan izin berhasil dikirim")
}

// ListChildLeaveRequests: GET /parent/children/{studentUserId}/leave-requests
func (h *Handler) ListChildLeaveRequests(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "orang_tua") {
		return
	}
	studentID := chi.URLParam(r, "studentUserId")
	if !h.requireGuardianLink(w, r, claims, studentID) {
		return
	}
	list, err := h.Store.ListLeaveRequests(r.Context(), store.LeaveFilter{SchoolID: claims.SchoolID, StudentID: studentID})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.enrichLeaves(r, list)
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// ListLeaveRequests: GET /leave-requests?status=&classroom_id=
// admin/kepala_sekolah: whole school; superadmin: ?school_id= or all;
// walikelas: classroom_id required and must be their own classroom.
func (h *Handler) ListLeaveRequests(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin", "kepala_sekolah", "walikelas") {
		return
	}
	q := r.URL.Query()
	f := store.LeaveFilter{SchoolID: scopeFromRequest(claims, r), ClassroomID: q.Get("classroom_id"), Status: q.Get("status")}
	if claims.ActiveRole == "walikelas" && !h.requireOwnClassroom(w, r, claims, f.ClassroomID) {
		return
	}
	list, err := h.Store.ListLeaveRequests(r.Context(), f)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.enrichLeaves(r, list)
	common.WriteJSON(w, http.StatusOK, list, nil)
}

type reviewLeaveBody struct {
	Note string `json:"note"`
}

// ApproveLeaveRequest / RejectLeaveRequest: POST /leave-requests/{id}/approve|reject
func (h *Handler) ApproveLeaveRequest(w http.ResponseWriter, r *http.Request) {
	h.reviewLeave(w, r, "approved")
}

func (h *Handler) RejectLeaveRequest(w http.ResponseWriter, r *http.Request) {
	h.reviewLeave(w, r, "rejected")
}

func (h *Handler) reviewLeave(w http.ResponseWriter, r *http.Request, status string) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin", "walikelas") {
		return
	}
	l := h.loadLeaveForStaff(w, r, claims)
	if l == nil {
		return
	}
	var body reviewLeaveBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Note = strings.TrimSpace(body.Note)
	if status == "rejected" && body.Note == "" {
		common.WriteError(w, http.StatusBadRequest, "alasan penolakan wajib diisi")
		return
	}
	updated, err := h.Store.ReviewLeaveRequest(r.Context(), l.ID, status, claims.UserID, body.Note)
	if errors.Is(err, store.ErrLeaveNotPending) {
		common.WriteError(w, http.StatusConflict, "pengajuan ini sudah diproses")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memproses pengajuan: "+err.Error())
		return
	}
	label := map[string]string{"approved": "disetujui", "rejected": "ditolak"}[status]
	msg := "Pengajuan " + updated.Type + " " + updated.StartDate + " s.d. " + updated.EndDate + " " + label + "."
	if body.Note != "" {
		msg += " Catatan: " + body.Note
	}
	h.notify(r, updated.SchoolID, updated.RequestedBy, "attendance.leave."+status, "Pengajuan izin "+label, msg, "leave-"+updated.ID+"-"+status)
	out := []store.LeaveRequest{*updated}
	h.enrichLeaves(r, out)
	common.WriteJSONMsg(w, http.StatusOK, out[0], nil, "Pengajuan "+label)
}

// LeaveAttachment: GET /leave-requests/{id}/attachment -- staff in scope, or
// a guardian of the student.
func (h *Handler) LeaveAttachment(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims.ActiveRole == "orang_tua" {
		l, err := h.Store.GetLeaveRequest(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			common.WriteError(w, http.StatusNotFound, "pengajuan tidak ditemukan")
			return
		}
		if l.SchoolID != claims.SchoolID {
			common.WriteError(w, http.StatusNotFound, "pengajuan tidak ditemukan")
			return
		}
		if !h.requireGuardianLink(w, r, claims, l.StudentID) {
			return
		}
	} else {
		if !requireRole(w, claims, "admin", "superadmin", "kepala_sekolah", "walikelas") {
			return
		}
		if h.loadLeaveForStaff(w, r, claims) == nil {
			return
		}
	}
	name, ct, data, err := h.Store.GetLeaveAttachment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "lampiran tidak ditemukan")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", `inline; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

// loadLeaveForStaff loads {id} and enforces tenant + wali-kelas scope.
func (h *Handler) loadLeaveForStaff(w http.ResponseWriter, r *http.Request, claims *common.Claims) *store.LeaveRequest {
	l, err := h.Store.GetLeaveRequest(r.Context(), chi.URLParam(r, "id"))
	if err != nil || (claims.ActiveRole != "superadmin" && l.SchoolID != claims.SchoolID) {
		common.WriteError(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return nil
	}
	if claims.ActiveRole == "walikelas" {
		cid := ""
		if l.ClassroomID != nil {
			cid = *l.ClassroomID
		}
		if cid == "" {
			common.WriteError(w, http.StatusForbidden, "forbidden: siswa belum terdaftar di kelas, hanya admin yang dapat memproses")
			return nil
		}
		if !h.requireOwnClassroom(w, r, claims, cid) {
			return nil
		}
	}
	return l
}

// enrichLeaves fills student/classroom/reviewer names (best-effort).
func (h *Handler) enrichLeaves(r *http.Request, list []store.LeaveRequest) {
	users, classes := map[string]string{}, map[string]string{}
	user := func(id string) string {
		if n, ok := users[id]; ok {
			return n
		}
		if u, err := h.Client.GetUser(r.Context(), id); err == nil && u != nil {
			users[id] = u.Name
		} else {
			users[id] = ""
		}
		return users[id]
	}
	for i := range list {
		list[i].StudentName = user(list[i].StudentID)
		if list[i].ReviewedBy != nil {
			list[i].ReviewerName = user(*list[i].ReviewedBy)
		}
		if c := list[i].ClassroomID; c != nil {
			if _, ok := classes[*c]; !ok {
				classes[*c] = ""
				if cr, err := h.Client.GetClassroom(r.Context(), *c); err == nil && cr != nil {
					classes[*c] = cr.Name
				}
			}
			list[i].ClassroomName = classes[*c]
		}
	}
}

func (h *Handler) notify(r *http.Request, schoolID, userID, typ, title, body, key string) {
	if err := h.Client.Notify(r.Context(), schoolID, userID, typ, title, body, key); err != nil {
		log.Printf("leave notification %s failed: %v", key, err)
	}
}
