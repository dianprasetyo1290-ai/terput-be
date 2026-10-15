package wallet

import (
	"errors"
	"testing"
	"time"

	"skci/billing/internal/model"
)

func ptr(v int64) *int64 { return &v }

func TestCanSpend(t *testing.T) {
	cases := []struct {
		name                   string
		balance, spent, amount int64
		limit                  *int64
		want                   error
	}{
		{"AC-124-01 melebihi batas harian", 100000, 20000, 10000, ptr(25000), ErrDailyLimit},
		{"tepat di batas", 100000, 15000, 10000, ptr(25000), nil},
		{"saldo kurang", 5000, 0, 10000, nil, ErrInsufficient},
		{"limit NULL hanya cek saldo", 50000, 1000000, 10000, nil, nil},
		{"nominal nol", 50000, 0, 0, nil, ErrAmount},
		{"limit dan saldo kurang: limit dulu", 5000, 20000, 10000, ptr(25000), ErrDailyLimit},
	}
	for _, tc := range cases {
		if got := CanSpend(tc.balance, tc.limit, tc.spent, tc.amount); !errors.Is(got, tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
	if ErrDailyLimit.Error() != "Melebihi batas jajan harian" {
		t.Fatalf("pesan AC-124-01 berubah: %q", ErrDailyLimit.Error())
	}
}

func TestSpentToday(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Date(2026, 10, 12, 0, 1, 0, 0, loc)
	mv := []Movement{
		{Kind: MovePosSale, AmountIDR: -7000, CreatedAt: time.Date(2026, 10, 11, 23, 59, 0, 0, loc)}, // kemarin
		{Kind: MovePosSale, AmountIDR: -5000, CreatedAt: time.Date(2026, 10, 12, 0, 1, 0, 0, loc)},
		{Kind: MovePosSale, AmountIDR: -3000, CreatedAt: time.Date(2026, 10, 12, 0, 0, 30, 0, loc)},
		{Kind: MovePosVoid, AmountIDR: 3000, CreatedAt: time.Date(2026, 10, 12, 0, 0, 40, 0, loc)},
		{Kind: MoveTopupGateway, AmountIDR: 100000, CreatedAt: time.Date(2026, 10, 12, 0, 0, 10, 0, loc)},
	}
	if got := SpentToday(mv, now, loc); got != 5000 {
		t.Fatalf("SpentToday = %d, want 5000", got)
	}
	// 23.59 WIB is 16.59 UTC of the same day: still "yesterday" in Jakarta.
	if got := SpentToday(mv[:1], now.UTC(), loc); got != 0 {
		t.Fatalf("23.59 kemarin terhitung hari ini: %d", got)
	}
}

func TestCanTopup(t *testing.T) {
	if err := CanTopup(100000, 50000, nil); err != nil {
		t.Fatal(err)
	}
	if err := CanTopup(100000, 50000, ptr(150000)); err != nil {
		t.Fatal(err)
	}
	if err := CanTopup(100000, 50001, ptr(150000)); !errors.Is(err, ErrMaxBalance) {
		t.Fatalf("want ErrMaxBalance, got %v", err)
	}
	if err := CanTopup(0, -1, nil); !errors.Is(err, ErrAmount) {
		t.Fatalf("want ErrAmount, got %v", err)
	}
}

var testAcc = Accounts{AllowanceLiability: "2101", SavingsLiability: "2102", Cash: "1101", TenantPayable: "2201",
	CanteenRevenue: "4301", RentRevenue: "4302", TenantReceivable: "1301"}
var testGw = model.GatewayAccounts{Clearing: "1102", FeeExpense: "6101"}

func lineMap(lines []model.JournalLine) map[string]int64 {
	m := map[string]int64{}
	for _, l := range lines {
		m[l.AccCode] += l.DebitIDR - l.CreditIDR
	}
	return m
}

func TestDailyJournal(t *testing.T) {
	agg := DailyAgg{
		ByKind: map[string]map[string]int64{
			KindAllowance: {MoveTopupGateway: 200000, MoveTopupCashier: 50000, MovePosSale: -30000, MovePosVoid: 5000, MoveTransferOut: -10000},
			KindSavings:   {MoveSavingsDeposit: 100000, MoveSavingsWithdrawal: -40000, MoveRefundExit: -20000, MoveTransferIn: 10000},
		},
		GatewaySchoolFeeIDR: 1500,
	}
	lines, err := DailyJournal(agg, testAcc, testGw)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.ValidateJournal(lines); err != nil {
		t.Fatal(err)
	}
	got := lineMap(lines)
	want := map[string]int64{
		"2101": -(200000 + 50000 - 30000 + 5000 - 10000), // credit 215.000
		"2102": -(100000 - 40000 - 20000 + 10000),        // credit 50.000
		"1101": 50000 + 100000 - 40000 - 20000,           // debit 90.000
		"2201": -25000,                                   // credit 25.000
		"1102": 200000 - 1500,
		"6101": 1500,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("akun %s = %d, want %d", k, got[k], v)
		}
	}
	// Same-kind transfer nets to zero: no line.
	none, err := DailyJournal(DailyAgg{ByKind: map[string]map[string]int64{KindSavings: {MoveTransferIn: 5000, MoveTransferOut: -5000}}}, testAcc, testGw)
	if err != nil || len(none) != 0 {
		t.Fatalf("transfer satu jenis harus tanpa baris: %v %v", none, err)
	}
	// Void only (reverse of a sale) flips sides.
	void, err := DailyJournal(DailyAgg{ByKind: map[string]map[string]int64{KindAllowance: {MovePosVoid: 4000}}}, testAcc, testGw)
	if err != nil || lineMap(void)["2201"] != 4000 || lineMap(void)["2101"] != -4000 {
		t.Fatalf("void: %v %v", void, err)
	}
	if _, err := DailyJournal(DailyAgg{ByKind: map[string]map[string]int64{KindAllowance: {MoveTopupGateway: 1000}}}, testAcc, model.GatewayAccounts{}); err == nil {
		t.Fatal("top-up gateway tanpa akun gateway harus error")
	}
	if _, err := DailyJournal(DailyAgg{ByKind: map[string]map[string]int64{KindAllowance: {"unknown": 1}}}, testAcc, testGw); err == nil {
		t.Fatal("jenis mutasi tak dikenal harus error")
	}
}

func TestReconcile(t *testing.T) {
	if r := Reconcile(1000000, 1000000); r.Status != "matched" || r.DiffIDR != 0 {
		t.Fatalf("%+v", r)
	}
	if r := Reconcile(1000000, 1000001); r.Status != "mismatch" || r.DiffIDR != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestExitDisposition(t *testing.T) {
	out := ExitDisposition([]WalletBalance{{WalletID: "s", Kind: KindSavings, BalanceIDR: 350000}, {WalletID: "a", Kind: KindAllowance}})
	if len(out) != 1 || out[0].AmountIDR != 350000 || out[0].WalletID != "s" {
		t.Fatalf("AC-125-02: %+v", out)
	}
	if len(ExitDisposition([]WalletBalance{{WalletID: "a", BalanceIDR: 0}})) != 0 {
		t.Fatal("saldo 0 tidak boleh membuat refund")
	}
}

func TestClosureForRefund(t *testing.T) {
	cases := []struct {
		refund, closure, wallet string
		ok                      bool
	}{
		{"requested", "pending_approval", "closing", true},
		{"approved", "approved", "closing", true},
		{"rejected", "rejected", "active", true},
		{"processed", "completed", "closed", true},
		{"cancelled", "", "", false},
	}
	for _, tc := range cases {
		c, w, ok := ClosureForRefund(tc.refund)
		if c != tc.closure || w != tc.wallet || ok != tc.ok {
			t.Errorf("%s: got %s %s %v", tc.refund, c, w, ok)
		}
	}
}

// A processed wallet refund (refund_exit) is journaled by the daily wallet
// journal as Dr wallet liability / Cr cash.
func TestWalletRefundJournal(t *testing.T) {
	cases := []struct {
		kind      string
		amount    int64
		liability string
	}{
		{KindAllowance, 75000, "2101"},
		{KindSavings, 120000, "2102"},
	}
	for _, tc := range cases {
		lines, err := DailyJournal(DailyAgg{ByKind: map[string]map[string]int64{tc.kind: {MoveRefundExit: -tc.amount}}}, testAcc, model.GatewayAccounts{})
		if err != nil {
			t.Fatal(err)
		}
		got := lineMap(lines)
		if len(got) != 2 || got[tc.liability] != tc.amount || got["1101"] != -tc.amount {
			t.Errorf("%s: lines %v", tc.kind, got)
		}
	}
}
