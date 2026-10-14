package userimport

import (
	"strings"
	"testing"
)

func TestMapKindColumns(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		header  []string
		mapping map[string]string
		missing string
	}{
		{"parent ok", KindOrangTua, []string{"Nama", "Email", "Hubungan", "ID Sumber Siswa"}, nil, ""},
		{"parent nisn ok", KindOrangTua, []string{"Nama", "Email", "Hubungan", "NISN Siswa"}, nil, ""},
		{"parent no relationship", KindOrangTua, []string{"Nama", "Email", "NISN Siswa"}, nil, "Hubungan"},
		{"parent no student", KindOrangTua, []string{"Nama", "Email", "Hubungan"}, nil, "ID Sumber Siswa atau NISN Siswa"},
		{"class ok", KindKelas, []string{"Rombel", "NISN"}, nil, ""},
		{"class mapping", KindKelas, []string{"Kls", "Kode Siswa"}, map[string]string{"class_name": "Kls", "student_ref": "Kode Siswa"}, ""},
		{"class no name", KindKelas, []string{"NISN"}, nil, "Nama Kelas"},
	}
	for _, c := range cases {
		_, missing := MapKindColumns(c.kind, c.header, c.mapping)
		got := strings.Join(missing, ", ")
		if (c.missing == "" && got != "") || (c.missing != "" && !strings.Contains(got, c.missing)) {
			t.Errorf("%s: missing=%q want %q", c.name, got, c.missing)
		}
	}
}

func TestValidateGuardianRows(t *testing.T) {
	cols, _ := MapKindColumns(KindOrangTua, []string{"Nama", "Email", "Hubungan", "ID Sumber Siswa", "NISN Siswa"}, nil)
	rows := [][]string{
		{"Ibu A", "ibu@x.id", "Ibu", "S1", ""},
		{"Ibu A", "ibu@x.id", "ibu", "S2", ""},              // second child: ok
		{"Ibu B", "ibu@x.id", "ibu", "S3", ""},              // same email, other name
		{"Ibu A", "ibu@x.id", "ibu", "S1", ""},              // duplicate pair
		{"Ayah", "ayah@x.id", "paman", "S1", ""},            // bad relationship
		{"Ayah", "ayah@x.id", "ayah", "", ""},               // no student
		{"Ayah", "ayah@x.id", "ayah", "", "123"},            // bad NISN
		{"", "", "", "", ""},                                // blank
		{"Wali", "wali@x.id", "guardian", "", "0012345678"}, // ok
	}
	valid, errs := ValidateGuardianRows(rows, cols)
	if len(valid) != 3 || len(errs) != 5 {
		t.Fatalf("valid=%d errs=%v", len(valid), errs)
	}
	wantIssues := map[int]string{4: "nama berbeda", 5: "Relasi sama dengan baris 2", 6: "ayah, ibu, atau wali", 7: "Isi ID Sumber Siswa", 8: "10 digit"}
	for _, e := range errs {
		if !strings.Contains(e.Issue, wantIssues[e.Row]) {
			t.Errorf("row %d: %q", e.Row, e.Issue)
		}
	}
	if valid[2].Relationship != "wali" {
		t.Errorf("relationship alias: %q", valid[2].Relationship)
	}
}

func TestValidateClassRows(t *testing.T) {
	cols, _ := MapKindColumns(KindKelas, []string{"Nama Kelas", "Email Wali Kelas", "ID Sumber Siswa"}, nil)
	rows := [][]string{
		{"X-A", "wali@x.id", "S1"},
		{"x-a", "", "S2"},
		{"X-A", "lain@x.id", "S3"}, // other wali for same class
		{"X-B", "", "S1"},          // student twice
		{"", "", "S4"},             // no class
		{"X-C", "bukan-email", "S5"},
	}
	valid, errs := ValidateClassRows(rows, cols)
	if len(valid) != 2 || len(errs) != 4 {
		t.Fatalf("valid=%d errs=%v", len(valid), errs)
	}
	want := map[int]string{4: "Wali kelas berbeda", 5: "Siswa sama dengan baris 2", 6: "Nama Kelas wajib", 7: "tidak valid"}
	for _, e := range errs {
		if !strings.Contains(e.Issue, want[e.Row]) {
			t.Errorf("row %d: %q", e.Row, e.Issue)
		}
	}
}
