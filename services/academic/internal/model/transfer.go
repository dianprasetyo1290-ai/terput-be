package model

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	KindClassTransfer = "class_transfer"
	KindUnitTransfer  = "unit_transfer"
	KindWithdrawal    = "withdrawal"

	StatusPendingApproval = "pending_approval"
	StatusApproved        = "approved"
	StatusRejected        = "rejected"
	StatusCancelled       = "cancelled"
	StatusApplied         = "applied"

	PolicyVoid = "void"
	PolicyKeep = "keep"
)

type AttachmentItem struct {
	Name      string `json:"name"`
	ObjectKey string `json:"object_key"`
	Mime      string `json:"mime,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type FinanceSnapshot struct {
	StudentUserID       string           `json:"student_user_id"`
	SchoolID            string           `json:"school_id"`
	TotalOutstandingIDR int64            `json:"total_outstanding_idr"`
	OpenBillsIDR        int64            `json:"open_bills_idr"`
	UnmaturedBillsIDR   int64            `json:"unmatured_bills_idr"`
	DepositIDR          int64            `json:"deposit_idr"`
	RefundableIDR       int64            `json:"refundable_idr"`
	Bills               []map[string]any `json:"bills,omitempty"`
}

type StudentStatusRequest struct {
	ID                string          `json:"id"`
	SchoolID          string          `json:"school_id"`
	StudentUserID     string          `json:"student_user_id"`
	Kind              string          `json:"kind"`
	FromClassroomID   *string         `json:"from_classroom_id,omitempty"`
	ToClassroomID     *string         `json:"to_classroom_id,omitempty"`
	ToSchoolID        *string         `json:"to_school_id,omitempty"`
	EffectiveDate     string          `json:"effective_date"` // YYYY-MM-DD
	Reason            string          `json:"reason"`
	Attachments       json.RawMessage `json:"attachments"`
	FinanceSnapshot   json.RawMessage `json:"finance_snapshot,omitempty"`
	RefundAmountIDR   *int64          `json:"refund_amount_idr,omitempty"`
	Status            string          `json:"status"`
	ApprovalID        *string         `json:"approval_id,omitempty"`
	AppliedAt         *time.Time      `json:"applied_at,omitempty"`
	RequestedBy       string          `json:"requested_by"`
	IdempotencyKey    *string         `json:"idempotency_key,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type AcademicSettings struct {
	SchoolID           string    `json:"school_id"`
	PostExitBillPolicy string    `json:"post_exit_bill_policy"`
	ParentGraceDays    int       `json:"parent_grace_days"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ValidateCreateStatusRequest validates parameters for initiating a student status request.
func ValidateCreateStatusRequest(kind, reason, fromClassroomID, toClassroomID, toSchoolID, schoolID, effectiveDate string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("alasan pengajuan wajib diisi")
	}

	if strings.TrimSpace(effectiveDate) == "" {
		return errors.New("tanggal efektif wajib diisi")
	}
	if _, err := time.Parse("2006-01-02", effectiveDate); err != nil {
		return errors.New("format tanggal efektif harus YYYY-MM-DD")
	}

	switch kind {
	case KindClassTransfer:
		if strings.TrimSpace(fromClassroomID) == "" {
			return errors.New("siswa belum terdaftar di kelas aktif")
		}
		if strings.TrimSpace(toClassroomID) == "" {
			return errors.New("kelas tujuan wajib dipilih untuk pindah kelas")
		}
		if toClassroomID == fromClassroomID {
			return errors.New("kelas tujuan harus berbeda dari kelas asal")
		}
	case KindUnitTransfer:
		if strings.TrimSpace(toSchoolID) == "" {
			return errors.New("sekolah/unit tujuan wajib dipilih untuk pindah unit")
		}
		if toSchoolID == schoolID {
			return errors.New("unit tujuan harus berbeda dari unit asal")
		}
	case KindWithdrawal:
		if strings.TrimSpace(fromClassroomID) == "" {
			return errors.New("siswa belum terdaftar di kelas aktif")
		}
	default:
		return errors.New("jenis pengajuan tidak valid (harus class_transfer, unit_transfer, atau withdrawal)")
	}

	return nil
}

// IsValidStatusTransition validates state transitions for student status requests.
func IsValidStatusTransition(current, target string) bool {
	switch current {
	case StatusPendingApproval:
		return target == StatusApproved || target == StatusRejected || target == StatusCancelled
	case StatusApproved:
		return target == StatusApplied
	default:
		return false
	}
}

// DetermineApprovalFlags extracts approval amount and flags from a financial snapshot.
func DetermineApprovalFlags(snapshot *FinanceSnapshot) (int64, []string) {
	if snapshot == nil {
		return 0, []string{}
	}
	amount := snapshot.TotalOutstandingIDR
	var flags []string
	if snapshot.TotalOutstandingIDR > 0 {
		flags = append(flags, "has_outstanding")
	}
	if snapshot.DepositIDR > 0 {
		flags = append(flags, "has_deposit")
	}
	return amount, flags
}

// ShouldApplyImmediately reports whether the effective date is today or in the past (Asia/Jakarta).
func ShouldApplyImmediately(effectiveDate string, now time.Time, loc *time.Location) (bool, error) {
	if loc == nil {
		var err error
		loc, err = time.LoadLocation("Asia/Jakarta")
		if err != nil {
			loc = time.FixedZone("WIB", 7*3600)
		}
	}
	eff, err := time.ParseInLocation("2006-01-02", effectiveDate, loc)
	if err != nil {
		return false, err
	}
	nowInLoc := now.In(loc)
	today := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), 0, 0, 0, 0, loc)

	return !eff.After(today), nil
}
