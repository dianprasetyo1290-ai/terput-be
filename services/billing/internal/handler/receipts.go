package handler

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"skci/billing/internal/model"
)

// atomicNextReceiptNo generates gapless unique receipt numbers safe for concurrency (T6 §3).
func (a *App) atomicNextReceiptNo(ctx context.Context, schoolID string, year int) (string, error) {
	if year <= 0 {
		year = time.Now().Year()
	}
	var seq int
	query := `INSERT INTO receipt_sequences (school_id, year, last)
	          VALUES ($1, $2, 1)
	          ON CONFLICT (school_id, year)
	          DO UPDATE SET last = receipt_sequences.last + 1
	          RETURNING last`
	err := a.DB.QueryRowContext(ctx, query, schoolID, year).Scan(&seq)
	if err != nil {
		return "", err
	}
	return model.FormatReceiptNo(year, seq), nil
}

// ReceiptDetails holds receipt metadata for digital generation.
type ReceiptDetails struct {
	ReceiptNo   string
	SchoolName  string
	StudentName string
	PayerName   string
	AmountIDR  int64
	MethodLabel string
	Date        time.Time
	Items       []string
}

// generateReceiptDocument creates a printable PDF receipt saved in private storage.
func (a *App) generateReceiptDocument(ctx context.Context, schoolID string, d ReceiptDetails) (string, error) {
	docKey := fmt.Sprintf("receipts/%s/%s.pdf", schoolID, d.ReceiptNo)
	pdfBytes := buildReceiptPDF(d)

	r := bytes.NewReader(pdfBytes)
	_, err := a.Storage.Put(ctx, docKey, "application/pdf", r, int64(len(pdfBytes)))
	if err != nil {
		return "", err
	}
	return docKey, nil
}

func escapePDF(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}

// buildReceiptPDF constructs a compliant PDF-1.4 document in pure Go.
func buildReceiptPDF(d ReceiptDetails) []byte {
	itemsStr := strings.Join(d.Items, ", ")
	textStream := fmt.Sprintf(
		"BT\n"+
			"/F1 18 Tf\n"+
			"50 740 Td\n"+
			"(KUITANSI PEMBAYARAN RESMI) Tj\n"+
			"/F1 11 Tf\n"+
			"0 -30 Td\n"+
			"(No. Kuitansi: %s) Tj\n"+
			"0 -22 Td\n"+
			"(Sekolah: %s) Tj\n"+
			"0 -22 Td\n"+
			"(Tanggal: %s) Tj\n"+
			"0 -22 Td\n"+
			"(Pembayar: %s) Tj\n"+
			"0 -22 Td\n"+
			"(Siswa: %s) Tj\n"+
			"0 -22 Td\n"+
			"(Metode: %s) Tj\n"+
			"0 -22 Td\n"+
			"(Keterangan: %s) Tj\n"+
			"0 -30 Td\n"+
			"/F1 14 Tf\n"+
			"(TOTAL TERBAYAR: Rp %d) Tj\n"+
			"/F1 9 Tf\n"+
			"0 -40 Td\n"+
			"(Dokumen ini sah diterbitkan secara digital oleh SEKOCI LMS.) Tj\n"+
			"ET\n",
		escapePDF(d.ReceiptNo),
		escapePDF(d.SchoolName),
		escapePDF(d.Date.Format("02-01-2006 15:04 WIB")),
		escapePDF(d.PayerName),
		escapePDF(d.StudentName),
		escapePDF(d.MethodLabel),
		escapePDF(itemsStr),
		d.AmountIDR,
	)

	streamLen := len(textStream)

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n"
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", streamLen, textStream)
	obj5 := "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n"

	header := "%PDF-1.4\n"
	offset1 := len(header)
	offset2 := offset1 + len(obj1)
	offset3 := offset2 + len(obj2)
	offset4 := offset3 + len(obj3)
	offset5 := offset4 + len(obj4)
	xrefOffset := offset5 + len(obj5)

	xref := fmt.Sprintf(
		"xref\n"+
			"0 6\n"+
			"0000000000 65535 f \n"+
			"%010d 00000 n \n"+
			"%010d 00000 n \n"+
			"%010d 00000 n \n"+
			"%010d 00000 n \n"+
			"%010d 00000 n \n",
		offset1, offset2, offset3, offset4, offset5,
	)
	trailer := fmt.Sprintf(
		"trailer\n"+
			"<< /Size 6 /Root 1 0 R >>\n"+
			"startxref\n"+
			"%d\n"+
			"%%%%EOF\n",
		xrefOffset,
	)

	var buf bytes.Buffer
	buf.WriteString(header)
	buf.WriteString(obj1)
	buf.WriteString(obj2)
	buf.WriteString(obj3)
	buf.WriteString(obj4)
	buf.WriteString(obj5)
	buf.WriteString(xref)
	buf.WriteString(trailer)

	return buf.Bytes()
}
