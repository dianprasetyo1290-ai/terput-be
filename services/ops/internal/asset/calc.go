package asset

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"skci/ops/internal/procurement"
)

type MovementState struct {
	RoomID          string `json:"room_id"`
	CustodianUserID string `json:"custodian_user_id"`
	Condition       string `json:"condition"`
	SchoolID        string `json:"school_id"`
}

// GenerateQRToken generates a 128-bit (32 hex chars) secure random token
func GenerateQRToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CandidatesFromReceipt creates candidate asset records from GoodsReceivedEvent
// (AC-105-02 / FR-107-002: 1 line qty 2 price 12.000.000 -> 2 candidates with unit_seq 1,2 each 12.000.000)
func CandidatesFromReceipt(ev procurement.GoodsReceivedEvent) []Asset {
	var candidates []Asset
	for _, line := range ev.Lines {
		if line.QtyAccepted <= 0 {
			continue
		}
		receivedOn := line.ReceivedOn.Format("2006-01-02")
		for s := 1; s <= int(line.QtyAccepted); s++ {
			seq := s
			grLineID := line.GRLineID
			poID := line.POID
			vendorID := line.VendorID
			itemName := line.ItemName
			if itemName == "" {
				itemName = "Barang Aset Penerimaan"
			}
			candidates = append(candidates, Asset{
				SchoolID:           ev.SchoolID,
				Name:               itemName,
				Status:             "candidate",
				Source:             "goods_receipt",
				GRLineID:           &grLineID,
				UnitSeq:            &seq,
				POID:               &poID,
				VendorID:           &vendorID,
				AcquisitionCostIDR: line.UnitPriceIDR,
				AcquiredOn:         &receivedOn,
				ResidualIDR:        0,
				QRToken:            GenerateQRToken(),
			})
		}
	}
	return candidates
}

// CanActivate validates whether an asset is ready to be activated
// (AC-107-01: room and custodian required; category with useful_life_months and GL accounts required)
func CanActivate(asset Asset, category *Category) []string {
	var missing []string

	if asset.RoomID == nil || strings.TrimSpace(*asset.RoomID) == "" {
		missing = append(missing, "lokasi wajib")
	}

	if asset.CustodianUserID == nil || strings.TrimSpace(*asset.CustodianUserID) == "" {
		missing = append(missing, "penanggung jawab wajib")
	}

	if category == nil {
		missing = append(missing, "kategori aset belum lengkap")
	} else {
		if category.UsefulLifeMonths == nil || *category.UsefulLifeMonths <= 0 ||
			category.AssetAccCode == nil || strings.TrimSpace(*category.AssetAccCode) == "" ||
			category.AccumDepAccCode == nil || strings.TrimSpace(*category.AccumDepAccCode) == "" ||
			category.DepExpenseAccCode == nil || strings.TrimSpace(*category.DepExpenseAccCode) == "" {
			missing = append(missing, "kategori aset belum lengkap")
		}
	}

	if asset.Source == "manual" {
		if asset.OffsetAccCode == nil || strings.TrimSpace(*asset.OffsetAccCode) == "" {
			missing = append(missing, "akun lawan wajib untuk perolehan manual")
		}
	}

	return missing
}

// AcquisitionPayload builds the payload for Accounting Engine POST /api/internal/assets (§4.2)
func AcquisitionPayload(asset Asset, category Category, settings Settings) map[string]any {
	postJournal := (asset.Source == "manual")

	payload := map[string]any{
		"school_id":            asset.SchoolID,
		"asset_ref":            asset.ID,
		"code":                 asset.Code,
		"name":                 asset.Name,
		"cost_idr":             asset.AcquisitionCostIDR,
		"residual_idr":         asset.ResidualIDR,
		"useful_life_months":   0,
		"dep_start":            "acquisition_month",
		"asset_acc_code":       "",
		"accum_dep_acc_code":   "",
		"dep_expense_acc_code": "",
		"post_journal":         postJournal,
		"reference":            "asset:acq:" + asset.ID,
	}

	if asset.AcquiredOn != nil {
		payload["acquired_on"] = *asset.AcquiredOn
	} else {
		payload["acquired_on"] = time.Now().Format("2006-01-02")
	}

	if category.UsefulLifeMonths != nil {
		payload["useful_life_months"] = *category.UsefulLifeMonths
	}
	if category.DepStart != nil {
		payload["dep_start"] = *category.DepStart
	}
	if category.AssetAccCode != nil {
		payload["asset_acc_code"] = *category.AssetAccCode
	}
	if category.AccumDepAccCode != nil {
		payload["accum_dep_acc_code"] = *category.AccumDepAccCode
	}
	if category.DepExpenseAccCode != nil {
		payload["dep_expense_acc_code"] = *category.DepExpenseAccCode
	}

	if postJournal && asset.OffsetAccCode != nil {
		payload["offset_acc_code"] = *asset.OffsetAccCode
	}

	return payload
}

// MovementKind determines the kinds of movement occurring between two states
func MovementKind(before, after MovementState) []string {
	var kinds []string
	if before.RoomID != after.RoomID {
		kinds = append(kinds, "location")
	}
	if before.CustodianUserID != after.CustodianUserID {
		kinds = append(kinds, "custodian")
	}
	if before.Condition != after.Condition {
		kinds = append(kinds, "condition")
	}
	if before.SchoolID != "" && after.SchoolID != "" && before.SchoolID != after.SchoolID {
		kinds = append(kinds, "unit")
	}
	return kinds
}

// CanDispose checks if an asset can be submitted for disposal
func CanDispose(asset Asset, hasOpenDisposal bool) error {
	if asset.Status != "active" {
		return errors.New("hanya aset berstatus aktif yang dapat diajukan untuk pelepasan")
	}
	if hasOpenDisposal {
		return errors.New("aset memiliki pengajuan pelepasan yang masih menunggu approval")
	}
	return nil
}

// ApplyDisposalDecision updates asset status based on approval decision
// (AC-108-01: pending/rejected -> active; approved -> disposed + event disposal)
func ApplyDisposalDecision(asset Asset, decision string) (Asset, string) {
	switch strings.ToLower(decision) {
	case "approved":
		asset.Status = "disposed"
		return asset, "disposal"
	default:
		// pending, rejected, cancelled -> remain active
		return asset, ""
	}
}

var seqTokenRegex = regexp.MustCompile(`\{seq(?::(\d+))?\}`)

// FormatAssetCode generates code using template tokens {cat}, {yyyy}, {seq:N}
func FormatAssetCode(format, cat string, year, seq int) string {
	if format == "" {
		format = "{cat}-{yyyy}-{seq:4}"
	}

	res := strings.ReplaceAll(format, "{cat}", cat)
	res = strings.ReplaceAll(res, "{yyyy}", fmt.Sprintf("%04d", year))

	res = seqTokenRegex.ReplaceAllStringFunc(res, func(m string) string {
		sub := seqTokenRegex.FindStringSubmatch(m)
		width := 4
		if len(sub) > 1 && sub[1] != "" {
			if w, err := strconv.Atoi(sub[1]); err == nil && w > 0 {
				width = w
			}
		}
		return fmt.Sprintf("%0*d", width, seq)
	})

	return res
}

// ScanView produces QR scan response based on user role
// (AC-107-02: financial/asset roles see full details & book values; other staff see basic info only)
func ScanView(asset Asset, bookVal *BookValue, role string) map[string]any {
	highPrivilegeRoles := []string{
		"asset_officer", "kepala_sekolah", "finance_manager",
		"finance_admin", "admin", "superadmin", "kepala_yayasan",
	}

	canViewFinancials := slices.Contains(highPrivilegeRoles, role)

	base := map[string]any{
		"id":        asset.ID,
		"code":      asset.Code,
		"name":      asset.Name,
		"status":    asset.Status,
		"condition": asset.Condition,
		"room_id":   asset.RoomID,
		"room_name": asset.RoomName,
		"photo_url": asset.PhotoURL,
	}

	if !canViewFinancials {
		return base
	}

	// Full details for asset/finance/principal roles
	base["category_id"] = asset.CategoryID
	base["category_name"] = asset.CategoryName
	base["category_code"] = asset.CategoryCode
	base["custodian_user_id"] = asset.CustodianUserID
	base["custodian_name"] = asset.CustodianName
	base["serial_no"] = asset.SerialNo
	base["source"] = asset.Source
	base["acquired_on"] = asset.AcquiredOn
	base["cost_idr"] = asset.AcquisitionCostIDR
	base["residual_idr"] = asset.ResidualIDR
	base["activated_at"] = asset.ActivatedAt

	if bookVal != nil {
		base["book_value_idr"] = bookVal.BookValueIDR
		base["accumulated_idr"] = bookVal.AccumulatedIDR
		base["book_value_as_of"] = bookVal.AsOf
		base["last_period"] = bookVal.LastPeriod
	}

	return base
}
