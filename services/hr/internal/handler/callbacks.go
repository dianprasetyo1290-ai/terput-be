package handler

import (
	"fmt"
	"net/http"

	"skci/hr/internal/common"
	"skci/hr/internal/store"
)

type ApprovalCallbackRequest struct {
	ApprovalID   string  `json:"approval_id"`
	ResourceType string  `json:"resource_type"`
	ResourceID   string  `json:"resource_id"`
	Decision     string  `json:"decision"` // "approved" or "rejected"
	Notes        *string `json:"notes"`
}

func (a *App) InternalApprovalCallback(w http.ResponseWriter, r *http.Request) {
	var body ApprovalCallbackRequest
	if err := decodeJSON(r, &body); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if body.ResourceID == "" || body.ResourceType == "" || (body.Decision != "approved" && body.Decision != "rejected") {
		common.Error(w, http.StatusBadRequest, "Data callback tidak lengkap atau status tidak valid")
		return
	}

	var err error
	switch body.ResourceType {
	case "leave_request":
		err = store.ApplyLeaveDecisionTx(r.Context(), a.DB, body.ResourceID, body.Decision)
	case "overtime_request":
		err = store.ApplyOvertimeDecisionTx(r.Context(), a.DB, body.ResourceID, body.Decision)
	case "profile_change_request":
		err = store.ApplyProfileChangeRequestTx(r.Context(), a.DB, body.ResourceID, body.Decision)
	default:
		common.Error(w, http.StatusBadRequest, fmt.Sprintf("Jenis sumber daya approval '%s' tidak dikenal", body.ResourceType))
		return
	}

	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memproses hasil persetujuan: "+err.Error())
		return
	}

	common.JSON(w, http.StatusOK, map[string]string{
		"message": "Callback persetujuan berhasil diproses",
		"status":  body.Decision,
	})
}
