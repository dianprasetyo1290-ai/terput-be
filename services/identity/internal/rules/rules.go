// Package rules holds the pure master-data validation rules of the identity
// service (units, users, academic years, student status). No I/O: handlers
// call these and turn the returned field errors into a 400/409.
package rules

import (
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// FieldErrors maps a JSON field to an Indonesian message.
type FieldErrors map[string]string

var (
	reSchoolCode = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,19}$`)
	reNISN       = regexp.MustCompile(`^[0-9]{10}$`)
	reNIK        = regexp.MustCompile(`^[0-9]{16}$`)
	reAccount    = regexp.MustCompile(`^[0-9]{5,20}$`)
	reYearLabel  = regexp.MustCompile(`^(\d{4})\s*/\s*(\d{4})$`)
)

// ValidEmail accepts a plain address (no display name) with a dotted domain.
func ValidEmail(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 254 {
		return false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || a.Name != "" {
		return false
	}
	at := strings.LastIndex(s, "@")
	domain := s[at+1:]
	return at > 0 && strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

// ValidNISN: Nomor Induk Siswa Nasional, 10 digits.
func ValidNISN(s string) bool { return reNISN.MatchString(s) }

// ValidNIK: Nomor Induk Kependudukan, 16 digits.
func ValidNIK(s string) bool { return reNIK.MatchString(s) }

// NormalizePhone reduces an Indonesian phone number to digits with a 62
// prefix so "0812-3456", "+62 812 3456" and "628123456" compare equal.
func NormalizePhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case strings.HasPrefix(d, "0"):
		d = "62" + d[1:]
	case strings.HasPrefix(d, "8"):
		d = "62" + d
	}
	return d
}

// School is the subset of a unit payload that is validated.
type School struct {
	Name                    string
	Code                    *string
	Level                   *string
	Email                   *string
	SettlementAccountNumber *string
	SettlementBankName      *string
	SettlementAccountName   *string
}

func blank(p *string) bool { return p == nil || strings.TrimSpace(*p) == "" }

// ValidateSchool enforces BUG-013 (code and level required), code shape,
// email format and a complete settlement account (BUG-012).
func ValidateSchool(s School) FieldErrors {
	errs := FieldErrors{}
	if strings.TrimSpace(s.Name) == "" {
		errs["name"] = "Nama unit wajib diisi"
	}
	if blank(s.Code) {
		errs["code"] = "Kode unit wajib diisi"
	} else if !reSchoolCode.MatchString(strings.TrimSpace(*s.Code)) {
		errs["code"] = "Kode unit 2–20 karakter: huruf, angka, - atau _"
	}
	if blank(s.Level) {
		errs["level"] = "Jenjang wajib diisi"
	} else if len(strings.TrimSpace(*s.Level)) > 20 {
		errs["level"] = "Jenjang maksimal 20 karakter"
	}
	if !blank(s.Email) && !ValidEmail(*s.Email) {
		errs["email"] = "Format email tidak valid"
	}
	anyAcct := !blank(s.SettlementAccountNumber) || !blank(s.SettlementBankName) || !blank(s.SettlementAccountName)
	if anyAcct {
		if blank(s.SettlementBankName) {
			errs["settlement_bank_name"] = "Nama bank rekening settlement wajib diisi"
		}
		if blank(s.SettlementAccountName) {
			errs["settlement_account_name"] = "Nama pemilik rekening settlement wajib diisi"
		}
		if blank(s.SettlementAccountNumber) {
			errs["settlement_account_number"] = "Nomor rekening settlement wajib diisi"
		} else if !reAccount.MatchString(strings.TrimSpace(*s.SettlementAccountNumber)) {
			errs["settlement_account_number"] = "Nomor rekening hanya angka, 5–20 digit"
		}
	}
	return errs
}

// ValidateIdentityNumbers checks NISN/NIK when present (BUG-074).
func ValidateIdentityNumbers(nisn, nik *string) FieldErrors {
	errs := FieldErrors{}
	if !blank(nisn) && !ValidNISN(strings.TrimSpace(*nisn)) {
		errs["nisn"] = "Format NISN tidak valid (10 digit angka)"
	}
	if !blank(nik) && !ValidNIK(strings.TrimSpace(*nik)) {
		errs["nik"] = "Format NIK tidak valid (16 digit angka)"
	}
	return errs
}

// ---- academic years ---------------------------------------------------------

const dateLayout = "2006-01-02"

// ParseDate parses YYYY-MM-DD; empty -> nil.
func ParseDate(s string) (*time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil, false
	}
	return &t, true
}

// DefaultYearRange derives 1 July - 30 June from a "YYYY/YYYY+1" label.
func DefaultYearRange(label string) (start, end time.Time, ok bool) {
	m := reYearLabel.FindStringSubmatch(strings.TrimSpace(label))
	if m == nil {
		return
	}
	var y1, y2 int
	for _, c := range m[1] {
		y1 = y1*10 + int(c-'0')
	}
	for _, c := range m[2] {
		y2 = y2*10 + int(c-'0')
	}
	if y2 != y1+1 {
		return
	}
	return time.Date(y1, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(y2, 6, 30, 0, 0, 0, 0, time.UTC), true
}

// ValidateYearDates: both required, end after start, at most ~2 years long.
func ValidateYearDates(start, end *time.Time) FieldErrors {
	errs := FieldErrors{}
	if start == nil {
		errs["start_date"] = "Tanggal mulai tahun ajaran wajib diisi"
	}
	if end == nil {
		errs["end_date"] = "Tanggal selesai tahun ajaran wajib diisi"
	}
	if start != nil && end != nil {
		if !end.After(*start) {
			errs["end_date"] = "Tanggal selesai harus setelah tanggal mulai"
		} else if end.Sub(*start) > 2*366*24*time.Hour {
			errs["end_date"] = "Rentang tahun ajaran maksimal 2 tahun"
		}
	}
	return errs
}

// ---- student status (BUG-017/075) ------------------------------------------

const (
	StudentCalon  = "calon"
	StudentAktif  = "aktif"
	StudentLulus  = "lulus"
	StudentKeluar = "keluar"
)

// ValidStudentStatus reports whether s is one of the four statuses.
func ValidStudentStatus(s string) bool {
	switch s {
	case StudentCalon, StudentAktif, StudentLulus, StudentKeluar:
		return true
	}
	return false
}

// normal transitions an admin may apply directly.
var studentTransitions = map[string][]string{
	StudentCalon:  {StudentAktif, StudentKeluar},
	StudentAktif:  {StudentLulus, StudentKeluar},
	StudentKeluar: {StudentAktif}, // re-admission
	StudentLulus:  {},             // final; only a correction may undo it
}

// StudentTransition decides whether from -> to is allowed. A correction
// (undoing lulus, or any other off-path change) needs the superadmin or
// kepala_sekolah role and a written reason. Moving a student to keluar
// always needs a reason. Returns "" when allowed, otherwise the
// message for the client.
func StudentTransition(from, to string, correction bool, actorRole, reason string) string {
	if !ValidStudentStatus(to) {
		return "Status siswa tidak dikenal (calon, aktif, lulus, keluar)"
	}
	if from == to {
		return "Status siswa sudah " + to
	}
	reason = strings.TrimSpace(reason)
	for _, allowed := range studentTransitions[from] {
		if allowed == to {
			if to == StudentKeluar && reason == "" {
				return "Alasan keluar wajib diisi"
			}
			return ""
		}
	}
	if !correction {
		if from == StudentLulus {
			return "Status lulus hanya dapat dikoreksi melalui alur koreksi oleh kepala sekolah atau superadmin"
		}
		return "Perubahan status dari " + from + " ke " + to + " tidak diizinkan"
	}
	if actorRole != "superadmin" && actorRole != "kepala_sekolah" {
		return "Koreksi status siswa hanya dapat dilakukan kepala sekolah atau superadmin"
	}
	if len(reason) < 10 {
		return "Alasan koreksi wajib diisi (minimal 10 karakter)"
	}
	return ""
}

// RoleStatusFor maps a student status onto user_school_roles.status (login
// access for the siswa role): only calon and aktif can sign in.
func RoleStatusFor(studentStatus string) string {
	if studentStatus == StudentAktif || studentStatus == StudentCalon {
		return "active"
	}
	return "inactive"
}

var reUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUID reports whether s is a canonical UUID string.
func IsUUID(s string) bool { return reUUID.MatchString(s) }
