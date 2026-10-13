package discipline

import (
	"sort"
	"time"
)

// Constants
const (
	TypeKindViolation = "violation"
	TypeKindMerit     = "merit"

	ScopeAcademicYear = "academic_year"
	ScopeAllTime     = "all_time"

	RecordStatusActive   = "active"
	RecordStatusAppealed = "appealed"
	RecordStatusAnnulled = "annulled"

	ActionStatusOpen      = "open"
	ActionStatusDone      = "done"
	ActionStatusCancelled = "cancelled"

	AppealStatusPending  = "pending"
	AppealStatusApproved = "approved"
	AppealStatusRejected = "rejected"
)

// EvidenceItem holds uploaded evidence file metadata.
type EvidenceItem struct {
	ObjectKey string `json:"object_key"`
	FileName  string `json:"file_name"`
	Mime      string `json:"mime"`
	Size      int64  `json:"size,omitempty"`
}

// Type models discipline_types row.
type Type struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	Points    int        `json:"points"`
	Active    bool       `json:"active"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Threshold models discipline_thresholds row.
type Threshold struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	Points        int        `json:"points"`
	ActionName    string     `json:"action_name"`
	NotifyParent  bool       `json:"notify_parent"`
	EffectiveFrom string     `json:"effective_from"`
	EffectiveTo   *string    `json:"effective_to,omitempty"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Settings models discipline_settings row.
type Settings struct {
	SchoolID          string    `json:"school_id"`
	AccumulationScope string    `json:"accumulation_scope"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Record models discipline_records row.
type Record struct {
	ID              string         `json:"id"`
	SchoolID        string         `json:"school_id"`
	StudentID       string         `json:"student_id"`
	StudentName     string         `json:"student_name,omitempty"`
	StudentNISN     string         `json:"student_nisn,omitempty"`
	AcademicYearID  *string        `json:"academic_year_id,omitempty"`
	TypeID          string         `json:"type_id"`
	TypeCode        string         `json:"type_code,omitempty"`
	TypeName        string         `json:"type_name,omitempty"`
	Kind            string         `json:"kind"`
	Points          int            `json:"points"`
	OccurredOn      string         `json:"occurred_on"`
	Description     string         `json:"description"`
	Evidence        []EvidenceItem `json:"evidence"`
	ReporterUserID  string         `json:"reporter_user_id"`
	ReporterName    string         `json:"reporter_name,omitempty"`
	Status          string         `json:"status"`
	AnnulledReason  *string        `json:"annulled_reason,omitempty"`
	IdempotencyKey  *string        `json:"idempotency_key,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	CanAppeal       bool           `json:"can_appeal,omitempty"`
}

// Action models discipline_actions row.
type Action struct {
	ID                  string     `json:"id"`
	SchoolID            string     `json:"school_id"`
	StudentID           string     `json:"student_id"`
	StudentName         string     `json:"student_name,omitempty"`
	AcademicYearID      *string    `json:"academic_year_id,omitempty"`
	ThresholdID         string     `json:"threshold_id"`
	ActionName          string     `json:"action_name,omitempty"`
	TriggeredByRecordID string     `json:"triggered_by_record_id"`
	PointsAtTrigger     int        `json:"points_at_trigger"`
	Status              string     `json:"status"`
	AssigneeUserID      *string    `json:"assignee_user_id,omitempty"`
	AssigneeName        string     `json:"assignee_name,omitempty"`
	Notes               *string    `json:"notes,omitempty"`
	DoneAt              *time.Time `json:"done_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ReviewNeeded        bool       `json:"review_needed,omitempty"` // true if triggering record was annulled
}

// Appeal models discipline_appeals row.
type Appeal struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	RecordID       string    `json:"record_id"`
	GuardianUserID string    `json:"guardian_user_id"`
	GuardianName   string    `json:"guardian_name,omitempty"`
	Reason         string    `json:"reason"`
	Status         string    `json:"status"`
	ApprovalID     *string   `json:"approval_id,omitempty"`
	IdempotencyKey *string   `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// StudentSummary holds total violation and merit points + active actions.
type StudentSummary struct {
	StudentID       string   `json:"student_id"`
	ViolationPoints int      `json:"violation_points"`
	MeritPoints     int      `json:"merit_points"`
	Actions         []Action `json:"actions"`
}

// Accumulate calculates total violation and merit points for records (FR-131-002).
// Violation points include status 'active' and 'appealed' (annulled are excluded).
// Merit points include status 'active' and are kept separate (they do NOT subtract from violations).
func Accumulate(records []Record, scope string, yearID *string) (int, int) {
	violationTotal := 0
	meritTotal := 0

	for _, r := range records {
		if scope == ScopeAcademicYear && yearID != nil {
			if r.AcademicYearID == nil || *r.AcademicYearID != *yearID {
				continue
			}
		}

		if r.Kind == TypeKindViolation {
			if r.Status == RecordStatusActive || r.Status == RecordStatusAppealed {
				violationTotal += r.Points
			}
		} else if r.Kind == TypeKindMerit {
			if r.Status == RecordStatusActive {
				meritTotal += r.Points
			}
		}
	}

	return violationTotal, meritTotal
}

// CrossedThresholds identifies newly crossed thresholds when points increase (AC-131-01, BR-DIS-01).
// Returns thresholds where before < t.Points <= after, sorted ascending by points,
// skipping thresholds that have already been triggered.
func CrossedThresholds(beforePoints, afterPoints int, thresholds []Threshold, triggeredIDs map[string]bool, occurredOn string) []Threshold {
	if afterPoints <= beforePoints {
		return nil
	}

	var eligible []Threshold
	for _, t := range thresholds {
		if t.DeletedAt != nil {
			continue
		}
		// Effective date check
		if occurredOn != "" {
			if t.EffectiveFrom != "" && t.EffectiveFrom > occurredOn {
				continue
			}
			if t.EffectiveTo != nil && *t.EffectiveTo != "" && *t.EffectiveTo < occurredOn {
				continue
			}
		}

		// Trigger point check
		if t.Points > beforePoints && t.Points <= afterPoints {
			if triggeredIDs != nil && triggeredIDs[t.ID] {
				continue
			}
			eligible = append(eligible, t)
		}
	}

	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Points < eligible[j].Points
	})

	return eligible
}

// CanAppeal checks if a guardian can appeal a record (FR-131-004).
// Can appeal if record is active and there is no pending appeal.
func CanAppeal(r Record, pendingAppealsCount int) bool {
	return r.Status == RecordStatusActive && pendingAppealsCount == 0
}
