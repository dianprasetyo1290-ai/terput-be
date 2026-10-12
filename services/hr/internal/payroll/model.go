package payroll

import (
	"time"
)

type PayComponent struct {
	ID               string    `json:"id" db:"id"`
	SchoolID         string    `json:"school_id" db:"school_id"`
	Code             string    `json:"code" db:"code"`
	Name             string    `json:"name" db:"name"`
	Kind             string    `json:"kind" db:"kind"`
	Nature           string    `json:"nature" db:"nature"`
	Taxable          bool      `json:"taxable" db:"taxable"`
	BPJSBase         bool      `json:"bpjs_base" db:"bpjs_base"`
	Calc             string    `json:"calc" db:"calc"`
	DefaultAmountIDR *int64    `json:"default_amount_idr,omitempty" db:"default_amount_idr"`
	RatePerUnitIDR   *int64    `json:"rate_per_unit_idr,omitempty" db:"rate_per_unit_idr"`
	MaxInputIDR      *int64    `json:"max_input_idr,omitempty" db:"max_input_idr"`
	Sort             int       `json:"sort" db:"sort"`
	Status           string    `json:"status" db:"status"`
	ApprovalID       *string   `json:"approval_id,omitempty" db:"approval_id"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type PayrollRun struct {
	ID             string     `json:"id" db:"id"`
	SchoolID       string     `json:"school_id" db:"school_id"`
	Period         string     `json:"period" db:"period"`
	Kind           string     `json:"kind" db:"kind"`
	Status         string     `json:"status" db:"status"`
	ApprovalID     *string    `json:"approval_id,omitempty" db:"approval_id"`
	CalculatedAt   *time.Time `json:"calculated_at,omitempty" db:"calculated_at"`
	FinalizedAt    *time.Time `json:"finalized_at,omitempty" db:"finalized_at"`
	FinalizedBy    *string    `json:"finalized_by,omitempty" db:"finalized_by"`
	Totals         string     `json:"totals" db:"totals"`
	IdempotencyKey string     `json:"idempotency_key" db:"idempotency_key"`
	TotalGrossIDR  int64      `json:"total_gross_idr,omitempty"`
	TotalNetIDR    int64      `json:"total_net_idr,omitempty"`
	TotalLines     int        `json:"total_lines,omitempty"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type PayrollLine struct {
	ID                  string    `json:"id" db:"id"`
	SchoolID            string    `json:"school_id" db:"school_id"`
	RunID               string    `json:"run_id" db:"run_id"`
	EmployeeID          string    `json:"employee_id" db:"employee_id"`
	EmployeeName        string    `json:"employee_name,omitempty" db:"employee_name"`
	EmployeeNIP         string    `json:"employee_nip,omitempty" db:"employee_nip"`
	GrossIDR            int64     `json:"gross_idr" db:"gross_idr"`
	TaxableIDR          int64     `json:"taxable_idr" db:"taxable_idr"`
	BPJSBaseIDR         int64     `json:"bpjs_base_idr" db:"bpjs_base_idr"`
	BPJSEmployeeIDR     int64     `json:"bpjs_employee_idr" db:"bpjs_employee_idr"`
	BPJSEmployerIDR     int64     `json:"bpjs_employer_idr" db:"bpjs_employer_idr"`
	PPh21IDR            int64     `json:"pph21_idr" db:"pph21_idr"`
	OtherDeductionsIDR  int64     `json:"other_deductions_idr" db:"other_deductions_idr"`
	NetIDR              int64     `json:"net_idr" db:"net_idr"`
	PrevNetIDR          *int64    `json:"prev_net_idr,omitempty" db:"prev_net_idr"`
	VarianceBP          *int      `json:"variance_bp,omitempty" db:"variance_bp"`
	Flagged             bool      `json:"flagged" db:"flagged"`
	PaymentStatus       string    `json:"payment_status" db:"payment_status"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
}

type PayrollInput struct {
	ID          string     `json:"id" db:"id"`
	SchoolID    string     `json:"school_id" db:"school_id"`
	Period      string     `json:"period" db:"period"`
	EmployeeID  string     `json:"employee_id" db:"employee_id"`
	ComponentID string     `json:"component_id" db:"component_id"`
	Quantity    *float64   `json:"quantity,omitempty" db:"quantity"`
	AmountIDR   int64      `json:"amount_idr" db:"amount_idr"`
	Source      string     `json:"source" db:"source"`
	Locked      bool       `json:"locked" db:"locked"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

type PayrollInputView struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	Period        string    `json:"period"`
	EmployeeID    string    `json:"employee_id"`
	EmployeeName  string    `json:"employee_name"`
	EmployeeNIP   string    `json:"employee_nip"`
	ComponentID   string    `json:"component_id"`
	ComponentName string    `json:"component_name"`
	AmountIDR     int64     `json:"amount_idr"`
	Source        string    `json:"source"`
	Locked        bool      `json:"locked"`
	PendingChange bool      `json:"pending_change,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type PayrollInputChange struct {
	ID              string    `json:"id"`
	SchoolID        string    `json:"school_id"`
	InputID         *string   `json:"input_id,omitempty"`
	Period          string    `json:"period"`
	EmployeeID      string    `json:"employee_id"`
	ComponentID     string    `json:"component_id"`
	BeforeAmountIDR int64     `json:"before_amount_idr"`
	AfterAmountIDR  int64     `json:"after_amount_idr"`
	Reason          string    `json:"reason"`
	Status          string    `json:"status"`
	ApprovalID      *string   `json:"approval_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Payslip struct {
	ID             string     `json:"id" db:"id"`
	SchoolID       string     `json:"school_id" db:"school_id"`
	LineID         string     `json:"line_id" db:"line_id"`
	Period         string     `json:"period,omitempty"`
	EmployeeName   string     `json:"employee_name,omitempty"`
	GrossIDR       int64      `json:"gross_idr,omitempty"`
	NetIDR         int64      `json:"net_idr,omitempty"`
	ObjectKey      string     `json:"object_key" db:"object_key"`
	SHA256         string     `json:"sha256" db:"sha256"`
	PublishedAt    *time.Time `json:"published_at,omitempty" db:"published_at"`
	ReissuedCount  int        `json:"reissued_count" db:"reissued_count"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}

type PayrollJournal struct {
	ID        string    `json:"id" db:"id"`
	SchoolID  string    `json:"school_id" db:"school_id"`
	RunID     string    `json:"run_id" db:"run_id"`
	Period    string    `json:"period,omitempty"`
	Kind      string    `json:"kind" db:"kind"`
	Reference string    `json:"reference" db:"reference"`
	Status    string    `json:"status" db:"status"`
	TransNo   *string   `json:"trans_no,omitempty" db:"trans_no"`
	Attempts  int       `json:"attempts" db:"attempts"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
