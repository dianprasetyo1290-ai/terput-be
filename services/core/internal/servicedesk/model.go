// Package servicedesk implements menu 137-138: tickets, categories, SLA and
// escalation. This file holds the domain types and pure functions (no I/O).
package servicedesk

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Roles and feature key from T9 §2 (identity 0017). Core's common.AllRoles is
// widened by T9; these constants keep this package independent of that merge.
const (
	RoleAgent   = "servicedesk_agent"
	RoleManager = "servicedesk_manager"
	FeatureKey  = "service_desk"
	AuditModule = "service_desk"
	NoticeType  = "service_desk"
)

// Ticket statuses (CHECK on sd_tickets.status).
const (
	StatusNew        = "new"
	StatusAssigned   = "assigned"
	StatusInProgress = "in_progress"
	StatusResolved   = "resolved"
	StatusClosed     = "closed"
	StatusRejected   = "rejected"
)

// Reporter kinds (sd_categories.reporter_kinds).
const (
	KindParent   = "orang_tua"
	KindStudent  = "siswa"
	KindEmployee = "pegawai"
)

// Channels (sd_tickets.channel).
const (
	ChannelParent   = "parent_portal"
	ChannelStudent  = "student_portal"
	ChannelEmployee = "employee_portal"
	ChannelAgent    = "agent"
)

// SLA units and states.
const (
	UnitBusinessHours = "business_hours"
	UnitBusinessDays  = "business_days"

	SLAOk       = "ok"
	SLAWarning  = "warning"
	SLABreached = "breached"

	TriggerWarning = "warning"
	TriggerBreach  = "breach"

	TargetTeamManager = "team_manager"
	TargetRole        = "role"
	TargetUser        = "user"
)

var (
	Priorities    = []string{"low", "normal", "high", "urgent"}
	ReporterKinds = []string{KindParent, KindStudent, KindEmployee}
	FieldTypes    = []string{"text", "textarea", "select", "date", "number"}
	Units         = []string{UnitBusinessHours, UnitBusinessDays}
	Triggers      = []string{TriggerWarning, TriggerBreach}
	TargetKinds   = []string{TargetTeamManager, TargetRole, TargetUser}

	ErrInvalidTransition = errors.New("perubahan status tiket tidak diizinkan")
	ErrNoteRequired      = errors.New("catatan wajib diisi saat menolak tiket")
	ErrNoBusinessHours   = errors.New("jam layanan belum dikonfigurasi")
	ErrCannotRate        = errors.New("tiket belum dapat dinilai")
)

// FormField is one entry of sd_categories.form_schema (FR-137-001).
type FormField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

type Attachment struct {
	Name     string `json:"name"`
	Key      string `json:"key"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type Team struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	Name          string    `json:"name"`
	ManagerUserID *string   `json:"manager_user_id"`
	MemberIDs     []string  `json:"member_ids"`
	CreatedAt     time.Time `json:"created_at"`
}

type Category struct {
	ID                       string      `json:"id"`
	SchoolID                 string      `json:"school_id"`
	Name                     string      `json:"name"`
	Description              string      `json:"description"`
	TeamID                   string      `json:"team_id"`
	TeamName                 string      `json:"team_name,omitempty"`
	IsFacility               bool        `json:"is_facility"`
	FormSchema               []FormField `json:"form_schema"`
	ReporterKinds            []string    `json:"reporter_kinds"`
	ServiceRequestCategoryID *string     `json:"service_request_category_id"`
	IsFallback               bool        `json:"is_fallback"`
	Active                   bool        `json:"active"`
}

type EscalationRule struct {
	ID           string  `json:"id,omitempty"`
	Trigger      string  `json:"trigger"`
	Level        int     `json:"level"`
	TargetKind   string  `json:"target_kind"`
	TargetRole   *string `json:"target_role,omitempty"`
	TargetUserID *string `json:"target_user_id,omitempty"`
}

type Policy struct {
	ID              string           `json:"id"`
	SchoolID        string           `json:"school_id"`
	CategoryID      *string          `json:"category_id"`
	Priority        string           `json:"priority"`
	ResponseValue   int              `json:"response_value"`
	ResponseUnit    string           `json:"response_unit"`
	ResolutionValue int              `json:"resolution_value"`
	ResolutionUnit  string           `json:"resolution_unit"`
	WarnBp          int              `json:"warn_bp"`
	EffectiveFrom   string           `json:"effective_from"` // YYYY-MM-DD
	EffectiveTo     *string          `json:"effective_to"`
	Rules           []EscalationRule `json:"escalation_rules"`
}

type BusinessHour struct {
	Weekday  int    `json:"weekday"`   // 1 = Senin ... 7 = Minggu
	OpensAt  string `json:"opens_at"`  // HH:MM
	ClosesAt string `json:"closes_at"` // HH:MM
}

type Holiday struct {
	ID   string `json:"id"`
	Date string `json:"date"`
	Name string `json:"name"`
}

type Ticket struct {
	ID                   string          `json:"id"`
	SchoolID             string          `json:"school_id"`
	Number               string          `json:"number"`
	CategoryID           string          `json:"category_id"`
	CategoryName         string          `json:"category_name"`
	IsFacility           bool            `json:"is_facility"`
	TeamID               string          `json:"team_id"`
	TeamName             string          `json:"team_name"`
	Priority             string          `json:"priority"`
	Subject              string          `json:"subject"`
	Description          string          `json:"description"`
	FormAnswers          json.RawMessage `json:"form_answers"`
	Channel              string          `json:"channel"`
	ReporterUserID       string          `json:"reporter_user_id"`
	ReporterName         string          `json:"reporter_name,omitempty"`
	StudentUserID        *string         `json:"student_user_id"`
	ServiceRequestID     *string         `json:"service_request_id"`
	Status               string          `json:"status"`
	AssigneeUserID       *string         `json:"assignee_user_id"`
	AssigneeName         string          `json:"assignee_name,omitempty"`
	SLA                  *Policy         `json:"sla_snapshot"`
	ResponseDueAt        *time.Time      `json:"response_due_at"`
	ResolutionDueAt      *time.Time      `json:"resolution_due_at"`
	FirstRespondedAt     *time.Time      `json:"first_responded_at"`
	ResolvedAt           *time.Time      `json:"resolved_at"`
	ClosedAt             *time.Time      `json:"closed_at"`
	ResponseBreachedAt   *time.Time      `json:"response_breached_at"`
	ResolutionBreachedAt *time.Time      `json:"resolution_breached_at"`
	EscalationLevel      int             `json:"escalation_level"`
	WorkOrderID          *string         `json:"work_order_id"`
	WorkOrderNumber      *string         `json:"work_order_number"`
	SatisfactionScore    *int            `json:"satisfaction_score"`
	SatisfactionComment  *string         `json:"satisfaction_comment"`
	RatedAt              *time.Time      `json:"rated_at"`
	SLAState             string          `json:"sla_state"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type Event struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"`
	FromStatus  *string      `json:"from_status"`
	ToStatus    *string      `json:"to_status"`
	Body        *string      `json:"body"`
	Internal    bool         `json:"internal"`
	Attachments []Attachment `json:"attachments"`
	ActorUserID *string      `json:"actor_user_id"`
	ActorName   string       `json:"actor_name,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Status workflow (FR-137-003)
// ---------------------------------------------------------------------------

var transitions = map[string][]string{
	StatusNew:        {StatusAssigned, StatusRejected},
	StatusAssigned:   {StatusInProgress, StatusRejected},
	StatusInProgress: {StatusResolved, StatusRejected},
	StatusResolved:   {StatusClosed, StatusInProgress}, // in_progress = dibuka ulang pelapor
}

// ValidTransition checks a ticket status change; rejecting needs a note.
func ValidTransition(from, to, note string) error {
	if !slices.Contains(transitions[from], to) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
	}
	if to == StatusRejected && strings.TrimSpace(note) == "" {
		return ErrNoteRequired
	}
	return nil
}

// IsOpen reports whether the SLA clock still runs for a status.
func IsOpen(status string) bool {
	return status == StatusNew || status == StatusAssigned || status == StatusInProgress
}

// ToServiceRequestStatus maps a ticket status onto the menu 81 status (BR-SD-01).
func ToServiceRequestStatus(ticketStatus string) string {
	switch ticketStatus {
	case StatusNew:
		return "submitted"
	case StatusAssigned:
		return "accepted"
	case StatusInProgress:
		return "in_progress"
	case StatusResolved, StatusClosed:
		return "done"
	case StatusRejected:
		return "rejected"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Business calendar & SLA (FR-138-001, AC-138-01)
// ---------------------------------------------------------------------------

// Window is one day's opening interval in minutes after midnight.
type Window struct{ Open, Close int }

// Calendar is a school's service hours in its own time zone.
type Calendar struct {
	Loc      *time.Location
	Hours    map[time.Weekday]Window
	Holidays map[string]bool // "YYYY-MM-DD"
}

// ISOWeekday converts 1 (Senin) ... 7 (Minggu) to time.Weekday.
func ISOWeekday(d int) time.Weekday { return time.Weekday(d % 7) }

// ParseClock parses "HH:MM" (or "HH:MM:SS") into minutes after midnight.
func ParseClock(s string) (int, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return 0, fmt.Errorf("format jam tidak valid: %q", s)
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("format jam tidak valid: %q", s)
	}
	return h*60 + m, nil
}

// window returns the open interval of the day containing t, if it is open.
func (c Calendar) window(t time.Time) (Window, bool) {
	if c.Holidays[t.Format("2006-01-02")] {
		return Window{}, false
	}
	w, ok := c.Hours[t.Weekday()]
	return w, ok
}

func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func at(day time.Time, minute int) time.Time {
	return midnight(day).Add(time.Duration(minute) * time.Minute)
}

// maxScanDays bounds every calendar walk (a calendar whose open days are all
// holidays for longer than this is treated as unconfigured).
const maxScanDays = 3700

// normalize moves t forward to the first instant the desk is open.
func (c Calendar) normalize(t time.Time) (time.Time, error) {
	t = t.In(c.Loc)
	for i := 0; i < maxScanDays; i++ {
		day := midnight(t).AddDate(0, 0, i)
		w, ok := c.window(day)
		if !ok {
			continue
		}
		if i == 0 {
			if t.Before(at(day, w.Open)) {
				return at(day, w.Open), nil
			}
			if t.Before(at(day, w.Close)) {
				return t, nil
			}
			continue
		}
		return at(day, w.Open), nil
	}
	return time.Time{}, ErrNoBusinessHours
}

// DueAt adds value business hours/days to start. Business days keep the
// (normalized) time of day and skip closed days and holidays; business hours
// only count minutes inside opening windows.
func DueAt(start time.Time, value int, unit string, cal Calendar) (time.Time, error) {
	cur, err := cal.normalize(start)
	if err != nil {
		return time.Time{}, err
	}
	switch unit {
	case UnitBusinessDays:
		minuteOfDay := cur.Hour()*60 + cur.Minute()
		day := midnight(cur)
		for n, i := 0, 0; n < value; i++ {
			if i > maxScanDays {
				return time.Time{}, ErrNoBusinessHours
			}
			day = day.AddDate(0, 0, 1)
			if _, ok := cal.window(day); ok {
				n++
			}
		}
		w, _ := cal.window(day)
		m := min(max(minuteOfDay, w.Open), w.Close)
		return at(day, m).Add(time.Duration(cur.Second()) * time.Second), nil
	case UnitBusinessHours:
		remaining := time.Duration(value) * time.Hour
		for i := 0; i < maxScanDays; i++ {
			w, ok := cal.window(cur)
			if ok {
				closeAt := at(cur, w.Close)
				if avail := closeAt.Sub(cur); avail > 0 {
					if remaining <= avail {
						return cur.Add(remaining), nil
					}
					remaining -= avail
				}
			}
			next, err := cal.normalize(midnight(cur).AddDate(0, 0, 1))
			if err != nil {
				return time.Time{}, err
			}
			cur = next
		}
		return time.Time{}, ErrNoBusinessHours
	}
	return time.Time{}, fmt.Errorf("satuan SLA tidak dikenal: %q", unit)
}

// BusinessDuration counts opening-window time between a and b.
func BusinessDuration(a, b time.Time, cal Calendar) time.Duration {
	a, b = a.In(cal.Loc), b.In(cal.Loc)
	if !b.After(a) {
		return 0
	}
	var total time.Duration
	for day := midnight(a); day.Before(b); day = day.AddDate(0, 0, 1) {
		w, ok := cal.window(day)
		if !ok {
			continue
		}
		lo, hi := at(day, w.Open), at(day, w.Close)
		if a.After(lo) {
			lo = a
		}
		if b.Before(hi) {
			hi = b
		}
		if hi.After(lo) {
			total += hi.Sub(lo)
		}
	}
	return total
}

// SLAState: breached once now passes dueAt; warning once the business time
// used reaches warnBp (basis points) of the business time allowed.
func SLAState(dueAt, start, now time.Time, warnBp int, cal Calendar) string {
	if now.After(dueAt) {
		return SLABreached
	}
	allowed := BusinessDuration(start, dueAt, cal)
	if allowed <= 0 {
		return SLAOk
	}
	used := BusinessDuration(start, now, cal)
	if int64(used)*10000 >= int64(allowed)*int64(warnBp) {
		return SLAWarning
	}
	return SLAOk
}

// WorstState returns the more severe of two SLA states.
func WorstState(a, b string) string {
	rank := map[string]int{"": 0, SLAOk: 1, SLAWarning: 2, SLABreached: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// ResolvePolicy picks the policy for a category+priority effective on onDate
// (YYYY-MM-DD): a category policy beats a general one (category_id NULL).
// No match → nil (ticket is created without SLA).
func ResolvePolicy(policies []Policy, categoryID, priority, onDate string) *Policy {
	var general *Policy
	for i := range policies {
		p := &policies[i]
		if p.Priority != priority || onDate < p.EffectiveFrom || (p.EffectiveTo != nil && onDate > *p.EffectiveTo) {
			continue
		}
		if p.CategoryID != nil && *p.CategoryID == categoryID {
			return p
		}
		if p.CategoryID == nil && general == nil {
			general = p
		}
	}
	return general
}

// NextEscalations returns the rules to fire now. warning → the lowest
// pending warning rule; breached → every pending breach rule. A rule is
// pending when its level is above the ticket's escalation_level.
func NextEscalations(escalationLevel int, state string, rules []EscalationRule) []EscalationRule {
	var out []EscalationRule
	switch state {
	case SLAWarning:
		var next *EscalationRule
		for i := range rules {
			r := rules[i]
			if r.Trigger == TriggerWarning && r.Level > escalationLevel && (next == nil || r.Level < next.Level) {
				next = &r
			}
		}
		if next != nil {
			out = append(out, *next)
		}
	case SLABreached:
		for _, r := range rules {
			if r.Trigger == TriggerBreach && r.Level > escalationLevel {
				out = append(out, r)
			}
		}
		slices.SortFunc(out, func(a, b EscalationRule) int { return a.Level - b.Level })
	}
	return out
}

// ---------------------------------------------------------------------------
// Forms, reporters, rating, reporting
// ---------------------------------------------------------------------------

// ValidateSchema checks a category form definition.
func ValidateSchema(schema []FormField) error {
	seen := map[string]bool{}
	for _, f := range schema {
		if strings.TrimSpace(f.Key) == "" || strings.TrimSpace(f.Label) == "" {
			return errors.New("setiap field formulir wajib punya key dan label")
		}
		if seen[f.Key] {
			return fmt.Errorf("key field %q ganda", f.Key)
		}
		seen[f.Key] = true
		if !slices.Contains(FieldTypes, f.Type) {
			return fmt.Errorf("tipe field %q tidak dikenal", f.Type)
		}
		if f.Type == "select" && len(f.Options) == 0 {
			return fmt.Errorf("field pilihan %q wajib punya opsi", f.Key)
		}
	}
	return nil
}

// ValidateForm checks answers against a category form; returns field → message.
func ValidateForm(schema []FormField, answers map[string]any) map[string]string {
	errs := map[string]string{}
	for _, f := range schema {
		raw, present := answers[f.Key]
		s := ""
		if present && raw != nil {
			s = strings.TrimSpace(fmt.Sprint(raw))
		}
		if s == "" {
			if f.Required {
				errs[f.Key] = f.Label + " wajib diisi"
			}
			continue
		}
		switch f.Type {
		case "number":
			if _, ok := raw.(float64); !ok {
				if _, err := strconv.ParseFloat(s, 64); err != nil {
					errs[f.Key] = f.Label + " harus berupa angka"
				}
			}
		case "date":
			if _, err := time.Parse("2006-01-02", s); err != nil {
				errs[f.Key] = f.Label + " harus berupa tanggal (YYYY-MM-DD)"
			}
		case "select":
			if !slices.Contains(f.Options, s) {
				errs[f.Key] = f.Label + " berisi pilihan yang tidak tersedia"
			}
		}
	}
	return errs
}

// ReporterKindForRole maps the caller's active role to a reporter kind.
func ReporterKindForRole(role string) string {
	switch role {
	case KindParent:
		return KindParent
	case KindStudent:
		return KindStudent
	}
	return KindEmployee
}

// ChannelForKind is the intake channel of a self-service reporter.
func ChannelForKind(kind string) string {
	switch kind {
	case KindParent:
		return ChannelParent
	case KindStudent:
		return ChannelStudent
	}
	return ChannelEmployee
}

// CanReport reports whether a reporter kind may file tickets in a category (FR-137-002).
func CanReport(cat Category, reporterKind string) bool {
	return cat.Active && slices.Contains(cat.ReporterKinds, reporterKind)
}

// CanRate: only the reporter, after resolution, once (FR-137-005).
func CanRate(t Ticket, userID string) error {
	if t.ReporterUserID != userID || (t.Status != StatusResolved && t.Status != StatusClosed) || t.RatedAt != nil {
		return ErrCannotRate
	}
	return nil
}

// ComplianceBp is met/total in basis points (rounded down); nil when total is 0.
func ComplianceBp(met, total int) *int {
	if total <= 0 {
		return nil
	}
	v := met * 10000 / total
	return &v
}

// TicketNumber formats SD-<year>-<6-digit sequence>.
func TicketNumber(year, seq int) string { return fmt.Sprintf("SD-%d-%06d", year, seq) }

// WorkOrderPriority maps a ticket priority to the T9 work-order priority enum
// (ops work_orders.priority CHECK IN ('rendah','sedang','tinggi','darurat')).
func WorkOrderPriority(p string) string {
	switch p {
	case "low":
		return "rendah"
	case "high":
		return "tinggi"
	case "urgent":
		return "darurat"
	}
	return "sedang"
}
