package userimport

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestReadTableFormats(t *testing.T) {
	f := excelize.NewFile()
	f.SetSheetRow("Sheet1", "A1", &[]string{"Nama", "Email"})
	f.SetSheetRow("Sheet1", "A2", &[]string{"Ani", "ani@x.id"})
	var xb bytes.Buffer
	_ = f.Write(&xb)

	cases := []struct {
		name, file string
		data       []byte
		format     string
		rows       int
		wantErr    string
	}{
		{"xlsx", "data.xlsx", xb.Bytes(), "xlsx", 2, ""},
		{"xlsx upper ext", "DATA.XLSX", xb.Bytes(), "xlsx", 2, ""},
		{"csv comma with BOM", "a.csv", []byte("\xef\xbb\xbfNama,Email\nAni,ani@x.id\n"), "csv", 2, ""},
		{"csv semicolon (Excel ID locale)", "a.csv", []byte("Nama;Email\nAni;ani@x.id\n"), "csv", 2, ""},
		{"xls rejected", "a.xls", []byte("\xd0\xcf\x11\xe0"), "", 0, "Format file tidak didukung"},
		{"pdf rejected", "a.pdf", []byte("%PDF-1.4"), "", 0, "Format file tidak didukung"},
		{"fake xlsx", "a.xlsx", []byte("Nama,Email"), "", 0, "bukan file Excel"},
		{"broken csv quote", "a.csv", []byte("Nama,Email\n\"Ani,ani@x.id\n"), "", 0, "CSV tidak valid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, format, err := ReadTable(c.file, c.data)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil || format != c.format || len(rows) != c.rows {
				t.Fatalf("got %v %q %d rows", err, format, len(rows))
			}
			if rows[0][0] != "Nama" || rows[1][1] != "ani@x.id" {
				t.Fatalf("rows %v", rows)
			}
		})
	}
}

func TestMapColumns(t *testing.T) {
	cases := []struct {
		name      string
		header    []string
		mapping   map[string]string
		roleFixed bool
		want      map[string]int
		missing   string
	}{
		{"template headers, any order", []string{"ID Sumber", "Peran (guru/siswa) *", "Email *", "Nama *", "Telepon"}, nil, false,
			map[string]int{FieldSourceRef: 0, FieldRole: 1, FieldEmail: 2, FieldName: 3, FieldPhone: 4}, ""},
		{"aliases", []string{"Nama Lengkap", "E-mail", "No HP"}, nil, true, map[string]int{FieldName: 0, FieldEmail: 1, FieldPhone: 2}, ""},
		{"missing role when not fixed", []string{"Nama", "Email"}, nil, false, map[string]int{FieldName: 0, FieldEmail: 1}, "Peran"},
		{"missing email and name", []string{"Telepon"}, nil, true, map[string]int{FieldPhone: 0}, "Nama,Email"},
		{"explicit mapping", []string{"Student", "Mail"}, map[string]string{FieldName: "student", FieldEmail: "MAIL"}, true,
			map[string]int{FieldName: 0, FieldEmail: 1}, ""},
		{"mapping to unknown header", []string{"Nama", "Email"}, map[string]string{FieldEmail: "Surel"}, true, map[string]int{FieldName: 0}, "Email"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, missing := MapColumns(c.header, c.mapping, c.roleFixed)
			if strings.Join(missing, ",") != c.missing {
				t.Fatalf("missing = %v, want %q", missing, c.missing)
			}
			if len(got) != len(c.want) {
				t.Fatalf("cols = %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Fatalf("cols[%s] = %d, want %d", k, got[k], v)
				}
			}
		})
	}
}

func TestValidateRows(t *testing.T) {
	cols := map[string]int{FieldName: 0, FieldEmail: 1, FieldRole: 2, FieldSourceRef: 3}
	allowed := map[string]bool{"guru": true, "siswa": true}
	rows := [][]string{
		{"Ani", "ani@x.id", "Siswa", "S1"},  // 2 ok (role case-insensitive)
		{"", "", "", ""},                    // 3 blank, skipped
		{"Budi", "budi@x", "siswa", ""},     // 4 bad email
		{"Cici", "ANI@x.id", "siswa", ""},   // 5 duplicate of row 2
		{"Dodi", "dodi@x.id", "admin", ""},  // 6 role not allowed
		{"", "eka@x.id", "guru", ""},        // 7 name missing
		{"Fafa", "fafa@x.id", "guru", "S1"}, // 8 duplicate source id
		{"Gina", "taken@x.id", "guru", ""},  // 9 already registered
		{"Hadi", "hadi@x.id", "", ""},       // 10 role missing
		{"Ika", "ika@x.id", "guru"},         // 11 short row ok
	}
	valid, errs := Validate(rows, cols, "", allowed, map[string]bool{"taken@x.id": true})
	if len(valid) != 2 || valid[0].Number != 2 || valid[0].Role != "siswa" || valid[1].Number != 11 {
		t.Fatalf("valid = %+v", valid)
	}
	want := map[int]string{4: "Format email", 5: "baris 2", 6: "tidak diizinkan", 7: "Nama wajib", 8: "ID Sumber sama", 9: "sudah terdaftar", 10: "Peran wajib"}
	if len(errs) != len(want) {
		t.Fatalf("errs = %+v", errs)
	}
	for _, e := range errs {
		if !strings.Contains(e.Issue, want[e.Row]) {
			t.Errorf("row %d issue %q, want %q", e.Row, e.Issue, want[e.Row])
		}
	}

	// fixed role (Data Siswa) ignores the Peran column
	valid, errs = Validate([][]string{{"Ani", "ani@x.id", "guru"}}, cols, "siswa", allowed, nil)
	if len(errs) != 0 || valid[0].Role != "siswa" {
		t.Fatalf("fixed role: %+v %+v", valid, errs)
	}
}

func TestDataRowCount(t *testing.T) {
	rows := make([][]string, 0, MaxRows+2)
	for i := 0; i < MaxRows+1; i++ {
		rows = append(rows, []string{"x"})
	}
	rows = append(rows, []string{"", " "})
	if n := DataRowCount(rows); n != MaxRows+1 {
		t.Fatalf("count = %d", n)
	}
}
