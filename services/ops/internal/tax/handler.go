package tax

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	opsclient "skci/ops/internal/client"
	"skci/ops/internal/common"
)

var (
	TaxWriters = []string{
		common.RoleFinanceManager,
		common.RoleFinanceAdmin,
		common.RoleAdmin,
		common.RoleSuperadmin,
	}

	TaxReaders = []string{
		common.RoleFinanceManager,
		common.RoleFinanceAdmin,
		common.RoleAdmin,
		common.RoleSuperadmin,
		common.RoleKepalaSekolah,
		common.RoleKepalaYayasan,
	}
)

type Handler struct {
	Store  *Store
	Client *opsclient.Client
}

func NewHandler(store *Store, client *opsclient.Client) *Handler {
	return &Handler{
		Store:  store,
		Client: client,
	}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	st, err := h.Store.GetSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pengaturan pajak: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, st, nil)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req TaxSettings
	if !common.Decode(w, r, &req) {
		return
	}
	req.SchoolID = schoolID
	if err := h.Store.UpdateSettings(r.Context(), &req); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pengaturan pajak: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, req, nil)
}

func (h *Handler) ListTaxTypes(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListTaxTypes(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat jenis pajak: "+err.Error())
		return
	}
	if list == nil {
		list = []WithholdingTaxType{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetTaxType(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	t, err := h.Store.GetTaxType(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Jenis pajak tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

func (h *Handler) CreateTaxType(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req WithholdingTaxType
	if !common.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode dan nama jenis pajak wajib diisi")
		return
	}
	req.SchoolID = schoolID
	created, err := h.Store.CreateTaxType(r.Context(), &req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat jenis pajak: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateTaxType(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req WithholdingTaxType
	if !common.Decode(w, r, &req) {
		return
	}
	req.SchoolID = schoolID
	req.ID = id
	if err := h.Store.UpdateTaxType(r.Context(), &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal mengubah jenis pajak: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Jenis pajak berhasil diperbarui"}, nil)
}

func (h *Handler) ListWithholdingSlips(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	period := r.URL.Query().Get("period")
	status := r.URL.Query().Get("status")
	vendorID := r.URL.Query().Get("vendor_id")
	list, err := h.Store.ListWithholdingSlips(r.Context(), schoolID, period, status, vendorID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat bukti potong: "+err.Error())
		return
	}
	if list == nil {
		list = []WithholdingSlip{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetWithholdingSlip(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	ws, err := h.Store.GetWithholdingSlip(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Bukti potong tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, ws, nil)
}

func (h *Handler) CreateWithholdingSlip(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	var req WithholdingSlip
	if !common.Decode(w, r, &req) {
		return
	}
	if req.TypeID == "" || req.VendorID == "" {
		common.WriteError(w, http.StatusBadRequest, "Vendor dan jenis pajak wajib diisi")
		return
	}
	if req.GrossAmountIDR <= 0 {
		common.WriteError(w, http.StatusBadRequest, "Jumlah bruto harus lebih besar dari 0")
		return
	}
	req.SchoolID = schoolID
	if claims != nil {
		req.CreatedBy = claims.UserID
	}
	created, err := h.Store.CreateWithholdingSlip(r.Context(), &req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat bukti potong: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) IssueWithholdingSlip(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.IssueWithholdingSlip(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal menerbitkan bukti potong: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Bukti potong berhasil diterbitkan"}, nil)
}

func (h *Handler) ListInputVAT(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	period := r.URL.Query().Get("period")
	status := r.URL.Query().Get("status")
	list, err := h.Store.ListInputVAT(r.Context(), schoolID, period, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pajak masukan: "+err.Error())
		return
	}
	if list == nil {
		list = []InputVAT{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateInputVAT(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req InputVAT
	if !common.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.InvoiceNumber) == "" || req.VendorID == "" {
		common.WriteError(w, http.StatusBadRequest, "Nomor faktur dan vendor wajib diisi")
		return
	}
	req.SchoolID = schoolID
	created, err := h.Store.CreateInputVAT(r.Context(), &req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat faktur pajak masukan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateInputVATStatus(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req struct {
		Status string `json:"status"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if err := h.Store.UpdateInputVATStatus(r.Context(), schoolID, id, req.Status); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal mengubah status faktur pajak: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Status faktur pajak diperbarui"}, nil)
}

func (h *Handler) ListOutputTaxInvoices(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	period := r.URL.Query().Get("period")
	list, err := h.Store.ListOutputTaxInvoices(r.Context(), schoolID, period)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat faktur keluaran: "+err.Error())
		return
	}
	if list == nil {
		list = []OutputTaxInvoice{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateOutputTaxInvoice(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req OutputTaxInvoice
	if !common.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.InvoiceNumber) == "" || strings.TrimSpace(req.CustomerName) == "" {
		common.WriteError(w, http.StatusBadRequest, "Nomor faktur dan nama pelanggan wajib diisi")
		return
	}
	req.SchoolID = schoolID
	created, err := h.Store.CreateOutputTaxInvoice(r.Context(), &req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat faktur pajak keluaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) ExportTax(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	var req struct {
		Kind      string `json:"kind"`       // ebupot_23, efaktur_input
		TaxPeriod string `json:"tax_period"` // YYYY-MM
	}
	if !common.Decode(w, r, &req) {
		return
	}
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}
	res, err := h.Store.GenerateExport(r.Context(), schoolID, req.Kind, req.TaxPeriod, userID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal mengekspor data pajak: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, res, nil)
}
