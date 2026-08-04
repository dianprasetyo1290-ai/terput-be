package model

import "time"

// ReportCard is the header + status envelope for one student's report card
// in one classroom for one term. Subject-level scores live in Subjects.
type ReportCard struct {
	ID               string     `json:"id"`
	SchoolID         string     `json:"school_id"`
	AcademicYearID   string     `json:"academic_year_id"`
	TermID           string     `json:"term_id"`
	ClassroomID      string     `json:"classroom_id"`
	StudentID        string     `json:"student_id"`
	Status           string     `json:"status"` // draft | finalized
	FinalizedAt      *time.Time `json:"finalized_at,omitempty"`
	FinalizedBy      *string    `json:"finalized_by,omitempty"`
	UnlockedAt       *time.Time `json:"unlocked_at,omitempty"`
	UnlockedBy       *string    `json:"unlocked_by,omitempty"`
	ClassTeacherNote *string    `json:"class_teacher_note,omitempty"`

	Attendance AttendanceSummary `json:"attendance"`

	// Header snapshot fetched from identity/academic-service.
	StudentName         *string    `json:"student_name,omitempty"`
	StudentNISN         *string    `json:"student_nisn,omitempty"`
	ClassroomName       *string    `json:"classroom_name,omitempty"`
	TermName            *string    `json:"term_name,omitempty"`
	HomeroomTeacherName *string    `json:"homeroom_teacher_name,omitempty"`
	PrincipalName       *string    `json:"principal_name,omitempty"`
	HeaderSyncedAt      *time.Time `json:"header_synced_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Populated only on detail fetch (GET /report-cards/{id}).
	Subjects []ReportCardSubject `json:"subjects,omitempty"`
}

type AttendanceSummary struct {
	Hadir           *int       `json:"hadir"`
	Sakit           *int       `json:"sakit"`
	Izin            *int       `json:"izin"`
	TanpaKeterangan *int       `json:"tanpa_keterangan"`
	SyncedAt        *time.Time `json:"synced_at,omitempty"`
	SyncError       *string    `json:"sync_error,omitempty"`
}

// ReportCardSubject is one subject row in a report card. ComputedScore is
// pulled/aggregated from exam-service; OverrideScore, if set, is what an
// admin manually adjusted the value to. FinalScore (computed at read time,
// not stored) = OverrideScore if set, else ComputedScore.
type ReportCardSubject struct {
	ID            string   `json:"id"`
	ReportCardID  string   `json:"report_card_id"`
	SchoolID      string   `json:"school_id"`
	SubjectID     string   `json:"subject_id"`
	SubjectName   *string  `json:"subject_name,omitempty"`
	SubjectCode   *string  `json:"subject_code,omitempty"`
	ComputedScore *float64 `json:"computed_score"`
	FormatifScore *float64 `json:"formatif_score"`
	OverrideScore *float64 `json:"override_score"`
	FinalScore    *float64 `json:"final_score"`
	// StsScore/SasScore: same aggregation as ComputedScore but scoped to
	// only the exams whose ExamType.mapping_rapor names that bucket (see
	// classifyMapping in handler/report_card.go) -- "Capaian STS"/"Capaian
	// SAS" data for Deskripsi Rapor, distinct from the blended
	// ComputedScore used everywhere else (leger, report card final score).
	StsScore        *float64   `json:"sts_score"`
	SasScore        *float64   `json:"sas_score"`
	SourceExamCount int        `json:"source_exam_count"`
	GradeLabel      *string    `json:"grade_label,omitempty"`
	Notes           *string    `json:"notes,omitempty"`
	MidNote         *string    `json:"mid_note,omitempty"`   // Deskripsi Rapor: Capaian STS
	FinalNote       *string    `json:"final_note,omitempty"` // Deskripsi Rapor: Capaian SAS
	OverriddenBy    *string    `json:"overridden_by,omitempty"`
	OverriddenAt    *time.Time `json:"overridden_at,omitempty"`
	LastSyncedAt    *time.Time `json:"last_synced_at,omitempty"`
	SyncError       *string    `json:"sync_error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Final resolves the display score: override wins over computed.
func (s *ReportCardSubject) Final() *float64 {
	if s.OverrideScore != nil {
		return s.OverrideScore
	}
	return s.ComputedScore
}

// GradeBand is a school-configurable (or global default) predikat threshold.
type GradeBand struct {
	ID          string  `json:"id"`
	SchoolID    *string `json:"school_id"`
	MinScore    float64 `json:"min_score"`
	MaxScore    float64 `json:"max_score"`
	Label       string  `json:"label"`
	Description *string `json:"description,omitempty"`
}

// DefaultGradeBands is used when a school has no report_grade_bands rows of
// its own. Kurikulum-Merdeka-style predikat A/B/C/D bands — a reasonable
// default, not a spec requirement (documented in README > Assumptions).
var DefaultGradeBands = []GradeBand{
	{MinScore: 90, MaxScore: 100, Label: "A"},
	{MinScore: 80, MaxScore: 89.99, Label: "B"},
	{MinScore: 70, MaxScore: 79.99, Label: "C"},
	{MinScore: 0, MaxScore: 69.99, Label: "D"},
}

func LabelFor(score float64, bands []GradeBand) string {
	for _, b := range bands {
		if score >= b.MinScore && score <= b.MaxScore {
			return b.Label
		}
	}
	return ""
}

// LegerRow is one student's row in the classroom-wide leger grid.
type LegerRow struct {
	StudentID    string             `json:"student_id"`
	StudentName  *string            `json:"student_name,omitempty"`
	StudentNISN  *string            `json:"student_nisn,omitempty"`
	ReportCardID string             `json:"report_card_id"`
	Status       string             `json:"status"`
	Subjects     map[string]float64 `json:"subjects"` // subject_id -> final score
	Attendance   AttendanceSummary  `json:"attendance"`
}

// Leger is the classroom x term rekap read-model.
type Leger struct {
	ClassroomID   string       `json:"classroom_id"`
	ClassroomName *string      `json:"classroom_name,omitempty"`
	TermID        string       `json:"term_id"`
	TermName      *string      `json:"term_name,omitempty"`
	Subjects      []SubjectRef `json:"subjects"`
	Rows          []LegerRow   `json:"rows"`
}

type SubjectRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Code string `json:"code,omitempty"`
}
