package pkl

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

type CertificateDetails struct {
	CertificateNo string
	SchoolName    string
	StudentName   string
	StudentNIS    string
	MajorName     string
	PartnerName   string
	FinalScoreStr string
	StartsOn      time.Time
	EndsOn        time.Time
	IssuedDate    time.Time
	IssuedBy      string
}

func escapePDF(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}

// BuildCertificatePDF constructs a compliant PDF-1.4 document for PKL certificate.
func BuildCertificatePDF(d CertificateDetails) []byte {
	periodStr := fmt.Sprintf("%s s/d %s", d.StartsOn.Format("02 Jan 2006"), d.EndsOn.Format("02 Jan 2006"))
	schoolName := d.SchoolName
	if schoolName == "" {
		schoolName = "SMK Teratai Putih Global"
	}

	textStream := fmt.Sprintf(
		"BT\n"+
			"/F1 20 Tf\n"+
			"50 750 Td\n"+
			"(SERTIFIKAT PRAKTIK KERJA LAPANGAN) Tj\n"+
			"/F1 11 Tf\n"+
			"0 -30 Td\n"+
			"(Nomor Sertifikat: %s) Tj\n"+
			"0 -25 Td\n"+
			"(Diberikan oleh: %s) Tj\n"+
			"0 -25 Td\n"+
			"(Menyatakan bahwa siswa di bawah ini:) Tj\n"+
			"0 -28 Td\n"+
			"/F1 16 Tf\n"+
			"(NAMA: %s) Tj\n"+
			"/F1 11 Tf\n"+
			"0 -22 Td\n"+
			"(NIS: %s) Tj\n"+
			"0 -22 Td\n"+
			"(Program Keahlian / Jurusan: %s) Tj\n"+
			"0 -25 Td\n"+
			"(Telah menyelesaikan Praktik Kerja Lapangan (PKL) pada:) Tj\n"+
			"0 -22 Td\n"+
			"/F1 13 Tf\n"+
			"(Mitra Industri: %s) Tj\n"+
			"/F1 11 Tf\n"+
			"0 -22 Td\n"+
			"(Periode: %s) Tj\n"+
			"0 -28 Td\n"+
			"/F1 14 Tf\n"+
			"(NILAI AKHIR PRESTASI PKL: %s) Tj\n"+
			"/F1 10 Tf\n"+
			"0 -40 Td\n"+
			"(Diterbitkan di Jakarta, %s) Tj\n"+
			"0 -20 Td\n"+
			"(Koordinator PKL: %s) Tj\n"+
			"0 -35 Td\n"+
			"/F1 8 Tf\n"+
			"(Dokumen ini sah dan diterbitkan secara digital oleh SEKOCI LMS Teratai Putih.) Tj\n"+
			"ET\n",
		escapePDF(d.CertificateNo),
		escapePDF(schoolName),
		escapePDF(d.StudentName),
		escapePDF(d.StudentNIS),
		escapePDF(d.MajorName),
		escapePDF(d.PartnerName),
		escapePDF(periodStr),
		escapePDF(d.FinalScoreStr),
		escapePDF(d.IssuedDate.Format("02 January 2006")),
		escapePDF(d.IssuedBy),
	)

	streamLen := len(textStream)

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n"
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", streamLen, textStream)
	obj5 := "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>\nendobj\n"

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
