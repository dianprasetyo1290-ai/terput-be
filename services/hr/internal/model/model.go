package model

import (
	"encoding/json"
	"time"
)

type Department struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	ParentID  *string    `json:"parent_id,omitempty"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type Position struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Level          int        `json:"level"`
	JobDescription *string    `json:"job_description,omitempty"`
	DepartmentID   *string    `json:"department_id,omitempty"`
	DepartmentName *string    `json:"department_name,omitempty"`
	SalaryMinIDR   *int64     `json:"salary_min_idr,omitempty"`
	SalaryMaxIDR   *int64     `json:"salary_max_idr,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

type HeadcountPlan struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	PositionID     string    `json:"position_id"`
	PositionName   string    `json:"position_name,omitempty"`
	PositionCode   string    `json:"position_code,omitempty"`
	DepartmentID   *string   `json:"department_id,omitempty"`
	DepartmentName *string   `json:"department_name,omitempty"`
	Planned        int       `json:"planned"`
	PlannedCount   int       `json:"planned_count,omitempty"`
	Year           int       `json:"year,omitempty"`
	EffectiveFrom  string    `json:"effective_from"`
	EffectiveTo    *string   `json:"effective_to,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type HeadcountSummary struct {
	PositionID   string `json:"position_id"`
	PositionCode string `json:"position_code"`
	PositionName string `json:"position_name"`
	Planned      int    `json:"planned"`
	Filled       int    `json:"filled"`
	Vacant       int    `json:"vacant"`
	IsFull       bool   `json:"is_full"`
}

type OrgChangeLog struct {
	ID            string          `json:"id"`
	SchoolID      string          `json:"school_id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	Before        json.RawMessage `json:"before,omitempty"`
	After         json.RawMessage `json:"after,omitempty"`
	EffectiveDate string          `json:"effective_date"`
	ActorUserID   *string         `json:"actor_user_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type PTKPStatus struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type Employee struct {
	ID              string     `json:"id"`
	SchoolID        string     `json:"school_id"`
	UserID          *string    `json:"user_id,omitempty"`
	EmployeeNo      string     `json:"employee_no"`
	FullName        string     `json:"full_name"`
	Email           *string    `json:"email,omitempty"`
	Phone           *string    `json:"phone,omitempty"`
	PhoneEnc        []byte     `json:"-"`
	NIK             *string    `json:"nik,omitempty"`
	NIKEnc          []byte     `json:"-"`
	NIKBidx         *string    `json:"-"`
	NPWP            *string    `json:"npwp,omitempty"`
	NPWPEnc         []byte     `json:"-"`
	NPWPBidx        *string    `json:"-"`
	PTKPStatus      string     `json:"ptkp_status"`
	BankCode        *string    `json:"bank_code,omitempty"`
	BankAccount     *string    `json:"bank_account,omitempty"`
	BankAccountEnc  []byte     `json:"-"`
	BankAccountName *string    `json:"bank_account_name,omitempty"`
	BirthDate       *string    `json:"birth_date,omitempty"`
	Gender          *string    `json:"gender,omitempty"`
	Address         *string    `json:"address,omitempty"`
	AddressEnc      []byte     `json:"-"`
	HireDate        string     `json:"hire_date"`
	ExitDate        *string    `json:"exit_date,omitempty"`
	ExitReason      *string    `json:"exit_reason,omitempty"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`

	// Enriched from current assignment
	CurrentPositionID       *string `json:"current_position_id,omitempty"`
	CurrentPositionName     *string `json:"current_position_name,omitempty"`
	CurrentDepartmentID     *string `json:"current_department_id,omitempty"`
	CurrentDepartmentName   *string `json:"current_department_name,omitempty"`
	CurrentSupervisorID     *string `json:"current_supervisor_id,omitempty"`
	CurrentSupervisorName   *string `json:"current_supervisor_name,omitempty"`
	CurrentEmploymentType   *string `json:"current_employment_type,omitempty"`
	CurrentContractEnd      *string `json:"current_contract_end,omitempty"`
	CurrentBaseSalaryIDR    *int64  `json:"current_base_salary_idr,omitempty"`
	BaseSalaryEnc           []byte  `json:"-"`
	DocumentCompleteness    string  `json:"document_completeness,omitempty"` // "lengkap" or "belum lengkap: ..."
	MissingDocuments        []string `json:"missing_documents,omitempty"`
}

type EmploymentAssignment struct {
	ID                   string     `json:"id"`
	SchoolID             string     `json:"school_id"`
	EmployeeID           string     `json:"employee_id"`
	PositionID           string     `json:"position_id"`
	PositionName         string     `json:"position_name,omitempty"`
	DepartmentID         *string    `json:"department_id,omitempty"`
	DepartmentName       *string    `json:"department_name,omitempty"`
	SupervisorEmployeeID *string    `json:"supervisor_employee_id,omitempty"`
	SupervisorName       *string    `json:"supervisor_name,omitempty"`
	EmploymentType       string     `json:"employment_type"`
	Grade                *string    `json:"grade,omitempty"`
	BaseSalaryIDR        *int64     `json:"base_salary_idr,omitempty"`
	BaseSalaryEnc        []byte     `json:"-"`
	ContractEnd          *string    `json:"contract_end,omitempty"`
	EffectiveFrom        string     `json:"effective_from"`
	EffectiveTo          *string    `json:"effective_to,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type DocumentType struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	HasExpiry bool       `json:"has_expiry"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type RequiredDocument struct {
	ID               string    `json:"id"`
	SchoolID         string    `json:"school_id"`
	EmploymentType   string    `json:"employment_type"`
	DocumentTypeID   string    `json:"document_type_id"`
	DocumentTypeCode string    `json:"document_type_code,omitempty"`
	DocumentTypeName string    `json:"document_type_name,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type EmployeeDocument struct {
	ID               string     `json:"id"`
	SchoolID         string     `json:"school_id"`
	EmployeeID       string     `json:"employee_id"`
	DocumentTypeID   string     `json:"document_type_id"`
	DocumentTypeCode string     `json:"document_type_code,omitempty"`
	DocumentTypeName string     `json:"document_type_name,omitempty"`
	Version          int        `json:"version"`
	ObjectKey        string     `json:"object_key"`
	FileName         string     `json:"file_name"`
	MIME             string     `json:"mime"`
	SizeBytes        int64      `json:"size_bytes"`
	ExpiresOn        *string    `json:"expires_on,omitempty"`
	UploadedBy       *string    `json:"uploaded_by,omitempty"`
	RequestedByHR    bool       `json:"requested_by_hr"`
	SupersededAt     *time.Time `json:"superseded_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type DocumentRequest struct {
	ID               string    `json:"id"`
	SchoolID         string    `json:"school_id"`
	EmployeeID       string    `json:"employee_id"`
	EmployeeName     string    `json:"employee_name,omitempty"`
	DocumentTypeID   string    `json:"document_type_id"`
	DocumentTypeName string    `json:"document_type_name,omitempty"`
	Note             *string   `json:"note,omitempty"`
	Status           string    `json:"status"` // open, fulfilled, cancelled
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type WorkSchedule struct {
	ID                   string     `json:"id"`
	SchoolID             string     `json:"school_id"`
	Name                 string     `json:"name"`
	PositionID           *string    `json:"position_id,omitempty"`
	DepartmentID         *string    `json:"department_id,omitempty"`
	Days                 []int16    `json:"days"`
	StartTime            string     `json:"start_time"`
	EndTime              string     `json:"end_time"`
	LateToleranceMinutes int        `json:"late_tolerance_minutes"`
	EffectiveFrom        string     `json:"effective_from"`
	EffectiveTo          *string    `json:"effective_to,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type Holiday struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	Date      string    `json:"date"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type AttendanceImportProfile struct {
	ID         string          `json:"id"`
	SchoolID   string          `json:"school_id"`
	Name       string          `json:"name"`
	ColumnMap  json.RawMessage `json:"column_map"`
	DateFormat string          `json:"date_format"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type AttendanceImportBatch struct {
	ID        string          `json:"id"`
	SchoolID  string          `json:"school_id"`
	FileName  string          `json:"file_name"`
	ColumnMap json.RawMessage `json:"column_map"`
	Total     int             `json:"total"`
	OK        int             `json:"ok"`
	Failed    int             `json:"failed"`
	Errors    json.RawMessage `json:"errors"`
	CreatedBy *string         `json:"created_by,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type EmployeeAttendance struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	EmployeeID    string     `json:"employee_id"`
	EmployeeName  string     `json:"employee_name,omitempty"`
	EmployeeNo    string     `json:"employee_no,omitempty"`
	WorkDate      string     `json:"work_date"`
	Status        string     `json:"status"` // hadir, terlambat, izin, sakit, cuti, dinas_luar, alpa
	CheckInAt     *time.Time `json:"check_in_at,omitempty"`
	CheckOutAt    *time.Time `json:"check_out_at,omitempty"`
	Lat           *float64   `json:"lat,omitempty"`
	Lng           *float64   `json:"lng,omitempty"`
	Source        string     `json:"source"` // ess, manual, import, leave
	ImportBatchID *string    `json:"import_batch_id,omitempty"`
	Note          *string    `json:"note,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type LeaveType struct {
	ID               string     `json:"id"`
	SchoolID         string     `json:"school_id"`
	Code             string     `json:"code"`
	Name             string     `json:"name"`
	HasQuota         bool       `json:"has_quota"`
	AnnualQuotaDays  *float64   `json:"annual_quota_days,omitempty"`
	Accrual          string     `json:"accrual"` // none, monthly
	CarryOverMaxDays float64    `json:"carry_over_max_days"`
	RequiresDocument bool       `json:"requires_document"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

type LeaveBalance struct {
	ID          string    `json:"id"`
	SchoolID    string    `json:"school_id"`
	EmployeeID  string    `json:"employee_id"`
	LeaveTypeID string    `json:"leave_type_id"`
	TypeName    string    `json:"type_name,omitempty"`
	TypeCode    string    `json:"type_code,omitempty"`
	Year        int       `json:"year"`
	Entitled    float64   `json:"entitled"`
	Carried     float64   `json:"carried"`
	Used        float64   `json:"used"`
	Remaining   float64   `json:"remaining"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type LeaveRequest struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	EmployeeID     string    `json:"employee_id"`
	EmployeeName   string    `json:"employee_name,omitempty"`
	LeaveTypeID    string    `json:"leave_type_id"`
	LeaveTypeName  string    `json:"leave_type_name,omitempty"`
	StartDate      string    `json:"start_date"`
	EndDate        string    `json:"end_date"`
	Days           float64   `json:"days"`
	Reason         *string   `json:"reason,omitempty"`
	AttachmentKey  *string   `json:"attachment_key,omitempty"`
	Status         string    `json:"status"` // pending, approved, rejected, cancelled
	ApprovalID     *string   `json:"approval_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type OvertimeRequest struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	EmployeeID     string    `json:"employee_id"`
	EmployeeName   string    `json:"employee_name,omitempty"`
	WorkDate       string    `json:"work_date"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	Minutes        int       `json:"minutes"`
	Reason         string    `json:"reason"`
	Status         string    `json:"status"`
	ApprovalID     *string   `json:"approval_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AttendanceCorrection struct {
	ID           string          `json:"id"`
	SchoolID     string          `json:"school_id"`
	EmployeeID   string          `json:"employee_id"`
	EmployeeName string          `json:"employee_name,omitempty"`
	WorkDate     string          `json:"work_date"`
	Before       json.RawMessage `json:"before"`
	After        json.RawMessage `json:"after"`
	Reason       string          `json:"reason"`
	Status       string          `json:"status"`
	ApprovalID   *string         `json:"approval_id,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type AttendancePeriod struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Period    string     `json:"period"` // YYYY-MM
	Status    string     `json:"status"` // open, sent, locked
	SentAt    *time.Time `json:"sent_at,omitempty"`
	SentBy    *string    `json:"sent_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type ProfileChangeRequest struct {
	ID           string     `json:"id"`
	SchoolID     string     `json:"school_id"`
	EmployeeID   string     `json:"employee_id"`
	Field        string     `json:"field"` // address, phone, bank_account, ptkp_status
	NewValue     string     `json:"new_value"`
	NewValueEnc  []byte     `json:"-"`
	Status       string     `json:"status"`
	ApprovalID   *string    `json:"approval_id,omitempty"`
	AppliedAt    *time.Time `json:"applied_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type HRSettings struct {
	SchoolID             string          `json:"school_id"`
	ContractReminderDays int             `json:"contract_reminder_days"`
	DocumentReminderDays int             `json:"document_reminder_days"`
	GeolocationEnabled   bool            `json:"geolocation_enabled"`
	Geofence             json.RawMessage `json:"geofence,omitempty"`
	Timezone             string          `json:"timezone"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type GeofenceConfig struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	RadiusM float64 `json:"radius_m"`
}

type OrgNode struct {
	ID             string     `json:"id"`
	EmployeeNo     string     `json:"employee_no"`
	FullName       string     `json:"full_name"`
	PositionID     string     `json:"position_id"`
	PositionName   string     `json:"position_name"`
	DepartmentID   *string    `json:"department_id,omitempty"`
	DepartmentName *string    `json:"department_name,omitempty"`
	SupervisorID   *string    `json:"supervisor_id,omitempty"`
	Children       []*OrgNode `json:"children,omitempty"`
}

type DuplicateMatch struct {
	ID         string `json:"id"`
	EmployeeNo string `json:"employee_no"`
	FullName   string `json:"full_name"`
	Email      string `json:"email,omitempty"`
	MatchReason string `json:"match_reason"`
}

type AttendanceRecapRow struct {
	EmployeeNo string `json:"employee_no"`
	FullName   string `json:"full_name"`
	Department string `json:"department"`
	Position   string `json:"position"`
	Hadir      int    `json:"hadir"`
	Terlambat  int    `json:"terlambat"`
	Izin       int    `json:"izin"`
	Sakit      int    `json:"sakit"`
	Cuti       int    `json:"cuti"`
	DinasLuar  int    `json:"dinas_luar"`
	Alpa       int    `json:"alpa"`
	OvertimeMinutes int `json:"overtime_minutes"`
}

type ESSDashboard struct {
	LeaveBalanceAnnual float64                `json:"leave_balance_annual"`
	PendingRequests    int                    `json:"pending_requests"`
	LatestPayslip      *string                `json:"latest_payslip,omitempty"`
	RequiredTrainings  int                    `json:"required_trainings"`
	TodayAttendance    *EmployeeAttendance    `json:"today_attendance,omitempty"`
	ServerTime         time.Time              `json:"server_time"`
}
