package tax

import (
	"fmt"
	"strings"
)

// Pure calculation functions for Tax Non-Payroll & e-Faktur module.
// Strictly integer math, rupiah amounts, rates in basis points.

// PickRate selects the applicable withholding tax rate in basis points.
// If the vendor does not have a valid NPWP, the penalty rate (noNPWPRateBP) applies.
func PickRate(standardRateBP, noNPWPRateBP int, hasNPWP bool) int {
	if hasNPWP {
		return standardRateBP
	}
	if noNPWPRateBP > 0 {
		return noNPWPRateBP
	}
	return standardRateBP
}

// WithholdingAmount computes withholding tax in IDR.
// Formula: (grossIDR * rateBP) / 10000. Strictly integer math.
func WithholdingAmount(grossIDR int64, rateBP int) int64 {
	if grossIDR <= 0 || rateBP <= 0 {
		return 0
	}
	return (grossIDR * int64(rateBP)) / 10000
}

// CalculateVAT computes PPN in IDR.
// Formula: (dppIDR * ppnRateBP) / 10000.
func CalculateVAT(dppIDR int64, ppnRateBP int) int64 {
	if dppIDR <= 0 || ppnRateBP <= 0 {
		return 0
	}
	return (dppIDR * int64(ppnRateBP)) / 10000
}

// FormatEBupot23CSV exports e-Bupot 23 compliant format for DJP.
func FormatEBupot23CSV(slips []WithholdingSlip) string {
	var sb strings.Builder
	sb.WriteString("NO_BUKTI_POTONG,MASA_PAJAK,NPWP_VENDOR,NAMA_VENDOR,OBJEK_PAJAK,JUMLAH_BRUTO,TARIF_BP,JUMLAH_PPH\n")
	for _, s := range slips {
		npwp := ""
		if s.NPWPUsed != nil {
			npwp = *s.NPWPUsed
		}
		sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%d,%d,%d\n",
			s.SlipNumber,
			s.TaxPeriod,
			npwp,
			escapeCSV(s.VendorName),
			s.TypeName,
			s.GrossAmountIDR,
			s.RateBP,
			s.TaxAmountIDR,
		))
	}
	return sb.String()
}

// FormatEFakturInputCSV exports e-Faktur importable format for Pajak Masukan.
func FormatEFakturInputCSV(vats []InputVAT) string {
	var sb strings.Builder
	sb.WriteString("FM,KD_JENIS_TRANSAKSI,FG_PENGGANTI,NOMOR_FAKTUR,MASA_PAJAK,TAHUN_PAJAK,TANGGAL_FAKTUR,NPWP,NAMA,ALAMAT,JUMLAH_DPP,JUMLAH_PPN,JUMLAH_PPNBM,IS_CREDITABLE\n")
	for _, v := range vats {
		parts := strings.Split(v.TaxPeriod, "-")
		masa := parts[1]
		tahun := parts[0]
		isCred := "1"
		if !v.Creditable {
			isCred = "0"
		}
		sb.WriteString(fmt.Sprintf("FM,01,0,%s,%s,%s,%s,%s,%s,-,%d,%d,0,%s\n",
			v.InvoiceNumber,
			masa,
			tahun,
			v.InvoiceDate,
			"-", // NPWP
			escapeCSV(v.VendorName),
			v.DPPIDR,
			v.VATIDR,
			isCred,
		))
	}
	return sb.String()
}

func escapeCSV(val string) string {
	if strings.ContainsAny(val, ",\"\n") {
		return "\"" + strings.ReplaceAll(val, "\"", "\"\"") + "\""
	}
	return val
}
