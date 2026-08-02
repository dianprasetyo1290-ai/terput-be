package model

import "time"

type ExamType struct {
	ID              string    `json:"id"`
	SchoolID        string    `json:"school_id"`
	AcademicYearID  string    `json:"academic_year_id"`
	Name            string    `json:"name"`
	MappingRapor    string    `json:"mapping_rapor"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type QuestionFolder struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	AcademicYearID string    `json:"academic_year_id"`
	Name           string    `json:"name"`
	Description    *string   `json:"description,omitempty"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AnswerOption struct {
	ID         string  `json:"id"`
	QuestionID string  `json:"question_id"`
	Content    *string `json:"content,omitempty"`
	ImageURL   *string `json:"image_url,omitempty"`
	IsCorrect  bool    `json:"is_correct"`
	OrderIndex int     `json:"order_index"`
}

type EssayAnswerOption struct {
	ID         string `json:"id"`
	QuestionID string `json:"question_id"`
	AnswerText string `json:"answer_text"`
}

type Question struct {
	ID                   string              `json:"id"`
	SchoolID             string              `json:"school_id"`
	AcademicYearID       string              `json:"academic_year_id"`
	FolderID             string              `json:"folder_id"`
	CreatedBy            string              `json:"created_by"`
	QuestionType         string              `json:"question_type"`
	Content              string              `json:"content"`
	ImageURL             *string             `json:"image_url,omitempty"`
	NoKD                 *string             `json:"no_kd,omitempty"`
	KontenKD             *string             `json:"konten_kd,omitempty"`
	IndikatorPencapaian  *string             `json:"indikator_pencapaian,omitempty"`
	LevelKognitif        *string             `json:"level_kognitif,omitempty"`
	Poin                 float64             `json:"poin"`
	IsPublished          bool                `json:"is_published"`
	CreatedAt            time.Time           `json:"created_at"`
	UpdatedAt            time.Time           `json:"updated_at"`
	AnswerOptions        []AnswerOption      `json:"answer_options,omitempty"`
	EssayOptions         []EssayAnswerOption `json:"essay_options,omitempty"`
}

type Exam struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	AcademicYearID string     `json:"academic_year_id"`
	TermID         string     `json:"term_id"`
	ExamTypeID     string     `json:"exam_type_id"`
	Title          string     `json:"title"`
	Description    *string    `json:"description,omitempty"`
	ClassroomID    string     `json:"classroom_id"`
	SubjectID      string     `json:"subject_id"`
	TeacherID      string     `json:"teacher_id"`
	StartAt        time.Time  `json:"start_at"`
	EndAt          time.Time  `json:"end_at"`
	IsShuffled     bool       `json:"is_shuffled"`
	IsPublished    bool       `json:"is_published"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	TotalPoints    float64    `json:"total_points,omitempty"`
	QuestionCount  int        `json:"question_count,omitempty"`
	FolderID       *string    `json:"folder_id,omitempty"`
}

type ExamQuestion struct {
	ID         string    `json:"id"`
	ExamID     string    `json:"exam_id"`
	QuestionID string    `json:"question_id"`
	OrderIndex int       `json:"order_index"`
	Points     float64   `json:"points"`
	CreatedAt  time.Time `json:"created_at"`
	Question   *Question `json:"question,omitempty"`
}

// ExamToken is a student-level voucher (client spec 2026-07-24: usable
// across any exam within [ValidFrom, ValidUntil], not tied to one exam) —
// see EnterExam in store/attempts.go for how it gates exam entry.
type ExamToken struct {
	ID               string    `json:"id"`
	SchoolID         string    `json:"school_id"`
	AcademicYearID   *string   `json:"academic_year_id,omitempty"`
	UserID           string    `json:"user_id"`
	Code             string    `json:"code"`
	ValidFrom        time.Time `json:"valid_from"`
	ValidUntil       time.Time `json:"valid_until"`
	RegeneratedCount int       `json:"regenerated_count"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

const (
	StatusBelumMulai = "belum_mulai"
	StatusAktif      = "aktif"
	StatusBlokir     = "blokir"
	StatusDilarang   = "dilarang"
	StatusSelesai    = "selesai"
)

type ExamAttempt struct {
	ID               string     `json:"id"`
	SchoolID         string     `json:"school_id"`
	AcademicYearID   string     `json:"academic_year_id"`
	ExamID           string     `json:"exam_id"`
	UserID           string     `json:"user_id"`
	ClassroomID      string     `json:"classroom_id"`
	TokenID          *string    `json:"token_id,omitempty"`
	Status           string     `json:"status"`
	QuestionOrder    []string   `json:"question_order,omitempty"`
	CurrentPosition  int        `json:"current_position"`
	FocusLostCount   int        `json:"focus_lost_count"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	SubmittedAt      *time.Time `json:"submitted_at,omitempty"`
	BlockedAt        *time.Time `json:"blocked_at,omitempty"`
	BlockedReason    *string    `json:"blocked_reason,omitempty"`
	BlockedBy        *string    `json:"blocked_by,omitempty"`
	BannedAt         *time.Time `json:"banned_at,omitempty"`
	BannedReason     *string    `json:"banned_reason,omitempty"`
	BannedBy         *string    `json:"banned_by,omitempty"`
	Score            *float64   `json:"score,omitempty"`
	CorrectCount     *int       `json:"correct_count,omitempty"`
	GradedAt         *time.Time `json:"graded_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type ExamAttemptAnswer struct {
	ID               string     `json:"id"`
	AttemptID        string     `json:"attempt_id"`
	ExamQuestionID   string     `json:"exam_question_id"`
	SelectedOptionID *string    `json:"selected_option_id,omitempty"`
	EssayAnswer      *string    `json:"essay_answer,omitempty"`
	IsCorrect        *bool      `json:"is_correct,omitempty"`
	PointsAwarded    *float64   `json:"points_awarded,omitempty"`
	ManuallyGraded   bool       `json:"manually_graded"`
	AnsweredAt       time.Time  `json:"answered_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
