package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	ErrCapacityFull = errors.New("Kapasitas kendaraan penuh")
)

type Settings struct {
	SchoolID            string    `json:"school_id"`
	ProrataPolicy       string    `json:"prorata_policy"` // 'daily' | 'full_month'
	DocReminderDays     int       `json:"doc_reminder_days"`
	NotifyParentOnTrip  bool      `json:"notify_parent_on_trip"`
	ComponentCode       string    `json:"component_code"`
	BillDueDay          *int      `json:"bill_due_day"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Zone struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Name      string     `json:"name"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type Route struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Status    string     `json:"status"` // 'active' | 'inactive'
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Stops     []Stop     `json:"stops,omitempty"`
}

type Stop struct {
	ID          string     `json:"id"`
	SchoolID    string     `json:"school_id"`
	RouteID     string     `json:"route_id"`
	Seq         int        `json:"seq"`
	Name        string     `json:"name"`
	Address     *string    `json:"address,omitempty"`
	ZoneID      *string    `json:"zone_id,omitempty"`
	ZoneName    *string    `json:"zone_name,omitempty"`
	PickupTime  *string    `json:"pickup_time,omitempty"`
	DropoffTime *string    `json:"dropoff_time,omitempty"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Vehicle struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	PlateNo   string     `json:"plate_no"`
	Name      string     `json:"name"`
	Capacity  int        `json:"capacity"`
	Status    string     `json:"status"` // 'active' | 'maintenance' | 'inactive'
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type VehicleDocument struct {
	ID         string     `json:"id"`
	SchoolID   string     `json:"school_id"`
	VehicleID  string     `json:"vehicle_id"`
	Kind       string     `json:"kind"` // 'stnk' | 'uji_kir' | 'asuransi' | 'lainnya'
	Number     string     `json:"number"`
	ValidUntil time.Time  `json:"valid_until"`
	ObjectKey  *string    `json:"object_key,omitempty"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Crew struct {
	ID                string     `json:"id"`
	SchoolID          string     `json:"school_id"`
	Kind              string     `json:"kind"` // 'driver' | 'attendant'
	EmployeeID        *string    `json:"employee_id,omitempty"`
	UserID            *string    `json:"user_id,omitempty"`
	ExternalName      *string    `json:"external_name,omitempty"`
	Name              string     `json:"name,omitempty"` // computed display name
	Phone             *string    `json:"phone,omitempty"`
	LicenseNo         *string    `json:"license_no,omitempty"`
	LicenseValidUntil *time.Time `json:"license_valid_until,omitempty"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type RouteAssignment struct {
	ID              string     `json:"id"`
	SchoolID        string     `json:"school_id"`
	RouteID         string     `json:"route_id"`
	VehicleID       string     `json:"vehicle_id"`
	DriverCrewID    string     `json:"driver_crew_id"`
	AttendantCrewID *string    `json:"attendant_crew_id,omitempty"`
	ValidFrom       time.Time  `json:"valid_from"`
	ValidTo         *time.Time `json:"valid_to,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Fare struct {
	ID               string     `json:"id"`
	SchoolID         string     `json:"school_id"`
	ZoneID           *string    `json:"zone_id,omitempty"`
	RouteID          *string    `json:"route_id,omitempty"`
	MonthlyAmountIDR int64      `json:"monthly_amount_idr"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to,omitempty"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type Allocation struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	StudentUserID string     `json:"student_user_id"`
	StudentName   string     `json:"student_name,omitempty"`
	RouteID       string     `json:"route_id"`
	RouteCode     string     `json:"route_code,omitempty"`
	RouteName     string     `json:"route_name,omitempty"`
	StopID        string     `json:"stop_id"`
	StopName      string     `json:"stop_name,omitempty"`
	StartDate     time.Time  `json:"start_date"`
	EndDate       *time.Time `json:"end_date,omitempty"`
	EndReason     *string    `json:"end_reason,omitempty"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Trip struct {
	ID              string     `json:"id"`
	SchoolID        string     `json:"school_id"`
	RouteID         string     `json:"route_id"`
	RouteName       string     `json:"route_name,omitempty"`
	TripDate        time.Time  `json:"trip_date"`
	Direction       string     `json:"direction"` // 'jemput' | 'antar'
	VehicleID       string     `json:"vehicle_id"`
	VehiclePlateNo  string     `json:"vehicle_plate_no,omitempty"`
	DriverCrewID    string     `json:"driver_crew_id"`
	DriverName      string     `json:"driver_name,omitempty"`
	AttendantCrewID *string    `json:"attendant_crew_id,omitempty"`
	AttendantName   *string    `json:"attendant_name,omitempty"`
	Status          string     `json:"status"` // 'planned' | 'running' | 'done'
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type TripEvent struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	TripID        string    `json:"trip_id"`
	StudentUserID string    `json:"student_user_id"`
	StopID        *string   `json:"stop_id,omitempty"`
	Event         string    `json:"event"`  // 'board' | 'alight' | 'absent'
	Source        string    `json:"source"` // 'scan' | 'manual'
	RecordedBy    string    `json:"recorded_by"`
	RecordedAt    time.Time `json:"recorded_at"`
	ClientEventID string    `json:"client_event_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type AbsenceNotice struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	StudentUserID string     `json:"student_user_id"`
	NoticeDate    time.Time  `json:"notice_date"`
	Direction     string     `json:"direction"` // 'jemput' | 'antar' | 'both'
	Reason        *string    `json:"reason,omitempty"`
	CreatedBy     string     `json:"created_by"`
	CancelledAt   *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type ChargeItem struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	AmountIDR int64  `json:"amount_idr"`
	Days      int    `json:"days,omitempty"`
	RateIDR   int64  `json:"rate_idr,omitempty"`
}

type Charge struct {
	ID            string          `json:"id"`
	SchoolID      string          `json:"school_id"`
	StudentUserID string          `json:"student_user_id"`
	StudentName   string          `json:"student_name,omitempty"`
	PeriodYear    int             `json:"period_year"`
	PeriodMonth   int             `json:"period_month"`
	Items         json.RawMessage `json:"items"`
	AmountIDR     int64           `json:"amount_idr"`
	Version       int             `json:"version"`
	BillID        *string         `json:"bill_id,omitempty"`
	Status        string          `json:"status"` // 'issued' | 'adjusted' | 'void' | 'failed'
	LastError     *string         `json:"last_error,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Roster Models for Stops & Manifest
type StopRoster struct {
	Stop     Stop         `json:"stop"`
	Students []Allocation `json:"students"`
}

type ManifestEntry struct {
	StudentUserID string     `json:"student_user_id"`
	StudentName   string     `json:"student_name"`
	StopID        string     `json:"stop_id"`
	StopName      string     `json:"stop_name"`
	StopSeq       int        `json:"stop_seq"`
	Status        string     `json:"status"` // 'belum_tercatat' | 'naik' | 'turun' | 'tidak_ikut' | 'absen'
	BoardTime     *time.Time `json:"board_time,omitempty"`
	AlightTime    *time.Time `json:"alight_time,omitempty"`
	NoticeReason  *string    `json:"notice_reason,omitempty"`
}

// ---------------------------------------------------------------------
// Pure Calculation Functions (§3)
// ---------------------------------------------------------------------

// StudentsByStop orders stops by seq and attaches allocations active on date (AC-119-01).
func StudentsByStop(stops []Stop, allocations []Allocation, date time.Time) []StopRoster {
	// Normalize target date to date-only
	y, m, d := date.Date()
	targetDate := time.Date(y, m, d, 0, 0, 0, 0, date.Location())

	allocMap := make(map[string][]Allocation)
	for _, a := range allocations {
		ay, am, ad := a.StartDate.Date()
		start := time.Date(ay, am, ad, 0, 0, 0, 0, a.StartDate.Location())

		if targetDate.Before(start) {
			continue
		}
		if a.EndDate != nil {
			ey, em, ed := a.EndDate.Date()
			end := time.Date(ey, em, ed, 0, 0, 0, 0, a.EndDate.Location())
			if targetDate.After(end) {
				continue
			}
		}
		allocMap[a.StopID] = append(allocMap[a.StopID], a)
	}

	result := make([]StopRoster, len(stops))
	for i, s := range stops {
		students := allocMap[s.ID]
		if students == nil {
			students = []Allocation{}
		}
		result[i] = StopRoster{
			Stop:     s,
			Students: students,
		}
	}
	return result
}

// CanAllocate checks if vehicle capacity allows adding new allocations (AC-121-01, BR-TRS-01).
func CanAllocate(capacity, activeOnRoute, newCount int) error {
	if activeOnRoute+newCount > capacity {
		return fmt.Errorf("%w: Kapasitas kendaraan penuh (%d/%d)", ErrCapacityFull, activeOnRoute, capacity)
	}
	return nil
}

// CheckVehicleChange prevents assigning vehicle with smaller capacity than current peak allocations.
func CheckVehicleChange(newCapacity, peakAllocated int) error {
	if newCapacity < peakAllocated {
		return fmt.Errorf("Kapasitas kendaraan baru (%d) lebih kecil dari siswa teralokasi (%d)", newCapacity, peakAllocated)
	}
	return nil
}

// DocReminderDue calculates if a vehicle doc or driver license is due for expiration (AC-120-01).
func DocReminderDue(validUntil, today time.Time, reminderDays int) (due bool, daysLeft int) {
	vy, vm, vd := validUntil.Date()
	ty, tm, td := today.Date()

	vDate := time.Date(vy, vm, vd, 0, 0, 0, 0, time.UTC)
	tDate := time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC)

	diffHours := vDate.Sub(tDate).Hours()
	daysLeft = int(math.Round(diffHours / 24.0))

	if daysLeft <= reminderDays {
		return true, daysLeft
	}
	return false, daysLeft
}

// Manifest merges roster with trip events and absence notices.
func Manifest(roster []Allocation, events []TripEvent, notices []AbsenceNotice, direction string) []ManifestEntry {
	// Map notices by student
	noticeMap := make(map[string]AbsenceNotice)
	for _, n := range notices {
		if n.Direction == direction || n.Direction == "both" {
			noticeMap[n.StudentUserID] = n
		}
	}

	// Map events by student
	boardMap := make(map[string]TripEvent)
	alightMap := make(map[string]TripEvent)
	absentMap := make(map[string]TripEvent)
	for _, ev := range events {
		switch ev.Event {
		case "board":
			boardMap[ev.StudentUserID] = ev
		case "alight":
			alightMap[ev.StudentUserID] = ev
		case "absent":
			absentMap[ev.StudentUserID] = ev
		}
	}

	res := make([]ManifestEntry, len(roster))
	for i, a := range roster {
		status := "belum_tercatat"
		var boardTime, alightTime *time.Time
		var noticeReason *string

		if n, hasNotice := noticeMap[a.StudentUserID]; hasNotice {
			status = "tidak_ikut"
			noticeReason = n.Reason
		} else if ev, hasAlight := alightMap[a.StudentUserID]; hasAlight {
			status = "turun"
			alightTime = &ev.RecordedAt
			if b, hasBoard := boardMap[a.StudentUserID]; hasBoard {
				boardTime = &b.RecordedAt
			}
		} else if b, hasBoard := boardMap[a.StudentUserID]; hasBoard {
			status = "naik"
			boardTime = &b.RecordedAt
		} else if _, hasAbsent := absentMap[a.StudentUserID]; hasAbsent {
			status = "absen"
		}

		res[i] = ManifestEntry{
			StudentUserID: a.StudentUserID,
			StudentName:   a.StudentName,
			StopID:        a.StopID,
			StopName:      a.StopName,
			Status:        status,
			BoardTime:     boardTime,
			AlightTime:    alightTime,
			NoticeReason:  noticeReason,
		}
	}
	return res
}

// ValidateTripEvent verifies sequence validity (e.g. alight requires board).
func ValidateTripEvent(prev []TripEvent, ev TripEvent) error {
	hasBoard := false
	hasAlight := false

	for _, p := range prev {
		if p.StudentUserID == ev.StudentUserID {
			if p.ClientEventID == ev.ClientEventID && p.Event == ev.Event {
				return nil // idempotent retry
			}
			if p.Event == "board" {
				hasBoard = true
			}
			if p.Event == "alight" {
				hasAlight = true
			}
		}
	}

	if ev.Event == "alight" && !hasBoard {
		return errors.New("Siswa belum tercatat naik (board)")
	}
	if ev.Event == "board" && hasBoard {
		return errors.New("Siswa sudah tercatat naik")
	}
	if ev.Event == "alight" && hasAlight {
		return errors.New("Siswa sudah tercatat turun")
	}

	return nil
}

// FareFor resolves monthly amount, preferring route fare over zone fare (effective date check).
func FareFor(fares []Fare, routeID, zoneID *string, date time.Time) (int64, error) {
	y, m, d := date.Date()
	targetDate := time.Date(y, m, d, 0, 0, 0, 0, date.Location())

	var matchedRouteFare *Fare
	var matchedZoneFare *Fare

	for i := range fares {
		f := &fares[i]
		fy, fm, fd := f.EffectiveFrom.Date()
		efFrom := time.Date(fy, fm, fd, 0, 0, 0, 0, f.EffectiveFrom.Location())
		if targetDate.Before(efFrom) {
			continue
		}
		if f.EffectiveTo != nil {
			ty, tm, td := f.EffectiveTo.Date()
			efTo := time.Date(ty, tm, td, 0, 0, 0, 0, f.EffectiveTo.Location())
			if !targetDate.Before(efTo) {
				continue
			}
		}

		if routeID != nil && f.RouteID != nil && *f.RouteID == *routeID {
			matchedRouteFare = f
		} else if zoneID != nil && f.ZoneID != nil && *f.ZoneID == *zoneID {
			matchedZoneFare = f
		}
	}

	// Route fare wins over zone fare
	if matchedRouteFare != nil {
		return matchedRouteFare.MonthlyAmountIDR, nil
	}
	if matchedZoneFare != nil {
		return matchedZoneFare.MonthlyAmountIDR, nil
	}
	return 0, errors.New("Tarif belum diatur")
}

// ProrateIDR prorates monthly fare rounded half-up to integer rupiah (AC-122-01).
func ProrateIDR(monthly int64, daysUsed, daysInPeriod int) int64 {
	if daysUsed <= 0 || daysInPeriod <= 0 || monthly <= 0 {
		return 0
	}
	if daysUsed >= daysInPeriod {
		return monthly
	}
	ratio := float64(daysUsed) / float64(daysInPeriod)
	return int64(math.Round(float64(monthly) * ratio))
}

type AllocationSegment struct {
	AllocationID string
	RouteID      string
	RouteName    string
	ZoneID       *string
	StartDate    time.Time
	EndDate      *time.Time
}

// DaysInMonth returns total days in given year/month.
func DaysInMonth(year, month int) int {
	return time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
}

// ComputeCharge calculates line items and total for a student in a billing period (AC-122-01).
func ComputeCharge(segments []AllocationSegment, fares []Fare, policy string, periodYear, periodMonth int) ([]ChargeItem, int64, error) {
	daysInPeriod := DaysInMonth(periodYear, periodMonth)
	firstOfMonth := time.Date(periodYear, time.Month(periodMonth), 1, 0, 0, 0, 0, time.UTC)
	lastOfMonth := time.Date(periodYear, time.Month(periodMonth), daysInPeriod, 23, 59, 59, 0, time.UTC)

	type activeSeg struct {
		segment  AllocationSegment
		daysUsed int
		fare     int64
	}
	var actives []activeSeg

	for _, s := range segments {
		// Calculate overlap with period
		start := s.StartDate
		if start.Before(firstOfMonth) {
			start = firstOfMonth
		}
		end := lastOfMonth
		if s.EndDate != nil && s.EndDate.Before(lastOfMonth) {
			end = *s.EndDate
		}

		if start.After(end) {
			continue
		}

		// Days used inclusive: (end.Day - start.Day + 1)
		daysUsed := end.Day() - start.Day() + 1
		if daysUsed <= 0 {
			continue
		}

		fareMonthly, err := FareFor(fares, &s.RouteID, s.ZoneID, start)
		if err != nil {
			return nil, 0, fmt.Errorf("rute %s: %w", s.RouteName, err)
		}

		actives = append(actives, activeSeg{
			segment:  s,
			daysUsed: daysUsed,
			fare:     fareMonthly,
		})
	}

	if len(actives) == 0 {
		return []ChargeItem{}, 0, nil
	}

	var items []ChargeItem
	var total int64

	if policy == "full_month" {
		// Pick single full month fare from segment with most days (tie break: last segment)
		bestIdx := 0
		for i := 1; i < len(actives); i++ {
			if actives[i].daysUsed >= actives[bestIdx].daysUsed {
				bestIdx = i
			}
		}
		best := actives[bestIdx]
		items = append(items, ChargeItem{
			Code:      "TRANSPORT",
			Name:      fmt.Sprintf("Transportasi %s (Sebulan Penuh)", best.segment.RouteName),
			AmountIDR: best.fare,
			Days:      daysInPeriod,
			RateIDR:   best.fare,
		})
		total = best.fare
	} else {
		// Policy == 'daily' (prorated per segment)
		for _, a := range actives {
			prorated := ProrateIDR(a.fare, a.daysUsed, daysInPeriod)
			items = append(items, ChargeItem{
				Code:      "TRANSPORT",
				Name:      fmt.Sprintf("Transportasi %s (%d/%d hari)", a.segment.RouteName, a.daysUsed, daysInPeriod),
				AmountIDR: prorated,
				Days:      a.daysUsed,
				RateIDR:   a.fare,
			})
			total += prorated
		}
	}

	return items, total, nil
}

// ChargeDiff reports true if recomputed amount or count differs from issued charge.
func ChargeDiff(issued *Charge, recomputedItems []ChargeItem, recomputedTotal int64) bool {
	if issued == nil {
		return true
	}
	if issued.AmountIDR != recomputedTotal {
		return true
	}
	var existingItems []ChargeItem
	if err := json.Unmarshal(issued.Items, &existingItems); err != nil {
		return true
	}
	if len(existingItems) != len(recomputedItems) {
		return true
	}
	for i := range existingItems {
		if existingItems[i].AmountIDR != recomputedItems[i].AmountIDR ||
			existingItems[i].Code != recomputedItems[i].Code {
			return true
		}
	}
	return false
}
