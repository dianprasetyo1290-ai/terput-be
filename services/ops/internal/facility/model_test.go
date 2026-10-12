package facility

import (
	"errors"
	"testing"
	"time"
)

var jkt, _ = time.LoadLocation("Asia/Jakarta")

func at(day string, hm string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", day+" "+hm, jkt)
	if err != nil {
		panic(err)
	}
	return t
}

func iv(day, from, to string) Interval { return Interval{Start: at(day, from), End: at(day, to)} }

func strp(s string) *string { return &s }

// 2026-10-12 is a Monday.
const monday = "2026-10-12"

func lab() Room {
	return Room{ID: "lab", Kind: "lab", Capacity: 40, Usage: UsageBookable, Status: RoomActive,
		AvailableFrom: strp("07:00"), AvailableTo: strp("17:00")}
}

func TestBookable(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		room Room
		want bool
	}{
		{"lab aktif bookable", lab(), true},
		{"AC-116-01 aula dalam perbaikan tidak tampil", Room{Usage: UsageBookable, Status: RoomMaintenance}, false},
		{"FR-116-002 ruang kelas penggunaan tetap tidak tampil", Room{Usage: UsageFixed, Status: RoomActive}, false},
		{"ruang nonaktif", Room{Usage: UsageBookable, Status: RoomInactive}, false},
		{"ruang terhapus", Room{Usage: UsageBookable, Status: RoomActive, DeletedAt: &now}, false},
	}
	for _, c := range cases {
		if got := Bookable(c.room); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestOverlaps(t *testing.T) {
	cases := []struct {
		a, b Interval
		want bool
	}{
		{iv(monday, "10:00", "12:00"), iv(monday, "12:00", "13:00"), false},
		{iv(monday, "10:00", "12:00"), iv(monday, "11:00", "13:00"), true},
		{iv(monday, "10:00", "12:00"), iv(monday, "09:00", "10:00"), false},
		{iv(monday, "10:00", "12:00"), iv(monday, "10:30", "11:00"), true},
		{iv(monday, "10:00", "12:00"), iv(monday, "09:00", "13:00"), true},
	}
	for i, c := range cases {
		if got := Overlaps(c.a, c.b); got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}

func TestExpandFixed(t *testing.T) {
	sch := []FixedSchedule{
		{Weekday: 1, StartsTime: "07:00:00", EndsTime: "09:00:00", Label: "Praktikum IPA", ValidFrom: "2026-07-01", ValidTo: strp("2026-12-31")},
	}
	cases := []struct {
		day  string
		want int
	}{
		{monday, 1},       // Senin dalam masa berlaku
		{"2026-10-13", 0}, // Selasa
		{"2026-06-29", 0}, // Senin sebelum valid_from
		{"2027-01-04", 0}, // Senin setelah valid_to
	}
	for _, c := range cases {
		got := ExpandFixed(sch, at(c.day, "12:00"), jkt)
		if len(got) != c.want {
			t.Fatalf("%s: got %d slots want %d", c.day, len(got), c.want)
		}
		if c.want == 1 {
			if !got[0].Start.Equal(at(c.day, "07:00")) || !got[0].End.Equal(at(c.day, "09:00")) || got[0].Kind != BusyFixed {
				t.Errorf("%s: unexpected slot %+v", c.day, got[0])
			}
		}
	}
}

func TestCheckBooking(t *testing.T) {
	busy := []BusySlot{
		{Interval: iv(monday, "10:00", "12:00"), Kind: BusyBooking, Label: "Rapat"},
		{Interval: iv(monday, "07:00", "08:00"), Kind: BusyFixed, Label: "Praktikum"},
	}
	maint := lab()
	maint.Status = RoomMaintenance
	cases := []struct {
		name         string
		room         Room
		req          Interval
		participants int
		want         error
	}{
		{"AC-117-01 lab 10-12 dipesan, minta 11-13 bentrok", lab(), iv(monday, "11:00", "13:00"), 30, ErrConflict},
		{"BR-FAC-01 peserta 41 > kapasitas 40", lab(), iv(monday, "13:00", "14:00"), 41, ErrOverCapacity},
		{"peserta tepat kapasitas boleh", lab(), iv(monday, "13:00", "14:00"), 40, nil},
		{"di luar jam pakai", lab(), iv(monday, "16:00", "18:00"), 10, ErrOutsideHours},
		{"bentrok jadwal tetap", lab(), iv(monday, "07:30", "08:30"), 10, ErrConflict},
		{"bersebelahan 12-13 tidak bentrok", lab(), iv(monday, "12:00", "13:00"), 10, nil},
		{"ruang dalam perbaikan", maint, iv(monday, "13:00", "14:00"), 10, ErrNotBookable},
		{"selesai sebelum mulai", lab(), Interval{Start: at(monday, "13:00"), End: at(monday, "12:00")}, 10, ErrInvalidPeriod},
	}
	for _, c := range cases {
		slot, err := CheckBooking(c.room, c.req, c.participants, busy, jkt)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", c.name, err, c.want)
		}
		if c.want == ErrConflict && slot == nil {
			t.Errorf("%s: conflict without blocking slot", c.name)
		}
	}
}

func TestNearestFreeSlots(t *testing.T) {
	window := iv(monday, "07:00", "17:00")
	t.Run("AC-117-01 terdekat lebih dulu", func(t *testing.T) {
		got := NearestFreeSlots([]Interval{iv(monday, "10:00", "12:00")}, iv(monday, "11:00", "13:00"), window, 2)
		want := []Interval{iv(monday, "12:00", "14:00"), iv(monday, "08:00", "10:00")}
		if len(got) != 2 {
			t.Fatalf("got %d slots: %+v", len(got), got)
		}
		for i := range want {
			if !got[i].Start.Equal(want[i].Start) || !got[i].End.Equal(want[i].End) {
				t.Errorf("slot %d: got %v-%v want %v-%v", i, got[i].Start, got[i].End, want[i].Start, want[i].End)
			}
		}
	})
	t.Run("hari penuh kosong", func(t *testing.T) {
		got := NearestFreeSlots([]Interval{iv(monday, "07:00", "12:00"), iv(monday, "11:00", "17:00")}, iv(monday, "11:00", "13:00"), window, 2)
		if len(got) != 0 {
			t.Errorf("expected none, got %+v", got)
		}
	})
	t.Run("celah terlalu sempit dilewati", func(t *testing.T) {
		busy := []Interval{iv(monday, "07:00", "10:00"), iv(monday, "11:00", "15:00")}
		got := NearestFreeSlots(busy, iv(monday, "10:00", "12:00"), window, 2)
		if len(got) != 1 || !got[0].Start.Equal(at(monday, "15:00")) {
			t.Errorf("got %+v", got)
		}
	})
}

func TestNeedsApproval(t *testing.T) {
	r := lab()
	if NeedsApproval(r) {
		t.Error("lab tanpa approval")
	}
	r.RequiresApproval = true
	if !NeedsApproval(r) {
		t.Error("aula dengan approval")
	}
}

func TestNextDue(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	cases := []struct {
		from, unit string
		count      int
		want       string
	}{
		{"2026-01-15", "month", 3, "2026-04-15"}, // AC-118-01 servis AC tiap 3 bulan
		{"2026-01-31", "month", 1, "2026-02-28"}, // clamp akhir bulan
		{"2028-01-31", "month", 1, "2028-02-29"}, // kabisat
		{"2026-11-30", "month", 3, "2027-02-28"}, // lintas tahun
		{"2026-01-15", "week", 2, "2026-01-29"},
		{"2026-12-31", "day", 1, "2027-01-01"},
	}
	for _, c := range cases {
		if got := NextDue(d(c.from), c.unit, c.count).Format("2006-01-02"); got != c.want {
			t.Errorf("%s +%d %s: got %s want %s", c.from, c.count, c.unit, got, c.want)
		}
	}
}

func TestDueSchedules(t *testing.T) {
	now := time.Now()
	in := []Schedule{
		{ID: "due-today", Active: true, NextDueDate: "2026-04-15"},
		{ID: "overdue-2-periods", Active: true, NextDueDate: "2025-10-15"},
		{ID: "future", Active: true, NextDueDate: "2026-04-16"},
		{ID: "inactive", Active: false, NextDueDate: "2026-04-01"},
		{ID: "deleted", Active: true, NextDueDate: "2026-04-01", DeletedAt: &now},
	}
	got := DueSchedules(in, "2026-04-15")
	if len(got) != 2 || got[0].ID != "due-today" || got[1].ID != "overdue-2-periods" {
		t.Errorf("got %+v", got)
	}
}

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to string
		actors   []ActorKind
		want     bool
	}{
		{WODilaporkan, WODitugaskan, []ActorKind{ActorOfficer}, true},
		{WODilaporkan, WODitugaskan, []ActorKind{ActorAssignee}, false},
		{WODitugaskan, WODiproses, []ActorKind{ActorAssignee}, true},
		{WODiproses, WOMenungguMaterial, []ActorKind{ActorAssignee}, true},
		{WOMenungguMaterial, WOSelesai, []ActorKind{ActorAssignee}, true},
		{WODiproses, WOSelesai, []ActorKind{ActorAssignee}, true},
		{WOSelesai, WODitutup, []ActorKind{ActorOfficer}, true},
		{WOSelesai, WODitutup, []ActorKind{ActorReporter}, true},
		{WOSelesai, WODitutup, []ActorKind{ActorAssignee}, false},
		{WODilaporkan, WOSelesai, []ActorKind{ActorOfficer}, false},
		{WODitutup, WODiproses, []ActorKind{ActorOfficer}, false},
		{WODiproses, WODibatalkan, []ActorKind{ActorReporter}, false},
		{WODiproses, WODibatalkan, []ActorKind{ActorOfficer}, true},
		{WODitugaskan, WODiproses, nil, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to, c.actors...); got != c.want {
			t.Errorf("%s→%s %v: got %v want %v", c.from, c.to, c.actors, got, c.want)
		}
	}
}

func TestWOTotalCost(t *testing.T) {
	cases := []struct {
		costs []Cost
		want  int64
	}{
		{[]Cost{{AmountIDR: 150000}, {AmountIDR: 75000}}, 225000},
		{nil, 0},
	}
	for _, c := range cases {
		if got := WOTotalCost(c.costs); got != c.want {
			t.Errorf("got %d want %d", got, c.want)
		}
	}
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string]Clock{"07:00": 420, "17:30:00": 1050, "00:00": 0} {
		got, err := ParseClock(in)
		if err != nil || got != want {
			t.Errorf("%s: got %d %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "25:00", "7", "aa:bb"} {
		if _, err := ParseClock(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}
