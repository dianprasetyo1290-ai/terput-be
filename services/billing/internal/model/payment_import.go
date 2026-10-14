package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// PaymentImportFormat describes the CSV accepted for importing the current
// year's payment history from the previous payment aggregator (BUG-122).
const PaymentImportFormat = "CSV dengan baris judul, pemisah koma (,) atau titik koma (;). " +
	"Kolom wajib: student_user_id (ID siswa), period (YYYY-MM, periode tagihan), amount_idr (nominal rupiah bulat) " +
	"dan paid_at (tanggal bayar YYYY-MM-DD atau DD/MM/YYYY). Kolom opsional: reference (nomor transaksi aggregator) " +
	"dan bill_id (wajib bila siswa punya lebih dari satu tagihan pada periode itu)."

// PaymentImportRow is one validated history line.
type PaymentImportRow struct {
	Line          int
	StudentUserID string
	BillID        string
	Year, Month   int
	AmountIDR     int64
	PaidAt        time.Time
	Reference     string
	Hash          string // stable across re-imports of the same file
}

var paymentImportAliases = map[string][]string{
	"student":   {"student_user_id", "student_id", "id_siswa"},
	"bill":      {"bill_id", "id_tagihan"},
	"period":    {"period", "periode", "bulan_tagihan"},
	"amount":    {"amount_idr", "amount", "nominal", "jumlah"},
	"paid_at":   {"paid_at", "tanggal_bayar", "tanggal", "date"},
	"reference": {"reference", "referensi", "no_transaksi", "ref"},
}

// ParsePaymentImportCSV validates the whole file; any bad row rejects it
// (problems are listed per line) so history is never half-loaded.
func ParsePaymentImportCSV(data []byte, today time.Time) ([]PaymentImportRow, []string) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	firstLine := string(data)
	if i := strings.IndexAny(firstLine, "\r\n"); i >= 0 {
		firstLine = firstLine[:i]
	}
	if strings.TrimSpace(firstLine) == "" {
		return nil, []string{"file kosong"}
	}
	r := csv.NewReader(bytes.NewReader(data))
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return nil, []string{"baris judul tidak terbaca"}
	}
	col := map[string]int{}
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		for field, aliases := range paymentImportAliases {
			for _, a := range aliases {
				if _, dup := col[field]; key == a && !dup {
					col[field] = i
				}
			}
		}
	}
	var problems []string
	for _, f := range []string{"student", "period", "amount", "paid_at"} {
		if _, ok := col[f]; !ok {
			problems = append(problems, "kolom "+paymentImportAliases[f][0]+" tidak ditemukan")
		}
	}
	if len(problems) > 0 {
		return nil, problems
	}
	cell := func(rec []string, f string) string {
		i, ok := col[f]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	endOfToday := time.Date(today.Year(), today.Month(), today.Day(), 23, 59, 59, 0, time.UTC)
	var rows []PaymentImportRow
	seen := map[string]int{}
	line := 1
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			problems = append(problems, fmt.Sprintf("baris %d: %v", line, err))
			continue
		}
		if isBlank(rec) {
			continue
		}
		row := PaymentImportRow{Line: line, StudentUserID: cell(rec, "student"), BillID: cell(rec, "bill"), Reference: cell(rec, "reference")}
		if row.StudentUserID == "" {
			problems = append(problems, fmt.Sprintf("baris %d: student_user_id kosong", line))
		}
		p, perr := time.Parse("2006-01", cell(rec, "period"))
		if perr != nil {
			problems = append(problems, fmt.Sprintf("baris %d: period %q harus YYYY-MM", line, cell(rec, "period")))
		}
		row.Year, row.Month = p.Year(), int(p.Month())
		amt, aerr := ParseRupiah(cell(rec, "amount"))
		switch {
		case aerr != nil:
			problems = append(problems, fmt.Sprintf("baris %d: %v", line, aerr))
		case amt <= 0:
			problems = append(problems, fmt.Sprintf("baris %d: nominal harus lebih dari 0", line))
		}
		row.AmountIDR = amt
		d, derr := parseStatementDate(cell(rec, "paid_at"))
		switch {
		case derr != nil:
			problems = append(problems, fmt.Sprintf("baris %d: %v", line, derr))
		case d.After(endOfToday):
			problems = append(problems, fmt.Sprintf("baris %d: tanggal bayar di masa depan", line))
		}
		row.PaidAt = d
		base := strings.Join([]string{row.StudentUserID, row.BillID, cell(rec, "period"), strconv.FormatInt(amt, 10),
			d.Format("2006-01-02"), strings.ToUpper(row.Reference)}, "|")
		occurrence := seen[base]
		seen[base]++
		sum := sha256.Sum256([]byte(base + "|" + strconv.Itoa(occurrence)))
		row.Hash = hex.EncodeToString(sum[:])
		rows = append(rows, row)
	}
	if len(problems) > 0 {
		if len(problems) > maxReportedProblems {
			problems = append(problems[:maxReportedProblems], fmt.Sprintf("dan %d masalah lain", len(problems)-maxReportedProblems))
		}
		return nil, problems
	}
	if len(rows) == 0 {
		return nil, []string{"tidak ada baris pembayaran"}
	}
	return rows, nil
}
