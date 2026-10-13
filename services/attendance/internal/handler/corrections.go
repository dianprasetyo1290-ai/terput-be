package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"skci/attendance/internal/client"
	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
	"skci/attendance/internal/store"
)

// BR-ATT-01 / AC-41-02: once a day is locked, a change is a correction
// request: stored here as pending, opened in core's Approval Center
// (request_type attendance_correction), and only applied when core calls
// back with "approved". Request, decision and the applied before/after are
// all written to the audit outbox (forwarded to core's audit trail).

type createCorrectionBody struct {
	ClassroomID string  `json:"classroom_id"`
	StudentID   string  `json:"student_id"`
	SessionDate string  `json:"session_date"`
	Status      string  `json:"status"`
	Catatan     *string `json:"catatan"`
	Reason      string  `json:"reason"`
}

// CreateCorrection: POST /attendance-corrections (admin, superadmin,
// walikelas for their own classroom).
func (h *Handler) CreateCorrection(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	var body createCorrectionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if body.StudentID == "" || !validDate(body.SessionDate) {
		common.WriteError(w, http.StatusBadRequest, "student_id dan session_date (YYYY-MM-DD) wajib diisi")
		return
	}
	if body.Status == string(model.StatusBelumPresensi) || !model.AttendanceStatus(body.Status).Valid() {
		common.WriteError(w, http.StatusBadRequest, "status koreksi harus hadir, terlambat, sakit, izin, atau tanpa_keterangan")
		return
	}
	if body.Reason == "" {
		common.WriteError(w, http.StatusBadRequest, "alasan koreksi wajib diisi")
		return
	}
	schoolID, ok := h.dailyScope(w, r, claims, body.ClassroomID, true)
	if !ok {
		return
	}
	locked, err := h.dateLocked(r.Context(), schoolID, body.SessionDate)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !locked {
		common.WriteError(w, http.StatusConflict, "Presensi tanggal "+body.SessionDate+" belum dikunci, ubah langsung melalui input presensi")
		return
	}

	in := store.CreateCorrectionInput{
		SchoolID: schoolID, ClassroomID: body.ClassroomID, StudentID: body.StudentID, SessionDate: body.SessionDate,
		ToStatus: body.Status, Catatan: body.Catatan, Reason: body.Reason,
		RequestedBy: claims.UserID, RequestedRole: claims.ActiveRole,
	}
	cur, err := h.Store.GetStudentAttendanceOn(r.Context(), body.ClassroomID, body.SessionDate, body.StudentID)
	switch {
	case err == nil:
		if cur.Status == body.Status {
			common.WriteError(w, http.StatusBadRequest, "status koreksi sama dengan status saat ini")
			return
		}
		in.FromStatus, in.AcademicYearID = &cur.Status, cur.AcademicYearID
		if cur.TermID != nil {
			in.TermID = *cur.TermID
		}
	case errors.Is(err, store.ErrNotFound):
		// No row for that day yet: the student must at least be enrolled.
		roster, rerr := h.Client.GetRoster(r.Context(), body.ClassroomID)
		if rerr != nil {
			common.WriteError(w, http.StatusBadGateway, "gagal memuat daftar siswa kelas")
			return
		}
		found := false
		for _, s := range roster {
			found = found || s.StudentID == body.StudentID
		}
		if !found {
			common.WriteError(w, http.StatusBadRequest, "siswa tidak terdaftar di kelas ini")
			return
		}
	default:
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if in.AcademicYearID == "" {
		if c, err := h.Client.GetClassroom(r.Context(), body.ClassroomID); err == nil && c != nil {
			in.AcademicYearID = c.AcademicYearID
		}
		if in.AcademicYearID == "" {
			common.WriteError(w, http.StatusBadGateway, "tahun ajaran kelas tidak dapat ditentukan")
			return
		}
	}

	c, err := h.Store.CreateCorrection(r.Context(), in)
	if errors.Is(err, store.ErrCorrectionPending) {
		common.WriteError(w, http.StatusConflict, "sudah ada pengajuan koreksi yang menunggu persetujuan untuk siswa dan tanggal ini")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan pengajuan koreksi: "+err.Error())
		return
	}

	studentName := h.studentName(r.Context(), c.StudentID)
	from := "-"
	if c.FromStatus != nil {
		from = statusLabel(*c.FromStatus)
	}
	title := "Koreksi Presensi " + strings.TrimSpace(studentName+" ") + c.SessionDate + ": " + from + " → " + statusLabel(c.ToStatus)
	approvalID, err := h.Client.CreateApproval(r.Context(), client.ApprovalRequest{
		SchoolID: c.SchoolID, RequestType: client.ReqTypeAttendanceCorrection, SourceID: c.ID, Title: title,
		Summary: map[string]any{
			"student_id": c.StudentID, "student_name": studentName, "classroom_id": c.ClassroomID,
			"session_date": c.SessionDate, "from_status": c.FromStatus, "to_status": c.ToStatus,
			"catatan": c.Catatan, "reason": c.Reason, "requested_role": c.RequestedRole,
		},
		MakerUserID: claims.UserID,
	})
	if err != nil {
		_ = h.Store.DeleteCorrection(r.Context(), c, "approval center: "+err.Error())
		status, msg := client.ApprovalErrorMessage(err)
		common.WriteError(w, status, msg)
		return
	}
	if err := h.Store.SetCorrectionApprovalID(r.Context(), c.ID, approvalID); err != nil {
		log.Printf("correction %s: store approval id: %v", c.ID, err)
	}
	c.ApprovalID = &approvalID
	common.WriteJSONMsg(w, http.StatusCreated, c, nil, "Pengajuan koreksi dikirim ke Approval Center")
}

// ListCorrections: GET /attendance-corrections?status=&classroom_id=
func (h *Handler) ListCorrections(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin", "kepala_sekolah", "walikelas") {
		return
	}
	q := r.URL.Query()
	f := store.CorrectionFilter{SchoolID: scopeFromRequest(claims, r), ClassroomID: q.Get("classroom_id"),
		Status: q.Get("status"), StudentID: q.Get("student_id")}
	if claims.ActiveRole == "walikelas" && !h.requireOwnClassroom(w, r, claims, f.ClassroomID) {
		return
	}
	list, err := h.Store.ListCorrections(r.Context(), f)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	users, classes := map[string]string{}, map[string]string{}
	user := func(id string) string {
		if n, ok := users[id]; ok {
			return n
		}
		users[id] = h.studentName(r.Context(), id)
		return users[id]
	}
	for i := range list {
		list[i].StudentName = user(list[i].StudentID)
		list[i].RequesterName = user(list[i].RequestedBy)
		cid := list[i].ClassroomID
		if _, ok := classes[cid]; !ok {
			classes[cid] = ""
			if cr, err := h.Client.GetClassroom(r.Context(), cid); err == nil && cr != nil {
				classes[cid] = cr.Name
			}
		}
		list[i].ClassroomName = classes[cid]
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// approvalCallbackBody is core's callback payload (core jobs.CallbackPayload).
type approvalCallbackBody struct {
	ApprovalID  string     `json:"approval_id"`
	SourceID    string     `json:"source_id"`
	RequestType string     `json:"request_type"`
	Decision    string     `json:"decision"`
	DecidedBy   string     `json:"decided_by"`
	Comment     string     `json:"comment"`
	DecidedAt   *time.Time `json:"decided_at"`
}

// InternalApprovalCallback: POST /internal/approval-callbacks (X-Internal-Key,
// from core). Idempotent: a repeated callback answers 200 without changes.
func (h *Handler) InternalApprovalCallback(w http.ResponseWriter, r *http.Request) {
	var body approvalCallbackBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if body.SourceID == "" || (body.Decision != "approved" && body.Decision != "rejected") {
		common.WriteError(w, http.StatusBadRequest, "Data callback tidak lengkap atau keputusan tidak valid")
		return
	}
	if body.RequestType != client.ReqTypeAttendanceCorrection {
		common.WriteError(w, http.StatusUnprocessableEntity, "Jenis approval '"+body.RequestType+"' tidak ditangani attendance")
		return
	}
	c, changed, err := h.Store.ApplyCorrectionDecision(r.Context(), store.CorrectionDecision{
		CorrectionID: body.SourceID, ApprovalID: body.ApprovalID, Decision: body.Decision,
		DecidedBy: body.DecidedBy, Comment: body.Comment, DecidedAt: body.DecidedAt,
	})
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Data sumber approval tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses hasil persetujuan: "+err.Error())
		return
	}
	if changed {
		label := map[string]string{"approved": "disetujui", "rejected": "ditolak"}[c.Status]
		msg := "Koreksi presensi " + c.SessionDate + " menjadi " + statusLabel(c.ToStatus) + " " + label + "."
		if body.Comment != "" {
			msg += " Catatan: " + body.Comment
		}
		h.notify(r, c.SchoolID, c.RequestedBy, "attendance.correction."+c.Status, "Koreksi presensi "+label, msg, "attendance-correction-"+c.ID+"-"+c.Status)
		if c.Status == "approved" {
			var alpa alpaCollector
			alpa.add(c.SchoolID, c.StudentID, c.SessionDate, c.ToStatus)
			h.notifyAlpaAsync(alpa) // FR-41-004
		}
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Callback persetujuan berhasil diproses", "status": c.Status}, nil)
}

func statusLabel(s string) string {
	switch s {
	case "hadir":
		return "Hadir"
	case "terlambat":
		return "Terlambat"
	case "sakit":
		return "Sakit"
	case "izin":
		return "Izin"
	case "tanpa_keterangan":
		return "Alpa"
	case "belum_presensi":
		return "Belum Presensi"
	}
	return s
}
