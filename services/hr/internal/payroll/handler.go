package payroll

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/client"
	"skci/hr/internal/common"
)

type Handler struct {
	Store     *Store
	Approvals *client.Client
}

func NewHandler(s *Store, approvals *client.Client) *Handler {
	return &Handler{Store: s, Approvals: approvals}
}

// -----------------------------------------------------------------------------
// Pay Components
// -----------------------------------------------------------------------------

func (h *Handler) ListComponents(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListComponents(r.Context(), schoolID)
	if err != nil {
		common.InternalError(w, "gagal memuat komponen gaji", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateComponent(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var c PayComponent
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	c.SchoolID = schoolID

	created, err := h.Store.CreateComponent(r.Context(), c)
	if err != nil {
		common.InternalError(w, "gagal membuat komponen", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) SubmitComponentApproval(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	c, err := h.Store.GetComponent(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "komponen gaji tidak ditemukan")
		return
	}

	appID, err := h.Approvals.CreateApproval(r.Context(), client.ApprovalRequest{
		SchoolID:    schoolID,
		RequestType: client.ReqTypePayrollComponentChange,
		SourceID:    c.ID,
		Title:       fmt.Sprintf("Perubahan Komponen Gaji: %s (%s)", c.Name, c.Code),
		Summary: map[string]any{
			"code": c.Code, "name": c.Name, "kind": c.Kind,
			"nature": c.Nature, "taxable": c.Taxable, "calc": c.Calc,
		},
		MakerUserID: claims.UserID,
	})
	if err != nil {
		log.Printf("payroll component %s: create approval: %v", c.ID, err)
		status, msg := client.ApprovalErrorMessage(err)
		common.WriteError(w, status, msg)
		return
	}

	if err := h.Store.SubmitComponentApproval(r.Context(), schoolID, id, appID); err != nil {
		common.InternalError(w, "gagal mengajukan approval", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{
		"message":     "perubahan komponen diajukan untuk persetujuan",
		"approval_id": appID,
	}, nil)
}

// -----------------------------------------------------------------------------
// Payroll Inputs
// -----------------------------------------------------------------------------

func (h *Handler) ListInputs(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	period := r.URL.Query().Get("period")
	if period == "" {
		common.WriteError(w, http.StatusBadRequest, "parameter period wajib diisi")
		return
	}

	list, err := h.Store.ListInputs(r.Context(), schoolID, period)
	if err != nil {
		common.InternalError(w, "gagal memuat input payroll", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) SaveInput(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)

	var body struct {
		Period      string `json:"period"`
		EmployeeID  string `json:"employee_id"`
		ComponentID string `json:"component_id"`
		AmountIDR   int64  `json:"amount_idr"`
		Reason      string `json:"reason,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if body.Period == "" || body.EmployeeID == "" || body.ComponentID == "" {
		common.WriteError(w, http.StatusBadRequest, "period, employee_id, dan component_id wajib diisi")
		return
	}

	needsApproval, inputID, changeID, err := h.Store.SaveInput(
		r.Context(), schoolID, body.Period, body.EmployeeID, body.ComponentID, body.AmountIDR, body.Reason,
	)
	if err != nil {
		common.InternalError(w, "gagal menyimpan input payroll", err)
		return
	}

	if needsApproval {
		appID, err := h.Approvals.CreateApproval(r.Context(), client.ApprovalRequest{
			SchoolID:    schoolID,
			RequestType: client.ReqTypePayrollInputChange,
			SourceID:    changeID,
			Title:       fmt.Sprintf("Penyesuaian Input Payroll Periode %s (Lewat Cut-off)", body.Period),
			Summary: map[string]any{
				"period":       body.Period,
				"employee_id":  body.EmployeeID,
				"component_id": body.ComponentID,
				"amount_idr":   body.AmountIDR,
				"reason":       body.Reason,
			},
			AmountIDR:   &body.AmountIDR,
			MakerUserID: claims.UserID,
		})
		if err != nil {
			log.Printf("payroll input change %s: create approval: %v", changeID, err)
			status, msg := client.ApprovalErrorMessage(err)
			common.WriteError(w, status, msg)
			return
		}

		_ = h.Store.UpdateInputChangeApproval(r.Context(), changeID, appID)
		common.WriteJSON(w, http.StatusAccepted, map[string]any{
			"message":        "Perubahan setelah batas cut-off memerlukan persetujuan Approval Center",
			"needs_approval": true,
			"change_id":      changeID,
			"approval_id":    appID,
		}, nil)
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"message":        "Input payroll berhasil disimpan",
		"needs_approval": false,
		"input_id":       inputID,
	}, nil)
}

func (h *Handler) PullAttendance(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	period := r.URL.Query().Get("period")
	if period == "" {
		common.WriteError(w, http.StatusBadRequest, "parameter period wajib diisi")
		return
	}

	count, err := h.Store.PullAttendance(r.Context(), schoolID, period)
	if err != nil {
		common.InternalError(w, "gagal menarik data absensi", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"message": fmt.Sprintf("Berhasil menarik data absensi untuk %d entri", count),
		"count":   count,
	}, nil)
}

// -----------------------------------------------------------------------------
// Payroll Runs
// -----------------------------------------------------------------------------

func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListRuns(r.Context(), schoolID)
	if err != nil {
		common.InternalError(w, "gagal memuat payroll run", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	run, err := h.Store.GetRun(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "payroll run tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, run, nil)
}

func (h *Handler) CreateRun(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var run PayrollRun
	if err := json.NewDecoder(r.Body).Decode(&run); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	run.SchoolID = schoolID

	created, err := h.Store.CreateRun(r.Context(), run)
	if err != nil {
		common.InternalError(w, "gagal membuat payroll run", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) CalculateRun(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := h.Store.CalculateRun(r.Context(), schoolID, id); err != nil {
		common.InternalError(w, "gagal menghitung payroll", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "payroll berhasil dihitung"}, nil)
}

// SubmitRun opens core approval payroll_run (amount = total net) and only
// then marks the run pending_approval. Core refusing (e.g. no flow, 422) is
// returned as-is; the run stays calculated.
func (h *Handler) SubmitRun(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	run, err := h.Store.GetRun(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "payroll run tidak ditemukan")
		return
	}
	if run.Status != "calculated" && run.Status != "draft" {
		common.WriteError(w, http.StatusConflict, "payroll run dengan status ini tidak dapat diajukan")
		return
	}
	version := int64(0)
	if run.CalculatedAt != nil {
		version = run.CalculatedAt.Unix()
	}
	total := run.TotalNetIDR
	appID, err := h.Approvals.CreateApproval(r.Context(), client.ApprovalRequest{
		SchoolID:    schoolID,
		RequestType: client.ReqTypePayrollRun,
		SourceID:    run.ID,
		Title:       fmt.Sprintf("Payroll periode %s (%s)", run.Period, run.Kind),
		Summary:     map[string]any{"period": run.Period, "kind": run.Kind, "total_lines": run.TotalLines},
		AmountIDR:   &total,
		MakerUserID: claims.UserID,
		// A recalculated run is a new approval subject.
		IdempotencyKey: fmt.Sprintf("%s:%s:%d", client.ReqTypePayrollRun, run.ID, version),
	})
	if err != nil {
		log.Printf("payroll run %s: create approval: %v", run.ID, err)
		status, msg := client.ApprovalErrorMessage(err)
		common.WriteError(w, status, msg)
		return
	}

	if err := h.Store.SubmitRun(r.Context(), schoolID, id, appID); err != nil {
		common.InternalError(w, "gagal mengajukan approval", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "payroll diajukan untuk persetujuan", "approval_id": appID}, nil)
}

func (h *Handler) FinalizeRun(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	if err := h.Store.FinalizeRun(r.Context(), schoolID, id, userID); err != nil {
		if errors.Is(err, ErrConflict) {
			common.WriteError(w, http.StatusConflict, "payroll belum disetujui di Approval Center")
			return
		}
		common.InternalError(w, "gagal finalisasi payroll", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "payroll telah difinalisasi, slip dibuat dan jurnal dibukukan"}, nil)
}

// -----------------------------------------------------------------------------
// Lines & Payslips
// -----------------------------------------------------------------------------

func (h *Handler) ListLines(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	flagged := r.URL.Query().Get("flagged") == "true"

	lines, err := h.Store.ListLines(r.Context(), schoolID, id, flagged)
	if err != nil {
		common.InternalError(w, "gagal memuat rincian payroll", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, lines, nil)
}

func (h *Handler) MyPayslips(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)

	list, err := h.Store.ListPayslips(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.InternalError(w, "gagal memuat slip gaji", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) ListPayslips(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	empID := r.URL.Query().Get("employee_id")

	list, err := h.Store.ListPayslips(r.Context(), schoolID, empID)
	if err != nil {
		common.InternalError(w, "gagal memuat slip gaji", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) ListJournals(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListJournals(r.Context(), schoolID)
	if err != nil {
		common.InternalError(w, "gagal memuat jurnal payroll", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}
