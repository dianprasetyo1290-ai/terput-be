package inventory

import (
	"time"
)

type ItemCategory struct {
	ID                string     `json:"id"`
	SchoolID          string     `json:"school_id"`
	Code              string     `json:"code"`
	Name              string     `json:"name"`
	Tracking          string     `json:"tracking"` // 'stock', 'asset', 'none'
	PurchaseAccCode   *string    `json:"purchase_acc_code,omitempty"`
	WithholdingTypeID *string    `json:"withholding_type_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty"`
}

type Item struct {
	ID           string        `json:"id"`
	SchoolID     string        `json:"school_id"`
	Code         string        `json:"code"`
	Name         string        `json:"name"`
	CategoryID   string        `json:"category_id"`
	CategoryName string        `json:"category_name,omitempty"`
	Tracking     string        `json:"tracking,omitempty"`
	Unit         string        `json:"unit"`
	MinStock     int64         `json:"min_stock"`
	CurrentStock int64         `json:"current_stock"`
	BelowMin     bool          `json:"below_min"`
	Status       string        `json:"status"` // 'active', 'inactive'
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	DeletedAt    *time.Time    `json:"deleted_at,omitempty"`
	Category     *ItemCategory `json:"category,omitempty"`
}

type Warehouse struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	RoomID    *string    `json:"room_id,omitempty"`
	Status    string     `json:"status"` // 'active', 'inactive'
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type StockBalance struct {
	SchoolID      string `json:"school_id"`
	WarehouseID   string `json:"warehouse_id"`
	WarehouseName string `json:"warehouse_name,omitempty"`
	ItemID        string `json:"item_id"`
	ItemCode      string `json:"item_code,omitempty"`
	ItemName      string `json:"item_name,omitempty"`
	Unit          string `json:"unit,omitempty"`
	MinStock      int64  `json:"min_stock"`
	Qty           int64  `json:"qty"`
	BelowMin      bool   `json:"below_min"`
	UpdatedAt     string `json:"updated_at"`
}

type StockMovement struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	WarehouseID    string    `json:"warehouse_id"`
	WarehouseName  string    `json:"warehouse_name,omitempty"`
	ItemID         string    `json:"item_id"`
	ItemCode       string    `json:"item_code,omitempty"`
	ItemName       string    `json:"item_name,omitempty"`
	Unit           string    `json:"unit,omitempty"`
	Direction      string    `json:"direction"` // 'in', 'out'
	Qty            int64     `json:"qty"`
	Kind           string    `json:"kind"` // 'receipt', 'issue', 'transfer_in', 'transfer_out', 'adjustment', 'return_to_vendor'
	RefType        string    `json:"ref_type"`
	RefID          string    `json:"ref_id"`
	RefNo          string    `json:"ref_no"`
	TransferID     *string   `json:"transfer_id,omitempty"`
	BalanceAfter   int64     `json:"balance_after"`
	Note           *string   `json:"note,omitempty"`
	CreatedBy      string    `json:"created_by"`
	CreatedByName  string    `json:"created_by_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	IdempotencyKey string    `json:"idempotency_key"`
}

type StockCount struct {
	ID              string           `json:"id"`
	SchoolID        string           `json:"school_id"`
	WarehouseID     string           `json:"warehouse_id"`
	WarehouseName   string           `json:"warehouse_name,omitempty"`
	Number          string           `json:"number"`
	Status          string           `json:"status"` // 'counting', 'submitted', 'pending_approval', 'approved', 'rejected', 'adjusted', 'adjust_failed', 'cancelled'
	StartedAt       time.Time        `json:"started_at"`
	StartedBy       string           `json:"started_by"`
	StartedByName   string           `json:"started_by_name,omitempty"`
	SubmittedAt     *time.Time       `json:"submitted_at,omitempty"`
	ApprovalID      *string          `json:"approval_id,omitempty"`
	ReportObjectKey *string          `json:"report_object_key,omitempty"`
	Error           *string          `json:"error,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	Lines           []StockCountLine `json:"lines,omitempty"`
}

type StockCountLine struct {
	ID         string  `json:"id"`
	SchoolID   string  `json:"school_id"`
	CountID    string  `json:"count_id"`
	ItemID     string  `json:"item_id"`
	ItemCode   string  `json:"item_code,omitempty"`
	ItemName   string  `json:"item_name,omitempty"`
	Unit       string  `json:"unit,omitempty"`
	SystemQty  int64   `json:"system_qty"`
	CountedQty *int64  `json:"counted_qty,omitempty"`
	Variance   *int64  `json:"variance,omitempty"`
	Note       *string `json:"note,omitempty"`
}

type MoveInput struct {
	SchoolID       string
	WarehouseID    string
	ItemID         string
	Direction      string
	Qty            int64
	Kind           string
	RefType        string
	RefID          string
	RefNo          string
	TransferID     *string
	Note           *string
	CreatedBy      string
	ActorRole      string
	IdempotencyKey string
}

type StockCardEntry struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Direction    string    `json:"direction"`
	Qty          int64     `json:"qty"`
	Kind         string    `json:"kind"`
	RefNo        string    `json:"ref_no"`
	BalanceAfter int64     `json:"balance_after"`
	Note         string    `json:"note"`
}

type StockCardReport struct {
	ItemID         string           `json:"item_id"`
	ItemName       string           `json:"item_name"`
	WarehouseID    string           `json:"warehouse_id"`
	WarehouseName  string           `json:"warehouse_name"`
	OpeningBalance int64            `json:"opening_balance"`
	ClosingBalance int64            `json:"closing_balance"`
	Entries        []StockCardEntry `json:"entries"`
}
