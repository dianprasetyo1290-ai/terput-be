package model

import (
	"errors"
	"testing"
)

func TestParseRupiah(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		err  bool
	}{
		{"1500000", 1500000, false},
		{"1.500.000", 1500000, false},
		{"1,500,000", 1500000, false},
		{"Rp 1.500.000", 1500000, false},
		{"1500000,00", 1500000, false},
		{"1.500.000,00", 1500000, false},
		{"1500000.00", 1500000, false},
		{"-250000", -250000, false},
		{"(250.000)", -250000, false},
		{"1500000,50", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseRupiah(tc.in)
		if (err != nil) != tc.err || got != tc.want {
			t.Errorf("ParseRupiah(%q)=%d,%v", tc.in, got, err)
		}
	}
}

func TestParseBankStatementCSV(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		account string
		rows    int
		first   int64
		err     bool
	}{
		{"standard comma", "account,posted_on,amount_idr,description\nBCA-1,2026-09-01,500000,TRF SPP\n", "", 1, 500000, false},
		{"semicolon indonesian", "Tanggal;Keterangan;Debet;Kredit\n01/09/2026;TRF SPP;;750.000,00\n02/09/2026;BIAYA ADM;5.000;\n", "BCA-1", 2, 750000, false},
		{"account from form", "date,amount\n2026-09-01,100000\n", "MANDIRI-9", 1, 100000, false},
		{"bom and blank lines", "\xef\xbb\xbfposted_on,amount_idr,account\n\n2026-09-01,1,BCA\n\n", "", 1, 1, false},
		{"unknown format foo;bar", "foo;bar\n1;2\n", "BCA", 0, 0, true},
		{"no account anywhere", "date,amount\n2026-09-01,100000\n", "", 0, 0, true},
		{"bad date", "account,date,amount\nBCA,32/13/2026,100\n", "", 0, 0, true},
		{"bad amount", "account,date,amount\nBCA,2026-09-01,lima ratus\n", "", 0, 0, true},
		{"zero amount", "account,date,amount\nBCA,2026-09-01,0\n", "", 0, 0, true},
		{"header only", "account,date,amount\n", "", 0, 0, true},
		{"empty file", "", "BCA", 0, 0, true},
	}
	for _, tc := range cases {
		rows, err := ParseBankStatementCSV([]byte(tc.data), tc.account)
		if (err != nil) != tc.err {
			t.Errorf("%s: err=%v", tc.name, err)
			continue
		}
		if err != nil {
			var be *BankStatementError
			if !errors.As(err, &be) || len(be.Problems) == 0 {
				t.Errorf("%s: want BankStatementError, got %T", tc.name, err)
			}
			continue
		}
		if len(rows) != tc.rows || rows[0].AmountIDR != tc.first {
			t.Errorf("%s: rows=%d first=%d", tc.name, len(rows), rows[0].AmountIDR)
		}
	}
}

func TestBankStatementHashStable(t *testing.T) {
	data := []byte("account,date,amount,description\nBCA,2026-09-01,500000,TRF\nBCA,2026-09-01,500000,TRF\n")
	a, err := ParseBankStatementCSV(data, "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ParseBankStatementCSV(data, "")
	if a[0].Hash == a[1].Hash {
		t.Fatal("identical lines in one file must get distinct hashes")
	}
	if a[0].Hash != b[0].Hash || a[1].Hash != b[1].Hash {
		t.Fatal("re-import of the same file must produce the same hashes")
	}
	if a[1].AmountIDR != 500000 || a[0].Line != 2 {
		t.Fatalf("unexpected row %+v", a[0])
	}
}
