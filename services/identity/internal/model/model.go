package model

import "time"

type Foundation struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Address     *string   `json:"address"`
	YearFounded *int      `json:"year_founded"`
	YearJoined  *int      `json:"year_joined"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type School struct {
	ID                    string    `json:"id"`
	FoundationID          *string   `json:"foundation_id"`
	Name                  string    `json:"name"`
	Code                  *string   `json:"code"`
	Level                 *string   `json:"level"`
	Address               *string   `json:"address"`
	Subdistrict           *string   `json:"subdistrict"`
	District              *string   `json:"district"`
	City                  *string   `json:"city"`
	Province              *string   `json:"province"`
	RegistrationNumber    *string   `json:"registration_number"`
	YearFounded           *string   `json:"year_founded"`
	PhoneNumber           *string   `json:"phone_number"`
	Email                 *string   `json:"email"`
	Website               *string   `json:"website"`
	Logo                  *string   `json:"logo"`
	GovernmentLogo        *string   `json:"government_logo"`
	IsActive              bool      `json:"is_active"`
	CurrentAcademicYearID *string   `json:"current_academic_year_id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type AcademicYear struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	Label     string    `json:"label"`
	IsCurrent bool      `json:"is_current"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type User struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Email           string    `json:"email"`
	Phone           *string   `json:"phone"`
	DefaultPassword *string   `json:"default_password,omitempty"`
	PasswordChanged bool      `json:"password_changed"`
	Photo           *string   `json:"photo"`
	BirthDate       *string   `json:"birth_date"`
	BirthPlace      *string   `json:"birth_place"`
	Address         *string   `json:"address"`
	Religion        *string   `json:"religion"`
	NISN            *string    `json:"nisn"`
	IsSuperadmin    bool       `json:"is_superadmin"`
	ManagedByHR     bool       `json:"managed_by_hr"`
	DeactivateAt    *time.Time `json:"deactivate_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// UserWithRoles is what list/detail endpoints return: a user plus its
// role assignments (system roles) so callers don't need N+1 requests.
type UserWithRoles struct {
	User
	Roles []UserSchoolRole `json:"roles"`
}

type UserSchoolRole struct {
	ID             string  `json:"id"`
	UserID         string  `json:"user_id"`
	SchoolID       *string `json:"school_id"`
	Role           string  `json:"role"`
	Status         string  `json:"status"`
	AcademicYearID *string `json:"academic_year_id"`
}

type GuardianLink struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	GuardianUserID string     `json:"guardian_user_id"`
	StudentUserID  string     `json:"student_user_id"`
	Relationship   string     `json:"relationship"`
	IsPrimary      bool       `json:"is_primary"`
	Status         string     `json:"status"`
	VerifiedAt     *time.Time `json:"verified_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type LinkedChild struct {
	GuardianLink
	Name  string  `json:"name"`
	Email string  `json:"email"`
	NISN  *string `json:"nisn"`
	Photo *string `json:"photo"`
}

// GuardianChild is the minimal student projection returned by identity's
// service-to-service guardian reads (GET /internal/guardians/{id}/children).
// Deliberately narrower than LinkedChild: a sibling service resolving a
// parent's children only needs the id/name/NISN, so the internal contract
// stays {student_user_id,name,nisn} instead of leaking link metadata or the
// student's email onto another service's wire.
type GuardianChild struct {
	StudentUserID string  `json:"student_user_id"`
	Name          string  `json:"name"`
	NISN          *string `json:"nisn"`
}

type ConversionMapping struct {
	SchoolID       string  `json:"school_id"`
	Source         string  `json:"source"`
	ConversionRef  string  `json:"conversion_ref"`
	GuardianID     *string `json:"guardian_user_id"`
	StudentID      *string `json:"student_user_id"`
	GuardianLinkID *string `json:"guardian_link_id"`
}

type Position struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	Name      string    `json:"name"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PositionAssignment struct {
	ID           string    `json:"id"`
	PositionID   string    `json:"position_id"`
	PositionName string    `json:"position_name"`
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name"`
	SchoolID     string    `json:"school_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// Announcement is Kelola Informasi's content row. Two types share one table
// (see migration 0003): "event" (Acara/Event) uses Location/URL/ThumbnailURL/
// DocumentationPhotoURLs and leaves Jenis nil; "pengumuman" uses Jenis and
// leaves the event-only columns nil. Message doubles as Konten (event) /
// Pesan (pengumuman) — same free-text semantics either way.
type Announcement struct {
	ID                     string    `json:"id"`
	SchoolID               *string   `json:"school_id"`
	Type                   string    `json:"type"`
	Title                  string    `json:"title"`
	Message                string    `json:"message"`
	TargetRoles            []string  `json:"target_roles"`
	Location               *string   `json:"location"`
	URL                    *string   `json:"url"`
	ThumbnailURL           *string   `json:"thumbnail_url"`
	DocumentationPhotoURLs []string  `json:"documentation_photo_urls"`
	Jenis                  *string   `json:"jenis"`
	Published              bool      `json:"published"`
	CreatedBy              *string   `json:"created_by"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}
