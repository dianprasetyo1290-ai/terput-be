package tax

import (
	"time"
)

type TaxSettings struct {
	SchoolID       string    `json:"school_id"`
	NPWPMasked     *string   `json:"npwp_masked,omitempty"`
	NITKU          *string   `json:"nitku,omitempty"`
	EfakturEnabled bool      `json:"efaktur_enabled"`
	DefaultPPNBP   int       `json:"default_ppn_bp"` // 1100 = 11%
	UpdatedAt      time.Time `json:"updated_at"`
}

type WithholdingTaxType struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	Article        string    `json:"article"` // "21", "22", "23", "4(2)", "15"
	StandardRateBP int       `json:"standard_rate_bp"`
	NoNPWPRateBP   int       `json:"no_npwp_rate_bp"`
	Description    *string   `json:"description,omitempty"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type WithholdingSlip struct {
	ID              string     `json:"id"`
	SchoolID        string     `json:"school_id"`
	SlipNumber      string     `json:"slip_number"`
	TypeID          string     `json:"type_id"`
	TypeName        string     `json:"type_name,omitempty"`
	TypeArticle     string     `json:"type_article,omitempty"`
	VendorID        string     `json:"vendor_id"`
	VendorName      string     `json:"vendor_name,omitempty"`
	VendorInvoiceID *string    `json:"vendor_invoice_id,omitempty"`
	TaxPeriod       string     `json:"tax_period"` // YYYY-MM
	GrossAmountIDR  int64      `json:"gross_amount_idr"`
	RateBP          int        `json:"rate_bp"`
	TaxAmountIDR    int64      `json:"tax_amount_idr"`
	NPWPUsed        *string    `json:"npwp_used,omitempty"`
	Status          string     `json:"status"` // "draft", "issued", "reported", "cancelled"
	IssuedAt        *time.Time `json:"issued_at,omitempty"`
	ReportedAt      *time.Time `json:"reported_at,omitempty"`
	PDFObjectKey    *string    `json:"pdf_object_key,omitempty"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type InputVAT struct {
	ID              string    `json:"id"`
	SchoolID        string    `json:"school_id"`
	InvoiceNumber   string    `json:"invoice_number"` // 16 digit Faktur Pajak
	VendorID        string    `json:"vendor_id"`
	VendorName      string    `json:"vendor_name,omitempty"`
	VendorInvoiceID *string   `json:"vendor_invoice_id,omitempty"`
	TaxPeriod       string    `json:"tax_period"`
	InvoiceDate     string    `json:"invoice_date"`
	DPPIDR          int64     `json:"dpp_idr"`
	VATIDR          int64     `json:"vat_idr"`
	Creditable      bool      `json:"creditable"`
	Status          string    `json:"status"` // "received", "credited", "expired", "rejected"
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type OutputTaxInvoice struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	InvoiceNumber string    `json:"invoice_number"`
	CustomerName  string    `json:"customer_name"`
	CustomerNPWP  *string   `json:"customer_npwp,omitempty"`
	TaxPeriod     string    `json:"tax_period"`
	InvoiceDate   string    `json:"invoice_date"`
	DPPIDR        int64     `json:"dpp_idr"`
	VATIDR        int64     `json:"vat_idr"`
	Status        string    `json:"status"` // "draft", "approved", "cancelled"
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type TaxExportTemplate struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	Kind          string    `json:"kind"` // "ebupot_23", "ebupot_4_2", "efaktur_input", "efaktur_output"
	TaxPeriod     string    `json:"tax_period"`
	RecordCount   int       `json:"record_count"`
	TotalTaxIDR   int64     `json:"total_tax_idr"`
	FileObjectKey *string   `json:"file_object_key,omitempty"`
	GeneratedBy   string    `json:"generated_by"`
	CreatedAt     time.Time `json:"created_at"`
	ContentCSV    string    `json:"content_csv,omitempty"`
}
