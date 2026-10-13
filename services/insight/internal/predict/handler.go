package predict

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/insight/internal/common"
	"skci/insight/internal/config"
)

type AuditForwarder interface {
	ForwardAudit(ctx context.Context, schoolID, eventType, actorID, entityType, entityID string, diff map[string]interface{}) error
}

type Handler struct {
	cfg    *config.Config
	store  *Store
	client AuditForwarder
}

func NewHandler(cfg *config.Config, store *Store, client AuditForwarder) *Handler {
	return &Handler{
		cfg:    cfg,
		store:  store,
		client: client,
	}
}

// Settings (BR-AI-02)
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	st, err := h.store.GetSettings(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil pengaturan prediksi", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, st)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var req struct {
		PredictiveEnabled bool    `json:"predictive_enabled"`
		ReviewNote        *string `json:"review_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	// BR-AI-02: Enabling predictive requires a review_note (ethics & fairness note)
	if req.PredictiveEnabled {
		if req.ReviewNote == nil || *req.ReviewNote == "" {
			common.WriteError(w, http.StatusBadRequest, "Catatan tinjauan etika/keadilan (review_note) wajib diisi untuk mengaktifkan analitik prediktif")
			return
		}
	}

	var enabledBy *string
	if req.PredictiveEnabled {
		enabledBy = &claims.UserID
	}

	if err := h.store.UpdateSettings(r.Context(), claims.SchoolID, req.PredictiveEnabled, enabledBy, req.ReviewNote); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui pengaturan prediksi", err.Error())
		return
	}

	_ = h.client.ForwardAudit(r.Context(), claims.SchoolID, "update", claims.UserID, "insight_settings", claims.SchoolID, map[string]interface{}{
		"predictive_enabled": req.PredictiveEnabled,
	})

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Pengaturan prediksi berhasil diperbarui"})
}

// Rules CRUD
func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	model := r.URL.Query().Get("model")
	rules, err := h.store.ListRules(r.Context(), claims.SchoolID, model)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil aturan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, rules)
}

func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var rule RiskRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	rule.SchoolID = claims.SchoolID
	rule.CreatedBy = claims.UserID
	if rule.EffectiveFrom == "" {
		rule.EffectiveFrom = time.Now().Format("2006-01-02")
	}

	created, err := h.store.CreateRule(r.Context(), rule)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan aturan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created)
}

// Prediction Endpoints
func (h *Handler) GetArrearsRisk(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	st, _ := h.store.GetSettings(r.Context(), claims.SchoolID)
	if !st.PredictiveEnabled {
		common.WriteError(w, http.StatusConflict, "Analitik prediktif belum diaktifkan")
		return
	}

	level := r.URL.Query().Get("level")
	preds, err := h.store.ListLatestPredictions(r.Context(), claims.SchoolID, "arrears_risk", level)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil data risiko tunggakan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, preds)
}

func (h *Handler) GetAbsenceRisk(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	st, _ := h.store.GetSettings(r.Context(), claims.SchoolID)
	if !st.PredictiveEnabled {
		common.WriteError(w, http.StatusConflict, "Analitik prediktif belum diaktifkan")
		return
	}

	level := r.URL.Query().Get("level")
	preds, err := h.store.ListLatestPredictions(r.Context(), claims.SchoolID, "absence_risk", level)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil data risiko ketidakhadiran", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, preds)
}

func (h *Handler) GetPPDBForecast(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	st, _ := h.store.GetSettings(r.Context(), claims.SchoolID)
	if !st.PredictiveEnabled {
		common.WriteError(w, http.StatusConflict, "Analitik prediktif belum diaktifkan")
		return
	}

	preds, err := h.store.ListLatestPredictions(r.Context(), claims.SchoolID, "ppdb_forecast", "")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil proyeksi PPDB", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, preds)
}

func (h *Handler) GetCashflowForecast(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	st, _ := h.store.GetSettings(r.Context(), claims.SchoolID)
	if !st.PredictiveEnabled {
		common.WriteError(w, http.StatusConflict, "Analitik prediktif belum diaktifkan")
		return
	}

	preds, err := h.store.ListLatestPredictions(r.Context(), claims.SchoolID, "cashflow_forecast", "")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil proyeksi arus kas", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, preds)
}

func (h *Handler) GetAccuracy(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	model := r.URL.Query().Get("model")
	acc, err := h.store.ListAccuracy(r.Context(), claims.SchoolID, model)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil data akurasi", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, acc)
}

func (h *Handler) AddReview(w http.ResponseWriter, r *http.Request) {
	predID := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	var req struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Note == "" {
		common.WriteError(w, http.StatusBadRequest, "Catatan tinjauan wajib diisi")
		return
	}

	if err := h.store.AddReview(r.Context(), claims.SchoolID, predID, claims.UserID, req.Note); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan tinjauan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]string{"message": "Tinjauan berhasil dicatat"})
}
