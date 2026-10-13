package procurement

import (
	"time"
)

type Vendor struct {
	ID               string              `json:"id"`
	SchoolID         string              `json:"school_id"`
	Code             string              `json:"code"`
	Name             string              `json:"name"`
	NPWP             string              `json:"npwp,omitempty"` // decrypted or masked
	NPWPBidx         *string             `json:"-"`
	Address          *string             `json:"address,omitempty"`
	Categories       []string            `json:"categories"`
	PaymentTermsDays *int                `json:"payment_terms_days,omitempty"`
	Status           string              `json:"status"` // 'active', 'inactive', 'blocked'
	DuplicateAckBy   *string             `json:"duplicate_ack_by,omitempty"`
	Contacts         []VendorContact     `json:"contacts,omitempty"`
	Documents        []VendorDocument    `json:"documents,omitempty"`
	BankAccounts     []VendorBankAccount `json:"bank_accounts,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
	DeletedAt        *time.Time          `json:"deleted_at,omitempty"`
}

type VendorContact struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	VendorID  string    `json:"vendor_id"`
	Name      string    `json:"name"`
	Phone     *string   `json:"phone,omitempty"`
	Email     *string   `json:"email,omitempty"`
	IsPrimary bool      `json:"is_primary"`
	CreatedAt time.Time `json:"created_at"`
}

type VendorDocument struct {
	ID         string     `json:"id"`
	SchoolID   string     `json:"school_id"`
	VendorID   string     `json:"vendor_id"`
	Kind       string     `json:"kind"`
	ObjectKey  string     `json:"object_key"`
	ExpiresOn  *time.Time `json:"expires_on,omitempty"`
	UploadedBy string     `json:"uploaded_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

type VendorBankAccount struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	VendorID      string     `json:"vendor_id"`
	BankName      string     `json:"bank_name"`
	AccountNo     string     `json:"account_no,omitempty"` // decrypted/masked
	AccountHolder string     `json:"account_holder"`
	Status        string     `json:"status"` // 'pending_approval', 'active', 'rejected', 'superseded'
	ApprovalID    *string    `json:"approval_id,omitempty"`
	RequestedBy   string     `json:"requested_by"`
	ActivatedAt   *time.Time `json:"activated_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type PurchaseRequest struct {
	ID              string                `json:"id"`
	SchoolID        string                `json:"school_id"`
	Number          string                `json:"number"`
	RequesterUserID string                `json:"requester_user_id"`
	RequesterName   string                `json:"requester_name,omitempty"`
	Reason          string                `json:"reason"`
	NeededBy        *time.Time            `json:"needed_by,omitempty"`
	Status          string                `json:"status"` // 'draft', 'pending_approval', 'approved', 'rejected', 'cancelled', 'partially_ordered', 'ordered'
	TotalEstIDR     int64                 `json:"total_est_idr"`
	OverBudget      bool                  `json:"over_budget"`
	BudgetCheck     any                   `json:"budget_check,omitempty"`
	ApprovalID      *string               `json:"approval_id,omitempty"`
	Lines           []PurchaseRequestLine `json:"lines,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

type PurchaseRequestLine struct {
	ID              string  `json:"id"`
	SchoolID        string  `json:"school_id"`
	PRID            string  `json:"pr_id"`
	ItemID          *string `json:"item_id,omitempty"`
	ItemCode        *string `json:"item_code,omitempty"`
	ItemName        *string `json:"item_name,omitempty"`
	Description     string  `json:"description"`
	Qty             int64   `json:"qty"`
	Unit            string  `json:"unit"`
	EstUnitPriceIDR int64   `json:"est_unit_price_idr"`
	BudgetLineKey   *string `json:"budget_line_key,omitempty"`
	QtyOrdered      int64   `json:"qty_ordered"`
}

type PurchaseOrder struct {
	ID               string              `json:"id"`
	SchoolID         string              `json:"school_id"`
	Number           string              `json:"number"`
	VendorID         string              `json:"vendor_id"`
	VendorName       string              `json:"vendor_name,omitempty"`
	PaymentTermsDays int                 `json:"payment_terms_days"`
	ExpectedDate     *time.Time          `json:"expected_date,omitempty"`
	Status           string              `json:"status"` // 'draft', 'pending_approval', 'approved', 'rejected', 'sent', 'partially_received', 'received', 'closed', 'cancelled'
	TotalIDR         int64               `json:"total_idr"`
	ApprovalID       *string             `json:"approval_id,omitempty"`
	PDFObjectKey     *string             `json:"pdf_object_key,omitempty"`
	SentAt           *time.Time          `json:"sent_at,omitempty"`
	ClosedReason     *string             `json:"closed_reason,omitempty"`
	Lines            []PurchaseOrderLine `json:"lines,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

type PurchaseOrderLine struct {
	ID                string  `json:"id"`
	SchoolID          string  `json:"school_id"`
	POID              string  `json:"po_id"`
	PRLineID          *string `json:"pr_line_id,omitempty"`
	ItemID            *string `json:"item_id,omitempty"`
	ItemCode          *string `json:"item_code,omitempty"`
	ItemName          *string `json:"item_name,omitempty"`
	Description       string  `json:"description"`
	Qty               int64   `json:"qty"`
	UnitPriceIDR      int64   `json:"unit_price_idr"`
	AmountIDR         int64   `json:"amount_idr"`
	BudgetLineKey     *string `json:"budget_line_key,omitempty"`
	WithholdingTypeID *string `json:"withholding_type_id,omitempty"`
	WarehouseID       *string `json:"warehouse_id,omitempty"`
	WarehouseName     *string `json:"warehouse_name,omitempty"`
	QtyAcceptedNet    int64   `json:"qty_accepted_net"`
	QtyRemaining      int64   `json:"qty_remaining"`
}

type GoodsReceipt struct {
	ID             string             `json:"id"`
	SchoolID       string             `json:"school_id"`
	Number         string             `json:"number"`
	POID           string             `json:"po_id"`
	PONumber       string             `json:"po_number,omitempty"`
	ReceivedOn     time.Time          `json:"received_on"`
	ReceivedBy     string             `json:"received_by"`
	ReceivedByName string             `json:"received_by_name,omitempty"`
	Note           *string            `json:"note,omitempty"`
	Lines          []GoodsReceiptLine `json:"lines,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
}

type GoodsReceiptLine struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	GRID           string     `json:"gr_id"`
	POLineID       string     `json:"po_line_id"`
	ItemID         *string    `json:"item_id,omitempty"`
	ItemName       string     `json:"item_name,omitempty"`
	QtyReceived    int64      `json:"qty_received"`
	QtyAccepted    int64      `json:"qty_accepted"`
	QtyRejected    int64      `json:"qty_rejected"`
	RejectReason   *string    `json:"reject_reason,omitempty"`
	AssetHandoffAt *time.Time `json:"asset_handoff_at,omitempty"`
}

type GoodsReturn struct {
	ID         string    `json:"id"`
	SchoolID   string    `json:"school_id"`
	GRLineID   string    `json:"gr_line_id"`
	Qty        int64     `json:"qty"`
	Reason     string    `json:"reason"`
	ReturnedOn time.Time `json:"returned_on"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

type VendorEvaluation struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	VendorID       string    `json:"vendor_id"`
	GoodsReceiptID string    `json:"goods_receipt_id"`
	OnTime         *bool     `json:"on_time,omitempty"`
	QualityScore   int       `json:"quality_score"`
	PriceScore     int       `json:"price_score"`
	Note           *string   `json:"note,omitempty"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}
