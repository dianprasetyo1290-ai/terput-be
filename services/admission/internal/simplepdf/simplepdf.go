// Package simplepdf renders a one-page A4 text document as PDF using only the
// standard library (built-in Helvetica, WinAnsi encoding). It is enough for
// receipts such as the PPDB registration proof.
package simplepdf

import (
	"bytes"
	"fmt"
	"strings"
)

// Line is one line of text; Bold uses Helvetica-Bold, Size is in points
// (default 11).
type Line struct {
	Text string
	Bold bool
	Size float64
}

// Render lays the lines out top-down on one A4 page. Lines that do not fit
// are dropped, so callers keep documents short.
func Render(title string, lines []Line) []byte {
	var content bytes.Buffer
	y := 800.0
	for _, l := range lines {
		size := l.Size
		if size == 0 {
			size = 11
		}
		if y < 50 {
			break
		}
		font := "F1"
		if l.Bold {
			font = "F2"
		}
		fmt.Fprintf(&content, "BT /%s %.1f Tf 56 %.1f Td (%s) Tj ET\n", font, size, y, Escape(l.Text))
		y -= size * 1.6
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R /F2 5 0 R >> >> /Contents 6 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()),
		fmt.Sprintf("<< /Title (%s) /Producer (SEKOCI PPDB) >>", Escape(title)),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, obj := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, len(objects), xref)
	return out.Bytes()
}

// Escape makes text safe inside a PDF literal string: backslash and
// parentheses are escaped, characters outside Latin-1 become '?', control
// characters become spaces.
func Escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' || r == '(' || r == ')':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20:
			b.WriteByte(' ')
		case r < 0x80:
			b.WriteRune(r)
		case r <= 0xFF:
			fmt.Fprintf(&b, "\\%03o", r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}
