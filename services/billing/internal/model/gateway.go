package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type FeeKind string

const (
	FeeFlat    FeeKind = "flat"
	FeePercent FeeKind = "percent"
)

type FeeBearer string

const (
	FeeBearerParent FeeBearer = "parent"
	FeeBearerSchool FeeBearer = "school"
)

type GatewayMethod struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`
	Kind         string    `json:"kind"` // va, qris, ewallet
	ProviderCode string    `json:"provider_code"`
	Label        string    `json:"label"`
	Enabled      bool      `json:"enabled"`
	FeeKind      FeeKind   `json:"fee_kind"`
	FeeFlatIDR   int64     `json:"fee_flat_idr"`
	FeeBP        int       `json:"fee_bp"` // 1 bp = 0.01%
	FeeBearer    FeeBearer `json:"fee_bearer"`
	ValidMinutes int       `json:"valid_minutes"`
	Sort         int       `json:"sort"`
}

// Fee calculates transaction fee for given bills amount.
// Flat: returns FeeFlatIDR.
// Percent: floor((amount * FeeBP) / 10000).
func Fee(method GatewayMethod, amount int64) int64 {
	if amount <= 0 {
		return 0
	}
	switch method.FeeKind {
	case FeeFlat:
		if method.FeeFlatIDR < 0 {
			return 0
		}
		return method.FeeFlatIDR
	case FeePercent:
		if method.FeeBP <= 0 {
			return 0
		}
		return (amount * int64(method.FeeBP)) / 10000
	default:
		return 0
	}
}

// TransactionTotals holds the breakdown of payment amounts.
type TransactionTotals struct {
	BillsAmountIDR int64     `json:"bills_amount_idr"`
	FeeIDR         int64     `json:"fee_idr"`
	TotalIDR       int64     `json:"total_idr"`
	FeeBearer      FeeBearer `json:"fee_bearer"`
}

// Totals computes total charged and allocation according to fee bearer setting.
// AC-28-02: QRIS ditanggung orang tua: total = 1.000.000 + fee, alokasi tagihan 1.000.000.
// Ditanggung sekolah: total = 1.000.000, fee dicatat beban sekolah.
func Totals(billsAmount int64, method GatewayMethod) (TransactionTotals, error) {
	if billsAmount <= 0 {
		return TransactionTotals{}, errors.New("bills amount must be positive")
	}
	fee := Fee(method, billsAmount)
	totals := TransactionTotals{
		BillsAmountIDR: billsAmount,
		FeeIDR:         fee,
		FeeBearer:      method.FeeBearer,
	}
	if method.FeeBearer == FeeBearerParent {
		totals.TotalIDR = billsAmount + fee
	} else {
		totals.TotalIDR = billsAmount
	}
	return totals, nil
}

type OpenBill struct {
	BillID            string    `json:"bill_id"`
	OutstandingIDR    int64     `json:"outstanding_idr"`
	DueDate           time.Time `json:"due_date"`
	ComponentPriority int       `json:"component_priority"`
}

type BillAllocationResult struct {
	BillID    string `json:"bill_id"`
	AmountIDR int64  `json:"amount_idr"`
}

// Allocate distributes payment amount to open bills according to the selected rule:
// - "earliest_due": sorts by DueDate asc (BR-FIN-03)
// - "component_priority": sorts by ComponentPriority asc
// - "payer_choice": keeps the order provided
// Any remainder after satisfying all open bills becomes deposit (BR-FIN-04).
func Allocate(amount int64, openBills []OpenBill, rule string) ([]BillAllocationResult, int64, error) {
	if amount <= 0 {
		return nil, 0, errors.New("payment amount must be positive")
	}
	bills := make([]OpenBill, len(openBills))
	copy(bills, openBills)

	switch rule {
	case "earliest_due":
		// Bills without a due date go last.
		sort.SliceStable(bills, func(i, j int) bool {
			di, dj := bills[i].DueDate, bills[j].DueDate
			if di.IsZero() != dj.IsZero() {
				return dj.IsZero()
			}
			return di.Before(dj)
		})
	case "component_priority":
		sort.SliceStable(bills, func(i, j int) bool {
			return bills[i].ComponentPriority < bills[j].ComponentPriority
		})
	case "payer_choice":
		// keep the order the payer chose
	default:
		return nil, 0, fmt.Errorf("unknown allocation rule %q", rule)
	}

	remaining := amount
	var allocations []BillAllocationResult
	for _, b := range bills {
		if remaining <= 0 {
			break
		}
		if b.OutstandingIDR <= 0 {
			continue
		}
		alloc := b.OutstandingIDR
		if remaining < alloc {
			alloc = remaining
		}
		allocations = append(allocations, BillAllocationResult{
			BillID:    b.BillID,
			AmountIDR: alloc,
		})
		remaining -= alloc
	}

	deposit := int64(0)
	if remaining > 0 {
		deposit = remaining
	}
	return allocations, deposit, nil
}

// FormatReceiptNo returns standard formatted receipt string.
func FormatReceiptNo(year int, seq int) string {
	return fmt.Sprintf("RCP-%d-%05d", year, seq)
}

type ReconcileTx struct {
	TXNo        string `json:"tx_no"`
	ProviderRef string `json:"provider_ref"`
	AmountIDR   int64  `json:"amount_idr"`
	FeeIDR      int64  `json:"fee_idr"`
}

type ReconcileItem struct {
	ProviderRef string `json:"provider_ref"`
	AmountIDR   int64  `json:"amount_idr"`
	FeeIDR      int64  `json:"fee_idr"`
}

type ReconcileExceptionKind string

const (
	ExceptionMissingInSettlement ReconcileExceptionKind = "missing_in_settlement"
	ExceptionMissingInSekoci     ReconcileExceptionKind = "missing_in_sekoci"
	ExceptionAmountMismatch      ReconcileExceptionKind = "amount_mismatch"
	ExceptionBankUnmatched       ReconcileExceptionKind = "bank_unmatched"
)

type ReconcileException struct {
	Kind   ReconcileExceptionKind `json:"kind"`
	Ref    string                 `json:"ref"`
	Detail map[string]any         `json:"detail"`
}

type ReconcileMatch struct {
	TXNo        string `json:"tx_no"`
	ProviderRef string `json:"provider_ref"`
	AmountIDR   int64  `json:"amount_idr"`
}

type ReconcileResult struct {
	Matched    []ReconcileMatch     `json:"matched"`
	Exceptions []ReconcileException `json:"exceptions"`
}

// Reconcile compares successful transactions in SEKOCI with settlement report items.
// AC-31-01: TX-012 without match in settlement -> missing_in_settlement.
func Reconcile(successTxs []ReconcileTx, settlementItems []ReconcileItem) ReconcileResult {
	settleMap := make(map[string]ReconcileItem, len(settlementItems))
	matchedSettle := make(map[string]bool, len(settlementItems))
	for _, it := range settlementItems {
		settleMap[it.ProviderRef] = it
	}

	var res ReconcileResult

	for _, tx := range successTxs {
		it, found := settleMap[tx.ProviderRef]
		if !found {
			res.Exceptions = append(res.Exceptions, ReconcileException{
				Kind: ExceptionMissingInSettlement,
				Ref:  tx.TXNo,
				Detail: map[string]any{
					"provider_ref": tx.ProviderRef,
					"amount_idr":   tx.AmountIDR,
				},
			})
			continue
		}
		matchedSettle[tx.ProviderRef] = true
		if tx.AmountIDR != it.AmountIDR {
			res.Exceptions = append(res.Exceptions, ReconcileException{
				Kind: ExceptionAmountMismatch,
				Ref:  tx.TXNo,
				Detail: map[string]any{
					"provider_ref":      tx.ProviderRef,
					"sekoci_amount_idr": tx.AmountIDR,
					"settle_amount_idr": it.AmountIDR,
					"diff_idr":          it.AmountIDR - tx.AmountIDR,
				},
			})
		} else {
			res.Matched = append(res.Matched, ReconcileMatch{
				TXNo:        tx.TXNo,
				ProviderRef: tx.ProviderRef,
				AmountIDR:   tx.AmountIDR,
			})
		}
	}

	for _, it := range settlementItems {
		if !matchedSettle[it.ProviderRef] {
			res.Exceptions = append(res.Exceptions, ReconcileException{
				Kind: ExceptionMissingInSekoci,
				Ref:  it.ProviderRef,
				Detail: map[string]any{
					"amount_idr": it.AmountIDR,
				},
			})
		}
	}

	return res
}

// MethodShare calculates distribution of payment methods in basis points (1 bp = 0.01%).
// AC-26-01: VA 40 / QRIS 10 -> 8000 bp (80%) / 2000 bp (20%).
func MethodShare(counts map[string]int) map[string]int {
	total := 0
	for _, c := range counts {
		if c > 0 {
			total += c
		}
	}
	out := make(map[string]int, len(counts))
	if total == 0 {
		for k := range counts {
			out[k] = 0
		}
		return out
	}
	for k, c := range counts {
		if c <= 0 {
			out[k] = 0
		} else {
			out[k] = (c * 10000) / total
		}
	}
	return out
}

type BankLine struct {
	ID          string    `json:"id"`
	Account     string    `json:"account"`
	PostedOn    time.Time `json:"posted_on"`
	AmountIDR   int64     `json:"amount_idr"`
	Description string    `json:"description"`
}

type BankPaymentRecord struct {
	ID        string    `json:"id"`
	Date      time.Time `json:"date"`
	AmountIDR int64     `json:"amount_idr"`
	Ref       string    `json:"ref"`
}

type BankMatchResult struct {
	MatchedLines []string             `json:"matched_lines"`
	Matches      []BankMatch          `json:"matches"`
	Exceptions   []ReconcileException `json:"exceptions"`
}

// BankReconcile matches imported bank lines to expected credits (AC-31-02).
// A line matches a record of the same amount whose reference appears in the
// line description; failing that, the nearest-dated record of the same
// amount within toleranceDays. Each record matches at most one line.
func BankReconcile(lines []BankLine, records []BankPaymentRecord, toleranceDays int) BankMatchResult {
	var res BankMatchResult
	used := make(map[string]bool)
	tolerance := time.Duration(toleranceDays) * 24 * time.Hour
	for _, line := range lines {
		pick := -1
		var best time.Duration
		for i, rec := range records {
			if used[rec.ID] || rec.AmountIDR != line.AmountIDR {
				continue
			}
			if rec.Ref != "" && strings.Contains(strings.ToUpper(line.Description), strings.ToUpper(rec.Ref)) {
				pick = i
				break
			}
			diff := line.PostedOn.Sub(rec.Date)
			if diff < 0 {
				diff = -diff
			}
			if diff <= tolerance && (pick < 0 || diff < best) {
				pick, best = i, diff
			}
		}
		if pick >= 0 {
			used[records[pick].ID] = true
			res.MatchedLines = append(res.MatchedLines, line.ID)
			res.Matches = append(res.Matches, BankMatch{LineID: line.ID, RecordID: records[pick].ID, Ref: records[pick].Ref})
			continue
		}
		res.Exceptions = append(res.Exceptions, ReconcileException{
			Kind: ExceptionBankUnmatched,
			Ref:  line.ID,
			Detail: map[string]any{
				"account":     line.Account,
				"amount_idr":  line.AmountIDR,
				"description": line.Description,
				"posted_on":   line.PostedOn.Format("2006-01-02"),
			},
		})
	}
	return res
}

// BankMatch pairs one bank line with the record it settled.
type BankMatch struct {
	LineID   string `json:"line_id"`
	RecordID string `json:"record_id"`
	Ref      string `json:"ref"`
}

// GatewayAccounts is a school's account mapping for the gateway journal.
type GatewayAccounts struct {
	Clearing   string `json:"clearing_acc"`
	FeeExpense string `json:"fee_expense_acc"`
	Receivable string `json:"receivable_acc"`
	Deposit    string `json:"deposit_acc"`
}

// JournalTx is one successful gateway transaction as the journal sees it.
type JournalTx struct {
	BillsIDR   int64
	FeeIDR     int64
	TotalIDR   int64
	DepositIDR int64
	FeeBearer  FeeBearer
}

// JournalLine is one journal line in integer rupiah.
type JournalLine struct {
	AccCode     string `json:"acc_code"`
	Description string `json:"description"`
	DebitIDR    int64  `json:"debit_idr"`
	CreditIDR   int64  `json:"credit_idr"`
}

// GatewayJournalLines builds a day's gateway journal. The gateway keeps its
// fee, so the clearing account receives total - fee. When the school bears
// the fee it is booked as expense; when the parent bears it, total - fee is
// exactly the bills amount. The bills amount is credited to receivables,
// except the part parked as deposit (BR-FIN-04), which is a liability.
func GatewayJournalLines(acc GatewayAccounts, txs []JournalTx) ([]JournalLine, error) {
	var clearing, feeExpense, receivable, deposit int64
	for _, t := range txs {
		if t.TotalIDR <= 0 || t.FeeIDR < 0 || t.DepositIDR < 0 || t.DepositIDR > t.BillsIDR {
			return nil, errors.New("invalid gateway transaction amounts")
		}
		clearing += t.TotalIDR - t.FeeIDR
		if t.FeeBearer == FeeBearerSchool {
			feeExpense += t.FeeIDR
		}
		receivable += t.BillsIDR - t.DepositIDR
		deposit += t.DepositIDR
	}
	if len(txs) == 0 {
		return nil, errors.New("no transactions to journal")
	}
	lines := []JournalLine{{AccCode: acc.Clearing, Description: "Penerimaan payment gateway", DebitIDR: clearing}}
	if feeExpense > 0 {
		lines = append(lines, JournalLine{AccCode: acc.FeeExpense, Description: "Biaya payment gateway", DebitIDR: feeExpense})
	}
	if receivable > 0 {
		lines = append(lines, JournalLine{AccCode: acc.Receivable, Description: "Pelunasan piutang tagihan", CreditIDR: receivable})
	}
	if deposit > 0 {
		lines = append(lines, JournalLine{AccCode: acc.Deposit, Description: "Deposit kelebihan bayar", CreditIDR: deposit})
	}
	if err := ValidateJournal(lines); err != nil {
		return nil, err
	}
	return lines, nil
}

// ValidateJournal checks a journal has two or more one-sided, non-negative
// lines with an account code and equal debit and credit totals.
func ValidateJournal(lines []JournalLine) error {
	if len(lines) < 2 {
		return errors.New("jurnal minimal dua baris")
	}
	var debit, credit int64
	for _, l := range lines {
		if strings.TrimSpace(l.AccCode) == "" {
			return errors.New("kode akun wajib diisi")
		}
		if l.DebitIDR < 0 || l.CreditIDR < 0 || (l.DebitIDR > 0) == (l.CreditIDR > 0) {
			return errors.New("setiap baris harus berisi debit atau kredit saja")
		}
		debit += l.DebitIDR
		credit += l.CreditIDR
	}
	if debit != credit {
		return fmt.Errorf("jurnal tidak seimbang: debit %d, kredit %d", debit, credit)
	}
	return nil
}
