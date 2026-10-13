package bi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"time"

	"skci/insight/internal/common"
	"skci/insight/internal/metrics"
)

type ScopeClaims struct {
	UserID     string
	SchoolID   string
	ActiveRole string
}

type Dashboard struct {
	ID          string            `json:"id"`
	SchoolID    string            `json:"school_id"`
	OwnerUserID string            `json:"owner_user_id"`
	Title       string            `json:"title"`
	ScopeKind   string            `json:"scope_kind"` // 'unit', 'foundation'
	SharedRoles []string          `json:"shared_roles"`
	Widgets     []DashboardWidget `json:"widgets,omitempty"`
	DeletedAt   *time.Time        `json:"deleted_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type DashboardWidget struct {
	ID        string             `json:"id"`
	SchoolID  string             `json:"school_id"`
	DashboardID string           `json:"dashboard_id"`
	Sort      int                `json:"sort"`
	Title     string             `json:"title"`
	MetricKey string             `json:"metric_key"`
	Chart     string             `json:"chart"` // 'stat', 'bar', 'line', 'table'
	GroupBy   *string            `json:"group_by,omitempty"`
	Period    metrics.PeriodSpec `json:"period"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type SavedReport struct {
	ID          string                 `json:"id"`
	SchoolID    string                 `json:"school_id"`
	OwnerUserID string                 `json:"owner_user_id"`
	Title       string                 `json:"title"`
	Columns     []string               `json:"columns"`
	Filters     map[string]interface{} `json:"filters"`
	SharedRoles []string               `json:"shared_roles"`
	DeletedAt   *time.Time             `json:"deleted_at,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type ScheduledReport struct {
	ID               string     `json:"id"`
	SchoolID         string     `json:"school_id"`
	SavedReportID    string     `json:"saved_report_id"`
	Frequency        string     `json:"frequency"` // 'daily', 'weekly', 'monthly'
	Weekday          *int       `json:"weekday,omitempty"`
	MonthDay         *int       `json:"month_day,omitempty"`
	SendHour         int        `json:"send_hour"`
	RecipientUserIDs []string   `json:"recipient_user_ids"`
	Active           bool       `json:"active"`
	NextRunAt        time.Time  `json:"next_run_at"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type ScheduledReportRun struct {
	ID             string                 `json:"id"`
	SchoolID       string                 `json:"school_id"`
	ScheduleID     string                 `json:"schedule_id"`
	RecipientUserID string                `json:"recipient_user_id"`
	Status         string                 `json:"status"` // 'sent', 'skipped', 'failed'
	Reason         *string                `json:"reason,omitempty"`
	Deliveries     map[string]interface{} `json:"deliveries,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key"`
	CreatedAt      time.Time              `json:"created_at"`
}

type AnalyticsScopeGrant struct {
	ID              string     `json:"id"`
	SchoolID        string     `json:"school_id"`
	UserID          string     `json:"user_id"`
	GrantedSchoolID string     `json:"granted_school_id"`
	GrantedBy       string     `json:"granted_by"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type BiExportToken struct {
	ID         string     `json:"id"`
	SchoolID   string     `json:"school_id"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"-"`
	RawToken   string     `json:"raw_token,omitempty"` // only on creation
	MetricKeys []string   `json:"metric_keys"`
	SchoolIDs  []string   `json:"school_ids"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// ResolveScope clips unauthorized school_ids. AC-152-01: Requests outside scope are clipped, not rejected.
func ResolveScope(claims ScopeClaims, grants []AnalyticsScopeGrant, foundationSchools []string, requested []string) ([]string, error) {
	if claims.ActiveRole == common.RoleSuperadmin {
		if len(requested) > 0 {
			return requested, nil
		}
		if len(foundationSchools) > 0 {
			return foundationSchools, nil
		}
		return []string{claims.SchoolID}, nil
	}

	var allowedPool []string
	switch claims.ActiveRole {
	case common.RoleKepalaYayasan:
		// Allowed: all schools in foundation
		allowedPool = slices.Clone(foundationSchools)
		if len(allowedPool) == 0 && claims.SchoolID != "" {
			allowedPool = []string{claims.SchoolID}
		}

	case common.RoleBiAnalyst:
		// Allowed: own school + active grants for schools within foundation
		allowedPool = []string{claims.SchoolID}
		for _, g := range grants {
			if g.RevokedAt == nil {
				// grant must be within foundation
				if len(foundationSchools) == 0 || slices.Contains(foundationSchools, g.GrantedSchoolID) {
					if !slices.Contains(allowedPool, g.GrantedSchoolID) {
						allowedPool = append(allowedPool, g.GrantedSchoolID)
					}
				}
			}
		}

	case common.RoleKepalaSekolah, common.RoleAdmin, common.RoleFinanceManager:
		// Allowed: own school only
		allowedPool = []string{claims.SchoolID}

	default:
		// Any other role: own school only if present
		if claims.SchoolID != "" {
			allowedPool = []string{claims.SchoolID}
		} else {
			return nil, errors.New("unauthorized: no school scope")
		}
	}

	if len(requested) == 0 {
		return allowedPool, nil
	}

	// Clip requested to allowedPool
	var clipped []string
	for _, req := range requested {
		if slices.Contains(allowedPool, req) && !slices.Contains(clipped, req) {
			clipped = append(clipped, req)
		}
	}

	if len(clipped) == 0 {
		// Fallback to own school if allowed, or first allowed
		if slices.Contains(allowedPool, claims.SchoolID) {
			return []string{claims.SchoolID}, nil
		}
		if len(allowedPool) > 0 {
			return []string{allowedPool[0]}, nil
		}
		return nil, errors.New("no accessible school in scope")
	}

	return clipped, nil
}

// NextRun computes the next execution time for scheduled reports
func NextRun(sched ScheduledReport, now time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	t := now.In(loc)

	switch sched.Frequency {
	case "daily":
		target := time.Date(t.Year(), t.Month(), t.Day(), sched.SendHour, 0, 0, 0, loc)
		if !target.After(t) {
			target = target.AddDate(0, 0, 1)
		}
		return target

	case "weekly":
		targetWeekday := time.Monday
		if sched.Weekday != nil && *sched.Weekday >= 0 && *sched.Weekday <= 6 {
			targetWeekday = time.Weekday(*sched.Weekday)
		}
		// Start today with send_hour
		target := time.Date(t.Year(), t.Month(), t.Day(), sched.SendHour, 0, 0, 0, loc)
		// advance until weekday matches and target > t
		daysToAdd := (int(targetWeekday) - int(t.Weekday()) + 7) % 7
		if daysToAdd == 0 && !target.After(t) {
			daysToAdd = 7
		}
		return target.AddDate(0, 0, daysToAdd)

	case "monthly":
		desiredDay := 1
		if sched.MonthDay != nil && *sched.MonthDay >= 1 && *sched.MonthDay <= 31 {
			desiredDay = *sched.MonthDay
		}

		// Helper to find date for year, month, desiredDay
		dateForMonth := func(year int, month time.Month) time.Time {
			// Find max days in this month
			firstOfNextMonth := time.Date(year, month+1, 1, 0, 0, 0, 0, loc)
			lastDayOfMonth := firstOfNextMonth.AddDate(0, 0, -1).Day()
			day := desiredDay
			if day > lastDayOfMonth {
				day = lastDayOfMonth
			}
			return time.Date(year, month, day, sched.SendHour, 0, 0, 0, loc)
		}

		target := dateForMonth(t.Year(), t.Month())
		if !target.After(t) {
			// Move to next month
			nextMonth := t.Month() + 1
			year := t.Year()
			if nextMonth > 12 {
				nextMonth = 1
				year++
			}
			target = dateForMonth(year, nextMonth)
		}
		return target

	default:
		return t.Add(24 * time.Hour)
	}
}
