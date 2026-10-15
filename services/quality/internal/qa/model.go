package qa

import (
	"errors"
	"fmt"
	"time"
)

// RubricTier represents one score tier threshold in an indicator rubric.
type RubricTier struct {
	MinValue int64 `json:"min_value"`
	Score    int   `json:"score"`
}

type QAInstrument struct {
	ID           string     `json:"id"`
	SchoolID     string     `json:"school_id"`
	Code         string     `json:"code"`
	Name         string     `json:"name"`
	Issuer       string     `json:"issuer"`
	VersionLabel string     `json:"version_label"`
	ScoreMin     int        `json:"score_min"`
	ScoreMax     int        `json:"score_max"`
	Status       string     `json:"status"` // draft, active, retired
	EffectiveFrom *string   `json:"effective_from"`
	Standards    []QAStandard `json:"standards,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type QAStandard struct {
	ID           string        `json:"id"`
	SchoolID     string        `json:"school_id"`
	InstrumentID string        `json:"instrument_id"`
	Sort         int           `json:"sort"`
	Code         string        `json:"code"`
	Name         string        `json:"name"`
	WeightBP     int           `json:"weight_bp"`
	Items        []QAItem      `json:"items,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type QAItem struct {
	ID         string        `json:"id"`
	SchoolID   string        `json:"school_id"`
	StandardID string        `json:"standard_id"`
	Sort       int           `json:"sort"`
	Code       string        `json:"code"`
	Statement  string        `json:"statement"`
	Indicators []QAIndicator `json:"indicators,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

type QAIndicator struct {
	ID         string       `json:"id"`
	SchoolID   string       `json:"school_id"`
	ItemID     string       `json:"item_id"`
	Sort       int          `json:"sort"`
	Code       string       `json:"code"`
	Name       string       `json:"name"`
	WeightBP   int          `json:"weight_bp"`
	Unit       string       `json:"unit"` // percent_bp, count, ratio_milli, idr, days, score
	Direction  string       `json:"direction"` // higher_better, lower_better
	SourceKind string       `json:"source_kind"` // manual, metric
	MetricKey  *string      `json:"metric_key"`
	Rubric     []RubricTier `json:"rubric"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

type QATarget struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	IndicatorID    string    `json:"indicator_id"`
	AcademicYearID string    `json:"academic_year_id"`
	TargetValue    int64     `json:"target_value"`
	OwnerUserID    string    `json:"owner_user_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type QAEvidence struct {
	ID           string                 `json:"id"`
	SchoolID     string                 `json:"school_id"`
	Kind         string                 `json:"kind"` // file, sekoci_report, link
	Title        string                 `json:"title"`
	ObjectKey    *string                `json:"object_key,omitempty"`
	Mime         *string                `json:"mime,omitempty"`
	SizeBytes    *int64                 `json:"size_bytes,omitempty"`
	Checksum     *string                `json:"checksum,omitempty"`
	ReportRef    map[string]interface{} `json:"report_ref,omitempty"`
	URL          *string                `json:"url,omitempty"`
	Version      int                    `json:"version"`
	SupersedesID *string                `json:"supersedes_id,omitempty"`
	UploadedBy   string                 `json:"uploaded_by"`
	Status       string                 `json:"status"` // pending, verified, rejected
	VerifiedBy   *string                `json:"verified_by,omitempty"`
	VerifiedAt   *time.Time             `json:"verified_at,omitempty"`
	RejectReason *string                `json:"reject_reason,omitempty"`
	IndicatorIDs []string               `json:"indicator_ids,omitempty"`
	LockedAt     *time.Time             `json:"locked_at,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type QASelfEvaluation struct {
	ID               string               `json:"id"`
	SchoolID         string               `json:"school_id"`
	InstrumentID     string               `json:"instrument_id"`
	AcademicYearID   string               `json:"academic_year_id"`
	PeriodLabel      string               `json:"period_label"`
	PeriodFrom       string               `json:"period_from"`
	PeriodTo         string               `json:"period_to"`
	Status           string               `json:"status"` // draft, pending_finalization, finalized, pending_reopen
	ApprovalID       *string              `json:"approval_id,omitempty"`
	ReopenApprovalID *string              `json:"reopen_approval_id,omitempty"`
	FinalizedAt      *time.Time           `json:"finalized_at,omitempty"`
	FinalizedBy      *string              `json:"finalized_by,omitempty"`
	Version          int                  `json:"version"`
	Items            []QASelfEvalItem     `json:"items,omitempty"`
	// StandardScores: per-standard weighted score of the scored items
	// (standards with no scored indicator are omitted).
	StandardScores []SelfEvalStandardScore `json:"standard_scores,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

// SelfEvalStandardScore is one standard's aggregate on a self evaluation.
type SelfEvalStandardScore struct {
	StandardID string  `json:"standard_id"`
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	ScoreMilli int64   `json:"score_milli"`
	Score      float64 `json:"score"`
}

type QASelfEvalItem struct {
	ID               string          `json:"id"`
	SchoolID         string          `json:"school_id"`
	SelfEvaluationID string          `json:"self_evaluation_id"`
	IndicatorID      string          `json:"indicator_id"`
	TargetValue      *int64          `json:"target_value"`
	ActualValue      *int64          `json:"actual_value"`
	ValueSource      string          `json:"value_source"` // manual, metric
	MetricAsOf       *time.Time      `json:"metric_as_of,omitempty"`
	MetricFetchedAt  *time.Time      `json:"metric_fetched_at,omitempty"`
	Score            *int            `json:"score"`
	BelowTarget      bool            `json:"below_target"`
	Note             *string         `json:"note"`
	AssessedBy       *string         `json:"assessed_by,omitempty"`
	ActionPlans      []QAActionPlan  `json:"action_plans,omitempty"`
	Indicator        *QAIndicator    `json:"indicator,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type QAActionPlan struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	SelfEvalItemID string     `json:"self_eval_item_id"`
	Action         string     `json:"action"`
	OwnerUserID    string     `json:"owner_user_id"`
	DueDate        string     `json:"due_date"`
	Status         string     `json:"status"` // open, in_progress, done, cancelled
	IsOverdue      bool       `json:"is_overdue"` // calculated based on Asia/Jakarta today
	DoneAt         *time.Time `json:"done_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ItemScore input for StandardScore.
type ItemScore struct {
	Score    int
	WeightBP int
}

// StandardScoreItem input for InstrumentScore.
type StandardScoreItem struct {
	StandardScoreMilli int64
	WeightBP           int
}

type Blocker struct {
	Code        string `json:"code"`
	IndicatorID string `json:"indicator_id"`
	Message     string `json:"message"`
}

// ScoreFromRubric determines score from rubric tiers.
// Rubric tiers: [{min_value, score}].
// Higher threshold takes precedence. If below all thresholds, returns scoreMin.
func ScoreFromRubric(value int64, rubric []RubricTier, scoreMin int) int {
	bestScore := scoreMin
	bestMin := int64(-1)
	matched := false

	for _, tier := range rubric {
		if value >= tier.MinValue {
			if !matched || tier.MinValue >= bestMin {
				bestMin = tier.MinValue
				bestScore = tier.Score
				matched = true
			}
		}
	}
	return bestScore
}

// StandardScore computes weighted score for a standard in milli points (e.g. 3500 for 3.5).
// Rounding is half-up.
func StandardScore(items []ItemScore) (int64, error) {
	var totalWeight int64
	var totalWeightedScore int64

	for _, item := range items {
		if item.WeightBP <= 0 {
			continue
		}
		totalWeight += int64(item.WeightBP)
		totalWeightedScore += int64(item.Score) * 1000 * int64(item.WeightBP)
	}

	if totalWeight == 0 {
		return 0, errors.New("bobot standar belum diisi")
	}

	// Half-up rounding: (numerator + denominator/2) / denominator
	res := (totalWeightedScore + (totalWeight / 2)) / totalWeight
	return res, nil
}

// InstrumentScore computes total score across all standards in milli points.
// Rounding is half-up.
func InstrumentScore(standards []StandardScoreItem) (int64, error) {
	var totalWeight int64
	var totalWeightedScore int64

	for _, std := range standards {
		if std.WeightBP <= 0 {
			continue
		}
		totalWeight += int64(std.WeightBP)
		totalWeightedScore += std.StandardScoreMilli * int64(std.WeightBP)
	}

	if totalWeight == 0 {
		return 0, errors.New("bobot instrumen belum diisi")
	}

	res := (totalWeightedScore + (totalWeight / 2)) / totalWeight
	return res, nil
}

// BelowTarget evaluates whether actual is below target value.
// Target & actual use the same unit (e.g. percent_bp, ratio_milli, count, idr).
func BelowTarget(actual *int64, target int64, direction string) bool {
	if actual == nil {
		return false
	}
	if direction == "lower_better" {
		return *actual > target
	}
	// default higher_better
	return *actual < target
}

// FinalizeBlockers checks for conditions blocking self evaluation finalization.
func FinalizeBlockers(items []QASelfEvalItem, plans []QAActionPlan) []Blocker {
	var blockers []Blocker

	// Build map of item_id -> active plans count
	plansByItem := make(map[string]int)
	for _, p := range plans {
		if p.Status == "open" || p.Status == "in_progress" || p.Status == "done" {
			plansByItem[p.SelfEvalItemID]++
		}
	}

	for _, item := range items {
		if item.Score == nil {
			blockers = append(blockers, Blocker{
				Code:        "missing_score",
				IndicatorID: item.IndicatorID,
				Message:     fmt.Sprintf("Indikator %s belum dinilai", item.IndicatorID),
			})
		}
		if item.BelowTarget {
			activeCount := plansByItem[item.ID]
			if activeCount == 0 {
				blockers = append(blockers, Blocker{
					Code:        "missing_action_plan",
					IndicatorID: item.IndicatorID,
					Message:     "Rencana tindak lanjut wajib untuk indikator di bawah target",
				})
			}
		}
	}
	return blockers
}

// Completeness computes percentage of indicators having verified evidence in basis points (0-10000).
func Completeness(totalIndicators int, withVerifiedEvidence int) int64 {
	if totalIndicators <= 0 {
		return 0
	}
	if withVerifiedEvidence <= 0 {
		return 0
	}
	if withVerifiedEvidence >= totalIndicators {
		return 10000
	}
	// (verified * 10000 + total/2) / total (half-up)
	return int64((withVerifiedEvidence*10000 + totalIndicators/2) / totalIndicators)
}

// IsLocked checks whether self evaluation state prevents modification.
func IsLocked(evalStatus string) bool {
	switch evalStatus {
	case "finalized", "pending_finalization", "pending_reopen":
		return true
	default:
		return false
	}
}

// UnitCompatible checks whether indicator unit matches metric unit.
func UnitCompatible(indicatorUnit, metricUnit string) bool {
	return indicatorUnit == metricUnit
}
