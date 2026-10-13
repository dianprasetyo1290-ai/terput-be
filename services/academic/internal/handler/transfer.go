package handler

import (
	"encoding/json"
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

// CreateStudentStatusRequest is POST /student-status-requests (admin, superadmin, walikelas).
func (h *Handler) CreateStudentStatusRequest(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if claims.ActiveRole != "admin" && claims.ActiveRole != common.RoleSuperadmin && claims.ActiveRole != "walikelas" {
		common.WriteError(w, http.StatusForbidden, "forbidden: role not permitted to initiate status requests")
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

	if h.Core != nil {
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
		if cErr != nil {
			log.Printf("student status request %s: submit approval: %v", created.ID, cErr)
		}
		if cErr == nil && apprResp != nil && apprResp.ID != "" {
			_ = h.Store.UpdateStatusRequestApproval(r.Context(), schoolID, created.ID, created.Status, apprResp.ID)
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
	}

	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// ListStudentStatusRequests is GET /student-status-requests (admin, superadmin, walikelas, kepala_sekolah).
func (h *Handler) ListStudentStatusRequests(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
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
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
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

	if in.Decision == "rejected" {
		if err := h.Store.UpdateStatusRequestApproval(r.Context(), req.SchoolID, req.ID, model.StatusRejected, in.ApprovalID); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to update rejection: "+err.Error())
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"status": "rejected"}, nil)
		return
	}

	if in.Decision == "approved" {
		if err := h.Store.UpdateStatusRequestApproval(r.Context(), req.SchoolID, req.ID, model.StatusApproved, in.ApprovalID); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to update approval: "+err.Error())
			return
		}
		req.Status = model.StatusApproved
		req.ApprovalID = &in.ApprovalID

		// Check if effective_date <= today in Asia/Jakarta
		applyNow, _ := model.ShouldApplyImmediately(req.EffectiveDate, time.Now(), nil)
		if applyNow {
			if err := h.applyRequest(r, req); err != nil {
				common.WriteError(w, http.StatusInternalServerError, "failed to apply request: "+err.Error())
				return
			}
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Status}, nil)
		return
	}

	common.WriteError(w, http.StatusUnprocessableEntity, "unrecognized decision: "+in.Decision)
}

// ApplyStudentStatusRequest is POST /student-status-requests/{id}/apply (admin/superadmin).
func (h *Handler) ApplyStudentStatusRequest(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
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
