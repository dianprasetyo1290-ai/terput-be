// Package domain holds small pure business-logic helpers shared by handlers.
package domain

import (
	"time"

	"skci/evaluation/internal/model"
)

// PeriodStatus computes the date-driven status of a period. Status is never
// stored -- it's derived on every read from `now()` vs the period's publish
// window plus the explicit `published` flag, per CONVENTIONS/spec:
//
//	Draft               : not yet published by admin
//	Published            : published, but now < publish_start_date
//	Sedang Berlangsung   : published AND publish_start_date <= now <= publish_end_date
//	Selesai              : now > publish_end_date (regardless of published flag)
func PeriodStatus(now time.Time, published bool, startDate, endDate string) string {
	start, errS := time.Parse("2006-01-02", startDate)
	end, errE := time.Parse("2006-01-02", endDate)
	if errS != nil || errE != nil {
		return model.StatusDraft
	}
	// end date is inclusive through the whole day
	endOfDay := end.Add(24 * time.Hour)

	if now.After(endOfDay) || now.Equal(endOfDay) {
		return model.StatusFinished
	}
	if !published {
		return model.StatusDraft
	}
	if now.Before(start) {
		return model.StatusPublished
	}
	return model.StatusOngoing
}

// FinalScoreLabel implements the spec's 2-tier label:
// [3.00, 4.00] -> "Promotif", below 3.00 -> "Tidak Promotif".
func FinalScoreLabel(score float64) string {
	if score >= 3.00 {
		return "Promotif"
	}
	return "Tidak Promotif"
}

// PerformanceCategory is a bonus finer-grained 4-tier label taken directly
// from the reference spreadsheet's RINGKASAN sheet (not required by spec,
// included as extra detail on the report/results payload).
func PerformanceCategory(score float64) string {
	switch {
	case score >= 3.60:
		return "Sangat Baik / Excellent"
	case score >= 3.00:
		return "Baik / Good"
	case score >= 2.00:
		return "Cukup / Adequate"
	default:
		return "Perlu Pembinaan / Needs Support"
	}
}
