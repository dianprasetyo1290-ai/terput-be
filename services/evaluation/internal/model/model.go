package model

import "time"

// ---------------- RPM (lesson plans) ----------------

type LessonPlan struct {
	ID             string                 `json:"id"`
	SchoolID       string                 `json:"school_id"`
	AcademicYearID string                 `json:"academic_year_id"`
	GuruUserID     string                 `json:"guru_user_id"`
	Title          string                 `json:"title"`
	SectionA       map[string]interface{} `json:"section_a"` // A. Identitas & Karakteristik
	SectionB       map[string]interface{} `json:"section_b"` // B. Analisis Kesiapan & Desain
	SectionC       map[string]interface{} `json:"section_c"` // C. Langkah Pembelajaran
	// DimensiLulusan holds the 8 graduate-profile dimensions (see
	// DimensiLulusanFields), keyed by dimension code -> guru-authored text.
	// Populated exclusively via the Excel-template import
	// (POST /me/lesson-plans/{id}/dimensi-lulusan-import), not a form field.
	DimensiLulusan map[string]interface{} `json:"dimensi_lulusan"`
	Status         string                 `json:"status"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// DimensiLulusanField is one of the 8 "Dimensi Lulusan" (graduate-profile
// dimensions) the client's RPM template asks a guru to fill per lesson --
// Kurikulum Merdeka's 2024 successor to the older 6-dimension "Profil
// Pelajar Pancasila", per Kepmendikdasmen. Code is the JSONB key; Label is
// the Excel template's column header (and the display label).
type DimensiLulusanField struct {
	Code  string
	Label string
}

// DimensiLulusanFields is the fixed set of 8 dimensions, in template-column
// order. Not school-configurable -- the client specified these by name.
var DimensiLulusanFields = []DimensiLulusanField{
	{"keimanan_ketakwaan", "Keimanan dan Ketakwaan terhadap Tuhan YME"},
	{"kewargaan", "Kewargaan"},
	{"penalaran_kritis", "Penalaran Kritis"},
	{"kreativitas", "Kreativitas"},
	{"kolaborasi", "Kolaborasi"},
	{"kemandirian", "Kemandirian"},
	{"kesehatan", "Kesehatan"},
	{"komunikasi", "Komunikasi"},
}

// ---------------- Penilaian Guru ----------------

type Period struct {
	ID                     string    `json:"id"`
	SchoolID               string    `json:"school_id"`
	AcademicYearID         string    `json:"academic_year_id"`
	Title                  string    `json:"title"`
	Semester               string    `json:"semester"`
	PublishStartDate       string    `json:"publish_start_date"` // YYYY-MM-DD
	PublishEndDate         string    `json:"publish_end_date"`
	Published              bool      `json:"published"`
	HeadmasterUserID       *string   `json:"headmaster_user_id"`
	CurriculumVPUserID     *string   `json:"curriculum_vp_user_id"`
	StudentAffairsVPUserID *string   `json:"student_affairs_vp_user_id"`
	TargetTeacherIDs       []string  `json:"target_teacher_ids"`
	PeerEvaluateeIDs       []string  `json:"peer_evaluatee_ids"`
	PeerEvaluatorIDs       []string  `json:"peer_evaluator_ids"`
	StudentClassroomIDs    []string  `json:"student_classroom_ids"`
	LeadAssessorName       string    `json:"lead_assessor_name"`
	Status                 string    `json:"status"` // computed: Draft|Published|Sedang Berlangsung|Selesai
	CreatedBy              *string   `json:"created_by"`
	UpdatedBy              *string   `json:"updated_by"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type EvalForm struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	Code          string    `json:"code"`
	Title         string    `json:"title"`
	EvaluatorType string    `json:"evaluator_type"`
	Priority      int       `json:"priority"`
	Weight        float64   `json:"weight"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type EvalItem struct {
	ID          string            `json:"id"`
	FormID      string            `json:"form_id"`
	SchoolID    string            `json:"school_id"`
	Code        string            `json:"code"`
	Indicator   string            `json:"indicator"`
	IndicatorEn string            `json:"indicator_en"`
	Weight      float64           `json:"weight"`
	MaxScore    int               `json:"max_score"`
	ScoreLabels map[string]string `json:"score_labels"`
	SortOrder   int               `json:"sort_order"`
}

type EvalTask struct {
	ID                  string     `json:"id"`
	PeriodID            string     `json:"period_id"`
	FormID              string     `json:"form_id"`
	SchoolID            string     `json:"school_id"`
	AcademicYearID      string     `json:"academic_year_id"`
	TargetTeacherUserID string     `json:"target_teacher_user_id"`
	EvaluatorUserID     string     `json:"evaluator_user_id"`
	EvaluatorType       string     `json:"evaluator_type"`
	ClassroomID         *string    `json:"classroom_id"`
	Status              string     `json:"status"`
	SubmittedAt         *time.Time `json:"submitted_at"`
	RawScore            *float64   `json:"raw_score"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type EvalAnswer struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	ItemID    string    `json:"item_id"`
	SchoolID  string    `json:"school_id"`
	Score     int       `json:"score"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

// Status constants for periods (computed, date-driven)
const (
	StatusDraft     = "Draft"
	StatusPublished = "Published"
	StatusOngoing   = "Sedang Berlangsung"
	StatusFinished  = "Selesai"
)

// Evaluator type constants (map to form.evaluator_type / task.evaluator_type)
const (
	EvalTypeKepalaSekolah = "kepala_sekolah"
	EvalTypeWakaKurikulum = "waka_kurikulum"
	EvalTypeWakaKesiswaan = "waka_kesiswaan"
	EvalTypeSelf          = "self"
	EvalTypePeer          = "peer"
	EvalTypeSiswa         = "siswa"

	// Upward-feedback track: guru rates a school leader. Separate from the
	// 6 categories above (self 25/siswa 22/kepsek 20/waka-kurikulum 18/
	// waka-kesiswaan 10/peer 5 = 100%) -- these are NOT blended into that
	// weighted score (no room in the 100%, and it's the opposite direction:
	// teacher rating their supervisor, not the supervisor's report card).
	// Reuses the same EvalTask row shape: for these types
	// target_teacher_user_id holds the LEADER being rated and
	// evaluator_user_id holds the rating guru (swapped vs. the
	// kepala_sekolah/waka_* types above, where the leader is the evaluator).
	EvalTypeGuruToKepsek        = "guru_to_kepsek"
	EvalTypeGuruToWakaKurikulum = "guru_to_waka_kurikulum"
	EvalTypeGuruToWakaKesiswaan = "guru_to_waka_kesiswaan"
)

// IsUpwardEvalType reports whether t is one of the guru-rates-a-leader
// upward-feedback types (see EvalTypeGuruToKepsek doc comment).
func IsUpwardEvalType(t string) bool {
	return t == EvalTypeGuruToKepsek || t == EvalTypeGuruToWakaKurikulum || t == EvalTypeGuruToWakaKesiswaan
}

const (
	TaskStatusPending   = "pending"
	TaskStatusSubmitted = "submitted"
)
