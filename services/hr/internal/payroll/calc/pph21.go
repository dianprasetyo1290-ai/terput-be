package calc

import (
	"fmt"
	"sort"
)

// Params is every statutory number one payroll period needs, loaded from the
// effective-dated statutory_* tables (T3 §3: rates are data, never code).
type Params struct {
	BPJS                  map[string]BPJSRule     `json:"bpjs"` // program -> rule
	TER                   map[string][]TERBracket `json:"ter"`  // category -> brackets
	TERCat                map[string]string       `json:"ter_categories"`
	Progressive           []TERBracket            `json:"progressive"`
	PTKP                  map[string]int64        `json:"ptkp"`
	BiayaJabatanBP        int                     `json:"biaya_jabatan_bp"`
	BiayaJabatanMaxAnnual int64                   `json:"biaya_jabatan_max_annual_idr"`
}

// BPJS programs. Employees contribute to kesehatan, JHT and JP; JKK and JKM
// are employer-only. Employer premiums for kesehatan, JKK and JKM are part
// of the PPh 21 gross (PMK 168/2023); JHT/JP employer shares are not.
var (
	BPJSPrograms         = []string{"kesehatan", "jht", "jp", "jkk", "jkm"}
	EmployerTaxedProgram = map[string]bool{"kesehatan": true, "jkk": true, "jkm": true}
	PensionPrograms      = map[string]bool{"jht": true, "jp": true}
)

// MissingParamError is a statutory parameter the school has not configured.
// The run must stop with 422 rather than guess (T3 §2).
type MissingParamError struct{ What string }

func (e *MissingParamError) Error() string {
	return fmt.Sprintf("parameter %s belum dikonfigurasi", e.What)
}

// BPJSResult is one employee's contributions per program.
type BPJSResult struct {
	Employee      map[string]int64 `json:"employee"`
	Employer      map[string]int64 `json:"employer"`
	EmployeeTotal int64            `json:"employee_total"`
	EmployerTotal int64            `json:"employer_total"`
	EmployerTaxed int64            `json:"employer_taxed"` // part of PPh 21 gross
	Pension       int64            `json:"pension"`        // employee JHT + JP, deductible yearly
}

// CalcAllBPJS applies every configured program to the BPJS base.
func CalcAllBPJS(baseIDR int64, p Params) (BPJSResult, error) {
	res := BPJSResult{Employee: map[string]int64{}, Employer: map[string]int64{}}
	for _, prog := range BPJSPrograms {
		rule, ok := p.BPJS[prog]
		if !ok {
			return res, &MissingParamError{What: "BPJS " + prog}
		}
		emp, er := CalcBPJS(baseIDR, rule)
		res.Employee[prog] = emp
		res.Employer[prog] = er
		res.EmployeeTotal += emp
		res.EmployerTotal += er
		if EmployerTaxedProgram[prog] {
			res.EmployerTaxed += er
		}
		if PensionPrograms[prog] {
			res.Pension += emp
		}
	}
	return res, nil
}

// TERCategory maps a PTKP status (TK/0 … K/3) to its TER category.
func TERCategory(ptkp string, p Params) (string, error) {
	cat, ok := p.TERCat[ptkp]
	if !ok {
		return "", &MissingParamError{What: "kategori TER untuk status PTKP " + ptkp}
	}
	return cat, nil
}

// TERRate finds the monthly effective rate for a gross amount. A gross above
// the last configured bracket means the table is incomplete: error, not 0%.
func TERRate(grossIDR int64, category string, p Params) (int, error) {
	brackets := p.TER[category]
	if len(brackets) == 0 {
		return 0, &MissingParamError{What: "tabel TER kategori " + category}
	}
	for _, b := range brackets {
		if grossIDR >= b.LowerIDR && (b.UpperIDR == 0 || grossIDR <= b.UpperIDR) {
			return b.RateBP, nil
		}
	}
	return 0, &MissingParamError{What: fmt.Sprintf("tabel TER kategori %s untuk bruto Rp%d", category, grossIDR)}
}

// CalcPPh21TER is the January–November (and non-final month) withholding:
// TER rate of the category × monthly gross, floored to the rupiah.
func CalcPPh21TER(grossIDR int64, ptkp string, p Params) (tax int64, category string, rateBP int, err error) {
	category, err = TERCategory(ptkp, p)
	if err != nil {
		return 0, "", 0, err
	}
	rateBP, err = TERRate(grossIDR, category, p)
	if err != nil {
		return 0, category, 0, err
	}
	return CalcPPh21Monthly(grossIDR, rateBP), category, rateBP, nil
}

// Progressive applies the Pasal 17 layers to a yearly PKP.
func Progressive(pkpIDR int64, layers []TERBracket) int64 {
	sorted := append([]TERBracket(nil), layers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].LowerIDR < sorted[j].LowerIDR })
	var tax int64
	for _, l := range sorted {
		lower := l.LowerIDR
		if lower > 0 {
			lower-- // layers are stored as 0–60jt, 60jt+1–250jt, …
		}
		if pkpIDR <= lower {
			break
		}
		top := pkpIDR
		if l.UpperIDR > 0 && top > l.UpperIDR {
			top = l.UpperIDR
		}
		tax += (top - lower) * int64(l.RateBP) / 10000
	}
	return tax
}

// AnnualInput is one employee's year to date at the last period of the tax
// year (December, or the exit month).
type AnnualInput struct {
	PTKP            string `json:"ptkp"`
	GrossYearIDR    int64  `json:"gross_year_idr"`    // PPh 21 gross Jan..last, incl. this month
	PensionYearIDR  int64  `json:"pension_year_idr"`  // employee JHT + JP Jan..last
	WithheldPrevIDR int64  `json:"withheld_prev_idr"` // PPh 21 withheld before this month
	Months          int    `json:"months"`
}

// AnnualResult explains the December reconciliation on the payslip.
type AnnualResult struct {
	BiayaJabatanIDR int64 `json:"biaya_jabatan_idr"`
	NettoIDR        int64 `json:"netto_idr"`
	PTKPIDR         int64 `json:"ptkp_idr"`
	PKPIDR          int64 `json:"pkp_idr"`
	AnnualTaxIDR    int64 `json:"annual_tax_idr"`
	ThisMonthIDR    int64 `json:"this_month_idr"`
	OverpaidIDR     int64 `json:"overpaid_idr,omitempty"`
}

// CalcPPh21December: progressive tax on the year's PKP minus what TER
// already withheld. An overpayment is reported, never withheld negative.
func CalcPPh21December(in AnnualInput, p Params) (AnnualResult, error) {
	var r AnnualResult
	ptkp, ok := p.PTKP[in.PTKP]
	if !ok {
		return r, &MissingParamError{What: "PTKP " + in.PTKP}
	}
	if len(p.Progressive) == 0 {
		return r, &MissingParamError{What: "tarif progresif Pasal 17"}
	}
	if p.BiayaJabatanBP <= 0 || p.BiayaJabatanMaxAnnual <= 0 {
		return r, &MissingParamError{What: "biaya jabatan"}
	}
	r.BiayaJabatanIDR = in.GrossYearIDR * int64(p.BiayaJabatanBP) / 10000
	// The yearly cap is pro-rated by months worked (PMK 168/2023 Pasal 9).
	maxBJ := p.BiayaJabatanMaxAnnual
	if in.Months > 0 && in.Months < 12 {
		maxBJ = p.BiayaJabatanMaxAnnual * int64(in.Months) / 12
	}
	if r.BiayaJabatanIDR > maxBJ {
		r.BiayaJabatanIDR = maxBJ
	}
	r.NettoIDR = in.GrossYearIDR - r.BiayaJabatanIDR - in.PensionYearIDR
	r.PTKPIDR = ptkp
	pkp := r.NettoIDR - ptkp
	if pkp < 0 {
		pkp = 0
	}
	r.PKPIDR = pkp / 1000 * 1000 // rounded down to the thousand
	r.AnnualTaxIDR = Progressive(r.PKPIDR, p.Progressive)
	r.ThisMonthIDR = r.AnnualTaxIDR - in.WithheldPrevIDR
	if r.ThisMonthIDR < 0 {
		r.OverpaidIDR = -r.ThisMonthIDR
		r.ThisMonthIDR = 0
	}
	return r, nil
}
