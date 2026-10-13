package payable

import (
	"fmt"
	"strings"
	"time"
)

// Pure calculation functions for Accounts Payable (Utang Usaha 3-Way Match & Pembayaran).
// Strictly integer math, rupiah amounts, basis points.

// ThreeWayMatch validates vendor invoice line items against PO and Goods Receipt.
// A line passes if:
// 1. Qty billed <= Qty accepted in Goods Receipt.
// 2. Unit price billed <= Unit price in PO + toleranceIDR.
func ThreeWayMatch(lines []MatchInputLine, toleranceIDR int64) MatchResult {
	res := MatchResult{
		MatchStatus: "matched_ok",
		OK:          true,
		Lines:       make([]MatchResultLine, 0, len(lines)),
	}

	var discrepancies []string

	for _, l := range lines {
		qtyDiff := l.QtyInvoice - l.QtyAcceptedGR
		priceDiff := l.UnitPriceInvoice - l.UnitPricePO

		rl := MatchResultLine{
			POLineID:         l.POLineID,
			QtyInvoice:       l.QtyInvoice,
			QtyAcceptedGR:    l.QtyAcceptedGR,
			UnitPriceInvoice: l.UnitPriceInvoice,
			UnitPricePO:      l.UnitPricePO,
			QtyDiff:          qtyDiff,
			PriceDiff:        priceDiff,
			DiscrepancyType:  "none",
			OK:               true,
		}

		if qtyDiff > 0 {
			rl.DiscrepancyType = "qty_mismatch"
			rl.OK = false
			res.OK = false
			discrepancies = append(discrepancies, fmt.Sprintf("%s: kuantitas tagihan (%d) melebihi barang diterima (%d)", l.Description, l.QtyInvoice, l.QtyAcceptedGR))
		}

		if priceDiff > toleranceIDR {
			if rl.DiscrepancyType == "none" {
				rl.DiscrepancyType = "price_mismatch"
			} else {
				rl.DiscrepancyType = "over_billed"
			}
			rl.OK = false
			res.OK = false
			discrepancies = append(discrepancies, fmt.Sprintf("%s: harga satuan tagihan (Rp %d) melebihi PO (Rp %d) toleransi Rp %d", l.Description, l.UnitPriceInvoice, l.UnitPricePO, toleranceIDR))
		}

		res.Lines = append(res.Lines, rl)
	}

	if !res.OK {
		res.MatchStatus = "discrepancy"
		res.DiscrepancyNote = strings.Join(discrepancies, "; ")
	}

	return res
}

// CanSchedule checks whether scheduled payment amount does not exceed remaining unpaid invoice balance.
func CanSchedule(invoiceTotalIDR, paidAmountIDR, scheduledAmountIDR int64) bool {
	if scheduledAmountIDR <= 0 {
		return false
	}
	remaining := invoiceTotalIDR - paidAmountIDR
	return scheduledAmountIDR <= remaining
}

// DueDate calculates invoice due date based on payment terms in days.
func DueDate(invoiceDate time.Time, termsDays int) time.Time {
	if termsDays <= 0 {
		termsDays = 30
	}
	return invoiceDate.AddDate(0, 0, termsDays)
}

// AgingBucket calculates aging category and days overdue relative to asOf date.
// Buckets: "current", "1_30", "31_60", "61_90", "90_plus".
func AgingBucket(dueDate, asOf time.Time) (bucket string, daysOverdue int) {
	diffDays := int(asOf.Sub(dueDate).Hours() / 24)
	if diffDays <= 0 {
		return "current", 0
	}
	if diffDays <= 30 {
		return "1_30", diffDays
	}
	if diffDays <= 60 {
		return "31_60", diffDays
	}
	if diffDays <= 90 {
		return "61_90", diffDays
	}
	return "90_plus", diffDays
}

// BuildInvoiceJournal constructs double-entry journal lines for auto-journaling vendor invoice:
// Debit Expense: subtotal
// Debit VAT In: vat
// Credit AP: total
// Credit Withholding Tax Payable: wht
// Total Debits (subtotal + vat) MUST EQUAL Total Credits (total + wht) where total = subtotal + vat - wht.
func BuildInvoiceJournal(schoolID, invoiceNo, date, vendorID string, subtotal, vat, wht, total int64, apAcc, vatAcc, expAcc, whtAcc string) map[string]any {
	lines := []map[string]any{
		{
			"acc_code":   expAcc,
			"debit_idr":  subtotal,
			"credit_idr": int64(0),
			"vendor_id":  vendorID,
		},
	}

	if vat > 0 && vatAcc != "" {
		lines = append(lines, map[string]any{
			"acc_code":   vatAcc,
			"debit_idr":  vat,
			"credit_idr": int64(0),
			"vendor_id":  vendorID,
		})
	}

	lines = append(lines, map[string]any{
		"acc_code":   apAcc,
		"debit_idr":  int64(0),
		"credit_idr": total,
		"vendor_id":  vendorID,
	})

	if wht > 0 && whtAcc != "" {
		lines = append(lines, map[string]any{
			"acc_code":   whtAcc,
			"debit_idr":  int64(0),
			"credit_idr": wht,
			"vendor_id":  vendorID,
		})
	}

	return map[string]any{
		"school_id":   schoolID,
		"source":      "payable",
		"reference":   "ap:inv:" + invoiceNo,
		"trans_date":  date,
		"description": "Faktur Pembelian: " + invoiceNo,
		"lines":       lines,
	}
}

// BuildPaymentJournal constructs journal lines for payment execution:
// Debit AP: amount
// Credit Bank: amount
func BuildPaymentJournal(schoolID, paymentRef, date, vendorID string, amount int64, apAcc, bankAcc string) map[string]any {
	return map[string]any{
		"school_id":   schoolID,
		"source":      "payable",
		"reference":   "ap:pay:" + paymentRef,
		"trans_date":  date,
		"description": "Pembayaran Utang Usaha: " + paymentRef,
		"lines": []map[string]any{
			{
				"acc_code":   apAcc,
				"debit_idr":  amount,
				"credit_idr": int64(0),
				"vendor_id":  vendorID,
			},
			{
				"acc_code":   bankAcc,
				"debit_idr":  int64(0),
				"credit_idr": amount,
			},
		},
	}
}
