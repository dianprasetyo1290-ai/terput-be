package procurement

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrOverBudget = errors.New("Melebihi sisa anggaran pos")
)

type PRLineCalc struct {
	Qty              int64
	EstUnitPriceIDR  int64
	QtyOrdered       int64
	BudgetLineKey    *string
}

type POLineReceiptCalc struct {
	POLineID       string
	QtyOrdered     int64
	QtyAcceptedNet int64 // accepted - returned
}

type ReceiptLineInput struct {
	POLineID    string
	ItemID      string
	QtyReceived int64
	QtyAccepted int64
	QtyRejected int64
}

// PRTotal computes total estimated IDR for purchase request lines.
func PRTotal(lines []PRLineCalc) int64 {
	var total int64
	for _, l := range lines {
		total += l.Qty * l.EstUnitPriceIDR
	}
	return total
}

// ApprovalFlags evaluates budget check result and returns approval condition flags or rejection.
// AC-104-01: hook nil -> []; policy "extra_approval" + over -> ["over_budget"]; policy "reject" + over -> ErrOverBudget.
func ApprovalFlags(res *BudgetCheckResult) ([]string, error) {
	if res == nil {
		return nil, nil
	}
	if !res.Over {
		return nil, nil
	}
	if res.Policy == "reject" {
		return nil, ErrOverBudget
	}
	return []string{"over_budget"}, nil
}

// PRStatusAfterOrder determines whether a PR is completely or partially ordered.
func PRStatusAfterOrder(lines []PRLineCalc) string {
	if len(lines) == 0 {
		return "approved"
	}
	allOrdered := true
	anyOrdered := false
	for _, l := range lines {
		if l.QtyOrdered > 0 {
			anyOrdered = true
		}
		if l.QtyOrdered < l.Qty {
			allOrdered = false
		}
	}
	if allOrdered {
		return "ordered"
	}
	if anyOrdered {
		return "partially_ordered"
	}
	return "approved"
}

// POReceiptStatus determines overall PO receipt status and remaining unreceived quantities.
// AC-105-01: 10 ordered, receive 6 -> "partially_received", remaining 4;
// 10/10 -> "received"; retur 2 from 10 -> "partially_received", remaining 2.
func POReceiptStatus(lines []POLineReceiptCalc) (status string, remaining map[string]int64) {
	remaining = make(map[string]int64)
	allReceived := true
	anyReceived := false

	for _, l := range lines {
		rem := l.QtyOrdered - l.QtyAcceptedNet
		if rem < 0 {
			rem = 0
		}
		remaining[l.POLineID] = rem
		if l.QtyAcceptedNet > 0 {
			anyReceived = true
		}
		if l.QtyAcceptedNet < l.QtyOrdered {
			allReceived = false
		}
	}

	if allReceived && len(lines) > 0 {
		return "received", remaining
	}
	if anyReceived {
		return "partially_received", remaining
	}
	return "approved", remaining
}

// SplitGoodsReceipt partitions received lines by item tracking mode ('stock', 'asset', 'none').
// AC-105-02: laptop 'asset' -> assetLines (hook called once with qty), no stock movement;
// chair 'stock' accepted 6 -> stockMoves 'in' 6, rejected does not enter stock.
func SplitGoodsReceipt(lines []ReceiptLineInput, itemTracking map[string]string) (stockMoves []ReceiptLineInput, assetLines []ReceiptLineInput, noneLines []ReceiptLineInput) {
	for _, l := range lines {
		tracking := itemTracking[l.ItemID]
		switch tracking {
		case "asset":
			if l.QtyAccepted > 0 {
				assetLines = append(assetLines, l)
			}
		case "stock":
			if l.QtyAccepted > 0 {
				stockMoves = append(stockMoves, l)
			}
		default: // "none" or services
			if l.QtyAccepted > 0 {
				noneLines = append(noneLines, l)
			}
		}
	}
	return stockMoves, assetLines, noneLines
}

type ExistingVendorInfo struct {
	VendorID    string
	NPWPBidx    *string
	AccountBidx []string
}

// VendorDuplicates matches candidate duplicates by NPWP blind index or bank account blind index.
func VendorDuplicates(npwpBidx string, accountBidx string, existing []ExistingVendorInfo) []string {
	var duplicates []string
	seen := make(map[string]bool)

	for _, v := range existing {
		isDup := false
		if npwpBidx != "" && v.NPWPBidx != nil && *v.NPWPBidx == npwpBidx {
			isDup = true
		}
		if accountBidx != "" {
			for _, acc := range v.AccountBidx {
				if acc == accountBidx {
					isDup = true
					break
				}
			}
		}
		if isDup && !seen[v.VendorID] {
			seen[v.VendorID] = true
			duplicates = append(duplicates, v.VendorID)
		}
	}
	return duplicates
}

// OnTime compares received date with expected delivery date.
func OnTime(expectedDate *time.Time, receivedOn time.Time) *bool {
	if expectedDate == nil {
		return nil
	}
	onTime := !receivedOn.After(*expectedDate)
	return &onTime
}

// FormatDocNumber helper for procurement numbers (PR, PO, GR).
func FormatDocNumber(format string, year int, seq int) string {
	if format == "" {
		return fmt.Sprintf("DOC-%04d-%04d", year, seq)
	}
	return formatPattern(format, year, seq)
}

func formatPattern(format string, year, seq int) string {
	res := format
	res = strings.ReplaceAll(res, "{yyyy}", fmt.Sprintf("%04d", year))
	res = strings.ReplaceAll(res, "{yy}", fmt.Sprintf("%02d", year%100))
	res = strings.ReplaceAll(res, "{seq:4}", fmt.Sprintf("%04d", seq))
	res = strings.ReplaceAll(res, "{seq}", fmt.Sprintf("%d", seq))
	return res
}
