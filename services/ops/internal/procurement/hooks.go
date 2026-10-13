package procurement

import (
	"context"
	"database/sql"
	"time"
)

// Hook nil-safe ke modul akuntansi (T17) dan aset (T16)

type BudgetCheckInput struct {
	SchoolID      string
	Lines         []BudgetCheckLine
	ExcludeSource string // "pr:<id>" atau "po:<id>"
}

type BudgetCheckLine struct {
	BudgetLineKey string
	AmountIDR     int64
}

type BudgetLineResult struct {
	BudgetLineKey string
	BudgetIDR     int64
	ActualIDR     int64
	CommitmentIDR int64
	RemainingIDR  int64
	RequestIDR    int64
	Over          bool
}

type BudgetCheckResult struct {
	Policy string             // "extra_approval" | "reject"
	Lines  []BudgetLineResult
	Over   bool
	AsOf   time.Time
}

// BudgetCheck dipanggil saat submit PR dan submit PO (didaftarkan T17).
var BudgetCheck func(ctx context.Context, tx *sql.Tx, in BudgetCheckInput) (BudgetCheckResult, error)

type GoodsReceivedLine struct {
	GRLineID       string
	POLineID       string
	POID           string
	PONumber       string
	VendorID       string
	ItemID         string
	ItemName       string
	ItemCategoryID string
	QtyAccepted    int64
	UnitPriceIDR   int64
	ReceivedOn     time.Time
}

type GoodsReceivedEvent struct {
	SchoolID       string
	GoodsReceiptID string
	GRNumber       string
	Lines          []GoodsReceivedLine // hanya baris tracking=asset
}

// OnGoodsReceived dipanggil di dalam transaksi penerimaan untuk baris aset (didaftarkan T16).
var OnGoodsReceived func(ctx context.Context, tx *sql.Tx, ev GoodsReceivedEvent) error
