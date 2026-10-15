package payroll

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/common"
)

// GetStatutory: GET /payroll/statutory?date=YYYY-MM-DD (default today).
func (h *Handler) GetStatutory(w http.ResponseWriter, r *http.Request) {
	date := time.Now()
	if v := r.URL.Query().Get("date"); v != "" {
		d, err := time.Parse("2006-01-02", v)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "tanggal harus berformat YYYY-MM-DD")
			return
		}
		date = d
	}
	st, _, err := LoadStatutory(r.Context(), h.Store.DB, common.EffectiveSchoolID(r), date)
	if err != nil {
		common.InternalError(w, "gagal memuat parameter statutori", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, st, nil)
}

func periodParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	p := r.URL.Query().Get("period")
	if !ValidPeriod(p) {
		common.WriteError(w, http.StatusBadRequest, "parameter period wajib diisi dengan format YYYY-MM")
		return "", false
	}
	return p, true
}

func (h *Handler) ReportPPh21(w http.ResponseWriter, r *http.Request) {
	p, ok := periodParam(w, r)
	if !ok {
		return
	}
	rep, err := h.Store.PPh21Monthly(r.Context(), common.EffectiveSchoolID(r), p)
	if err != nil {
		common.InternalError(w, "gagal memuat laporan PPh 21", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, rep, nil)
}

func (h *Handler) ReportPPh21Annual(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(r.URL.Query().Get("year"))
	if err != nil || year < 2000 || year > 2100 {
		common.WriteError(w, http.StatusBadRequest, "parameter year wajib diisi (contoh 2026)")
		return
	}
	rep, err := h.Store.PPh21Annual(r.Context(), common.EffectiveSchoolID(r), year)
	if err != nil {
		common.InternalError(w, "gagal memuat laporan PPh 21 tahunan", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, rep, nil)
}

func (h *Handler) ReportBPJS(w http.ResponseWriter, r *http.Request) {
	p, ok := periodParam(w, r)
	if !ok {
		return
	}
	rep, err := h.Store.BPJSMonthly(r.Context(), common.EffectiveSchoolID(r), p)
	if err != nil {
		common.InternalError(w, "gagal memuat laporan BPJS", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, rep, nil)
}

func (h *Handler) ReportReconciliation(w http.ResponseWriter, r *http.Request) {
	p, ok := periodParam(w, r)
	if !ok {
		return
	}
	rep, err := h.Store.Reconciliation(r.Context(), common.EffectiveSchoolID(r), p)
	if err != nil {
		common.InternalError(w, "gagal memuat rekonsiliasi setoran", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, rep, nil)
}

func (h *Handler) ListRemittances(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListRemittances(r.Context(), common.EffectiveSchoolID(r), r.URL.Query().Get("period"))
	if err != nil {
		common.InternalError(w, "gagal memuat setoran", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateRemittance(w http.ResponseWriter, r *http.Request) {
	var body Remittance
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	body.Ref = strings.TrimSpace(body.Ref)
	switch {
	case !ValidPeriod(body.Period):
		common.WriteError(w, http.StatusBadRequest, "periode wajib diisi dengan format YYYY-MM")
		return
	case body.Kind != "pph21" && body.Kind != "bpjs_kesehatan" && body.Kind != "bpjs_tk":
		common.WriteError(w, http.StatusBadRequest, "jenis setoran harus pph21, bpjs_kesehatan, atau bpjs_tk")
		return
	case body.AmountIDR <= 0:
		common.WriteError(w, http.StatusBadRequest, "nominal setoran harus lebih dari 0")
		return
	case body.Ref == "":
		common.WriteError(w, http.StatusBadRequest, "nomor bukti setor (NTPN/referensi) wajib diisi")
		return
	}
	if _, err := time.Parse("2006-01-02", body.PaidOn); err != nil {
		common.WriteError(w, http.StatusBadRequest, "tanggal setor harus berformat YYYY-MM-DD")
		return
	}
	created, err := h.Store.CreateRemittance(r.Context(), common.EffectiveSchoolID(r), body)
	if err != nil {
		common.InternalError(w, "gagal menyimpan setoran", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) writePDF(w http.ResponseWriter, pdf []byte, name string) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

func (h *Handler) payslipPDFError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		common.WriteError(w, http.StatusNotFound, "slip gaji tidak ditemukan")
	case errors.Is(err, ErrForbidden):
		common.WriteError(w, http.StatusForbidden, "slip gaji ini bukan milik Anda")
	default:
		common.InternalError(w, "gagal membuat PDF slip gaji", err)
	}
}

// PayslipPDF: GET /payroll/payslips/{id}/pdf (payroll roles).
func (h *Handler) PayslipPDF(w http.ResponseWriter, r *http.Request) {
	pdf, name, err := h.Store.PayslipPDF(r.Context(), common.EffectiveSchoolID(r), chi.URLParam(r, "id"), "")
	if err != nil {
		h.payslipPDFError(w, err)
		return
	}
	h.writePDF(w, pdf, name)
}

// MyPayslipPDF: GET /me/payslips/{id}/pdf, owner only (AC-96-02).
func (h *Handler) MyPayslipPDF(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	empID, err := h.Store.EmployeeIDByUser(r.Context(), schoolID, claims.UserID)
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusForbidden, "akun Anda tidak terhubung ke data pegawai")
		return
	}
	if err != nil {
		common.InternalError(w, "gagal memuat slip gaji", err)
		return
	}
	pdf, name, err := h.Store.PayslipPDF(r.Context(), schoolID, chi.URLParam(r, "id"), empID)
	if err != nil {
		h.payslipPDFError(w, err)
		return
	}
	h.writePDF(w, pdf, name)
}

// RetryJournal: POST /payroll/journals/{id}/retry (FR-98-004).
func (h *Handler) RetryJournal(w http.ResponseWriter, r *http.Request) {
	j, err := h.Store.PostJournal(r.Context(), common.EffectiveSchoolID(r), chi.URLParam(r, "id"))
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "jurnal payroll tidak ditemukan")
		return
	}
	if err != nil {
		common.InternalError(w, "gagal mengirim ulang jurnal", err)
		return
	}
	msg := "jurnal dibukukan"
	status := http.StatusOK
	switch j.Status {
	case "failed":
		msg = "jurnal masih gagal: " + derefStr(j.Error)
		status = http.StatusBadGateway
	case "pending":
		msg = derefStr(j.Error)
		status = http.StatusServiceUnavailable
	}
	if status != http.StatusOK {
		common.WriteError(w, status, msg)
		return
	}
	common.WriteJSON(w, status, map[string]any{"message": msg, "journal": j}, nil)
}
