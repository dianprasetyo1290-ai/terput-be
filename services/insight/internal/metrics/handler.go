package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"skci/insight/internal/client"
	"skci/insight/internal/common"
	"skci/insight/internal/config"
)

type Handler struct {
	cfg    *config.Config
	store  *Store
	client *client.Client
}

func NewHandler(cfg *config.Config, store *Store, client *client.Client) *Handler {
	return &Handler{
		cfg:    cfg,
		store:  store,
		client: client,
	}
}

func (h *Handler) ListMetrics(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "active"
	}
	defs, err := h.store.ListDefinitions(r.Context(), status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil kamus metrik", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, defs)
}

func (h *Handler) GetMetric(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	def, err := h.store.GetDefinition(r.Context(), key)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Metrik tidak ditemukan")
		return
	}

	claims := common.FromContext(r.Context())
	var params map[string]interface{}
	if claims != nil && claims.SchoolID != "" {
		params, _ = h.store.GetSchoolParams(r.Context(), claims.SchoolID, key)
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"definition":    def,
		"school_params": params,
	})
}

func (h *Handler) UpdateDescription(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var req struct {
		Description string  `json:"description"`
		Limitations *string `json:"limitations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if err := h.store.UpdateDescription(r.Context(), key, req.Description, req.Limitations); err != nil {
		common.WriteError(w, http.StatusNotFound, "Metrik tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Deskripsi metrik berhasil diperbarui"})
}

func (h *Handler) UpdateSchoolParams(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	claims := common.FromContext(r.Context())
	if claims == nil || claims.SchoolID == "" {
		common.WriteError(w, http.StatusUnauthorized, "School ID required")
		return
	}

	var req struct {
		Params        map[string]interface{} `json:"params"`
		EffectiveFrom string                 `json:"effective_from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	if req.EffectiveFrom == "" {
		req.EffectiveFrom = time.Now().Format("2006-01-02")
	}

	if err := h.store.UpsertSchoolParams(r.Context(), claims.SchoolID, key, req.Params, req.EffectiveFrom); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui parameter metrik sekolah", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Parameter metrik sekolah berhasil diperbarui"})
}

// Internal Contract (§4.1 for T23 and other services)
func (h *Handler) InternalListMetrics(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	defs, err := h.store.ListDefinitions(r.Context(), status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Failed to list internal metrics", err.Error())
		return
	}

	type metricItem struct {
		Key            string     `json:"key"`
		Name           string     `json:"name"`
		Unit           MetricUnit `json:"unit"`
		AllowedGroupBy []string   `json:"allowed_group_by"`
		OwnerService   string     `json:"owner_service"`
		Formula        string     `json:"formula"`
		Limitations    *string    `json:"limitations"`
	}

	items := make([]metricItem, len(defs))
	for i, d := range defs {
		items[i] = metricItem{
			Key:            d.Key,
			Name:           d.Name,
			Unit:           d.Unit,
			AllowedGroupBy: d.AllowedGroupBy,
			OwnerService:   d.OwnerService,
			Formula:        d.Formula,
			Limitations:    d.Limitations,
		}
	}
	common.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) InternalGetMetricValue(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	schoolID := r.URL.Query().Get("school_id")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	groupBy := r.URL.Query().Get("group_by")

	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id query param is required")
		return
	}

	def, err := h.store.GetDefinition(r.Context(), key)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Metrik tidak dikenal")
		return
	}

	if groupBy != "" && !slices.Contains(def.AllowedGroupBy, groupBy) {
		common.WriteError(w, http.StatusUnprocessableEntity, "group_by tidak diizinkan untuk metrik ini")
		return
	}

	periodStart := from
	if periodStart == "" {
		periodStart = time.Now().Format("2006-01") + "-01"
	}

	snap, _ := h.store.GetSnapshot(r.Context(), schoolID, key, "month", periodStart, groupBy, "")
	now := time.Now()

	// If missing or stale, attempt refresh
	if IsStale(snap, *def, now, h.cfg.MetricRefreshDefaultMin) {
		refreshedSnap, err := h.refreshSnapshot(r.Context(), *def, schoolID, from, to, groupBy, periodStart)
		if err == nil {
			snap = refreshedSnap
		}
	}

	if snap == nil {
		// 503: source failed and no snapshot exists yet. AC-146-01: never return false 0!
		common.WriteError(w, http.StatusServiceUnavailable, "Sumber data gagal dan belum ada snapshot metrik")
		return
	}

	resp := map[string]interface{}{
		"key":         snap.MetricKey,
		"unit":        def.Unit,
		"value":       snap.Value,
		"numerator":   snap.Numerator,
		"denominator": snap.Denominator,
		"as_of":       snap.SourceAsOf,
		"computed_at": snap.ComputedAt,
	}

	common.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) refreshSnapshot(ctx context.Context, def MetricDef, schoolID, from, to, groupBy, periodStart string) (*MetricSnapshot, error) {
	started := time.Now()
	facts, err := h.client.FetchMetricFacts(ctx, def.OwnerService, def.SourceMetric, schoolID, from, to, groupBy)
	finished := time.Now()

	if err != nil {
		errStr := err.Error()
		_ = h.store.RecordRefreshRun(ctx, schoolID, def.Key, periodStart, "failed", &errStr, started, finished)
		return nil, err
	}

	snap := MetricSnapshot{
		SchoolID:    schoolID,
		MetricKey:   def.Key,
		PeriodKind:  "month",
		PeriodStart: periodStart,
		PeriodEnd:   to,
		GroupBy:     groupBy,
		DimValue:    "",
		Numerator:   facts.Numerator,
		Denominator: facts.Denominator,
		Value:       facts.Value,
		SourceAsOf:  facts.AsOf,
		ComputedAt:  time.Now(),
	}
	if snap.PeriodEnd == "" {
		snap.PeriodEnd = periodStart
	}

	if err := h.store.UpsertSnapshot(ctx, snap); err != nil {
		return nil, err
	}

	// Also upsert rows if grouped
	for _, row := range facts.Rows {
		rowSnap := snap
		rowSnap.DimValue = row.DimValue
		rowSnap.Numerator = row.Numerator
		rowSnap.Denominator = row.Denominator
		rowSnap.Value = row.Value
		_ = h.store.UpsertSnapshot(ctx, rowSnap)
	}

	_ = h.store.RecordRefreshRun(ctx, schoolID, def.Key, periodStart, "ok", nil, started, finished)
	return &snap, nil
}
