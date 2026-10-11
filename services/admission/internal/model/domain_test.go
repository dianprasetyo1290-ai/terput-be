package model

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Application state machine
// ---------------------------------------------------------------------------

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to ApplicationStatus
		want     bool
	}{
		{StatusDraft, StatusSubmitted, true},
		{StatusDraft, StatusRanked, false},
		{StatusSubmitted, StatusDocumentReview, true},
		{StatusSubmitted, StatusOffered, false},
		{StatusDocumentReview, StatusEligibleForTest, true},
		{StatusEligibleForTest, StatusTested, true},
		{StatusTested, StatusRanked, true},
		{StatusRanked, StatusOffered, true},
		{StatusRanked, StatusWaitlisted, true},
		{StatusWaitlisted, StatusOffered, true},
		{StatusOffered, StatusReregistration, true},
		{StatusReregistration, StatusConverted, true},
		{StatusConverted, StatusWithdrawn, false}, // terminal
		{StatusWithdrawn, StatusDraft, false},     // terminal
		{StatusOffered, StatusTested, false},      // no going back
	}
	for _, tc := range cases {
		if got := CanTransition(tc.from, tc.to); got != tc.want {
			t.Errorf("CanTransition(%s,%s)=%v want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestValidateTransitionReason(t *testing.T) {
	if err := ValidateTransition(StatusDraft, StatusSubmitted, ""); err != nil {
		t.Fatalf("draft->submitted without reason should be allowed: %v", err)
	}
	if err := ValidateTransition(StatusSubmitted, StatusRejected, ""); err == nil {
		t.Fatal("reject without reason must be rejected")
	}
	if err := ValidateTransition(StatusSubmitted, StatusRejected, "berkas tidak lengkap"); err != nil {
		t.Fatalf("reject with reason should be allowed: %v", err)
	}
	if err := ValidateTransition(StatusRanked, StatusSubmitted, "x"); err == nil {
		t.Fatal("ranked->submitted is not a legal transition")
	}
	if err := ValidateTransition(StatusDraft, StatusDraft, ""); err == nil {
		t.Fatal("no-op transition must be rejected")
	}
}

func TestGuardianCanTransition(t *testing.T) {
	if !GuardianCanTransition(StatusDraft, StatusSubmitted) {
		t.Fatal("guardian should be able to submit a draft")
	}
	if GuardianCanTransition(StatusRanked, StatusOffered) {
		t.Fatal("guardian must not publish an offer")
	}
	if GuardianCanTransition(StatusTested, StatusRanked) {
		t.Fatal("guardian must not run ranking")
	}
}

// ---------------------------------------------------------------------------
// Payment state machine and money invariants
// ---------------------------------------------------------------------------

func TestValidatePaymentTransition(t *testing.T) {
	cases := []struct {
		from, to PaymentStatus
		wantErr  bool
	}{
		{PaymentSubmitted, PaymentPendingReview, false},
		{PaymentSubmitted, PaymentVerified, false},
		{PaymentPendingReview, PaymentVerified, false},
		{PaymentPendingReview, PaymentRejected, false},
		{PaymentVerified, PaymentReversed, false},
		{PaymentVerified, PaymentRejected, true}, // verified is immutable
		{PaymentReversed, PaymentVerified, true}, // no un-reversing
		{PaymentRejected, PaymentVerified, true}, // rejected cannot be revived
		{PaymentPendingReview, PaymentPendingReview, true},
	}
	for _, tc := range cases {
		err := ValidatePaymentTransition(tc.from, tc.to)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidatePaymentTransition(%s,%s) err=%v wantErr=%v", tc.from, tc.to, err, tc.wantErr)
		}
	}
}

func TestValidateAllocations(t *testing.T) {
	good := []Allocation{{"a", 400}, {"b", 600}}
	if err := ValidateAllocations(1000, good); err != nil {
		t.Fatalf("valid allocations rejected: %v", err)
	}
	if err := ValidateAllocations(1000, []Allocation{{"a", 400}}); err == nil {
		t.Fatal("allocation total below payment must be rejected")
	}
	if err := ValidateAllocations(1000, []Allocation{{"a", 1100}}); err == nil {
		t.Fatal("allocation total above payment must be rejected")
	}
	if err := ValidateAllocations(1000, []Allocation{{"a", 0}}); err == nil {
		t.Fatal("zero allocation must be rejected")
	}
	if err := ValidateAllocations(1000, nil); err == nil {
		t.Fatal("verifying with no allocations must be rejected")
	}
}

func TestOutstanding(t *testing.T) {
	got, err := Outstanding(1000, []int64{400, 100}, nil)
	if err != nil || got != 500 {
		t.Fatalf("Outstanding= (%d,%v) want (500,nil)", got, err)
	}
	got, err = Outstanding(1000, []int64{400}, []int64{150})
	if err != nil || got != 750 {
		t.Fatalf("Outstanding with reversal= (%d,%v) want (750,nil)", got, err)
	}
	if _, err := Outstanding(100, []int64{200}, nil); err == nil {
		t.Fatal("allocations exceeding the charge must error")
	}
	if _, err := Outstanding(100, []int64{-1}, nil); err == nil {
		t.Fatal("negative allocation must error")
	}
}

// ---------------------------------------------------------------------------
// Discount engine
// ---------------------------------------------------------------------------

func TestApplyDiscountsOrderingAndSnapshot(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := []DiscountCandidate{
		{ID: "c", Source: "manual", Kind: DiscountFixed, Value: 50_000, Priority: 20, CreatedAt: base},
		{ID: "a", Source: "auto", Kind: DiscountPercent, Value: 1000, Priority: 10, CreatedAt: base},               // 10%
		{ID: "b", Source: "auto", Kind: DiscountPercent, Value: 500, Priority: 10, CreatedAt: base.Add(time.Hour)}, // 5%, later
	}
	net, steps, err := ApplyDiscounts(1_000_000, in)
	if err != nil {
		t.Fatalf("ApplyDiscounts error: %v", err)
	}
	// 10% of 1,000,000 = 100,000 -> 900,000; then 5% of 900,000 = 45,000 -> 855,000; then fixed 50,000 -> 805,000.
	if net != 805_000 {
		t.Fatalf("net=%d want 805000", net)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	wantOrder := []string{"a", "b", "c"}
	for i, id := range wantOrder {
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
	if steps[2].BaseAfter != 805_000 {
		t.Fatalf("final base_after=%d want 805000", steps[2].BaseAfter)
	}
}

func TestApplyDiscountsCapAndFloor(t *testing.T) {
	in := []DiscountCandidate{
		{ID: "big", Source: "auto", Kind: DiscountFixed, Value: 2_000_000, Priority: 1},
		{ID: "capped", Source: "auto", Kind: DiscountPercent, Value: 10000, Priority: 2, Cap: 100_000},
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

func TestApplyDiscountsValidation(t *testing.T) {
	if _, _, err := ApplyDiscounts(-1, nil); err == nil {
		t.Fatal("negative base must error")
	}
	if _, _, err := ApplyDiscounts(100, []DiscountCandidate{{ID: "p", Kind: DiscountPercent, Value: 10001}}); err == nil {
		t.Fatal("over-100% percentage must error")
	}
	if _, _, err := ApplyDiscounts(100, []DiscountCandidate{{ID: "n", Kind: DiscountFixed, Value: -5}}); err == nil {
		t.Fatal("negative fixed discount must error")
	}
	if _, _, err := ApplyDiscounts(100, []DiscountCandidate{{ID: "k", Kind: "bogus", Value: 5}}); err == nil {
		t.Fatal("unknown kind must error")
	}
}

func TestApplyDiscountsStableTieBreak(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Same priority and timestamp -> id decides, regardless of input order.
	in := []DiscountCandidate{
		{ID: "z", Kind: DiscountFixed, Value: 1, Priority: 1, CreatedAt: base},
		{ID: "a", Kind: DiscountFixed, Value: 1, Priority: 1, CreatedAt: base},
		{ID: "m", Kind: DiscountFixed, Value: 1, Priority: 1, CreatedAt: base},
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
// Ranking
// ---------------------------------------------------------------------------

func rankingFormula() Formula {
	return Formula{
		Weights:      map[string]int64{"cbt": 6000, "raport": 4000},
		Minimums:     map[string]int64{"cbt": 40, "raport": 0},
		MinimumTotal: 0,
		TieBreak:     []string{"cbt"},
	}
}

func TestRankWeightedAndRecommended(t *testing.T) {
	inputs := []RankInput{
		{ApplicationID: "a", RegistrationNumber: "003", SubmittedAt: time.Unix(100, 0), Scores: map[string]int64{"cbt": 80, "raport": 70}},
		{ApplicationID: "b", RegistrationNumber: "002", SubmittedAt: time.Unix(100, 0), Scores: map[string]int64{"cbt": 90, "raport": 60}},
		{ApplicationID: "c", RegistrationNumber: "001", SubmittedAt: time.Unix(100, 0), Scores: map[string]int64{"cbt": 90, "raport": 90}},
	}
	got, err := Rank(inputs, rankingFormula(), 2)
	if err != nil {
		t.Fatalf("Rank error: %v", err)
	}
	// c = 90*0.6+90*0.4 = 90; b = 90*0.6+60*0.4 = 78; a = 80*0.6+70*0.4 = 76.
	if got[0].ApplicationID != "c" || got[0].Total != 90 {
		t.Fatalf("rank1 = %+v want c/90", got[0])
	}
	if got[1].ApplicationID != "b" || got[1].Total != 78 {
		t.Fatalf("rank2 = %+v want b/78", got[1])
	}
	if got[2].ApplicationID != "a" || got[2].Total != 76 {
		t.Fatalf("rank3 = %+v want a/76", got[2])
	}
	if got[0].Recommendation != "accepted" || got[1].Recommendation != "accepted" {
		t.Fatalf("top-2 within quota must be accepted: %+v", got)
	}
	if got[2].Recommendation != "waitlisted" {
		t.Fatalf("rank3 beyond quota must be waitlisted, got %s", got[2].Recommendation)
	}
	for i, e := range got {
		if e.Rank != i+1 {
			t.Fatalf("rank at %d = %d", i, e.Rank)
		}
	}
}

func TestRankTieBreakDeterministic(t *testing.T) {
	// Equal totals -> tie-break on cbt, then earlier submission, then reg number.
	inputs := []RankInput{
		{ApplicationID: "a", RegistrationNumber: "999", SubmittedAt: time.Unix(400, 0), Scores: map[string]int64{"cbt": 80, "raport": 80}},
		{ApplicationID: "b", RegistrationNumber: "111", SubmittedAt: time.Unix(300, 0), Scores: map[string]int64{"cbt": 80, "raport": 80}},
		{ApplicationID: "c", RegistrationNumber: "222", SubmittedAt: time.Unix(100, 0), Scores: map[string]int64{"cbt": 80, "raport": 80}},
		{ApplicationID: "d", RegistrationNumber: "555", SubmittedAt: time.Unix(300, 0), Scores: map[string]int64{"cbt": 80, "raport": 80}},
	}
	first, err := Rank(inputs, rankingFormula(), 10)
	if err != nil {
		t.Fatalf("Rank error: %v", err)
	}
	// c (submitted earliest) wins; then among unix(300) b(111) < d(555).
	want := []string{"c", "b", "d", "a"}
	for i, id := range want {
		if first[i].ApplicationID != id {
			t.Fatalf("position %d = %s want %s", i, first[i].ApplicationID, id)
		}
	}
	// Running again must produce identical output (immutability / determinism).
	second, err := Rank(inputs, rankingFormula(), 10)
	if err != nil {
		t.Fatalf("Rank error: %v", err)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("non-deterministic ranking at %d: %+v vs %+v", i, first[i], second[i])
		}
	}
}

func TestRankDoesNotMutateFormula(t *testing.T) {
	// The formula declares a minimum only for "cbt"; ranking must not inject a
	// "raport" entry into that map as a side effect.
	formula := Formula{
		Weights:  map[string]int64{"cbt": 6000, "raport": 4000},
		Minimums: map[string]int64{"cbt": 40},
		TieBreak: []string{"cbt"},
	}
	inputs := []RankInput{{ApplicationID: "a", RegistrationNumber: "1", Scores: map[string]int64{"cbt": 50, "raport": 50}}}
	if _, err := Rank(inputs, formula, 1); err != nil {
		t.Fatalf("Rank error: %v", err)
	}
	if len(formula.Minimums) != 1 {
		t.Fatalf("Rank mutated the formula minimums map: %v", formula.Minimums)
	}
	if _, ok := formula.Minimums["raport"]; ok {
		t.Fatal("formula minimums must not gain keys during ranking")
	}
}

func TestRankThresholds(t *testing.T) {
	// Per-component minimum: cbt < 40 disqualifies.
	got, err := Rank([]RankInput{{ApplicationID: "a", Scores: map[string]int64{"cbt": 39, "raport": 100}}}, rankingFormula(), 1)
	if err != nil {
		t.Fatalf("Rank error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("applicant below the component minimum must be excluded, got %+v", got)
	}
	// Overall minimum total.
	f := rankingFormula()
	f.MinimumTotal = 80
	got, err = Rank([]RankInput{
		{ApplicationID: "high", Scores: map[string]int64{"cbt": 90, "raport": 90}},
		{ApplicationID: "low", Scores: map[string]int64{"cbt": 60, "raport": 50}}, // 56
	}, f, 1)
	if err != nil {
		t.Fatalf("Rank error: %v", err)
	}
	if len(got) != 1 || got[0].ApplicationID != "high" {
		t.Fatalf("minimum_total filter failed: %+v", got)
	}
}

func TestRankValidatesWeights(t *testing.T) {
	if _, err := Rank(nil, Formula{Weights: map[string]int64{"cbt": 5000}}, 1); err == nil {
		t.Fatal("weights not totalling 100% must error")
	}
	if _, err := Rank(nil, Formula{Weights: map[string]int64{"cbt": -10000}}, 1); err == nil {
		t.Fatal("negative weight must error")
	}
}

func TestFormulaSpecCompile(t *testing.T) {
	spec := FormulaSpec{
		Name: "Seleksi 2026",
		Components: []Component{
			{Code: "cbt", Name: "CBT", Weight: 6000},
			{Code: "raport", Name: "Raport", Weight: 4000, Minimum: 60},
		},
		TieBreakers: []string{"cbt"},
	}
	formula, err := spec.Compile()
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if formula.Weights["cbt"] != 6000 || formula.Minimums["raport"] != 60 {
		t.Fatalf("compiled formula wrong: %+v", formula)
	}
	bad := spec
	bad.Components = []Component{{Code: "cbt", Weight: 5000}}
	if _, err := bad.Compile(); err == nil {
		t.Fatal("weights below 100% must fail to compile")
	}
	dup := FormulaSpec{Name: "x", Components: []Component{{Code: "a", Weight: 5000}, {Code: "a", Weight: 5000}}}
	if _, err := dup.Compile(); err == nil {
		t.Fatal("duplicate component codes must fail to compile")
	}
}

func TestRecommendedDecision(t *testing.T) {
	if RecommendedDecision(1, 10) != "accepted" || RecommendedDecision(10, 10) != "accepted" {
		t.Fatal("ranks within quota must be accepted")
	}
	if RecommendedDecision(11, 10) != "waitlisted" {
		t.Fatal("ranks beyond quota must be waitlisted")
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Tes CBT":        "tes_cbt",
		"Nilai Raport ":  "nilai_raport",
		"  Wawancara  ":  "wawancara",
		"Matematika-IPA": "matematika_ipa",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q)=%q want %q", in, got, want)
		}
	}
}

func TestApplyDiscountPolicy(t *testing.T) {
	c := []DiscountCandidate{
		{ID: "a", Kind: DiscountFixed, Value: 1_500_000, Priority: 1},
		{ID: "b", Kind: DiscountFixed, Value: 1_000_000, Priority: 2},
	}
	// AC-09-06: two promos worth 2.5M capped at 2M.
	net, steps, err := ApplyDiscountPolicy(10_000_000, c, "stack", 2_000_000)
	if err != nil || net != 8_000_000 || len(steps) != 2 || steps[1].Applied != 500_000 {
		t.Fatalf("stack+cap: net=%d steps=%+v err=%v", net, steps, err)
	}
	// mode max keeps only the biggest.
	net, steps, _ = ApplyDiscountPolicy(10_000_000, c, "max", 0)
	if net != 8_500_000 || len(steps) != 1 || steps[0].ID != "a" {
		t.Fatalf("max: net=%d steps=%+v", net, steps)
	}
	// AC-15-01 / AC-15-02: 10% off 10M = 9M, 50% minimum = 4.5M.
	net, _, _ = ApplyDiscountPolicy(10_000_000, []DiscountCandidate{{ID: "eb", Kind: DiscountPercent, Value: 1000}}, "stack", 0)
	if net != 9_000_000 || MinimumPayment(net, 50) != 4_500_000 || MinimumPayment(net, 0) != 0 {
		t.Fatalf("early bird net=%d", net)
	}
}
