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

	// ReportType discriminates "semester" (regular E-Rapor, Nilai Akhir
	// column) from "bayangan" (Rapot Bayangan, quarterly shadow report,
	// Nilai Sementara column) -- item 3. Both share this same table/
	// lifecycle (status/finalize/unlock/header snapshot); only the score
	// formula and which report_card_subjects columns get populated
	// differ (see handler/report_card.go's calcSubjectScores). PeriodLabel
	// (e.g. "Triwulan 1") disambiguates multiple bayangan reports within
	// the same term -- always "" for semester rows.
	ReportType  string `json:"report_type"`
	PeriodLabel string `json:"period_label,omitempty"`

	// Populated only on detail fetch (GET /report-cards/{id}).
	Subjects []ReportCardSubject `json:"subjects,omitempty"`

	// School: the printable letterhead/signature-block school identity
	// (name/address/city/province/logo), resolved live from identity-service
	// at single-card read time -- NOT cached on this row like the other
	// header-snapshot fields above. School info changes rarely and
	// GetReportCard is a one-report-card-at-a-time read (not a per-row loop
	// like Leger/generate), so a live call here doesn't multiply upstream
	// load the way caching this pattern was originally meant to avoid. Nil
	// on list endpoints (ListStudentReportCards, generate's response list)
	// -- those stay N-call-free on purpose.
	School *SchoolInfo `json:"school,omitempty"`
}

// SchoolInfo is the subset of identity-service's School record the printed
// rapor's letterhead/signature date line needs (see CRITICAL multi-tenancy
// note in the task brief this was built against: every school must see its
// own identity here, never another school's). Akreditasi is intentionally
// not included -- identity-service's schools table has no such field yet,
// and adding one is upload/data-entry plumbing out of scope for this pass.
type SchoolInfo struct {
	Name     string  `json:"name"`
	Address  *string `json:"address"`
	City     *string `json:"city"`
	Province *string `json:"province"`
	Logo     *string `json:"logo"`
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
	StsScore *float64 `json:"sts_score"`
	SasScore *float64 `json:"sas_score"`
	// PraktikScore: Rapot Bayangan's "Nilai Praktik" bucket (item 3) --
	// only ever set on report_type='bayangan' subject rows.
	PraktikScore    *float64 `json:"praktik_score"`
	SourceExamCount int      `json:"source_exam_count"`
	GradeLabel      *string  `json:"grade_label,omitempty"`
	// StsLabel/SasLabel: "Capaian STS"/"Capaian SAS" (item 2) -- the same
	// report_grade_bands mechanism as GradeLabel, applied to StsScore/
	// SasScore individually instead of only the blended final score.
	// Computed at read time (see handler.loadReportCard), not stored.
	StsLabel *string `json:"sts_label,omitempty"`
	SasLabel *string `json:"sas_label,omitempty"`
	// SubjectGroup: the print template's "A. Mata Pelajaran Umum / B.
	// Kejuruan / Pilihan / C. Muatan Lokal" grouping (see PDF template
	// notes). Resolved live from academic-service's subjects.subject_group
	// at single-card read time, same non-cached rationale as
	// ReportCard.School -- not a DB column here. Nil if academic-service
	// couldn't be reached or the school hasn't set a group for this
	// subject; the print view falls back to a flat ungrouped list in that
	// case rather than assuming every school groups its subjects this way.
	SubjectGroup *string    `json:"subject_group,omitempty"`
	Notes        *string    `json:"notes,omitempty"`
	MidNote      *string    `json:"mid_note,omitempty"`   // Deskripsi Rapor: Capaian STS
	FinalNote    *string    `json:"final_note,omitempty"` // Deskripsi Rapor: Capaian SAS
	OverriddenBy *string    `json:"overridden_by,omitempty"`
	OverriddenAt *time.Time `json:"overridden_at,omitempty"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	SyncError    *string    `json:"sync_error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
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

// AssessmentType is one admin-defined "tipe penilaian" (item 4/5): tugas,
// praktek, STS, SAS, ... Bucket says which report column it rolls up into
// (formatif/praktik/sts/sas); WeightPercent pools multiple types sharing a
// bucket into that bucket's single score. ExamLinked marks types (STS/SAS)
// whose score can auto-populate from exam-service results.
type AssessmentType struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Bucket        string    `json:"bucket"` // formatif | praktik | sts | sas
	WeightPercent float64   `json:"weight_percent"`
	ExamLinked    bool      `json:"exam_linked"`
	SortOrder     int       `json:"sort_order"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// WeightSettings is the per-school formula for Nilai Akhir (semester) and
// Nilai Sementara (Rapot Bayangan) -- the other half of item 4's "dynamic,
// admin-configurable" report structure, alongside AssessmentType.
type WeightSettings struct {
	SchoolID                string    `json:"school_id"`
	AkhirFormatifWeight     float64   `json:"akhir_formatif_weight"`
	AkhirStsWeight          float64   `json:"akhir_sts_weight"`
	AkhirSasWeight          float64   `json:"akhir_sas_weight"`
	SementaraFormatifWeight float64   `json:"sementara_formatif_weight"`
	SementaraPraktikWeight  float64   `json:"sementara_praktik_weight"`
	SementaraStsWeight      float64   `json:"sementara_sts_weight"`
	UpdatedBy               *string   `json:"updated_by,omitempty"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// DefaultWeightSettings is used when a school has no report_weight_settings
// row of its own -- same "illustrative default, not a spec number" caveat
// as DefaultGradeBands (client said "bobot bisa disesuaikan kebijakan
// sekolah" / admin-configurable, gave no universal default).
var DefaultWeightSettings = WeightSettings{
	AkhirFormatifWeight:     30,
	AkhirStsWeight:          30,
	AkhirSasWeight:          40,
	SementaraFormatifWeight: 30,
	SementaraPraktikWeight:  40,
	SementaraStsWeight:      30,
}

// WeightedPair is one (value, weight) input to WeightedAverage.
type WeightedPair struct {
	Value  *float64
	Weight float64
}

// WeightedAverage computes a normalized weighted average over the given
// pairs, skipping nil values and non-positive weights. Weights of the
// pairs actually present are normalized so a missing bucket (e.g. no STS
// exam recorded yet) doesn't drag the result toward zero -- it's simply
// excluded, and the remaining weights are rescaled to fill the gap.
// Returns nil if nothing usable was present at all.
func WeightedAverage(pairs ...WeightedPair) *float64 {
	sumWeight, sumWeighted := 0.0, 0.0
	for _, p := range pairs {
		if p.Value != nil && p.Weight > 0 {
			sumWeight += p.Weight
			sumWeighted += p.Weight * (*p.Value)
		}
	}
	if sumWeight == 0 {
		return nil
	}
	v := sumWeighted / sumWeight
	return &v
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

// ReportTemplate is one uploaded reference .xlsx's metadata (Import
// Template, Generate Rapor admin screen) -- FileData (the actual bytes) is
// deliberately not part of this struct; it's only read/written directly by
// the store layer for the download endpoint, so a template list response
// never accidentally inlines a multi-MB blob into JSON.
type ReportTemplate struct {
	ID          string    `json:"id"`
	SchoolID    string    `json:"school_id"`
	Type        string    `json:"template_type"` // tengah_semester | akhir_semester | akhir_semester_hal2
	FileName    string    `json:"file_name"`
	ContentType string    `json:"content_type"`
	FileSize    int       `json:"file_size"`
	UploadedBy  *string   `json:"uploaded_by,omitempty"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

// ReportTemplateTypes is the fixed 3-option list the Import Template popup
// offers, matching the 3 reference .xlsx files the task was built against.
var ReportTemplateTypes = []string{"tengah_semester", "akhir_semester", "akhir_semester_hal2"}

func ValidReportTemplateType(t string) bool {
	for _, v := range ReportTemplateTypes {
		if v == t {
			return true
		}
	}
	return false
}
