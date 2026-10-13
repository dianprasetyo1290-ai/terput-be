package asset

import (
	"time"
)

type Category struct {
	ID                 string     `json:"id"`
	SchoolID           string     `json:"school_id"`
	Code               string     `json:"code"`
	Name               string     `json:"name"`
	UsefulLifeMonths   *int       `json:"useful_life_months,omitempty"`
	DepStart           *string    `json:"dep_start,omitempty"` // "acquisition_month", "next_month"
	AssetAccCode       *string    `json:"asset_acc_code,omitempty"`
	AccumDepAccCode    *string    `json:"accum_dep_acc_code,omitempty"`
	DepExpenseAccCode  *string    `json:"dep_expense_acc_code,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
}

type Settings struct {
	SchoolID            string    `json:"school_id"`
	CodeFormat          string    `json:"code_format"`
	InterunitAccCode    *string   `json:"interunit_acc_code,omitempty"`
	DisposalGainAccCode *string   `json:"disposal_gain_acc_code,omitempty"`
	DisposalLossAccCode *string   `json:"disposal_loss_acc_code,omitempty"`
	ProceedsAccCode     *string   `json:"proceeds_acc_code,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Asset struct {
	ID                     string     `json:"id"`
	SchoolID               string     `json:"school_id"`
	Code                   string     `json:"code"`
	Name                   string     `json:"name"`
	CategoryID             *string    `json:"category_id,omitempty"`
	CategoryName           *string    `json:"category_name,omitempty"`
	CategoryCode           *string    `json:"category_code,omitempty"`
	Status                 string     `json:"status"` // candidate, active, disposed, transferred_out
	Condition              *string    `json:"condition,omitempty"` // baik, rusak_ringan, rusak_berat
	AcquisitionCostIDR     int64      `json:"acquisition_cost_idr"`
	AcquiredOn             *string    `json:"acquired_on,omitempty"`
	ResidualIDR            int64      `json:"residual_idr"`
	RoomID                 *string    `json:"room_id,omitempty"`
	RoomName               *string    `json:"room_name,omitempty"`
	CustodianUserID        *string    `json:"custodian_user_id,omitempty"`
	CustodianName          *string    `json:"custodian_name,omitempty"`
	SerialNo               *string    `json:"serial_no,omitempty"`
	PhotoObjectKey         *string    `json:"photo_object_key,omitempty"`
	PhotoURL               *string    `json:"photo_url,omitempty"`
	QRToken                string     `json:"qr_token"`
	Source                 string     `json:"source"` // goods_receipt, manual, interunit_transfer
	GRLineID               *string    `json:"gr_line_id,omitempty"`
	UnitSeq                *int       `json:"unit_seq,omitempty"`
	POID                   *string    `json:"po_id,omitempty"`
	VendorID               *string    `json:"vendor_id,omitempty"`
	TransferredFromAssetID *string    `json:"transferred_from_asset_id,omitempty"`
	OffsetAccCode          *string    `json:"offset_acc_code,omitempty"`
	ActivatedAt            *time.Time `json:"activated_at,omitempty"`
	ActivatedBy            *string    `json:"activated_by,omitempty"`
	DeletedAt              *time.Time `json:"deleted_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`

	// Accounting status overlay
	AccountingStatus *string `json:"accounting_status,omitempty"` // "pending", "posted", "failed"
	BookValueIDR     *int64  `json:"book_value_idr,omitempty"`
}

type Movement struct {
	ID            string         `json:"id"`
	SchoolID      string         `json:"school_id"`
	AssetID       string         `json:"asset_id"`
	Kind          string         `json:"kind"` // location, custodian, condition, unit
	FromValue     map[string]any `json:"from_value"`
	ToValue       map[string]any `json:"to_value"`
	EffectiveDate string         `json:"effective_date"`
	Reason        string         `json:"reason"`
	Status        string         `json:"status"` // done, pending_approval, approved, rejected
	ApprovalID    *string        `json:"approval_id,omitempty"`
	ToSchoolID    *string        `json:"to_school_id,omitempty"`
	CreatedBy     string         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
}

type Disposal struct {
	ID          string    `json:"id"`
	SchoolID    string    `json:"school_id"`
	AssetID     string    `json:"asset_id"`
	AssetName   *string   `json:"asset_name,omitempty"`
	AssetCode   *string   `json:"asset_code,omitempty"`
	Kind        string    `json:"kind"` // dijual, dihibahkan, rusak, hilang
	DisposedOn  string    `json:"disposed_on"`
	ProceedsIDR int64     `json:"proceeds_idr"`
	Reason      string    `json:"reason"`
	Status      string    `json:"status"` // pending_approval, approved, rejected, cancelled
	ApprovalID  *string   `json:"approval_id,omitempty"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

type AccountingEvent struct {
	ID        string         `json:"id"`
	SchoolID  string         `json:"school_id"`
	AssetID   string         `json:"asset_id"`
	Kind      string         `json:"kind"` // acquisition, transfer, disposal
	SourceID  *string        `json:"source_id,omitempty"`
	Reference string         `json:"reference"`
	Payload   map[string]any `json:"payload"`
	Status    string         `json:"status"` // pending, posted, failed
	Attempts  int            `json:"attempts"`
	Error     *string        `json:"error,omitempty"`
	TransNos  []string       `json:"trans_nos,omitempty"`
	PostedAt  *time.Time     `json:"posted_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type BookValue struct {
	AssetID        string    `json:"asset_id"`
	SchoolID       string    `json:"school_id"`
	CostIDR        int64     `json:"cost_idr"`
	AccumulatedIDR int64     `json:"accumulated_idr"`
	BookValueIDR   int64     `json:"book_value_idr"`
	LastPeriod     *string   `json:"last_period,omitempty"`
	AsOf           time.Time `json:"as_of"`
}

type DepreciationRun struct {
	SchoolID  string    `json:"school_id"`
	Period    string    `json:"period"`
	Status    string    `json:"status"` // pending, done, failed
	Error     *string   `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type DepreciationScheduleItem struct {
	Period    string `json:"period"`
	AmountIDR int64  `json:"amount_idr"`
}

type RoomSummary struct {
	RoomID          string            `json:"room_id"`
	Total           int               `json:"total"`
	ByCondition     map[string]int    `json:"by_condition"` // baik, rusak_ringan, rusak_berat, tanpa_kondisi
	ByCategory      []CategorySummary `json:"by_category"`
	PendingDisposal int               `json:"pending_disposal"`
}

type CategorySummary struct {
	CategoryID string `json:"category_id"`
	Name       string `json:"name"`
	Count      int    `json:"count"`
}
