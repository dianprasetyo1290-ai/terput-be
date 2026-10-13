package payable

import (
	"time"
)

type PayableSettings struct {
	SchoolID                 string    `json:"school_id"`
	DefaultAPAccCode         string    `json:"default_ap_acc_code"`          // Default "2101" (Utang Usaha)
	DefaultVATInAccCode      string    `json:"default_vat_in_acc_code"`      // Default "1181" (PPN Masukan)
	DefaultTaxPayableAccCode string    `json:"default_tax_payable_acc_code"` // Default "2131" (Utang PPh)
	DefaultBankAccCode       string    `json:"default_bank_acc_code"`        // Default "1102" (Bank)
	ToleranceAmountIDR       int64     `json:"tolerance_amount_idr"`
	AutoJournalOnInvoice     bool      `json:"auto_journal_on_invoice"`
	AutoJournalOnPayment     bool      `json:"auto_journal_on_payment"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type VendorInvoice struct {
	ID                   string              `json:"id"`
	SchoolID             string              `json:"school_id"`
	Number               string              `json:"number"`
	VendorID             string              `json:"vendor_id"`
	VendorName           string              `json:"vendor_name,omitempty"`
	POID                 *string             `json:"po_id,omitempty"`
	PONumber             *string             `json:"po_number,omitempty"`
	InvoiceDate          string              `json:"invoice_date"`
	DueDate              string              `json:"due_date"`
	SubtotalIDR          int64               `json:"subtotal_idr"`
	VATIDR               int64               `json:"vat_idr"`
	WithholdingTaxIDR    int64               `json:"withholding_tax_idr"`
	TotalIDR             int64               `json:"total_idr"`
	PaidAmountIDR        int64               `json:"paid_amount_idr"`
	Status               string              `json:"status"`       // draft, pending_match, matched, discrepancy, override_approved, approved, partially_paid, paid, cancelled
	MatchStatus          string              `json:"match_status"` // pending, matched_ok, qty_mismatch, price_mismatch, over_billed, override_needed
	MatchDiscrepancyNote *string             `json:"match_discrepancy_note,omitempty"`
	OverrideApprovalID   *string             `json:"override_approval_id,omitempty"`
	PDFObjectKey         *string             `json:"pdf_object_key,omitempty"`
	JournalTransNo       *string             `json:"journal_trans_no,omitempty"`
	IdempotencyKey       string              `json:"idempotency_key"`
	CreatedBy            string              `json:"created_by"`
	CreatedAt            time.Time           `json:"created_at"`
	UpdatedAt            time.Time           `json:"updated_at"`
	Lines                []VendorInvoiceLine `json:"lines,omitempty"`
}

type VendorInvoiceLine struct {
	ID                string    `json:"id"`
	SchoolID          string    `json:"school_id"`
	InvoiceID         string    `json:"invoice_id"`
	POLineID          *string   `json:"po_line_id,omitempty"`
	GRLineID          *string   `json:"gr_line_id,omitempty"`
	Description       string    `json:"description"`
	Qty               int64     `json:"qty"`
	UnitPriceIDR      int64     `json:"unit_price_idr"`
	AmountIDR         int64     `json:"amount_idr"`
	WithholdingTypeID *string   `json:"withholding_type_id,omitempty"`
	WithholdingTaxIDR int64     `json:"withholding_tax_idr"`
	BudgetLineKey     *string   `json:"budget_line_key,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type PaymentSchedule struct {
	ID                   string     `json:"id"`
	SchoolID             string     `json:"school_id"`
	InvoiceID            string     `json:"invoice_id"`
	InvoiceNumber        string     `json:"invoice_number,omitempty"`
	VendorID             string     `json:"vendor_id"`
	VendorName           string     `json:"vendor_name,omitempty"`
	ScheduledDate        string     `json:"scheduled_date"`
	AmountIDR            int64      `json:"amount_idr"`
	PaymentMethod        string     `json:"payment_method"` // bank_transfer, cash, giro, other
	SourceBankAccCode    *string    `json:"source_bank_acc_code,omitempty"`
	VendorBankAccountID  *string    `json:"vendor_bank_account_id,omitempty"`
	Status               string     `json:"status"` // scheduled, pending_approval, approved, executed, cancelled
	ApprovalID           *string    `json:"approval_id,omitempty"`
	ExecutedAt           *time.Time `json:"executed_at,omitempty"`
	ExecutedBy           *string    `json:"executed_by,omitempty"`
	ReferenceNo          *string    `json:"reference_no,omitempty"`
	JournalTransNo       *string    `json:"journal_trans_no,omitempty"`
	CreatedBy            string     `json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type PayableJournal struct {
	ID           string    `json:"id"`
	SchoolID     string    `json:"school_id"`
	SourceType   string    `json:"source_type"` // invoice, payment
	SourceID     string    `json:"source_id"`
	TransNo      string    `json:"trans_no"`
	TransDate    string    `json:"trans_date"`
	AmountIDR    int64     `json:"amount_idr"`
	Description  string    `json:"description"`
	Status       string    `json:"status"` // posted, failed, reversed
	ErrorMessage *string   `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type MatchInputLine struct {
	POLineID         string
	GRLineID         string
	Description      string
	QtyInvoice       int64
	UnitPriceInvoice int64
	QtyAcceptedGR    int64
	UnitPricePO      int64
}

type MatchResultLine struct {
	POLineID         string `json:"po_line_id"`
	QtyInvoice       int64  `json:"qty_invoice"`
	QtyAcceptedGR    int64  `json:"qty_accepted_gr"`
	UnitPriceInvoice int64  `json:"unit_price_invoice"`
	UnitPricePO      int64  `json:"unit_price_po"`
	QtyDiff          int64  `json:"qty_diff"`
	PriceDiff        int64  `json:"price_diff"`
	DiscrepancyType  string `json:"discrepancy_type"` // none, qty_mismatch, price_mismatch, over_billed
	OK               bool   `json:"ok"`
}

type MatchResult struct {
	MatchStatus     string            `json:"match_status"` // matched_ok, discrepancy, override_needed
	DiscrepancyNote string            `json:"discrepancy_note,omitempty"`
	OK              bool              `json:"ok"`
	Lines           []MatchResultLine `json:"lines"`
}

type AgingBucketItem struct {
	InvoiceID     string `json:"invoice_id"`
	InvoiceNumber string `json:"invoice_number"`
	VendorID      string `json:"vendor_id"`
	VendorName    string `json:"vendor_name"`
	DueDate       string `json:"due_date"`
	DaysOverdue   int    `json:"days_overdue"`
	TotalIDR      int64  `json:"total_idr"`
	OutstandingIDR int64 `json:"outstanding_idr"`
}

type AgingReport struct {
	AsOf           string            `json:"as_of"`
	TotalAPIDR     int64             `json:"total_ap_idr"`
	CurrentIDR     int64             `json:"current_idr"`      // Belum jatuh tempo
	Overdue1to30   int64             `json:"overdue_1_30_idr"`  // 1-30 hari
	Overdue31to60  int64             `json:"overdue_31_60_idr"` // 31-60 hari
	Overdue61to90  int64             `json:"overdue_61_90_idr"` // 61-90 hari
	Overdue90Plus  int64             `json:"overdue_90_plus_idr"` // >90 hari
	Items          []AgingBucketItem `json:"items"`
}
