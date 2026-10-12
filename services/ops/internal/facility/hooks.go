package facility

import "context"

// PurchaseRequester turns a work order's material needs into a purchase
// request (FR-118-004, menu 104). Registered by T15 in cmd/server/main.go;
// nil until then, and the endpoint answers 409 "Modul pengadaan belum aktif".
type PurchaseRequester interface {
	CreateFromWorkOrder(ctx context.Context, in WOPurchaseInput) (prRef string, err error)
}

// WOPurchaseInput is the contract body for PurchaseRequester (T9 §4.1).
type WOPurchaseInput struct {
	SchoolID        string
	RequesterUserID string
	WorkOrderID     string
	WorkOrderCode   string
	Lines           []WOPurchaseLine
}

type WOPurchaseLine struct {
	Description  string `json:"description"`
	Qty          int    `json:"qty"`
	EstAmountIDR int64  `json:"est_amount_idr"`
}

// AssetLookup resolves an asset of the school (FR-118-001). Registered by
// T16; nil until then, and asset_id is rejected with 422 "Modul aset belum
// aktif". roomID is the asset's current room ("" when unknown).
type AssetLookup interface {
	Exists(ctx context.Context, schoolID, assetID string) (roomID string, ok bool, err error)
}
