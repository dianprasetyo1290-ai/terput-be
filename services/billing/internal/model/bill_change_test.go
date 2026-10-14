package model

import "testing"

func TestValidateBillChange(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	cases := []struct {
		name, kind, reason, status string
		newNet                     *int64
		ok                         bool
	}{
		{"cancel", BillChangeCancel, "salah siswa", "unpaid", nil, true},
		{"cancel needs reason", BillChangeCancel, " ", "unpaid", nil, false},
		{"cancel with amount", BillChangeCancel, "x", "unpaid", n(5), false},
		{"already void", BillChangeCancel, "x", "void", nil, false},
		{"adjust", BillChangeAdjust, "koreksi", "partial", n(400), true},
		{"adjust to zero", BillChangeAdjust, "koreksi", "partial", n(0), false},
		{"adjust missing amount", BillChangeAdjust, "koreksi", "unpaid", nil, false},
		{"adjust negative", BillChangeAdjust, "koreksi", "unpaid", n(-1), false},
		{"adjust unchanged", BillChangeAdjust, "koreksi", "unpaid", n(1000), false},
		{"unknown kind", "delete", "x", "unpaid", nil, false},
	}
	for _, tc := range cases {
		if err := ValidateBillChange(tc.kind, tc.newNet, tc.reason, tc.status, 1000); (err == nil) != tc.ok {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestPlanBillChange(t *testing.T) {
	cases := []struct {
		name              string
		kind              string
		net, paid, newNet int64
		netAfter, delta   int64
		deposit           int64
		status            BillStatus
	}{
		{"cancel unpaid", BillChangeCancel, 1000, 0, 0, 0, -1000, 0, BillVoid},
		{"cancel partly paid moves money to deposit", BillChangeCancel, 1000, 400, 0, 0, -1000, 400, BillVoid},
		{"lower, still owing", BillChangeAdjust, 1000, 300, 800, 800, -200, 0, BillPartial},
		{"lower below paid", BillChangeAdjust, 1000, 900, 600, 600, -400, 300, BillPaid},
		{"raise a paid bill", BillChangeAdjust, 1000, 1000, 1200, 1200, 200, 0, BillPartial},
	}
	for _, tc := range cases {
		p, err := PlanBillChange(tc.kind, tc.net, tc.paid, tc.newNet)
		if err != nil || p.NetAfterIDR != tc.netAfter || p.DeltaIDR != tc.delta || p.DepositIDR != tc.deposit || p.Status != tc.status {
			t.Errorf("%s: %+v %v", tc.name, p, err)
		}
	}
}

func TestSplitSigned(t *testing.T) {
	if got := SplitSigned(-100, []int64{75, 25}); got[0] != -75 || got[1] != -25 {
		t.Fatalf("negative %v", got)
	}
	if got := SplitSigned(10, []int64{1, 1}); got[0] != 5 || got[1] != 5 {
		t.Fatalf("positive %v", got)
	}
}

func TestApprovalOutcome(t *testing.T) {
	cases := []struct {
		name, current, decision, by string
		approved, changed, err      bool
	}{
		{"approve", BillChangePending, "approved", "checker", true, true, false},
		{"reject", BillChangePending, "rejected", "checker", false, true, false},
		{"replay", BillChangeApplied, "approved", "checker", false, false, false},
		{"self approval", BillChangePending, "approved", "maker", false, false, true},
		{"bad decision", BillChangePending, "maybe", "checker", false, false, true},
		{"no approver", BillChangePending, "approved", "", false, false, true},
	}
	for _, tc := range cases {
		a, c, err := ApprovalOutcome(tc.current, BillChangePending, tc.decision, tc.by, "maker")
		if a != tc.approved || c != tc.changed || (err != nil) != tc.err {
			t.Errorf("%s: %v %v %v", tc.name, a, c, err)
		}
	}
}
