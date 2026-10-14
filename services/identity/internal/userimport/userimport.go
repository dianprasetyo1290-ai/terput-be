// Package userimport parses and validates a bulk user file (Data Pengguna /
// Siswa / Guru import) without touching the database:
//   - .xlsx and .csv (comma or semicolon, UTF-8 with or without BOM) (BUG-076/151)
//   - columns found by header name, with aliases and an optional explicit
//     mapping; missing required columns reject the whole file (BUG-080)
//   - at most MaxRows data rows (BUG-081)
//   - per-row validation, including duplicates inside the file (BUG-018)
package userimport

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"skci/identity/internal/rules"
)

// MaxRows is the largest number of data rows accepted in one file.
const MaxRows = 1000

// Canonical fields.
const (
	FieldName      = "name"
	FieldEmail     = "email"
	FieldPhone     = "phone"
	FieldRole      = "role"
	FieldSourceRef = "source_ref"
)

// FieldLabels are the Indonesian column names used in messages/template.
var FieldLabels = map[string]string{
	FieldName: "Nama", FieldEmail: "Email", FieldPhone: "Telepon", FieldRole: "Peran", FieldSourceRef: "ID Sumber",
}

var aliases = map[string][]string{
	FieldName:      {"nama", "nama lengkap", "name", "nama siswa", "nama guru"},
	FieldEmail:     {"email", "e-mail", "alamat email"},
	FieldPhone:     {"telepon", "no telepon", "nomor telepon", "no hp", "nomor hp", "hp", "phone", "no. telepon", "no. hp"},
	FieldRole:      {"peran", "role", "peran guru/siswa"},
	FieldSourceRef: {"id sumber", "id lama", "source id", "id sistem lama", "kode lama"},
}

// ErrUnsupportedFormat is answered with the list of supported formats.
var ErrUnsupportedFormat = errors.New("Format file tidak didukung. Gunakan file .xlsx atau .csv sesuai template")

// ReadTable returns all rows (header first) and the detected format.
func ReadTable(fileName string, data []byte) ([][]string, string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	isZip := bytes.HasPrefix(data, []byte("PK\x03\x04"))
	switch {
	case ext == ".xlsx" || (ext == "" && isZip):
		if !isZip {
			return nil, "", errors.New("File .xlsx rusak atau bukan file Excel. Gunakan file .xlsx atau .csv sesuai template")
		}
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			return nil, "", errors.New("File .xlsx tidak dapat dibaca. Gunakan file .xlsx atau .csv sesuai template")
		}
		defer f.Close()
		rows, err := f.GetRows(f.GetSheetName(0))
		if err != nil {
			return nil, "", errors.New("Lembar pertama file .xlsx tidak dapat dibaca")
		}
		return rows, "xlsx", nil
	case ext == ".csv":
		rows, err := readCSV(data)
		return rows, "csv", err
	}
	return nil, "", ErrUnsupportedFormat
}

func readCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	firstLine := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		firstLine = data[:i]
	}
	r := csv.NewReader(bytes.NewReader(data))
	if bytes.Count(firstLine, []byte(";")) > bytes.Count(firstLine, []byte(",")) {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("File CSV tidak valid: %v", err)
		}
		rows = append(rows, rec)
	}
	return rows, nil
}

func normHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, "*")
	if i := strings.Index(s, "("); i > 0 {
		s = s[:i]
	}
	return strings.Join(strings.Fields(strings.TrimSpace(strings.TrimSuffix(s, "*"))), " ")
}

// MapColumns finds each field's column index. mapping (field -> header text
// in the file) overrides the aliases. Required: name, email, and role when
// the role is not fixed by the caller. Returns the missing required labels.
func MapColumns(header []string, mapping map[string]string, roleFixed bool) (map[string]int, []string) {
	idx := map[string]int{}
	byHeader := map[string]int{}
	for i, h := range header {
		if n := normHeader(h); n != "" {
			if _, dup := byHeader[n]; !dup {
				byHeader[n] = i
			}
		}
	}
	for field, al := range aliases {
		if m, ok := mapping[field]; ok && strings.TrimSpace(m) != "" {
			if i, ok := byHeader[normHeader(m)]; ok {
				idx[field] = i
			}
			continue
		}
		for _, a := range al {
			if i, ok := byHeader[a]; ok {
				idx[field] = i
				break
			}
		}
	}
	required := []string{FieldName, FieldEmail}
	if !roleFixed {
		required = append(required, FieldRole)
	}
	var missing []string
	for _, f := range required {
		if _, ok := idx[f]; !ok {
			missing = append(missing, FieldLabels[f])
		}
	}
	return idx, missing
}

// Row is one valid data row.
type Row struct {
	Number    int    `json:"row"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone,omitempty"`
	Role      string `json:"role"`
	SourceRef string `json:"source_ref,omitempty"`
}

// RowError is one rejected row (1-based spreadsheet row number).
type RowError struct {
	Row   int    `json:"row"`
	Issue string `json:"issue"`
}

// Validate checks the data rows. fixedRole "" means read from the Peran
// column; allowedRoles limits what a row may ask for. emailTaken reports
// emails that already exist in the database.
func Validate(rows [][]string, cols map[string]int, fixedRole string, allowedRoles map[string]bool, emailTaken map[string]bool) ([]Row, []RowError) {
	cell := func(row []string, f string) string {
		i, ok := cols[f]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	var valid []Row
	var errs []RowError
	seenEmail := map[string]int{}
	seenRef := map[string]int{}
	for i, row := range rows {
		num := i + 2 // header is row 1
		r := Row{Number: num, Name: cell(row, FieldName), Email: cell(row, FieldEmail), Phone: cell(row, FieldPhone), SourceRef: cell(row, FieldSourceRef)}
		if r.Name == "" && r.Email == "" && r.Phone == "" && cell(row, FieldRole) == "" {
			continue // blank line
		}
		var issues []string
		if r.Name == "" {
			issues = append(issues, "Nama wajib diisi")
		}
		lower := strings.ToLower(r.Email)
		switch {
		case r.Email == "":
			issues = append(issues, "Email wajib diisi")
		case !rules.ValidEmail(r.Email):
			issues = append(issues, "Format email tidak valid")
		case seenEmail[lower] > 0:
			issues = append(issues, fmt.Sprintf("Email sama dengan baris %d", seenEmail[lower]))
		case emailTaken[lower]:
			issues = append(issues, fmt.Sprintf("Email %q sudah terdaftar", r.Email))
		}
		if r.Email != "" && seenEmail[lower] == 0 {
			seenEmail[lower] = num
		}
		if r.SourceRef != "" {
			if prev := seenRef[r.SourceRef]; prev > 0 {
				issues = append(issues, fmt.Sprintf("ID Sumber sama dengan baris %d", prev))
			} else {
				seenRef[r.SourceRef] = num
			}
		}
		r.Role = fixedRole
		if r.Role == "" {
			r.Role = strings.ToLower(cell(row, FieldRole))
			if r.Role == "" {
				issues = append(issues, "Peran wajib diisi")
			}
		}
		if r.Role != "" && !allowedRoles[r.Role] {
			issues = append(issues, fmt.Sprintf("Peran %q tidak diizinkan untuk impor ini", r.Role))
		}
		if len(issues) > 0 {
			errs = append(errs, RowError{Row: num, Issue: strings.Join(issues, "; ")})
			continue
		}
		valid = append(valid, r)
	}
	return valid, errs
}

// DataRowCount counts non-blank rows after the header.
func DataRowCount(rows [][]string) int {
	n := 0
	for _, row := range rows {
		for _, c := range row {
			if strings.TrimSpace(c) != "" {
				n++
				break
			}
		}
	}
	return n
}
