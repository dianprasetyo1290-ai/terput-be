package payable

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	opsclient "skci/ops/internal/client"
	"skci/ops/internal/common"
)

var (
	PayableWriters = []string{
		common.RoleFinanceManager,
		common.RoleFinanceAdmin,
		common.RoleAdmin,
		common.RoleSuperadmin,
	}

	PayableReaders = []string{
		common.RoleFinanceManager,
		common.RoleFinanceAdmin,
		common.RoleAdmin,
		common.RoleSuperadmin,
		common.RoleKepalaSekolah,
		common.RoleKepalaYayasan,
	}
)

type Handler struct {
	Store        *Store
	Client       *opsclient.Client
	CallbackBase string
}

func NewHandler(store *Store, client *opsclient.Client, callbackBase string) *Handler {
	return &Handler{
		Store:        store,
		Client:       client,
		CallbackBase: callbackBase,
	}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	st, err := h.Store.GetSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pengaturan utang: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, st, nil)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req PayableSettings
	if !common.Decode(w, r, &req) {
		return
	}
	req.SchoolID = schoolID
	if err := h.Store.UpdateSettings(r.Context(), &req); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pengaturan utang: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, req, nil)
}

func (h *Handler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")
	vendorID := r.URL.Query().Get("vendor_id")
	list, err := h.Store.ListInvoices(r.Context(), schoolID, status, vendorID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat faktur vendor: "+err.Error())
		return
	}
	if list == nil {
		list = []VendorInvoice{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	inv, err := h.Store.GetInvoice(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Faktur vendor tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, inv, nil)
}

func (h *Handler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	var req VendorInvoice
	if !common.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Number) == "" || req.VendorID == "" {
		common.WriteError(w, http.StatusBadRequest, "Nomor faktur dan vendor wajib diisi")
		return
	}
	if len(req.Lines) == 0 {
		common.WriteError(w, http.StatusBadRequest, "Faktur harus memiliki minimal satu baris item")
		return
	}
	req.SchoolID = schoolID
	if claims != nil {
		req.CreatedBy = claims.UserID
	}
	created, err := h.Store.CreateInvoice(r.Context(), &req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat faktur vendor: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) RunMatch(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	res, err := h.Store.RunThreeWayMatch(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menjalankan 3-way match: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, res, nil)
}

func (h *Handler) RequestOverride(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var req struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &req) {
		return
	}

	inv, err := h.Store.GetInvoice(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Faktur vendor tidak ditemukan")
		return
	}
	if inv.Status != "discrepancy" {
		common.WriteError(w, http.StatusBadRequest, "Hanya faktur dengan status diskrepansi yang dapat mengajukan dispensasi override")
		return
	}

	requesterID := ""
	if claims != nil {
		requesterID = claims.UserID
	}

	discNote := ""
	if inv.MatchDiscrepancyNote != nil {
		discNote = *inv.MatchDiscrepancyNote
	}

	approvalID := ""
	if h.Client != nil {
		callbackURL := ""
		if h.CallbackBase != "" {
			callbackURL = h.CallbackBase + "/internal/approval-callbacks"
		}
		var err error
		approvalID, err = h.Client.CreateApproval(r.Context(), opsclient.ApprovalRequest{
			SchoolID:       schoolID,
			RequestType:    "vendor_invoice_override",
			SourceService:  "ops",
			SourceID:       inv.ID,
			Title:          "Dispensasi Diskrepansi Faktur Vendor (" + inv.Number + ")",
			Summary: map[string]any{
				"invoice_id":       inv.ID,
				"invoice_number":   inv.Number,
				"vendor_name":      inv.VendorName,
				"total_idr":        inv.TotalIDR,
				"reason":           req.Reason,
				"discrepancy_note": discNote,
			},
			MakerUserID:    requesterID,
			CallbackURL:    callbackURL,
			IdempotencyKey: "vinv:override:" + inv.ID,
		})
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal mengajukan dispensasi override: "+err.Error())
			return
		}
	}

	if err := h.Store.RequestMatchOverride(r.Context(), schoolID, id, approvalID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui status dispensasi: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{
		"message":     "Dispensasi override berhasil diajukan",
		"approval_id": approvalID,
	}, nil)
}

func (h *Handler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	invoiceID := r.URL.Query().Get("invoice_id")
	status := r.URL.Query().Get("status")
	list, err := h.Store.ListPaymentSchedules(r.Context(), schoolID, invoiceID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat jadwal pembayaran: "+err.Error())
		return
	}
	if list == nil {
		list = []PaymentSchedule{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	var req PaymentSchedule
	if !common.Decode(w, r, &req) {
		return
	}
	if req.InvoiceID == "" || req.ScheduledDate == "" || req.AmountIDR <= 0 {
		common.WriteError(w, http.StatusBadRequest, "Faktur, tanggal jadwal, dan jumlah pembayaran wajib valid")
		return
	}
	req.SchoolID = schoolID
	if claims != nil {
		req.CreatedBy = claims.UserID
	}
	created, err := h.Store.CreatePaymentSchedule(r.Context(), &req)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal menjadwalkan pembayaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) SubmitSchedule(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")

	requesterID := ""
	if claims != nil {
		requesterID = claims.UserID
	}

	approvalID := ""
	if h.Client != nil {
		callbackURL := ""
		if h.CallbackBase != "" {
			callbackURL = h.CallbackBase + "/internal/approval-callbacks"
		}
		var err error
		approvalID, err = h.Client.CreateApproval(r.Context(), opsclient.ApprovalRequest{
			SchoolID:       schoolID,
			RequestType:    "vendor_payment",
			SourceService:  "ops",
			SourceID:       id,
			Title:          "Persetujuan Pembayaran Utang Vendor (" + id + ")",
			Summary: map[string]any{
				"schedule_id": id,
			},
			MakerUserID:    requesterID,
			CallbackURL:    callbackURL,
			IdempotencyKey: "vpay:submit:" + id,
		})
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal mengajukan persetujuan pembayaran: "+err.Error())
			return
		}
	}

	if err := h.Store.SubmitPaymentSchedule(r.Context(), schoolID, id, approvalID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui status pengajuan: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{
		"message":     "Jadwal pembayaran berhasil diajukan untuk persetujuan",
		"approval_id": approvalID,
	}, nil)
}

func (h *Handler) ExecuteSchedule(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var req struct {
		ReferenceNo string `json:"reference_no"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	executorID := ""
	if claims != nil {
		executorID = claims.UserID
	}
	if err := h.Store.ExecutePaymentSchedule(r.Context(), schoolID, id, executorID, req.ReferenceNo); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal mengeksekusi pembayaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Pembayaran berhasil dieksekusi"}, nil)
}

func (h *Handler) GetAgingReport(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	rep, err := h.Store.GetAgingReport(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat laporan aging AP: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, rep, nil)
}

func (h *Handler) ListJournals(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListPayableJournals(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat riwayat jurnal utang: "+err.Error())
		return
	}
	if list == nil {
		list = []PayableJournal{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}
