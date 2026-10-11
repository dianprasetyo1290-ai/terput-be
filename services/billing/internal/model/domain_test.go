package model

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Bill status
// ---------------------------------------------------------------------------

func TestDeriveBillStatus(t *testing.T) {
	cases := []struct {
		net, paid int64
		want      BillStatus
	}{
		{0, 0, BillUnpaid},
		{100_000, 0, BillUnpaid},
		{100_000, 40_000, BillPartial},
		{100_000, 100_000, BillPaid},
		{100_000, 150_000, BillPaid}, // overpayment still counts as paid
		{0, 1_000, BillPaid},
	}
	for _, tc := range cases {
		if got := DeriveBillStatus(tc.net, tc.paid); got != tc.want {
			t.Errorf("DeriveBillStatus(%d,%d)=%s want %s", tc.net, tc.paid, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Payment state machine
// ---------------------------------------------------------------------------

func TestValidatePaymentTransition(t *testing.T) {
	cases := []struct {
		from, to PaymentStatus
		wantErr  bool
	}{
		{PaymentPending, PaymentVerified, false},
		{PaymentPending, PaymentRejected, false},
		{PaymentPending, PaymentPending, true},
		{PaymentVerified, PaymentRejected, true}, // verified is immutable
		{PaymentVerified, PaymentPending, true},
		{PaymentRejected, PaymentVerified, true}, // rejected cannot be revived
	}
	for _, tc := range cases {
		err := ValidatePaymentTransition(tc.from, tc.to)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidatePaymentTransition(%s,%s) err=%v wantErr=%v", tc.from, tc.to, err, tc.wantErr)
		}
	}
}

func TestPaymentAction(t *testing.T) {
	if got, err := PaymentAction("verify"); err != nil || got != PaymentVerified {
		t.Fatalf("PaymentAction(verify)=(%s,%v)", got, err)
	}
	if got, err := PaymentAction("REJECTED"); err != nil || got != PaymentRejected {
		t.Fatalf("PaymentAction(REJECTED)=(%s,%v)", got, err)
	}
	if _, err := PaymentAction("nope"); err == nil {
		t.Fatal("unknown action must error")
	}
}

func TestValidateAllocations(t *testing.T) {
	good := []Allocation{{BillItemID: "a", AmountIDR: 400}, {AmountIDR: 600}}
	if err := ValidateAllocations(1000, good); err != nil {
		t.Fatalf("valid allocations rejected: %v", err)
	}
	if err := ValidateAllocations(1000, []Allocation{{AmountIDR: 400}}); err == nil {
		t.Fatal("allocation total below payment must be rejected")
	}
	if err := ValidateAllocations(1000, []Allocation{{AmountIDR: 1100}}); err == nil {
		t.Fatal("allocation total above payment must be rejected")
	}
	if err := ValidateAllocations(1000, []Allocation{{AmountIDR: 0}}); err == nil {
		t.Fatal("zero allocation must be rejected")
	}
	if err := ValidateAllocations(1000, nil); err == nil {
		t.Fatal("verifying with no allocations must be rejected")
	}
}

// ---------------------------------------------------------------------------
// Outstanding + reversal invariant
// ---------------------------------------------------------------------------

func TestOutstanding(t *testing.T) {
	got, err := Outstanding(1000, []int64{400, 100}, nil)
	if err != nil || got != 500 {
		t.Fatalf("Outstanding=(%d,%v) want (500,nil)", got, err)
	}
	// A reversal restores exactly what it reverses.
	got, err = Outstanding(1000, []int64{400}, []int64{150})
	if err != nil || got != 750 {
		t.Fatalf("Outstanding with partial reversal=(%d,%v) want (750,nil)", got, err)
	}
	// Full reversal restores the original bill.
	got, err = Outstanding(1000, []int64{400, 600}, []int64{1000})
	if err != nil || got != 1000 {
		t.Fatalf("Outstanding after full reversal=(%d,%v) want (1000,nil)", got, err)
	}
	// Overpayment is floored at zero, never negative debt.
	got, err = Outstanding(100, []int64{200}, nil)
	if err != nil || got != 0 {
		t.Fatalf("Outstanding overpaid=(%d,%v) want (0,nil)", got, err)
	}
}

func TestOutstandingReversalInvariant(t *testing.T) {
	// verify(300) then reverse(300) must leave the balance unchanged, for any
	// starting balance.
	for _, net := range []int64{0, 1, 500, 100_000} {
		before, err := Outstanding(net, nil, nil)
		if err != nil {
			t.Fatalf("baseline error: %v", err)
		}
		afterVerify, err := Outstanding(net, []int64{300}, nil)
		if err != nil {
			t.Fatalf("verify error: %v", err)
		}
		afterReversal, err := Outstanding(net, []int64{300}, []int64{300})
		if err != nil {
			t.Fatalf("reversal error: %v", err)
		}
		if afterReversal != before {
			t.Fatalf("net=%d: reversal did not restore balance: before=%d after=%d (after verify=%d)",
				net, before, afterReversal, afterVerify)
		}
	}
}

func TestOutstandingValidation(t *testing.T) {
	if _, err := Outstanding(-1, nil, nil); err == nil {
		t.Fatal("negative net must error")
	}
	if _, err := Outstanding(100, []int64{-1}, nil); err == nil {
		t.Fatal("negative verified amount must error")
	}
	if _, err := Outstanding(100, nil, []int64{-1}); err == nil {
		t.Fatal("negative reversal amount must error")
	}
}

// ---------------------------------------------------------------------------
// Discount engine — ordered + capped snapshots
// ---------------------------------------------------------------------------

func TestApplyDiscountsOrderingAndSnapshot(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := []DiscountCandidate{
		{ID: "c", Source: "yayasan", Label: "Beasiswa Yayasan", Kind: DiscountNominal, Value: 50_000, Priority: 20, CreatedAt: base},
		{ID: "a", Source: "sibling", Label: "Saudara kandung", Kind: DiscountPercent, Value: 1000, Priority: 10, CreatedAt: base},            // 10%
		{ID: "b", Source: "scholarship", Label: "Prestasi", Kind: DiscountPercent, Value: 500, Priority: 10, CreatedAt: base.Add(time.Hour)}, // 5%, later
	}
	net, steps, err := ApplyDiscounts(1_000_000, in)
	if err != nil {
		t.Fatalf("ApplyDiscounts error: %v", err)
	}
	// 10% of 1,000,000 = 100,000 -> 900,000; then 5% of 900,000 = 45,000 -> 855,000; then nominal 50,000 -> 805,000.
	if net != 805_000 {
		t.Fatalf("net=%d want 805000", net)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	for i, id := range []string{"a", "b", "c"} {
		if steps[i].ID != id {
			t.Fatalf("step %d id=%s want %s", i, steps[i].ID, id)
		}
	}
	if steps[0].BaseBefore != 1_000_000 || steps[0].Applied != 100_000 || steps[0].BaseAfter != 900_000 {
		t.Fatalf("step0 snapshot wrong: %+v", steps[0])
	}
	if steps[1].BaseBefore != 900_000 || steps[1].Applied != 45_000 || steps[1].BaseAfter != 855_000 {
		t.Fatalf("step1 snapshot wrong: %+v", steps[1])
	}
	if steps[2].BaseBefore != 855_000 || steps[2].Applied != 50_000 || steps[2].BaseAfter != 805_000 {
		t.Fatalf("step2 snapshot wrong: %+v", steps[2])
	}
	if steps[1].Label != "Prestasi" {
		t.Fatalf("label not preserved: %q", steps[1].Label)
	}
}

func TestApplyDiscountsCapAndFloor(t *testing.T) {
	in := []DiscountCandidate{
		{ID: "big", Source: "yayasan", Kind: DiscountNominal, Value: 2_000_000, Priority: 1},
		{ID: "capped", Source: "scholarship", Kind: DiscountPercent, Value: 10000, Priority: 2, Cap: 100_000},
	}
	net, steps, err := ApplyDiscounts(500_000, in)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if net != 0 {
		t.Fatalf("floor is zero, got %d", net)
	}
	if steps[0].Applied != 500_000 {
		t.Fatalf("discount must be clamped to the base, applied=%d", steps[0].Applied)
	}
	if steps[1].Requested != 0 || steps[1].Applied != 0 {
		t.Fatalf("second discount must request/apply 0 on a zero base, got %+v", steps[1])
	}
}

func TestApplyDiscountsCapLimitsRequest(t *testing.T) {
	// 50% of 1,000,000 = 500,000 requested, but the rule is capped at 200,000.
	net, steps, err := ApplyDiscounts(1_000_000, []DiscountCandidate{
		{ID: "s", Source: "scholarship", Kind: DiscountPercent, Value: 5000, Priority: 1, Cap: 200_000},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if net != 800_000 {
		t.Fatalf("net=%d want 800000", net)
	}
	if steps[0].Requested != 500_000 || steps[0].Applied != 200_000 {
		t.Fatalf("cap not applied: %+v", steps[0])
	}
}

func TestApplyDiscountsValidation(t *testing.T) {
	if _, _, err := ApplyDiscounts(-1, nil); err == nil {
		t.Fatal("negative base must error")
	}
	if _, _, err := ApplyDiscounts(100, []DiscountCandidate{{ID: "p", Kind: DiscountPercent, Value: 10001}}); err == nil {
		t.Fatal("over-100% percentage must error")
	}
	if _, _, err := ApplyDiscounts(100, []DiscountCandidate{{ID: "n", Kind: DiscountNominal, Value: -5}}); err == nil {
		t.Fatal("negative nominal discount must error")
	}
	if _, _, err := ApplyDiscounts(100, []DiscountCandidate{{ID: "k", Kind: "bogus", Value: 5}}); err == nil {
		t.Fatal("unknown kind must error")
	}
}

func TestApplyDiscountsStableTieBreak(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := []DiscountCandidate{
		{ID: "z", Kind: DiscountNominal, Value: 1, Priority: 1, CreatedAt: base},
		{ID: "a", Kind: DiscountNominal, Value: 1, Priority: 1, CreatedAt: base},
		{ID: "m", Kind: DiscountNominal, Value: 1, Priority: 1, CreatedAt: base},
	}
	_, steps, err := ApplyDiscounts(100, in)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if steps[0].ID != "a" || steps[1].ID != "m" || steps[2].ID != "z" {
		t.Fatalf("tie-break not deterministic: %s,%s,%s", steps[0].ID, steps[1].ID, steps[2].ID)
	}
}

// ---------------------------------------------------------------------------
// Idempotent generation
// ---------------------------------------------------------------------------

func TestPlanGenerationIdempotent(t *testing.T) {
	candidates := []BillCandidate{
		{Key: BillKey{StudentUserID: "s1", Year: 2026, Month: 7, SchemeID: "sch"}},
		{Key: BillKey{StudentUserID: "s2", Year: 2026, Month: 7, SchemeID: "sch"}},
	}
	first, err := PlanGeneration(nil, candidates)
	if err != nil {
		t.Fatalf("first plan error: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first plan len=%d want 2", len(first))
	}

	// Second run: the two bills now exist, so nothing new is planned.
	existing := []BillKey{first[0].Key, first[1].Key}
	second, err := PlanGeneration(existing, candidates)
	if err != nil {
		t.Fatalf("second plan error: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second (idempotent) plan len=%d want 0", len(second))
	}
}

func TestPlanGenerationDedupesBatchAndKeepsNew(t *testing.T) {
	candidates := []BillCandidate{
		{Key: BillKey{StudentUserID: "s1", Year: 2026, Month: 7, SchemeID: "sch"}},
		{Key: BillKey{StudentUserID: "s1", Year: 2026, Month: 7, SchemeID: "sch"}}, // duplicate in batch
		{Key: BillKey{StudentUserID: "s2", Year: 2026, Month: 7, SchemeID: "sch"}},
		{Key: BillKey{StudentUserID: "s3", Year: 2026, Month: 6, SchemeID: "sch"}}, // different period -> new
	}
	existing := []BillKey{{StudentUserID: "s2", Year: 2026, Month: 7, SchemeID: "sch"}}
	plan, err := PlanGeneration(existing, candidates)
	if err != nil {
		t.Fatalf("plan error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("plan len=%d want 2 (%+v)", len(plan), plan)
	}
	if plan[0].Key.StudentUserID != "s1" || plan[1].Key.StudentUserID != "s3" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestPlanGenerationValidation(t *testing.T) {
	if _, err := PlanGeneration(nil, []BillCandidate{{Key: BillKey{Year: 2026, Month: 7, SchemeID: "sch"}}}); err == nil {
		t.Fatal("missing student must error")
	}
	if _, err := PlanGeneration(nil, []BillCandidate{{Key: BillKey{StudentUserID: "s", Year: 2026, Month: 13, SchemeID: "sch"}}}); err == nil {
		t.Fatal("month out of range must error")
	}
}

func TestDueDateClampsToMonthEnd(t *testing.T) {
	if got := DueDate(2026, 2, 31); !got.Equal(time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Feb clamp=%v want 2026-02-28", got)
	}
	if got := DueDate(2028, 2, 31); !got.Equal(time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("leap Feb clamp=%v want 2028-02-29", got)
	}
	if got := DueDate(2026, 7, 10); !got.Equal(time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("normal due date=%v want 2026-07-10", got)
	}
}

func TestTotalBase(t *testing.T) {
	total, err := TotalBase([]BillItem{
		{Name: "SPP Juli", AmountIDR: 500_000},
		{Name: "Uang gedung", AmountIDR: 2_000_000},
	})
	if err != nil || total != 2_500_000 {
		t.Fatalf("TotalBase=(%d,%v) want (2500000,nil)", total, err)
	}
	if _, err := TotalBase([]BillItem{{Name: "bad", AmountIDR: -1}}); err == nil {
		t.Fatal("negative item must error")
	}
}

func TestSpreadRefund(t *testing.T) {
	src := []RefundSource{{"p2", 300}, {"p1", 0}, {"p0", 1000}}
	got, err := SpreadRefund(800, src)
	if err != nil || len(got) != 2 || got[0] != (RefundPart{"p2", 300}) || got[1] != (RefundPart{"p0", 500}) {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := SpreadRefund(1301, src); err == nil {
		t.Fatal("over-refund must fail")
	}
	if _, err := SpreadRefund(0, src); err == nil {
		t.Fatal("zero refund must fail")
	}
}
