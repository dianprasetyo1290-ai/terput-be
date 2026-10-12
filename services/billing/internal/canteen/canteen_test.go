package canteen

import (
	"errors"
	"testing"
	"time"

	"skci/billing/internal/model"
	"skci/billing/internal/wallet"
)

func d(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func TestOfflineAccept(t *testing.T) {
	rec := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	if r := OfflineAccept(rec, rec.Add(30*time.Minute), 30); !r.Offline || r.LateSync {
		t.Fatalf("30 menit: %+v", r)
	}
	if r := OfflineAccept(rec, rec.Add(31*time.Minute), 30); !r.Offline || !r.LateSync {
		t.Fatalf("31 menit: %+v", r)
	}
}

func TestMergeSync(t *testing.T) {
	fresh, dup := MergeSync(map[string]bool{"a": true}, []string{"a", "b", "c", "b"})
	if len(fresh) != 2 || fresh[0] != "b" || fresh[1] != "c" {
		t.Fatalf("fresh %v", fresh)
	}
	if len(dup) != 2 || dup[0] != "a" || dup[1] != "b" {
		t.Fatalf("dup %v", dup)
	}
}

func TestCanVoid(t *testing.T) {
	cases := []struct {
		s      SaleState
		reason string
		want   error
	}{
		{SaleState{Status: SaleCompleted}, "salah input", nil},
		{SaleState{Status: SaleVoided}, "x", ErrAlreadyVoided},
		{SaleState{Status: SaleCompleted, SettlementID: "s1"}, "x", ErrSettled},
		{SaleState{Status: SaleCompleted}, "  ", ErrReasonRequired},
	}
	for i, tc := range cases {
		if got := CanVoid(tc.s, tc.reason); !errors.Is(got, tc.want) {
			t.Errorf("%d: got %v want %v", i, got, tc.want)
		}
	}
}

func TestSettle(t *testing.T) {
	share := Scheme{ID: "sc", Kind: SchemeRevenueShare, ShareBP: 1000, ShareBase: BaseWallet, EffectiveFrom: d("2026-01-01")}
	s, err := Settle(share, 10000000, 3000000, d("2026-10-01"), d("2026-10-31"))
	if err != nil || s.SchoolShareIDR != 1000000 || s.TenantPayoutIDR != 9000000 {
		t.Fatalf("AC-126-01: %+v %v", s, err)
	}
	// Rounding: school share rounded down.
	s, _ = Settle(Scheme{Kind: SchemeRevenueShare, ShareBP: 333, ShareBase: BaseWallet, EffectiveFrom: d("2026-01-01")}, 10001, 0, d("2026-10-01"), d("2026-10-31"))
	if s.SchoolShareIDR != 333 || s.TenantPayoutIDR != 9668 {
		t.Fatalf("pembulatan: %+v", s)
	}
	// share_base all with large cash -> negative payout.
	all := share
	all.ShareBase = BaseAll
	all.ShareBP = 2000
	s, _ = Settle(all, 1000000, 9000000, d("2026-10-01"), d("2026-10-31"))
	if s.SchoolShareIDR != 2000000 || s.TenantPayoutIDR != -1000000 {
		t.Fatalf("base all: %+v", s)
	}
	rent := Scheme{Kind: SchemeRent, RentIDR: 1500000, EffectiveFrom: d("2026-01-01")}
	s, err = Settle(rent, 4000000, 0, d("2026-02-01"), d("2026-02-28"))
	if err != nil || s.SchoolShareIDR != 1500000 || s.TenantPayoutIDR != 2500000 {
		t.Fatalf("sewa: %+v %v", s, err)
	}
	if _, err := Settle(rent, 1, 0, d("2026-02-01"), d("2026-02-27")); !errors.Is(err, ErrRentPeriod) {
		t.Fatalf("sewa bukan sebulan: %v", err)
	}
	to := d("2026-10-15")
	changed := Scheme{Kind: SchemeRevenueShare, ShareBP: 1000, ShareBase: BaseWallet, EffectiveFrom: d("2026-01-01"), EffectiveTo: &to}
	if _, err := Settle(changed, 1, 0, d("2026-10-01"), d("2026-10-31")); !errors.Is(err, ErrSplitPeriod) {
		t.Fatalf("skema berganti: %v", err)
	}
	if _, err := Settle(Scheme{Kind: SchemeRevenueShare, ShareBP: 1000, EffectiveFrom: d("2026-10-10")}, 1, 0, d("2026-10-01"), d("2026-10-31")); !errors.Is(err, ErrSplitPeriod) {
		t.Fatalf("skema mulai di tengah: %v", err)
	}
}

func TestSalesSummary(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	at := time.Date(2026, 10, 12, 23, 30, 0, 0, loc)
	lines := []SaleLine{
		{SaleID: "1", TenantID: "A", TenantName: "Tenant A", ItemID: "i1", ItemName: "Nasi", PayMethod: PayWallet, Status: SaleCompleted, RecordedAt: at, Qty: 2, LineIDR: 20000},
		{SaleID: "1", TenantID: "A", TenantName: "Tenant A", ItemID: "i2", ItemName: "Teh", PayMethod: PayWallet, Status: SaleCompleted, RecordedAt: at, Qty: 1, LineIDR: 3000},
		{SaleID: "2", TenantID: "A", TenantName: "Tenant A", ItemID: "i1", ItemName: "Nasi", PayMethod: PayCash, Status: SaleCompleted, RecordedAt: at.Add(time.Hour), Qty: 1, LineIDR: 10000},
		{SaleID: "3", TenantID: "B", TenantName: "Tenant B", ItemID: "i3", ItemName: "Mie", PayMethod: PayCash, Status: SaleVoided, RecordedAt: at, Qty: 1, LineIDR: 8000},
	}
	byTenant := SalesSummary(lines, "tenant", loc)
	if len(byTenant) != 1 || byTenant[0].Count != 2 || byTenant[0].TotalIDR != 33000 {
		t.Fatalf("tenant: %+v", byTenant)
	}
	byItem := SalesSummary(lines, "item", loc)
	if len(byItem) != 2 || byItem[0].Key != "i1" || byItem[0].Count != 3 || byItem[0].TotalIDR != 30000 {
		t.Fatalf("item: %+v", byItem)
	}
	byDay := SalesSummary(lines, "day", loc)
	if len(byDay) != 2 || byDay[0].Key != "2026-10-12" || byDay[1].Key != "2026-10-13" {
		t.Fatalf("day: %+v", byDay)
	}
	byMethod := SalesSummary(lines, "method", loc)
	if len(byMethod) != 2 || byMethod[0].Key != PayCash || byMethod[0].TotalIDR != 10000 || byMethod[1].TotalIDR != 23000 {
		t.Fatalf("method: %+v", byMethod)
	}
}

var acc = wallet.Accounts{AllowanceLiability: "2101", SavingsLiability: "2102", Cash: "1101", TenantPayable: "2201",
	CanteenRevenue: "4301", RentRevenue: "4302", TenantReceivable: "1301"}

func TestSettlementJournals(t *testing.T) {
	share, paid, err := SettlementJournals(Settlement{SchemeKind: SchemeRevenueShare, WalletSalesIDR: 10000000, SchoolShareIDR: 1000000, TenantPayoutIDR: 9000000}, acc)
	if err != nil || len(share) != 2 || share[0].DebitIDR != 1000000 || share[1].CreditIDR != 1000000 || paid[0].DebitIDR != 9000000 || paid[1].AccCode != "1101" {
		t.Fatalf("normal: %+v %+v %v", share, paid, err)
	}
	share, paid, err = SettlementJournals(Settlement{SchemeKind: SchemeRevenueShare, WalletSalesIDR: 1000000, SchoolShareIDR: 2000000, TenantPayoutIDR: -1000000}, acc)
	if err != nil || len(paid) != 0 || len(share) != 3 || share[1].AccCode != "1301" || share[1].DebitIDR != 1000000 {
		t.Fatalf("payout negatif: %+v %+v %v", share, paid, err)
	}
	for _, j := range [][]model.JournalLine{share} {
		if err := model.ValidateJournal(j); err != nil {
			t.Fatal(err)
		}
	}
	share, _, err = SettlementJournals(Settlement{SchemeKind: SchemeRent, WalletSalesIDR: 4000000, SchoolShareIDR: 1500000, TenantPayoutIDR: 2500000}, acc)
	if err != nil || share[len(share)-1].AccCode != "4302" {
		t.Fatalf("sewa: %+v %v", share, err)
	}
	noRent := acc
	noRent.RentRevenue = ""
	if _, _, err := SettlementJournals(Settlement{SchemeKind: SchemeRent, SchoolShareIDR: 1}, noRent); err == nil {
		t.Fatal("akun sewa kosong harus error")
	}
}
