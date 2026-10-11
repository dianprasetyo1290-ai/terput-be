package calc

// BPJSRule holds parameters for BPJS calculations
type BPJSRule struct {
	EmployerBP int   `json:"employer_bp"`
	EmployeeBP int   `json:"employee_bp"`
	WageCapIDR int64 `json:"wage_cap_idr"`
}

// CalcBPJS computes employee and employer contribution with wage capping.
// Vectors AC-95-01 and cap test:
// base 5.000.000, emp 100bp, empyr 400bp, cap 12.000.000 -> emp 50.000, empyr 200.000
// base 15.000.000, cap 12.000.000 -> base becomes 12.000.000 -> emp 120.000, empyr 480.000
func CalcBPJS(baseIDR int64, r BPJSRule) (employee, employer int64) {
	effectiveBase := baseIDR
	if r.WageCapIDR > 0 && effectiveBase > r.WageCapIDR {
		effectiveBase = r.WageCapIDR
	}
	employee = (effectiveBase * int64(r.EmployeeBP)) / 10000
	employer = (effectiveBase * int64(r.EmployerBP)) / 10000
	return employee, employer
}

// TERBracket defines income bracket and tax rate in bp
type TERBracket struct {
	LowerIDR int64 `json:"lower_idr"`
	UpperIDR int64 `json:"upper_idr"` // 0 means unbounded
	RateBP   int   `json:"rate_bp"`
}

// CalcPPh21Monthly calculates monthly PPh 21 using TER bracket rate.
// Vector AC-95-02: gross 10.000.000, bracket 200bp -> 200.000
func CalcPPh21Monthly(grossIDR int64, rateBP int) int64 {
	return (grossIDR * int64(rateBP)) / 10000
}

// CalcPerHour calculates earning for hourly rate components.
// Vector AC-94-01: tarif 50.000 * 40 jam -> 2.000.000
func CalcPerHour(ratePerUnitIDR int64, hours int) int64 {
	return ratePerUnitIDR * int64(hours)
}

// Variance calculates difference in basis points and flags if exceeding threshold.
// Vector AC-95-03: prev 6.000.000, cur 7.200.000 -> 2000 bp, flagged
func Variance(prev, cur int64, thresholdBP int) (bp int, flagged bool) {
	if prev <= 0 {
		return 0, false
	}
	diff := cur - prev
	// bp = (diff * 10000) / prev
	bp = int((diff * 10000) / prev)
	absBP := bp
	if absBP < 0 {
		absBP = -absBP
	}
	if thresholdBP <= 0 {
		thresholdBP = 1000 // default 10%
	}
	flagged = absBP >= thresholdBP
	return bp, flagged
}

// AccrualJournalResult represents balanced journal entries
type AccrualJournalResult struct {
	GrossExpenseIDR        int64 `json:"gross_expense_idr"`
	BPJSEmployerExpenseIDR int64 `json:"bpjs_employer_expense_idr"`
	SalaryPayableIDR       int64 `json:"salary_payable_idr"`
	PPh21PayableIDR        int64 `json:"pph21_payable_idr"`
	BPJSPayableIDR         int64 `json:"bpjs_payable_idr"`
	OtherDeductionsIDR     int64 `json:"other_deductions_idr"`
	TotalDebit             int64 `json:"total_debit"`
	TotalCredit            int64 `json:"total_credit"`
	IsBalanced             bool  `json:"is_balanced"`
}

// BuildAccrualJournal balances accrual journal entries.
// Vector AC-98-01: beban 150.000.000, PPh 5.000.000, BPJS 3.000.000 -> utang gaji 142.000.000, debit=credit
func BuildAccrualJournal(grossExpenseIDR, bpjsEmployerExpenseIDR, pph21PayableIDR, bpjsPayableIDR, otherDeductionsIDR int64) AccrualJournalResult {
	// Total employee deductions = pph21 + employee bpjs + other
	// Net salary payable = gross - pph21 - bpjs_employee - other
	// Total Debit = Gross expense + BPJS employer expense
	// Total Credit = Salary payable + PPh21 payable + BPJS total payable (employer+employee) + other deductions
	salaryPayable := grossExpenseIDR - (pph21PayableIDR + (bpjsPayableIDR - bpjsEmployerExpenseIDR) + otherDeductionsIDR)

	totalDebit := grossExpenseIDR + bpjsEmployerExpenseIDR
	totalCredit := salaryPayable + pph21PayableIDR + bpjsPayableIDR + otherDeductionsIDR

	return AccrualJournalResult{
		GrossExpenseIDR:        grossExpenseIDR,
		BPJSEmployerExpenseIDR: bpjsEmployerExpenseIDR,
		SalaryPayableIDR:       salaryPayable,
		PPh21PayableIDR:        pph21PayableIDR,
		BPJSPayableIDR:         bpjsPayableIDR,
		OtherDeductionsIDR:     otherDeductionsIDR,
		TotalDebit:             totalDebit,
		TotalCredit:            totalCredit,
		IsBalanced:             totalDebit == totalCredit,
	}
}
