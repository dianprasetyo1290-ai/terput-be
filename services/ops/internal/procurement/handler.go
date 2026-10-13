package procurement

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/ops/internal/client"
	"skci/ops/internal/common"
	"skci/ops/internal/storage"
)

var Officers = []string{common.RoleProcurement, common.RoleFinanceManager, common.RoleAdmin, common.RoleSuperadmin}
var Readers = []string{common.RoleProcurement, common.RoleFinanceManager, common.RoleAdmin, common.RoleSuperadmin, common.RoleKepalaSekolah}

type Handler struct {
	Store        *Store
	Client       *client.Client
	Storage      storage.Storage
	CallbackBase string
}

func NewHandler(s *Store, c *client.Client, st storage.Storage, cb string) *Handler {
	return &Handler{
		Store:        s,
		Client:       c,
		Storage:      st,
		CallbackBase: cb,
	}
}

// --- Vendors ---

func (h *Handler) ListVendors(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	category := r.URL.Query().Get("category")
	status := r.URL.Query().Get("status")
	list, err := h.Store.ListVendors(r.Context(), schoolID, category, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetVendor(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	v, err := h.Store.GetVendor(r.Context(), schoolID, id)
	if err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Vendor tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, v, nil)
}

func (h *Handler) CreateVendor(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req struct {
		Code             string   `json:"code"`
		Name             string   `json:"name"`
		NPWP             string   `json:"npwp"`
		Address          *string  `json:"address"`
		Categories       []string `json:"categories"`
		PaymentTermsDays *int     `json:"payment_terms_days"`
		Status           string   `json:"status"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode dan nama vendor wajib diisi")
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Categories == nil {
		req.Categories = []string{}
	}

	v := Vendor{
		SchoolID:         schoolID,
		Code:             req.Code,
		Name:             req.Name,
		NPWP:             req.NPWP,
		Address:          req.Address,
		Categories:       req.Categories,
		PaymentTermsDays: req.PaymentTermsDays,
		Status:           req.Status,
	}
	created, err := h.Store.CreateVendor(r.Context(), v)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) CheckDuplicate(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req struct {
		NPWP      string `json:"npwp"`
		AccountNo string `json:"account_no"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	dups, err := h.Store.CheckDuplicates(r.Context(), schoolID, req.NPWP, req.AccountNo)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"is_duplicate": len(dups) > 0,
		"vendor_ids":   dups,
	}, nil)
}

func (h *Handler) AddBankAccount(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	vendorID := chi.URLParam(r, "id")
	userID := ""
	if claims := common.FromCtx(r); claims != nil {
		userID = claims.UserID
	}

	var req struct {
		BankName      string `json:"bank_name"`
		AccountNo     string `json:"account_no"`
		AccountHolder string `json:"account_holder"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.BankName == "" || req.AccountNo == "" || req.AccountHolder == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama bank, nomor rekening, dan nama pemilik wajib diisi")
		return
	}

	b := VendorBankAccount{
		SchoolID:      schoolID,
		VendorID:      vendorID,
		BankName:      req.BankName,
		AccountHolder: req.AccountHolder,
		RequestedBy:   userID,
	}
	created, err := h.Store.AddBankAccount(r.Context(), b, req.AccountNo)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// AC-106-01: submit approval to core for new bank account
	if h.Client != nil {
		callbackURL := fmt.Sprintf("%s/internal/approval-callbacks", h.CallbackBase)
		appID, err := h.Client.CreateApproval(r.Context(), client.ApprovalRequest{
			SchoolID:       schoolID,
			RequestType:    "vendor_bank_change",
			SourceService:  "ops",
			SourceID:       created.ID,
			Title:          fmt.Sprintf("Perubahan Rekening Vendor (%s)", created.BankName),
			Summary:        map[string]any{"bank_name": created.BankName, "account_holder": created.AccountHolder, "account_no_masked": created.AccountNo},
			MakerUserID:    userID,
			CallbackURL:    callbackURL,
			IdempotencyKey: fmt.Sprintf("vbank:%s", created.ID),
		})
		if err == nil {
			_, _ = h.Store.db.ExecContext(r.Context(), `UPDATE vendor_bank_accounts SET approval_id = $1 WHERE id = $2`, appID, created.ID)
			created.ApprovalID = &appID
		}
	}

	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) ListEvaluations(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	vendorID := chi.URLParam(r, "id")
	list, err := h.Store.ListEvaluations(r.Context(), schoolID, vendorID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// --- Purchase Requests ---

func (h *Handler) ListPRs(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	requesterID := ""
	if r.URL.Query().Get("mine") == "1" {
		if claims := common.FromCtx(r); claims != nil {
			requesterID = claims.UserID
		}
	}
	status := r.URL.Query().Get("status")
	list, err := h.Store.ListPRs(r.Context(), schoolID, requesterID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetPR(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	pr, err := h.Store.GetPR(r.Context(), schoolID, id)
	if err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Permintaan pembelian tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, pr, nil)
}

func (h *Handler) CreatePR(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID := ""
	if claims := common.FromCtx(r); claims != nil {
		userID = claims.UserID
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("pr:%s:%d", userID, time.Now().UnixNano())
	}

	var req struct {
		Reason   string     `json:"reason"`
		NeededBy *time.Time `json:"needed_by"`
		Lines    []struct {
			ItemID          *string `json:"item_id"`
			Description     string  `json:"description"`
			Qty             int64   `json:"qty"`
			Unit            string  `json:"unit"`
			EstUnitPriceIDR int64   `json:"est_unit_price_idr"`
			BudgetLineKey   *string `json:"budget_line_key"`
		} `json:"lines"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.Reason == "" || len(req.Lines) == 0 {
		common.WriteError(w, http.StatusBadRequest, "Alasan dan daftar barang wajib diisi")
		return
	}

	docNo, err := h.Store.NextDocNumber(r.Context(), schoolID, "PR", time.Now().Year(), "PR-{yyyy}-{seq:4}")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat nomor dokumen: "+err.Error())
		return
	}

	pr := PurchaseRequest{
		SchoolID:        schoolID,
		Number:          docNo,
		RequesterUserID: userID,
		Reason:          req.Reason,
		NeededBy:        req.NeededBy,
	}

	var lines []PurchaseRequestLine
	for _, l := range req.Lines {
		if l.Qty <= 0 {
			common.WriteError(w, http.StatusBadRequest, "Kuantitas baris harus > 0")
			return
		}
		lines = append(lines, PurchaseRequestLine{
			ItemID:          l.ItemID,
			Description:     l.Description,
			Qty:             l.Qty,
			Unit:            l.Unit,
			EstUnitPriceIDR: l.EstUnitPriceIDR,
			BudgetLineKey:   l.BudgetLineKey,
		})
	}

	created, err := h.Store.CreatePR(r.Context(), pr, lines, idempotencyKey)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) SubmitPR(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	userID := ""
	if claims := common.FromCtx(r); claims != nil {
		userID = claims.UserID
	}

	pr, err := h.Store.GetPR(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Permintaan pembelian tidak ditemukan")
		return
	}
	if pr.Status != "draft" {
		common.WriteError(w, http.StatusConflict, "Hanya draf yang dapat diajukan")
		return
	}

	// AC-104-01: Call BudgetCheck hook if registered
	var budgetRes *BudgetCheckResult
	var checkLines []BudgetCheckLine
	for _, l := range pr.Lines {
		if l.BudgetLineKey != nil && *l.BudgetLineKey != "" {
			checkLines = append(checkLines, BudgetCheckLine{
				BudgetLineKey: *l.BudgetLineKey,
				AmountIDR:     l.Qty * l.EstUnitPriceIDR,
			})
		}
	}

	if len(checkLines) > 0 && BudgetCheck != nil {
		res, err := BudgetCheck(r.Context(), nil, BudgetCheckInput{
			SchoolID:      schoolID,
			Lines:         checkLines,
			ExcludeSource: fmt.Sprintf("pr:%s", pr.ID),
		})
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal pengecekan anggaran: "+err.Error())
			return
		}
		budgetRes = &res
	}

	approvalFlags, err := ApprovalFlags(budgetRes)
	if err != nil {
		// Policy "reject"
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	overBudget := false
	for _, f := range approvalFlags {
		if f == "over_budget" {
			overBudget = true
		}
	}

	// Create Core approval
	callbackURL := fmt.Sprintf("%s/internal/approval-callbacks", h.CallbackBase)
	appID := ""
	if h.Client != nil {
		summary := map[string]any{
			"number":        pr.Number,
			"reason":        pr.Reason,
			"total_est_idr": pr.TotalEstIDR,
			"over_budget":   overBudget,
		}
		appID, err = h.Client.CreateApproval(r.Context(), client.ApprovalRequest{
			SchoolID:       schoolID,
			RequestType:    "purchase_request",
			SourceService:  "ops",
			SourceID:       pr.ID,
			Title:          fmt.Sprintf("Permintaan Pembelian %s (Rp%d)", pr.Number, pr.TotalEstIDR),
			Summary:        summary,
			MakerUserID:    userID,
			CallbackURL:    callbackURL,
			IdempotencyKey: fmt.Sprintf("pr_app:%s", pr.ID),
		})
		if err != nil {
			common.WriteError(w, http.StatusBadGateway, "Gagal mengajukan persetujuan ke Approval Center: "+err.Error())
			return
		}
	}

	if err := h.Store.SubmitPR(r.Context(), schoolID, id, appID, overBudget, budgetRes); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "pending_approval",
		"approval_id": appID,
		"over_budget": overBudget,
	}, nil)
}

func (h *Handler) CancelPR(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.CancelPR(r.Context(), schoolID, id); err != nil {
		if err == ErrInvalidState {
			common.WriteError(w, http.StatusConflict, "Permintaan pembelian tidak dapat dibatalkan pada status ini")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"}, nil)
}

// --- Purchase Orders ---

func (h *Handler) ListPOs(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	vendorID := r.URL.Query().Get("vendor_id")
	status := r.URL.Query().Get("status")
	list, err := h.Store.ListPOs(r.Context(), schoolID, vendorID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetPO(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	po, err := h.Store.GetPO(r.Context(), schoolID, id)
	if err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Pesanan pembelian tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, po, nil)
}

func (h *Handler) CreatePO(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("po:%s:%d", schoolID, time.Now().UnixNano())
	}

	var req struct {
		VendorID         string     `json:"vendor_id"`
		PaymentTermsDays int        `json:"payment_terms_days"`
		ExpectedDate     *time.Time `json:"expected_date"`
		Lines            []struct {
			PRLineID          *string `json:"pr_line_id"`
			ItemID            *string `json:"item_id"`
			Description       string  `json:"description"`
			Qty               int64   `json:"qty"`
			UnitPriceIDR      int64   `json:"unit_price_idr"`
			BudgetLineKey     *string `json:"budget_line_key"`
			WithholdingTypeID *string `json:"withholding_type_id"`
			WarehouseID       *string `json:"warehouse_id"`
		} `json:"lines"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.VendorID == "" || len(req.Lines) == 0 {
		common.WriteError(w, http.StatusBadRequest, "Vendor dan daftar barang wajib diisi")
		return
	}

	docNo, err := h.Store.NextDocNumber(r.Context(), schoolID, "PO", time.Now().Year(), "PO-{yyyy}-{seq:4}")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat nomor dokumen: "+err.Error())
		return
	}

	po := PurchaseOrder{
		SchoolID:         schoolID,
		Number:           docNo,
		VendorID:         req.VendorID,
		PaymentTermsDays: req.PaymentTermsDays,
		ExpectedDate:     req.ExpectedDate,
	}

	var lines []PurchaseOrderLine
	for _, l := range req.Lines {
		if l.Qty <= 0 || l.UnitPriceIDR < 0 {
			common.WriteError(w, http.StatusBadRequest, "Kuantitas (>0) dan harga satuan (>=0) wajib valid")
			return
		}
		lines = append(lines, PurchaseOrderLine{
			PRLineID:          l.PRLineID,
			ItemID:            l.ItemID,
			Description:       l.Description,
			Qty:               l.Qty,
			UnitPriceIDR:      l.UnitPriceIDR,
			BudgetLineKey:     l.BudgetLineKey,
			WithholdingTypeID: l.WithholdingTypeID,
			WarehouseID:       l.WarehouseID,
		})
	}

	created, err := h.Store.CreatePO(r.Context(), po, lines, idempotencyKey)
	if err != nil {
		if err == ErrVendorBlocked {
			common.WriteError(w, http.StatusUnprocessableEntity, "Vendor diblokir atau tidak aktif")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) SubmitPO(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	userID := ""
	if claims := common.FromCtx(r); claims != nil {
		userID = claims.UserID
	}

	po, err := h.Store.GetPO(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Pesanan pembelian tidak ditemukan")
		return
	}
	if po.Status != "draft" {
		common.WriteError(w, http.StatusConflict, "Hanya draf yang dapat diajukan")
		return
	}

	// Create Core approval
	callbackURL := fmt.Sprintf("%s/internal/approval-callbacks", h.CallbackBase)
	appID := ""
	if h.Client != nil {
		summary := map[string]any{
			"number":      po.Number,
			"vendor_name": po.VendorName,
			"total_idr":   po.TotalIDR,
		}
		appID, err = h.Client.CreateApproval(r.Context(), client.ApprovalRequest{
			SchoolID:       schoolID,
			RequestType:    "purchase_order",
			SourceService:  "ops",
			SourceID:       po.ID,
			Title:          fmt.Sprintf("Pesanan Pembelian %s (Rp%d)", po.Number, po.TotalIDR),
			Summary:        summary,
			MakerUserID:    userID,
			CallbackURL:    callbackURL,
			IdempotencyKey: fmt.Sprintf("po_app:%s", po.ID),
		})
		if err != nil {
			common.WriteError(w, http.StatusBadGateway, "Gagal mengajukan persetujuan ke Approval Center: "+err.Error())
			return
		}
	}

	if err := h.Store.SubmitPO(r.Context(), schoolID, id, appID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{
		"status":      "pending_approval",
		"approval_id": appID,
	}, nil)
}

func (h *Handler) MarkPOSent(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.MarkPOSent(r.Context(), schoolID, id); err != nil {
		if err == ErrInvalidState {
			common.WriteError(w, http.StatusConflict, "Hanya PO yang berstatus approved yang dapat ditandai terkirim")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "sent"}, nil)
}

func (h *Handler) ClosePO(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if err := h.Store.ClosePO(r.Context(), schoolID, id, req.Reason); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "closed"}, nil)
}

func (h *Handler) CancelPO(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.CancelPO(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"}, nil)
}

func (h *Handler) GetPOReportPDF(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	po, err := h.Store.GetPO(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "PO tidak ditemukan")
		return
	}

	pdfData := buildPOPDF(po)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="PO-%s.pdf"`, po.Number))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfData)
}

func buildPOPDF(po *PurchaseOrder) []byte {
	content := fmt.Sprintf("%%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >> endobj\n5 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\n4 0 obj << /Length %d >> stream\nBT /F1 16 Tf 50 720 Td (PURCHASE ORDER) Tj ET\nBT /F1 10 Tf 50 690 Td (Nomor: %s   Vendor: %s   Total: Rp%d) Tj ET\n", len(po.Number)+100, po.Number, po.VendorName, po.TotalIDR)
	y := 650
	for i, l := range po.Lines {
		if i > 25 {
			break
		}
		lineStr := fmt.Sprintf("%s: %d x Rp%d = Rp%d", l.Description, l.Qty, l.UnitPriceIDR, l.AmountIDR)
		content += fmt.Sprintf("BT /F1 9 Tf 50 %d Td (%s) Tj ET\n", y, lineStr)
		y -= 18
	}
	content += "endstream\nendobj\nxref\n0 6\n0000000000 65535 f \n0000000010 00000 n \n0000000060 00000 n \n0000000117 00000 n \n0000000280 00000 n \n0000000215 00000 n \ntrailer << /Size 6 /Root 1 0 R >>\nstartxref\n500\n%%EOF"
	return []byte(content)
}

// --- Goods Receipts & Returns ---

func (h *Handler) ListReceipts(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	poID := r.URL.Query().Get("po_id")
	list, err := h.Store.ListReceipts(r.Context(), schoolID, poID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateReceipt(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	poID := chi.URLParam(r, "id")
	userID := ""
	if claims := common.FromCtx(r); claims != nil {
		userID = claims.UserID
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("gr:%s:%d", poID, time.Now().UnixNano())
	}

	var req struct {
		ReceivedOn time.Time `json:"received_on"`
		Note       *string   `json:"note"`
		Lines      []struct {
			POLineID     string  `json:"po_line_id"`
			QtyReceived  int64   `json:"qty_received"`
			QtyAccepted  int64   `json:"qty_accepted"`
			QtyRejected  int64   `json:"qty_rejected"`
			RejectReason *string `json:"reject_reason"`
		} `json:"lines"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if len(req.Lines) == 0 {
		common.WriteError(w, http.StatusBadRequest, "Daftar penerimaan baris wajib diisi")
		return
	}
	if req.ReceivedOn.IsZero() {
		req.ReceivedOn = time.Now()
	}

	docNo, err := h.Store.NextDocNumber(r.Context(), schoolID, "GR", time.Now().Year(), "GR-{yyyy}-{seq:4}")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat nomor dokumen: "+err.Error())
		return
	}

	gr := GoodsReceipt{
		SchoolID:   schoolID,
		Number:     docNo,
		POID:       poID,
		ReceivedOn: req.ReceivedOn,
		ReceivedBy: userID,
		Note:       req.Note,
	}

	var lines []GoodsReceiptLine
	for _, l := range req.Lines {
		lines = append(lines, GoodsReceiptLine{
			POLineID:     l.POLineID,
			QtyReceived:  l.QtyReceived,
			QtyAccepted:  l.QtyAccepted,
			QtyRejected:  l.QtyRejected,
			RejectReason: l.RejectReason,
		})
	}

	created, err := h.Store.CreateReceipt(r.Context(), gr, lines, idempotencyKey)
	if err != nil {
		if err == ErrExcessReceipt {
			common.WriteError(w, http.StatusUnprocessableEntity, "Kuantitas diterima melebihi sisa PO")
			return
		}
		if err == ErrInvalidState {
			common.WriteError(w, http.StatusConflict, "PO belum siap untuk penerimaan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) CreateReturn(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	grLineID := chi.URLParam(r, "id")
	userID := ""
	if claims := common.FromCtx(r); claims != nil {
		userID = claims.UserID
	}

	var req struct {
		Qty    int64  `json:"qty"`
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.Qty <= 0 || req.Reason == "" {
		common.WriteError(w, http.StatusBadRequest, "Kuantitas retur (>0) dan alasan wajib diisi")
		return
	}

	ret := GoodsReturn{
		SchoolID:   schoolID,
		GRLineID:   grLineID,
		Qty:        req.Qty,
		Reason:     req.Reason,
		ReturnedOn: time.Now(),
		CreatedBy:  userID,
	}
	created, err := h.Store.CreateReturn(r.Context(), ret)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) CreateEvaluation(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	grID := chi.URLParam(r, "id")
	userID := ""
	if claims := common.FromCtx(r); claims != nil {
		userID = claims.UserID
	}

	var req struct {
		VendorID     string  `json:"vendor_id"`
		QualityScore int     `json:"quality_score"`
		PriceScore   int     `json:"price_score"`
		Note         *string `json:"note"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.QualityScore < 1 || req.QualityScore > 5 || req.PriceScore < 1 || req.PriceScore > 5 {
		common.WriteError(w, http.StatusBadRequest, "Skor kualitas dan harga harus antara 1 sampai 5")
		return
	}

	eval := VendorEvaluation{
		SchoolID:       schoolID,
		VendorID:       req.VendorID,
		GoodsReceiptID: grID,
		QualityScore:   req.QualityScore,
		PriceScore:     req.PriceScore,
		Note:           req.Note,
		CreatedBy:      userID,
	}
	created, err := h.Store.CreateEvaluation(r.Context(), eval)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// ApprovalDecision matches Core callback.
type ApprovalDecision struct {
	ApprovalID  string `json:"approval_id"`
	SourceID    string `json:"source_id"`
	RequestType string `json:"request_type"`
	Decision    string `json:"decision"`
	DecidedBy   string `json:"decided_by"`
	Comment     string `json:"comment"`
}

func (h *Handler) ApplyApprovalDecision(ctx context.Context, d ApprovalDecision) error {
	switch d.RequestType {
	case "purchase_request":
		status := "approved"
		if d.Decision != "approved" {
			status = "rejected"
		}
		_, err := h.Store.db.ExecContext(ctx, `
			UPDATE purchase_requests
			SET status = $1, updated_at = now()
			WHERE id = $2 AND status = 'pending_approval'
		`, status, d.SourceID)
		return err

	case "purchase_order":
		if d.Decision == "approved" {
			// Save PDF object key if needed
			return h.Store.SetPOApproved(ctx, d.SourceID, fmt.Sprintf("po/%s.pdf", d.SourceID))
		}
		_, err := h.Store.db.ExecContext(ctx, `
			UPDATE purchase_orders
			SET status = 'rejected', updated_at = now()
			WHERE id = $1 AND status = 'pending_approval'
		`, d.SourceID)
		return err

	case "vendor_bank_change":
		var schoolID string
		err := h.Store.db.QueryRowContext(ctx, `SELECT school_id FROM vendor_bank_accounts WHERE id = $1`, d.SourceID).Scan(&schoolID)
		if err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return err
		}
		if d.Decision == "approved" {
			return h.Store.ActivateBankAccount(ctx, schoolID, d.SourceID)
		}
		return h.Store.RejectBankAccount(ctx, schoolID, d.SourceID)

	default:
		return fmt.Errorf("unsupported request type: %s", d.RequestType)
	}
}
