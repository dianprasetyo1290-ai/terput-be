package bi

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/insight/internal/client"
	"skci/insight/internal/common"
	"skci/insight/internal/config"
	"skci/insight/internal/metrics"
)

type Handler struct {
	cfg          *config.Config
	store        *Store
	metricStore  *metrics.Store
	client       *client.Client
}

func NewHandler(cfg *config.Config, store *Store, metricStore *metrics.Store, client *client.Client) *Handler {
	return &Handler{
		cfg:         cfg,
		store:       store,
		metricStore: metricStore,
		client:      client,
	}
}

type QueryRequest struct {
	MetricKey string             `json:"metric_key"`
	Period    metrics.PeriodSpec `json:"period"`
	GroupBy   string             `json:"group_by"`
	SchoolIDs []string           `json:"school_ids"`
}

func (h *Handler) Query(w http.ResponseWriter, r *http.Request) {
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid query payload")
		return
	}

	claims := common.FromContext(r.Context())
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// 1. Resolve Scope
	scopeClaims := ScopeClaims{
		UserID:     claims.UserID,
		SchoolID:   claims.SchoolID,
		ActiveRole: claims.ActiveRole,
	}
	grants, _ := h.store.ListScopeGrants(r.Context(), claims.UserID)
	foundationSchoolsRaw, _ := h.client.FetchFoundationSchools(r.Context(), claims.SchoolID)
	foundationSchools := make([]string, len(foundationSchoolsRaw))
	for i, f := range foundationSchoolsRaw {
		foundationSchools[i] = f.ID
	}

	resolvedSchools, err := ResolveScope(scopeClaims, grants, foundationSchools, req.SchoolIDs)
	if err != nil {
		common.WriteError(w, http.StatusForbidden, "Akses di luar cakupan sekolah yang diizinkan")
		return
	}

	// 2. Metric definition
	def, err := h.metricStore.GetDefinition(r.Context(), req.MetricKey)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Metrik tidak ditemukan")
		return
	}

	// 3. Resolve Period
	now := time.Now()
	pStart, pEnd, err := metrics.ResolvePeriod(req.Period, now, nil)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Periode tidak valid: "+err.Error())
		return
	}
	startStr := pStart.Format("2006-01-02")
	endStr := pEnd.Format("2006-01-02")

	// 4. Gather snapshots across resolved schools
	var aggregateRows []metrics.AggregateRow
	latestAsOf := pStart

	for _, sID := range resolvedSchools {
		snap, _ := h.metricStore.GetSnapshot(r.Context(), sID, req.MetricKey, "month", startStr, req.GroupBy, "")
		if snap == nil || metrics.IsStale(snap, *def, now, h.cfg.MetricRefreshDefaultMin) {
			// On-demand fetch facts
			facts, errF := h.client.FetchMetricFacts(r.Context(), def.OwnerService, def.SourceMetric, sID, startStr, endStr, req.GroupBy)
			if errF == nil && facts != nil {
				newSnap := metrics.MetricSnapshot{
					SchoolID:    sID,
					MetricKey:   def.Key,
					PeriodKind:  "month",
					PeriodStart: startStr,
					PeriodEnd:   endStr,
					GroupBy:     req.GroupBy,
					DimValue:    "",
					Numerator:   facts.Numerator,
					Denominator: facts.Denominator,
					Value:       facts.Value,
					SourceAsOf:  facts.AsOf,
					ComputedAt:  now,
				}
				_ = h.metricStore.UpsertSnapshot(r.Context(), newSnap)
				snap = &newSnap
			}
		}

		if snap != nil {
			var num, den, val int64
			if snap.Numerator != nil {
				num = *snap.Numerator
			}
			if snap.Denominator != nil {
				den = *snap.Denominator
			}
			if snap.Value != nil {
				val = *snap.Value
			}
			aggregateRows = append(aggregateRows, metrics.AggregateRow{
				SchoolID:    sID,
				DimValue:    snap.DimValue,
				Numerator:   num,
				Denominator: den,
				Value:       val,
			})
			if snap.SourceAsOf.After(latestAsOf) {
				latestAsOf = snap.SourceAsOf
			}
		}
	}

	perUnit, consolidated := metrics.Aggregate(*def, aggregateRows)

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"metric_key":   def.Key,
		"unit":         def.Unit,
		"per_unit":     perUnit,
		"consolidated": consolidated,
		"as_of":        latestAsOf,
		"computed_at":  now,
	})
}

// Dashboards CRUD
func (h *Handler) ListDashboards(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	dashboards, err := h.store.ListDashboards(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar dashboard", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, dashboards)
}

func (h *Handler) CreateDashboard(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var d Dashboard
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	d.SchoolID = claims.SchoolID
	d.OwnerUserID = claims.UserID

	created, err := h.store.CreateDashboard(r.Context(), d)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat dashboard", err.Error())
		return
	}

	if len(d.Widgets) > 0 {
		for i := range d.Widgets {
			d.Widgets[i].SchoolID = claims.SchoolID
		}
		_ = h.store.ReplaceWidgets(r.Context(), created.ID, d.Widgets)
		created.Widgets = d.Widgets
	}

	_ = h.client.ForwardAudit(r.Context(), claims.SchoolID, "create", claims.UserID, "dashboard", created.ID, map[string]interface{}{"title": created.Title})
	common.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	d, err := h.store.GetDashboard(r.Context(), id, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Dashboard tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, d)
}

func (h *Handler) UpdateDashboard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	var d Dashboard
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	d.ID = id
	d.SchoolID = claims.SchoolID

	if err := h.store.UpdateDashboard(r.Context(), d); err != nil {
		common.WriteError(w, http.StatusNotFound, "Dashboard tidak ditemukan")
		return
	}
	if len(d.Widgets) > 0 {
		for i := range d.Widgets {
			d.Widgets[i].SchoolID = claims.SchoolID
		}
		_ = h.store.ReplaceWidgets(r.Context(), id, d.Widgets)
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Dashboard berhasil diperbarui"})
}

func (h *Handler) DeleteDashboard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	if err := h.store.DeleteDashboard(r.Context(), id, claims.SchoolID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus dashboard", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Dashboard berhasil dihapus"})
}

// AC-152-01: Foundation dashboard opened by kepala_sekolah SD -> only shows SD data!
func (h *Handler) GetDashboardData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	d, err := h.store.GetDashboard(r.Context(), id, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Dashboard tidak ditemukan")
		return
	}

	scopeClaims := ScopeClaims{
		UserID:     claims.UserID,
		SchoolID:   claims.SchoolID,
		ActiveRole: claims.ActiveRole,
	}
	grants, _ := h.store.ListScopeGrants(r.Context(), claims.UserID)
	foundationSchoolsRaw, _ := h.client.FetchFoundationSchools(r.Context(), claims.SchoolID)
	foundationSchools := make([]string, len(foundationSchoolsRaw))
	for i, f := range foundationSchoolsRaw {
		foundationSchools[i] = f.ID
	}

	// Resolve scope according to viewer's rights (clipping foundation scope to own unit if not authorized)
	var requested []string
	if d.ScopeKind == "foundation" {
		requested = foundationSchools
	} else {
		requested = []string{claims.SchoolID}
	}
	resolvedSchools, _ := ResolveScope(scopeClaims, grants, foundationSchools, requested)

	now := time.Now()
	widgetResults := make([]map[string]interface{}, 0, len(d.Widgets))

	for _, w := range d.Widgets {
		def, errDef := h.metricStore.GetDefinition(r.Context(), w.MetricKey)
		if errDef != nil {
			continue
		}

		pStart, pEnd, _ := metrics.ResolvePeriod(w.Period, now, nil)
		startStr := pStart.Format("2006-01-02")
		endStr := pEnd.Format("2006-01-02")
		groupBy := ""
		if w.GroupBy != nil {
			groupBy = *w.GroupBy
		}

		var aggRows []metrics.AggregateRow
		latestAsOf := pStart

		for _, sID := range resolvedSchools {
			snap, _ := h.metricStore.GetSnapshot(r.Context(), sID, w.MetricKey, "month", startStr, groupBy, "")
			if snap == nil || metrics.IsStale(snap, *def, now, h.cfg.MetricRefreshDefaultMin) {
				facts, errF := h.client.FetchMetricFacts(r.Context(), def.OwnerService, def.SourceMetric, sID, startStr, endStr, groupBy)
				if errF == nil && facts != nil {
					newSnap := metrics.MetricSnapshot{
						SchoolID:    sID,
						MetricKey:   def.Key,
						PeriodKind:  "month",
						PeriodStart: startStr,
						PeriodEnd:   endStr,
						GroupBy:     groupBy,
						DimValue:    "",
						Numerator:   facts.Numerator,
						Denominator: facts.Denominator,
						Value:       facts.Value,
						SourceAsOf:  facts.AsOf,
						ComputedAt:  now,
					}
					_ = h.metricStore.UpsertSnapshot(r.Context(), newSnap)
					snap = &newSnap
				}
			}

			if snap != nil {
				var num, den, val int64
				if snap.Numerator != nil {
					num = *snap.Numerator
				}
				if snap.Denominator != nil {
					den = *snap.Denominator
				}
				if snap.Value != nil {
					val = *snap.Value
				}
				aggRows = append(aggRows, metrics.AggregateRow{
					SchoolID:    sID,
					DimValue:    snap.DimValue,
					Numerator:   num,
					Denominator: den,
					Value:       val,
				})
				if snap.SourceAsOf.After(latestAsOf) {
					latestAsOf = snap.SourceAsOf
				}
			}
		}

		perUnit, consolidated := metrics.Aggregate(*def, aggRows)
		widgetResults = append(widgetResults, map[string]interface{}{
			"widget_id":    w.ID,
			"title":        w.Title,
			"chart":        w.Chart,
			"metric_key":   def.Key,
			"unit":         def.Unit,
			"per_unit":     perUnit,
			"consolidated": consolidated,
			"as_of":        latestAsOf,
		})
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"dashboard": d,
		"widgets":   widgetResults,
		"schools":   resolvedSchools,
	})
}

// Saved Reports CRUD & Export
func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	reports, err := h.store.ListReports(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil laporan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, reports)
}

func (h *Handler) CreateReport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var rep SavedReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	rep.SchoolID = claims.SchoolID
	rep.OwnerUserID = claims.UserID

	created, err := h.store.CreateReport(r.Context(), rep)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat laporan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	rep, err := h.store.GetReport(r.Context(), id, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Laporan tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, rep)
}

func (h *Handler) UpdateReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	var rep SavedReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	rep.ID = id
	rep.SchoolID = claims.SchoolID
	if err := h.store.UpdateReport(r.Context(), rep); err != nil {
		common.WriteError(w, http.StatusNotFound, "Laporan tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Laporan berhasil diperbarui"})
}

func (h *Handler) DeleteReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	if err := h.store.DeleteReport(r.Context(), id, claims.SchoolID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus laporan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Laporan berhasil dihapus"})
}

func (h *Handler) RunReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	rep, err := h.store.GetReport(r.Context(), id, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Laporan tidak ditemukan")
		return
	}

	// Generate aggregate tabular rows for the report columns
	rows := []map[string]interface{}{}
	now := time.Now()
	for _, col := range rep.Columns {
		def, errD := h.metricStore.GetDefinition(r.Context(), col)
		if errD != nil {
			continue
		}
		snap, _ := h.metricStore.GetSnapshot(r.Context(), claims.SchoolID, col, "month", now.Format("2006-01")+"-01", "", "")
		var val int64
		if snap != nil && snap.Value != nil {
			val = *snap.Value
		}
		rows = append(rows, map[string]interface{}{
			"metric_key": def.Key,
			"name":       def.Name,
			"unit":       def.Unit,
			"value":      val,
		})
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"report": rep,
		"rows":   rows,
		"as_of":  now,
	})
}

func (h *Handler) ExportReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	claims := common.FromContext(r.Context())
	rep, err := h.store.GetReport(r.Context(), id, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Laporan tidak ditemukan")
		return
	}

	now := time.Now()
	var reportData [][]string
	reportData = append(reportData, []string{"Metrik", "Nama", "Satuan", "Nilai"})

	for _, col := range rep.Columns {
		def, errD := h.metricStore.GetDefinition(r.Context(), col)
		if errD != nil {
			continue
		}
		snap, _ := h.metricStore.GetSnapshot(r.Context(), claims.SchoolID, col, "month", now.Format("2006-01")+"-01", "", "")
		valStr := "0"
		if snap != nil && snap.Value != nil {
			valStr = fmt.Sprintf("%d", *snap.Value)
		}
		reportData = append(reportData, []string{def.Key, def.Name, string(def.Unit), valStr})
	}

	_ = h.client.ForwardAudit(r.Context(), claims.SchoolID, "export", claims.UserID, "report", rep.ID, map[string]interface{}{"format": format})

	if format == "xlsx" {
		f := excelize.NewFile()
		sheet := "Report"
		f.SetSheetName("Sheet1", sheet)
		for rIdx, row := range reportData {
			for cIdx, cell := range row {
				cellName, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
				_ = f.SetCellValue(sheet, cellName, cell)
			}
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.xlsx", rep.Title))
		_ = f.Write(w)
		return
	}

	// Default CSV
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", rep.Title))
	writer := csv.NewWriter(w)
	_ = writer.WriteAll(reportData)
	writer.Flush()
}

// Scheduled Reports CRUD
func (h *Handler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schedules, err := h.store.ListSchedules(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil jadwal", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, schedules)
}

func (h *Handler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var sched ScheduledReport
	if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	sched.SchoolID = claims.SchoolID
	sched.CreatedBy = claims.UserID
	sched.NextRunAt = NextRun(sched, time.Now())

	created, err := h.store.CreateSchedule(r.Context(), sched)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat jadwal", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) ListScheduleRuns(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	runs, err := h.store.ListScheduleRuns(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil riwayat run", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, runs)
}

// Scope Grants CRUD
func (h *Handler) ListScopeGrants(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	grants, err := h.store.ListSchoolScopeGrants(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar akses", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, grants)
}

func (h *Handler) CreateScopeGrant(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var grant AnalyticsScopeGrant
	if err := json.NewDecoder(r.Body).Decode(&grant); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	grant.SchoolID = claims.SchoolID
	grant.GrantedBy = claims.UserID

	created, err := h.store.CreateScopeGrant(r.Context(), grant)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memberikan izin akses", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) RevokeScopeGrant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	if err := h.store.RevokeScopeGrant(r.Context(), id, claims.SchoolID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencabut izin akses", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Izin akses berhasil dicabut"})
}

// Export Tokens CRUD & Feed
func (h *Handler) ListExportTokens(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	tokens, err := h.store.ListExportTokens(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar token", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, tokens)
}

func (h *Handler) CreateExportToken(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var req struct {
		Name       string   `json:"name"`
		MetricKeys []string `json:"metric_keys"`
		SchoolIDs  []string `json:"school_ids"`
		ExpiresInDays int   `json:"expires_in_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	if req.ExpiresInDays <= 0 {
		req.ExpiresInDays = 30
	}

	rawBytes := make([]byte, 24)
	_, _ = rand.Read(rawBytes)
	rawToken := "skci_bi_" + hex.EncodeToString(rawBytes)
	hash := HashToken(rawToken)

	token := BiExportToken{
		SchoolID:   claims.SchoolID,
		Name:       req.Name,
		TokenHash:  hash,
		RawToken:   rawToken,
		MetricKeys: req.MetricKeys,
		SchoolIDs:  req.SchoolIDs,
		ExpiresAt:  time.Now().AddDate(0, 0, req.ExpiresInDays),
		CreatedBy:  claims.UserID,
	}

	created, err := h.store.CreateExportToken(r.Context(), token)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat export token", err.Error())
		return
	}
	created.RawToken = rawToken
	common.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) RevokeExportToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	if err := h.store.RevokeExportToken(r.Context(), id, claims.SchoolID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencabut token", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Token berhasil dicabut"})
}

// Feed CSV via Bearer export token
func (h *Handler) ExportFeed(w http.ResponseWriter, r *http.Request) {
	metricKey := chi.URLParam(r, "metric_key")
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		common.WriteError(w, http.StatusUnauthorized, "Missing Bearer token")
		return
	}
	rawToken := strings.TrimPrefix(authHeader, "Bearer ")
	tokenHash := HashToken(rawToken)

	token, err := h.store.GetExportTokenByHash(r.Context(), tokenHash)
	if err != nil || token.RevokedAt != nil || token.ExpiresAt.Before(time.Now()) {
		common.WriteError(w, http.StatusUnauthorized, "Token tidak valid atau kedaluwarsa")
		return
	}

	if !slices.Contains(token.MetricKeys, metricKey) {
		common.WriteError(w, http.StatusForbidden, "Metrik tidak termasuk dalam cakupan token ini")
		return
	}

	_ = h.store.TouchExportToken(r.Context(), token.ID)
	def, errD := h.metricStore.GetDefinition(r.Context(), metricKey)
	if errD != nil {
		common.WriteError(w, http.StatusNotFound, "Metrik tidak ditemukan")
		return
	}

	now := time.Now()
	var rows [][]string
	rows = append(rows, []string{"SchoolID", "MetricKey", "Unit", "Value", "AsOf"})

	for _, sID := range token.SchoolIDs {
		snap, _ := h.metricStore.GetSnapshot(r.Context(), sID, metricKey, "month", now.Format("2006-01")+"-01", "", "")
		valStr := "0"
		if snap != nil && snap.Value != nil {
			valStr = fmt.Sprintf("%d", *snap.Value)
		}
		rows = append(rows, []string{sID, def.Key, string(def.Unit), valStr, now.Format(time.RFC3339)})
	}

	w.Header().Set("Content-Type", "text/csv")
	writer := csv.NewWriter(w)
	_ = writer.WriteAll(rows)
	writer.Flush()
}
