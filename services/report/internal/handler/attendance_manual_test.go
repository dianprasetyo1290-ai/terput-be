package handler

import "testing"

func ip(v int) *int { return &v }

// Manual Ketidakhadiran entry (GPM & Yadika, 2026-09-24). These figures go
// straight onto a printed rapor, so a typo that slips through is a wrong
// legal document, not a cosmetic bug.
func TestValidAttendanceCounts(t *testing.T) {
	ok := []attendanceBody{
		{},                                                // all blank -> rapor prints "-"
		{Sakit: ip(0), Izin: ip(0), TanpaKeterangan: ip(0)}, // a perfect attendance record
		{Sakit: ip(3), Izin: ip(1), TanpaKeterangan: ip(12)},
		{Sakit: ip(2)}, // partial fill is fine
	}
	for _, b := range ok {
		if msg, good := validAttendanceCounts(b); !good {
			t.Errorf("%+v should be accepted, got %q", b, msg)
		}
	}

	for _, b := range []attendanceBody{
		{Sakit: ip(-1)},
		{Izin: ip(-1)},
		{TanpaKeterangan: ip(-5)},
	} {
		if _, good := validAttendanceCounts(b); good {
			t.Errorf("%+v should be rejected", b)
		}
	}

	// The message must name the offending field, and name it consistently:
	// ranging a map here would pick a random one when two are both wrong.
	msg, _ := validAttendanceCounts(attendanceBody{Izin: ip(-1), TanpaKeterangan: ip(-1)})
	for i := 0; i < 20; i++ {
		if again, _ := validAttendanceCounts(attendanceBody{Izin: ip(-1), TanpaKeterangan: ip(-1)}); again != msg {
			t.Fatalf("message must be stable, got %q then %q", msg, again)
		}
	}
	if msg != "jumlah izin tidak boleh negatif" {
		t.Errorf("want the first bad field named, got %q", msg)
	}
}
