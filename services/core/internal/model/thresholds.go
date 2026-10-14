package model

import "fmt"

// ValidateAmountThresholds (BUG-166, TC-79-013) rejects overlapping nominal
// rules. Steps with min_amount_idr form a tiered ladder per condition_flag:
// each tier must start strictly above the previous one, so two steps with the
// same threshold (or a lower threshold after a higher one) for the same
// condition are refused. Steps without a threshold always apply and are not
// part of the ladder. Thresholds must be positive.
func ValidateAmountThresholds(steps []FlowStep) error {
	last := map[string]FlowStep{}
	for _, st := range steps {
		if st.MinAmountIDR == nil {
			continue
		}
		if *st.MinAmountIDR <= 0 {
			return fmt.Errorf("tahap %d: ambang nominal (min_amount_idr) harus lebih dari 0", st.Seq)
		}
		cond := ""
		if st.ConditionFlag != nil {
			cond = *st.ConditionFlag
		}
		if prev, ok := last[cond]; ok && *st.MinAmountIDR <= *prev.MinAmountIDR {
			return fmt.Errorf("tahap %d: ambang nominal Rp%d tumpang tindih dengan tahap %d (Rp%d); ambang tahap berikutnya harus lebih besar",
				st.Seq, *st.MinAmountIDR, prev.Seq, *prev.MinAmountIDR)
		}
		last[cond] = st
	}
	return nil
}
