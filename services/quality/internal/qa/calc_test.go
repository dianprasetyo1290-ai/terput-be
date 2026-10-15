package qa

import (
	"testing"
)

func TestScoreFromRubric(t *testing.T) {
	rubric := []RubricTier{
		{MinValue: 8500, Score: 4}, // 85%
		{MinValue: 7000, Score: 3}, // 70%
		{MinValue: 5500, Score: 2}, // 55%
	}
	scoreMin := 1

	tests := []struct {
		name     string
		value    int64
		expected int
	}{
		{"exact top threshold", 8500, 4},
		{"above top threshold", 9200, 4},
		{"middle tier", 7500, 3},
		{"lower tier", 6000, 2},
		{"below all tiers returns scoreMin", 4000, 1},
		{"boundary lowest", 5500, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScoreFromRubric(tc.value, rubric, scoreMin)
			if got != tc.expected {
				t.Errorf("ScoreFromRubric(%d) = %d; want %d", tc.value, got, tc.expected)
			}
		})
	}
}

func TestStandardAndInstrumentScore(t *testing.T) {
	items := []ItemScore{
		{Score: 4, WeightBP: 5000}, // 50%
		{Score: 3, WeightBP: 5000}, // 50%
	}
	score, err := StandardScore(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// average is 3.5 -> 3500 milli
	if score != 3500 {
		t.Errorf("StandardScore() = %d; want 3500", score)
	}

	// Instrument score combining standards
	standards := []StandardScoreItem{
		{StandardScoreMilli: 3500, WeightBP: 4000}, // 40% -> 1400
		{StandardScoreMilli: 4000, WeightBP: 6000}, // 60% -> 2400
	}
	instScore, err := InstrumentScore(standards)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if instScore != 3800 {
		t.Errorf("InstrumentScore() = %d; want 3800", instScore)
	}

	// Zero weight errors
	_, err = StandardScore([]ItemScore{{Score: 4, WeightBP: 0}})
	if err == nil {
		t.Error("expected error for zero weight standard, got nil")
	}

	_, err = InstrumentScore([]StandardScoreItem{{StandardScoreMilli: 4000, WeightBP: 0}})
	if err == nil {
		t.Error("expected error for zero weight instrument, got nil")
	}
}

func TestBelowTarget_AC_144_01(t *testing.T) {
	// AC-144-01: target 2 (ratio_milli 2000), capaian 1.4 (1400), higher_better -> true
	actual1400 := int64(1400)
	target2000 := int64(2000)

	if !BelowTarget(&actual1400, target2000, "higher_better") {
		t.Errorf("BelowTarget(1400, 2000, higher_better) should be true")
	}

	actual2100 := int64(2100)
	if BelowTarget(&actual2100, target2000, "higher_better") {
		t.Errorf("BelowTarget(2100, 2000, higher_better) should be false")
	}

	// lower_better (e.g. angka putus sekolah)
	if !BelowTarget(&actual2100, target2000, "lower_better") {
		t.Errorf("BelowTarget(2100, 2000, lower_better) should be true (actual exceeds target threshold)")
	}

	// nil actual -> false
	if BelowTarget(nil, target2000, "higher_better") {
		t.Errorf("BelowTarget(nil, 2000) should be false")
	}
}

func TestFinalizeBlockers_AC_144_01(t *testing.T) {
	four := 4
	items := []QASelfEvalItem{
		{ID: "item-1", IndicatorID: "ind-1", Score: &four, BelowTarget: false},
		{ID: "item-2", IndicatorID: "ind-2", Score: &four, BelowTarget: true},
		{ID: "item-3", IndicatorID: "ind-3", Score: nil, BelowTarget: false},
	}

	// Without action plans for item-2
	plans := []QAActionPlan{}
	blockers := FinalizeBlockers(items, plans)

	if len(blockers) != 2 {
		t.Fatalf("expected 2 blockers (missing RTL for item-2, missing score for item-3), got %d: %+v", len(blockers), blockers)
	}

	// Add action plan for item-2
	plans = append(plans, QAActionPlan{
		ID:             "plan-1",
		SelfEvalItemID: "item-2",
		Status:         "open",
	})
	blockers = FinalizeBlockers(items, plans)
	if len(blockers) != 1 {
		t.Fatalf("expected 1 blocker (missing score for item-3), got %d: %+v", len(blockers), blockers)
	}
	if blockers[0].Code != "missing_score" {
		t.Errorf("expected blocker code missing_score, got %s", blockers[0].Code)
	}
}

func TestCompleteness_AC_143_01(t *testing.T) {
	// AC-143-01: 10 & 7 -> 7000 bp (70%)
	bp := Completeness(10, 7)
	if bp != 7000 {
		t.Errorf("Completeness(10, 7) = %d; want 7000", bp)
	}

	if Completeness(0, 0) != 0 {
		t.Errorf("Completeness(0, 0) should be 0 without panic")
	}

	if Completeness(10, 10) != 10000 {
		t.Errorf("Completeness(10, 10) = %d; want 10000", Completeness(10, 10))
	}
}

func TestIsLocked_And_UnitCompatible(t *testing.T) {
	if !IsLocked("finalized") {
		t.Error("finalized must be locked")
	}
	if !IsLocked("pending_finalization") {
		t.Error("pending_finalization must be locked")
	}
	if !IsLocked("pending_reopen") {
		t.Error("pending_reopen must be locked")
	}
	if IsLocked("draft") {
		t.Error("draft must not be locked")
	}

	if !UnitCompatible("percent_bp", "percent_bp") {
		t.Error("percent_bp vs percent_bp should be compatible")
	}
	if UnitCompatible("percent_bp", "idr") {
		t.Error("percent_bp vs idr should not be compatible")
	}
}

func TestStandardScores(t *testing.T) {
	inst := &QAInstrument{Standards: []QAStandard{
		{ID: "s1", Code: "SKL", Name: "Lulusan", WeightBP: 5000, Items: []QAItem{{Indicators: []QAIndicator{
			{ID: "i1", WeightBP: 5000}, {ID: "i2", WeightBP: 5000},
		}}}},
		{ID: "s2", Code: "SI", Name: "Isi", WeightBP: 5000, Items: []QAItem{{Indicators: []QAIndicator{
			{ID: "i3", WeightBP: 10000},
		}}}},
	}}
	three, four := 3, 4
	se := &QASelfEvaluation{Items: []QASelfEvalItem{
		{IndicatorID: "i1", Score: &three}, {IndicatorID: "i2", Score: &four},
	}}
	got := standardScores(inst, se)
	if len(got) != 1 {
		t.Fatalf("want only the scored standard, got %+v", got)
	}
	if got[0].StandardID != "s1" || got[0].Code != "SKL" || got[0].ScoreMilli != 3500 || got[0].Score != 3.5 {
		t.Fatalf("unexpected standard score: %+v", got[0])
	}
}
