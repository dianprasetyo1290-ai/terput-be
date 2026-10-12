// Package wallet holds the pure student-wallet rules (PRD menu 124-125):
// spending checks (BR-WAL-01), top-up caps (OQ-29), the daily liability
// journal (BR-WAL-02, FR-125-006), reconciliation (NFR-OPS-04) and the exit
// disposition (FR-125-005). No I/O; all money is integer rupiah.
package wallet

import (
	"errors"
	"fmt"
	"time"

	"skci/billing/internal/model"
)

// Wallet kinds.
const (
	KindAllowance = "allowance"
	KindSavings   = "savings"
)

// Kinds lists every wallet kind in display order.
var Kinds = []string{KindAllowance, KindSavings}

// ValidKind reports whether k is a known wallet kind.
func ValidKind(k string) bool { return k == KindAllowance || k == KindSavings }

// Movement kinds (wallet_movements.kind).
const (
	MoveTopupGateway      = "topup_gateway"
	MoveTopupCashier      = "topup_cashier"
	MovePosSale           = "pos_sale"
	MovePosVoid           = "pos_void"
	MoveSavingsDeposit    = "savings_deposit"
	MoveSavingsWithdrawal = "savings_withdrawal"
	MoveTransferIn        = "transfer_in"
	MoveTransferOut       = "transfer_out"
	MoveRefundExit        = "refund_exit"
)

var (
	ErrDailyLimit   = errors.New("Melebihi batas jajan harian")
	ErrInsufficient = errors.New("Saldo uang saku tidak mencukupi")
	ErrMaxBalance   = errors.New("Top-up melebihi batas saldo maksimum sekolah")
	ErrAmount       = errors.New("Nominal harus lebih dari 0")
)

// CanSpend applies BR-WAL-01: the balance must cover the amount and, when the
// parent set a daily limit, today's spending plus the amount must stay within it.
func CanSpend(balance int64, dailyLimit *int64, spentToday, amount int64) error {
	if amount <= 0 {
		return ErrAmount
	}
	if dailyLimit != nil && spentToday+amount > *dailyLimit {
		return ErrDailyLimit
	}
	if amount > balance {
		return ErrInsufficient
	}
	return nil
}

// movementOrder fixes the journal line order (map iteration is random).
var movementOrder = []string{MoveTopupGateway, MoveTopupCashier, MoveSavingsDeposit, MovePosSale, MovePosVoid,
	MoveSavingsWithdrawal, MoveRefundExit, MoveTransferIn, MoveTransferOut}

var knownMovement = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range movementOrder {
		m[k] = true
	}
	return m
}()

// Movement is the subset of a wallet movement the pure rules need.
type Movement struct {
	Kind      string
	AmountIDR int64 // + in, - out
	CreatedAt time.Time
}

// SpentToday is POS spending (sales minus voids) on now's calendar day in loc.
func SpentToday(movements []Movement, now time.Time, loc *time.Location) int64 {
	y, m, d := now.In(loc).Date()
	var spent int64
	for _, mv := range movements {
		my, mm, md := mv.CreatedAt.In(loc).Date()
		if my != y || mm != m || md != d {
			continue
		}
		switch mv.Kind {
		case MovePosSale, MovePosVoid:
			spent -= mv.AmountIDR
		}
	}
	if spent < 0 {
		return 0
	}
	return spent
}

// CanTopup checks a credit against the school's maximum balance (nil = no cap).
func CanTopup(balance, amount int64, max *int64) error {
	if amount <= 0 {
		return ErrAmount
	}
	if max != nil && balance+amount > *max {
		return ErrMaxBalance
	}
	return nil
}

// Accounts is a school's wallet account mapping (wallet_accounts).
type Accounts struct {
	AllowanceLiability string `json:"allowance_liability_acc"`
	SavingsLiability   string `json:"savings_liability_acc"`
	Cash               string `json:"cash_acc"`
	TenantPayable      string `json:"tenant_payable_acc"`
	CanteenRevenue     string `json:"canteen_revenue_acc"`
	RentRevenue        string `json:"rent_revenue_acc"`
	TenantReceivable   string `json:"tenant_receivable_acc"`
}

// Liability returns the liability account of a wallet kind.
func (a Accounts) Liability(kind string) string {
	if kind == KindSavings {
		return a.SavingsLiability
	}
	return a.AllowanceLiability
}

// DailyAgg is one day of movements: per wallet kind, the signed sum per
// movement kind, plus the gateway fee the school bore on top-ups that day.
type DailyAgg struct {
	ByKind              map[string]map[string]int64
	GatewaySchoolFeeIDR int64
}

// DailyJournal builds a day's wallet journal (FR-125-006, IF-OPS-01). Every
// movement changes a liability (dana titipan) account by its signed amount;
// the counter-account depends on the movement kind:
//   - topup_gateway: gateway clearing (total - fee) + fee expense when the
//     school bears the fee,
//   - topup_cashier, savings_deposit, savings_withdrawal, refund_exit: cash,
//   - pos_sale / pos_void: tenant payable,
//   - transfer_in / transfer_out: none (liability to liability; a transfer
//     within one kind nets to zero and produces no line).
//
// Amounts are netted per account and emitted on the side their sign says. A
// day without net effect returns no lines and no error.
func DailyJournal(agg DailyAgg, acc Accounts, gw model.GatewayAccounts) ([]model.JournalLine, error) {
	net := map[string]int64{} // account -> debit-positive amount
	var order []string
	add := func(code string, debit int64) {
		if _, ok := net[code]; !ok {
			order = append(order, code)
		}
		net[code] += debit
	}
	var gatewayTopup int64
	for _, kind := range Kinds {
		sums := agg.ByKind[kind]
		if len(sums) == 0 {
			continue
		}
		for mk := range sums {
			if !knownMovement[mk] {
				return nil, fmt.Errorf("jenis mutasi dompet tidak dikenal: %s", mk)
			}
		}
		var liability int64
		for _, mk := range movementOrder {
			amt := sums[mk]
			switch mk {
			case MoveTopupGateway:
				gatewayTopup += amt
			case MoveTopupCashier, MoveSavingsDeposit, MoveSavingsWithdrawal, MoveRefundExit:
				add(acc.Cash, amt)
			case MovePosSale, MovePosVoid:
				add(acc.TenantPayable, amt)
			}
			liability += amt
		}
		add(acc.Liability(kind), -liability)
	}
	if gatewayTopup != 0 || agg.GatewaySchoolFeeIDR != 0 {
		if gw.Clearing == "" || gw.FeeExpense == "" {
			return nil, errors.New("akun payment gateway belum dipetakan")
		}
		add(gw.Clearing, gatewayTopup-agg.GatewaySchoolFeeIDR)
		add(gw.FeeExpense, agg.GatewaySchoolFeeIDR)
	}
	var lines []model.JournalLine
	for _, code := range order {
		v := net[code]
		switch {
		case v > 0:
			lines = append(lines, model.JournalLine{AccCode: code, Description: journalLabel(code, acc, gw), DebitIDR: v})
		case v < 0:
			lines = append(lines, model.JournalLine{AccCode: code, Description: journalLabel(code, acc, gw), CreditIDR: -v})
		}
	}
	if len(lines) == 0 {
		return nil, nil
	}
	if err := model.ValidateJournal(lines); err != nil {
		return nil, err
	}
	return lines, nil
}

func journalLabel(code string, acc Accounts, gw model.GatewayAccounts) string {
	switch code {
	case acc.AllowanceLiability:
		return "Dana titipan uang saku siswa"
	case acc.SavingsLiability:
		return "Dana titipan tabungan siswa"
	case acc.Cash:
		return "Kas dompet siswa"
	case acc.TenantPayable:
		return "Utang ke tenant kantin"
	case gw.Clearing:
		return "Top-up dompet via payment gateway"
	case gw.FeeExpense:
		return "Biaya payment gateway top-up dompet"
	}
	return "Jurnal dompet siswa"
}

// Reconciliation is the result of comparing wallet balances with the ledger.
type Reconciliation struct {
	Status  string `json:"status"` // matched | mismatch
	DiffIDR int64  `json:"diff_idr"`
}

// Reconcile compares the sum of wallet balances with the liability account
// balance (both positive rupiah). Any difference, even 1 rupiah, is a mismatch.
func Reconcile(walletTotal, ledger int64) Reconciliation {
	diff := ledger - walletTotal
	if diff == 0 {
		return Reconciliation{Status: "matched"}
	}
	return Reconciliation{Status: "mismatch", DiffIDR: diff}
}

// WalletBalance is one wallet as the exit disposition sees it.
type WalletBalance struct {
	WalletID   string
	Kind       string
	BalanceIDR int64
}

// ExitRefund is one refund request to create when a student exits.
type ExitRefund struct {
	WalletID  string
	Kind      string
	AmountIDR int64
}

// ExitDisposition returns one refund per wallet that still holds money
// (AC-125-02); empty wallets produce nothing.
func ExitDisposition(wallets []WalletBalance) []ExitRefund {
	var out []ExitRefund
	for _, w := range wallets {
		if w.BalanceIDR > 0 {
			out = append(out, ExitRefund{WalletID: w.WalletID, Kind: w.Kind, AmountIDR: w.BalanceIDR})
		}
	}
	return out
}
