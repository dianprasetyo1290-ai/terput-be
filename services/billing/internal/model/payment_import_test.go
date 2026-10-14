package model

import (
	"strings"
	"testing"
	"time"
)

func TestParsePaymentImportCSV(t *testing.T) {
	today := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	hdr := "student_user_id,period,amount_idr,paid_at,reference\n"
	cases := []struct {
		name    string
		csv     string
		rows    int
		problem string
	}{
		{"ok comma", hdr + "s1,2026-07,500000,2026-07-05,TRX1\ns1,2026-08,500.000,05/08/2026,TRX2\n", 2, ""},
		{"ok semicolon aliases", "id_siswa;periode;nominal;tanggal_bayar\ns1;2026-07;500000;2026-07-05\n", 1, ""},
		{"bom and blank lines", "\xef\xbb\xbf" + hdr + "\ns1,2026-07,1,2026-07-05,\n\n", 1, ""},
		{"missing column", "student_user_id,period,amount_idr\ns1,2026-07,1\n", 0, "kolom paid_at"},
		{"bad period", hdr + "s1,07-2026,1,2026-07-05,\n", 0, "harus YYYY-MM"},
		{"zero amount", hdr + "s1,2026-07,0,2026-07-05,\n", 0, "lebih dari 0"},
		{"future date", hdr + "s1,2026-07,1,2026-12-01,\n", 0, "masa depan"},
		{"no student", hdr + ",2026-07,1,2026-07-05,\n", 0, "student_user_id kosong"},
		{"empty", "", 0, "file kosong"},
		{"header only", hdr, 0, "tidak ada baris"},
	}
	for _, tc := range cases {
		rows, problems := ParsePaymentImportCSV([]byte(tc.csv), today)
		if tc.problem == "" {
			if len(problems) > 0 || len(rows) != tc.rows {
				t.Errorf("%s: rows=%d problems=%v", tc.name, len(rows), problems)
			}
			continue
		}
		if rows != nil || !strings.Contains(strings.Join(problems, ";"), tc.problem) {
			t.Errorf("%s: want %q, got rows=%d %v", tc.name, tc.problem, len(rows), problems)
		}
	}
	// Duplicate identical lines get distinct, stable hashes.
	a, _ := ParsePaymentImportCSV([]byte(hdr+"s1,2026-07,1,2026-07-05,X\ns1,2026-07,1,2026-07-05,X\n"), today)
	b, _ := ParsePaymentImportCSV([]byte(hdr+"s1,2026-07,1,2026-07-05,X\ns1,2026-07,1,2026-07-05,X\n"), today)
	if a[0].Hash == a[1].Hash || a[0].Hash != b[0].Hash || a[1].Hash != b[1].Hash {
		t.Fatal("hashes must be distinct per occurrence and stable across files")
	}
	if a[0].Year != 2026 || a[0].Month != 7 || a[0].AmountIDR != 1 {
		t.Fatalf("parsed %+v", a[0])
	}
}
