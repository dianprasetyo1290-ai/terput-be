package calc

import (
	"testing"
)

func TestCalcBPJS(t *testing.T) {
	rule := BPJSRule{
		EmployerBP: 400, // 4%
		EmployeeBP: 100, // 1%
		WageCapIDR: 12_000_000,
	}

	// AC-95-01: base 5.000.000 -> employee 50.000, employer 200.000
	emp, empyr := CalcBPJS(5_000_000, rule)
	if emp != 50_000 || empyr != 200_000 {
		t.Errorf("AC-95-01 failed: got emp=%d, empyr=%d", emp, empyr)
	}

	// cap test: base 15.000.000 -> base becomes 12.000.000 -> emp 120.000, empyr 480.000
	empCap, empyrCap := CalcBPJS(15_000_000, rule)
	if empCap != 120_000 || empyrCap != 480_000 {
		t.Errorf("cap test failed: got emp=%d, empyr=%d", empCap, empyrCap)
	}
}

func TestCalcPPh21Monthly(t *testing.T) {
	// AC-95-02: gross 10.000.000, bracket 200bp -> 200.000
	tax := CalcPPh21Monthly(10_000_000, 200)
	if tax != 200_000 {
		t.Errorf("AC-95-02 failed: expected 200000, got %d", tax)
	}
}

func TestCalcPerHour(t *testing.T) {
	// AC-94-01: per_hour tarif 50.000 * 40 jam -> 2.000.000
	earning := CalcPerHour(50_000, 40)
	if earning != 2_000_000 {
		t.Errorf("AC-94-01 failed: expected 2000000, got %d", earning)
	}
}

func TestVariance(t *testing.T) {
	// AC-95-03: prev 6.000.000, cur 7.200.000 -> 2000 bp (20%), flagged
	bp, flagged := Variance(6_000_000, 7_200_000, 1000)
	if bp != 2000 || !flagged {
		t.Errorf("AC-95-03 failed: expected 2000 bp and flagged=true, got bp=%d, flagged=%v", bp, flagged)
	}

	// Within 5% threshold (not flagged)
	bpNormal, flaggedNormal := Variance(5_000_000, 5_200_000, 1000)
	if bpNormal != 400 || flaggedNormal {
		t.Errorf("expected 400 bp and flagged=false, got bp=%d, flagged=%v", bpNormal, flaggedNormal)
	}
}

func TestBuildAccrualJournal(t *testing.T) {
	// AC-98-01: beban 150.000.000, PPh 5.000.000, BPJS 3.000.000 (misal employer 2.000.000, total 5.000.000)
	// If grossExpense=150.000.000, bpjsEmployer=0, pph=5.000.000, bpjsPayable=3.000.000
	res := BuildAccrualJournal(150_000_000, 0, 5_000_000, 3_000_000, 0)
	if res.SalaryPayableIDR != 142_000_000 {
		t.Errorf("AC-98-01 failed: expected salaryPayable 142000000, got %d", res.SalaryPayableIDR)
	}
	if !res.IsBalanced {
		t.Errorf("AC-98-01 failed: journal is not balanced (debit=%d, credit=%d)", res.TotalDebit, res.TotalCredit)
	}
}
