// Package facility implements menu 116–118: master ruang (BR-FAC-02),
// pemesanan ruang dengan cek bentrok (BR-FAC-01) dan perintah kerja
// perawatan. This file holds the types and the pure rules (no I/O) so the
// business logic is table-tested in model_test.go.
package facility

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Room kinds, usage, statuses: single Go source for the CHECK lists in
// migrations/0002_facility.up.sql.
var (
	RoomKinds     = []string{"kelas", "lab", "aula", "lapangan", "perpustakaan", "ruang_serbaguna", "kantin", "kantor", "uks", "gudang", "lainnya"}
	RoomUsages    = []string{UsageBookable, UsageFixed}
	RoomStatuses  = []string{RoomActive, RoomMaintenance, RoomInactive}
	Priorities    = []string{"rendah", "sedang", "tinggi", "darurat"}
	CostKinds     = []string{"material", "jasa", "lainnya"}
	IntervalUnits = []string{"day", "week", "month"}
)

const (
	UsageBookable = "bookable"
	UsageFixed    = "fixed"

	RoomActive      = "active"
	RoomMaintenance = "maintenance"
	RoomInactive    = "inactive"

	BookingPending   = "pending_approval"
	BookingConfirmed = "confirmed"
	BookingRejected  = "rejected"
	BookingCancelled = "cancelled"

	WODilaporkan       = "dilaporkan"
	WODitugaskan       = "ditugaskan"
	WODiproses         = "diproses"
	WOMenungguMaterial = "menunggu_material"
	WOSelesai          = "selesai"
	WODitutup          = "ditutup"
	WODibatalkan       = "dibatalkan"

	SourceReport     = "report"
	SourcePreventive = "preventive"
	SourceTicket     = "ticket"

	BusyBooking = "booking"
	BusyFixed   = "fixed"
)

// Booking rule violations (BR-FAC-01, FR-116-002, AC-117-01).
var (
	ErrNotBookable   = errors.New("Ruang tidak dapat dipesan (penggunaan tetap, dalam perbaikan, atau nonaktif)")
	ErrOverCapacity  = errors.New("Jumlah peserta melebihi kapasitas ruang")
	ErrOutsideHours  = errors.New("Waktu pemesanan di luar jam pakai ruang")
	ErrConflict      = errors.New("Waktu pemesanan bentrok dengan pemakaian lain")
	ErrInvalidPeriod = errors.New("Waktu selesai harus setelah waktu mulai pada hari yang sama")
)

// Clock is a wall-clock time of day in minutes since midnight (Postgres TIME).
type Clock int

// ParseClock accepts "HH:MM" or "HH:MM:SS".
func ParseClock(s string) (Clock, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("format jam tidak valid: %q", s)
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("format jam tidak valid: %q", s)
	}
	return Clock(h*60 + m), nil
}

func (c Clock) String() string { return fmt.Sprintf("%02d:%02d", int(c)/60, int(c)%60) }

// on returns the instant this clock falls on for the given local day.
func (c Clock) on(day time.Time) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, int(c)/60, int(c)%60, 0, 0, day.Location())
}

// Interval is a half-open [Start, End) period.
type Interval struct {
	Start time.Time `json:"starts_at"`
	End   time.Time `json:"ends_at"`
}

// Overlaps reports whether two half-open intervals share any instant:
// 10–12 vs 12–13 do not overlap, 10–12 vs 11–13 do.
func Overlaps(a, b Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

// Room is one row of the room master (FR-116-001/002).
type Room struct {
	ID               string      `json:"id"`
	SchoolID         string      `json:"-"`
	FloorID          string      `json:"floor_id"`
	Code             string      `json:"code"`
	Name             string      `json:"name"`
	Kind             string      `json:"kind"`
	Capacity         int         `json:"capacity"`
	Usage            string      `json:"usage"`
	RequiresApproval bool        `json:"requires_approval"`
	Status           string      `json:"status"`
	AvailableFrom    *string     `json:"available_from"`
	AvailableTo      *string     `json:"available_to"`
	Notes            *string     `json:"notes"`
	DeletedAt        *time.Time  `json:"-"`
	Building         RefBuilding `json:"building"`
	Floor            RefFloor    `json:"floor"`
	Facilities       []Facility  `json:"facilities"`
}

type RefBuilding struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RefFloor struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
	Name  string `json:"name"`
}

type Facility struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

// Bookable: only active, non-deleted rooms marked bookable appear in the
// booking search (AC-116-01: aula dalam perbaikan tidak tampil; FR-116-002:
// ruang kelas penggunaan tetap tidak tampil).
func Bookable(r Room) bool {
	return r.Usage == UsageBookable && r.Status == RoomActive && r.DeletedAt == nil
}

// NeedsApproval: rooms flagged requires_approval go through core's Approval
// Center (FR-117-003); others are confirmed immediately.
func NeedsApproval(r Room) bool { return r.RequiresApproval }

// Window is the room's usable period on the given local day; rooms without
// available_from/to are usable the whole day.
func Window(r Room, day time.Time) Interval {
	y, m, d := day.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, day.Location())
	w := Interval{Start: start, End: start.AddDate(0, 0, 1)}
	if r.AvailableFrom == nil || r.AvailableTo == nil {
		return w
	}
	from, err1 := ParseClock(*r.AvailableFrom)
	to, err2 := ParseClock(*r.AvailableTo)
	if err1 != nil || err2 != nil {
		return w
	}
	return Interval{Start: from.on(start), End: to.on(start)}
}

// FixedSchedule is a recurring weekly block on a bookable room (e.g. jadwal
// praktikum). Weekday is ISO: 1 = Senin … 7 = Minggu.
type FixedSchedule struct {
	ID         string  `json:"id"`
	RoomID     string  `json:"room_id"`
	Weekday    int     `json:"weekday"`
	StartsTime string  `json:"starts_time"`
	EndsTime   string  `json:"ends_time"`
	Label      string  `json:"label"`
	ValidFrom  string  `json:"valid_from"`
	ValidTo    *string `json:"valid_to"`
}

// BusySlot is one occupied period of a room (booking or fixed schedule).
type BusySlot struct {
	Interval
	Kind         string  `json:"kind"`
	Label        string  `json:"label"`
	BookedByName *string `json:"booked_by_name,omitempty"`
	BookingID    *string `json:"booking_id,omitempty"`
}

func isoWeekday(t time.Time) int {
	wd := int(t.Weekday())
	if wd == 0 {
		return 7
	}
	return wd
}

// ExpandFixed materialises the fixed schedules that fall on day (interpreted
// in loc), honouring valid_from/valid_to.
func ExpandFixed(schedules []FixedSchedule, day time.Time, loc *time.Location) []BusySlot {
	local := day.In(loc)
	y, m, d := local.Date()
	dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
	dateStr := dayStart.Format("2006-01-02")
	out := []BusySlot{}
	for _, s := range schedules {
		if s.Weekday != isoWeekday(dayStart) || dateStr < s.ValidFrom || (s.ValidTo != nil && dateStr > *s.ValidTo) {
			continue
		}
		from, err1 := ParseClock(s.StartsTime)
		to, err2 := ParseClock(s.EndsTime)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, BusySlot{
			Interval: Interval{Start: from.on(dayStart), End: to.on(dayStart)},
			Kind:     BusyFixed,
			Label:    s.Label,
		})
	}
	return out
}

// CheckBooking applies BR-FAC-01 to one request: room must be bookable,
// participants within capacity, period within the room's hours on a single
// local day, and no overlap with busy slots. On conflict it also returns the
// slot that blocks the request.
func CheckBooking(room Room, req Interval, participants int, busy []BusySlot, loc *time.Location) (*BusySlot, error) {
	if !Bookable(room) {
		return nil, ErrNotBookable
	}
	if participants > room.Capacity {
		return nil, ErrOverCapacity
	}
	start, end := req.Start.In(loc), req.End.In(loc)
	if !end.After(start) {
		return nil, ErrInvalidPeriod
	}
	w := Window(room, start)
	if start.Before(w.Start) || end.After(w.End) {
		if w.End.Sub(w.Start) == 24*time.Hour {
			return nil, ErrInvalidPeriod
		}
		return nil, ErrOutsideHours
	}
	for i := range busy {
		if Overlaps(req, busy[i].Interval) {
			return &busy[i], ErrConflict
		}
	}
	return nil, nil
}

// NearestFreeSlots proposes up to n alternative periods of the same length
// as req inside window (same day, within the room's hours), closest to the
// requested start first. One candidate per free gap: the position inside the
// gap nearest to req.Start. AC-117-01: busy 10–12, req 11–13, window 07–17
// → [12–14, 08–10]. A fully booked day returns an empty slice.
func NearestFreeSlots(busy []Interval, req Interval, window Interval, n int) []Interval {
	dur := req.End.Sub(req.Start)
	if dur <= 0 || n <= 0 {
		return []Interval{}
	}
	sorted := slices.Clone(busy)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })

	var gaps []Interval
	cursor := window.Start
	for _, b := range sorted {
		if !b.End.After(cursor) {
			continue
		}
		if b.Start.After(cursor) {
			gapEnd := b.Start
			if gapEnd.After(window.End) {
				gapEnd = window.End
			}
			if gapEnd.After(cursor) {
				gaps = append(gaps, Interval{Start: cursor, End: gapEnd})
			}
		}
		if b.End.After(cursor) {
			cursor = b.End
		}
		if !cursor.Before(window.End) {
			break
		}
	}
	if cursor.Before(window.End) {
		gaps = append(gaps, Interval{Start: cursor, End: window.End})
	}

	type cand struct {
		iv   Interval
		dist time.Duration
	}
	var cands []cand
	for _, g := range gaps {
		if g.End.Sub(g.Start) < dur {
			continue
		}
		start := req.Start
		if start.Before(g.Start) {
			start = g.Start
		}
		if latest := g.End.Add(-dur); start.After(latest) {
			start = latest
		}
		d := start.Sub(req.Start)
		if d < 0 {
			d = -d
		}
		cands = append(cands, cand{iv: Interval{Start: start, End: start.Add(dur)}, dist: d})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		return cands[i].iv.Start.Before(cands[j].iv.Start)
	})
	out := []Interval{}
	for _, c := range cands {
		if len(out) == n {
			break
		}
		out = append(out, c.iv)
	}
	return out
}

// NextDue advances a due date by count units; month steps clamp to the end
// of the target month (2026-01-31 + 1 bulan → 2026-02-28).
func NextDue(date time.Time, unit string, count int) time.Time {
	switch unit {
	case "day":
		return date.AddDate(0, 0, count)
	case "week":
		return date.AddDate(0, 0, 7*count)
	default:
		y, m, d := date.Date()
		first := time.Date(y, m+time.Month(count), 1, 0, 0, 0, 0, date.Location())
		last := first.AddDate(0, 1, -1).Day()
		if d > last {
			d = last
		}
		return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, date.Location())
	}
}

// Schedule is one preventive-maintenance plan (FR-118-003).
type Schedule struct {
	ID                    string     `json:"id"`
	RoomID                *string    `json:"room_id"`
	AssetID               *string    `json:"asset_id"`
	RoomName              *string    `json:"room_name"`
	Title                 string     `json:"title"`
	Priority              string     `json:"priority"`
	IntervalUnit          string     `json:"interval_unit"`
	IntervalCount         int        `json:"interval_count"`
	NextDueDate           string     `json:"next_due_date"`
	DefaultAssigneeUserID *string    `json:"default_assignee_user_id"`
	DefaultAssigneeName   *string    `json:"default_assignee_name"`
	Active                bool       `json:"active"`
	DeletedAt             *time.Time `json:"-"`
	SchoolID              string     `json:"-"`
	CreatedBy             string     `json:"-"`
}

// DueSchedules picks the active schedules whose next_due_date is on or
// before today. The job creates one WO for next_due_date and advances by a
// single period per run, so a schedule that missed two periods catches up
// one period per run instead of piling up WOs at once.
func DueSchedules(schedules []Schedule, today string) []Schedule {
	out := []Schedule{}
	for _, s := range schedules {
		if s.Active && s.DeletedAt == nil && s.NextDueDate <= today {
			out = append(out, s)
		}
	}
	return out
}

// ActorKind is the caller's relation to a work order.
type ActorKind string

const (
	ActorOfficer  ActorKind = "officer"  // facility_officer / admin / superadmin
	ActorAssignee ActorKind = "assignee" // teknisi yang ditugaskan
	ActorReporter ActorKind = "reporter" // pelapor
)

var woTransitions = map[string]map[string][]ActorKind{
	WODilaporkan: {
		WODitugaskan: {ActorOfficer},
		WODibatalkan: {ActorOfficer},
	},
	WODitugaskan: {
		WODitugaskan:       {ActorOfficer}, // tugaskan ulang
		WODiproses:         {ActorAssignee, ActorOfficer},
		WOMenungguMaterial: {ActorOfficer},
		WODibatalkan:       {ActorOfficer},
	},
	WODiproses: {
		WOMenungguMaterial: {ActorAssignee, ActorOfficer},
		WOSelesai:          {ActorAssignee, ActorOfficer},
		WODibatalkan:       {ActorOfficer},
	},
	WOMenungguMaterial: {
		WODiproses:   {ActorAssignee, ActorOfficer},
		WOSelesai:    {ActorAssignee, ActorOfficer},
		WODibatalkan: {ActorOfficer},
	},
	WOSelesai: {
		WODitutup: {ActorOfficer, ActorReporter},
	},
}

// CanTransition reports whether any of the caller's relations allows the
// work-order status change. Anything else is answered with 422.
func CanTransition(from, to string, actors ...ActorKind) bool {
	allowed := woTransitions[from][to]
	for _, a := range actors {
		if slices.Contains(allowed, a) {
			return true
		}
	}
	return false
}

// IsActiveWO: "active" for the internal list = not finished or cancelled.
func IsActiveWO(status string) bool {
	return status != WOSelesai && status != WODitutup && status != WODibatalkan
}

// WOTotalCost sums the cost lines (FR-118-004): 150.000 + 75.000 = 225.000.
func WOTotalCost(costs []Cost) int64 {
	var total int64
	for _, c := range costs {
		total += c.AmountIDR
	}
	return total
}

// Cost is one work-order cost line.
type Cost struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Description string    `json:"description"`
	AmountIDR   int64     `json:"amount_idr"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}
