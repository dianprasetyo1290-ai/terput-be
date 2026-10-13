package metrics

import (
	"errors"
	"time"
)

type MetricUnit string

const (
	UnitPercentBP  MetricUnit = "percent_bp"
	UnitIDR        MetricUnit = "idr"
	UnitCount      MetricUnit = "count"
	UnitRatioMilli MetricUnit = "ratio_milli"
	UnitDays       MetricUnit = "days"
)

type MetricDef struct {
	Key            string     `json:"key"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Formula        string     `json:"formula"`
	Unit           MetricUnit `json:"unit"`
	OwnerService   string     `json:"owner_service"`
	DataOwner      string     `json:"data_owner"`
	SourceMetric   string     `json:"source_metric"`
	AllowedGroupBy []string   `json:"allowed_group_by"`
	Additive       bool       `json:"additive"`
	RefreshMinutes *int       `json:"refresh_minutes"`
	Limitations    *string    `json:"limitations"`
	Version        int        `json:"version"`
	Status         string     `json:"status"`
}

type MetricSchoolParam struct {
	ID            string                 `json:"id"`
	SchoolID      string                 `json:"school_id"`
	MetricKey     string                 `json:"metric_key"`
	Params        map[string]interface{} `json:"params"`
	EffectiveFrom string                 `json:"effective_from"`
	EffectiveTo   *string                `json:"effective_to,omitempty"`
}

type MetricSnapshot struct {
	SchoolID    string     `json:"school_id"`
	MetricKey   string     `json:"metric_key"`
	PeriodKind  string     `json:"period_kind"`
	PeriodStart string     `json:"period_start"`
	PeriodEnd   string     `json:"period_end"`
	GroupBy     string     `json:"group_by"`
	DimValue    string     `json:"dim_value"`
	Numerator   *int64     `json:"numerator,omitempty"`
	Denominator *int64     `json:"denominator,omitempty"`
	Value       *int64     `json:"value,omitempty"`
	SourceAsOf  time.Time  `json:"source_as_of"`
	ComputedAt  time.Time  `json:"computed_at"`
}

type PeriodSpec struct {
	Kind  string `json:"kind"`  // 'relative' or 'explicit'
	Value string `json:"value"` // 'current_academic_year', 'last_12_months', 'this_month'
	From  string `json:"from"`  // 'YYYY-MM-DD'
	To    string `json:"to"`    // 'YYYY-MM-DD'
}

type TermInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

type AggregateRow struct {
	SchoolID    string `json:"school_id"`
	DimValue    string `json:"dim_value"`
	Numerator   int64  `json:"numerator"`
	Denominator int64  `json:"denominator"`
	Value       int64  `json:"value"`
}

type UnitValue struct {
	SchoolID    string `json:"school_id"`
	DimValue    string `json:"dim_value"`
	Numerator   int64  `json:"numerator"`
	Denominator int64  `json:"denominator"`
	Value       int64  `json:"value"`
}

type ConsolidatedValue struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
	Value       int64 `json:"value"`
}

// ResolvePeriod resolves dates based on timezone Asia/Jakarta
func ResolvePeriod(spec PeriodSpec, today time.Time, terms []TermInfo) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	t := today.In(loc)

	if spec.Kind == "explicit" {
		if spec.From == "" || spec.To == "" {
			return time.Time{}, time.Time{}, errors.New("from and to dates required for explicit period")
		}
		from, err := time.ParseInLocation("2006-01-02", spec.From, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to, err := time.ParseInLocation("2006-01-02", spec.To, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return from, to, nil
	}

	switch spec.Value {
	case "this_month":
		year, month, _ := t.Date()
		start := time.Date(year, month, 1, 0, 0, 0, 0, loc)
		end := start.AddDate(0, 1, -1)
		return start, end, nil

	case "last_12_months":
		year, month, _ := t.Date()
		thisMonthStart := time.Date(year, month, 1, 0, 0, 0, 0, loc)
		start := thisMonthStart.AddDate(-1, 0, 0)
		end := thisMonthStart.AddDate(0, 0, -1)
		return start, end, nil

	case "current_academic_year":
		// Standard Indonesian Academic Year: July 1 to June 30
		year, month, _ := t.Date()
		var startYear int
		if month >= time.July {
			startYear = year
		} else {
			startYear = year - 1
		}
		start := time.Date(startYear, time.July, 1, 0, 0, 0, 0, loc)
		end := time.Date(startYear+1, time.June, 30, 0, 0, 0, 0, loc)
		return start, end, nil

	default:
		return time.Time{}, time.Time{}, errors.New("unknown period spec value: " + spec.Value)
	}
}

// IsStale checks if snapshot needs refresh
func IsStale(snapshot *MetricSnapshot, def MetricDef, now time.Time, defaultMinutes int) bool {
	if snapshot == nil {
		return true
	}
	refreshMin := defaultMinutes
	if def.RefreshMinutes != nil && *def.RefreshMinutes > 0 {
		refreshMin = *def.RefreshMinutes
	}
	if refreshMin <= 0 {
		refreshMin = 60
	}
	cutoff := now.Add(-time.Duration(refreshMin) * time.Minute)
	return snapshot.ComputedAt.Before(cutoff)
}

// Aggregate calculates per-unit and consolidated values across rows
func Aggregate(def MetricDef, rows []AggregateRow) ([]UnitValue, ConsolidatedValue) {
	perUnit := make([]UnitValue, 0, len(rows))
	var totalNum int64
	var totalDen int64
	var totalVal int64

	for _, r := range rows {
		perUnit = append(perUnit, UnitValue{
			SchoolID:    r.SchoolID,
			DimValue:    r.DimValue,
			Numerator:   r.Numerator,
			Denominator: r.Denominator,
			Value:       r.Value,
		})
		totalNum += r.Numerator
		totalDen += r.Denominator
		totalVal += r.Value
	}

	var consolidated ConsolidatedValue
	consolidated.Numerator = totalNum
	consolidated.Denominator = totalDen

	if def.Additive {
		consolidated.Value = totalVal
	} else {
		// Ratio / percent: calculate from total numerator and denominator
		if totalDen > 0 {
			if def.Unit == UnitPercentBP {
				consolidated.Value = (totalNum * 10000) / totalDen
			} else if def.Unit == UnitRatioMilli {
				consolidated.Value = (totalNum * 1000) / totalDen
			} else {
				consolidated.Value = (totalNum * 10000) / totalDen
			}
		} else {
			consolidated.Value = 0
		}
	}

	return perUnit, consolidated
}
