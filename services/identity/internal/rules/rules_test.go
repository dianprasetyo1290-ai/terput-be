package rules

import (
	"testing"
	"time"
)

func sp(s string) *string { return &s }

func TestValidEmail(t *testing.T) {
	cases := map[string]bool{
		"guru@teratai.sch.id": true, "a.b+c@x.co": true,
		"bukan-email": false, "a@b": false, "": false, "Budi <b@x.id>": false, "a@.x": false, "a@x.": false, " a@x.id": true,
	}
	for in, want := range cases {
		if got := ValidEmail(in); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}

func TestIdentityNumbers(t *testing.T) {
	cases := []struct {
		nisn, nik *string
		want      []string
	}{
		{nil, nil, nil},
		{sp("0012345678"), sp("3171012345678901"), nil},
		{sp("ABC123XYZ"), nil, []string{"nisn"}},
		{sp("123456789"), sp("317101234567890"), []string{"nisn", "nik"}},
		{sp(""), sp(""), nil},
	}
	for i, c := range cases {
		errs := ValidateIdentityNumbers(c.nisn, c.nik)
		if len(errs) != len(c.want) {
			t.Errorf("case %d: %v", i, errs)
		}
		for _, f := range c.want {
			if errs[f] == "" {
				t.Errorf("case %d: missing %s", i, f)
			}
		}
	}
}

func TestValidateSchool(t *testing.T) {
	cases := []struct {
		name string
		s    School
		want []string
	}{
		{"valid", School{Name: "SD Teratai", Code: sp("TPGS"), Level: sp("SD")}, nil},
		{"no code no level (BUG-013)", School{Name: "[QA] Unit Tanpa Kode"}, []string{"code", "level"}},
		{"blank code", School{Name: "X", Code: sp("  "), Level: sp("SD")}, []string{"code"}},
		{"bad code", School{Name: "X", Code: sp("A B"), Level: sp("SD")}, []string{"code"}},
		{"bad email", School{Name: "X", Code: sp("AB"), Level: sp("SD"), Email: sp("x")}, []string{"email"}},
		{"partial settlement", School{Name: "X", Code: sp("AB"), Level: sp("SD"), SettlementAccountNumber: sp("12ab")},
			[]string{"settlement_bank_name", "settlement_account_name", "settlement_account_number"}},
		{"full settlement", School{Name: "X", Code: sp("AB"), Level: sp("SD"), SettlementAccountNumber: sp("1234567890"), SettlementBankName: sp("BSI"), SettlementAccountName: sp("Yayasan")}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateSchool(c.s)
			if len(errs) != len(c.want) {
				t.Fatalf("%v", errs)
			}
			for _, f := range c.want {
				if errs[f] == "" {
					t.Fatalf("missing %s in %v", f, errs)
				}
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	for _, in := range []string{"0812-3456-789", "+62 812 3456 789", "628123456789", "8123456789"} {
		if got := NormalizePhone(in); got != "628123456789" {
			t.Errorf("%q -> %q", in, got)
		}
	}
}

func TestYearDates(t *testing.T) {
	d := func(s string) *time.Time { t, _ := ParseDate(s); return t }
	cases := []struct {
		name       string
		start, end *time.Time
		want       []string
	}{
		{"ok", d("2026-07-01"), d("2027-06-30"), nil},
		{"missing", nil, nil, []string{"start_date", "end_date"}},
		{"end before start", d("2026-10-14"), d("2026-01-01"), []string{"end_date"}},
		{"same day", d("2026-07-01"), d("2026-07-01"), []string{"end_date"}},
		{"too long", d("2026-07-01"), d("2029-06-30"), []string{"end_date"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateYearDates(c.start, c.end)
			if len(errs) != len(c.want) {
				t.Fatalf("%v", errs)
			}
		})
	}
	if _, ok := ParseDate("14-10-2026"); ok {
		t.Error("bad layout accepted")
	}
	s, e, ok := DefaultYearRange("2026/2027")
	if !ok || s.Format(dateLayout) != "2026-07-01" || e.Format(dateLayout) != "2027-06-30" {
		t.Errorf("default range %v %v %v", s, e, ok)
	}
	if _, _, ok := DefaultYearRange("2026/2028"); ok {
		t.Error("non-consecutive label accepted")
	}
	if _, _, ok := DefaultYearRange("TA 2026"); ok {
		t.Error("free label accepted")
	}
}

func TestStudentTransition(t *testing.T) {
	cases := []struct {
		name       string
		from, to   string
		correction bool
		role       string
		reason     string
		ok         bool
	}{
		{"calon -> aktif", "calon", "aktif", false, "admin", "", true},
		{"aktif -> lulus", "aktif", "lulus", false, "admin", "", true},
		{"aktif -> keluar needs reason", "aktif", "keluar", false, "admin", "", false},
		{"aktif -> keluar with reason", "aktif", "keluar", false, "admin", "pindah sekolah", true},
		{"keluar -> aktif (re-admission)", "keluar", "aktif", false, "admin", "", true},
		{"lulus -> aktif refused (BUG-075)", "lulus", "aktif", false, "admin", "", false},
		{"lulus -> aktif correction by admin refused", "lulus", "aktif", true, "admin", "salah input kelulusan", false},
		{"lulus -> aktif correction by kepsek, short reason", "lulus", "aktif", true, "kepala_sekolah", "salah", false},
		{"lulus -> aktif correction by kepsek", "lulus", "aktif", true, "kepala_sekolah", "salah input kelulusan", true},
		{"calon -> lulus refused", "calon", "lulus", false, "admin", "", false},
		{"unknown status", "aktif", "alumni", false, "admin", "", false},
		{"same status", "aktif", "aktif", false, "admin", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := StudentTransition(c.from, c.to, c.correction, c.role, c.reason)
			if (got == "") != c.ok {
				t.Fatalf("got %q", got)
			}
		})
	}
	if RoleStatusFor("lulus") != "inactive" || RoleStatusFor("aktif") != "active" || RoleStatusFor("calon") != "active" {
		t.Error("role status mapping")
	}
}
