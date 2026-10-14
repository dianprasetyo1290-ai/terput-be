// Package pdfdoc is a minimal PDF-1.4 writer (Helvetica, A4, text only)
// with correct stream lengths and xref offsets, used for printable ops
// documents (purchase orders, stock opname). Same writer as hr payslips.
package pdfdoc

import (
	"bytes"
	"fmt"
	"strings"
)

// Line is one text line; X defaults to 50, Size to 10. Right > 0 right-aligns
// the text ending near that x; Gap adds space before the line (negative
// keeps it on the previous line's baseline).
type Line struct {
	X, Size float64
	Bold    bool
	Text    string
	Right   float64 // >0: right-aligned text ending at this x (approximate)
	Gap     float64 // extra space before this line
}

func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' || r == '(' || r == ')':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 32:
			b.WriteByte(' ')
		case r > 126:
			// WinAnsi subset only; keep the document valid.
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// buildPDF lays lines top to bottom, starting a new page when full.
func Build(lines []Line) []byte {
	const top, bottom = 800.0, 50.0
	var pages []string
	var cur strings.Builder
	y := top
	flush := func() {
		pages = append(pages, cur.String())
		cur.Reset()
		y = top
	}
	for _, l := range lines {
		size := l.Size
		if size == 0 {
			size = 10
		}
		step := size*1.45 + l.Gap
		if y-step < bottom {
			flush()
		}
		y -= step
		font := "/F1"
		if l.Bold {
			font = "/F2"
		}
		x := l.X
		if x == 0 {
			x = 50
		}
		if l.Right > 0 {
			// Helvetica digits are 0.556 em; good enough for amounts.
			x = l.Right - float64(len(l.Text))*size*0.53
		}
		fmt.Fprintf(&cur, "BT %s %.1f Tf %.1f %.1f Td (%s) Tj ET\n", font, size, x, y, escape(l.Text))
	}
	flush()

	// Objects: 1 catalog, 2 pages, 3 F1, 4 F2, then page/content pairs.
	var objs []string
	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 5+2*i)
	}
	objs = append(objs,
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>",
	)
	for i, content := range pages {
		objs = append(objs,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents %d 0 R /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> >>", 6+2*i),
			fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content),
		)
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return buf.Bytes()
}

// rupiah formats an amount as Rp1.234.567 (negative with a leading minus).
func Rupiah(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%d", v)
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-Rp" + string(out)
	}
	return "Rp" + string(out)
}
