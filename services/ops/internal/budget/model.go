package budget

import (
	"time"
)

type BudgetSettings struct {
	SchoolID             string    `json:"school_id"`
	FiscalYearStartMonth int       `json:"fiscal_year_start_month"` // 1-12 (default 7 = Juli)
	LockRule             string    `json:"lock_rule"`               // "warn", "hard_stop", "approval_required"
	ToleranceBP          int       `json:"tolerance_bp"`            // Basis points, e.g. 500 = 5%
	WarnThresholdBP      int       `json:"warn_threshold_bp"`       // Basis points, e.g. 8000 = 80%
	UpdatedAt            time.Time `json:"updated_at"`
}

type Budget struct {
	ID              string     `json:"id"`
	SchoolID        string     `json:"school_id"`
	Code            string     `json:"code"`
	Name            string     `json:"name"`
	FiscalYear      string     `json:"fiscal_year"`
	PeriodStart     string     `json:"period_start"`
	PeriodEnd       string     `json:"period_end"`
	Status          string     `json:"status"` // "draft", "pending_approval", "approved", "revised", "closed"
	TotalPlannedIDR int64      `json:"total_planned_idr"`
	ApprovalID      *string    `json:"approval_id,omitempty"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Lines           []BudgetLine `json:"lines,omitempty"`
}

type BudgetLine struct {
	ID               string    `json:"id"`
	SchoolID         string    `json:"school_id"`
	BudgetID         string    `json:"budget_id"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Category         string    `json:"category"` // operasional, investasi, personalia, pemeliharaan, lainnya
	AccCode          string    `json:"acc_code"`
	DivCode          *string   `json:"div_code,omitempty"`
	AmountPlannedIDR int64     `json:"amount_planned_idr"`
	ActualIDR        int64     `json:"actual_idr"`
	CommitmentIDR    int64     `json:"commitment_idr"`
	RemainingIDR     int64     `json:"remaining_idr"`
	AbsorptionBP     int       `json:"absorption_bp"` // basis points (10000 = 100%)
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type BudgetAlert struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	BudgetLineID   string     `json:"budget_line_id"`
	BudgetLineCode string     `json:"budget_line_code,omitempty"`
	BudgetLineName string     `json:"budget_line_name,omitempty"`
	ThresholdBP    int        `json:"threshold_bp"`
	CurrentBP      int        `json:"current_bp"`
	Message        string     `json:"message"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy *string    `json:"acknowledged_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type BudgetReport struct {
	Budget           Budget       `json:"budget"`
	Settings         BudgetSettings `json:"settings"`
	TotalPlannedIDR  int64        `json:"total_planned_idr"`
	TotalActualIDR   int64        `json:"total_actual_idr"`
	TotalCommitIDR   int64        `json:"total_commitment_idr"`
	TotalRemainingIDR int64       `json:"total_remaining_idr"`
	TotalAbsorptionBP int         `json:"total_absorption_bp"`
	Lines            []BudgetLine `json:"lines"`
}
