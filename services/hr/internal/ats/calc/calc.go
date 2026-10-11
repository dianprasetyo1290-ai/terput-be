package calc

import (
	"time"
)

// ExceedsHeadcount checks if the requested headcount exceeds vacant slots.
// Vector AC-88-01: vacant 1, requested 2 -> true
func ExceedsHeadcount(vacant, requested int) bool {
	return requested > vacant
}

// WeightedScore calculates overall weighted score rounded to the nearest integer.
// criteriaWeights in bp (total 10000) or relative weights, panelScores 0-100.
// Vector AC-91-01: weights 6000 and 4000, scores 80 and 90 -> 84
func WeightedScore(criteriaWeights []int, scores []int) int {
	if len(criteriaWeights) == 0 || len(criteriaWeights) != len(scores) {
		return 0
	}
	totalWeight := 0
	weightedSum := 0
	for i := range criteriaWeights {
		w := criteriaWeights[i]
		s := scores[i]
		totalWeight += w
		weightedSum += s * w
	}
	if totalWeight == 0 {
		return 0
	}
	// Round half-up: (2*sum + total) / (2*total)
	return (2*weightedSum + totalWeight) / (2 * totalWeight)
}

type PanelScoreItem struct {
	PanelUserID string `json:"panel_user_id"`
	CriterionID string `json:"criterion_id"`
	Score       int    `json:"score"`
}

// VisibleScores filters scores so that a panelist who hasn't submitted can only see their own scores.
// Vector AC-91-02: belum submit -> hanya miliknya
func VisibleScores(viewerID string, isViewerSubmitted bool, allScores []PanelScoreItem) []PanelScoreItem {
	if isViewerSubmitted {
		return allScores
	}
	var visible []PanelScoreItem
	for _, sc := range allScores {
		if sc.PanelUserID == viewerID {
			visible = append(visible, sc)
		}
	}
	return visible
}

// SalaryOutOfRange checks if base salary exceeds or is lower than the position's salary range.
// Vector AC-92-01: 5.5 jt vs 4-5 jt -> true
func SalaryOutOfRange(baseIDR, minIDR, maxIDR int64) bool {
	if minIDR > 0 && baseIDR < minIDR {
		return true
	}
	if maxIDR > 0 && baseIDR > maxIDR {
		return true
	}
	return false
}

// ShouldClose checks whether a job posting should be automatically closed.
// Vector AC-89-02: closes_on 31 Oct, today 1 Nov -> close
func ShouldClose(closesOn *time.Time, today time.Time, filled bool) bool {
	if filled {
		return true
	}
	if closesOn != nil && !closesOn.IsZero() {
		// Compare date only
		closesDate := time.Date(closesOn.Year(), closesOn.Month(), closesOn.Day(), 23, 59, 59, 0, closesOn.Location())
		if today.After(closesDate) {
			return true
		}
	}
	return false
}

// ShouldAnonymize checks whether candidate data should be anonymized according to retention policy.
// Vector AC-90-02: rejected, tanpa talent pool, > 6 bln -> true
func ShouldAnonymize(status string, consentTalentPool bool, lastActivity time.Time, retentionMonths int, now time.Time) bool {
	if consentTalentPool {
		return false
	}
	if status != "rejected" && status != "withdrawn" {
		return false
	}
	cutoff := lastActivity.AddDate(0, retentionMonths, 0)
	return now.After(cutoff)
}
