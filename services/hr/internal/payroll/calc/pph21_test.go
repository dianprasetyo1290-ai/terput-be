package calc

import (
	"errors"
	"testing"
)

// fixtureParams mirrors the shape of the seeded statutory tables; the
// numbers are test fixtures, the engine never hardcodes them.
func fixtureParams() Params {
	return Params{
		BPJS: map[string]BPJSRule{
			"kesehatan": {EmployerBP: 400, EmployeeBP: 100, WageCapIDR: 12_000_000},
			"jht":       {EmployerBP: 370, EmployeeBP: 200},
			"jp":        {EmployerBP: 200, EmployeeBP: 100, WageCapIDR: 10_042_300},
			"jkk":       {EmployerBP: 24},
			"jkm":       {EmployerBP: 30},
		},
		TER: map[string][]TERBracket{
			"A": {
				{LowerIDR: 0, UpperIDR: 5_400_000, RateBP: 0},
				{LowerIDR: 5_400_001, UpperIDR: 5_650_000, RateBP: 25},
				{LowerIDR: 9_650_001, UpperIDR: 10_050_000, RateBP: 200},
				{LowerIDR: 10_050_001, UpperIDR: 10_350_000, RateBP: 225},
			},
		},
		TERCat:                map[string]string{"TK/0": "A", "K/0": "A", "K/1": "B"},
		Progressive:           []TERBracket{{0, 60_000_000, 500}, {60_000_001, 250_000_000, 1500}, {250_000_001, 500_000_000, 2500}, {500_000_001, 5_000_000_000, 3000}, {5_000_000_001, 0, 3500}},
		PTKP:                  map[string]int64{"TK/0": 54_000_000, "K/1": 63_000_000},
		BiayaJabatanBP:        500,
		BiayaJabatanMaxAnnual: 6_000_000,
	}
}

func TestCalcPPh21TER(t *testing.T) {
	p := fixtureParams()
	// AC-95-02: gross 10.000.000 in the 2% bracket -> 200.000
	tax, cat, rate, err := CalcPPh21TER(10_000_000, "TK/0", p)
	if err != nil || tax != 200_000 || cat != "A" || rate != 200 {
		t.Fatalf("got tax=%d cat=%s rate=%d err=%v", tax, cat, rate, err)
	}
	// Bruto above the last configured bracket: table incomplete -> error.
	var miss *MissingParamError
	if _, _, _, err := CalcPPh21TER(20_000_000, "TK/0", p); !errors.As(err, &miss) {
		t.Fatalf("want MissingParamError, got %v", err)
	}
	// Category B has no brackets configured.
	if _, _, _, err := CalcPPh21TER(8_000_000, "K/1", p); !errors.As(err, &miss) {
		t.Fatalf("want MissingParamError for empty category, got %v", err)
	}
	// Unknown PTKP status.
	if _, _, _, err := CalcPPh21TER(8_000_000, "K/9", p); !errors.As(err, &miss) {
		t.Fatalf("want MissingParamError for unknown PTKP, got %v", err)
	}
}

func TestCalcAllBPJS(t *testing.T) {
	p := fixtureParams()
	r, err := CalcAllBPJS(15_000_000, p)
	if err != nil {
		t.Fatal(err)
	}
	// kesehatan capped at 12jt: 120.000 / 480.000; JHT uncapped 300.000 /
	// 555.000; JP capped at 10.042.300: 100.423 / 200.846.
	if r.Employee["kesehatan"] != 120_000 || r.Employer["kesehatan"] != 480_000 {
		t.Errorf("kesehatan %+v", r)
	}
	if r.Employee["jp"] != 100_423 || r.Employer["jp"] != 200_846 {
		t.Errorf("jp %+v", r)
	}
	if r.EmployeeTotal != 120_000+300_000+100_423 {
		t.Errorf("employee total %d", r.EmployeeTotal)
	}
	if r.EmployerTaxed != 480_000+36_000+45_000 {
		t.Errorf("employer taxed %d", r.EmployerTaxed)
	}
	if r.Pension != 300_000+100_423 {
		t.Errorf("pension %d", r.Pension)
	}
	delete(p.BPJS, "jkm")
	var miss *MissingParamError
	if _, err := CalcAllBPJS(1, p); !errors.As(err, &miss) {
		t.Errorf("missing program must fail, got %v", err)
	}
}

func TestProgressive(t *testing.T) {
	layers := fixtureParams().Progressive
	cases := map[int64]int64{
		0:           0,
		60_000_000:  3_000_000,
		100_000_000: 3_000_000 + 6_000_000,
		300_000_000: 3_000_000 + 28_500_000 + 12_500_000,
	}
	for pkp, want := range cases {
		if got := Progressive(pkp, layers); got != want {
			t.Errorf("Progressive(%d) = %d, want %d", pkp, got, want)
		}
	}
}

// Donor case (TK/0, 12 months of Rp10.000.000 gross, 200.000 TER per month
// Jan–Nov): netto 120jt − 6jt BJ − pension; December = annual − withheld.
func TestCalcPPh21December(t *testing.T) {
	p := fixtureParams()
	in := AnnualInput{PTKP: "TK/0", GrossYearIDR: 120_000_000, PensionYearIDR: 3_600_000, WithheldPrevIDR: 2_200_000, Months: 12}
	r, err := CalcPPh21December(in, p)
	if err != nil {
		t.Fatal(err)
	}
	// netto 120 − 6 − 3,6 = 110,4jt; PKP 56,4jt; tax 5% = 2.820.000
	if r.BiayaJabatanIDR != 6_000_000 || r.PKPIDR != 56_400_000 || r.AnnualTaxIDR != 2_820_000 || r.ThisMonthIDR != 620_000 {
		t.Fatalf("got %+v", r)
	}
	// Mid-year joiner (6 months): biaya jabatan cap pro-rated to 3jt.
	r, _ = CalcPPh21December(AnnualInput{PTKP: "TK/0", GrossYearIDR: 70_000_000, Months: 6}, p)
	if r.BiayaJabatanIDR != 3_000_000 {
		t.Errorf("pro-rated BJ = %d", r.BiayaJabatanIDR)
	}
	// Over-withheld: December is 0 and the overpayment is reported.
	r, _ = CalcPPh21December(AnnualInput{PTKP: "TK/0", GrossYearIDR: 60_000_000, WithheldPrevIDR: 1_000_000, Months: 12}, p)
	if r.ThisMonthIDR != 0 || r.OverpaidIDR == 0 {
		t.Errorf("overpaid case %+v", r)
	}
}
