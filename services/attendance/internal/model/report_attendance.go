package model

// Attendance Report (PRD menu 43, FR-43-001..003) response shapes. Kept in
// their own file so the report work never collides with edits to model.go.

// AttendanceCounts is the per-status tally behind every recap row. Terlambat
// is always present as a key (0 until the 'terlambat' status exists in the
// student_attendances CHECK constraint). BelumPresensi (unmarked rows seeded
// at check-in) is reported for transparency but excluded from Total, so it
// never drags percentages down.
type AttendanceCounts struct {
	Hadir           int `json:"hadir"`
	Terlambat       int `json:"terlambat"`
	Sakit           int `json:"sakit"`
	Izin            int `json:"izin"`
	TanpaKeterangan int `json:"tanpa_keterangan"`
	BelumPresensi   int `json:"belum_presensi"`
	Total           int `json:"total"`
}

// Add accumulates o into c (Total included).
func (c *AttendanceCounts) Add(o AttendanceCounts) {
	c.Hadir += o.Hadir
	c.Terlambat += o.Terlambat
	c.Sakit += o.Sakit
	c.Izin += o.Izin
	c.TanpaKeterangan += o.TanpaKeterangan
	c.BelumPresensi += o.BelumPresensi
	c.Total += o.Total
}

// AttendancePercentages are shares of AttendanceCounts.Total, rounded to one
// decimal. Kehadiran = hadir + terlambat (the student was present);
// TidakHadir = sakit + izin + tanpa_keterangan.
type AttendancePercentages struct {
	Hadir           float64 `json:"hadir"`
	Terlambat       float64 `json:"terlambat"`
	Sakit           float64 `json:"sakit"`
	Izin            float64 `json:"izin"`
	TanpaKeterangan float64 `json:"tanpa_keterangan"`
	Kehadiran       float64 `json:"kehadiran"`
	TidakHadir      float64 `json:"tidak_hadir"`
}

// AttendanceRecapRow is one line of a recap: a student, a class, a unit
// (school), or a subject (optionally narrowed to a class/student). Names are
// resolved server-side; IDs are kept for drill-down links.
type AttendanceRecapRow struct {
	Scope         string                `json:"scope"` // student | class | unit | subject
	SchoolID      string                `json:"school_id,omitempty"`
	SchoolName    string                `json:"school_name,omitempty"`
	ClassroomID   string                `json:"classroom_id,omitempty"`
	ClassroomName string                `json:"classroom_name,omitempty"`
	StudentID     string                `json:"student_id,omitempty"`
	StudentName   string                `json:"student_name,omitempty"`
	NISN          string                `json:"nisn,omitempty"`
	SubjectID     string                `json:"subject_id,omitempty"`
	SubjectName   string                `json:"subject_name,omitempty"`
	Students      int                   `json:"students,omitempty"` // distinct students (class/unit/subject rows)
	Sessions      int                   `json:"sessions,omitempty"` // subject meetings (subject rows)
	Counts        AttendanceCounts      `json:"counts"`
	Percent       AttendancePercentages `json:"percent"`
	Rank          int                   `json:"rank,omitempty"`         // class comparison only
	DeltaVsAvg    *float64              `json:"delta_vs_avg,omitempty"` // class comparison: kehadiran pct minus overall
}

// AttendanceTrendPoint is one bucket (day / week / month) of a trend series.
type AttendanceTrendPoint struct {
	Period  string                `json:"period"` // bucket start, YYYY-MM-DD
	Counts  AttendanceCounts      `json:"counts"`
	Percent AttendancePercentages `json:"percent"`
}
