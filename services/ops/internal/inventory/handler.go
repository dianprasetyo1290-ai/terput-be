package inventory

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/ops/internal/client"
	"skci/ops/internal/common"
	"skci/ops/internal/pdfdoc"
	"skci/ops/internal/storage"
)

var Officers = []string{common.RoleInventoryOfficer, common.RoleAdmin, common.RoleSuperadmin}
var Readers = []string{common.RoleInventoryOfficer, common.RoleAdmin, common.RoleSuperadmin, common.RoleKepalaSekolah, common.RoleFinanceManager}

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

// --- Categories ---

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListCategories(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req struct {
		Code              string  `json:"code"`
		Name              string  `json:"name"`
		Tracking          string  `json:"tracking"`
		PurchaseAccCode   *string `json:"purchase_acc_code"`
		WithholdingTypeID *string `json:"withholding_type_id"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" || (req.Tracking != "stock" && req.Tracking != "asset" && req.Tracking != "none") {
		common.WriteError(w, http.StatusBadRequest, "Kode, nama, dan jenis tracking ('stock'/'asset'/'none') wajib valid")
		return
	}
	c := ItemCategory{
		SchoolID:          schoolID,
		Code:              req.Code,
		Name:              req.Name,
		Tracking:          req.Tracking,
		PurchaseAccCode:   req.PurchaseAccCode,
		WithholdingTypeID: req.WithholdingTypeID,
	}
	created, err := h.Store.CreateCategory(r.Context(), c)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req struct {
		Name              string  `json:"name"`
		Tracking          string  `json:"tracking"`
		PurchaseAccCode   *string `json:"purchase_acc_code"`
		WithholdingTypeID *string `json:"withholding_type_id"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	c := ItemCategory{
		ID:                id,
		SchoolID:          schoolID,
		Name:              req.Name,
		Tracking:          req.Tracking,
		PurchaseAccCode:   req.PurchaseAccCode,
		WithholdingTypeID: req.WithholdingTypeID,
	}
	if err := h.Store.UpdateCategory(r.Context(), c); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Kategori tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteCategory(r.Context(), schoolID, id); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Kategori tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// --- Items ---

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	categoryID := r.URL.Query().Get("category_id")
	status := r.URL.Query().Get("status")
	list, err := h.Store.ListItems(r.Context(), schoolID, categoryID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	item, err := h.Store.GetItem(r.Context(), schoolID, id)
	if err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Barang tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req struct {
		Code       string `json:"code"`
		Name       string `json:"name"`
		CategoryID string `json:"category_id"`
		Unit       string `json:"unit"`
		MinStock   int64  `json:"min_stock"`
		Status     string `json:"status"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" || req.CategoryID == "" || req.Unit == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode, nama, kategori, dan satuan wajib diisi")
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	it := Item{
		SchoolID:   schoolID,
		Code:       req.Code,
		Name:       req.Name,
		CategoryID: req.CategoryID,
		Unit:       req.Unit,
		MinStock:   req.MinStock,
		Status:     req.Status,
	}
	created, err := h.Store.CreateItem(r.Context(), it)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req struct {
		Name       string `json:"name"`
		CategoryID string `json:"category_id"`
		Unit       string `json:"unit"`
		MinStock   int64  `json:"min_stock"`
		Status     string `json:"status"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	it := Item{
		ID:         id,
		SchoolID:   schoolID,
		Name:       req.Name,
		CategoryID: req.CategoryID,
		Unit:       req.Unit,
		MinStock:   req.MinStock,
		Status:     req.Status,
	}
	if err := h.Store.UpdateItem(r.Context(), it); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Barang tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteItem(r.Context(), schoolID, id); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Barang tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// --- Warehouses ---

func (h *Handler) ListWarehouses(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListWarehouses(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateWarehouse(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req struct {
		Code   string  `json:"code"`
		Name   string  `json:"name"`
		RoomID *string `json:"room_id"`
		Status string  `json:"status"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode dan nama gudang wajib diisi")
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	wh := Warehouse{
		SchoolID: schoolID,
		Code:     req.Code,
		Name:     req.Name,
		RoomID:   req.RoomID,
		Status:   req.Status,
	}
	created, err := h.Store.CreateWarehouse(r.Context(), wh)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateWarehouse(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req struct {
		Name   string  `json:"name"`
		RoomID *string `json:"room_id"`
		Status string  `json:"status"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	wh := Warehouse{
		ID:       id,
		SchoolID: schoolID,
		Name:     req.Name,
		RoomID:   req.RoomID,
		Status:   req.Status,
	}
	if err := h.Store.UpdateWarehouse(r.Context(), wh); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Gudang tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (h *Handler) DeleteWarehouse(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteWarehouse(r.Context(), schoolID, id); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Gudang tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// --- Stock & Movements ---

func (h *Handler) ListStock(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	warehouseID := r.URL.Query().Get("warehouse_id")
	itemID := r.URL.Query().Get("item_id")
	belowMin := r.URL.Query().Get("below_min") == "1" || r.URL.Query().Get("below_min") == "true"

	balances, err := h.Store.ListBalances(r.Context(), schoolID, warehouseID, itemID, belowMin)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, balances, nil)
}

func (h *Handler) GetStockCard(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	itemID := r.URL.Query().Get("item_id")
	warehouseID := r.URL.Query().Get("warehouse_id")
	// warehouse_id is optional: without it the card is consolidated across
	// every warehouse ("Konsolidasi Semua Gudang" in the SPA).
	if itemID == "" {
		common.WriteError(w, http.StatusBadRequest, "item_id wajib diisi")
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" {
		fromStr = r.URL.Query().Get("start") // SPA param name
	}
	if toStr == "" {
		toStr = r.URL.Query().Get("end")
	}
	from := time.Now().AddDate(0, -1, 0)
	to := time.Now()
	if t, err := time.Parse("2006-01-02", fromStr); err == nil {
		from = t
	}
	if t, err := time.Parse("2006-01-02", toStr); err == nil {
		to = t.Add(24*time.Hour - time.Nanosecond)
	}

	card, err := h.Store.GetStockCard(r.Context(), schoolID, warehouseID, itemID, from, to)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, card, nil)
}

func (h *Handler) CreateMovement(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID, role := "", ""
	if claims := common.FromCtx(r); claims != nil {
		userID, role = claims.UserID, claims.ActiveRole
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		common.WriteError(w, http.StatusBadRequest, "Header Idempotency-Key wajib diisi")
		return
	}

	var req struct {
		Kind        string  `json:"kind"` // 'issue' or 'receipt'
		WarehouseID string  `json:"warehouse_id"`
		ItemID      string  `json:"item_id"`
		Qty         int64   `json:"qty"`
		RefNo       string  `json:"ref_no"`
		Note        *string `json:"note"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.WarehouseID == "" || req.ItemID == "" || req.Qty <= 0 || (req.Kind != "issue" && req.Kind != "receipt") {
		common.WriteError(w, http.StatusBadRequest, "Gudang, barang, kuantitas (>0), dan jenis ('issue'/'receipt') wajib valid")
		return
	}

	direction := "in"
	if req.Kind == "issue" {
		direction = "out"
	}

	refNo := req.RefNo
	if refNo == "" {
		refNo = fmt.Sprintf("MANUAL-%d", time.Now().Unix())
	}

	tx, err := h.Store.db.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	in := MoveInput{
		SchoolID:       schoolID,
		WarehouseID:    req.WarehouseID,
		ItemID:         req.ItemID,
		Direction:      direction,
		Qty:            req.Qty,
		Kind:           req.Kind,
		RefType:        "manual",
		RefID:          schoolID,
		RefNo:          refNo,
		Note:           req.Note,
		CreatedBy:      userID,
		ActorRole:      role,
		IdempotencyKey: idempotencyKey,
	}

	balanceAfter, crossedBelow, err := h.Store.Move(r.Context(), tx, in)
	if err != nil {
		if err == ErrCountingActive {
			common.WriteError(w, http.StatusConflict, "Gudang sedang stock opname")
			return
		}
		// Negative stock balance gives standard 422
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// AC-102-02: notify inventory officers if crossed below minimum
	if crossedBelow && h.Client != nil {
		go func() {
			users, _ := h.Client.UsersWithRole(context.Background(), schoolID, "inventory_officer")
			for _, u := range users {
				_ = h.Client.Notify(context.Background(), client.Notification{
					SchoolID: schoolID,
					UserID:   u.ID,
					Type:     "stock_min",
					Title:    "Stok Menipis",
					Body:     fmt.Sprintf("Stok barang %s telah mencapai/berada di bawah batas minimum.", req.ItemID),
				}, fmt.Sprintf("stockmin:%s:%s", idempotencyKey, u.ID))
			}
		}()
	}

	common.WriteJSON(w, http.StatusCreated, map[string]any{
		"balance_after": balanceAfter,
		"below_min":     crossedBelow,
	}, nil)
}

func (h *Handler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID, role := "", ""
	if claims := common.FromCtx(r); claims != nil {
		userID, role = claims.UserID, claims.ActiveRole
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		common.WriteError(w, http.StatusBadRequest, "Header Idempotency-Key wajib diisi")
		return
	}

	var req struct {
		FromWarehouseID string `json:"from_warehouse_id"`
		ToWarehouseID   string `json:"to_warehouse_id"`
		Lines           []struct {
			ItemID string  `json:"item_id"`
			Qty    int64   `json:"qty"`
			Note   *string `json:"note"`
		} `json:"lines"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.FromWarehouseID == "" || req.ToWarehouseID == "" || len(req.Lines) == 0 {
		common.WriteError(w, http.StatusBadRequest, "Gudang asal, tujuan, dan daftar barang wajib diisi")
		return
	}
	if req.FromWarehouseID == req.ToWarehouseID {
		common.WriteError(w, http.StatusBadRequest, "Gudang asal dan tujuan tidak boleh sama")
		return
	}

	tx, err := h.Store.db.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// stock_movements.transfer_id is a UUID column; derive a deterministic
	// UUID from the idempotency key so retries reuse the same transfer id
	// (the old "trf:<key>" string could never be inserted).
	var transferID string
	if err := tx.QueryRowContext(r.Context(),
		`SELECT md5('trf:' || $1::text)::uuid::text`, idempotencyKey).Scan(&transferID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	refNo := fmt.Sprintf("TRF-%d", time.Now().Unix())

	for i, l := range req.Lines {
		if l.Qty <= 0 {
			common.WriteError(w, http.StatusBadRequest, "Kuantitas baris harus > 0")
			return
		}
		// 1. Out from source
		_, _, err := h.Store.Move(r.Context(), tx, MoveInput{
			SchoolID:       schoolID,
			WarehouseID:    req.FromWarehouseID,
			ItemID:         l.ItemID,
			Direction:      "out",
			Qty:            l.Qty,
			Kind:           "transfer_out",
			RefType:        "transfer",
			RefID:          schoolID,
			RefNo:          refNo,
			TransferID:     &transferID,
			Note:           l.Note,
			CreatedBy:      userID,
			ActorRole:      role,
			IdempotencyKey: fmt.Sprintf("%s:out:%d", idempotencyKey, i),
		})
		if err != nil {
			if err == ErrCountingActive {
				common.WriteError(w, http.StatusConflict, "Gudang asal sedang stock opname")
				return
			}
			common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		// 2. In to destination
		_, _, err = h.Store.Move(r.Context(), tx, MoveInput{
			SchoolID:       schoolID,
			WarehouseID:    req.ToWarehouseID,
			ItemID:         l.ItemID,
			Direction:      "in",
			Qty:            l.Qty,
			Kind:           "transfer_in",
			RefType:        "transfer",
			RefID:          schoolID,
			RefNo:          refNo,
			TransferID:     &transferID,
			Note:           l.Note,
			CreatedBy:      userID,
			ActorRole:      role,
			IdempotencyKey: fmt.Sprintf("%s:in:%d", idempotencyKey, i),
		})
		if err != nil {
			if err == ErrCountingActive {
				common.WriteError(w, http.StatusConflict, "Gudang tujuan sedang stock opname")
				return
			}
			common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusCreated, map[string]string{"ref_no": refNo, "status": "completed"}, nil)
}

func (h *Handler) ListMovements(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	warehouseID := r.URL.Query().Get("warehouse_id")
	itemID := r.URL.Query().Get("item_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	list, err := h.Store.ListMovements(r.Context(), schoolID, warehouseID, itemID, limit)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// --- Stock Opname ---

func (h *Handler) ListStockCounts(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	warehouseID := r.URL.Query().Get("warehouse_id")
	list, err := h.Store.ListStockCounts(r.Context(), schoolID, warehouseID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateStockCount(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID := ""
	if claims := common.FromCtx(r); claims != nil {
		userID = claims.UserID
	}
	var req struct {
		WarehouseID string `json:"warehouse_id"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.WarehouseID == "" {
		common.WriteError(w, http.StatusBadRequest, "warehouse_id wajib diisi")
		return
	}

	docNo, err := h.Store.NextDocNumber(r.Context(), schoolID, "SO", time.Now().Year(), "SO-{yyyy}-{seq:4}")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghasilkan nomor dokumen: "+err.Error())
		return
	}

	count, err := h.Store.CreateStockCount(r.Context(), schoolID, req.WarehouseID, userID, docNo)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, count, nil)
}

func (h *Handler) GetStockCount(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	count, err := h.Store.GetStockCount(r.Context(), schoolID, id)
	if err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Stock opname tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, count, nil)
}

func (h *Handler) UpdateCountLines(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req struct {
		Lines []StockCountLine `json:"lines"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if err := h.Store.UpdateCountLines(r.Context(), schoolID, id, req.Lines); err != nil {
		if err == ErrInvalidState {
			common.WriteError(w, http.StatusConflict, "Stock opname sudah tidak dalam status counting/submitted")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (h *Handler) SubmitStockCount(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.SubmitStockCount(r.Context(), schoolID, id); err != nil {
		if err == ErrInvalidState {
			common.WriteError(w, http.StatusConflict, "Stock opname tidak berstatus counting")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "submitted"}, nil)
}

func (h *Handler) RequestAdjustment(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID := ""
	if claims := common.FromCtx(r); claims != nil {
		userID = claims.UserID
	}
	id := chi.URLParam(r, "id")

	count, err := h.Store.GetStockCount(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Stock opname tidak ditemukan")
		return
	}
	if count.Status != "submitted" && count.Status != "counting" {
		common.WriteError(w, http.StatusConflict, "Stock opname belum disubmit")
		return
	}

	var diffItems int
	for _, l := range count.Lines {
		if l.Variance != nil && *l.Variance != 0 {
			diffItems++
		}
	}

	callbackURL := fmt.Sprintf("%s/internal/approval-callbacks", h.CallbackBase)
	appID := ""
	if h.Client != nil {
		idempotencyKey := fmt.Sprintf("stock_adj:%s", count.ID)
		summary := map[string]any{
			"number":         count.Number,
			"warehouse_name": count.WarehouseName,
			"diff_items":     diffItems,
		}
		var err error
		appID, err = h.Client.CreateApproval(r.Context(), client.ApprovalRequest{
			SchoolID:       schoolID,
			RequestType:    "stock_adjustment",
			SourceService:  "ops",
			SourceID:       count.ID,
			Title:          fmt.Sprintf("Penyesuaian Stok Opname %s", count.Number),
			Summary:        summary,
			MakerUserID:    userID,
			CallbackURL:    callbackURL,
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			common.WriteError(w, http.StatusBadGateway, "Gagal mengajukan persetujuan ke Approval Center: "+err.Error())
			return
		}
	}

	if err := h.Store.SetCountApproval(r.Context(), id, appID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{
		"status":      "pending_approval",
		"approval_id": appID,
	}, nil)
}

func (h *Handler) CancelStockCount(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.CancelStockCount(r.Context(), schoolID, id); err != nil {
		if err == ErrInvalidState {
			common.WriteError(w, http.StatusConflict, "Stock opname tidak dapat dibatalkan pada status ini")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"}, nil)
}

func (h *Handler) GetStockCountReportPDF(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	count, err := h.Store.GetStockCount(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Stock opname tidak ditemukan")
		return
	}

	// Simple pure-Go PDF document generator
	pdfData := buildStockOpnamePDF(count)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="opname-%s.pdf"`, count.Number))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfData)
}

func buildStockOpnamePDF(c *StockCount) []byte {
	lines := []pdfdoc.Line{
		{Text: "BERITA ACARA STOCK OPNAME", Size: 16, Bold: true},
		{Text: "Nomor: " + c.Number, Gap: 6},
		{Text: "Gudang: " + c.WarehouseName},
		{Text: "Status: " + c.Status},
		{Text: "Mulai: " + c.StartedAt.Format("02-01-2006 15:04")},
		{Text: "Barang", Bold: true, Gap: 12},
		{Text: "Sistem", Bold: true, Right: 400, Gap: -14.5},
		{Text: "Fisik", Bold: true, Right: 470, Gap: -14.5},
		{Text: "Selisih", Bold: true, Right: 545, Gap: -14.5},
	}
	for _, l := range c.Lines {
		counted, diff := "-", "-"
		if l.CountedQty != nil {
			counted = fmt.Sprintf("%d", *l.CountedQty)
		}
		if l.Variance != nil {
			diff = fmt.Sprintf("%+d", *l.Variance)
		}
		lines = append(lines,
			pdfdoc.Line{Text: fmt.Sprintf("%s - %s (%s)", l.ItemCode, l.ItemName, l.Unit), X: 60},
			pdfdoc.Line{Text: fmt.Sprintf("%d", l.SystemQty), Right: 400, Gap: -14.5},
			pdfdoc.Line{Text: counted, Right: 470, Gap: -14.5},
			pdfdoc.Line{Text: diff, Right: 545, Gap: -14.5},
		)
	}
	lines = append(lines,
		pdfdoc.Line{Text: "Petugas penghitung", Gap: 40},
		pdfdoc.Line{Text: "Mengetahui", X: 350, Gap: -14.5},
		pdfdoc.Line{Text: "(____________________)", Gap: 40},
		pdfdoc.Line{Text: "(____________________)", X: 350, Gap: -14.5},
	)
	return pdfdoc.Build(lines)
}

// ApprovalDecision matches Core callback payload.
type ApprovalDecision struct {
	ApprovalID  string `json:"approval_id"`
	SourceID    string `json:"source_id"`
	RequestType string `json:"request_type"`
	Decision    string `json:"decision"` // 'approved' or 'rejected'
	DecidedBy   string `json:"decided_by"`
	Comment     string `json:"comment"`
}

func (h *Handler) ApplyApprovalDecision(ctx context.Context, d ApprovalDecision) error {
	switch d.RequestType {
	case "stock_adjustment":
		if d.Decision == "approved" {
			var schoolID string
			err := h.Store.db.QueryRowContext(ctx, `SELECT school_id FROM stock_counts WHERE id = $1`, d.SourceID).Scan(&schoolID)
			if err != nil {
				if err == sql.ErrNoRows {
					return ErrNotFound
				}
				return err
			}
			return h.Store.ApplyAdjustment(ctx, schoolID, d.SourceID)
		} else {
			var schoolID string
			err := h.Store.db.QueryRowContext(ctx, `SELECT school_id FROM stock_counts WHERE id = $1`, d.SourceID).Scan(&schoolID)
			if err != nil {
				return err
			}
			return h.Store.RejectStockCount(ctx, schoolID, d.SourceID)
		}
	default:
		return fmt.Errorf("unsupported request type: %s", d.RequestType)
	}
}
