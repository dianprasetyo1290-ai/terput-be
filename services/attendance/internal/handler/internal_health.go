package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"skci/attendance/internal/common"
	"skci/attendance/internal/store"
)

// healthAbsenceStore is the seam over the store write (tests inject a fake).
type healthAbsenceStore interface {
	ApplyHealthAbsence(ctx context.Context, in store.HealthAbsenceInput) (bool, string, error)
}

type healthAbsenceBody struct {
	SchoolID    string    `json:"school_id"`
	StudentID   string    `json:"student_id"`
	Date        string    `json:"date"`
	SickSince   time.Time `json:"sick_since"`
	SourceRef   string    `json:"source_ref"`
	Outcome     string    `json:"outcome"`
	ActorUserID string    `json:"actor_user_id"`
	Reason      string    `json:"reason"`
}

// InternalHealthAbsence: POST /internal/student-absences/health (studentlife,
// X-Internal-Key). Marks the student 'sakit' for the day of a UKS visit that
// ended in sent_home/referred (AC-128-01). Idempotent by source_ref.
func (h *Handler) InternalHealthAbsence(w http.ResponseWriter, r *http.Request) {
	var b healthAbsenceBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&b); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if b.SchoolID == "" || b.StudentID == "" || b.SourceRef == "" || b.ActorUserID == "" || b.SickSince.IsZero() {
		common.WriteError(w, http.StatusBadRequest, "school_id, student_id, source_ref, actor_user_id dan sick_since wajib diisi")
		return
	}
	if _, err := time.Parse("2006-01-02", b.Date); err != nil {
		common.WriteError(w, http.StatusBadRequest, "date harus berformat YYYY-MM-DD")
		return
	}
	if b.Outcome != "sent_home" && b.Outcome != "referred" {
		common.WriteError(w, http.StatusUnprocessableEntity, "outcome harus sent_home atau referred")
		return
	}
	reason := strings.TrimSpace(b.Reason)
	if reason == "" {
		reason = "Dicatat sakit oleh UKS"
	}
	classroomID := ""
	if h.Client != nil {
		classroomID, _ = h.Client.GetStudentClassroomID(r.Context(), b.StudentID, b.SchoolID)
	}
	applied, why, err := h.healthAbsence.ApplyHealthAbsence(r.Context(), store.HealthAbsenceInput{
		SchoolID: b.SchoolID, StudentID: b.StudentID, Date: b.Date, SourceRef: b.SourceRef, ActorUserID: b.ActorUserID,
		Reason: reason, ClassroomID: classroomID, SickSince: b.SickSince,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal mencatat absen sakit")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"applied": applied, "reason": why}, nil)
}
