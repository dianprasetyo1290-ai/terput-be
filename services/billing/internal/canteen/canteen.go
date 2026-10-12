// Package canteen holds the pure canteen/POS rules (PRD menu 123-126):
// offline sale acceptance (NFR-OPS-02), idempotent sync (FR-124-005), void
// rules (FR-124-004), revenue-share/rent settlement (FR-126-002, AC-126-01),
// sales recap (FR-126-001) and settlement journals (FR-126-003). No I/O; all
// money is integer rupiah.
package canteen

import (
	"errors"
	"sort"
	"strings"
	"time"

	"skci/billing/internal/model"
	"skci/billing/internal/wallet"
)

// Payment methods and scheme kinds.
const (
	PayWallet = "wallet"
	PayCash   = "cash"

	SchemeRevenueShare = "revenue_share"
	SchemeRent         = "rent"

	BaseWallet = "wallet"
	BaseAll    = "all"

	SaleCompleted = "completed"
	SaleVoided    = "voided"
)

// OfflineResult says how a synced offline sale is recorded.
type OfflineResult struct {
	Offline  bool
	LateSync bool
}

// OfflineAccept classifies an offline sale recorded at recordedAt and synced
// at now: within maxMinutes it is a normal offline sale, later it is still
// accepted (the cash was taken) but flagged late_sync for review.
func OfflineAccept(recordedAt, now time.Time, maxMinutes int) OfflineResult {
	if now.Sub(recordedAt) > time.Duration(maxMinutes)*time.Minute {
		return OfflineResult{Offline: true, LateSync: true}
	}
	return OfflineResult{Offline: true}
}

// MergeSync splits a batch of client transaction ids into new ones and
// duplicates (already stored, or repeated inside the batch).
func MergeSync(existing map[string]bool, batch []string) (fresh, duplicate []string) {
	seen := map[string]bool{}
	for _, id := range batch {
		if existing[id] || seen[id] {
			duplicate = append(duplicate, id)
			continue
		}
		seen[id] = true
		fresh = append(fresh, id)
	}
	return fresh, duplicate
}

var (
	ErrAlreadyVoided  = errors.New("Transaksi sudah dibatalkan")
	ErrSettled        = errors.New("Transaksi sudah masuk penyelesaian bagi hasil")
	ErrReasonRequired = errors.New("Alasan pembatalan wajib diisi")
)

// SaleState is what CanVoid needs to know about a sale.
type SaleState struct {
	Status       string
	SettlementID string
}

// CanVoid enforces FR-124-004: a sale is voided once, never after it entered
// a settlement, and always with a reason.
func CanVoid(s SaleState, reason string) error {
	if s.Status == SaleVoided {
		return ErrAlreadyVoided
	}
	if s.SettlementID != "" {
		return ErrSettled
	}
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	return nil
}

// Scheme is one tenant scheme row.
type Scheme struct {
	ID            string
	Kind          string
	ShareBP       int
	ShareBase     string
	RentIDR       int64
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
}

// Settlement is the computed split for a period.
type Settlement struct {
	SchemeID        string `json:"scheme_id"`
	SchemeKind      string `json:"scheme_kind"`
	WalletSalesIDR  int64  `json:"wallet_sales_idr"`
	CashSalesIDR    int64  `json:"cash_sales_idr"`
	SchoolShareIDR  int64  `json:"school_share_idr"`
	TenantPayoutIDR int64  `json:"tenant_payout_idr"`
}

var (
	ErrSplitPeriod  = errors.New("Skema berganti di tengah periode, pecah periode penyelesaian")
	ErrRentPeriod   = errors.New("Skema sewa wajib satu bulan kalender penuh")
	ErrPeriod       = errors.New("Periode tidak valid")
	ErrSchemeConfig = errors.New("Skema tenant tidak lengkap")
)

// SchoolShare is the single rounding rule of the revenue share: the school's
// part is rounded down to the rupiah (the tenant keeps the fraction).
func SchoolShare(base int64, shareBP int) int64 {
	if base <= 0 || shareBP <= 0 {
		return 0
	}
	return base * int64(shareBP) / 10000
}

// Settle computes the school share and tenant payout for one tenant period.
// The tenant receives wallet sales (it already holds its cash sales) minus
// the school share, so a share based on all sales with large cash takings
// yields a negative payout (the tenant owes the school).
func Settle(s Scheme, walletSales, cashSales int64, from, to time.Time) (Settlement, error) {
	if to.Before(from) || walletSales < 0 || cashSales < 0 {
		return Settlement{}, ErrPeriod
	}
	if from.Before(s.EffectiveFrom) || (s.EffectiveTo != nil && to.After(*s.EffectiveTo)) {
		return Settlement{}, ErrSplitPeriod
	}
	out := Settlement{SchemeID: s.ID, SchemeKind: s.Kind, WalletSalesIDR: walletSales, CashSalesIDR: cashSales}
	switch s.Kind {
	case SchemeRevenueShare:
		base := walletSales
		switch s.ShareBase {
		case BaseWallet:
		case BaseAll:
			base += cashSales
		default:
			return Settlement{}, ErrSchemeConfig
		}
		out.SchoolShareIDR = SchoolShare(base, s.ShareBP)
	case SchemeRent:
		if from.Day() != 1 || !to.Equal(from.AddDate(0, 1, -1)) {
			return Settlement{}, ErrRentPeriod
		}
		out.SchoolShareIDR = s.RentIDR
	default:
		return Settlement{}, ErrSchemeConfig
	}
	out.TenantPayoutIDR = walletSales - out.SchoolShareIDR
	return out, nil
}

// SaleLine is one sale item row as the recap sees it.
type SaleLine struct {
	SaleID     string
	TenantID   string
	TenantName string
	ItemID     string
	ItemName   string
	PayMethod  string
	Status     string
	RecordedAt time.Time
	Qty        int
	LineIDR    int64
}

// SummaryRow is one recap group.
type SummaryRow struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Count    int    `json:"count"` // sales (or item quantity for group_by=item)
	TotalIDR int64  `json:"total_idr"`
}

// Group-by options of the sales recap.
var GroupBys = map[string]bool{"tenant": true, "item": true, "day": true, "method": true}

// SalesSummary groups sale lines by tenant, item, day (in loc) or method,
// excluding voided sales (FR-126-001). Rows are ordered by key.
func SalesSummary(lines []SaleLine, groupBy string, loc *time.Location) []SummaryRow {
	rows := map[string]*SummaryRow{}
	counted := map[string]map[string]bool{}
	for _, l := range lines {
		if l.Status == SaleVoided {
			continue
		}
		var key, label string
		switch groupBy {
		case "item":
			key, label = l.ItemID, l.ItemName
		case "day":
			key = l.RecordedAt.In(loc).Format("2006-01-02")
			label = key
		case "method":
			key = l.PayMethod
			label = map[string]string{PayWallet: "Dompet", PayCash: "Tunai"}[l.PayMethod]
		default:
			key, label = l.TenantID, l.TenantName
		}
		r := rows[key]
		if r == nil {
			r = &SummaryRow{Key: key, Label: label}
			rows[key] = r
			counted[key] = map[string]bool{}
		}
		r.TotalIDR += l.LineIDR
		if groupBy == "item" {
			r.Count += l.Qty
		} else if !counted[key][l.SaleID] {
			counted[key][l.SaleID] = true
			r.Count++
		}
	}
	out := make([]SummaryRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// SettlementJournals returns the approval journal (school share) and the
// payment journal (tenant payout). The share is taken first from what the
// school owes the tenant (wallet sales booked to tenant payable); any excess
// becomes a receivable from the tenant. A payout <= 0 has no payment journal.
func SettlementJournals(s Settlement, acc wallet.Accounts) (share, paid []model.JournalLine, err error) {
	if s.SchoolShareIDR > 0 {
		revenue := acc.CanteenRevenue
		if s.SchemeKind == SchemeRent {
			revenue = acc.RentRevenue
		}
		if revenue == "" {
			return nil, nil, errors.New("akun pendapatan kantin/sewa belum dipetakan")
		}
		fromPayable := min(s.SchoolShareIDR, s.WalletSalesIDR)
		if fromPayable > 0 {
			share = append(share, model.JournalLine{AccCode: acc.TenantPayable, Description: "Potongan bagi hasil dari utang tenant", DebitIDR: fromPayable})
		}
		if rest := s.SchoolShareIDR - fromPayable; rest > 0 {
			if acc.TenantReceivable == "" {
				return nil, nil, errors.New("akun piutang tenant belum dipetakan")
			}
			share = append(share, model.JournalLine{AccCode: acc.TenantReceivable, Description: "Piutang bagi hasil/sewa tenant", DebitIDR: rest})
		}
		share = append(share, model.JournalLine{AccCode: revenue, Description: "Pendapatan bagi hasil/sewa kantin", CreditIDR: s.SchoolShareIDR})
		if err := model.ValidateJournal(share); err != nil {
			return nil, nil, err
		}
	}
	if s.TenantPayoutIDR > 0 {
		paid = []model.JournalLine{
			{AccCode: acc.TenantPayable, Description: "Pembayaran hak tenant kantin", DebitIDR: s.TenantPayoutIDR},
			{AccCode: acc.Cash, Description: "Pembayaran hak tenant kantin", CreditIDR: s.TenantPayoutIDR},
		}
		if err := model.ValidateJournal(paid); err != nil {
			return nil, nil, err
		}
	}
	return share, paid, nil
}
