package budget

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	opsclient "skci/ops/internal/client"
	"skci/ops/internal/common"
	"skci/ops/internal/procurement"
)

var (
	FinanceRoles = []string{
		common.RoleFinanceManager,
		common.RoleFinanceAdmin,
		common.RoleAdmin,
		common.RoleSuperadmin,
		common.RoleKepalaSekolah,
		common.RoleKepalaYayasan,
	}

	BudgetWriters = []string{
		common.RoleFinanceManager,
		common.RoleFinanceAdmin,
		common.RoleAdmin,
		common.RoleSuperadmin,
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

// Satisfies procurement.BudgetCheck contract hook
func (h *Handler) CheckBudget(ctx context.Context, tx *sql.Tx, in procurement.BudgetCheckInput) (procurement.BudgetCheckResult, error) {
	return h.Store.Check(ctx, tx, in)
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	st, err := h.Store.GetSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pengaturan anggaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, st, nil)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req BudgetSettings
	if !common.Decode(w, r, &req) {
		return
	}
	req.SchoolID = schoolID
	if req.FiscalYearStartMonth < 1 || req.FiscalYearStartMonth > 12 {
		req.FiscalYearStartMonth = 7
	}
	if req.LockRule == "" {
		req.LockRule = "warn"
	}
	if err := h.Store.UpdateSettings(r.Context(), &req); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pengaturan anggaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, req, nil)
}

func (h *Handler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")
	list, err := h.Store.ListBudgets(r.Context(), schoolID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar anggaran: "+err.Error())
		return
	}
	if list == nil {
		list = []Budget{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetBudget(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	b, err := h.Store.GetBudget(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anggaran tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, b, nil)
}

func (h *Handler) CreateBudget(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	var req Budget
	if !common.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode dan nama anggaran wajib diisi")
		return
	}
	if strings.TrimSpace(req.FiscalYear) == "" {
		common.WriteError(w, http.StatusBadRequest, "Tahun ajaran/anggaran wajib diisi")
		return
	}
	req.SchoolID = schoolID
	if claims != nil {
		req.CreatedBy = claims.UserID
	}

	created, err := h.Store.CreateBudget(r.Context(), &req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat anggaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateBudget(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req Budget
	if !common.Decode(w, r, &req) {
		return
	}
	req.SchoolID = schoolID
	req.ID = id
	if err := h.Store.UpdateBudget(r.Context(), &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal mengubah anggaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Anggaran berhasil diperbarui"}, nil)
}

func (h *Handler) SubmitBudget(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")

	b, err := h.Store.GetBudget(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anggaran tidak ditemukan")
		return
	}
	if b.Status != "draft" {
		common.WriteError(w, http.StatusBadRequest, "Anggaran tidak dalam status draft")
		return
	}

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
			RequestType:    "budget",
			SourceService:  "ops",
			SourceID:       b.ID,
			Title:          "Persetujuan Anggaran RAPBS (" + b.Code + " - " + b.FiscalYear + ")",
			Summary: map[string]any{
				"budget_id":   b.ID,
				"code":        b.Code,
				"name":        b.Name,
				"fiscal_year": b.FiscalYear,
				"total_idr":   b.TotalPlannedIDR,
			},
			MakerUserID:    requesterID,
			CallbackURL:    callbackURL,
			IdempotencyKey: "budget:submit:" + b.ID,
		})
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal mengajukan persetujuan anggaran: "+err.Error())
			return
		}
	}

	if err := h.Store.SubmitBudget(r.Context(), schoolID, id, approvalID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui status pengajuan: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{
		"message":     "Anggaran berhasil diajukan untuk persetujuan",
		"approval_id": approvalID,
	}, nil)
}

func (h *Handler) ListBudgetLines(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	budgetID := chi.URLParam(r, "id")
	lines, err := h.Store.ListBudgetLines(r.Context(), schoolID, budgetID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pos anggaran: "+err.Error())
		return
	}
	if lines == nil {
		lines = []BudgetLine{}
	}
	common.WriteJSON(w, http.StatusOK, lines, nil)
}

func (h *Handler) CreateBudgetLine(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	budgetID := chi.URLParam(r, "id")
	var req BudgetLine
	if !common.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode dan nama pos wajib diisi")
		return
	}
	if strings.TrimSpace(req.AccCode) == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode akun akuntansi wajib diisi")
		return
	}
	if req.Category == "" {
		req.Category = "operasional"
	}
	req.SchoolID = schoolID
	req.BudgetID = budgetID

	created, err := h.Store.CreateBudgetLine(r.Context(), &req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat pos anggaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateBudgetLine(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	budgetID := chi.URLParam(r, "id")
	lineID := chi.URLParam(r, "lineId")
	var req BudgetLine
	if !common.Decode(w, r, &req) {
		return
	}
	req.SchoolID = schoolID
	req.BudgetID = budgetID
	req.ID = lineID
	if err := h.Store.UpdateBudgetLine(r.Context(), &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal mengubah pos anggaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Pos anggaran berhasil diperbarui"}, nil)
}

func (h *Handler) DeleteBudgetLine(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	budgetID := chi.URLParam(r, "id")
	lineID := chi.URLParam(r, "lineId")
	if err := h.Store.DeleteBudgetLine(r.Context(), schoolID, budgetID, lineID); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal menghapus pos anggaran: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Pos anggaran berhasil dihapus"}, nil)
}

func (h *Handler) SyncActuals(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	budgetID := chi.URLParam(r, "id")
	count, err := h.Store.SyncActualsFromAccounting(r.Context(), schoolID, budgetID)
	if err != nil {
		if strings.Contains(err.Error(), "409") {
			common.WriteError(w, http.StatusConflict, "Modul akuntansi tidak aktif")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal sinkronisasi realisasi: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"message": "Realisasi berhasil disinkronkan dari buku besar",
		"synced":  count,
	}, nil)
}

func (h *Handler) GetBudgetReport(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	budgetID := chi.URLParam(r, "id")
	rep, err := h.Store.GetBudgetReport(r.Context(), schoolID, budgetID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anggaran tidak ditemukan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, rep, nil)
}

func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	unack := r.URL.Query().Get("unacknowledged") == "true"
	list, err := h.Store.ListAlerts(r.Context(), schoolID, unack)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat alert anggaran: "+err.Error())
		return
	}
	if list == nil {
		list = []BudgetAlert{}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.FromCtx(r)
	alertID := chi.URLParam(r, "id")
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}
	if err := h.Store.AcknowledgeAlert(r.Context(), schoolID, alertID, userID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menandai alert: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Alert berhasil ditandai"}, nil)
}
