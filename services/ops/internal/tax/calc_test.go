package tax

import (
	"strings"
	"testing"
)

func TestPickRate(t *testing.T) {
	stdBP := 200     // 2% PPh 23
	noNpwpBP := 400  // 4% 100% surcharge

	// With NPWP
	if rate := PickRate(stdBP, noNpwpBP, true); rate != 200 {
		t.Errorf("PickRate with NPWP = %d; want 200", rate)
	}

	// Without NPWP
	if rate := PickRate(stdBP, noNpwpBP, false); rate != 400 {
		t.Errorf("PickRate without NPWP = %d; want 400", rate)
	}

	// Without NPWP when surcharge is not defined (e.g. final 10%)
	if rate := PickRate(1000, 0, false); rate != 1000 {
		t.Errorf("PickRate without NPWP when surcharge=0: %d; want 1000", rate)
	}
}

func TestWithholdingAmount(t *testing.T) {
	// Rp 10,000,000 * 2% = Rp 200,000
	gross := int64(10_000_000)
	tax := WithholdingAmount(gross, 200)
	if tax != 200_000 {
		t.Errorf("WithholdingAmount = %d; want 200000", tax)
	}

	// Rp 10,000,000 * 4% = Rp 400,000
	taxNoNpwp := WithholdingAmount(gross, 400)
	if taxNoNpwp != 400_000 {
		t.Errorf("WithholdingAmount = %d; want 400000", taxNoNpwp)
	}

	// Truncation check: Rp 1,555,555 * 2% = 31111.1 -> strictly 31111
	fracTax := WithholdingAmount(1_555_555, 200)
	if fracTax != 31111 {
		t.Errorf("WithholdingAmount fractional = %d; want 31111", fracTax)
	}
}

func TestCalculateVAT(t *testing.T) {
	// Rp 50,000,000 * 11% = Rp 5,500,000
	dpp := int64(50_000_000)
	vat := CalculateVAT(dpp, 1100)
	if vat != 5_500_000 {
		t.Errorf("CalculateVAT = %d; want 5500000", vat)
	}
}

func TestFormatEBupot23CSV(t *testing.T) {
	npwp := "01.234.567.8-901.000"
	slips := []WithholdingSlip{
		{
			SlipNumber:     "BP-2026-10-001",
			TaxPeriod:      "2026-10",
			NPWPUsed:       &npwp,
			VendorName:     "PT Maju Bersama",
			TypeName:       "PPh 23 Jasa",
			GrossAmountIDR: 10_000_000,
			RateBP:         200,
			TaxAmountIDR:   200_000,
		},
	}

	csv := FormatEBupot23CSV(slips)
	if !strings.Contains(csv, "BP-2026-10-001") {
		t.Errorf("CSV missing slip number")
	}
	if !strings.Contains(csv, "PT Maju Bersama") {
		t.Errorf("CSV missing vendor name")
	}
}
