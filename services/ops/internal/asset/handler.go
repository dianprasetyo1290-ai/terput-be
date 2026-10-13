package asset

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	opsclient "skci/ops/internal/client"
	"skci/ops/internal/common"
	"skci/ops/internal/procurement"
	"skci/ops/internal/storage"
)

var (
	AssetWriters = []string{
		common.RoleAssetOfficer,
		common.RoleAdmin,
		common.RoleSuperadmin,
	}

	AssetReaders = []string{
		common.RoleAssetOfficer,
		common.RoleKepalaSekolah,
		common.RoleFinanceManager,
		common.RoleFinanceAdmin,
		common.RoleAdmin,
		common.RoleSuperadmin,
		common.RoleKepalaYayasan,
		common.RoleFacilityOfficer, // Allowed for 3D Campus overlay (T25)
	}

	FinanceRoles = []string{
		common.RoleAssetOfficer,
		common.RoleFinanceManager,
		common.RoleFinanceAdmin,
		common.RoleAdmin,
		common.RoleSuperadmin,
	}
)

type Handler struct {
	Store      *Store
	Client     *opsclient.Client
	Accounting AccountingClient
	Storage    storage.Storage
	PublicURL  string
}

func NewHandler(store *Store, client *opsclient.Client, ac AccountingClient, st storage.Storage, publicURL string) *Handler {
	return &Handler{
		Store:      store,
		Client:     client,
		Accounting: ac,
		Storage:    st,
		PublicURL:  strings.TrimRight(publicURL, "/"),
	}
}

func (h *Handler) OnGoodsReceived(ctx context.Context, tx *sql.Tx, ev procurement.GoodsReceivedEvent) error {
	return h.Store.OnGoodsReceived(ctx, tx, ev)
}

// -------------------------------------------------------------------------
// Categories
// -------------------------------------------------------------------------

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListCategories(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kategori aset: "+err.Error())
		return
	}
	if list == nil {
		list = []Category{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	cat, err := h.Store.GetCategory(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kategori aset: "+err.Error())
		return
	}
	if cat == nil {
		common.WriteError(w, http.StatusNotFound, "Kategori aset tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, cat, nil)
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req Category
	if !common.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode dan nama kategori wajib diisi")
		return
	}
	req.SchoolID = schoolID
	created, err := h.Store.CreateCategory(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat kategori aset: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req Category
	if !common.Decode(w, r, &req) {
		return
	}
	req.SchoolID = schoolID
	req.ID = id
	updated, err := h.Store.UpdateCategory(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui kategori aset: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteCategory(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Kategori aset berhasil dihapus"}, nil)
}

// -------------------------------------------------------------------------
// Settings
// -------------------------------------------------------------------------

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	st, err := h.Store.GetSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pengaturan aset: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, st, nil)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req Settings
	if !common.Decode(w, r, &req) {
		return
	}
	req.SchoolID = schoolID
	updated, err := h.Store.UpdateSettings(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pengaturan aset: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// -------------------------------------------------------------------------
// Assets
// -------------------------------------------------------------------------

func (h *Handler) ListAssets(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))

	var roomIDs []string
	if rawRoomIDs := q.Get("room_ids"); rawRoomIDs != "" {
		for _, part := range strings.Split(rawRoomIDs, ",") {
			if part = strings.TrimSpace(part); part != "" {
				roomIDs = append(roomIDs, part)
			}
		}
	}

	filter := AssetFilter{
		SchoolID:        schoolID,
		Status:          q.Get("status"),
		CategoryID:      q.Get("category_id"),
		RoomID:          q.Get("room_id"),
		RoomIDs:         roomIDs,
		CustodianUserID: q.Get("custodian_user_id"),
		Query:           q.Get("q"),
		Page:            page,
		PerPage:         perPage,
	}

	claims := common.FromCtx(r)
	callerRole := ""
	if claims != nil {
		callerRole = claims.ActiveRole
	}

	// Cross-school yayasan reader
	if callerRole == common.RoleKepalaYayasan && q.Get("school_id") != "" {
		filter.SchoolID = q.Get("school_id")
	}

	list, total, err := h.Store.ListAssets(r.Context(), filter)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar aset: "+err.Error())
		return
	}
	if list == nil {
		list = []Asset{}
	}

	for i := range list {
		if list[i].PhotoObjectKey != nil && *list[i].PhotoObjectKey != "" {
			url := fmt.Sprintf("/api/v1/assets/%s/photo", list[i].ID)
			list[i].PhotoURL = &url
		}
	}

	meta := &common.Meta{
		Page:    filter.Page,
		PerPage: filter.PerPage,
		Total:   total,
	}
	common.WriteJSON(w, http.StatusOK, list, meta)
}

func (h *Handler) GetAsset(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	claims := common.FromCtx(r)
	callerRole := ""
	if claims != nil {
		callerRole = claims.ActiveRole
	}

	asset, err := h.Store.GetAsset(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat detail aset: "+err.Error())
		return
	}
	if asset == nil {
		common.WriteError(w, http.StatusNotFound, "Aset tidak ditemukan")
		return
	}

	if asset.PhotoObjectKey != nil && *asset.PhotoObjectKey != "" {
		url := fmt.Sprintf("/api/v1/assets/%s/photo", asset.ID)
		asset.PhotoURL = &url
	}

	// Filter book value / financial information if not authorized
	canViewFinancials := common.HasActiveRole(claims, FinanceRoles...) || callerRole == common.RoleKepalaSekolah
	if !canViewFinancials {
		asset.BookValueIDR = nil
	}

	common.WriteJSON(w, http.StatusOK, asset, nil)
}

func (h *Handler) SummaryByRoom(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	raw := r.URL.Query().Get("room_ids")
	if raw == "" {
		common.WriteJSON(w, http.StatusOK, []RoomSummary{}, nil)
		return
	}

	var roomIDs []string
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			roomIDs = append(roomIDs, id)
		}
	}

	if len(roomIDs) > 500 {
		roomIDs = roomIDs[:500]
	}

	summaries, err := h.Store.RoomSummary(r.Context(), schoolID, roomIDs)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat ringkasan aset per ruang: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, summaries, nil)
}

func (h *Handler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req Asset
	if !common.Decode(w, r, &req) {
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama aset wajib diisi")
		return
	}

	req.SchoolID = schoolID
	created, err := h.Store.CreateManualAsset(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat aset kandidat: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateAsset(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req Asset
	if !common.Decode(w, r, &req) {
		return
	}

	req.SchoolID = schoolID
	req.ID = id
	if err := h.Store.UpdateAssetPhysical(r.Context(), req); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui data fisik aset: "+err.Error())
		return
	}

	updated, _ := h.Store.GetAsset(r.Context(), schoolID, id)
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

func (h *Handler) ActivateAsset(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	claims := common.FromCtx(r)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	activated, err := h.Store.ActivateAsset(r.Context(), schoolID, id, userID)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Trigger async event dispatch
	go func() {
		_ = h.Store.DispatchPendingEvents(context.Background(), h.Accounting, h.Client)
	}()

	common.WriteJSON(w, http.StatusOK, activated, nil)
}

func (h *Handler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := r.ParseMultipartForm(5 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format file tidak valid atau ukuran melebihi batas 5MB")
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "File foto wajib diunggah")
		return
	}
	defer file.Close()

	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "image/jpeg"
	}

	key := fmt.Sprintf("schools/%s/assets/%s/photo_%d", schoolID, id, time.Now().Unix())
	obj, err := h.Storage.Put(r.Context(), key, mime, file, header.Size)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal menyimpan foto: "+err.Error())
		return
	}

	if err := h.Store.UpdatePhoto(r.Context(), schoolID, id, obj.Key); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui tautan foto aset: "+err.Error())
		return
	}

	photoURL := fmt.Sprintf("/api/v1/assets/%s/photo", id)
	common.WriteJSON(w, http.StatusOK, map[string]string{"photo_url": photoURL, "key": obj.Key}, nil)
}

func (h *Handler) GetPhoto(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	ast, err := h.Store.GetAsset(r.Context(), schoolID, id)
	if err != nil || ast == nil || ast.PhotoObjectKey == nil || *ast.PhotoObjectKey == "" {
		common.WriteError(w, http.StatusNotFound, "Foto aset tidak ditemukan")
		return
	}

	rc, err := h.Storage.Open(r.Context(), *ast.PhotoObjectKey)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Berkas foto tidak ditemukan di penyimpanan")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, rc)
}

// -------------------------------------------------------------------------
// QR Label & Scan
// -------------------------------------------------------------------------

func (h *Handler) GetLabel(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	ast, err := h.Store.GetAsset(r.Context(), schoolID, id)
	if err != nil || ast == nil {
		common.WriteError(w, http.StatusNotFound, "Aset tidak ditemukan")
		return
	}

	label := map[string]any{
		"id":       ast.ID,
		"code":     ast.Code,
		"name":     ast.Name,
		"qr_token": ast.QRToken,
		"scan_url": fmt.Sprintf("/aset/scan/%s", ast.QRToken),
	}
	common.WriteJSON(w, http.StatusOK, label, nil)
}

func (h *Handler) GetLabels(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	rawIDs := r.URL.Query().Get("ids")
	if rawIDs == "" {
		common.WriteJSON(w, http.StatusOK, []any{}, nil)
		return
	}

	var labels []map[string]any
	for _, id := range strings.Split(rawIDs, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ast, err := h.Store.GetAsset(r.Context(), schoolID, id)
			if err == nil && ast != nil {
				labels = append(labels, map[string]any{
					"id":       ast.ID,
					"code":     ast.Code,
					"name":     ast.Name,
					"qr_token": ast.QRToken,
					"scan_url": fmt.Sprintf("/aset/scan/%s", ast.QRToken),
				})
			}
		}
	}

	common.WriteJSON(w, http.StatusOK, labels, nil)
}

func (h *Handler) ScanAsset(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "qr_token")
	claims := common.FromCtx(r)
	callerRole := ""
	schoolID := ""
	if claims != nil {
		callerRole = claims.ActiveRole
		schoolID = claims.SchoolID
	}

	ast, err := h.Store.GetAssetByToken(r.Context(), token)
	if err != nil || ast == nil {
		common.WriteError(w, http.StatusNotFound, "Aset tidak ditemukan untuk token QR ini")
		return
	}

	// Verify same school unless superadmin / cross-school yayasan
	if callerRole != common.RoleSuperadmin && callerRole != common.RoleKepalaYayasan && ast.SchoolID != schoolID {
		common.WriteError(w, http.StatusNotFound, "Aset tidak ditemukan di unit sekolah Anda")
		return
	}

	if ast.PhotoObjectKey != nil && *ast.PhotoObjectKey != "" {
		url := fmt.Sprintf("/api/v1/assets/%s/photo", ast.ID)
		ast.PhotoURL = &url
	}

	var bv *BookValue
	if common.HasActiveRole(claims, FinanceRoles...) || callerRole == common.RoleKepalaSekolah {
		bv, _ = h.Store.GetAssetBookValue(r.Context(), ast.SchoolID, ast.ID)
	}

	res := ScanView(*ast, bv, callerRole)
	common.WriteJSON(w, http.StatusOK, res, nil)
}

// -------------------------------------------------------------------------
// Movements & Disposals
// -------------------------------------------------------------------------

func (h *Handler) CreateMovement(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	claims := common.FromCtx(r)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	var req Movement
	if !common.Decode(w, r, &req) {
		return
	}

	current, err := h.Store.GetAsset(r.Context(), schoolID, id)
	if err != nil || current == nil {
		common.WriteError(w, http.StatusNotFound, "Aset tidak ditemukan")
		return
	}

	req.SchoolID = schoolID
	req.AssetID = id
	req.CreatedBy = userID

	m, err := h.Store.CreateMovement(r.Context(), req, *current, h.Client)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusCreated, m, nil)
}

func (h *Handler) ListMovements(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	list, err := h.Store.ListMovements(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat riwayat mutasi: "+err.Error())
		return
	}
	if list == nil {
		list = []Movement{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateDisposal(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	claims := common.FromCtx(r)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	var req Disposal
	if !common.Decode(w, r, &req) {
		return
	}

	if strings.TrimSpace(req.Reason) == "" {
		common.WriteError(w, http.StatusBadRequest, "Alasan pelepasan aset wajib diisi")
		return
	}

	current, err := h.Store.GetAsset(r.Context(), schoolID, id)
	if err != nil || current == nil {
		common.WriteError(w, http.StatusNotFound, "Aset tidak ditemukan")
		return
	}

	req.SchoolID = schoolID
	req.AssetID = id
	req.CreatedBy = userID

	d, err := h.Store.CreateDisposal(r.Context(), req, *current, h.Client)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusCreated, d, nil)
}

func (h *Handler) CancelDisposal(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	claims := common.FromCtx(r)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	if err := h.Store.CancelDisposal(r.Context(), schoolID, id, userID, h.Client); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Pengajuan pelepasan aset berhasil dibatalkan"}, nil)
}

func (h *Handler) ListDisposals(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")

	list, err := h.Store.ListDisposals(r.Context(), schoolID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar pelepasan aset: "+err.Error())
		return
	}
	if list == nil {
		list = []Disposal{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// -------------------------------------------------------------------------
// Depreciation & Internal Retry
// -------------------------------------------------------------------------

func (h *Handler) GetDepreciation(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	bv, err := h.Store.GetAssetBookValue(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat data penyusutan: "+err.Error())
		return
	}

	res := map[string]any{
		"asset_id":   id,
		"book_value": bv,
	}

	// Try fetching fresh schedule from accounting engine if available
	if h.Accounting != nil {
		dtos, err := h.Accounting.GetBookValues(r.Context(), schoolID, []string{id})
		if err == nil && len(dtos) > 0 {
			res["schedule"] = dtos[0].Schedule
			res["accounting_sync"] = dtos[0]
		}
	}

	common.WriteJSON(w, http.StatusOK, res, nil)
}

func (h *Handler) InternalRetryEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Store.RetryEvent(r.Context(), id); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	go func() {
		_ = h.Store.DispatchPendingEvents(context.Background(), h.Accounting, h.Client)
	}()

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Event berhasil dijadwalkan ulang untuk pengiriman"}, nil)
}

func (h *Handler) ApplyApprovalDecision(ctx context.Context, requestType, sourceID, decision string) error {
	return h.Store.HandleApprovalCallback(ctx, requestType, sourceID, decision)
}
