package pdfdoc

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"testing"
)

// The xref offsets must point at each "N 0 obj" and the stream /Length must
// match its content; the old hand-written PDFs had neither.
func TestBuildIsWellFormed(t *testing.T) {
	var lines []Line
	for i := 0; i < 120; i++ { // forces several pages
		lines = append(lines, Line{Text: fmt.Sprintf("Baris (%d) \\ uji", i)})
	}
	out := Build(lines)
	if !bytes.HasPrefix(out, []byte("%PDF-1.4")) || !bytes.HasSuffix(out, []byte("%%EOF\n")) {
		t.Fatal("missing header/trailer")
	}
	m := regexp.MustCompile(`startxref\n(\d+)\n`).FindSubmatch(out)
	xref, _ := strconv.Atoi(string(m[1]))
	if !bytes.HasPrefix(out[xref:], []byte("xref\n")) {
		t.Fatalf("startxref %d does not point at xref", xref)
	}
	offs := regexp.MustCompile(`(\d{10}) 00000 n `).FindAllSubmatch(out[xref:], -1)
	for i, o := range offs {
		off, _ := strconv.Atoi(string(o[1]))
		if want := fmt.Sprintf("%d 0 obj", i+1); !bytes.HasPrefix(out[off:], []byte(want)) {
			t.Fatalf("xref entry %d -> %q", i+1, out[off:off+10])
		}
	}
	for _, sm := range regexp.MustCompile(`/Length (\d+) >>\nstream\n`).FindAllSubmatchIndex(out, -1) {
		n, _ := strconv.Atoi(string(out[sm[2]:sm[3]]))
		if !bytes.HasPrefix(out[sm[1]+n:], []byte("endstream")) {
			t.Fatal("stream /Length mismatch")
		}
	}
	if !bytes.Contains(out, []byte(`Baris \(7\) \\ uji`)) {
		t.Fatal("text not escaped")
	}
	if Rupiah(-1234567) != "-Rp1.234.567" {
		t.Fatal(Rupiah(-1234567))
	}
}
