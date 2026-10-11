package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// WorkingDays calculates inclusive working days between start and end,
// excluding non-scheduled days (scheduleDays: 1=Mon..7=Sun) and holidays.
func WorkingDays(start, end time.Time, scheduleDays []int16, holidays map[string]bool) (float64, error) {
	if start.After(end) {
		return 0, errors.New("start date cannot be after end date")
	}

	if len(scheduleDays) == 0 {
		return 0, errors.New("jadwal kerja belum diatur")
	}
	activeDays := map[int]bool{}
	for _, d := range scheduleDays {
		activeDays[int(d)] = true
	}

	days := 0.0
	// Normalize to midnight UTC for date comparison
	curr := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

	for !curr.After(last) {
		// Go Weekday(): Sunday=0, Monday=1 .. Saturday=6
		// ISO Weekday: Monday=1 .. Sunday=7
		isoWeekday := int(curr.Weekday())
		if isoWeekday == 0 {
			isoWeekday = 7
		}

		dateStr := curr.Format("2006-01-02")
		if activeDays[isoWeekday] && !holidays[dateStr] {
			days += 1.0
		}
		curr = curr.AddDate(0, 0, 1)
	}

	return days, nil
}

// CanTakeLeave enforces BR-HR-04: if leave type has a quota,
// entitled + carried - used must be >= days.
func CanTakeLeave(balance LeaveBalance, days float64, hasQuota bool) (bool, string) {
	if !hasQuota {
		return true, ""
	}
	remaining := balance.Entitled + balance.Carried - balance.Used
	if remaining < days {
		return false, fmt.Sprintf("Saldo cuti tidak mencukupi (sisa %g hari)", remaining)
	}
	return true, ""
}

// ApplyLeaveDecision updates the leave balance upon approval or cancellation.
func ApplyLeaveDecision(balance *LeaveBalance, days float64, approved bool) {
	if balance == nil {
		return
	}
	if approved {
		balance.Used += days
	} else {
		balance.Used -= days
		if balance.Used < 0 {
			balance.Used = 0
		}
	}
	balance.Remaining = balance.Entitled + balance.Carried - balance.Used
}

// AttendanceStatus determines whether a check-in is 'hadir' or 'terlambat'.
func AttendanceStatus(checkIn time.Time, schedule WorkSchedule, loc *time.Location) string {
	if schedule.StartTime == "" {
		return "hadir"
	}
	if loc == nil {
		loc = time.UTC
	}
	tIn := checkIn.In(loc)

	// schedule.StartTime is "15:04" or "15:04:05"
	parts := strings.Split(schedule.StartTime, ":")
	if len(parts) < 2 {
		return "hadir"
	}
	var schedHour, schedMin int
	fmt.Sscanf(parts[0], "%d", &schedHour)
	fmt.Sscanf(parts[1], "%d", &schedMin)

	schedTime := time.Date(tIn.Year(), tIn.Month(), tIn.Day(), schedHour, schedMin, 0, 0, loc)
	deadline := schedTime.Add(time.Duration(schedule.LateToleranceMinutes) * time.Minute)

	if tIn.After(deadline) {
		return "terlambat"
	}
	return "hadir"
}

// MaskString masks a sensitive string, keeping at most the last 4 characters.
func MaskString(s string) string {
	clean := strings.TrimSpace(s)
	if clean == "" {
		return ""
	}
	if len(clean) <= 4 {
		return "••••"
	}
	tail := clean[len(clean)-4:]
	return "••••" + tail
}

func MaskNIK(s string) string         { return MaskString(s) }
func MaskNPWP(s string) string        { return MaskString(s) }
func MaskBankAccount(s string) string { return MaskString(s) }
func MaskPhone(s string) string       { return MaskString(s) }

// MaskEmployee applies BR-HR-03 and AC-83-04:
// Full data only for hr_admin, hr_manager, payroll_officer, or the employee himself.
func MaskEmployee(emp Employee, viewerRoles []string, viewerUserID string) Employee {
	canViewFull := false
	if emp.UserID != nil && *emp.UserID != "" && *emp.UserID == viewerUserID {
		canViewFull = true
	}
	for _, r := range viewerRoles {
		if r == "hr_admin" || r == "hr_manager" || r == "payroll_officer" {
			canViewFull = true
			break
		}
	}

	if canViewFull {
		return emp
	}

	masked := emp
	if emp.NIK != nil {
		m := MaskString(*emp.NIK)
		masked.NIK = &m
	}
	if emp.NPWP != nil {
		m := MaskString(*emp.NPWP)
		masked.NPWP = &m
	}
	if emp.BankAccount != nil {
		m := MaskString(*emp.BankAccount)
		masked.BankAccount = &m
	}
	if emp.Phone != nil {
		m := MaskString(*emp.Phone)
		masked.Phone = &m
	}
	if emp.Address != nil {
		m := "••••"
		masked.Address = &m
	}
	masked.CurrentBaseSalaryIDR = nil
	return masked
}

// DuplicateCheck finds duplicate candidates based on NIK blind index or email.
func DuplicateCheck(candidates []Employee, nikBidx string, email string) []DuplicateMatch {
	var matches []DuplicateMatch
	normEmail := strings.ToLower(strings.TrimSpace(email))

	for _, c := range candidates {
		matchReason := ""
		if nikBidx != "" && c.NIKBidx != nil && *c.NIKBidx == nikBidx {
			matchReason = "NIK sudah terdaftar"
		} else if normEmail != "" && c.Email != nil && strings.ToLower(strings.TrimSpace(*c.Email)) == normEmail {
			matchReason = "Email sudah terdaftar"
		}

		if matchReason != "" {
			em := ""
			if c.Email != nil {
				em = *c.Email
			}
			matches = append(matches, DuplicateMatch{
				ID:          c.ID,
				EmployeeNo:  c.EmployeeNo,
				FullName:    c.FullName,
				Email:       em,
				MatchReason: matchReason,
			})
		}
	}
	return matches
}

// DocumentCompleteness checks required documents against employee documents.
// AC-85-02: marks completeness, identifying missing or expired documents.
func DocumentCompleteness(required []RequiredDocument, docs []EmployeeDocument, today time.Time) (bool, []string) {
	if len(required) == 0 {
		return true, nil
	}

	todayStr := today.Format("2006-01-02")
	docByType := map[string]EmployeeDocument{}
	for _, d := range docs {
		// Use latest valid version
		if existing, ok := docByType[d.DocumentTypeID]; !ok || d.Version > existing.Version {
			docByType[d.DocumentTypeID] = d
		}
	}

	var missing []string
	for _, req := range required {
		doc, found := docByType[req.DocumentTypeID]
		name := req.DocumentTypeName
		if name == "" {
			name = req.DocumentTypeCode
		}
		if name == "" {
			name = "dokumen"
		}
		if !found {
			missing = append(missing, strings.ToLower(name))
			continue
		}
		if doc.ExpiresOn != nil && *doc.ExpiresOn != "" && *doc.ExpiresOn < todayStr {
			missing = append(missing, strings.ToLower(name)+" (kedaluwarsa)")
		}
	}

	return len(missing) == 0, missing
}

// Accrue calculates monthly leave accrual: (annualQuota / 12 * monthsWorked), rounded down to 0.5.
func Accrue(annualQuota float64, monthsWorked int) float64 {
	if monthsWorked <= 0 || annualQuota <= 0 {
		return 0
	}
	raw := (annualQuota / 12.0) * float64(monthsWorked)
	// Round down to 0.5: floor(raw * 2) / 2
	return math.Floor(raw*2.0) / 2.0
}

// CarryOver calculates unused leave carry-over into the new year capped at maxCarryOver.
func CarryOver(prevRemaining float64, maxCarryOver float64) float64 {
	if prevRemaining <= 0 || maxCarryOver <= 0 {
		return 0
	}
	if prevRemaining > maxCarryOver {
		return maxCarryOver
	}
	return prevRemaining
}

// DistanceM calculates great-circle distance in meters between two lat/lng coordinates (Haversine formula).
func DistanceM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371000.0 // meters
	toRad := func(d float64) float64 { return d * math.Pi / 180.0 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2.0)*math.Sin(dLat/2.0) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2.0)*math.Sin(dLng/2.0)
	c := 2.0 * math.Atan2(math.Sqrt(a), math.Sqrt(1.0-a))
	return earthRadius * c
}
