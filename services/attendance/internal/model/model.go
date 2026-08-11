package model

import "time"

// AttendanceStatus enumerates the per-student status values from the spec.
// A fresh session starts every student at BelumPresensi (not yet marked);
// the guru moves them to Hadir via "Pilih Semua (Hadir)" or to
// Sakit/Izin/TanpaKeterangan via the per-student override.
type AttendanceStatus string

const (
	StatusBelumPresensi   AttendanceStatus = "belum_presensi"
	StatusHadir           AttendanceStatus = "hadir"
	StatusSakit           AttendanceStatus = "sakit"
	StatusIzin            AttendanceStatus = "izin"
	StatusTanpaKeterangan AttendanceStatus = "tanpa_keterangan"
)

func (s AttendanceStatus) Valid() bool {
	switch s {
	case StatusBelumPresensi, StatusHadir, StatusSakit, StatusIzin, StatusTanpaKeterangan:
		return true
	}
	return false
}

// BentukKegiatan enumerates the "Agenda Pembelajaran Harian" activity
// checkboxes from the spec.
var ValidBentukKegiatan = map[string]bool{
	"teori":                   true,
	"praktikum":               true,
	"formatif":                true,
	"sumatif_tengah_semester": true,
	"sumatif_akhir_jenjang":   true,
}

// TugasJenis enumerates the "tugas" checkbox added on request of Sekolah
// GPM: whether homework/task was given for a session, and which type.
type TugasJenis string

const (
	TugasIndividu TugasJenis = "individu"
	TugasKelompok TugasJenis = "kelompok"
)

var ValidTugasJenis = map[string]bool{
	string(TugasIndividu): true,
	string(TugasKelompok): true,
}

// ClassSession is one guru check-in + daily agenda for a (schedule_id, date).
type ClassSession struct {
	ID             string  `json:"id"`
	SchoolID       string  `json:"school_id"`
	AcademicYearID string  `json:"academic_year_id"`
	TermID         *string `json:"term_id,omitempty"`
	ScheduleID     string  `json:"schedule_id"`
	ClassroomID    string  `json:"classroom_id"`
	SubjectID      *string `json:"subject_id,omitempty"`
	GuruID         string  `json:"guru_id"`
	SessionDate    string  `json:"session_date"` // YYYY-MM-DD

	CheckedInAt   *time.Time `json:"checked_in_at,omitempty"`  // jam masuk
	CheckedOutAt  *time.Time `json:"checked_out_at,omitempty"` // jam keluar
	SlotStartTime *string    `json:"slot_start_time,omitempty"` // HH:MM:SS
	SlotEndTime   *string    `json:"slot_end_time,omitempty"`

	Pertemuan       *string    `json:"pertemuan,omitempty"`
	Pembahasan      *string    `json:"pembahasan,omitempty"`
	MateriPertemuan *string    `json:"materi_pertemuan,omitempty"`
	BentukKegiatan  []string   `json:"bentuk_kegiatan"`
	AgendaFilledAt  *time.Time `json:"agenda_filled_at,omitempty"`

	// Catatan is a session-level note (client example: "ada tugas yang harus
	// diselesaikan hari itu juga") -- distinct from StudentAttendance.Catatan,
	// which is per-student.
	Catatan        *string `json:"catatan,omitempty"`
	TugasDiberikan bool    `json:"tugas_diberikan"`
	TugasJenis     *string `json:"tugas_jenis,omitempty"` // "individu" | "kelompok"

	// Pergantian guru (substitute teacher): GuruID is always the guru
	// currently covering this session; OriginalGuruID/SubstitutedAt/
	// SubstitutedBy record the handoff when one happened.
	OriginalGuruID *string    `json:"original_guru_id,omitempty"`
	SubstitutedAt  *time.Time `json:"substituted_at,omitempty"`
	SubstitutedBy  *string    `json:"substituted_by,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// enrichment, best-effort, populated from identity/academic internal calls
	GuruName      string `json:"guru_name,omitempty"`
	ClassroomName string `json:"classroom_name,omitempty"`
}

// StudentAttendance is a per-student attendance row within a ClassSession.
type StudentAttendance struct {
	ID             string    `json:"id"`
	ClassSessionID string    `json:"class_session_id"`
	SchoolID       string    `json:"school_id"`
	AcademicYearID string    `json:"academic_year_id"`
	TermID         *string   `json:"term_id,omitempty"`
	ClassroomID    string    `json:"classroom_id"`
	StudentID      string    `json:"student_id"`
	Status         string    `json:"status"`
	Catatan        *string   `json:"catatan,omitempty"`
	MarkedBy       *string   `json:"marked_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// enrichment, best-effort
	StudentName string `json:"student_name,omitempty"`
}

// AttendanceSummary is the aggregate shape used by both the report-service
// integration endpoint and the admin dashboard summary/graph endpoints.
type AttendanceSummary struct {
	Hadir           int `json:"hadir"`
	Sakit           int `json:"sakit"`
	Izin            int `json:"izin"`
	TanpaKeterangan int `json:"tanpa_keterangan"`
}

// StudentMonthlyAttendance is one row in the "Presensi Siswa" admin default
// view (client revision V4, item 1): one row per student (per classroom),
// Hadir/Sakit/Izin/Tanpa Keterangan counts aggregated for one month — no
// duplicate student rows, re-aggregates when the month filter changes.
type StudentMonthlyAttendance struct {
	StudentID       string `json:"student_id"`
	ClassroomID     string `json:"classroom_id"`
	Hadir           int    `json:"hadir"`
	Sakit           int    `json:"sakit"`
	Izin            int    `json:"izin"`
	TanpaKeterangan int    `json:"tanpa_keterangan"`
}

// MonthlyStatusCount is one bucket for the dashboard chart endpoints.
type MonthlyStatusCount struct {
	Month           string `json:"month"` // YYYY-MM
	Hadir           int    `json:"hadir"`
	Sakit           int    `json:"sakit"`
	Izin            int    `json:"izin"`
	TanpaKeterangan int    `json:"tanpa_keterangan"`
}

// ActiveTeacher is a row in the "guru sedang mengajar" dashboard monitor.
// Status is "aktif" once the guru has filled the Agenda Pembelajaran Harian
// for this session (agenda_filled_at set, i.e. "Simpan & Lanjutkan ke
// Presensi" was clicked) and "tidak_aktif" while that's still pending —
// reusing class_sessions.agenda_filled_at rather than a new flag.
type ActiveTeacher struct {
	SessionID     string `json:"session_id"`
	GuruID        string `json:"guru_id"`
	GuruName      string `json:"guru_name,omitempty"`
	ClassroomID   string `json:"classroom_id"`
	ClassroomName string `json:"classroom_name,omitempty"`
	SubjectID     string `json:"subject_id,omitempty"`
	SubjectName   string `json:"subject_name,omitempty"`
	ScheduleID    string `json:"schedule_id"`
	SlotStartTime string `json:"slot_start_time,omitempty"`
	SlotEndTime   string `json:"slot_end_time,omitempty"`
	CheckedInAt   string `json:"checked_in_at"`
	Status        string `json:"status"` // "aktif" | "tidak_aktif"
}
