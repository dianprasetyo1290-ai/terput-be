package payroll

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/client"
	"skci/hr/internal/common"
	"skci/hr/internal/payroll/calc"
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
	c.Code = strings.ToUpper(strings.TrimSpace(c.Code))
	c.Name = strings.TrimSpace(c.Name)
	if c.Code == "" || c.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "kode dan nama komponen wajib diisi")
		return
	}
	if c.Kind != "earning" && c.Kind != "deduction" {
		common.WriteError(w, http.StatusBadRequest, "jenis komponen harus earning atau deduction")
		return
	}
	if c.Nature == "" {
		c.Nature = "fixed"
	}
	if c.Nature != "fixed" && c.Nature != "variable" {
		common.WriteError(w, http.StatusBadRequest, "sifat komponen harus fixed atau variable")
		return
	}
	switch c.Calc {
	case "fixed_amount", "per_hour", "from_structure", "from_input", "overtime":
	default:
		common.WriteError(w, http.StatusBadRequest, "metode hitung komponen tidak dikenal")
		return
	}
	if (c.Calc == "per_hour" || c.Calc == "overtime") && (c.RatePerUnitIDR == nil || *c.RatePerUnitIDR <= 0) {
		common.WriteError(w, http.StatusBadRequest, "tarif per satuan wajib diisi untuk komponen per jam/lembur")
		return
	}
	if c.AccCode == nil || strings.TrimSpace(*c.AccCode) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "akun beban/kewajiban komponen wajib diisi (BR-PRL-04)")
		return
	}

	created, err := h.Store.CreateComponent(r.Context(), c)
	if errors.Is(err, ErrConflict) {
		common.WriteError(w, http.StatusConflict, "kode komponen sudah dipakai")
		return
	}
	if err != nil {
		common.InternalError(w, "gagal membuat komponen", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// SetComponentAccounts: PUT /payroll/components/{id}/accounts.
func (h *Handler) SetComponentAccounts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccCode          string  `json:"acc_code"`
		LiabilityAccCode *string `json:"liability_acc_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.AccCode) == "" {
		common.WriteError(w, http.StatusBadRequest, "acc_code wajib diisi")
		return
	}
	err := h.Store.SetComponentAccounts(r.Context(), common.EffectiveSchoolID(r), chi.URLParam(r, "id"), strings.TrimSpace(body.AccCode), body.LiabilityAccCode)
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "komponen gaji tidak ditemukan")
		return
	}
	if err != nil {
		common.InternalError(w, "gagal menyimpan mapping akun", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "mapping akun disimpan"}, nil)
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
		var body struct {
			Period string `json:"period"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		period = body.Period
	}
	if !ValidPeriod(period) {
		common.WriteError(w, http.StatusBadRequest, "periode wajib diisi dengan format YYYY-MM")
		return
	}

	res, err := h.Store.PullAttendance(r.Context(), schoolID, period)
	if errors.Is(err, ErrAttendanceNotSent) {
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		common.InternalError(w, "gagal menarik data absensi", err)
		return
	}
	msg := fmt.Sprintf("Absensi ditarik: %d input lembur, %d input potongan alpa", res.Overtime, res.Absence)
	if len(res.Missing) > 0 {
		msg += ". Belum dikonfigurasi: " + strings.Join(res.Missing, ", ")
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"message":        msg,
		"inserted_count": res.InsertedCount,
		"result":         res,
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
	if !ValidPeriod(run.Period) {
		common.WriteError(w, http.StatusBadRequest, "periode wajib diisi dengan format YYYY-MM")
		return
	}
	if run.Kind == "" {
		run.Kind = "regular"
	}
	if run.Kind != "regular" && run.Kind != "supplementary" {
		common.WriteError(w, http.StatusBadRequest, "jenis run harus regular atau supplementary")
		return
	}

	created, err := h.Store.CreateRun(r.Context(), run)
	var unmapped *UnmappedComponentsError
	switch {
	case errors.As(err, &unmapped):
		common.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": unmapped.Error(), "components": unmapped.Codes}, nil)
		return
	case errors.Is(err, ErrDuplicateRegular):
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, ErrConflict):
		common.WriteError(w, http.StatusConflict, "payroll run dengan kunci yang sama sudah ada")
		return
	case err != nil && strings.HasPrefix(err.Error(), "run susulan"):
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case err != nil:
		common.InternalError(w, "gagal membuat payroll run", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) CalculateRun(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	totals, err := h.Store.CalculateRun(r.Context(), schoolID, id)
	var missing *calc.MissingParamError
	switch {
	case errors.Is(err, ErrNotFound):
		common.WriteError(w, http.StatusNotFound, "payroll run tidak ditemukan")
		return
	case errors.Is(err, ErrRunNotEditable):
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	case errors.As(err, &missing):
		common.WriteError(w, http.StatusUnprocessableEntity, "Parameter statutori belum lengkap: "+missing.What+" belum dikonfigurasi")
		return
	case err != nil:
		common.InternalError(w, "gagal menghitung payroll", err)
		return
	}
	msg := fmt.Sprintf("payroll berhasil dihitung untuk %d pegawai", totals.Lines)
	if len(totals.Skipped) > 0 {
		msg += fmt.Sprintf("; %d pegawai dilewati (lihat rincian)", len(totals.Skipped))
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"message": msg, "totals": totals}, nil)
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

	journal, err := h.Store.FinalizeRun(r.Context(), schoolID, id, userID)
	var unmapped *UnmappedComponentsError
	switch {
	case errors.Is(err, ErrConflict):
		common.WriteError(w, http.StatusConflict, "payroll belum disetujui di Approval Center")
		return
	case errors.As(err, &unmapped):
		common.WriteError(w, http.StatusUnprocessableEntity, strings.Join(unmapped.Codes, "; "))
		return
	case err != nil:
		common.InternalError(w, "gagal finalisasi payroll", err)
		return
	}
	msg := "payroll final, slip gaji diterbitkan"
	switch journal.Status {
	case "posted":
		msg += " dan jurnal dibukukan (" + derefStr(journal.TransNo) + ")"
	case "failed":
		msg += "; jurnal gagal dikirim ke akuntansi: " + derefStr(journal.Error) + ". Coba lagi dari menu Jurnal Payroll"
	default:
		msg += "; jurnal menunggu: " + derefStr(journal.Error)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"message": msg, "journal": journal}, nil)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
	empID, err := h.Store.EmployeeIDByUser(r.Context(), schoolID, claims.UserID)
	if errors.Is(err, ErrNotFound) {
		common.WriteJSON(w, http.StatusOK, []Payslip{}, nil)
		return
	}
	if err != nil {
		common.InternalError(w, "gagal memuat slip gaji", err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.Store.ListPayslips(r.Context(), schoolID, empID, "", limit)
	if err != nil {
		common.InternalError(w, "gagal memuat slip gaji", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) ListPayslips(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	list, err := h.Store.ListPayslips(r.Context(), schoolID, q.Get("employee_id"), q.Get("run_id"), limit)
	if err != nil {
		common.InternalError(w, "gagal memuat slip gaji", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) ListJournals(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListJournals(r.Context(), schoolID, r.URL.Query().Get("period"))
	if err != nil {
		common.InternalError(w, "gagal memuat jurnal payroll", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": list, "accounting_configured": h.Store.Accounting.Configured()}, nil)
}
