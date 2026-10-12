// Package chat holds the pure rules of the teacher–parent chat (PRD menu
// 148): who may start a conversation (BR-COM-02) and the service-hours
// notice (FR-148-003). No I/O; table-tested.
package chat

import (
	"fmt"
	"strings"
	"time"
)

// Rejection messages shown to users.
const (
	MsgTeacherNotRelated  = "Anda tidak mengajar atau mewalikan siswa ini"
	MsgGuardianNotRelated = "Anda bukan wali dari siswa ini"
	MsgPeerNotTeacher     = "Guru tersebut tidak mengajar atau mewalikan siswa ini"
	MsgPeerNotGuardian    = "Orang tua tersebut bukan wali dari siswa ini"
)

// CanStartTeacherParent applies BR-COM-02 for one (teacher, guardian,
// student) trio: the student must be in the set of students the teacher
// teaches or is wali kelas of, and the guardian must be an active guardian
// of that student. callerIsTeacher picks which side's message is returned.
func CanStartTeacherParent(teacherStudents map[string]bool, studentID string, guardianHasChild bool, callerIsTeacher bool) (bool, string) {
	if !teacherStudents[studentID] {
		if callerIsTeacher {
			return false, MsgTeacherNotRelated
		}
		return false, MsgPeerNotTeacher
	}
	if !guardianHasChild {
		if callerIsTeacher {
			return false, MsgPeerNotGuardian
		}
		return false, MsgGuardianNotRelated
	}
	return true, ""
}

// Settings is a school's service-hours configuration. Start/End are "HH:MM"
// (nil = no service hours); Days uses 1=Monday..7=Sunday (empty = every day).
type Settings struct {
	Start *string
	End   *string
	Days  []int
}

var dayNames = map[int]string{1: "Senin", 2: "Selasa", 3: "Rabu", 4: "Kamis", 5: "Jumat", 6: "Sabtu", 7: "Minggu"}

// ServiceWindow reports whether now (evaluated in loc) is outside the
// configured service hours, with the notice to show the sender. A message
// outside hours is still delivered; only the notice is added.
func ServiceWindow(s *Settings, now time.Time, loc *time.Location) (bool, string) {
	if s == nil || s.Start == nil || s.End == nil {
		return false, ""
	}
	start, err1 := minutes(*s.Start)
	end, err2 := minutes(*s.End)
	if err1 != nil || err2 != nil || start == end {
		return false, ""
	}
	local := now.In(loc)
	wd := int(local.Weekday())
	if wd == 0 {
		wd = 7
	}
	cur := local.Hour()*60 + local.Minute()
	dayOK := len(s.Days) == 0
	for _, d := range s.Days {
		if d == wd {
			dayOK = true
		}
	}
	var inside bool
	if start < end {
		inside = cur >= start && cur < end
	} else { // window crosses midnight
		inside = cur >= start || cur < end
	}
	if dayOK && inside {
		return false, ""
	}
	return true, fmt.Sprintf("Pesan Anda terkirim di luar jam layanan (%s %s–%s %s). Balasan akan diberikan pada jam layanan.",
		daysLabel(s.Days), clock(*s.Start), clock(*s.End), zoneLabel(loc))
}

func minutes(v string) (int, error) {
	t, err := time.Parse("15:04", clock(v))
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

// clock trims a "HH:MM:SS" DB time to "HH:MM".
func clock(v string) string {
	if len(v) >= 5 {
		return v[:5]
	}
	return v
}

func daysLabel(days []int) string {
	if len(days) == 0 {
		return "setiap hari"
	}
	names := make([]string, 0, len(days))
	for _, d := range days {
		if n, ok := dayNames[d]; ok {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}

func zoneLabel(loc *time.Location) string {
	switch loc.String() {
	case "Asia/Jakarta":
		return "WIB"
	case "Asia/Makassar":
		return "WITA"
	case "Asia/Jayapura":
		return "WIT"
	}
	return loc.String()
}
