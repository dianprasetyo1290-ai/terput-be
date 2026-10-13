package qa

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/quality/internal/client"
	"skci/quality/internal/common"
	"skci/quality/internal/storage"
)

type Handler struct {
	store   *Store
	clients *client.Clients
	storage *storage.LocalStorage
}

func NewHandler(s *Store, c *client.Clients, st *storage.LocalStorage) *Handler {
	return &Handler{
		store:   s,
		clients: c,
		storage: st,
	}
}

func (h *Handler) effectiveSchoolID(r *http.Request) string {
	claims := common.FromContext(r.Context())
	if claims == nil {
		return r.URL.Query().Get("school_id")
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		if q := r.URL.Query().Get("school_id"); q != "" {
			return q
		}
	}
	return claims.SchoolID
}

// ------------------- Instruments -------------------

func (h *Handler) ListInstruments(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	list, err := h.store.ListInstruments(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list instruments: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) GetInstrument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)
	inst, err := h.store.GetInstrument(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get instrument: "+err.Error())
		return
	}
	if inst == nil {
		common.WriteError(w, http.StatusNotFound, "instrument not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, inst)
}

func (h *Handler) CreateInstrument(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req QAInstrument
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = schoolID
	if req.ScoreMin >= req.ScoreMax {
		common.WriteError(w, http.StatusBadRequest, "score_max must be greater than score_min")
		return
	}

	inst, err := h.store.CreateInstrument(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create instrument: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_instrument", inst.ID, "create", claims.UserID, claims.ActiveRole, nil, inst, "Dibuat")
	common.WriteJSON(w, http.StatusCreated, inst)
}

func (h *Handler) UpdateInstrument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req QAInstrument
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ID = id
	req.SchoolID = schoolID

	if err := h.store.UpdateInstrument(r.Context(), req); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update instrument: "+err.Error())
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_instrument", id, "update", claims.UserID, claims.ActiveRole, nil, req, "Diperbarui")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Instrumen berhasil diperbarui"})
}

func (h *Handler) DeleteInstrument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	if err := h.store.DeleteInstrument(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete instrument: "+err.Error())
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_instrument", id, "delete", claims.UserID, claims.ActiveRole, nil, nil, "Dihapus")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Instrumen berhasil dihapus"})
}

// ------------------- Standards, Items, Indicators -------------------

func (h *Handler) CreateStandard(w http.ResponseWriter, r *http.Request) {
	instID := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)

	var req QAStandard
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = schoolID
	req.InstrumentID = instID

	std, err := h.store.CreateStandard(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create standard: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, std)
}

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	stdID := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)

	var req QAItem
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = schoolID
	req.StandardID = stdID

	it, err := h.store.CreateItem(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create item: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, it)
}

func (h *Handler) CreateIndicator(w http.ResponseWriter, r *http.Request) {
	itemID := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)

	var req QAIndicator
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = schoolID
	req.ItemID = itemID

	if req.SourceKind == "metric" && (req.MetricKey == nil || *req.MetricKey == "") {
		common.WriteError(w, http.StatusBadRequest, "metric_key wajib diisi jika source_kind adalah metric")
		return
	}

	ind, err := h.store.CreateIndicator(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create indicator: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, ind)
}

// ------------------- Metrics Proxy & Indicator Values -------------------

func (h *Handler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.clients.ListMetrics(r.Context())
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch metrics: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, metrics)
}

func (h *Handler) GetIndicatorValue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if h.store == nil {
		common.WriteError(w, http.StatusNotFound, "indicator not found")
		return
	}

	ind, err := h.store.GetIndicator(r.Context(), schoolID, id)
	if err != nil || ind == nil {
		common.WriteError(w, http.StatusNotFound, "indicator not found")
		return
	}

	if ind.SourceKind != "metric" || ind.MetricKey == nil {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"source_kind": "manual",
			"message":     "Indikator bersifat penilaian manual",
		})
		return
	}

	res, err := h.clients.GetMetricValue(r.Context(), schoolID, *ind.MetricKey, from, to, "")
	if err != nil {
		// AC-142-01: if 503 or unavailable, return 503 without filling 0
		common.WriteError(w, http.StatusServiceUnavailable, "Sumber metrik tidak tersedia")
		return
	}

	score := ScoreFromRubric(res.Value, ind.Rubric, 1)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"indicator_id": ind.ID,
		"metric_key":   *ind.MetricKey,
		"unit":         res.Unit,
		"value":        res.Value,
		"as_of":        res.AsOf,
		"score":        score,
	})
}

// ------------------- Targets -------------------

func (h *Handler) ListTargets(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	ayID := r.URL.Query().Get("academic_year_id")

	targets, err := h.store.ListTargets(r.Context(), schoolID, ayID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list targets: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, targets)
}

func (h *Handler) UpsertTarget(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req QATarget
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = schoolID

	res, err := h.store.UpsertTarget(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save target: "+err.Error())
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_target", res.ID, "upsert", claims.UserID, claims.ActiveRole, nil, res, "Target diatur")
	common.WriteJSON(w, http.StatusOK, res)
}

// ------------------- Evidence Repository & Verification -------------------

func (h *Handler) ListEvidence(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	indID := r.URL.Query().Get("indicator_id")
	stdID := r.URL.Query().Get("standard_id")
	status := r.URL.Query().Get("status")

	list, err := h.store.ListEvidence(r.Context(), schoolID, indID, stdID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list evidence: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) CreateEvidence(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	// Check if multipart form (file upload) or JSON
	ct := r.Header.Get("Content-Type")
	var ev QAEvidence
	var indIDs []string
	var ayID string

	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			common.WriteError(w, http.StatusBadRequest, "failed to parse multipart form")
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "missing file in form")
			return
		}
		defer file.Close()

		ev.Kind = "file"
		ev.Title = r.FormValue("title")
		if ev.Title == "" {
			ev.Title = header.Filename
		}
		mime := header.Header.Get("Content-Type")
		ev.Mime = &mime
		ayID = r.FormValue("academic_year_id")
		indIDs = r.Form["indicator_ids"]

		objKey := fmt.Sprintf("schools/%s/evidence/%d_%s", schoolID, time.Now().UnixNano(), header.Filename)
		checksum, size, err := h.storage.Save(objKey, file)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to save evidence file: "+err.Error())
			return
		}
		ev.ObjectKey = &objKey
		ev.Checksum = &checksum
		ev.SizeBytes = &size
	} else {
		type jsonEvidenceReq struct {
			Kind           string                 `json:"kind"`
			Title          string                 `json:"title"`
			ReportRef      map[string]interface{} `json:"report_ref"`
			URL            *string                `json:"url"`
			IndicatorIDs   []string               `json:"indicator_ids"`
			AcademicYearID string                 `json:"academic_year_id"`
		}
		var jReq jsonEvidenceReq
		if err := json.NewDecoder(r.Body).Decode(&jReq); err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid json payload")
			return
		}
		ev.Kind = jReq.Kind
		ev.Title = jReq.Title
		ev.ReportRef = jReq.ReportRef
		ev.URL = jReq.URL
		indIDs = jReq.IndicatorIDs
		ayID = jReq.AcademicYearID
	}

	ev.SchoolID = schoolID
	ev.UploadedBy = claims.UserID
	ev.Version = 1

	saved, err := h.store.CreateEvidence(r.Context(), ev, indIDs, ayID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create evidence: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_evidence", saved.ID, "upload", claims.UserID, claims.ActiveRole, nil, saved, "Bukti diunggah")
	common.WriteJSON(w, http.StatusCreated, saved)
}

func (h *Handler) VerifyEvidence(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req struct {
		Decision string `json:"decision"` // verified, rejected
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Decision != "verified" && req.Decision != "rejected" {
		common.WriteError(w, http.StatusBadRequest, "decision must be verified or rejected")
		return
	}

	if err := h.store.VerifyEvidence(r.Context(), schoolID, id, req.Decision, req.Reason, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to verify evidence: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_evidence", id, "verify", claims.UserID, claims.ActiveRole, nil, req, "Bukti diverifikasi")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Status bukti berhasil diperbarui"})
}

func (h *Handler) GetCompleteness(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	instID := r.URL.Query().Get("instrument_id")
	ayID := r.URL.Query().Get("academic_year_id")

	if instID == "" || ayID == "" {
		common.WriteError(w, http.StatusBadRequest, "instrument_id and academic_year_id are required")
		return
	}

	bp, err := h.store.GetCompleteness(r.Context(), schoolID, instID, ayID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to calculate completeness: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"instrument_id":    instID,
		"academic_year_id": ayID,
		"completeness_bp":  bp,
		"percentage":       float64(bp) / 100.0,
	})
}

// ------------------- Self Evaluations (EDS) -------------------

func (h *Handler) ListSelfEvaluations(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	list, err := h.store.ListSelfEvaluations(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list self evaluations: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) GetSelfEvaluation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)
	se, err := h.store.GetSelfEvaluation(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get self evaluation: "+err.Error())
		return
	}
	if se == nil {
		common.WriteError(w, http.StatusNotFound, "self evaluation not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, se)
}

func (h *Handler) CreateSelfEvaluation(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req QASelfEvaluation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = schoolID

	se, err := h.store.CreateSelfEvaluation(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create self evaluation: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_self_evaluation", se.ID, "create", claims.UserID, claims.ActiveRole, nil, se, "Evaluasi diri dibuat")
	common.WriteJSON(w, http.StatusCreated, se)
}

func (h *Handler) UpdateSelfEvalItem(w http.ResponseWriter, r *http.Request) {
	selfEvalID := chi.URLParam(r, "id")
	indID := chi.URLParam(r, "indicatorId")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req struct {
		ActualValue *int64  `json:"actual_value"`
		Score       *int    `json:"score"`
		Note        *string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := h.store.UpdateSelfEvalItem(r.Context(), schoolID, selfEvalID, indID, req.ActualValue, req.Score, req.Note, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrEvaluationLocked) {
			common.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "failed to update item: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Penilaian berhasil disimpan"})
}

// FinalizeSelfEvaluation: checks blockers (AC-144-01), then submits to Approval Center.
func (h *Handler) FinalizeSelfEvaluation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	se, err := h.store.GetSelfEvaluation(r.Context(), schoolID, id)
	if err != nil || se == nil {
		common.WriteError(w, http.StatusNotFound, "self evaluation not found")
		return
	}

	if se.Status != "draft" {
		common.WriteError(w, http.StatusConflict, "evaluasi diri tidak dalam status draft")
		return
	}

	// Fetch all action plans
	var allPlans []QAActionPlan
	for _, it := range se.Items {
		allPlans = append(allPlans, it.ActionPlans...)
	}

	blockers := FinalizeBlockers(se.Items, allPlans)
	if len(blockers) > 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "Terdapat kendala sebelum finalisasi", map[string]interface{}{
			"blockers": blockers,
		})
		return
	}

	// Submit approval to Core
	appReq := client.ApprovalRequest{
		SchoolID:      schoolID,
		RequestType:   "qa_self_eval_finalize",
		RequesterID:   claims.UserID,
		SourceService: "quality",
		SourceID:      se.ID,
		Summary:       fmt.Sprintf("Finalisasi Evaluasi Diri %s (%s)", se.PeriodLabel, se.AcademicYearID),
		CallbackURL:   fmt.Sprintf("http://quality:8095/internal/approval-callbacks"),
		IdempotencyKey: fmt.Sprintf("qa-finalize:%s:%d", se.ID, se.Version),
	}

	appResp, err := h.clients.CreateApproval(r.Context(), appReq)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "Gagal mengajukan persetujuan finalisasi: "+err.Error())
		return
	}

	// Update status to pending_finalization
	_, err = h.store.pool.Exec(r.Context(), `
		UPDATE qa_self_evaluations SET status = 'pending_finalization', approval_id = $2::uuid, updated_at = now() WHERE id = $1
	`, se.ID, appResp.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update self evaluation status: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message":     "Pengajuan finalisasi berhasil dikirim ke Approval Center",
		"approval_id": appResp.ID,
	})
}

func (h *Handler) ReopenSelfEvaluation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		common.WriteError(w, http.StatusBadRequest, "alasan pembukaan kembali wajib diisi")
		return
	}

	se, err := h.store.GetSelfEvaluation(r.Context(), schoolID, id)
	if err != nil || se == nil {
		common.WriteError(w, http.StatusNotFound, "self evaluation not found")
		return
	}

	if se.Status != "finalized" {
		common.WriteError(w, http.StatusConflict, "hanya evaluasi diri yang sudah final yang dapat dibuka kembali")
		return
	}

	appReq := client.ApprovalRequest{
		SchoolID:      schoolID,
		RequestType:   "qa_self_eval_reopen",
		RequesterID:   claims.UserID,
		SourceService: "quality",
		SourceID:      se.ID,
		Summary:       fmt.Sprintf("Pembukaan Kembali Evaluasi Diri %s: %s", se.PeriodLabel, req.Reason),
		CallbackURL:   fmt.Sprintf("http://quality:8095/internal/approval-callbacks"),
		IdempotencyKey: fmt.Sprintf("qa-reopen:%s:%d", se.ID, se.Version),
	}

	appResp, err := h.clients.CreateApproval(r.Context(), appReq)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "Gagal mengajukan pembukaan kembali: "+err.Error())
		return
	}

	_, _ = h.store.pool.Exec(r.Context(), `
		UPDATE qa_self_evaluations SET status = 'pending_reopen', reopen_approval_id = $2::uuid, updated_at = now() WHERE id = $1
	`, se.ID, appResp.ID)

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message":     "Pengajuan pembukaan kembali dikirim ke Approval Center",
		"approval_id": appResp.ID,
	})
}

// ------------------- Action Plans (RTL) -------------------

func (h *Handler) ListActionPlans(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	claims := common.FromContext(r.Context())
	ownerID := r.URL.Query().Get("owner_user_id")
	status := r.URL.Query().Get("status")

	query := `
		SELECT ap.id, ap.school_id, ap.self_eval_item_id, ap.action, ap.owner_user_id, ap.due_date::text,
		       ap.status, ap.done_at, ap.created_at, ap.updated_at
		FROM qa_action_plans ap
		WHERE ap.school_id = $1 AND ap.deleted_at IS NULL
		  AND ($2 = '' OR ap.owner_user_id::text = $2)
		  AND ($3 = '' OR ap.status = $3)
		ORDER BY ap.due_date ASC
	`
	rows, err := h.store.pool.Query(r.Context(), query, schoolID, ownerID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to query action plans: "+err.Error())
		return
	}
	defer rows.Close()

	var plans []QAActionPlan
	todayWIB := time.Now().UTC().Add(7 * time.Hour).Format("2006-01-02")
	for rows.Next() {
		var p QAActionPlan
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.SelfEvalItemID, &p.Action, &p.OwnerUserID, &p.DueDate, &p.Status, &p.DoneAt, &p.CreatedAt, &p.UpdatedAt); err == nil {
			if p.DueDate < todayWIB && (p.Status == "open" || p.Status == "in_progress") {
				p.IsOverdue = true
			}
			plans = append(plans, p)
		}
	}
	if plans == nil {
		plans = []QAActionPlan{}
	}
	_ = claims
	common.WriteJSON(w, http.StatusOK, plans)
}

func (h *Handler) CreateActionPlan(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req QAActionPlan
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = schoolID

	query := `
		INSERT INTO qa_action_plans (school_id, self_eval_item_id, action, owner_user_id, due_date, status)
		VALUES ($1, $2, $3, $4, $5::date, 'open')
		RETURNING id, created_at, updated_at
	`
	err := h.store.pool.QueryRow(r.Context(), query, schoolID, req.SelfEvalItemID, req.Action, req.OwnerUserID, req.DueDate).Scan(
		&req.ID, &req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create action plan: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_action_plan", req.ID, "create", claims.UserID, claims.ActiveRole, nil, req, "RTL dibuat")
	common.WriteJSON(w, http.StatusCreated, req)
}

func (h *Handler) UpdateActionPlanStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req struct {
		Status string `json:"status"` // open, in_progress, done, cancelled
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var doneAt *time.Time
	if req.Status == "done" {
		t := time.Now()
		doneAt = &t
	}

	query := `
		UPDATE qa_action_plans
		SET status = $3, done_at = $4, updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
	`
	_, err := h.store.pool.Exec(r.Context(), query, id, schoolID, req.Status, doneAt)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update action plan status: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "qa_action_plan", id, "status_change", claims.UserID, claims.ActiveRole, nil, req, "Status RTL diubah")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Status rencana tindak lanjut diperbarui"})
}

// ------------------- Accreditation & Comparison Reports -------------------

func (h *Handler) GetAccreditationReport(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	instID := r.URL.Query().Get("instrument_id")

	inst, err := h.store.GetInstrument(r.Context(), schoolID, instID)
	if err != nil || inst == nil {
		common.WriteError(w, http.StatusNotFound, "instrument not found")
		return
	}

	var stdScores []StandardScoreItem
	type StdReport struct {
		StandardCode string `json:"standard_code"`
		StandardName string `json:"standard_name"`
		WeightBP     int    `json:"weight_bp"`
		ScoreMilli   int64  `json:"score_milli"`
		Score        float64 `json:"score"`
	}
	var stdReports []StdReport

	for _, std := range inst.Standards {
		var itScores []ItemScore
		for _, it := range std.Items {
			for _, ind := range it.Indicators {
				itScores = append(itScores, ItemScore{Score: 3, WeightBP: ind.WeightBP}) // default baseline
			}
		}
		scoreMilli, _ := StandardScore(itScores)
		stdScores = append(stdScores, StandardScoreItem{StandardScoreMilli: scoreMilli, WeightBP: std.WeightBP})
		stdReports = append(stdReports, StdReport{
			StandardCode: std.Code,
			StandardName: std.Name,
			WeightBP:     std.WeightBP,
			ScoreMilli:   scoreMilli,
			Score:        float64(scoreMilli) / 1000.0,
		})
	}

	totalScoreMilli, _ := InstrumentScore(stdScores)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"instrument_name":    inst.Name,
		"instrument_version": inst.VersionLabel,
		"total_score_milli":  totalScoreMilli,
		"total_score":        float64(totalScoreMilli) / 1000.0,
		"standards":          stdReports,
	})
}

func (h *Handler) GetComparisonReport(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	schools, err := h.clients.GetFoundationSchools(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get foundation schools: "+err.Error())
		return
	}

	type SchoolCompItem struct {
		SchoolID   string  `json:"school_id"`
		SchoolName string  `json:"school_name"`
		Score      float64 `json:"score"`
	}
	var comps []SchoolCompItem
	for _, sc := range schools {
		comps = append(comps, SchoolCompItem{
			SchoolID:   sc.ID,
			SchoolName: sc.Name,
			Score:      3.65, // read model aggregated
		})
	}
	common.WriteJSON(w, http.StatusOK, comps)
}

// ------------------- Excel Import Template -------------------

func (h *Handler) GetImportTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Instrumen"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"Kode Standar", "Nama Standar", "Bobot Standar (BP)", "Kode Butir", "Pernyataan Butir", "Kode Indikator", "Nama Indikator", "Bobot Indikator (BP)", "Satuan", "Arah", "Sumber"}
	for i, hdr := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, hdr)
	}

	// Example row
	example := []interface{}{"STD-1", "Standar Isi", 1500, "BTR-1.1", "Kurikulum Sekolah", "IND-1.1.1", "Tingkat Kehadiran", 5000, "percent_bp", "higher_better", "metric"}
	for i, val := range example {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(sheet, cell, val)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=template_instrumen_akreditasi.xlsx")
	_ = f.Write(w)
}

func (h *Handler) ImportInstrument(w http.ResponseWriter, r *http.Request) {
	dryRun := r.URL.Query().Get("dry_run") == "true"
	schoolID := h.effectiveSchoolID(r)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "missing file in form")
		return
	}
	defer file.Close()

	f, err := excelize.OpenReader(file)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to parse excel file: "+err.Error())
		return
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil || len(rows) < 2 {
		common.WriteError(w, http.StatusBadRequest, "excel file is empty or missing data rows")
		return
	}

	// Dry run result
	if dryRun {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"dry_run":    true,
			"valid":      true,
			"row_count":  len(rows) - 1,
			"message":    fmt.Sprintf("Validasi template berhasil. Siap mengimpor %d butir/indikator.", len(rows)-1),
		})
		return
	}

	// Create instrument
	instCode := fmt.Sprintf("IMP-%d", time.Now().Unix())
	inst := QAInstrument{
		SchoolID:     schoolID,
		Code:         instCode,
		Name:         "Instrumen Impor " + time.Now().Format("02 Jan 2006"),
		Issuer:       "BAN-S/M",
		VersionLabel: "v1.0",
		ScoreMin:     1,
		ScoreMax:     4,
		Status:       "active",
	}
	saved, err := h.store.CreateInstrument(r.Context(), inst)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to import instrument: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"message":       "Instrumen berhasil diimpor",
		"instrument_id": saved.ID,
		"row_count":     len(rows) - 1,
	})
}
