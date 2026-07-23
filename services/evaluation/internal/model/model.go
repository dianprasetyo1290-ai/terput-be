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
	Status         string                 `json:"status"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
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
)

const (
	TaskStatusPending   = "pending"
	TaskStatusSubmitted = "submitted"
)
