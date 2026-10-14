package userimport

import (
	"fmt"
	"strings"

	"skci/identity/internal/rules"
)

// Import kinds besides plain accounts (QA-IDN BUG-021): parents with their
// relation to students, and class membership, both from the old system.
const (
	KindUsers    = "users"
	KindOrangTua = "orang_tua"
	KindKelas    = "kelas"
)

// Extra canonical fields for the orang_tua / kelas sheets.
const (
	FieldStudentRef    = "student_ref"
	FieldStudentNISN   = "student_nisn"
	FieldRelationship  = "relationship"
	FieldClassName     = "class_name"
	FieldGradeLevel    = "grade_level"
	FieldHomeroomEmail = "homeroom_email"
)

func init() {
	FieldLabels[FieldStudentRef] = "ID Sumber Siswa"
	FieldLabels[FieldStudentNISN] = "NISN Siswa"
	FieldLabels[FieldRelationship] = "Hubungan"
	FieldLabels[FieldClassName] = "Nama Kelas"
	FieldLabels[FieldGradeLevel] = "Tingkat"
	FieldLabels[FieldHomeroomEmail] = "Email Wali Kelas"
}

var kindAliases = map[string][]string{
	FieldStudentRef:    {"id sumber siswa", "id siswa lama", "id lama siswa", "kode siswa lama"},
	FieldStudentNISN:   {"nisn siswa", "nisn"},
	FieldRelationship:  {"hubungan", "relasi", "hubungan dengan siswa"},
	FieldClassName:     {"nama kelas", "kelas", "rombel"},
	FieldGradeLevel:    {"tingkat", "kelas tingkat", "grade"},
	FieldHomeroomEmail: {"email wali kelas", "wali kelas", "email walikelas"},
}

// Relationships accepted for guardian links (guardian_links.relationship).
var Relationships = map[string]string{
	"ayah": "ayah", "bapak": "ayah", "father": "ayah",
	"ibu": "ibu", "mother": "ibu",
	"wali": "wali", "guardian": "wali",
}

// KindTemplates are the header rows of the downloadable templates.
var KindTemplates = map[string][]string{
	KindOrangTua: {"Nama*", "Email*", "Telepon", "ID Sumber", "ID Sumber Siswa", "NISN Siswa", "Hubungan*"},
	KindKelas:    {"Nama Kelas*", "Tingkat", "Email Wali Kelas", "ID Sumber Siswa", "NISN Siswa"},
}

// MapKindColumns maps the columns of an orang_tua or kelas sheet. Required:
// orang_tua: Nama, Email, Hubungan and a student column (ID Sumber Siswa or
// NISN Siswa); kelas: Nama Kelas and a student column.
func MapKindColumns(kind string, header []string, mapping map[string]string) (map[string]int, []string) {
	idx, _ := MapColumns(header, mapping, true)
	byHeader := map[string]int{}
	for i, h := range header {
		if n := normHeader(h); n != "" {
			if _, dup := byHeader[n]; !dup {
				byHeader[n] = i
			}
		}
	}
	for field, al := range kindAliases {
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
	// "kelas" alone also matches the Nama Kelas alias; a NISN column named
	// just "nisn" is the student's.
	var required []string
	switch kind {
	case KindOrangTua:
		required = []string{FieldName, FieldEmail, FieldRelationship}
	case KindKelas:
		required = []string{FieldClassName}
	}
	var missing []string
	for _, f := range required {
		if _, ok := idx[f]; !ok {
			missing = append(missing, FieldLabels[f])
		}
	}
	_, hasRef := idx[FieldStudentRef]
	_, hasNISN := idx[FieldStudentNISN]
	if !hasRef && !hasNISN {
		missing = append(missing, "ID Sumber Siswa atau NISN Siswa")
	}
	return idx, missing
}

// GuardianRow is one valid orang_tua row.
type GuardianRow struct {
	Number       int    `json:"row"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone,omitempty"`
	SourceRef    string `json:"source_ref,omitempty"`
	StudentRef   string `json:"student_ref,omitempty"`
	StudentNISN  string `json:"student_nisn,omitempty"`
	Relationship string `json:"relationship"`
	// Filled by the handler after DB resolution.
	StudentUserID string `json:"student_user_id,omitempty"`
	StudentName   string `json:"student_name,omitempty"`
	ExistingUser  bool   `json:"existing_account"`
}

// ClassRow is one valid kelas row (one student in one class).
type ClassRow struct {
	Number        int    `json:"row"`
	ClassName     string `json:"class_name"`
	GradeLevel    string `json:"grade_level,omitempty"`
	HomeroomEmail string `json:"homeroom_email,omitempty"`
	StudentRef    string `json:"student_ref,omitempty"`
	StudentNISN   string `json:"student_nisn,omitempty"`
	// Filled by the handler.
	StudentUserID  string `json:"student_user_id,omitempty"`
	StudentName    string `json:"student_name,omitempty"`
	HomeroomUserID string `json:"homeroom_user_id,omitempty"`
}

func cellOf(cols map[string]int, row []string, f string) string {
	i, ok := cols[f]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func blank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// ValidateGuardianRows checks orang_tua rows without the DB. The same parent
// email may repeat (one row per child) as long as the name matches.
func ValidateGuardianRows(rows [][]string, cols map[string]int) ([]GuardianRow, []RowError) {
	var valid []GuardianRow
	var errs []RowError
	nameByEmail := map[string]string{}
	seenPair := map[string]int{}
	for i, row := range rows {
		num := i + 2
		if blank(row) {
			continue
		}
		g := GuardianRow{Number: num, Name: cellOf(cols, row, FieldName), Email: cellOf(cols, row, FieldEmail),
			Phone: cellOf(cols, row, FieldPhone), SourceRef: cellOf(cols, row, FieldSourceRef),
			StudentRef: cellOf(cols, row, FieldStudentRef), StudentNISN: cellOf(cols, row, FieldStudentNISN)}
		var issues []string
		if g.Name == "" {
			issues = append(issues, "Nama wajib diisi")
		}
		lower := strings.ToLower(g.Email)
		switch {
		case g.Email == "":
			issues = append(issues, "Email wajib diisi")
		case !rules.ValidEmail(g.Email):
			issues = append(issues, "Format email tidak valid")
		case nameByEmail[lower] != "" && !strings.EqualFold(nameByEmail[lower], g.Name):
			issues = append(issues, "Email yang sama dipakai dengan nama berbeda")
		}
		rel, ok := Relationships[strings.ToLower(cellOf(cols, row, FieldRelationship))]
		if !ok {
			issues = append(issues, "Hubungan harus ayah, ibu, atau wali")
		}
		g.Relationship = rel
		if g.StudentRef == "" && g.StudentNISN == "" {
			issues = append(issues, "Isi ID Sumber Siswa atau NISN Siswa")
		}
		if g.StudentNISN != "" && !rules.ValidNISN(g.StudentNISN) {
			issues = append(issues, "NISN Siswa harus 10 digit")
		}
		pair := lower + "|" + g.StudentRef + "|" + g.StudentNISN
		if prev := seenPair[pair]; prev > 0 {
			issues = append(issues, fmt.Sprintf("Relasi sama dengan baris %d", prev))
		}
		if len(issues) > 0 {
			errs = append(errs, RowError{Row: num, Issue: strings.Join(issues, "; ")})
			continue
		}
		seenPair[pair] = num
		nameByEmail[lower] = g.Name
		valid = append(valid, g)
	}
	return valid, errs
}

// ValidateClassRows checks kelas rows without the DB. A student may appear
// in only one class per file; a class keeps one wali kelas.
func ValidateClassRows(rows [][]string, cols map[string]int) ([]ClassRow, []RowError) {
	var valid []ClassRow
	var errs []RowError
	seenStudent := map[string]int{}
	homeroomByClass := map[string]string{}
	for i, row := range rows {
		num := i + 2
		if blank(row) {
			continue
		}
		c := ClassRow{Number: num, ClassName: cellOf(cols, row, FieldClassName), GradeLevel: cellOf(cols, row, FieldGradeLevel),
			HomeroomEmail: cellOf(cols, row, FieldHomeroomEmail), StudentRef: cellOf(cols, row, FieldStudentRef),
			StudentNISN: cellOf(cols, row, FieldStudentNISN)}
		var issues []string
		if c.ClassName == "" {
			issues = append(issues, "Nama Kelas wajib diisi")
		}
		if c.StudentRef == "" && c.StudentNISN == "" {
			issues = append(issues, "Isi ID Sumber Siswa atau NISN Siswa")
		}
		if c.StudentNISN != "" && !rules.ValidNISN(c.StudentNISN) {
			issues = append(issues, "NISN Siswa harus 10 digit")
		}
		if c.HomeroomEmail != "" && !rules.ValidEmail(c.HomeroomEmail) {
			issues = append(issues, "Email Wali Kelas tidak valid")
		}
		key := strings.ToLower(c.ClassName)
		if c.HomeroomEmail != "" {
			if prev, ok := homeroomByClass[key]; ok && !strings.EqualFold(prev, c.HomeroomEmail) {
				issues = append(issues, "Wali kelas berbeda dengan baris lain untuk kelas yang sama")
			}
		}
		stu := c.StudentRef + "|" + c.StudentNISN
		if prev := seenStudent[stu]; prev > 0 {
			issues = append(issues, fmt.Sprintf("Siswa sama dengan baris %d", prev))
		}
		if len(issues) > 0 {
			errs = append(errs, RowError{Row: num, Issue: strings.Join(issues, "; ")})
			continue
		}
		seenStudent[stu] = num
		if c.HomeroomEmail != "" {
			homeroomByClass[key] = c.HomeroomEmail
		}
		valid = append(valid, c)
	}
	return valid, errs
}
