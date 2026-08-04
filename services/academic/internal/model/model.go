package model

import "time"

type Term struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	AcademicYearID string     `json:"academic_year_id"`
	Name           string     `json:"name"`
	Order          int        `json:"order"`
	StartDate      *time.Time `json:"start_date"`
	EndDate        *time.Time `json:"end_date"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Classroom struct {
	ID                    string    `json:"id"`
	SchoolID              string    `json:"school_id"`
	AcademicYearID        string    `json:"academic_year_id"`
	Name                  string    `json:"name"`
	GradeLevel            *string   `json:"grade_level"`
	HomeroomTeacherUserID *string   `json:"homeroom_teacher_user_id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type ClassEnrollment struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	AcademicYearID string    `json:"academic_year_id"`
	ClassroomID    string    `json:"classroom_id"`
	StudentUserID  string    `json:"student_user_id"`
	Status         string    `json:"status"`
	EnrolledAt     time.Time `json:"enrolled_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Subject struct {
	ID           string    `json:"id"`
	SchoolID     string    `json:"school_id"`
	Name         string    `json:"name"`
	Description  *string   `json:"description"`
	SubjectGroup *string   `json:"subject_group"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ClassroomSubject struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	AcademicYearID string    `json:"academic_year_id"`
	ClassroomID    string    `json:"classroom_id"`
	SubjectID      string    `json:"subject_id"`
	TeacherUserID  *string   `json:"teacher_user_id"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Enriched fields, populated by joins for convenience (not DB columns).
	SubjectName  *string `json:"subject_name,omitempty"`
	SubjectGroup *string `json:"subject_group,omitempty"`
}

type StudentSubjectEnrollment struct {
	ID                 string    `json:"id"`
	SchoolID           string    `json:"school_id"`
	AcademicYearID     string    `json:"academic_year_id"`
	StudentUserID      string    `json:"student_user_id"`
	ClassroomSubjectID string    `json:"classroom_subject_id"`
	IsEnrolled         bool      `json:"is_enrolled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type SchedulePeriod struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	AcademicYearID string    `json:"academic_year_id"`
	ClassroomID    string    `json:"classroom_id"`
	DayOfWeek      int       `json:"day_of_week"`
	PeriodNo       int       `json:"period_no"`
	StartTime      string    `json:"start_time"`
	EndTime        string    `json:"end_time"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Schedule struct {
	ID                 string    `json:"id"`
	SchoolID           string    `json:"school_id"`
	AcademicYearID     string    `json:"academic_year_id"`
	ClassroomID        string    `json:"classroom_id"`
	SchedulePeriodID   string    `json:"schedule_period_id"`
	ClassroomSubjectID string    `json:"classroom_subject_id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Enriched fields for read endpoints (e.g. /me/schedule).
	SubjectName   *string `json:"subject_name,omitempty"`
	SubjectID     *string `json:"subject_id,omitempty"`
	ClassroomName *string `json:"classroom_name,omitempty"`
	DayOfWeek     *int    `json:"day_of_week,omitempty"`
	PeriodNo      *int    `json:"period_no,omitempty"`
	StartTime     *string `json:"start_time,omitempty"`
	EndTime       *string `json:"end_time,omitempty"`
	TeacherUserID *string `json:"teacher_user_id,omitempty"`
}
