package payroll

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Minimal PDF-1.4 writer (Helvetica, A4, text only) with correct stream
// lengths and xref offsets. Output is deterministic for the same input, so a
// payslip re-rendered from the same payroll_lines row has the same sha256
// (T3 FR-96-004).

type pdfLine struct {
	X, Size float64
	Bold    bool
	Text    string
	Right   float64 // >0: right-aligned text ending at this x (approximate)
	Gap     float64 // extra space before this line
}

func pdfEscape(s string) string {
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
func buildPDF(lines []pdfLine) []byte {
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
		fmt.Fprintf(&cur, "BT %s %.1f Tf %.1f %.1f Td (%s) Tj ET\n", font, size, x, y, pdfEscape(l.Text))
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
func rupiah(v int64) string {
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

// PayslipDoc is everything printed on one payslip.
type PayslipDoc struct {
	SchoolName   string
	Period       string
	RunKind      string
	EmployeeName string
	EmployeeNo   string
	Line         PayrollLine
	Details      LineDetails
}

var bpjsLabels = map[string]string{
	"kesehatan": "BPJS Kesehatan", "jht": "BPJS JHT", "jp": "BPJS JP", "jkk": "BPJS JKK", "jkm": "BPJS JKM",
}

// RenderPayslipPDF builds the payslip PDF and its sha256 (hex).
func RenderPayslipPDF(d PayslipDoc) ([]byte, string) {
	const amountX = 545.0
	kind := "Reguler"
	if d.RunKind == "supplementary" {
		kind = "Susulan"
	}
	school := d.SchoolName
	if school == "" {
		school = "Sekolah"
	}
	lines := []pdfLine{
		{Text: "SLIP GAJI", Size: 16, Bold: true},
		{Text: school, Size: 11},
		{Text: fmt.Sprintf("Periode %s (%s)", d.Period, kind), Size: 10},
		{Text: "Nama: " + d.EmployeeName, Gap: 10},
		{Text: "No. Pegawai: " + d.EmployeeNo},
		{Text: "Status PTKP: " + d.Details.PTKP},
		{Text: "PENDAPATAN", Bold: true, Gap: 12},
	}
	row := func(label string, v int64) {
		lines = append(lines, pdfLine{Text: label, X: 60}, pdfLine{Text: rupiah(v), Right: amountX, Gap: -14.5})
	}
	for _, it := range sortedItems(d.Details.Items) {
		if it.Kind == "earning" {
			row(it.Name, it.AmountIDR)
		}
	}
	lines = append(lines, pdfLine{Text: "Total Pendapatan (Bruto)", Bold: true, X: 60})
	lines = append(lines, pdfLine{Text: rupiah(d.Line.GrossIDR), Bold: true, Right: amountX, Gap: -14.5})

	lines = append(lines, pdfLine{Text: "POTONGAN", Bold: true, Gap: 12})
	for _, prog := range []string{"kesehatan", "jht", "jp"} {
		if v := d.Details.BPJS.Employee[prog]; v > 0 {
			row(bpjsLabels[prog], v)
		}
	}
	pphLabel := "PPh 21"
	if d.Details.TERCategory != "" {
		pphLabel = fmt.Sprintf("PPh 21 (TER %s %d,%02d%%)", d.Details.TERCategory, d.Details.TERRateBP/100, d.Details.TERRateBP%100)
	} else if d.Details.Annual != nil {
		pphLabel = "PPh 21 (perhitungan tahunan)"
	}
	row(pphLabel, d.Line.PPh21IDR)
	for _, it := range d.Details.Items {
		if it.Kind == "deduction" {
			row(it.Name, it.AmountIDR)
		}
	}
	totalDed := d.Line.BPJSEmployeeIDR + d.Line.PPh21IDR + d.Line.OtherDeductionsIDR
	lines = append(lines, pdfLine{Text: "Total Potongan", Bold: true, X: 60})
	lines = append(lines, pdfLine{Text: rupiah(totalDed), Bold: true, Right: amountX, Gap: -14.5})

	lines = append(lines, pdfLine{Text: "GAJI BERSIH (TAKE HOME PAY)", Bold: true, Size: 12, Gap: 14})
	lines = append(lines, pdfLine{Text: rupiah(d.Line.NetIDR), Bold: true, Size: 12, Right: amountX, Gap: -17.4})

	if a := d.Details.Annual; a != nil {
		lines = append(lines, pdfLine{Text: "Rekonsiliasi PPh 21 tahunan", Bold: true, Gap: 12})
		row("Biaya jabatan", a.BiayaJabatanIDR)
		row("Penghasilan neto setahun", a.NettoIDR)
		row("PTKP", a.PTKPIDR)
		row("PKP", a.PKPIDR)
		row("PPh 21 terutang setahun", a.AnnualTaxIDR)
		if a.OverpaidIDR > 0 {
			row("Lebih potong", a.OverpaidIDR)
		}
	}
	if len(d.Details.BPJS.Employer) > 0 {
		lines = append(lines, pdfLine{Text: "Iuran ditanggung sekolah (bukan potongan)", Bold: true, Gap: 12})
		for _, prog := range []string{"kesehatan", "jht", "jp", "jkk", "jkm"} {
			if v := d.Details.BPJS.Employer[prog]; v > 0 {
				row(bpjsLabels[prog], v)
			}
		}
	}
	lines = append(lines, pdfLine{Text: "Dokumen ini diterbitkan secara elektronik oleh SEKOCI dan sah tanpa tanda tangan.", Size: 8, Gap: 20})

	pdf := buildPDF(lines)
	sum := sha256.Sum256(pdf)
	return pdf, hex.EncodeToString(sum[:])
}
