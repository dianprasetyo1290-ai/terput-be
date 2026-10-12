package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"skci/hr/internal/ats"
	"skci/hr/internal/client"
	"skci/hr/internal/common"
	"skci/hr/internal/payroll"
	"skci/hr/internal/store"
)

// ApprovalCallbackRequest is the body core POSTs to callback_url once an
// approval is decided (T1 §3, core jobs.CallbackPayload).
type ApprovalCallbackRequest struct {
	ApprovalID  string     `json:"approval_id"`
	SourceID    string     `json:"source_id"`
	RequestType string     `json:"request_type"`
	Decision    string     `json:"decision"` // "approved" | "rejected"
	DecidedBy   string     `json:"decided_by"`
	Comment     string     `json:"comment"`
	DecidedAt   *time.Time `json:"decided_at"`
}

// InternalApprovalCallback applies a core decision to the hr source row,
// dispatched by request_type. Idempotent: a repeated callback (Idempotency-Key
// approval:<id>) on an already-final row answers 200 without changes.
func (a *App) InternalApprovalCallback(w http.ResponseWriter, r *http.Request) {
	var body ApprovalCallbackRequest
	if err := decodeJSON(r, &body); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if body.SourceID == "" || body.RequestType == "" || (body.Decision != "approved" && body.Decision != "rejected") {
		common.Error(w, http.StatusBadRequest, "Data callback tidak lengkap atau keputusan tidak valid")
		return
	}

	ctx := r.Context()
	var err error
	switch body.RequestType {
	case client.ReqTypeLeave:
		err = store.ApplyLeaveDecisionTx(ctx, a.DB, body.SourceID, body.Decision)
	case client.ReqTypeOvertime:
		err = store.ApplyOvertimeDecisionTx(ctx, a.DB, body.SourceID, body.Decision)
	case client.ReqTypeProfileChange:
		err = store.ApplyProfileChangeRequestTx(ctx, a.DB, body.SourceID, body.Decision)
	case client.ReqTypePayrollRun:
		err = payroll.NewStore(a.DB).ApplyRunDecision(ctx, body.SourceID, body.Decision)
	case client.ReqTypeRequisition:
		err = ats.NewStore(a.DB).ApplyRequisitionDecision(ctx, body.SourceID, body.Decision)
	default:
		common.Error(w, http.StatusUnprocessableEntity, fmt.Sprintf("Jenis approval '%s' tidak ditangani hr", body.RequestType))
		return
	}

	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, payroll.ErrNotFound), errors.Is(err, ats.ErrNotFound):
		common.Error(w, http.StatusNotFound, "Data sumber approval tidak ditemukan")
		return
	case err != nil:
		common.Error(w, http.StatusInternalServerError, "Gagal memproses hasil persetujuan: "+err.Error())
		return
	}

	common.JSON(w, http.StatusOK, map[string]string{
		"message": "Callback persetujuan berhasil diproses",
		"status":  body.Decision,
	})
}
