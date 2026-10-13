package predict

import (
	"math"
	"slices"
	"time"
)

type InsightSettings struct {
	SchoolID          string     `json:"school_id"`
	PredictiveEnabled bool       `json:"predictive_enabled"`
	EnabledBy         *string    `json:"enabled_by,omitempty"`
	EnabledAt         *time.Time `json:"enabled_at,omitempty"`
	ReviewNote        *string    `json:"review_note,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type RiskRule struct {
	ID            string                 `json:"id"`
	SchoolID      string                 `json:"school_id"`
	Model         string                 `json:"model"` // 'arrears_risk', 'absence_risk', 'ppdb_forecast', 'cashflow_forecast'
	Version       int                    `json:"version"`
	Params        map[string]interface{} `json:"params"`
	EffectiveFrom string                 `json:"effective_from"`
	CreatedBy     string                 `json:"created_by"`
	Status        string                 `json:"status"` // 'active', 'retired'
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type PredictionRun struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	Model     string    `json:"model"`
	RuleID    string    `json:"rule_id"`
	RunAt     time.Time `json:"run_at"`
	Status    string    `json:"status"` // 'ok', 'failed', 'skipped'
	Reason    *string   `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type RiskFactor struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	RawValue     float64 `json:"raw_value"`
	Contribution float64 `json:"contribution"`
}

type Prediction struct {
	ID             string        `json:"id"`
	SchoolID       string        `json:"school_id"`
	RunID          string        `json:"run_id"`
	Model          string        `json:"model"`
	SubjectKind    string        `json:"subject_kind"` // 'student', 'wave', 'month'
	SubjectID      string        `json:"subject_id"`
	Score          *int          `json:"score,omitempty"`
	Level          *string       `json:"level,omitempty"` // 'low', 'medium', 'high'
	ProjectedValue *int64        `json:"projected_value,omitempty"`
	Factors        []RiskFactor  `json:"factors"`
	ReviewLabel    string        `json:"review_label"` // 'perlu_tinjauan'
	Reviews        []PredictionReview `json:"reviews,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}

type PredictionReview struct {
	ID           string    `json:"id"`
	SchoolID     string    `json:"school_id"`
	PredictionID string    `json:"prediction_id"`
	ReviewedBy   string    `json:"reviewed_by"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}

type PredictionAccuracy struct {
	ID          string    `json:"id"`
	SchoolID    string    `json:"school_id"`
	Model       string    `json:"model"`
	RuleID      string    `json:"rule_id"`
	PeriodStart string    `json:"period_start"`
	PeriodEnd   string    `json:"period_end"`
	Measure     string    `json:"measure"` // 'precision_bp', 'recall_bp', 'mape_bp'
	ValueBP     int       `json:"value_bp"`
	SampleSize  int       `json:"sample_size"`
	ComputedAt  time.Time `json:"computed_at"`
}

// StudentArrearsFacts holds input facts for arrears risk scoring
type StudentArrearsFacts struct {
	StudentUserID      string `json:"student_user_id"`
	OverdueBillCount   int    `json:"overdue_bill_count"`
	MaxDaysOverdue     int    `json:"max_days_overdue"`
	OutstandingRatioBP int    `json:"outstanding_ratio_bp"`
	LatePaymentCount   int    `json:"late_payment_count"`
}

type ArrearsRuleParams struct {
	WeightOverdueBills      float64 `json:"weight_overdue_bills"`
	CapOverdueBills         float64 `json:"cap_overdue_bills"`
	WeightMaxDaysOverdue    float64 `json:"weight_max_days_overdue"`
	CapMaxDaysOverdue       float64 `json:"cap_max_days_overdue"`
	WeightOutstandingRatio  float64 `json:"weight_outstanding_ratio"`
	CapOutstandingRatioBP   float64 `json:"cap_outstanding_ratio_bp"`
	WeightLatePaymentCount  float64 `json:"weight_late_payment_count"`
	CapLatePaymentCount     float64 `json:"cap_late_payment_count"`
	HighThresholdScore      int     `json:"high_threshold_score"`
	MediumThresholdScore    int     `json:"medium_threshold_score"`
}

// DefaultArrearsRuleParams provides reasonable fallback weights
func DefaultArrearsRuleParams() ArrearsRuleParams {
	return ArrearsRuleParams{
		WeightOverdueBills:     30,
		CapOverdueBills:        5,
		WeightMaxDaysOverdue:   30,
		CapMaxDaysOverdue:      90,
		WeightOutstandingRatio: 25,
		CapOutstandingRatioBP:  10000,
		WeightLatePaymentCount: 15,
		CapLatePaymentCount:    4,
		HighThresholdScore:     70,
		MediumThresholdScore:   40,
	}
}

// ArrearsRiskScore computes score (0-100), level, and top 3 explanatory factors
func ArrearsRiskScore(facts StudentArrearsFacts, params ArrearsRuleParams) (int, string, []RiskFactor) {
	norm := func(val, cap float64) float64 {
		if cap <= 0 {
			return 0
		}
		ratio := val / cap
		if ratio > 1.0 {
			return 1.0
		}
		if ratio < 0 {
			return 0
		}
		return ratio
	}

	fBills := norm(float64(facts.OverdueBillCount), params.CapOverdueBills) * params.WeightOverdueBills
	fDays := norm(float64(facts.MaxDaysOverdue), params.CapMaxDaysOverdue) * params.WeightMaxDaysOverdue
	fRatio := norm(float64(facts.OutstandingRatioBP), params.CapOutstandingRatioBP) * params.WeightOutstandingRatio
	fLate := norm(float64(facts.LatePaymentCount), params.CapLatePaymentCount) * params.WeightLatePaymentCount

	totalScore := int(math.Round(fBills + fDays + fRatio + fLate))
	if totalScore > 100 {
		totalScore = 100
	}

	level := "low"
	if totalScore >= params.HighThresholdScore {
		level = "high"
	} else if totalScore >= params.MediumThresholdScore {
		level = "medium"
	}

	factors := []RiskFactor{
		{
			Key:          "overdue_bill_count",
			Label:        "Jumlah Tagihan Jatuh Tempo",
			RawValue:     float64(facts.OverdueBillCount),
			Contribution: fBills,
		},
		{
			Key:          "max_days_overdue",
			Label:        "Keterlambatan Terlama (Hari)",
			RawValue:     float64(facts.MaxDaysOverdue),
			Contribution: fDays,
		},
		{
			Key:          "outstanding_ratio_bp",
			Label:        "Rasio Tunggakan",
			RawValue:     float64(facts.OutstandingRatioBP),
			Contribution: fRatio,
		},
		{
			Key:          "late_payment_count",
			Label:        "Riwayat Pembayaran Terlambat",
			RawValue:     float64(facts.LatePaymentCount),
			Contribution: fLate,
		},
	}

	// Sort descending by contribution and take top 3
	slices.SortFunc(factors, func(a, b RiskFactor) int {
		if b.Contribution > a.Contribution {
			return 1
		} else if b.Contribution < a.Contribution {
			return -1
		}
		return 0
	})

	top3 := factors
	if len(top3) > 3 {
		top3 = top3[:3]
	}

	return totalScore, level, top3
}

// StudentAbsenceFacts holds facts for absence risk scoring
type StudentAbsenceFacts struct {
	StudentUserID         string `json:"student_user_id"`
	AbsenceRateBP         int    `json:"absence_rate_bp"`
	ConsecutiveAbsentDays int    `json:"consecutive_absent_days"`
	UnexcusedCount        int    `json:"unexcused_count"`
	TrendDeltaBP          int    `json:"trend_delta_bp"` // current window vs previous window delta
}

type AbsenceRuleParams struct {
	WeightAbsenceRate       float64 `json:"weight_absence_rate"`
	CapAbsenceRateBP        float64 `json:"cap_absence_rate_bp"`
	WeightConsecutiveAbsent float64 `json:"weight_consecutive_absent"`
	CapConsecutiveAbsent    float64 `json:"cap_consecutive_absent"`
	WeightUnexcused         float64 `json:"weight_unexcused"`
	CapUnexcused            float64 `json:"cap_unexcused"`
	WeightTrendDelta        float64 `json:"weight_trend_delta"`
	CapTrendDeltaBP         float64 `json:"cap_trend_delta_bp"`
	HighThresholdScore      int     `json:"high_threshold_score"`
	MediumThresholdScore    int     `json:"medium_threshold_score"`
}

func DefaultAbsenceRuleParams() AbsenceRuleParams {
	return AbsenceRuleParams{
		WeightAbsenceRate:       35,
		CapAbsenceRateBP:        2500, // 25% absence
		WeightConsecutiveAbsent: 30,
		CapConsecutiveAbsent:    5,    // 5 consecutive days
		WeightUnexcused:         20,
		CapUnexcused:            5,
		WeightTrendDelta:        15,
		CapTrendDeltaBP:         1500, // +15% trend
		HighThresholdScore:      70,
		MediumThresholdScore:    40,
	}
}

// AbsenceRiskScore computes score, level, and top 3 explanatory factors
func AbsenceRiskScore(facts StudentAbsenceFacts, params AbsenceRuleParams) (int, string, []RiskFactor) {
	norm := func(val, cap float64) float64 {
		if cap <= 0 {
			return 0
		}
		ratio := val / cap
		if ratio > 1.0 {
			return 1.0
		}
		if ratio < 0 {
			return 0
		}
		return ratio
	}

	fRate := norm(float64(facts.AbsenceRateBP), params.CapAbsenceRateBP) * params.WeightAbsenceRate
	fConsec := norm(float64(facts.ConsecutiveAbsentDays), params.CapConsecutiveAbsent) * params.WeightConsecutiveAbsent
	fUnex := norm(float64(facts.UnexcusedCount), params.CapUnexcused) * params.WeightUnexcused
	fTrend := norm(float64(facts.TrendDeltaBP), params.CapTrendDeltaBP) * params.WeightTrendDelta

	totalScore := int(math.Round(fRate + fConsec + fUnex + fTrend))
	if totalScore > 100 {
		totalScore = 100
	}

	level := "low"
	if totalScore >= params.HighThresholdScore {
		level = "high"
	} else if totalScore >= params.MediumThresholdScore {
		level = "medium"
	}

	factors := []RiskFactor{
		{
			Key:          "absence_rate_bp",
			Label:        "Tingkat Ketidakhadiran",
			RawValue:     float64(facts.AbsenceRateBP),
			Contribution: fRate,
		},
		{
			Key:          "consecutive_absent_days",
			Label:        "Hari Berturut-turut Absen",
			RawValue:     float64(facts.ConsecutiveAbsentDays),
			Contribution: fConsec,
		},
		{
			Key:          "unexcused_count",
			Label:        "Jumlah Alpa Tanpa Keterangan",
			RawValue:     float64(facts.UnexcusedCount),
			Contribution: fUnex,
		},
		{
			Key:          "trend_delta_bp",
			Label:        "Kenaikan Tren Ketidakhadiran",
			RawValue:     float64(facts.TrendDeltaBP),
			Contribution: fTrend,
		},
	}

	slices.SortFunc(factors, func(a, b RiskFactor) int {
		if b.Contribution > a.Contribution {
			return 1
		} else if b.Contribution < a.Contribution {
			return -1
		}
		return 0
	})

	top3 := factors
	if len(top3) > 3 {
		top3 = top3[:3]
	}

	return totalScore, level, top3
}

// ProjectApplicants projects total applicants using previous wave curve.
// FR-154-002: If prevCurve is empty or prevCum == 0 -> returns nil + "data historis tidak cukup".
func ProjectApplicants(currentCum int, dayIndex int, prevCurve []int) (*int, string) {
	if len(prevCurve) == 0 {
		return nil, "data historis tidak cukup"
	}
	if dayIndex < 0 || dayIndex >= len(prevCurve) {
		return nil, "indeks hari di luar batas historis"
	}

	totalPrev := prevCurve[len(prevCurve)-1]
	prevCumAtSameDay := prevCurve[dayIndex]

	if prevCumAtSameDay <= 0 || totalPrev <= 0 {
		return nil, "data historis tidak cukup"
	}

	projected := (currentCum * totalPrev) / prevCumAtSameDay
	return &projected, ""
}

// ProjectCashflow projects expected cashflow based on due amount and historical collection rate.
func ProjectCashflow(dueByMonth int64, avgCollectionRateBp int) int64 {
	if dueByMonth <= 0 || avgCollectionRateBp <= 0 {
		return 0
	}
	return (dueByMonth * int64(avgCollectionRateBp)) / 10000
}

// Precision calculates precision in basis points.
// FR-154-004: Empty set returns nil, NOT 0.
func Precision(flaggedHigh []string, actualOverdue []string) *int {
	if len(flaggedHigh) == 0 {
		return nil
	}

	actualSet := make(map[string]bool)
	for _, id := range actualOverdue {
		actualSet[id] = true
	}

	truePositives := 0
	for _, id := range flaggedHigh {
		if actualSet[id] {
			truePositives++
		}
	}

	bp := (truePositives * 10000) / len(flaggedHigh)
	return &bp
}

// Recall calculates recall in basis points.
// FR-154-004: Empty actual set returns nil, NOT 0.
func Recall(flaggedHigh []string, actualOverdue []string) *int {
	if len(actualOverdue) == 0 {
		return nil
	}

	actualSet := make(map[string]bool)
	for _, id := range actualOverdue {
		actualSet[id] = true
	}

	truePositives := 0
	for _, id := range flaggedHigh {
		if actualSet[id] {
			truePositives++
		}
	}

	bp := (truePositives * 10000) / len(actualOverdue)
	return &bp
}

// MAPE calculates Mean Absolute Percentage Error in basis points.
// Empty set returns nil, NOT 0.
func MAPE(projected []int64, actual []int64) *int {
	if len(projected) == 0 || len(actual) == 0 || len(projected) != len(actual) {
		return nil
	}

	var sumAPE float64
	validCount := 0

	for i := range actual {
		act := actual[i]
		proj := projected[i]
		if act > 0 {
			ape := math.Abs(float64(proj-act)) / float64(act)
			sumAPE += ape
			validCount++
		}
	}

	if validCount == 0 {
		return nil
	}

	meanAPE := sumAPE / float64(validCount)
	mapeBP := int(math.Round(meanAPE * 10000))
	return &mapeBP
}
