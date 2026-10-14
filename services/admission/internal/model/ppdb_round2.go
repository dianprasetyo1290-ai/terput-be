package model

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Registration form fields (BUG-095)
// ---------------------------------------------------------------------------

// FormField is one admin-configured field of the registration form. A field
// whose key names a built-in biodata column (see BuiltinApplicantFields)
// only toggles whether that column is mandatory; any other key is stored in
// applicants.custom_fields.
type FormField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Options  []string `json:"options"`
	Required bool     `json:"required"`
}

// BuiltinApplicantFields are the biodata columns an admin may mark mandatory.
var BuiltinApplicantFields = map[string]string{
	"nickname":              "Nama panggilan",
	"nisn":                  "NISN",
	"nik":                   "NIK",
	"birth_place":           "Tempat lahir",
	"previous_school":       "Sekolah asal",
	"address":               "Alamat",
	"guardian_relationship": "Hubungan dengan wali",
}

var fieldKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

// ValidateFormField checks a field definition before it is saved.
func ValidateFormField(f FormField) error {
	if !fieldKeyPattern.MatchString(f.Key) {
		return errors.New("kode field hanya boleh huruf kecil, angka, dan garis bawah (2-40 karakter, diawali huruf)")
	}
	if strings.TrimSpace(f.Label) == "" {
		return errors.New("label field wajib diisi")
	}
	if _, builtin := BuiltinApplicantFields[f.Key]; builtin {
		if f.Type != "text" {
			return errors.New("field bawaan biodata harus bertipe teks")
		}
		return nil
	}
	switch f.Type {
	case "text", "textarea", "number", "date", "checkbox":
		if len(f.Options) > 0 {
			return errors.New("pilihan hanya untuk field bertipe pilihan")
		}
	case "select":
		if len(f.Options) < 2 {
			return errors.New("field pilihan membutuhkan minimal dua opsi")
		}
		seen := map[string]bool{}
		for _, o := range f.Options {
			o = strings.TrimSpace(o)
			if o == "" || seen[o] {
				return errors.New("opsi pilihan tidak boleh kosong atau ganda")
			}
			seen[o] = true
		}
	default:
		return errors.New("tipe field tidak dikenal")
	}
	return nil
}

// NormalizeCustomValues validates guardian-entered values against the active
// custom fields. Unknown keys and empty values are dropped; a value of the
// wrong type is rejected. Required-ness is checked at submission
// (MissingFormFields), so a draft may be saved incomplete.
func NormalizeCustomValues(fields []FormField, values map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for _, f := range fields {
		if _, builtin := BuiltinApplicantFields[f.Key]; builtin {
			continue
		}
		raw, ok := values[f.Key]
		if !ok || raw == nil {
			continue
		}
		switch f.Type {
		case "checkbox":
			b, ok := raw.(bool)
			if !ok {
				return nil, fmt.Errorf("%s harus berupa ya/tidak", f.Label)
			}
			if b {
				out[f.Key] = true
			}
		case "number":
			var n float64
			switch v := raw.(type) {
			case float64:
				n = v
			case string:
				if strings.TrimSpace(v) == "" {
					continue
				}
				p, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
				if err != nil {
					return nil, fmt.Errorf("%s harus berupa angka", f.Label)
				}
				n = p
			default:
				return nil, fmt.Errorf("%s harus berupa angka", f.Label)
			}
			out[f.Key] = n
		default:
			s, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("%s harus berupa teks", f.Label)
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if len(s) > 2000 {
				return nil, fmt.Errorf("%s terlalu panjang", f.Label)
			}
			if f.Type == "date" {
				if _, err := time.Parse("2006-01-02", s); err != nil {
					return nil, fmt.Errorf("%s harus berupa tanggal (YYYY-MM-DD)", f.Label)
				}
			}
			if f.Type == "select" && !containsString(f.Options, s) {
				return nil, fmt.Errorf("%s: pilihan tidak valid", f.Label)
			}
			out[f.Key] = s
		}
	}
	return out, nil
}

// MissingFormFields lists the labels of required fields that are empty.
// builtin holds the applicant's biodata columns by key.
func MissingFormFields(fields []FormField, builtin map[string]string, custom map[string]any) []string {
	var missing []string
	for _, f := range fields {
		if !f.Required {
			continue
		}
		if _, ok := BuiltinApplicantFields[f.Key]; ok {
			if strings.TrimSpace(builtin[f.Key]) == "" {
				missing = append(missing, f.Label)
			}
			continue
		}
		v, ok := custom[f.Key]
		if !ok || v == nil || v == "" || v == false {
			missing = append(missing, f.Label)
		}
	}
	return missing
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Waves per jenjang (BUG-093)
// ---------------------------------------------------------------------------

// EducationLevels are the jenjang a wave can be opened for.
var EducationLevels = []string{"TK", "SD", "SMP", "SMA", "SMK"}

var gradeRange = map[string][2]int{"TK": {0, 0}, "SD": {1, 6}, "SMP": {7, 9}, "SMA": {10, 12}, "SMK": {10, 12}}

// ValidateWaveLevel checks the optional jenjang and entry grade of a wave.
func ValidateWaveLevel(level string, grade *int) error {
	if level != "" && !containsString(EducationLevels, level) {
		return errors.New("jenjang harus salah satu dari TK, SD, SMP, SMA, SMK")
	}
	if grade == nil {
		return nil
	}
	if level == "" {
		return errors.New("kelas masuk hanya dapat diisi bersama jenjang")
	}
	r := gradeRange[level]
	lo, hi := r[0], r[1]
	if *grade < lo || *grade > hi {
		return fmt.Errorf("kelas masuk untuk jenjang %s harus antara %d dan %d", level, lo, hi)
	}
	return nil
}

// SameLevel reports whether two waves compete for the same jenjang; only
// those are checked for overlapping registration windows. An unset jenjang
// overlaps with everything.
func SameLevel(a, b string) bool {
	return a == "" || b == "" || a == b
}

// ---------------------------------------------------------------------------
// Refunds (BUG-040)
// ---------------------------------------------------------------------------

// RefundAmount applies the period refund policy to what a dropped applicant
// already paid: policyBP of the paid amount (basis points) minus a flat
// administration fee, never below zero.
func RefundAmount(paid int64, policyBP int, adminFee int64) int64 {
	if paid <= 0 || policyBP <= 0 {
		return 0
	}
	if policyBP > 10000 {
		policyBP = 10000
	}
	amount := paid*int64(policyBP)/10000 - adminFee
	if amount < 0 {
		return 0
	}
	return amount
}

// MaskAccountNumber keeps the last four digits of a bank account number.
func MaskAccountNumber(n string) string {
	n = strings.TrimSpace(n)
	if len(n) <= 4 {
		return n
	}
	return strings.Repeat("*", len(n)-4) + n[len(n)-4:]
}
