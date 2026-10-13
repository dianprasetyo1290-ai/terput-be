package pkl

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Status constants
const (
	PeriodStatusPlanning = "planning"
	PeriodStatusOngoing  = "ongoing"
	PeriodStatusClosed   = "closed"

	PlacementStatusPlanned   = "planned"
	PlacementStatusActive    = "active"
	PlacementStatusCompleted = "completed"
	PlacementStatusCancelled = "cancelled"

	AttendanceStatusHadir = "hadir"
	AttendanceStatusIzin  = "izin"
	AttendanceStatusSakit = "sakit"

	ConfirmationPending   = "pending"
	ConfirmationConfirmed = "confirmed"
	ConfirmationRejected  = "rejected"

	AssessorIndustry = "industry"
	AssessorSchool   = "school"
)

type Major struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type ClassroomMajor struct {
	ClassroomID string `json:"classroom_id"`
	SchoolID    string `json:"school_id"`
	MajorID     string `json:"major_id"`
	MajorName   string `json:"major_name,omitempty"`
	MajorCode   string `json:"major_code,omitempty"`
}

type Settings struct {
	SchoolID          string    `json:"school_id"`
	CertificatePrefix *string   `json:"certificate_prefix,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Partner struct {
	ID           string     `json:"id"`
	SchoolID     string     `json:"school_id"`
	Name         string     `json:"name"`
	Field        *string    `json:"field,omitempty"`
	Address      *string    `json:"address,omitempty"`
	ContactName  *string    `json:"contact_name,omitempty"`
	ContactPhone *string    `json:"contact_phone,omitempty"`
	ContactEmail *string    `json:"contact_email,omitempty"`
	Active       bool       `json:"active"`
	CreatedAt    time.Time  `json:"created_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
	MouStatus    string     `json:"mou_status,omitempty"` // "berlaku", "berakhir", "belum_ada"
}

type PartnerMou struct {
	ID         string    `json:"id"`
	SchoolID   string    `json:"school_id"`
	PartnerID  string    `json:"partner_id"`
	MouNumber  string    `json:"mou_number"`
	ValidFrom  time.Time `json:"valid_from"`
	ValidTo    time.Time `json:"valid_to"`
	ObjectKey  *string   `json:"object_key,omitempty"`
	FileName   *string   `json:"file_name,omitempty"`
	Mime       *string   `json:"mime,omitempty"`
	UploadedBy *string   `json:"uploaded_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Period struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	AcademicYearID *string    `json:"academic_year_id,omitempty"`
	Name           string     `json:"name"`
	StartsOn       time.Time  `json:"starts_on"`
	EndsOn         time.Time  `json:"ends_on"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
}

type Quota struct {
	ID          string    `json:"id"`
	SchoolID    string    `json:"school_id"`
	PartnerID   string    `json:"partner_id"`
	PeriodID    string    `json:"period_id"`
	MajorID     string    `json:"major_id"`
	Quota       int       `json:"quota"`
	CreatedAt   time.Time `json:"created_at"`
	PartnerName string    `json:"partner_name,omitempty"`
	MajorName   string    `json:"major_name,omitempty"`
	FilledCount int       `json:"filled_count,omitempty"`
}

type IndustryMentor struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	PartnerID     string     `json:"partner_id"`
	UserID        string     `json:"user_id"`
	FullName      string     `json:"full_name"`
	Email         string     `json:"email"`
	Phone         *string    `json:"phone,omitempty"`
	JobTitle      *string    `json:"job_title,omitempty"`
	Active        bool       `json:"active"`
	CreatedAt     time.Time  `json:"created_at"`
	DeactivatedAt *time.Time `json:"deactivated_at,omitempty"`
}

type Placement struct {
	ID                 string     `json:"id"`
	SchoolID           string     `json:"school_id"`
	PeriodID           string     `json:"period_id"`
	PartnerID          string     `json:"partner_id"`
	MajorID            string     `json:"major_id"`
	StudentID          string     `json:"student_id"`
	SchoolMentorUserID string     `json:"school_mentor_user_id"`
	IndustryMentorID   *string    `json:"industry_mentor_id,omitempty"`
	StartsOn           time.Time  `json:"starts_on"`
	EndsOn             time.Time  `json:"ends_on"`
	Status             string     `json:"status"`
	CancelReason       *string    `json:"cancel_reason,omitempty"`
	CreatedBy          *string    `json:"created_by,omitempty"`
	IdempotencyKey     *string    `json:"idempotency_key,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	StudentName        string     `json:"student_name,omitempty"`
	StudentNIS         string     `json:"student_nis,omitempty"`
	ClassroomName      string     `json:"classroom_name,omitempty"`
	PartnerName        string     `json:"partner_name,omitempty"`
	MajorName          string     `json:"major_name,omitempty"`
	SchoolMentorName   string     `json:"school_mentor_name,omitempty"`
	IndustryMentorName string     `json:"industry_mentor_name,omitempty"`
}

type Attendance struct {
	ID           string     `json:"id"`
	SchoolID     string     `json:"school_id"`
	PlacementID  string     `json:"placement_id"`
	WorkDate     time.Time  `json:"work_date"`
	Status       string     `json:"status"`
	CheckInAt    *time.Time `json:"check_in_at,omitempty"`
	CheckOutAt   *time.Time `json:"check_out_at,omitempty"`
	Note         *string    `json:"note,omitempty"`
	AttachmentKey *string   `json:"attachment_key,omitempty"`
	Confirmation string     `json:"confirmation"`
	ConfirmedBy  *string    `json:"confirmed_by,omitempty"`
	ConfirmedAt  *time.Time `json:"confirmed_at,omitempty"`
	RejectReason *string    `json:"reject_reason,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StudentName  string     `json:"student_name,omitempty"`
}

type Journal struct {
	ID          string     `json:"id"`
	SchoolID    string     `json:"school_id"`
	PlacementID string     `json:"placement_id"`
	EntryDate   time.Time  `json:"entry_date"`
	Activity    string     `json:"activity"`
	Photos      []string   `json:"photos"`
	MentorNote  *string    `json:"mentor_note,omitempty"`
	MentorNoteBy *string   `json:"mentor_note_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type FinalReport struct {
	ID          string    `json:"id"`
	SchoolID    string    `json:"school_id"`
	PlacementID string    `json:"placement_id"`
	Version     int       `json:"version"`
	ObjectKey   string    `json:"object_key"`
	FileName    *string   `json:"file_name,omitempty"`
	Mime        *string   `json:"mime,omitempty"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

type Scheme struct {
	ID               string      `json:"id"`
	SchoolID         string      `json:"school_id"`
	PeriodID         string      `json:"period_id"`
	IndustryWeightBP int         `json:"industry_weight_bp"`
	SchoolWeightBP   int         `json:"school_weight_bp"`
	CreatedAt        time.Time   `json:"created_at"`
	Components       []Component `json:"components,omitempty"`
}

type Component struct {
	ID       string `json:"id"`
	SchoolID string `json:"school_id"`
	SchemeID string `json:"scheme_id"`
	Assessor string `json:"assessor"`
	Name     string `json:"name"`
	WeightBP int    `json:"weight_bp"`
	Sort     int    `json:"sort"`
}

type Score struct {
	ID          string    `json:"id"`
	SchoolID    string    `json:"school_id"`
	PlacementID string    `json:"placement_id"`
	ComponentID string    `json:"component_id"`
	ScoreX100   int       `json:"score_x100"`
	ScoredBy    string    `json:"scored_by"`
	ScoredAt    time.Time `json:"scored_at"`
}

type Result struct {
	PlacementID  string     `json:"placement_id"`
	SchoolID     string     `json:"school_id"`
	IndustryX100 *int       `json:"industry_x100,omitempty"`
	SchoolX100   *int       `json:"school_x100,omitempty"`
	FinalX100    *int       `json:"final_x100,omitempty"`
	ComputedAt   time.Time  `json:"computed_at"`
	FinalizedBy  *string    `json:"finalized_by,omitempty"`
	FinalizedAt  *time.Time `json:"finalized_at,omitempty"`
}

type Certificate struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	PlacementID   string    `json:"placement_id"`
	CertificateNo string    `json:"certificate_no"`
	ObjectKey     string    `json:"object_key"`
	IssuedBy      string    `json:"issued_by"`
	IssuedAt      time.Time `json:"issued_at"`
}

type CheckPlacementResult struct {
	Valid        bool   `json:"valid"`
	Warning      string `json:"warning,omitempty"`
	Quota        int    `json:"quota"`
	Filled       int    `json:"filled"`
	MajorID      string `json:"major_id,omitempty"`
	MajorName    string `json:"major_name,omitempty"`
	MouNumber    string `json:"mou_number,omitempty"`
	MouExpiresOn string `json:"mou_expires_on,omitempty"`
}

type PlacementFilter struct {
	PeriodID         string
	PartnerID        string
	MajorID          string
	StudentID        string
	SchoolMentorID   string
	IndustryMentorID string
	Status           string
}

type SupervisionItem struct {
	Placement
	PendingAttendanceCount int `json:"pending_attendance_count"`
	JournalCount           int `json:"journal_count"`
}


// ---------------------------------------------------------------------------
// Pure Functions (§3)
// ---------------------------------------------------------------------------

// MouCovering checks if any MoU validly covers the entire period (starts_on to ends_on).
// Returns the active covering MoU or an error (AC-139-01).
func MouCovering(mous []PartnerMou, period Period) (*PartnerMou, error) {
	if len(mous) == 0 {
		return nil, errors.New("Mitra belum memiliki MoU yang terdaftar")
	}

	var latestEnd time.Time
	var latestMou *PartnerMou

	for i := range mous {
		m := &mous[i]
		if latestMou == nil || m.ValidTo.After(latestEnd) {
			latestEnd = m.ValidTo
			latestMou = m
		}
		// Whole period must be covered: valid_from <= period.starts_on AND valid_to >= period.ends_on
		if !m.ValidFrom.After(period.StartsOn) && !m.ValidTo.Before(period.EndsOn) {
			return m, nil
		}
	}

	return nil, fmt.Errorf("MoU mitra sudah tidak berlaku (berakhir %s)", latestEnd.Format("02 January 2006"))
}

// CanPlace validates placement quotas and MoU status (BR-PKL-01, AC-140-01).
func CanPlace(quota *Quota, filled int, mouErr error) error {
	if mouErr != nil {
		return mouErr
	}
	if quota == nil {
		return errors.New("Kuota mitra untuk jurusan ini belum diatur")
	}
	if filled >= quota.Quota {
		pName := quota.PartnerName
		if pName == "" {
			pName = "tersebut"
		}
		mName := quota.MajorName
		if mName == "" {
			mName = "tersebut"
		}
		return fmt.Errorf("Kuota mitra %s jurusan %s periode ini penuh (%d/%d)", pName, mName, filled, quota.Quota)
	}
	return nil
}

// CountFilled counts placements with status planned, active, or completed.
func CountFilled(placements []Placement) int {
	count := 0
	for _, p := range placements {
		if p.Status != PlacementStatusCancelled {
			count++
		}
	}
	return count
}

// CanRecordAttendance validates if attendance can be recorded today (FR-140-002).
func CanRecordAttendance(p Placement, today time.Time) error {
	if p.Status != PlacementStatusActive {
		return errors.New("Hanya penempatan aktif yang dapat mencatat kehadiran")
	}
	tDay := today.Format("2006-01-02")
	pStart := p.StartsOn.Format("2006-01-02")
	pEnd := p.EndsOn.Format("2006-01-02")
	if tDay < pStart || tDay > pEnd {
		return errors.New("Pencatatan kehadiran di luar rentang tanggal penempatan PKL")
	}
	return nil
}

type AttendanceRecapResult struct {
	Total      int `json:"total"`
	Hadir      int `json:"hadir"`
	Izin       int `json:"izin"`
	Sakit      int `json:"sakit"`
	Confirmed  int `json:"confirmed"`
	Pending    int `json:"pending"`
	Rejected   int `json:"rejected"`
}

// AttendanceRecap aggregates attendance statistics (FR-140-003).
func AttendanceRecap(rows []Attendance) AttendanceRecapResult {
	res := AttendanceRecapResult{Total: len(rows)}
	for _, r := range rows {
		switch r.Status {
		case AttendanceStatusHadir:
			res.Hadir++
		case AttendanceStatusIzin:
			res.Izin++
		case AttendanceStatusSakit:
			res.Sakit++
		}
		switch r.Confirmation {
		case ConfirmationConfirmed:
			res.Confirmed++
		case ConfirmationPending:
			res.Pending++
		case ConfirmationRejected:
			res.Rejected++
		}
	}
	return res
}

// ValidateScheme ensures scheme weights and component weights sum to 10000 basis points (FR-141-002).
func ValidateScheme(scheme Scheme, components []Component) error {
	if scheme.IndustryWeightBP+scheme.SchoolWeightBP != 10000 {
		return errors.New("Total bobot industri dan sekolah harus tepat 100% (10.000 basis poin)")
	}

	var indTotal, schTotal int
	for _, c := range components {
		if c.WeightBP <= 0 {
			return fmt.Errorf("Komponen %s harus memiliki bobot lebih dari 0", c.Name)
		}
		if c.Assessor == AssessorIndustry {
			indTotal += c.WeightBP
		} else if c.Assessor == AssessorSchool {
			schTotal += c.WeightBP
		} else {
			return fmt.Errorf("Assessor tidak valid: %s", c.Assessor)
		}
	}

	if scheme.IndustryWeightBP > 0 && indTotal != 10000 {
		return fmt.Errorf("Total bobot komponen industri harus 100%% (10.000 bp), saat ini: %d", indTotal)
	}
	if scheme.SchoolWeightBP > 0 && schTotal != 10000 {
		return fmt.Errorf("Total bobot komponen sekolah harus 100%% (10.000 bp), saat ini: %d", schTotal)
	}

	return nil
}

// AssessorScore computes weighted score for an assessor.
// All components for that assessor must be scored (FR-141-002).
func AssessorScore(components []Component, scores []Score, assessor string) (int, error) {
	scoreMap := make(map[string]int)
	for _, s := range scores {
		scoreMap[s.ComponentID] = s.ScoreX100
	}

	var totalWeighted int64
	var foundCount, neededCount int

	for _, c := range components {
		if c.Assessor != assessor {
			continue
		}
		neededCount++
		val, ok := scoreMap[c.ID]
		if !ok {
			assessorLabel := "industri"
			if assessor == AssessorSchool {
				assessorLabel = "sekolah"
			}
			return 0, fmt.Errorf("Penilaian %s belum lengkap", assessorLabel)
		}
		foundCount++
		totalWeighted += int64(val) * int64(c.WeightBP)
	}

	if neededCount == 0 {
		return 0, nil
	}

	// Round half-up
	score := int((totalWeighted + 5000) / 10000)
	return score, nil
}

// FinalScore calculates composite final score with half-up rounding (AC-141-01).
func FinalScore(industryX100, schoolX100 int, scheme Scheme) int {
	val := int64(industryX100)*int64(scheme.IndustryWeightBP) + int64(schoolX100)*int64(scheme.SchoolWeightBP)
	res := (val + 5000) / 10000
	return int(res)
}

// FormatScore formats an x100 score to Indonesian string, removing trailing zeroes.
// e.g. 8350 -> "83,5", 8000 -> "80", 8525 -> "85,25".
func FormatScore(x100 int) string {
	intPart := x100 / 100
	decPart := x100 % 100
	if decPart == 0 {
		return fmt.Sprintf("%d", intPart)
	}
	if decPart%10 == 0 {
		return fmt.Sprintf("%d,%d", intPart, decPart/10)
	}
	return fmt.Sprintf("%d,%02d", intPart, decPart)
}

// CanIssueCertificate verifies placement is completed and result finalized (FR-141-003).
func CanIssueCertificate(p Placement, r Result) bool {
	return p.Status == PlacementStatusCompleted && r.FinalizedAt != nil
}

// CertificateNo formats sequential certificate number.
func CertificateNo(prefix string, year int, seq int) string {
	cleanPrefix := strings.TrimSpace(prefix)
	if cleanPrefix != "" && !strings.HasSuffix(cleanPrefix, "/") && !strings.HasSuffix(cleanPrefix, "-") {
		cleanPrefix += "/"
	}
	return fmt.Sprintf("%s%d/%04d", cleanPrefix, year, seq)
}

// Scope checks if a viewer can access placement data (FR-139-003).
func Scope(viewerRole, viewerID string, p Placement, industryMentorUserID *string) bool {
	if viewerRole == "koordinator_pkl" || viewerRole == "admin" || viewerRole == "superadmin" {
		return true
	}
	if p.SchoolMentorUserID == viewerID {
		return true
	}
	if industryMentorUserID != nil && *industryMentorUserID == viewerID {
		return true
	}
	if p.StudentID == viewerID {
		return true
	}
	return false
}
