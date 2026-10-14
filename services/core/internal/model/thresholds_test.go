package model

import "testing"

func TestValidateAmountThresholds(t *testing.T) {
	amt := func(v int64) *int64 { return &v }
	flag := func(v string) *string { return &v }
	tests := []struct {
		name    string
		steps   []FlowStep
		wantErr bool
	}{
		{"tanpa ambang", []FlowStep{{Seq: 1}, {Seq: 2}}, false},
		{"berjenjang naik", []FlowStep{{Seq: 1}, {Seq: 2, MinAmountIDR: amt(4_000_000)}, {Seq: 3, MinAmountIDR: amt(10_000_000)}}, false},
		{"BUG-166: ambang sama ditolak", []FlowStep{{Seq: 1, MinAmountIDR: amt(4_000_000)}, {Seq: 2, MinAmountIDR: amt(4_000_000)}}, true},
		{"ambang turun ditolak", []FlowStep{{Seq: 1, MinAmountIDR: amt(10_000_000)}, {Seq: 2, MinAmountIDR: amt(5_000_000)}}, true},
		{"ambang sama beda kondisi diterima", []FlowStep{{Seq: 1, MinAmountIDR: amt(4_000_000)}, {Seq: 2, MinAmountIDR: amt(4_000_000), ConditionFlag: flag("lintas_unit")}}, false},
		{"ambang sama kondisi sama ditolak", []FlowStep{{Seq: 1, MinAmountIDR: amt(4_000_000), ConditionFlag: flag("x")}, {Seq: 2, MinAmountIDR: amt(4_000_000), ConditionFlag: flag("x")}}, true},
		{"ambang nol ditolak", []FlowStep{{Seq: 1, MinAmountIDR: amt(0)}}, true},
		{"ambang negatif ditolak", []FlowStep{{Seq: 1, MinAmountIDR: amt(-1)}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateAmountThresholds(tt.steps); (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
