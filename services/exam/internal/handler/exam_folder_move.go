package handler

import (
	"encoding/json"
	"net/http"

	"skci/exam/internal/common"
)

// MoveExams: POST /exams/move-folder — {exam_ids: [...], folder_id: "..."}.
//
// Asked for by SMK GPM's admin ("REPORT FITUR ADMIN", 21 Sep 2026) right
// after the "Tanpa Folder" listing made stray exams visible: several of
// them belong in the STS folder. An empty folder_id moves them back out,
// which is also the undo for a wrong move.
//
// A guru may only re-file exams they authored; admin/superadmin may move
// any exam in their school. Both are enforced in the UPDATE itself (see
// store.MoveExamsToFolder), so a forged exam_id from another school or
// another guru simply doesn't match and isn't counted as moved.
func (h *Handler) MoveExams(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	var req struct {
		ExamIDs  []string `json:"exam_ids"`
		FolderID string   `json:"folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.ExamIDs) == 0 {
		common.WriteError(w, http.StatusBadRequest, "exam_ids is required")
		return
	}
	schoolID := common.EffectiveSchoolID(r, c)

	if req.FolderID != "" {
		folder, err := h.Store.GetFolder(r.Context(), req.FolderID, schoolID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		if folder == nil {
			common.WriteError(w, http.StatusNotFound, "folder not found")
			return
		}
	}

	onlyCreatedBy := ""
	if c != nil && c.ActiveRole == "guru" {
		onlyCreatedBy = c.UserID
	}
	moved, err := h.Store.MoveExamsToFolder(r.Context(), req.ExamIDs, req.FolderID, schoolID, onlyCreatedBy)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if moved == 0 {
		common.WriteError(w, http.StatusNotFound, "tidak ada ujian yang dipindahkan (bukan milik Anda, atau sudah dihapus)")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"moved_count": moved,
		"folder_id":   req.FolderID,
	}, nil)
}
