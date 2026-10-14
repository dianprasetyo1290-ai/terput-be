package model

import (
	"errors"
	"strings"
	"time"
)

// Discount program statuses (BUG-055).
const (
	DiscountPending  = "pending_approval"
	DiscountActive   = "active"
	DiscountRejected = "rejected"
	DiscountInactive = "inactive"
)

// DiscountProgramInput is a new scholarship/discount as submitted.
type DiscountProgramInput struct {
	Name          string
	Kind          DiscountKind
	Value         int64 // basis points for percent, rupiah for nominal
	CapIDR        int64
	Scope         string
	SchemeID      string
	GradeLevel    string
	StudentUserID string
	ValidFrom     *time.Time
	ValidTo       *time.Time
	Reason        string
}

// ValidateDiscountProgram checks a new program: a name and reason, a known
// kind with a positive value (percent at most 10000 bp = 100%), a
// non-negative cap, the scope's target and an ordered validity period.
func ValidateDiscountProgram(in DiscountProgramInput) error {
	switch {
	case strings.TrimSpace(in.Name) == "":
		return errors.New("nama program wajib diisi")
	case strings.TrimSpace(in.Reason) == "":
		return errors.New("alasan/dasar pemberian wajib diisi")
	case !in.Kind.Valid():
		return errors.New("jenis harus percent atau nominal")
	case in.Value <= 0:
		return errors.New("nilai potongan harus lebih dari 0")
	case in.Kind == DiscountPercent && in.Value > percentScale:
		return errors.New("persentase potongan maksimal 100% (10000 basis poin)")
	case in.CapIDR < 0:
		return errors.New("batas maksimal potongan tidak boleh negatif")
	}
	switch in.Scope {
	case "school":
	case "scheme":
		if strings.TrimSpace(in.SchemeID) == "" {
			return errors.New("skema wajib dipilih untuk cakupan skema")
		}
	case "grade":
		if strings.TrimSpace(in.GradeLevel) == "" {
			return errors.New("tingkat wajib diisi untuk cakupan tingkat")
		}
	case "student":
		if strings.TrimSpace(in.StudentUserID) == "" {
			return errors.New("siswa wajib dipilih untuk cakupan siswa")
		}
	default:
		return errors.New("cakupan harus school, scheme, grade atau student")
	}
	if in.ValidFrom != nil && in.ValidTo != nil && in.ValidTo.Before(*in.ValidFrom) {
		return errors.New("akhir periode berlaku tidak boleh sebelum awal periode")
	}
	return nil
}

// DiscountActiveInPeriod reports whether a program with the given validity
// window applies to the bill of period year/month: the window must overlap
// that calendar month. Open bounds are unlimited.
func DiscountActiveInPeriod(validFrom, validTo *time.Time, year, month int) bool {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1)
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	if validFrom != nil && day(*validFrom).After(last) {
		return false
	}
	if validTo != nil && day(*validTo).Before(first) {
		return false
	}
	return true
}
