package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/jobs"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

// Service-to-service endpoints (X-Internal-Key). Contract: T1 §3 — do not
// change field names without the reviewer; T2–T7 depend on them.

type internalCreateApprovalRequest struct {
	SchoolID       string         `json:"school_id"`
	RequestType    string         `json:"request_type"`
	SourceService  string         `json:"source_service"`
	SourceID       string         `json:"source_id"`
	Title          string         `json:"title"`
	Summary        map[string]any `json:"summary"`
	AmountIDR      *int64         `json:"amount_idr,omitempty"`
	Flags          []string       `json:"flags"`
	MakerUserID    string         `json:"maker_user_id"`
	SubjectUserID  *string        `json:"subject_user_id,omitempty"`
	CallbackURL    string         `json:"callback_url"`
	CorrelationID  string         `json:"correlation_id"`
	IdempotencyKey string         `json:"idempotency_key"`
}

type internalApprovalResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	CurrentSeq *int   `json:"current_seq"`
}

func approvalResponse(req *model.ApprovalRequest) internalApprovalResponse {
	return internalApprovalResponse{ID: req.ID, Status: req.Status, CurrentSeq: req.CurrentSeq}
}

func (in internalCreateApprovalRequest) validate() string {
	switch {
	case !isUUID(in.SchoolID):
		return "school_id wajib berupa UUID"
	case !isUUID(in.MakerUserID):
		return "maker_user_id wajib berupa UUID"
	case in.SubjectUserID != nil && !isUUID(*in.SubjectUserID):
		return "subject_user_id harus berupa UUID"
	case !inList(model.KnownRequestTypes, in.RequestType):
		return fmt.Sprintf("request_type %q tidak dikenal", in.RequestType)
	case strings.TrimSpace(in.SourceService) == "" || strings.TrimSpace(in.SourceID) == "":
		return "source_service dan source_id wajib diisi"
	case !model.SourceAllowed(in.RequestType, in.SourceService):
		return fmt.Sprintf("request_type %s hanya dapat diajukan oleh layanan %s", in.RequestType, model.RequestTypeSources[in.RequestType])
	case strings.TrimSpace(in.Title) == "":
		return "title wajib diisi"
	case in.Summary == nil:
		return "summary wajib diisi"
	case in.AmountIDR != nil && *in.AmountIDR < 0:
		return "amount_idr tidak boleh negatif"
	case !strings.HasPrefix(in.CallbackURL, "http://") && !strings.HasPrefix(in.CallbackURL, "https://"):
		return "callback_url wajib berupa URL http(s)"
	case strings.TrimSpace(in.CorrelationID) == "" || strings.TrimSpace(in.IdempotencyKey) == "":
		return "correlation_id dan idempotency_key wajib diisi"
	}
	return ""
}

// InternalCreateApproval: 201 new, 200 replay of the same idempotency_key,
// 422 when no active flow / no applicable step / supervisor unresolvable.
// Never auto-approves.
func (h *Handler) InternalCreateApproval(w http.ResponseWriter, r *http.Request) {
	var in internalCreateApprovalRequest
	if !common.Decode(w, r, &in) {
		return
	}
	if msg := in.validate(); msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}

	// Replay first: a retry must succeed even if the flow changed since.
	if existing, err := h.Store.GetRequestByIdempotency(r.Context(), in.SchoolID, in.IdempotencyKey); err == nil {
		common.WriteJSON(w, http.StatusOK, approvalResponse(existing), nil)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeStoreErr(w, err, "")
		return
	}

	flow, err := h.Store.GetFlowByRequestType(r.Context(), in.SchoolID, in.RequestType)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusUnprocessableEntity, fmt.Sprintf("alur approval %s belum dikonfigurasi", in.RequestType))
		return
	} else if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	resolved, err := model.ResolveSteps(flow.Steps, in.AmountIDR, in.Flags)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	steps, snapshot, msg := h.buildSteps(r.Context(), in, resolved)
	if msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	rerouted, msg := h.guardMakerOnlySteps(r.Context(), in.SchoolID, in.MakerUserID, steps)
	if msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}

	created, err := h.Store.CreateRequest(r.Context(), model.ApprovalRequest{
		SchoolID:       in.SchoolID,
		RequestType:    in.RequestType,
		SourceService:  in.SourceService,
		SourceID:       in.SourceID,
		Title:          in.Title,
		Summary:        model.MaskSensitive(in.Summary), // defense in depth
		AmountIDR:      in.AmountIDR,
		Flags:          in.Flags,
		MakerUserID:    in.MakerUserID,
		SubjectUserID:  in.SubjectUserID,
		CallbackURL:    in.CallbackURL,
		CorrelationID:  in.CorrelationID,
		IdempotencyKey: in.IdempotencyKey,
		FlowSnapshot:   snapshot,
	}, steps)
	if errors.Is(err, store.ErrIdempotentHit) {
		common.WriteJSON(w, http.StatusOK, approvalResponse(created), nil)
		return
	} else if err != nil {
		writeStoreErr(w, err, "")
		return
	}

	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID: &in.SchoolID, Service: "core", Module: "approval", EntityType: "approval_request",
		EntityID: created.ID, Action: "approval_requested", ActorUserID: &in.MakerUserID,
		AfterData: map[string]any{"request_type": in.RequestType, "source_service": in.SourceService, "source_id": in.SourceID,
			"amount_idr": in.AmountIDR, "steps": len(steps), "rerouted_steps": rerouted},
		CorrelationID: &in.CorrelationID,
	})
	if first := model.CurrentStep(created); first != nil {
		h.bg(func(ctx context.Context) {
			jobs.NotifyStep(ctx, h.Store, h.Client, created, *first, "Permintaan Persetujuan Baru",
				fmt.Sprintf("Pengajuan %q menunggu persetujuan Anda.", created.Title))
		})
	}
	common.WriteJSON(w, http.StatusCreated, approvalResponse(created), nil)
}

// buildSteps renumbers the applicable flow steps 1..n (filtered-out steps
// leave no gaps), resolves every direct_supervisor step to a concrete user via
// HR and stores it in approver_user_id. Returns a 422 message on failure:
// no silent fallback to another approver.
func (h *Handler) buildSteps(ctx context.Context, in internalCreateApprovalRequest, resolved []model.FlowStep) ([]model.ApprovalStep, []model.FlowStep, string) {
	steps := make([]model.ApprovalStep, 0, len(resolved))
	snapshot := make([]model.FlowStep, 0, len(resolved))
	now := time.Now()
	for i, fs := range resolved {
		fs.ID, fs.FlowID, fs.Seq = "", "", i+1
		st := model.ApprovalStep{
			Seq: fs.Seq, ApproverKind: fs.ApproverKind, ApproverRole: fs.ApproverRole, ApproverUserID: fs.ApproverUserID,
			Status: model.StepStatusWaiting, SLAHours: fs.SLAHours, EscalateToRole: fs.EscalateToRole,
		}
		if fs.ApproverKind == model.ApproverKindDirectSupervisor {
			if in.SubjectUserID == nil {
				return nil, nil, "subject_user_id diperlukan untuk tahap atasan langsung"
			}
			sup, err := h.supervisorOf(ctx, in.SchoolID, *in.SubjectUserID)
			if err != "" {
				return nil, nil, err
			}
			st.ApproverUserID = &sup
		}
		if i == 0 {
			st.Status = model.StepStatusPending
			if fs.SLAHours != nil {
				due := now.Add(time.Duration(*fs.SLAHours) * time.Hour)
				st.DueAt = &due
			}
		}
		steps = append(steps, st)
		snapshot = append(snapshot, fs)
	}
	return steps, snapshot, ""
}

// guardMakerOnlySteps (BUG-111, BR-GEN-09): a step whose only approver is
// the maker would wait forever, so it is routed to its escalate_to_role, or
// the request is refused (422) when no independent approver exists. Role
// holders come from identity; while identity is not configured the guard is
// skipped and the SLA escalation job stays the safety net.
func (h *Handler) guardMakerOnlySteps(ctx context.Context, schoolID, makerID string, steps []model.ApprovalStep) ([]int, string) {
	if h.Client == nil || h.Client.IdentityURL == "" {
		return nil, ""
	}
	holders := func(role string) ([]string, bool) {
		ids, err := h.Client.UsersByRole(ctx, schoolID, role)
		if err != nil {
			log.Printf("maker-only guard: holders of %s: %v", role, err)
			return nil, false
		}
		return ids, true
	}
	var rerouted []int
	for i, st := range steps {
		var approvers []string
		switch {
		case st.ApproverUserID != nil:
			approvers = []string{*st.ApproverUserID}
		case st.ApproverRole != nil:
			ids, ok := holders(*st.ApproverRole)
			if !ok {
				continue
			}
			approvers = ids
		}
		var escalate []string
		if st.EscalateToRole != nil && !slices.ContainsFunc(approvers, func(id string) bool { return id != makerID }) {
			ids, ok := holders(*st.EscalateToRole)
			if !ok {
				continue
			}
			escalate = ids
		}
		next, moved, err := model.RerouteMakerOnlyStep(st, makerID, approvers, escalate)
		if err != nil {
			return nil, fmt.Sprintf("tahap %d: %v; tambahkan penyetuju lain atau atur peran eskalasi pada alur approval", st.Seq, err)
		}
		if moved {
			steps[i] = next
			rerouted = append(rerouted, st.Seq)
		}
	}
	return rerouted, ""
}

func (h *Handler) supervisorOf(ctx context.Context, schoolID, subjectUserID string) (string, string) {
	if h.Client == nil || h.Client.HRURL == "" {
		return "", "layanan HR belum aktif sehingga atasan langsung tidak dapat ditentukan"
	}
	sup, err := h.Client.GetSupervisor(ctx, schoolID, subjectUserID)
	switch {
	case errors.Is(err, client.ErrNotFound):
		return "", "atasan langsung untuk pegawai ini belum ditetapkan atau belum memiliki akun"
	case err != nil:
		return "", "layanan HR tidak dapat dihubungi untuk menentukan atasan langsung: " + err.Error()
	case sup == subjectUserID:
		return "", "atasan langsung tidak boleh sama dengan pegawai yang bersangkutan"
	}
	return sup, ""
}

// InternalGetApprovalBySource: newest request + steps for a source
// transaction (FR-79-007). school_id is optional.
func (h *Handler) InternalGetApprovalBySource(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	schoolID, svc, srcID := q.Get("school_id"), q.Get("source_service"), q.Get("source_id")
	if svc == "" || srcID == "" || (schoolID != "" && !isUUID(schoolID)) {
		common.WriteError(w, http.StatusBadRequest, "source_service dan source_id diperlukan (school_id opsional, UUID)")
		return
	}
	req, err := h.Store.GetRequestBySource(r.Context(), schoolID, svc, srcID)
	if err != nil {
		writeStoreErr(w, err, "approval tidak ditemukan untuk transaksi ini")
		return
	}
	common.WriteJSON(w, http.StatusOK, req, nil)
}

type internalCancelRequest struct {
	ActorUserID string `json:"actor_user_id"`
	Reason      string `json:"reason"`
}

func (h *Handler) InternalCancelApproval(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in internalCancelRequest
	if !common.Decode(w, r, &in) {
		return
	}
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "permintaan approval tidak ditemukan")
		return
	}
	if !isUUID(in.ActorUserID) || strings.TrimSpace(in.Reason) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "actor_user_id (UUID) dan reason wajib diisi")
		return
	}
	cancelled, err := h.Store.CancelRequest(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err, "permintaan approval tidak ditemukan")
		return
	}
	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID: &cancelled.SchoolID, Service: "core", Module: "approval", EntityType: "approval_request",
		EntityID: id, Action: "approval_cancelled", ActorUserID: &in.ActorUserID, Reason: &in.Reason,
		CorrelationID: &cancelled.CorrelationID,
	})
	common.WriteJSON(w, http.StatusOK, approvalResponse(cancelled), nil)
}

type internalExceptionRequest struct {
	SchoolID       string         `json:"school_id"`
	SourceService  string         `json:"source_service"`
	Kind           string         `json:"kind"`
	Severity       string         `json:"severity"`
	CorrelationID  string         `json:"correlation_id"`
	ErrorMessage   string         `json:"error_message"`
	PayloadSummary map[string]any `json:"payload_summary"`
	RetryURL       *string        `json:"retry_url,omitempty"`
	MaxAttempts    int            `json:"max_attempts"`
	IdempotencyKey string         `json:"idempotency_key"`
}

func (in internalExceptionRequest) validate() string {
	switch {
	case !isUUID(in.SchoolID):
		return "school_id wajib berupa UUID"
	case strings.TrimSpace(in.SourceService) == "":
		return "source_service wajib diisi"
	case !inList(model.ExceptionKinds, in.Kind):
		return fmt.Sprintf("kind %q tidak dikenal", in.Kind)
	case !inList(model.Severities, in.Severity):
		return fmt.Sprintf("severity %q tidak dikenal", in.Severity)
	case strings.TrimSpace(in.CorrelationID) == "" || strings.TrimSpace(in.ErrorMessage) == "":
		return "correlation_id dan error_message wajib diisi"
	case in.MaxAttempts <= 0:
		return "max_attempts wajib lebih dari 0"
	case strings.TrimSpace(in.IdempotencyKey) == "":
		return "idempotency_key wajib diisi"
	case in.RetryURL != nil && !strings.HasPrefix(*in.RetryURL, "http://") && !strings.HasPrefix(*in.RetryURL, "https://"):
		return "retry_url wajib berupa URL http(s)"
	}
	return ""
}

// InternalCreateException: same idempotency_key -> attempts+1 and latest
// error on the existing row (200); new row -> 201.
func (h *Handler) InternalCreateException(w http.ResponseWriter, r *http.Request) {
	var in internalExceptionRequest
	if !common.Decode(w, r, &in) {
		return
	}
	if in.MaxAttempts <= 0 {
		in.MaxAttempts = 3
	}
	// BUG-113: accept the dialects existing reporters already send instead of
	// dropping their failures with 422 (severity aliases, no payload summary,
	// correlation id carried only in the X-Correlation-ID header).
	if sev, ok := model.NormalizeSeverity(in.Severity); ok {
		in.Severity = sev
	}
	if in.PayloadSummary == nil {
		in.PayloadSummary = map[string]any{}
	}
	if strings.TrimSpace(in.CorrelationID) == "" {
		in.CorrelationID = common.CorrelationID(r.Context())
	}
	if msg := in.validate(); msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	saved, isNew, err := h.Store.UpsertException(r.Context(), model.IntegrationException{
		SchoolID:       in.SchoolID,
		SourceService:  in.SourceService,
		Kind:           in.Kind,
		Severity:       in.Severity,
		CorrelationID:  in.CorrelationID,
		ErrorMessage:   in.ErrorMessage,
		MaxAttempts:    in.MaxAttempts,
		PayloadSummary: model.MaskSensitive(in.PayloadSummary), // AC-80-03, defense in depth
		RetryURL:       in.RetryURL,
		IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	if isNew {
		_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
			SchoolID: &in.SchoolID, Service: "core", Module: "exception_queue", EntityType: "integration_exception",
			EntityID: saved.ID, Action: "exception_logged", Reason: &in.ErrorMessage, CorrelationID: &in.CorrelationID,
			AfterData: map[string]any{"source_service": in.SourceService, "kind": in.Kind, "severity": in.Severity},
		})
		h.bg(func(ctx context.Context) { jobs.NotifyCriticalException(ctx, h.Client, saved) })
	}
	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}
	common.WriteJSON(w, status, saved, nil)
}

type internalAuditRequest struct {
	SchoolID      *string `json:"school_id,omitempty"`
	Service       string  `json:"service"`
	Module        string  `json:"module"`
	EntityType    string  `json:"entity_type"`
	EntityID      string  `json:"entity_id"`
	Action        string  `json:"action"`
	ActorUserID   *string `json:"actor_user_id,omitempty"`
	ActorRole     *string `json:"actor_role,omitempty"`
	Before        any     `json:"before,omitempty"`
	After         any     `json:"after,omitempty"`
	Reason        *string `json:"reason,omitempty"`
	CorrelationID *string `json:"correlation_id,omitempty"`
	// ClientIP is the end user's IP as seen by the calling service (optional).
	// Only its hash is stored; without it ip_hash stays NULL rather than
	// recording the calling service's own address.
	ClientIP string `json:"client_ip,omitempty"`
}

func maskAny(v any) any {
	if m, ok := v.(map[string]any); ok {
		return model.MaskSensitive(m)
	}
	return v
}

func (h *Handler) InternalCreateAudit(w http.ResponseWriter, r *http.Request) {
	var in internalAuditRequest
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Service == "" || in.Module == "" || in.EntityType == "" || in.EntityID == "" || in.Action == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "service, module, entity_type, entity_id, dan action wajib diisi")
		return
	}
	if (in.SchoolID != nil && !isUUID(*in.SchoolID)) || (in.ActorUserID != nil && !isUUID(*in.ActorUserID)) {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id dan actor_user_id harus berupa UUID")
		return
	}
	if in.CorrelationID == nil || *in.CorrelationID == "" {
		in.CorrelationID = correlationPtr(r) // BUG-117: X-Correlation-ID of the caller
	}
	if err := h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID:      in.SchoolID,
		Service:       in.Service,
		Module:        in.Module,
		EntityType:    in.EntityType,
		EntityID:      in.EntityID,
		Action:        in.Action,
		ActorUserID:   in.ActorUserID,
		ActorRole:     in.ActorRole,
		BeforeData:    maskAny(in.Before), // FR-06-001 / NFR-HR-01
		AfterData:     maskAny(in.After),
		Reason:        in.Reason,
		CorrelationID: in.CorrelationID,
		IPHash:        HashIP(in.ClientIP),
	}); err != nil {
		writeStoreErr(w, err, "")
		return
	}
	common.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"}, nil)
}
