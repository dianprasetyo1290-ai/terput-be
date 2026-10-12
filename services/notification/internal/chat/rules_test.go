package chat

import (
	"strings"
	"testing"
	"time"
)

// AC-148-01 and BR-COM-02.
func TestCanStartTeacherParent(t *testing.T) {
	mathTeacher := map[string]bool{"siswa-9A": true} // does not teach 9B
	cases := []struct {
		name          string
		set           map[string]bool
		student       string
		guardianChild bool
		teacherCaller bool
		ok            bool
		msg           string
	}{
		{"AC-148-01 math teacher, 9B student", mathTeacher, "siswa-9B", true, true, false, MsgTeacherNotRelated},
		{"wali kelas of the student", map[string]bool{"siswa-9B": true}, "siswa-9B", true, true, true, ""},
		{"guardian not linked to student", mathTeacher, "siswa-9A", false, true, false, MsgPeerNotGuardian},
		{"parent caller, not a guardian", mathTeacher, "siswa-9A", false, false, false, MsgGuardianNotRelated},
		{"parent caller, teacher does not teach child", mathTeacher, "siswa-9B", true, false, false, MsgPeerNotTeacher},
		{"parent caller ok", mathTeacher, "siswa-9A", true, false, true, ""},
		{"empty set fails closed", map[string]bool{}, "siswa-9A", true, true, false, MsgTeacherNotRelated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, msg := CanStartTeacherParent(c.set, c.student, c.guardianChild, c.teacherCaller)
			if ok != c.ok || msg != c.msg {
				t.Fatalf("got (%v,%q) want (%v,%q)", ok, msg, c.ok, c.msg)
			}
		})
	}
}

// AC-148-02 and FR-148-003.
func TestServiceWindow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	s := func(a, b string, days ...int) *Settings { return &Settings{Start: &a, End: &b, Days: days} }
	at := func(day, hh, mm int) time.Time { return time.Date(2026, 10, day, hh, mm, 0, 0, loc) } // 2026-10-12 = Monday
	cases := []struct {
		name    string
		set     *Settings
		now     time.Time
		outside bool
	}{
		{"AC-148-02 21:00 WIB outside 07-17", s("07:00", "17:00"), at(12, 21, 0), true},
		{"16:59 inside", s("07:00", "17:00"), at(12, 16, 59), false},
		{"17:00 is the end (exclusive)", s("07:00", "17:00"), at(12, 17, 0), true},
		{"07:00 inside", s("07:00", "17:00"), at(12, 7, 0), false},
		{"nil settings = no hours", nil, at(12, 23, 0), false},
		{"null hours = no hours", &Settings{}, at(12, 23, 0), false},
		{"DB time format HH:MM:SS", s("07:00:00", "17:00:00"), at(12, 21, 0), true},
		{"weekday-only, Sunday", s("07:00", "17:00", 1, 2, 3, 4, 5), at(18, 10, 0), true},
		{"weekday-only, Monday", s("07:00", "17:00", 1, 2, 3, 4, 5), at(12, 10, 0), false},
		{"overnight window inside", s("22:00", "06:00"), at(12, 23, 0), false},
		{"UTC instant converted to WIB", s("07:00", "17:00"), time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC), true}, // 21:00 WIB
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outside, notice := ServiceWindow(c.set, c.now, loc)
			if outside != c.outside {
				t.Fatalf("outside=%v want %v", outside, c.outside)
			}
			if outside && !strings.Contains(notice, "jam layanan") {
				t.Fatalf("notice %q lacks service hours", notice)
			}
			if !outside && notice != "" {
				t.Fatalf("inside must have no notice, got %q", notice)
			}
		})
	}
	_, notice := ServiceWindow(s("07:00", "17:00"), at(12, 21, 0), loc)
	if !strings.Contains(notice, "07:00–17:00 WIB") {
		t.Fatalf("notice = %q", notice)
	}
}
