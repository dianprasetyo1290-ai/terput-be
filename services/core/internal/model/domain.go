package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Request types known to the core system.
const (
	ReqTypeDiscountSpecial        = "discount_special"
	ReqTypeFeeChange              = "fee_change"
	ReqTypeBillAdjustment         = "bill_adjustment"
	ReqTypePaymentCorrection      = "payment_correction"
	ReqTypeScholarship            = "scholarship"
	ReqTypeRefund                 = "refund"
	ReqTypeJournal                = "journal"
	ReqTypeReconAdjustment        = "recon_adjustment"
	ReqTypeSelectionResult        = "selection_result"
	ReqTypeStudentTransfer        = "student_transfer"
	ReqTypeStudentWithdrawal      = "student_withdrawal"
	ReqTypeLeave                  = "leave"
	ReqTypeOvertime               = "overtime"
	ReqTypeAttendanceCorrection   = "attendance_correction"
	ReqTypeProfileChange          = "profile_change"
	ReqTypeRequisition            = "requisition"
	ReqTypeOffer                  = "offer"
	ReqTypePayrollComponentChange = "payroll_component_change"
	ReqTypePayrollRun             = "payroll_run"
	ReqTypePayrollInputChange     = "payroll_input_change"
	ReqTypeTrainingPaid           = "training_paid"
	ReqTypeRoomBooking            = "room_booking"
	ReqTypeHealthProfileUpdate    = "health_profile_update"
	ReqTypeCanteenSettlement      = "canteen_settlement"
	ReqTypeWalletTransfer         = "wallet_transfer"
	ReqTypePurchaseRequest        = "purchase_request"
	ReqTypePurchaseOrder          = "purchase_order"
	ReqTypeStockAdjustment        = "stock_adjustment"
	ReqTypeVendorBankChange       = "vendor_bank_change"
	ReqTypeDisciplineAppeal       = "discipline_appeal"
	ReqTypeEvent                  = "event"
	ReqTypeAssetDisposal          = "asset_disposal"
	ReqTypeAssetTransfer          = "asset_transfer"
	ReqTypeBudget                 = "budget"
	ReqTypeVendorInvoiceOverride  = "vendor_invoice_override"
	ReqTypeVendorPayment          = "vendor_payment"
	ReqTypeQASelfEvalFinalize     = "qa_self_eval_finalize"
	ReqTypeQASelfEvalReopen       = "qa_self_eval_reopen"
	ReqTypeRegulatoryReport       = "regulatory_report"
	// ReqTypeRoleConflictException: identity asks to grant a role that
	// conflicts with one the user already holds (SoD exception). Core calls
	// identity back at /internal/role-conflict-exceptions/approval-callback.
	ReqTypeRoleConflictException = "role_conflict_exception"
)

// RequestTypeLabels maps request_type to its human-readable Indonesian title.
var RequestTypeLabels = map[string]string{
	ReqTypeDiscountSpecial:        "Diskon Khusus",
	ReqTypeFeeChange:              "Perubahan Tarif",
	ReqTypeBillAdjustment:         "Penyesuaian Tagihan",
	ReqTypePaymentCorrection:      "Koreksi Pembayaran",
	ReqTypeScholarship:            "Beasiswa",
	ReqTypeRefund:                 "Refund",
	ReqTypeJournal:                "Penyesuaian Jurnal",
	ReqTypeReconAdjustment:        "Penyesuaian Rekonsiliasi",
	ReqTypeSelectionResult:        "Hasil Seleksi PPDB",
	ReqTypeStudentTransfer:        "Mutasi Siswa",
	ReqTypeStudentWithdrawal:      "Pengunduran Diri Siswa",
	ReqTypeLeave:                  "Pengajuan Cuti",
	ReqTypeOvertime:               "Lembur",
	ReqTypeAttendanceCorrection:   "Koreksi Presensi",
	ReqTypeProfileChange:          "Perubahan Profil Pegawai",
	ReqTypeRequisition:            "Permintaan Tenaga Kerja (Requisition)",
	ReqTypeOffer:                  "Penawaran Kerja (Offer)",
	ReqTypePayrollComponentChange: "Perubahan Komponen Gaji",
	ReqTypePayrollRun:             "Finalisasi Payroll",
	ReqTypePayrollInputChange:     "Perubahan Input Payroll",
	ReqTypeTrainingPaid:           "Pelatihan Berbayar",
	ReqTypeRoomBooking:            "Peminjaman Ruang",
	ReqTypeHealthProfileUpdate:    "Pembaruan Profil Kesehatan",
	ReqTypeCanteenSettlement:      "Pencairan Dana Kantin",
	ReqTypeWalletTransfer:         "Transfer Saldo Dompet",
	ReqTypePurchaseRequest:        "Permintaan Pembelian (PR)",
	ReqTypePurchaseOrder:          "Pesanan Pembelian (PO)",
	ReqTypeStockAdjustment:        "Penyesuaian Stok (Stock Opname)",
	ReqTypeVendorBankChange:       "Perubahan Rekening Vendor",
	ReqTypeDisciplineAppeal:       "Banding Disiplin Siswa",
	ReqTypeEvent:                  "Kegiatan/Event Sekolah",
	ReqTypeAssetDisposal:          "Pelepasan Aset",
	ReqTypeAssetTransfer:          "Mutasi Aset Antar-Unit",
	ReqTypeBudget:                 "Anggaran / RAPBS",
	ReqTypeVendorInvoiceOverride:  "Dispensasi Faktur Vendor (Override Match)",
	ReqTypeVendorPayment:          "Jadwal Pembayaran Vendor",
	ReqTypeQASelfEvalFinalize:     "Finalisasi Evaluasi Diri",
	ReqTypeQASelfEvalReopen:       "Pembukaan Kembali Evaluasi Diri",
	ReqTypeRegulatoryReport:       "Persetujuan Laporan Regulasi",
	ReqTypeRoleConflictException:  "Pengecualian Konflik Peran (SoD)",
}

// KnownRequestTypes lists all supported request types.
var KnownRequestTypes = []string{
	ReqTypeDiscountSpecial,
	ReqTypeFeeChange,
	ReqTypeBillAdjustment,
	ReqTypePaymentCorrection,
	ReqTypeScholarship,
	ReqTypeRefund,
	ReqTypeJournal,
	ReqTypeReconAdjustment,
	ReqTypeSelectionResult,
	ReqTypeStudentTransfer,
	ReqTypeStudentWithdrawal,
	ReqTypeLeave,
	ReqTypeOvertime,
	ReqTypeAttendanceCorrection,
	ReqTypeProfileChange,
	ReqTypeRequisition,
	ReqTypeOffer,
	ReqTypePayrollComponentChange,
	ReqTypePayrollRun,
	ReqTypePayrollInputChange,
	ReqTypeTrainingPaid,
	ReqTypeRoomBooking,
	ReqTypeHealthProfileUpdate,
	ReqTypeCanteenSettlement,
	ReqTypeWalletTransfer,
	ReqTypePurchaseRequest,
	ReqTypePurchaseOrder,
	ReqTypeStockAdjustment,
	ReqTypeVendorBankChange,
	ReqTypeDisciplineAppeal,
	ReqTypeEvent,
	ReqTypeAssetDisposal,
	ReqTypeAssetTransfer,
	ReqTypeBudget,
	ReqTypeVendorInvoiceOverride,
	ReqTypeVendorPayment,
	ReqTypeQASelfEvalFinalize,
	ReqTypeQASelfEvalReopen,
	ReqTypeRegulatoryReport,
	ReqTypeRoleConflictException,
}

// RequestTypeSources pins request types that grant privileges to the only
// service allowed to submit them. A role_conflict_exception approval makes
// identity grant a conflicting role, so no other service may raise one.
var RequestTypeSources = map[string]string{
	ReqTypeRoleConflictException: "identity",
}

// SourceAllowed reports whether sourceService may submit requestType.
func SourceAllowed(requestType, sourceService string) bool {
	want, pinned := RequestTypeSources[requestType]
	return !pinned || want == sourceService
}

// Request statuses
const (
	RequestStatusPending   = "pending"
	RequestStatusApproved  = "approved"
	RequestStatusRejected  = "rejected"
	RequestStatusCancelled = "cancelled"
)

// Step statuses
const (
	StepStatusWaiting   = "waiting"
	StepStatusPending   = "pending"
	StepStatusApproved  = "approved"
	StepStatusRejected  = "rejected"
	StepStatusSkipped   = "skipped"
	StepStatusEscalated = "escalated"
)

// Approver kinds
const (
	ApproverKindRole             = "role"
	ApproverKindDirectSupervisor = "direct_supervisor"
	ApproverKindUser             = "user"
)

// Exception kinds
const (
	ExceptionKindWebhook        = "webhook"
	ExceptionKindJournalPosting = "journal_posting"
	ExceptionKindNotification   = "notification"
	ExceptionKindDeadLetter     = "dead_letter"
	ExceptionKindImport         = "import"
	ExceptionKindCallback       = "callback"
)

// Exception severity
const (
	SeverityNormal   = "normal"
	SeverityCritical = "critical"
)

// Exception status
const (
	ExceptionStatusOpen     = "open"
	ExceptionStatusRetrying = "retrying"
	ExceptionStatusResolved = "resolved"
)

// Sensitive keys to mask for audit and exception views
var SensitiveKeys = map[string]bool{
	"nik":             true,
	"npwp":            true,
	"account_number":  true,
	"bank_account":    true,
	"phone":           true,
	"email":           true,
	"salary":          true,
	"base_salary_idr": true,
	"gross_idr":       true,
	"net_idr":         true,
}

// Condition flags a flow step may require (BR: AC-88-01 headcount, AC-92-01 salary band).
const (
	FlagExceedsHeadcount = "exceeds_headcount"
	FlagSalaryOutOfRange = "salary_out_of_range"
)

// ConditionFlags is the single list the flow editor and validator accept.
var ConditionFlags = []string{FlagExceedsHeadcount, FlagSalaryOutOfRange}

// ExceptionKinds / Severities are the values accepted by POST /internal/exceptions.
var (
	ExceptionKinds = []string{ExceptionKindWebhook, ExceptionKindJournalPosting, ExceptionKindNotification,
		ExceptionKindDeadLetter, ExceptionKindImport, ExceptionKindCallback}
	Severities = []string{SeverityNormal, SeverityCritical}
)

var ErrNoApplicableSteps = errors.New("tidak ada tahap berlaku")

// FlowStep represents a configured step in an approval flow.
type FlowStep struct {
	ID             string  `json:"id,omitempty"`
	FlowID         string  `json:"flow_id,omitempty"`
	Seq            int     `json:"seq"`
	ApproverKind   string  `json:"approver_kind"`
	ApproverRole   *string `json:"approver_role,omitempty"`
	ApproverUserID *string `json:"approver_user_id,omitempty"`
	MinAmountIDR   *int64  `json:"min_amount_idr,omitempty"`
	ConditionFlag  *string `json:"condition_flag,omitempty"`
	SLAHours       *int    `json:"sla_hours,omitempty"`
	EscalateToRole *string `json:"escalate_to_role,omitempty"`
}

// ApprovalFlow represents the workflow definition.
type ApprovalFlow struct {
	ID          string     `json:"id"`
	SchoolID    string     `json:"school_id"`
	RequestType string     `json:"request_type"`
	Name        string     `json:"name"`
	Active      bool       `json:"active"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	Steps       []FlowStep `json:"steps"`
}

// ApprovalRequest represents an instance of a request submitted for approval.
type ApprovalRequest struct {
	ID               string         `json:"id"`
	SchoolID         string         `json:"school_id"`
	RequestType      string         `json:"request_type"`
	SourceService    string         `json:"source_service"`
	SourceID         string         `json:"source_id"`
	Title            string         `json:"title"`
	Summary          map[string]any `json:"summary"`
	AmountIDR        *int64         `json:"amount_idr,omitempty"`
	Flags            []string       `json:"flags"`
	MakerUserID      string         `json:"maker_user_id"`
	SubjectUserID    *string        `json:"subject_user_id,omitempty"`
	Status           string         `json:"status"`
	CurrentSeq       *int           `json:"current_seq,omitempty"`
	CallbackURL      string         `json:"callback_url"`
	CorrelationID    string         `json:"correlation_id"`
	IdempotencyKey   string         `json:"idempotency_key"`
	FlowSnapshot     []FlowStep     `json:"flow_snapshot"`
	DecidedAt        *time.Time     `json:"decided_at,omitempty"`
	CallbackStatus   string         `json:"callback_status"`
	CallbackAttempts int            `json:"callback_attempts"`
	LastCallbackAt   *time.Time     `json:"last_callback_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	Steps            []ApprovalStep `json:"steps,omitempty"`
	CanDecide        bool           `json:"can_decide"`
}

// ApprovalStep represents an individual step in an approval request.
type ApprovalStep struct {
	ID                  string     `json:"id"`
	SchoolID            string     `json:"school_id"`
	RequestID           string     `json:"request_id"`
	Seq                 int        `json:"seq"`
	ApproverKind        string     `json:"approver_kind"`
	ApproverRole        *string    `json:"approver_role,omitempty"`
	ApproverUserID      *string    `json:"approver_user_id,omitempty"`
	Status              string     `json:"status"`
	SLAHours            *int       `json:"sla_hours,omitempty"`
	EscalateToRole      *string    `json:"escalate_to_role,omitempty"`
	DueAt               *time.Time `json:"due_at,omitempty"`
	DecidedBy           *string    `json:"decided_by,omitempty"`
	DecidedAsDelegateOf *string    `json:"decided_as_delegate_of,omitempty"`
	Comment             *string    `json:"comment,omitempty"`
	DecidedAt           *time.Time `json:"decided_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// ApprovalDelegation records delegation of authority.
type ApprovalDelegation struct {
	ID         string     `json:"id"`
	SchoolID   string     `json:"school_id"`
	FromUserID string     `json:"from_user_id"`
	ToUserID   string     `json:"to_user_id"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidTo    time.Time  `json:"valid_to"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// IntegrationException represents an item in the exception queue.
type IntegrationException struct {
	ID               string         `json:"id"`
	SchoolID         string         `json:"school_id"`
	SourceService    string         `json:"source_service"`
	Kind             string         `json:"kind"`
	Severity         string         `json:"severity"`
	CorrelationID    string         `json:"correlation_id"`
	ErrorMessage     string         `json:"error_message"`
	Attempts         int            `json:"attempts"`
	MaxAttempts      int            `json:"max_attempts"`
	PayloadSummary   map[string]any `json:"payload_summary"`
	RetryURL         *string        `json:"retry_url,omitempty"`
	Status           string         `json:"status"`
	AssigneeUserID   *string        `json:"assignee_user_id,omitempty"`
	ResolvedBy       *string        `json:"resolved_by,omitempty"`
	ResolutionReason *string        `json:"resolution_reason,omitempty"`
	ResolvedAt       *time.Time     `json:"resolved_at,omitempty"`
	IdempotencyKey   string         `json:"idempotency_key"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// AuditEvent represents a centralized audit log entry.
type AuditEvent struct {
	ID            int64     `json:"id"`
	SchoolID      *string   `json:"school_id,omitempty"`
	Service       string    `json:"service"`
	Module        string    `json:"module"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	Action        string    `json:"action"`
	ActorUserID   *string   `json:"actor_user_id,omitempty"`
	ActorRole     *string   `json:"actor_role,omitempty"`
	BeforeData    any       `json:"before_data,omitempty"`
	AfterData     any       `json:"after_data,omitempty"`
	Reason        *string   `json:"reason,omitempty"`
	CorrelationID *string   `json:"correlation_id,omitempty"`
	IPHash        *string   `json:"ip_hash,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ApproverStepInfo holds info needed to determine if a user can decide a step.
type ApproverStepInfo struct {
	Kind         string
	Role         string
	UserID       string
	SupervisorID string
}

// UserInfo holds caller information for decision evaluation.
type UserInfo struct {
	UserID     string
	ActiveRole string
	Roles      []string
}

// ActiveDelegation holds active delegation records for decision evaluation.
type ActiveDelegation struct {
	FromUserID    string
	ToUserID      string
	FromUserRoles []string
}

// ResolveSteps filters and sequences steps that apply to a request.
func ResolveSteps(flowSteps []FlowStep, amountIDR *int64, flags []string) ([]FlowStep, error) {
	flagSet := make(map[string]bool, len(flags))
	for _, f := range flags {
		flagSet[f] = true
	}

	var applicable []FlowStep
	for _, step := range flowSteps {
		// Minimum amount check
		if step.MinAmountIDR != nil {
			if amountIDR == nil || *amountIDR < *step.MinAmountIDR {
				continue
			}
		}
		// Condition flag check
		if step.ConditionFlag != nil && *step.ConditionFlag != "" {
			if !flagSet[*step.ConditionFlag] {
				continue
			}
		}
		applicable = append(applicable, step)
	}

	if len(applicable) == 0 {
		return nil, ErrNoApplicableSteps
	}

	sort.Slice(applicable, func(i, j int) bool {
		return applicable[i].Seq < applicable[j].Seq
	})

	return applicable, nil
}

// CanDecide checks if a user has authority to decide a step, adhering to maker-checker rules.
func CanDecide(makerUserID string, step ApproverStepInfo, user UserInfo, delegations []ActiveDelegation) (bool, *string) {
	// Maker-checker (BR-GEN-09, AC-79-01): The creator cannot approve their own request on any step.
	if user.UserID != "" && user.UserID == makerUserID {
		return false, nil
	}

	userHasRole := func(roles []string, activeRole, requiredRole string) bool {
		if activeRole == requiredRole {
			return true
		}
		for _, r := range roles {
			if r == requiredRole {
				return true
			}
		}
		return false
	}

	// 1. Direct authorization
	switch step.Kind {
	case ApproverKindRole:
		if userHasRole(user.Roles, user.ActiveRole, step.Role) {
			return true, nil
		}
	case ApproverKindUser:
		if user.UserID != "" && user.UserID == step.UserID {
			return true, nil
		}
	case ApproverKindDirectSupervisor:
		if user.UserID != "" && step.SupervisorID != "" && user.UserID == step.SupervisorID {
			return true, nil
		}
	}

	// 2. Authorization via active delegation
	for _, del := range delegations {
		if del.ToUserID != user.UserID {
			continue
		}
		// If the original authority is the maker, delegation cannot bypass maker-checker
		if del.FromUserID == makerUserID {
			continue
		}

		switch step.Kind {
		case ApproverKindRole:
			if userHasRole(del.FromUserRoles, "", step.Role) {
				fromUser := del.FromUserID
				return true, &fromUser
			}
		case ApproverKindUser:
			if del.FromUserID == step.UserID {
				fromUser := del.FromUserID
				return true, &fromUser
			}
		case ApproverKindDirectSupervisor:
			if step.SupervisorID != "" && del.FromUserID == step.SupervisorID {
				fromUser := del.FromUserID
				return true, &fromUser
			}
		}
	}

	return false, nil
}

// MaskSensitive recursively redacts sensitive fields according to FR-80-002 / AC-80-03 / NFR-HR-01.
func MaskSensitive(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = maskValue(strings.ToLower(k), v)
	}
	return out
}

func maskValue(key string, val any) any {
	if val == nil {
		return nil
	}

	// If key is in sensitive list, mask directly
	if SensitiveKeys[key] {
		return applyMask(key, val)
	}

	switch v := val.(type) {
	case map[string]any:
		return MaskSensitive(v)
	case []any:
		res := make([]any, len(v))
		for i, item := range v {
			if m, ok := item.(map[string]any); ok {
				res[i] = MaskSensitive(m)
			} else {
				res[i] = item
			}
		}
		return res
	default:
		return val
	}
}

func applyMask(key string, val any) any {
	// Money or numeric salary fields -> "•••"
	switch key {
	case "salary", "base_salary_idr", "gross_idr", "net_idr":
		return "•••"
	}

	strVal := fmt.Sprintf("%v", val)
	if strVal == "" {
		return ""
	}

	// Email masking: a•••@domain
	if key == "email" || strings.Contains(strVal, "@") {
		parts := strings.SplitN(strVal, "@", 2)
		if len(parts) == 2 {
			local := parts[0]
			domain := parts[1]
			if len(local) > 0 {
				return string(local[0]) + "•••@" + domain
			}
			return "•••@" + domain
		}
	}

	// String masking: last 4 chars visible (••••1234)
	if len(strVal) <= 4 {
		return "••••"
	}
	return "••••" + strVal[len(strVal)-4:]
}

// CurrentStep returns the pending step a request is waiting on, or nil.
func CurrentStep(req *ApprovalRequest) *ApprovalStep {
	if req == nil || req.Status != RequestStatusPending || req.CurrentSeq == nil {
		return nil
	}
	for i := range req.Steps {
		if req.Steps[i].Seq == *req.CurrentSeq {
			return &req.Steps[i]
		}
	}
	return nil
}

// StepApprover converts a stored step into the CanDecide input. For
// direct_supervisor steps the supervisor resolved from HR at creation time is
// stored in approver_user_id.
func StepApprover(st ApprovalStep) ApproverStepInfo {
	info := ApproverStepInfo{Kind: st.ApproverKind}
	if st.ApproverRole != nil {
		info.Role = *st.ApproverRole
	}
	if st.ApproverUserID != nil {
		info.UserID = *st.ApproverUserID
		if st.ApproverKind == ApproverKindDirectSupervisor {
			info.SupervisorID = *st.ApproverUserID
		}
	}
	return info
}

// CanView reports whether a non-admin user may read a request: the maker, the
// subject, anyone named on a step (directly, by role, or as decider), or a
// delegate of such a person.
func CanView(req *ApprovalRequest, user UserInfo, delegations []ActiveDelegation) bool {
	if req == nil || user.UserID == "" {
		return false
	}
	if req.MakerUserID == user.UserID || (req.SubjectUserID != nil && *req.SubjectUserID == user.UserID) {
		return true
	}
	type actor struct {
		id    string
		roles []string
	}
	actors := []actor{{user.UserID, append([]string{user.ActiveRole}, user.Roles...)}}
	for _, d := range delegations {
		if d.ToUserID == user.UserID {
			actors = append(actors, actor{d.FromUserID, d.FromUserRoles})
		}
	}
	for _, st := range req.Steps {
		for _, a := range actors {
			if st.ApproverUserID != nil && *st.ApproverUserID == a.id {
				return true
			}
			if st.DecidedBy != nil && *st.DecidedBy == a.id {
				return true
			}
			if st.ApproverRole != nil {
				for _, r := range a.roles {
					if r == *st.ApproverRole {
						return true
					}
				}
			}
		}
	}
	return false
}

// FinalDecision returns who made the deciding call on a finished request and
// their comment: the rejecting step, or the last approved step.
func FinalDecision(req *ApprovalRequest) (decidedBy, comment string) {
	for _, st := range req.Steps {
		if st.Status != StepStatusApproved && st.Status != StepStatusRejected {
			continue
		}
		if st.DecidedBy != nil {
			decidedBy = *st.DecidedBy
		}
		comment = ""
		if st.Comment != nil {
			comment = *st.Comment
		}
		if st.Status == StepStatusRejected {
			break
		}
	}
	return decidedBy, comment
}

// AuditAccessGrant is a time-boxed window in which an auditor may read the
// audit trail and exception queue of a school (BUG-087, AC-RBAC-10).
type AuditAccessGrant struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	UserID    string     `json:"user_id"`
	ValidFrom time.Time  `json:"valid_from"`
	ValidTo   time.Time  `json:"valid_to"`
	Reason    string     `json:"reason"`
	GrantedBy string     `json:"granted_by"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	RevokedBy *string    `json:"revoked_by,omitempty"`
}

// Active reports whether the grant allows access at now.
func (g AuditAccessGrant) Active(now time.Time) bool {
	return g.RevokedAt == nil && !now.Before(g.ValidFrom) && now.Before(g.ValidTo)
}
