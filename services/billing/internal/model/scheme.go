package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SchemeComponent is one tariff line of a billing scheme.
type SchemeComponent struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	AmountIDR int64  `json:"amount_idr"`
	IsFree    bool   `json:"is_free"`
}

// ValidateComponents enforces the tariff rules: unique codes per scheme
// (BUG-171), no negative amounts, and a zero amount only on a component
// explicitly flagged free (BUG-172).
func ValidateComponents(comps []SchemeComponent) error {
	seen := map[string]bool{}
	for _, c := range comps {
		code := strings.ToUpper(strings.TrimSpace(c.Code))
		switch {
		case code == "" || strings.TrimSpace(c.Name) == "":
			return errors.New("setiap komponen wajib memiliki kode dan nama")
		case seen[code]:
			return fmt.Errorf("kode komponen %s duplikat dalam skema", code)
		case c.AmountIDR < 0:
			return fmt.Errorf("tarif komponen %s tidak boleh negatif", code)
		case c.AmountIDR == 0 && !c.IsFree:
			return fmt.Errorf("tarif komponen %s Rp0 hanya boleh bila komponen ditandai gratis", code)
		case c.IsFree && c.AmountIDR != 0:
			return fmt.Errorf("komponen gratis %s harus bertarif Rp0", code)
		case c.Kind != "" && c.Kind != "monthly" && c.Kind != "one_time":
			return fmt.Errorf("jenis komponen %s harus monthly atau one_time", code)
		}
		seen[code] = true
	}
	return nil
}

func componentKey(c SchemeComponent) string {
	kind := c.Kind
	if kind == "" {
		kind = "one_time"
	}
	return fmt.Sprintf("%s|%s|%d|%t", strings.ToUpper(strings.TrimSpace(c.Code)), kind, c.AmountIDR, c.IsFree)
}

// TariffChanged reports whether two component lists bill different
// amounts (a code, kind, amount or free flag differs, or a line is added or
// removed). Renaming a component or reordering lines is not a tariff change.
func TariffChanged(before, after []SchemeComponent) bool {
	if len(before) != len(after) {
		return true
	}
	a := make([]string, len(before))
	b := make([]string, len(after))
	for i := range before {
		a[i] = componentKey(before[i])
		b[i] = componentKey(after[i])
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

// SchemeWindow is the dimension and validity of a scheme used to detect a
// double tariff (BUG-127).
type SchemeWindow struct {
	ID             string
	Name           string
	AcademicYearID string
	GradeLevel     string
	ClassroomID    string
	From, To       *time.Time // inclusive dates; nil = open
	Codes          []string
}

func sameDim(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// WindowsOverlap reports whether two inclusive date windows (nil = open)
// share at least one day.
func WindowsOverlap(aFrom, aTo, bFrom, bTo *time.Time) bool {
	if aTo != nil && bFrom != nil && aTo.Before(*bFrom) {
		return false
	}
	if bTo != nil && aFrom != nil && bTo.Before(*aFrom) {
		return false
	}
	return true
}

// SchemeConflict returns the component codes two schemes would both bill
// to the same students in the same period, or nil when they can coexist.
func SchemeConflict(a, b SchemeWindow) []string {
	if a.ID != "" && a.ID == b.ID {
		return nil
	}
	if !sameDim(a.AcademicYearID, b.AcademicYearID) || !sameDim(a.GradeLevel, b.GradeLevel) || !sameDim(a.ClassroomID, b.ClassroomID) {
		return nil
	}
	if !WindowsOverlap(a.From, a.To, b.From, b.To) {
		return nil
	}
	codes := map[string]bool{}
	for _, c := range a.Codes {
		codes[strings.ToUpper(strings.TrimSpace(c))] = true
	}
	var shared []string
	for _, c := range b.Codes {
		if k := strings.ToUpper(strings.TrimSpace(c)); codes[k] {
			shared = append(shared, k)
		}
	}
	sort.Strings(shared)
	return shared
}

// PeriodInWindow reports whether a billing month falls inside a scheme's
// validity window (month granularity: a window starting mid-month covers
// that month).
func PeriodInWindow(year, month int, from, to *time.Time) bool {
	p := year*12 + month
	if from != nil && p < from.Year()*12+int(from.Month()) {
		return false
	}
	if to != nil && p > to.Year()*12+int(to.Month()) {
		return false
	}
	return true
}
