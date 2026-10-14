package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// BankStatementFormat describes the CSV layouts ParseBankStatementCSV
// accepts; it is returned verbatim to the user when a file is rejected.
const BankStatementFormat = "CSV dengan baris judul, pemisah koma (,) atau titik koma (;). " +
	"Kolom wajib: tanggal (posted_on/date/tanggal/tgl, format YYYY-MM-DD, DD/MM/YYYY atau DD-MM-YYYY) " +
	"dan nominal (amount_idr/amount/nominal/jumlah, atau pasangan kredit/credit dan debit/debet). " +
	"Kolom opsional: account/rekening/no_rekening (wajib bila rekening tidak dipilih saat impor) " +
	"dan description/keterangan/deskripsi/uraian. Nominal dalam rupiah bulat, mis. 1500000, 1.500.000 atau 1500000,00."

// BankStatementRow is one parsed, validated statement line.
type BankStatementRow struct {
	Line        int // 1-based line number in the file (header = 1)
	Account     string
	PostedOn    time.Time
	AmountIDR   int64 // credit positive, debit negative
	Description string
	Hash        string // idempotency key, stable across re-imports of the same file
}

// BankStatementError lists why a file was rejected. Imports are all or
// nothing: one bad row rejects the file so a statement is never half-loaded.
type BankStatementError struct {
	Problems []string
}

func (e *BankStatementError) Error() string {
	return "format mutasi bank tidak dikenali: " + strings.Join(e.Problems, "; ") + ". Format yang didukung: " + BankStatementFormat
}

var headerAliases = map[string][]string{
	"account":     {"account", "rekening", "no_rekening", "no rekening", "nomor_rekening", "account_no"},
	"date":        {"posted_on", "date", "tanggal", "tgl", "tanggal_transaksi", "transaction_date"},
	"amount":      {"amount_idr", "amount", "nominal", "jumlah", "mutasi"},
	"credit":      {"kredit", "credit", "cr"},
	"debit":       {"debit", "debet", "db"},
	"description": {"description", "keterangan", "deskripsi", "uraian", "berita"},
}

const maxReportedProblems = 10

// ParseBankStatementCSV parses an uploaded bank mutation file. account is
// used for rows without an account column; an empty account with no account
// column is a format error (no default account is invented).
func ParseBankStatementCSV(data []byte, account string) ([]BankStatementRow, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	firstLine := string(data)
	if i := strings.IndexAny(firstLine, "\r\n"); i >= 0 {
		firstLine = firstLine[:i]
	}
	if strings.TrimSpace(firstLine) == "" {
		return nil, &BankStatementError{Problems: []string{"file kosong"}}
	}
	delim := ','
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		delim = ';'
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return nil, &BankStatementError{Problems: []string{"baris judul tidak terbaca"}}
	}
	col := map[string]int{}
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		for field, aliases := range headerAliases {
			for _, a := range aliases {
				if key == a {
					if _, dup := col[field]; !dup {
						col[field] = i
					}
				}
			}
		}
	}
	var problems []string
	if _, ok := col["date"]; !ok {
		problems = append(problems, "kolom tanggal tidak ditemukan")
	}
	_, hasAmount := col["amount"]
	_, hasCredit := col["credit"]
	_, hasDebit := col["debit"]
	if !hasAmount && !hasCredit && !hasDebit {
		problems = append(problems, "kolom nominal tidak ditemukan")
	}
	_, hasAccount := col["account"]
	account = strings.TrimSpace(account)
	if !hasAccount && account == "" {
		problems = append(problems, "rekening tidak ditentukan (isi kolom account atau pilih rekening saat impor)")
	}
	if len(problems) > 0 {
		return nil, &BankStatementError{Problems: problems}
	}

	cell := func(rec []string, field string) string {
		i, ok := col[field]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	var rows []BankStatementRow
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
		row := BankStatementRow{Line: line, Account: account, Description: cell(rec, "description")}
		if v := cell(rec, "account"); v != "" {
			row.Account = v
		}
		if row.Account == "" {
			problems = append(problems, fmt.Sprintf("baris %d: rekening kosong", line))
		}
		d, err := parseStatementDate(cell(rec, "date"))
		if err != nil {
			problems = append(problems, fmt.Sprintf("baris %d: %v", line, err))
		}
		row.PostedOn = d
		amt, err := statementAmount(cell(rec, "amount"), cell(rec, "credit"), cell(rec, "debit"), hasAmount)
		if err != nil {
			problems = append(problems, fmt.Sprintf("baris %d: %v", line, err))
		}
		row.AmountIDR = amt
		if err == nil && amt == 0 {
			problems = append(problems, fmt.Sprintf("baris %d: nominal nol", line))
		}
		base := strings.Join([]string{row.Account, row.PostedOn.Format("2006-01-02"), strconv.FormatInt(row.AmountIDR, 10),
			strings.ToUpper(strings.Join(strings.Fields(row.Description), " "))}, "|")
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
		return nil, &BankStatementError{Problems: problems}
	}
	if len(rows) == 0 {
		return nil, &BankStatementError{Problems: []string{"tidak ada baris transaksi"}}
	}
	return rows, nil
}

func isBlank(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

var statementDateLayouts = []string{"2006-01-02", "02/01/2006", "02-01-2006", "2/1/2006", "2006/01/02"}

func parseStatementDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("tanggal kosong")
	}
	for _, l := range statementDateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("tanggal %q tidak dikenali", s)
}

func statementAmount(amount, credit, debit string, hasAmount bool) (int64, error) {
	if hasAmount && amount != "" {
		return ParseRupiah(amount)
	}
	var total int64
	if credit != "" {
		c, err := ParseRupiah(credit)
		if err != nil {
			return 0, err
		}
		total += c
	}
	if debit != "" {
		d, err := ParseRupiah(debit)
		if err != nil {
			return 0, err
		}
		if d < 0 {
			d = -d
		}
		total -= d
	}
	if credit == "" && debit == "" {
		return 0, errors.New("nominal kosong")
	}
	return total, nil
}

// ParseRupiah reads a whole-rupiah amount written as 1500000, 1.500.000,
// 1,500,000, Rp 1.500.000 or with a zero fraction (1500000,00 / 1500000.00).
// A non-zero fraction is rejected: money is stored in whole rupiah.
func ParseRupiah(s string) (int64, error) {
	orig := s
	s = strings.TrimSpace(s)
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, s[1:len(s)-1]
	}
	if strings.HasPrefix(s, "-") {
		neg, s = true, s[1:]
	}
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s, "Rp"), "rp"))
	s = strings.ReplaceAll(s, " ", "")
	if n := len(s); n > 3 && (s[n-3] == ',' || s[n-3] == '.') {
		frac := s[n-2:]
		head := s[:n-3]
		// "1.500" style thousands groups have 3 digits after the separator,
		// so a 2-digit tail is always a fraction.
		if frac != "00" {
			return 0, fmt.Errorf("nominal %q memuat sen; gunakan rupiah bulat", orig)
		}
		s = head
	}
	s = strings.NewReplacer(".", "", ",", "").Replace(s)
	if s == "" {
		return 0, fmt.Errorf("nominal %q tidak valid", orig)
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("nominal %q tidak valid", orig)
	}
	if neg {
		v = -v
	}
	return v, nil
}
