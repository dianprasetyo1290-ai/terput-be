package handler

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Invoice PDF per bill (BUG-129)
// ---------------------------------------------------------------------------

type invoiceLine struct {
	Label     string
	AmountIDR int64
}

type invoiceDoc struct {
	InvoiceNo   string
	SchoolName  string
	StudentName string
	Title       string
	Period      string
	Status      string
	IssuedAt    time.Time
	DueAt       *time.Time
	Items       []invoiceLine
	Discounts   []invoiceLine
	NetIDR      int64
	PaidIDR     int64
	Outstanding int64
	Terms       []model.InstallmentState
	VoidReason  string
}

var invoiceStatusLabel = map[string]string{"unpaid": "BELUM DIBAYAR", "partial": "DIBAYAR SEBAGIAN", "paid": "LUNAS", "void": "DIBATALKAN"}

// InvoiceNumber is the printed number of a bill's invoice: period plus the
// bill id prefix, stable for the life of the bill.
func InvoiceNumber(year, month int, billID string) string {
	short := strings.ToUpper(strings.ReplaceAll(billID, "-", ""))
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("INV/%04d%02d/%s", year, month, short)
}

// pdfText keeps a line printable in the standard Helvetica (WinAnsi)
// encoding: PDF string escapes plus non-ASCII replaced.
func pdfText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' || r == '(' || r == ')':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case r < 32 || r > 126:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

type pdfLine struct {
	Size int
	Text string
	Gap  int // extra space before the line
}

// buildTextPDF lays out lines top-down on A4 pages (new page when full)
// and returns a self-contained PDF-1.4 document.
func buildTextPDF(lines []pdfLine) []byte {
	const top, bottom = 790, 50
	var pages []string
	var cur strings.Builder
	y := top
	open := func() {
		cur.Reset()
		cur.WriteString("BT\n")
		y = top
	}
	flush := func() {
		cur.WriteString("ET\n")
		pages = append(pages, cur.String())
	}
	open()
	for _, l := range lines {
		size := l.Size
		if size == 0 {
			size = 10
		}
		step := size + 6 + l.Gap
		if y-step < bottom {
			flush()
			open()
		}
		y -= step
		fmt.Fprintf(&cur, "/F1 %d Tf\n1 0 0 1 50 %d Tm\n(%s) Tj\n", size, y, pdfText(l.Text))
	}
	flush()

	// Objects: 1 catalog, 2 pages, 3 font, then (page, content) per page.
	var objs []string
	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 4+2*i)
	}
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)))
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	for i, content := range pages {
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>", 5+2*i))
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content))
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

// invoiceLines is the printed content of an invoice.
func invoiceLines(d invoiceDoc) []pdfLine {
	date := func(t time.Time) string { return t.Format("02-01-2006") }
	ls := []pdfLine{
		{Size: 18, Text: "INVOICE TAGIHAN"},
		{Size: 11, Text: d.SchoolName, Gap: 4},
		{Text: "No. Invoice: " + d.InvoiceNo, Gap: 10},
		{Text: "Siswa: " + d.StudentName},
		{Text: "Tagihan: " + d.Title + " - periode " + d.Period},
		{Text: "Tanggal terbit: " + date(d.IssuedAt)},
	}
	if d.DueAt != nil {
		ls = append(ls, pdfLine{Text: "Jatuh tempo: " + date(*d.DueAt)})
	}
	status := invoiceStatusLabel[d.Status]
	if status == "" {
		status = strings.ToUpper(d.Status)
	}
	ls = append(ls, pdfLine{Size: 11, Text: "Status: " + status})
	if d.VoidReason != "" {
		ls = append(ls, pdfLine{Text: "Alasan pembatalan: " + d.VoidReason})
	}
	ls = append(ls, pdfLine{Size: 12, Text: "Rincian", Gap: 10})
	for _, it := range d.Items {
		ls = append(ls, pdfLine{Text: fmt.Sprintf("  %s: %s", it.Label, formatRupiah(it.AmountIDR))})
	}
	for _, it := range d.Discounts {
		ls = append(ls, pdfLine{Text: fmt.Sprintf("  Potongan %s: -%s", it.Label, formatRupiah(it.AmountIDR))})
	}
	ls = append(ls,
		pdfLine{Size: 12, Text: "Total tagihan: " + formatRupiah(d.NetIDR), Gap: 8},
		pdfLine{Text: "Sudah dibayar: " + formatRupiah(d.PaidIDR)},
		pdfLine{Size: 12, Text: "Sisa yang harus dibayar: " + formatRupiah(d.Outstanding)},
	)
	if len(d.Terms) > 0 {
		ls = append(ls, pdfLine{Size: 12, Text: "Jadwal termin", Gap: 10})
		for _, t := range d.Terms {
			ls = append(ls, pdfLine{Text: fmt.Sprintf("  Termin %d - jatuh tempo %s: %s (dibayar %s)",
				t.Seq, date(t.DueAt), formatRupiah(t.AmountIDR), formatRupiah(t.PaidIDR))})
		}
	}
	ls = append(ls, pdfLine{Size: 8, Text: "Dokumen ini diterbitkan secara digital oleh SEKOCI dan sah tanpa tanda tangan.", Gap: 20})
	return ls
}

// loadInvoice reads one bill of the school (and of studentID when set) as
// an invoice. sql.ErrNoRows when it does not exist there.
func (a *App) loadInvoice(ctx context.Context, schoolID, billID, studentID string) (invoiceDoc, error) {
	var (
		d                  invoiceDoc
		student            string
		y, m               int
		dueAt              sql.NullTime
		verified, reversed int64
	)
	err := a.DB.QueryRowContext(ctx,
		`SELECT b.student_user_id::text, COALESCE(s.name, b.source_module, 'Tagihan'), b.status, COALESCE(b.void_reason, ''),
		        b.period_year, b.period_month, b.net_idr, b.due_at, b.created_at
		   FROM student_bills b LEFT JOIN billing_schemes s ON s.id = b.scheme_id
		  WHERE b.id::text = $1 AND b.school_id = $2 AND ($3 = '' OR b.student_user_id::text = $3)`,
		billID, schoolID, studentID).
		Scan(&student, &d.Title, &d.Status, &d.VoidReason, &y, &m, &d.NetIDR, &dueAt, &d.IssuedAt)
	if err != nil {
		return d, err
	}
	loc := a.loc()
	d.IssuedAt = d.IssuedAt.In(loc)
	if dueAt.Valid {
		t := dueAt.Time.In(loc)
		d.DueAt = &t
	}
	d.InvoiceNo = InvoiceNumber(y, m, billID)
	d.Period = fmt.Sprintf("%02d/%d", m, y)
	d.StudentName = a.userNames(ctx, []string{student})[student]
	if d.StudentName == "" {
		d.StudentName = student
	}
	if a.IdentitySvc != nil {
		if n, err := a.IdentitySvc.SchoolName(ctx, schoolID); err == nil {
			d.SchoolName = n
		}
	}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT name, amount_idr FROM bill_items WHERE bill_id::text = $1 AND school_id = $2 ORDER BY created_at, id`, billID, schoolID)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var l invoiceLine
		if err := rows.Scan(&l.Label, &l.AmountIDR); err != nil {
			rows.Close()
			return d, err
		}
		d.Items = append(d.Items, l)
	}
	rows.Close()
	rows, err = a.DB.QueryContext(ctx,
		`SELECT label, applied_idr FROM bill_discount_steps WHERE bill_id::text = $1 AND school_id = $2 AND applied_idr > 0 ORDER BY sequence`, billID, schoolID)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var l invoiceLine
		if err := rows.Scan(&l.Label, &l.AmountIDR); err != nil {
			rows.Close()
			return d, err
		}
		d.Discounts = append(d.Discounts, l)
	}
	rows.Close()
	_, verified, reversed, d.Outstanding, err = billTotalsQ(ctx, a.DB, schoolID, billID)
	if err != nil {
		return d, err
	}
	d.PaidIDR = verified - reversed
	ins, err := loadInstallments(ctx, a.DB, schoolID, billID)
	if err != nil {
		return d, err
	}
	d.Terms = model.InstallmentProgress(ins, d.PaidIDR)
	for i := range d.Terms {
		d.Terms[i].DueAt = d.Terms[i].DueAt.In(loc)
	}
	return d, nil
}

func (a *App) writeInvoice(w http.ResponseWriter, r *http.Request, schoolID, billID, studentID string) {
	d, err := a.loadInvoice(r.Context(), schoolID, billID, studentID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "tagihan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	pdf := buildTextPDF(invoiceLines(d))
	name := strings.ReplaceAll(d.InvoiceNo, "/", "-") + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

// AdminBillInvoice is GET /admin/bills/{id}/invoice.pdf (finance viewers).
func (a *App) AdminBillInvoice(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	a.writeInvoice(w, r, schoolID, param(r, "id"), "")
}

// ChildBillInvoice is GET /parent/children/{studentUserId}/bills/{billId}/invoice.pdf.
func (a *App) ChildBillInvoice(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireChild(w, r)
	if !ok {
		return
	}
	a.writeInvoice(w, r, c.SchoolID, param(r, "billId"), studentID)
}

// MyBillInvoice is GET /me/bills/{id}/invoice.pdf (the student, when bills
// are visible to them).
func (a *App) MyBillInvoice(w http.ResponseWriter, r *http.Request) {
	c := common.RequireStudent(w, r)
	if c == nil {
		return
	}
	if !a.checkStudentBillVisibility(w, r, c.SchoolID, c.UserID) {
		return
	}
	a.writeInvoice(w, r, c.SchoolID, param(r, "id"), c.UserID)
}
