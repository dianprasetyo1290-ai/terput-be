package simplepdf

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestEscape(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Bukti (PPDB)", `Bukti \(PPDB\)`},
		{`a\b`, `a\\b`},
		{"Rp1.000\n", "Rp1.000 "},
		{"é", `\351`},
		{"日本", "??"},
	}
	for _, c := range cases {
		if got := Escape(c.in); got != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestRenderStructure(t *testing.T) {
	pdf := Render("Bukti", []Line{{Text: "BUKTI PENDAFTARAN", Bold: true, Size: 16}, {Text: "No: PPDB-2027-G01-0001"}})
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		t.Fatal("missing header/trailer")
	}
	s := string(pdf)
	for _, want := range []string{"/Helvetica-Bold", "(PPDB-2027-G01-0001)", "xref", "startxref"} {
		if !strings.Contains(s, want) && !strings.Contains(s, strings.Trim(want, "()")) {
			t.Errorf("missing %s", want)
		}
	}
	// Each xref offset must point at its "n 0 obj".
	idx := strings.Index(s, "xref\n")
	entries := strings.Split(s[idx:], "\n")[3:10]
	for i, e := range entries {
		var off int
		if _, err := fmt.Sscanf(e, "%010d", &off); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(s[off:], strconv.Itoa(i+1)+" 0 obj") {
			t.Errorf("xref %d points to %q", i+1, s[off:off+10])
		}
	}
}
