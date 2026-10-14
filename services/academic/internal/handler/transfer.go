package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/client"
	"skci/academic/internal/common"
	"skci/academic/internal/model"
	"skci/academic/internal/store"
)

// ---------------------------------------------------------------------------
// Student Status Requests (Menu 78: Transfer & Withdrawal Siswa)
// ---------------------------------------------------------------------------

type createStatusRequestInput struct {
	StudentUserID   string                 `json:"student_user_id"`
	Kind            string                 `json:"kind"`
	ToClassroomID   *string                `json:"to_classroom_id"`
	ToSchoolID      *string                `json:"to_school_id"`
	EffectiveDate   string                 `json:"effective_date"`
	Reason          string                 `json:"reason"`
	Attachments     []model.AttachmentItem `json:"attachments"`
	RefundAmountIDR *int64                 `json:"refund_amount_idr"`
	IdempotencyKey  *string                `json:"idempotency_key"`
}

// requireMenu78Role enforces the Menu 78 role matrix (model.Can*): superadmin
// has no Mutasi Siswa access (BUG-177).
func requireMenu78Role(w http.ResponseWriter, r *http.Request, allowed func(string) bool) (*common.Claims, bool) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	if !allowed(claims.ActiveRole) {
		common.WriteError(w, http.StatusForbidden, "forbidden: peran Anda tidak memiliki akses ke menu Pindah & Keluar Siswa")
		return nil, false
	}
	return claims, true
}

// writeErrorWithData is common.WriteError plus a data payload (e.g. the id of
// the request that blocks a duplicate).
func writeErrorWithData(w http.ResponseWriter, status int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(common.Envelope{Data: data, Message: msg, Errors: map[string]string{"message": msg}})
}

func writeActiveRequestConflict(w http.ResponseWriter, existing *model.StudentStatusRequest) {
	writeErrorWithData(w, http.StatusConflict,
		"siswa sudah memiliki pengajuan mutasi yang sedang berjalan; selesaikan atau batalkan pengajuan tersebut terlebih dahulu",
		map[string]any{
			"existing_request_id": existing.ID,
			"existing_status":     existing.Status,
			"existing_kind":       existing.Kind,
			"link":                "/akademik/mutasi-siswa/" + existing.ID,
		})
}

// CreateStudentStatusRequest is POST /student-status-requests (admin, walikelas).
// Fail-closed (BUG-057): the request is only kept when the Approval Center
// accepted it; otherwise nothing is stored and the configuration error is shown.
func (h *Handler) CreateStudentStatusRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireMenu78Role(w, r, model.CanCreateStatusRequest)
	if !ok {
		return
	}
	if h.Core == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "Approval Center belum terhubung (CORE_BASE_URL); pengajuan tidak dapat dikirim")
		return
	}

	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}

	var in createStatusRequestInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	in.StudentUserID = strings.TrimSpace(in.StudentUserID)
	if in.StudentUserID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "student_user_id wajib diisi")
		return
	}

	// 1. Check student's current active enrollment
	enrollment, err := h.Store.GetActiveStudentEnrollment(r.Context(), schoolID, in.StudentUserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to check student enrollment: "+err.Error())
		return
	}
	if enrollment == nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "siswa tidak memiliki kelas aktif di sekolah ini")
		return
	}

	// 2. If walikelas, verify they are the homeroom teacher of the student's current classroom
	if !h.requireOwnClassroomIfWaliKelas(w, r, schoolID, enrollment.ClassroomID) {
		return
	}

	// 2b. Idempotent replay returns the original request; otherwise one
	// active request per student (BUG-137).
	if in.IdempotencyKey != nil && *in.IdempotencyKey != "" {
		prev, err := h.Store.GetStudentStatusRequestByIdempotency(r.Context(), schoolID, *in.IdempotencyKey)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to check idempotency: "+err.Error())
			return
		}
		if prev != nil {
			common.WriteJSON(w, http.StatusCreated, prev, nil)
			return
		}
	}
	active, err := h.Store.FindActiveStatusRequest(r.Context(), schoolID, in.StudentUserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to check running requests: "+err.Error())
		return
	}
	if active != nil {
		writeActiveRequestConflict(w, active)
		return
	}

	fromClassroomID := enrollment.ClassroomID
	toClassID := ""
	if in.ToClassroomID != nil {
		toClassID = strings.TrimSpace(*in.ToClassroomID)
	}
	toSchoolID := ""
	if in.ToSchoolID != nil {
		toSchoolID = strings.TrimSpace(*in.ToSchoolID)
	}

	// 3. Validate domain rules
	if err := model.ValidateCreateStatusRequest(in.Kind, in.Reason, fromClassroomID, toClassID, toSchoolID, schoolID, in.EffectiveDate); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// BUG-176: not before the student entered the active class.
	if err := model.ValidateEffectiveDateAfterEnrollment(in.EffectiveDate, enrollment.EnrolledAt, nil); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// 3b. Capacity of the destination classroom (FR-39-001). Checked again,
	// under lock, when the transfer is applied.
	if in.Kind == model.KindClassTransfer && toClassID != "" {
		if err := h.Store.CheckClassroomCapacity(r.Context(), schoolID, toClassID, in.StudentUserID); err != nil {
			if !writeCapacityError(w, err) {
				common.WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
	}

	// 4. Fetch financial summary from billing (FR-78-002, AC-78-01)
	var snapJSON json.RawMessage
	var totalOutstanding int64
	var depositIDR int64
	if h.Billing != nil {
		snap, bErr := h.Billing.GetStudentFinancialSummary(r.Context(), schoolID, in.StudentUserID)
		if bErr == nil && snap != nil {
			totalOutstanding = snap.TotalOutstandingIDR
			depositIDR = snap.DepositIDR
			if raw, err := json.Marshal(snap); err == nil {
				snapJSON = raw
			}
		}
	}
	if len(snapJSON) == 0 {
		snapJSON = json.RawMessage("{}")
	}

	attachBytes, _ := json.Marshal(in.Attachments)
	if len(attachBytes) == 0 {
		attachBytes = []byte("[]")
	}

	var toClassPtr *string
	if toClassID != "" {
		toClassPtr = &toClassID
	}
	var toSchoolPtr *string
	if toSchoolID != "" {
		toSchoolPtr = &toSchoolID
	}

	req := &model.StudentStatusRequest{
		SchoolID:        schoolID,
		StudentUserID:   in.StudentUserID,
		Kind:            in.Kind,
		FromClassroomID: &fromClassroomID,
		ToClassroomID:   toClassPtr,
		ToSchoolID:      toSchoolPtr,
		EffectiveDate:   in.EffectiveDate,
		Reason:          in.Reason,
		Attachments:     attachBytes,
		FinanceSnapshot: snapJSON,
		RefundAmountIDR: in.RefundAmountIDR,
		Status:          model.StatusPendingApproval,
		RequestedBy:     claims.UserID,
		IdempotencyKey:  in.IdempotencyKey,
	}

	created, err := h.Store.CreateStudentStatusRequest(r.Context(), req)
	if errors.Is(err, store.ErrActiveStatusRequestExists) {
		if active, _ := h.Store.FindActiveStatusRequest(r.Context(), schoolID, in.StudentUserID); active != nil {
			writeActiveRequestConflict(w, active)
			return
		}
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create status request: "+err.Error())
		return
	}

	// 5. Submit to core approval (FR-78-003, AC-78-01)
	requestType := "student_transfer"
	if in.Kind == model.KindWithdrawal {
		requestType = "student_withdrawal"
	}
	var flags []string
	if totalOutstanding > 0 {
		flags = append(flags, "has_outstanding")
	}
	if depositIDR > 0 {
		flags = append(flags, "has_deposit")
	}

	summaryMap := map[string]any{
		"student_user_id":       in.StudentUserID,
		"kind":                  in.Kind,
		"effective_date":        in.EffectiveDate,
		"reason":                in.Reason,
		"total_outstanding_idr": totalOutstanding,
	}
	if in.RefundAmountIDR != nil {
		summaryMap["refund_amount_idr"] = *in.RefundAmountIDR
	}

	callbackURL := h.Core.CallbackBaseURL + "/internal/student-status-requests/approval-callback"
	coreReq := client.CoreApprovalRequest{
		SchoolID:       schoolID,
		RequestType:    requestType,
		SourceService:  "academic",
		SourceID:       created.ID,
		Title:          "Pengajuan " + in.Kind + " siswa: " + in.StudentUserID,
		Summary:        summaryMap,
		AmountIDR:      &totalOutstanding,
		Flags:          flags,
		MakerUserID:    claims.UserID,
		SubjectUserID:  &in.StudentUserID,
		CallbackURL:    callbackURL,
		CorrelationID:  created.ID,
		IdempotencyKey: "academic:status-request:" + created.ID,
	}
	apprResp, cErr := h.Core.SubmitApproval(r.Context(), coreReq)
	if cErr == nil && (apprResp == nil || apprResp.ID == "") {
		cErr = errors.New("core approval returned no id")
	}
	if cErr != nil {
		// Fail closed: a request the Approval Center never saw would stay
		// pending_approval forever. Drop it and tell the user why.
		log.Printf("student status request %s: submit approval: %v", created.ID, cErr)
		if dErr := h.Store.DeleteUnsubmittedStatusRequest(r.Context(), schoolID, created.ID); dErr != nil {
			log.Printf("student status request %s: rollback after failed submit: %v", created.ID, dErr)
		}
		status, msg := approvalSubmitError(requestType, cErr)
		common.WriteError(w, status, msg)
		return
	}
	if err := h.Store.UpdateStatusRequestApproval(r.Context(), schoolID, created.ID, created.Status, apprResp.ID); err != nil {
		log.Printf("student status request %s: store approval id %s: %v", created.ID, apprResp.ID, err)
	} else {
		created.ApprovalID = &apprResp.ID
	}

	_ = h.Core.Audit(r.Context(), client.CoreAuditEvent{
		SchoolID:    schoolID,
		Service:     "academic",
		Module:      "transfer",
		EntityType:  "student_status_request",
		EntityID:    created.ID,
		Action:      "request_created",
		ActorUserID: claims.UserID,
		ActorRole:   claims.ActiveRole,
		After:       summaryMap,
		Reason:      in.Reason,
	})

	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// approvalSubmitError maps a failed core submit to the user-facing answer.
// Core 4xx = configuration/validation problem at the school (most often no
// approval flow for the request type) -> 422 with core's message; anything
// else (network, 5xx, bad internal key -> 401/403) -> 502.
func approvalSubmitError(requestType string, err error) (int, string) {
	var ce *client.CoreError
	if errors.As(err, &ce) && ce.Status >= 400 && ce.Status < 500 &&
		ce.Status != http.StatusUnauthorized && ce.Status != http.StatusForbidden {
		msg := ce.Message
		if msg == "" {
			msg = http.StatusText(ce.Status)
		}
		return http.StatusUnprocessableEntity, fmt.Sprintf(
			"Pengajuan tidak dapat dikirim ke Approval Center: %s. Admin sekolah perlu mengatur alur approval %q di menu Alur Approval, lalu ajukan ulang.",
			msg, requestType)
	}
	return http.StatusBadGateway, "Approval Center tidak dapat dihubungi atau menolak kredensial layanan; pengajuan tidak disimpan. Coba lagi atau hubungi admin TI."
}

// CancelStudentStatusRequest is POST /student-status-requests/{id}/cancel:
// the requester withdraws a request that is still pending_approval
// (BUG-057). The core approval is cancelled first, so an approver can no
// longer decide it.
func (h *Handler) CancelStudentStatusRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireMenu78Role(w, r, model.CanCreateStatusRequest)
	if !ok {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "Dibatalkan oleh pengaju"
	}

	req, err := h.Store.GetStudentStatusRequest(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch request: "+err.Error())
		return
	}
	if req == nil {
		common.WriteError(w, http.StatusNotFound, "request not found")
		return
	}
	switch err := model.CheckCancelStatusRequest(req.Status, req.RequestedBy, claims.UserID); {
	case errors.Is(err, model.ErrCancelNotRequester):
		common.WriteError(w, http.StatusForbidden, err.Error())
		return
	case err != nil:
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	}

	if req.ApprovalID != nil && *req.ApprovalID != "" {
		if h.Core == nil {
			common.WriteError(w, http.StatusServiceUnavailable, "Approval Center belum terhubung; pembatalan tidak dapat diproses")
			return
		}
		if cErr := h.Core.CancelApproval(r.Context(), *req.ApprovalID, claims.UserID, reason); cErr != nil {
			var ce *client.CoreError
			switch {
			case errors.As(cErr, &ce) && ce.Status == http.StatusConflict:
				common.WriteError(w, http.StatusConflict, "pengajuan sudah diputuskan di Approval Center dan tidak dapat dibatalkan")
				return
			case errors.As(cErr, &ce) && ce.Status == http.StatusNotFound:
				// Core has no such approval: nothing to withdraw there.
			default:
				log.Printf("student status request %s: cancel approval %s: %v", req.ID, *req.ApprovalID, cErr)
				common.WriteError(w, http.StatusBadGateway, "Approval Center tidak dapat dihubungi; pembatalan tidak diproses. Coba lagi.")
				return
			}
		}
	}

	moved, err := h.Store.TransitionStatusRequest(r.Context(), schoolID, req.ID, model.StatusPendingApproval, model.StatusCancelled, "")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to cancel request: "+err.Error())
		return
	}
	if !moved {
		common.WriteError(w, http.StatusConflict, model.ErrCancelNotPending.Error())
		return
	}
	if h.Core != nil {
		_ = h.Core.Audit(r.Context(), client.CoreAuditEvent{
			SchoolID:    req.SchoolID,
			Service:     "academic",
			Module:      "transfer",
			EntityType:  "student_status_request",
			EntityID:    req.ID,
			Action:      "request_cancelled",
			ActorUserID: claims.UserID,
			ActorRole:   claims.ActiveRole,
			Before:      map[string]any{"status": req.Status},
			After:       map[string]any{"status": model.StatusCancelled},
			Reason:      reason,
		})
	}
	req.Status = model.StatusCancelled
	common.WriteJSON(w, http.StatusOK, req, nil)
}

// ListStudentStatusRequests is GET /student-status-requests (admin, walikelas, kepala_sekolah).
func (h *Handler) ListStudentStatusRequests(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireMenu78Role(w, r, model.CanViewStatusRequests); !ok {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}

	kind := r.URL.Query().Get("kind")
	status := r.URL.Query().Get("status")
	studentUserID := r.URL.Query().Get("student_user_id")

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	perPage := 20
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}
	offset := (page - 1) * perPage

	list, total, err := h.Store.ListStudentStatusRequests(r.Context(), schoolID, kind, status, studentUserID, perPage, offset)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list status requests: "+err.Error())
		return
	}

	meta := map[string]any{
		"page":     page,
		"per_page": perPage,
		"total":    total,
	}
	common.WriteJSON(w, http.StatusOK, list, meta)
}

// GetStudentStatusRequest is GET /student-status-requests/{id}.
func (h *Handler) GetStudentStatusRequest(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireMenu78Role(w, r, model.CanViewStatusRequests); !ok {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	req, err := h.Store.GetStudentStatusRequest(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch status request: "+err.Error())
		return
	}
	if req == nil {
		common.WriteError(w, http.StatusNotFound, "status request not found")
		return
	}

	common.WriteJSON(w, http.StatusOK, req, nil)
}

// ApprovalCallback handles callbacks from core approval service.
// POST /internal/student-status-requests/approval-callback
func (h *Handler) ApprovalCallback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ApprovalID  string `json:"approval_id"`
		SourceID    string `json:"source_id"`
		RequestType string `json:"request_type"`
		Decision    string `json:"decision"` // approved | rejected
		DecidedBy   string `json:"decided_by"`
		Comment     string `json:"comment"`
		DecidedAt   string `json:"decided_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid callback payload")
		return
	}

	req, err := h.Store.GetStudentStatusRequest(r.Context(), "", in.SourceID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to find request: "+err.Error())
		return
	}
	if req == nil {
		common.WriteError(w, http.StatusNotFound, "request not found")
		return
	}

	var target string
	switch in.Decision {
	case "rejected":
		target = model.StatusRejected
	case "approved":
		target = model.StatusApproved
	default:
		common.WriteError(w, http.StatusUnprocessableEntity, "unrecognized decision: "+in.Decision)
		return
	}

	// Replays of the same decision are acknowledged; any other transition
	// (e.g. a decision arriving after the requester cancelled) is refused.
	if req.Status == target || (target == model.StatusApproved && req.Status == model.StatusApplied) {
		common.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Status}, nil)
		return
	}
	if !model.IsValidStatusTransition(req.Status, target) {
		common.WriteError(w, http.StatusConflict, "status pengajuan "+req.Status+" tidak dapat diubah menjadi "+target)
		return
	}
	moved, err := h.Store.TransitionStatusRequest(r.Context(), req.SchoolID, req.ID, req.Status, target, in.ApprovalID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update request: "+err.Error())
		return
	}
	if !moved {
		common.WriteError(w, http.StatusConflict, "status pengajuan berubah bersamaan; keputusan tidak diterapkan")
		return
	}
	req.Status = target
	if in.ApprovalID != "" {
		req.ApprovalID = &in.ApprovalID
	}

	if target == model.StatusApproved {
		// Check if effective_date <= today in Asia/Jakarta
		applyNow, _ := model.ShouldApplyImmediately(req.EffectiveDate, time.Now(), nil)
		if applyNow {
			if err := h.applyRequest(r, req); err != nil {
				common.WriteError(w, http.StatusInternalServerError, "failed to apply request: "+err.Error())
				return
			}
		}
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Status}, nil)
}

// ApplyStudentStatusRequest is POST /student-status-requests/{id}/apply (admin).
func (h *Handler) ApplyStudentStatusRequest(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireMenu78Role(w, r, model.CanApplyStatusRequest); !ok {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	req, err := h.Store.GetStudentStatusRequest(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch request: "+err.Error())
		return
	}
	if req == nil {
		common.WriteError(w, http.StatusNotFound, "request not found")
		return
	}

	if req.Status != model.StatusApproved {
		common.WriteError(w, http.StatusUnprocessableEntity, "hanya pengajuan yang sudah disetujui (approved) yang dapat diterapkan")
		return
	}

	if err := h.applyRequest(r, req); err != nil {
		if writeCapacityError(w, err) {
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "failed to apply request: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "applied", "applied_at": req.AppliedAt.Format(time.RFC3339)}, nil)
}

// ApplyPendingRequestsJob is POST /internal/student-status-requests/apply-pending.
func (h *Handler) ApplyPendingRequestsJob(w http.ResponseWriter, r *http.Request) {
	schoolID := r.URL.Query().Get("school_id")
	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	today := time.Now().In(loc).Format("2006-01-02")

	list, err := h.Store.ListApprovedPendingRequests(r.Context(), schoolID, today)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list approved pending requests: "+err.Error())
		return
	}

	appliedCount := 0
	for i := range list {
		if err := h.applyRequest(r, &list[i]); err == nil {
			appliedCount++
		}
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"applied_count": appliedCount,
		"total_pending": len(list),
	}, nil)
}

// enrollmentChangeFor tags the class_enrollment_history rows written while
// applying a Menu 78 request with the request id, its reason, and the user
// applying it (empty for the internal apply-pending job / approval callback).
func enrollmentChangeFor(r *http.Request, req *model.StudentStatusRequest) store.EnrollmentChange {
	return store.EnrollmentChange{RequestID: req.ID, Reason: req.Reason, ActorUserID: actorUserID(r)}
}

// applyRequest executes the actual status change across academic, billing, and identity.
func (h *Handler) applyRequest(r *http.Request, req *model.StudentStatusRequest) error {
	ctx := r.Context()
	// Idempotency: if already applied, do nothing
	if req.AppliedAt != nil || req.Status == model.StatusApplied {
		return nil
	}

	settings, _ := h.Store.GetAcademicSettings(ctx, req.SchoolID)
	policy := model.PolicyVoid
	if settings != nil && settings.PostExitBillPolicy != "" {
		policy = settings.PostExitBillPolicy
	}

	switch req.Kind {
	case model.KindClassTransfer:
		fromClass := ""
		if req.FromClassroomID != nil {
			fromClass = *req.FromClassroomID
		}
		toClass := ""
		if req.ToClassroomID != nil {
			toClass = *req.ToClassroomID
		}
		if err := h.Store.ApplyClassTransfer(ctx, req.SchoolID, req.StudentUserID, fromClass, toClass, enrollmentChangeFor(r, req)); err != nil {
			return err
		}

	case model.KindUnitTransfer:
		// AC-78-02: mark old enrollment transferred_out, link student to to_school_id in identity, notify billing
		if err := h.Store.ApplyUnitTransfer(ctx, req.SchoolID, req.StudentUserID, enrollmentChangeFor(r, req)); err != nil {
			return err
		}
		if req.ToSchoolID != nil && *req.ToSchoolID != "" && h.Identity != nil {
			_ = h.Identity.CreateOrLinkStudent(*req.ToSchoolID, req.StudentUserID, "", "")
		}
		if h.Billing != nil {
			_ = h.Billing.ExitStudent(ctx, req.SchoolID, req.StudentUserID, req.EffectiveDate, policy)
		}

	case model.KindWithdrawal:
		// Mark enrollment withdrawn (BR-STU-01, FR-78-005)
		if err := h.Store.ApplyWithdrawal(ctx, req.SchoolID, req.StudentUserID, enrollmentChangeFor(r, req)); err != nil {
			return err
		}
		if h.Billing != nil {
			_ = h.Billing.ExitStudent(ctx, req.SchoolID, req.StudentUserID, req.EffectiveDate, policy)
			// Refund if refund_amount_idr > 0 (FR-78-006, AC-78-03)
			if req.RefundAmountIDR != nil && *req.RefundAmountIDR > 0 {
				_ = h.Billing.RefundStudent(ctx, req.SchoolID, req.StudentUserID, *req.RefundAmountIDR, "Refund penarikan siswa", "withdrawal:"+req.ID)
			}
		}
		if h.Identity != nil {
			// Access deactivation (FR-78-007)
			_ = h.Identity.DeactivateUser(req.SchoolID, req.StudentUserID, "Siswa keluar (withdrawal)")
		}
	}

	if err := h.Store.MarkStatusRequestApplied(ctx, req.SchoolID, req.ID); err != nil {
		return err
	}
	now := time.Now()
	req.Status = model.StatusApplied
	req.AppliedAt = &now

	if h.Core != nil {
		claims := common.FromCtx(r)
		actorID := ""
		actorRole := ""
		if claims != nil {
			actorID = claims.UserID
			actorRole = claims.ActiveRole
		}
		_ = h.Core.Audit(ctx, client.CoreAuditEvent{
			SchoolID:    req.SchoolID,
			Service:     "academic",
			Module:      "transfer",
			EntityType:  "student_status_request",
			EntityID:    req.ID,
			Action:      "request_applied",
			ActorUserID: actorID,
			ActorRole:   actorRole,
			After: map[string]any{
				"status":     model.StatusApplied,
				"applied_at": now.Format(time.RFC3339),
			},
		})
	}

	return nil
}

// GetStudentFinancialSummaryProxy is GET /student-status-requests/financial-summary?student_user_id=.
// Helper for frontend form to display financial snapshot before submitting (AC-78-01).
func (h *Handler) GetStudentFinancialSummaryProxy(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireMenu78Role(w, r, model.CanViewStatusRequests); !ok {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	studentUserID := strings.TrimSpace(r.URL.Query().Get("student_user_id"))
	if studentUserID == "" {
		common.WriteError(w, http.StatusBadRequest, "student_user_id is required")
		return
	}

	if h.Billing == nil {
		common.WriteJSON(w, http.StatusOK, map[string]any{
			"student_user_id":       studentUserID,
			"school_id":             schoolID,
			"total_outstanding_idr": 0,
			"open_bills_idr":        0,
			"unmatured_bills_idr":   0,
			"deposit_idr":           0,
			"refundable_idr":        0,
			"bills":                 []any{},
		}, nil)
		return
	}

	snap, err := h.Billing.GetStudentFinancialSummary(r.Context(), schoolID, studentUserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch financial summary from billing: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, snap, nil)
}

// GetAcademicSettings is GET /academic-settings (admin).
func (h *Handler) GetAcademicSettings(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}

	settings, err := h.Store.GetAcademicSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get academic settings: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, settings, nil)
}

// UpdateAcademicSettings is PUT /academic-settings (admin).
func (h *Handler) UpdateAcademicSettings(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}

	var in struct {
		PostExitBillPolicy string `json:"post_exit_bill_policy"`
		ParentGraceDays    int    `json:"parent_grace_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	settings := model.AcademicSettings{
		SchoolID:           schoolID,
		PostExitBillPolicy: in.PostExitBillPolicy,
		ParentGraceDays:    in.ParentGraceDays,
	}

	if err := h.Store.UpsertAcademicSettings(r.Context(), settings); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save academic settings: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, settings, nil)
}
