package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/report/internal/common"
	"skci/report/internal/store"
)

// GetNoteTemplate: GET /note-templates — Deskripsi Rapor's "Atur Template
// Deskripsi" (old app), one template pair per school.
func (h *Handler) GetNoteTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	schoolID := resolveSchoolID(r, claims)
	t, err := h.d.Store.GetNoteTemplate(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch note template")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

// SetNoteTemplate: PUT /note-templates
func (h *Handler) SetNoteTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	var body store.NoteTemplate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	schoolID := resolveSchoolID(r, claims)
	if err := h.d.Store.SetNoteTemplate(r.Context(), schoolID, body, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save note template")
		return
	}
	common.WriteJSON(w, http.StatusOK, body, nil)
}

type subjectNotesBody struct {
	MidNote   *string `json:"mid_note"`
	FinalNote *string `json:"final_note"`
	// Capaian kolom 2 (Report Sekoci V15 item 1). nil still means "leave
	// unchanged", same rule as the first pair -- each textarea saves on its
	// own blur and must not wipe its neighbour.
	MidNote2   *string `json:"mid_note2"`
	FinalNote2 *string `json:"final_note2"`
}

// PatchSubjectNotes: PATCH /report-cards/{id}/subjects/{subject_id}/notes —
// Deskripsi Rapor's Capaian STS/SAS free-text per subject. Unlike score
// overrides this is admin+guru (subject teachers write their own notes),
// matching the old app's ReportDescription view being editable by the
// subject's own guru, not admin-only.
func (h *Handler) PatchSubjectNotes(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	subjectID := chi.URLParam(r, "subject_id")

	rc, err := h.d.Store.GetReportCard(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report card")
		return
	}
	if rc == nil {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	// Wali kelas's Deskripsi Rapor grid writes Capaian STS/SAS notes through
	// this same endpoint, scoped to their own classroom (see
	// isWaliKelasOwner) — admin/superadmin/guru behavior is unchanged.
	if !isStaff(claims) && !h.isWaliKelasOwner(r.Context(), claims, rc.ClassroomID) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only, or the wali kelas of this classroom")
		return
	}
	if claims.ActiveRole != "superadmin" && rc.SchoolID != claims.SchoolID {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}

	var body subjectNotesBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.d.Store.PatchSubjectNotes(r.Context(), id, subjectID, body.MidNote, body.FinalNote, body.MidNote2, body.FinalNote2); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			common.WriteError(w, http.StatusNotFound, "subject not found on this report card")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "failed to save notes")
		return
	}

	full, err := h.loadReportCard(r.Context(), id)
	if err != nil || full == nil {
		common.WriteError(w, http.StatusInternalServerError, "notes saved but failed to reload report card")
		return
	}
	common.WriteJSON(w, http.StatusOK, full, nil)
}
