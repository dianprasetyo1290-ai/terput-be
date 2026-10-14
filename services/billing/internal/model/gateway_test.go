package model

import (
	"fmt"
	"testing"
	"time"
)

func TestFee(t *testing.T) {
	tests := []struct {
		name   string
		method GatewayMethod
		amount int64
		want   int64
	}{
		{
			name: "flat fee 4000 IDR",
			method: GatewayMethod{
				FeeKind:    FeeFlat,
				FeeFlatIDR: 4000,
			},
			amount: 500000,
			want:   4000,
		},
		{
			name: "percent fee 70 bp (0.7%) on 1,000,000",
			method: GatewayMethod{
				FeeKind: FeePercent,
				FeeBP:   70, // 0.70%
			},
			amount: 1000000,
			want:   7000, // 1000000 * 70 / 10000 = 7000
		},
		{
			name: "percent fee floor behavior",
			method: GatewayMethod{
				FeeKind: FeePercent,
				FeeBP:   70,
			},
			amount: 1050,
			want:   7, // 1050 * 70 / 10000 = 7.35 -> 7
		},
		{
			name: "zero amount",
			method: GatewayMethod{
				FeeKind:    FeeFlat,
				FeeFlatIDR: 4000,
			},
			amount: 0,
			want:   0,
		},
		{
			name: "negative amount",
			method: GatewayMethod{
				FeeKind:    FeeFlat,
				FeeFlatIDR: 4000,
			},
			amount: -100,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Fee(tt.method, tt.amount)
			if got != tt.want {
				t.Errorf("Fee() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTotals_AC28_02(t *testing.T) {
	// AC-28-02 Given biaya QRIS diatur ditanggung orang tua
	// When orang tua membayar tagihan Rp1.000.000 dengan QRIS
	// Then total bayar Rp1.000.000 ditambah biaya QRIS, dan tagihan teralokasi Rp1.000.000
	qrisMethodParent := GatewayMethod{
		Kind:      "qris",
		FeeKind:   FeePercent,
		FeeBP:     70, // 0.7% = 7000 IDR
		FeeBearer: FeeBearerParent,
	}
	resParent, err := Totals(1000000, qrisMethodParent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resParent.BillsAmountIDR != 1000000 {
		t.Errorf("BillsAmountIDR = %d, want 1000000", resParent.BillsAmountIDR)
	}
	if resParent.FeeIDR != 7000 {
		t.Errorf("FeeIDR = %d, want 7000", resParent.FeeIDR)
	}
	if resParent.TotalIDR != 1007000 {
		t.Errorf("TotalIDR = %d, want 1007000 (1000000 + 7000)", resParent.TotalIDR)
	}
	if resParent.FeeBearer != FeeBearerParent {
		t.Errorf("FeeBearer = %s, want parent", resParent.FeeBearer)
	}

	// Ditto when borne by school
	qrisMethodSchool := GatewayMethod{
		Kind:      "qris",
		FeeKind:   FeePercent,
		FeeBP:     70,
		FeeBearer: FeeBearerSchool,
	}
	resSchool, err := Totals(1000000, qrisMethodSchool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resSchool.BillsAmountIDR != 1000000 {
		t.Errorf("BillsAmountIDR = %d, want 1000000", resSchool.BillsAmountIDR)
	}
	if resSchool.FeeIDR != 7000 {
		t.Errorf("FeeIDR = %d, want 7000 (dicatat beban)", resSchool.FeeIDR)
	}
	if resSchool.TotalIDR != 1000000 {
		t.Errorf("TotalIDR = %d, want 1000000", resSchool.TotalIDR)
	}
}

func TestAllocate_BRFIN03_BRFIN04(t *testing.T) {
	now := time.Now()
	bill1 := OpenBill{BillID: "b1", OutstandingIDR: 500000, DueDate: now.Add(24 * time.Hour), ComponentPriority: 2}
	bill2 := OpenBill{BillID: "b2", OutstandingIDR: 300000, DueDate: now.Add(48 * time.Hour), ComponentPriority: 1}

	// Case 1: Pay 800,000 for both bills (exact amount)
	allocs, deposit, err := Allocate(800000, []OpenBill{bill1, bill2}, "earliest_due")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allocs) != 2 || allocs[0].AmountIDR != 500000 || allocs[1].AmountIDR != 300000 || deposit != 0 {
		t.Errorf("exact payment allocs = %+v, deposit = %d", allocs, deposit)
	}

	// Case 2: Partial payment 600,000 with earliest_due rule
	allocs, deposit, err = Allocate(600000, []OpenBill{bill1, bill2}, "earliest_due")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allocs) != 2 || allocs[0].AmountIDR != 500000 || allocs[1].AmountIDR != 100000 || deposit != 0 {
		t.Errorf("partial earliest_due allocs = %+v, deposit = %d", allocs, deposit)
	}

	// Case 3: Overpayment 1,000,000 with remainder going to deposit (BR-FIN-04)
	allocs, deposit, err = Allocate(1000000, []OpenBill{bill1, bill2}, "earliest_due")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allocs) != 2 || allocs[0].AmountIDR != 500000 || allocs[1].AmountIDR != 300000 || deposit != 200000 {
		t.Errorf("overpayment deposit = %d, want 200000", deposit)
	}

	// Case 4: Component priority rule
	allocs, deposit, err = Allocate(400000, []OpenBill{bill1, bill2}, "component_priority")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// bill2 has ComponentPriority 1 (higher priority than bill1's 2)
	if len(allocs) != 2 || allocs[0].BillID != "b2" || allocs[0].AmountIDR != 300000 || allocs[1].BillID != "b1" || allocs[1].AmountIDR != 100000 {
		t.Errorf("component_priority allocs = %+v", allocs)
	}
}

func TestFormatReceiptNo(t *testing.T) {
	got := FormatReceiptNo(2026, 42)
	want := "RCP-2026-00042"
	if got != want {
		t.Errorf("FormatReceiptNo = %q, want %q", got, want)
	}
}

func TestReconcile_AC31_01(t *testing.T) {
	// AC-31-01 Given TX-012 sukses tanggal 1 Oktober dan laporan settlement 1 Oktober tidak memuatnya
	// When rekonsiliasi harian berjalan
	// Then TX-012 tercatat di laporan pengecualian
	successTxs := []ReconcileTx{
		{TXNo: "TX-011", ProviderRef: "REF-011", AmountIDR: 500000, FeeIDR: 4000},
		{TXNo: "TX-012", ProviderRef: "REF-012", AmountIDR: 800000, FeeIDR: 4000}, // missing in settlement
		{TXNo: "TX-013", ProviderRef: "REF-013", AmountIDR: 300000, FeeIDR: 2000}, // amount mismatch
	}

	settleItems := []ReconcileItem{
		{ProviderRef: "REF-011", AmountIDR: 500000, FeeIDR: 4000}, // match
		{ProviderRef: "REF-013", AmountIDR: 290000, FeeIDR: 2000}, // mismatch
		{ProviderRef: "REF-999", AmountIDR: 100000, FeeIDR: 1000}, // missing in sekoci
	}

	res := Reconcile(successTxs, settleItems)

	if len(res.Matched) != 1 || res.Matched[0].TXNo != "TX-011" {
		t.Errorf("Matched = %+v, want only TX-011", res.Matched)
	}

	var foundMissingSettlement, foundMismatch, foundMissingSekoci bool
	for _, ex := range res.Exceptions {
		if ex.Kind == ExceptionMissingInSettlement && ex.Ref == "TX-012" {
			foundMissingSettlement = true
		}
		if ex.Kind == ExceptionAmountMismatch && ex.Ref == "TX-013" {
			foundMismatch = true
		}
		if ex.Kind == ExceptionMissingInSekoci && ex.Ref == "REF-999" {
			foundMissingSekoci = true
		}
	}

	if !foundMissingSettlement {
		t.Errorf("ExceptionMissingInSettlement for TX-012 not found: %+v", res.Exceptions)
	}
	if !foundMismatch {
		t.Errorf("ExceptionAmountMismatch for TX-013 not found: %+v", res.Exceptions)
	}
	if !foundMissingSekoci {
		t.Errorf("ExceptionMissingInSekoci for REF-999 not found: %+v", res.Exceptions)
	}
}

func TestMethodShare_AC26_01(t *testing.T) {
	// AC-26-01 Given hari ini terdapat 40 transaksi VA dan 10 transaksi QRIS yang sukses
	// When Finance Manager membuka dashboard pembayaran
	// Then distribusi metode menampilkan VA 80% dan QRIS 20% (8000 bp / 2000 bp)
	counts := map[string]int{
		"va":   40,
		"qris": 10,
	}
	shares := MethodShare(counts)

	if shares["va"] != 8000 {
		t.Errorf("shares[va] = %d bp, want 8000 bp (80%%)", shares["va"])
	}
	if shares["qris"] != 2000 {
		t.Errorf("shares[qris] = %d bp, want 2000 bp (20%%)", shares["qris"])
	}

	// Zero count edge case
	zeroShares := MethodShare(map[string]int{"va": 0, "qris": 0})
	if zeroShares["va"] != 0 || zeroShares["qris"] != 0 {
		t.Errorf("zero shares = %+v", zeroShares)
	}
}

func TestBankReconcile_AC31_02(t *testing.T) {
	d1 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)

	lines := []BankLine{
		{ID: "line-1", Account: "ACC-01", PostedOn: d1, AmountIDR: 500000, Description: "Transfer SPP"},
		{ID: "line-2", Account: "ACC-01", PostedOn: d2, AmountIDR: 350000, Description: "Unknown"},
	}

	records := []BankPaymentRecord{
		{ID: "rec-1", Date: d1, AmountIDR: 500000, Ref: "TX-01"},
	}

	res := BankReconcile(lines, records, 2)
	if len(res.MatchedLines) != 1 || res.MatchedLines[0] != "line-1" {
		t.Errorf("MatchedLines = %+v, want [line-1]", res.MatchedLines)
	}
	if len(res.Exceptions) != 1 || res.Exceptions[0].Ref != "line-2" {
		t.Errorf("Exceptions = %+v, want line-2", res.Exceptions)
	}
}

func TestAllocate_PayerChoiceAndEdges(t *testing.T) {
	due := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	bills := []OpenBill{
		{BillID: "late", OutstandingIDR: 300000, DueDate: due.AddDate(0, 1, 0)},
		{BillID: "nodue", OutstandingIDR: 100000},
		{BillID: "early", OutstandingIDR: 500000, DueDate: due},
	}
	// payer_choice keeps the given order; the remainder becomes deposit.
	allocs, dep, err := Allocate(450000, bills[:2], "payer_choice")
	if err != nil || len(allocs) != 2 || allocs[0].BillID != "late" || allocs[1].BillID != "nodue" || allocs[1].AmountIDR != 100000 || dep != 50000 {
		t.Fatalf("payer_choice = %+v dep=%d err=%v", allocs, dep, err)
	}
	// earliest_due puts bills without a due date last.
	allocs, _, _ = Allocate(900000, bills, "earliest_due")
	if allocs[0].BillID != "early" || allocs[1].BillID != "late" || allocs[2].BillID != "nodue" {
		t.Fatalf("earliest_due order = %+v", allocs)
	}
	// No open bills: everything is deposit.
	if allocs, dep, _ := Allocate(1000, nil, "earliest_due"); len(allocs) != 0 || dep != 1000 {
		t.Fatalf("no bills = %+v dep=%d", allocs, dep)
	}
	if _, _, err := Allocate(1000, bills, "bogus"); err == nil {
		t.Fatal("unknown rule accepted")
	}
	if _, _, err := Allocate(0, bills, "earliest_due"); err == nil {
		t.Fatal("zero amount accepted")
	}
}

func TestBankReconcile_ReferenceAndTolerance(t *testing.T) {
	d := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	records := []BankPaymentRecord{
		{ID: "r-near", Date: d, AmountIDR: 100000, Ref: "STL-1"},
		{ID: "r-ref", Date: d.AddDate(0, 0, -10), AmountIDR: 100000, Ref: "STL-2"},
	}
	lines := []BankLine{
		{ID: "l-ref", PostedOn: d, AmountIDR: 100000, Description: "settlement stl-2 nicepay"},
		{ID: "l-date", PostedOn: d.AddDate(0, 0, 2), AmountIDR: 100000, Description: "transfer"},
		{ID: "l-far", PostedOn: d.AddDate(0, 0, 3), AmountIDR: 100000, Description: "transfer"},
	}
	res := BankReconcile(lines, records, 2)
	if len(res.Matches) != 2 || res.Matches[0].RecordID != "r-ref" || res.Matches[1].RecordID != "r-near" {
		t.Fatalf("matches = %+v", res.Matches)
	}
	if len(res.Exceptions) != 1 || res.Exceptions[0].Ref != "l-far" {
		t.Fatalf("exceptions = %+v", res.Exceptions)
	}
}

// AC-31-02: 50 bank lines, the matched ones are marked, the rest listed.
func TestBankReconcile_AC31_02_FiftyLines(t *testing.T) {
	d := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var lines []BankLine
	var records []BankPaymentRecord
	for i := 0; i < 50; i++ {
		amt := int64(100000 + i)
		lines = append(lines, BankLine{ID: fmt.Sprintf("l%d", i), PostedOn: d, AmountIDR: amt})
		if i < 45 {
			records = append(records, BankPaymentRecord{ID: fmt.Sprintf("r%d", i), Date: d, AmountIDR: amt})
		}
	}
	res := BankReconcile(lines, records, 0)
	if len(res.MatchedLines) != 45 || len(res.Exceptions) != 5 {
		t.Fatalf("matched=%d exceptions=%d", len(res.MatchedLines), len(res.Exceptions))
	}
}

func TestGatewayJournalLines(t *testing.T) {
	acc := GatewayAccounts{Clearing: "1103", FeeExpense: "6201", Receivable: "1201", Deposit: "2105"}
	cases := []struct {
		name string
		txs  []JournalTx
		want []JournalLine
	}{
		{"parent bears fee", []JournalTx{{BillsIDR: 1000000, FeeIDR: 7000, TotalIDR: 1007000, FeeBearer: FeeBearerParent}},
			[]JournalLine{{AccCode: "1103", DebitIDR: 1000000}, {AccCode: "1201", CreditIDR: 1000000}}},
		{"school bears fee", []JournalTx{{BillsIDR: 1000000, FeeIDR: 4000, TotalIDR: 1000000, FeeBearer: FeeBearerSchool}},
			[]JournalLine{{AccCode: "1103", DebitIDR: 996000}, {AccCode: "6201", DebitIDR: 4000}, {AccCode: "1201", CreditIDR: 1000000}}},
		{"aggregated day with deposit", []JournalTx{
			{BillsIDR: 500000, FeeIDR: 4000, TotalIDR: 504000, FeeBearer: FeeBearerParent},
			{BillsIDR: 300000, FeeIDR: 2100, TotalIDR: 300000, DepositIDR: 50000, FeeBearer: FeeBearerSchool}},
			[]JournalLine{{AccCode: "1103", DebitIDR: 797900}, {AccCode: "6201", DebitIDR: 2100}, {AccCode: "1201", CreditIDR: 750000}, {AccCode: "2105", CreditIDR: 50000}}},
	}
	for _, c := range cases {
		got, err := GatewayJournalLines(acc, c.txs)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %+v", c.name, got)
		}
		for i := range got {
			if got[i].AccCode != c.want[i].AccCode || got[i].DebitIDR != c.want[i].DebitIDR || got[i].CreditIDR != c.want[i].CreditIDR {
				t.Fatalf("%s line %d: got %+v want %+v", c.name, i, got[i], c.want[i])
			}
		}
	}
	if _, err := GatewayJournalLines(acc, nil); err == nil {
		t.Fatal("empty day accepted")
	}
}

func TestValidateJournal(t *testing.T) {
	ok := []JournalLine{{AccCode: "1", DebitIDR: 10}, {AccCode: "2", CreditIDR: 10}}
	if err := ValidateJournal(ok); err != nil {
		t.Fatal(err)
	}
	bad := map[string][]JournalLine{
		"one line":   {{AccCode: "1", DebitIDR: 10}},
		"unbalanced": {{AccCode: "1", DebitIDR: 10}, {AccCode: "2", CreditIDR: 9}},
		"both sides": {{AccCode: "1", DebitIDR: 10, CreditIDR: 10}, {AccCode: "2", CreditIDR: 0, DebitIDR: 0}},
		"no account": {{AccCode: " ", DebitIDR: 10}, {AccCode: "2", CreditIDR: 10}},
		"negative":   {{AccCode: "1", DebitIDR: -10}, {AccCode: "2", CreditIDR: -10}},
	}
	for name, lines := range bad {
		if ValidateJournal(lines) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestValidateManualAllocations(t *testing.T) {
	open := []OpenBill{{BillID: "b1", OutstandingIDR: 100}, {BillID: "b2", OutstandingIDR: 50}}
	cases := []struct {
		name    string
		amount  int64
		allocs  []BillAllocationResult
		deposit int64
		wantErr bool
	}{
		{"exact split", 150, []BillAllocationResult{{"b1", 100}, {"b2", 50}}, 0, false},
		{"partial plus deposit", 200, []BillAllocationResult{{"b1", 60}}, 140, false},
		{"total exceeds payment", 100, []BillAllocationResult{{"b1", 80}, {"b2", 40}}, 0, true},
		{"exceeds outstanding", 500, []BillAllocationResult{{"b2", 51}}, 0, true},
		{"foreign or closed bill", 100, []BillAllocationResult{{"other", 10}}, 0, true},
		{"duplicate bill", 100, []BillAllocationResult{{"b1", 10}, {"b1", 10}}, 0, true},
		{"zero amount line", 100, []BillAllocationResult{{"b1", 0}}, 0, true},
		{"negative line", 100, []BillAllocationResult{{"b1", -5}}, 0, true},
		{"non-positive payment", 0, nil, 0, true},
	}
	for _, tc := range cases {
		dep, err := ValidateManualAllocations(tc.amount, tc.allocs, open)
		if (err != nil) != tc.wantErr || (!tc.wantErr && dep != tc.deposit) {
			t.Errorf("%s: deposit=%d err=%v", tc.name, dep, err)
		}
	}
}
