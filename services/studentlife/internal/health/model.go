// Package health implements Kesehatan Sekolah (PRD menu 127–129): profil
// kesehatan, kunjungan UKS, imunisasi & skrining. This file holds the pure
// rules (no I/O) so they can be table-tested.
package health

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"skci/studentlife/internal/consent"
)

// Daftar pilihan (CHECK di migrasi 0002/0003).
const (
	OutcomeBackToClass = "back_to_class"
	OutcomeSentHome    = "sent_home"
	OutcomeReferred    = "referred"

	SyncNotNeeded = "not_needed"
	SyncPending   = "pending"
	SyncDone      = "done"
	SyncSkipped   = "skipped"
	SyncFailed    = "failed"

	ChangePending    = "pending"
	ChangeApproved   = "approved"
	ChangeRejected   = "rejected"
	ChangeSuperseded = "superseded"

	KindImmunization = "immunization"
	KindScreening    = "screening"

	ProgramDraft     = "draft"
	ProgramPublished = "published"
	ProgramCompleted = "completed"
	ProgramCancelled = "cancelled"

	TargetClassroom  = "classroom"
	TargetGradeLevel = "grade_level"
	TargetStudent    = "student"

	ResultNotRecorded = "not_recorded"
	ResultDone        = "done"
	ResultAbsent      = "absent"
	ResultExcluded    = "excluded"

	// ConsentSubjectProgram is the parent_consents.subject_type for programs.
	ConsentSubjectProgram = "health_program"
	// RequestTypeProfileUpdate is the core approval request_type (§4.5).
	RequestTypeProfileUpdate = "health_profile_update"
)

var (
	Outcomes       = []string{OutcomeBackToClass, OutcomeSentHome, OutcomeReferred}
	ProgramKinds   = []string{KindImmunization, KindScreening}
	TargetKinds    = []string{TargetClassroom, TargetGradeLevel, TargetStudent}
	ResultStatuses = []string{ResultNotRecorded, ResultDone, ResultAbsent, ResultExcluded}
	BloodTypes     = []string{"A", "B", "AB", "O", "tidak_diketahui"}
	Rhesus         = []string{"", "+", "-"}
)

// ---- Profil kesehatan (menu 127) -----------------------------------------

type Allergy struct {
	Name     string `json:"name"`
	Reaction string `json:"reaction"`
	IsAlert  bool   `json:"is_alert"`
}

type Condition struct {
	Name    string `json:"name"`
	Note    string `json:"note"`
	IsAlert bool   `json:"is_alert"`
}

type Medication struct {
	Name     string `json:"name"`
	Dose     string `json:"dose"`
	Schedule string `json:"schedule"`
}

type Contact struct {
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	Phone        string `json:"phone"`
}

// Profile is the decrypted content of health_profiles.profile_enc.
type Profile struct {
	BloodType          string       `json:"blood_type"`
	Rhesus             string       `json:"rhesus"`
	Allergies          []Allergy    `json:"allergies"`
	Conditions         []Condition  `json:"conditions"`
	RoutineMedications []Medication `json:"routine_medications"`
}

// ProfileInput is the full editable payload (PUT profile / usulan wali).
type ProfileInput struct {
	Profile
	EmergencyContacts []Contact `json:"emergency_contacts"`
}

// AlertItem is one medical alert shown to the homeroom teacher.
type AlertItem struct {
	Kind   string `json:"kind"` // allergy | condition
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// Alert is the AlertOnly projection (health_profiles.alert_enc).
type Alert struct {
	Items             []AlertItem `json:"items"`
	EmergencyContacts []Contact   `json:"emergency_contacts"`
}

// AlertView keeps only allergies/conditions flagged is_alert plus emergency
// contacts (BR-HLT-01, AC-127-01).
func AlertView(p Profile, contacts []Contact) Alert {
	a := Alert{Items: []AlertItem{}, EmergencyContacts: contacts}
	if a.EmergencyContacts == nil {
		a.EmergencyContacts = []Contact{}
	}
	for _, al := range p.Allergies {
		if al.IsAlert {
			a.Items = append(a.Items, AlertItem{Kind: "allergy", Name: al.Name, Detail: al.Reaction})
		}
	}
	for _, c := range p.Conditions {
		if c.IsAlert {
			a.Items = append(a.Items, AlertItem{Kind: "condition", Name: c.Name, Detail: c.Note})
		}
	}
	return a
}

// ErrValidation wraps a user-facing validation message.
type ErrValidation struct{ Msg string }

func (e ErrValidation) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return ErrValidation{Msg: fmt.Sprintf(format, a...)} }

var phonePattern = regexp.MustCompile(`^\+?[0-9]{6,15}$`)

// NormalizePhone strips spaces, dashes, dots and parentheses.
func NormalizePhone(p string) string {
	return strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "").Replace(strings.TrimSpace(p))
}

// ValidateProfile checks FR-127-001 input and normalizes it in place.
func ValidateProfile(p *ProfileInput) error {
	if !slices.Contains(BloodTypes, p.BloodType) {
		return invalid("Golongan darah tidak valid")
	}
	if !slices.Contains(Rhesus, p.Rhesus) {
		return invalid("Rhesus tidak valid")
	}
	for i := range p.Allergies {
		if p.Allergies[i].Name = strings.TrimSpace(p.Allergies[i].Name); p.Allergies[i].Name == "" {
			return invalid("Nama alergi wajib diisi")
		}
	}
	for i := range p.Conditions {
		if p.Conditions[i].Name = strings.TrimSpace(p.Conditions[i].Name); p.Conditions[i].Name == "" {
			return invalid("Nama kondisi khusus wajib diisi")
		}
	}
	for i := range p.RoutineMedications {
		if p.RoutineMedications[i].Name = strings.TrimSpace(p.RoutineMedications[i].Name); p.RoutineMedications[i].Name == "" {
			return invalid("Nama obat rutin wajib diisi")
		}
	}
	for i := range p.EmergencyContacts {
		c := &p.EmergencyContacts[i]
		if c.Name = strings.TrimSpace(c.Name); c.Name == "" {
			return invalid("Nama kontak darurat wajib diisi")
		}
		if c.Phone = NormalizePhone(c.Phone); !phonePattern.MatchString(c.Phone) {
			return invalid("Nomor telepon kontak darurat %s tidak valid", c.Name)
		}
	}
	return nil
}

// ErrSuperseded: the profile changed after the guardian's proposal was made.
var ErrSuperseded = errors.New("profil sudah berubah sejak usulan dibuat")

// ApplyChange applies an approved proposal (FR-127-002). currentVersion is
// the stored profile version (0 when the student has no profile yet). A
// proposal built on an older version is superseded rather than overwriting
// newer data. On success it returns the new version and the re-derived alert.
func ApplyChange(currentVersion, baseVersion int, proposed ProfileInput) (int, Alert, error) {
	if baseVersion != currentVersion {
		return currentVersion, Alert{}, ErrSuperseded
	}
	return currentVersion + 1, AlertView(proposed.Profile, proposed.EmergencyContacts), nil
}

// ChangedFields lists which top-level fields differ (for audit, no values).
func ChangedFields(before, after ProfileInput) []string {
	out := []string{}
	eq := func(a, b any) bool { return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) }
	if before.BloodType != after.BloodType || before.Rhesus != after.Rhesus {
		out = append(out, "blood_type")
	}
	if !eq(before.Allergies, after.Allergies) {
		out = append(out, "allergies")
	}
	if !eq(before.Conditions, after.Conditions) {
		out = append(out, "conditions")
	}
	if !eq(before.RoutineMedications, after.RoutineMedications) {
		out = append(out, "routine_medications")
	}
	if !eq(before.EmergencyContacts, after.EmergencyContacts) {
		out = append(out, "emergency_contacts")
	}
	return out
}

// ---- Kunjungan UKS (menu 128) --------------------------------------------

// VisitDetails is the decrypted content of clinic_visits.details_enc.
type VisitDetails struct {
	Complaint   string `json:"complaint"`
	Examination string `json:"examination"`
	Action      string `json:"action"`
	Note        string `json:"note"`
}

// Effects are the side effects a visit outcome triggers.
type Effects struct {
	NotifyParent bool `json:"notify_parent"`
	MarkSick     bool `json:"mark_sick"`
}

// VisitEffects: dipulangkan/dirujuk → wali diberi tahu + presensi sakit
// (AC-128-01). Dirujuk dianggap meninggalkan sekolah (asumsi, lihat handoff).
func VisitEffects(outcome string) Effects {
	switch outcome {
	case OutcomeSentHome, OutcomeReferred:
		return Effects{NotifyParent: true, MarkSick: true}
	}
	return Effects{}
}

// SickNote is the attendance note "sejak jam tersebut" in the school's zone.
func SickNote(outcome string, since time.Time, loc *time.Location) string {
	verb := "Dipulangkan"
	if outcome == OutcomeReferred {
		verb = "Dirujuk"
	}
	return fmt.Sprintf("%s dari UKS pukul %s %s", verb, since.In(loc).Format("15.04"), zoneLabel(since.In(loc)))
}

// zoneLabel maps Indonesian offsets to WIB/WITA/WIT, else the zone abbreviation.
func zoneLabel(t time.Time) string {
	name, offset := t.Zone()
	switch offset {
	case 7 * 3600:
		return "WIB"
	case 8 * 3600:
		return "WITA"
	case 9 * 3600:
		return "WIT"
	}
	return name
}

// OutcomeLabel is the parent-facing outcome text (no diagnosis).
func OutcomeLabel(outcome string) string {
	switch outcome {
	case OutcomeSentHome:
		return "dipulangkan"
	case OutcomeReferred:
		return "dirujuk"
	}
	return "kembali ke kelas"
}

// ValidateVisit checks FR-128-001 input.
func ValidateVisit(outcome string, d VisitDetails, referredTo string) error {
	if !slices.Contains(Outcomes, outcome) {
		return invalid("Hasil kunjungan tidak valid")
	}
	if strings.TrimSpace(d.Complaint) == "" {
		return invalid("Keluhan wajib diisi")
	}
	if outcome == OutcomeReferred && strings.TrimSpace(referredTo) == "" {
		return invalid("Tujuan rujukan wajib diisi")
	}
	return nil
}

// ---- Imunisasi & skrining (menu 129) -------------------------------------

// ErrConsentMissing: AC-129-01.
var ErrConsentMissing = errors.New("Persetujuan orang tua belum tersedia")

// CanRecordResult rejects recording a result for a program that requires
// consent while the participant's consent is not granted (AC-129-01).
// consentStatus is "" when no consent row exists.
func CanRecordResult(requiresConsent bool, programStatus, consentStatus string) error {
	if programStatus != ProgramPublished {
		return invalid("Program belum dipublikasikan atau sudah ditutup")
	}
	if requiresConsent && consentStatus != consent.StatusGranted {
		return ErrConsentMissing
	}
	return nil
}

// Target is one health_program_targets row.
type Target struct {
	Kind string `json:"target_kind"`
	Ref  string `json:"target_ref"`
}

// Rosters is the academic data ResolveTargets needs.
type Rosters struct {
	ByClassroom map[string][]string // classroom_id → student ids
	GradeOf     map[string]string   // classroom_id → grade_level
}

// ResolveTargets returns the unique, sorted student set of classroom,
// grade-level and individual targets (FR-129-001).
func ResolveTargets(targets []Target, r Rosters) []string {
	seen := map[string]bool{}
	add := func(ids []string) {
		for _, id := range ids {
			if id != "" {
				seen[id] = true
			}
		}
	}
	for _, t := range targets {
		switch t.Kind {
		case TargetClassroom:
			add(r.ByClassroom[t.Ref])
		case TargetGradeLevel:
			for cl, grade := range r.GradeOf {
				if grade == t.Ref {
					add(r.ByClassroom[cl])
				}
			}
		case TargetStudent:
			add([]string{t.Ref})
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ValidateProgram checks FR-129-001 input.
func ValidateProgram(kind, name string, startsOn, endsOn time.Time, requiresConsent bool, deadline *time.Time, targets []Target) error {
	if !slices.Contains(ProgramKinds, kind) {
		return invalid("Jenis program tidak valid")
	}
	if strings.TrimSpace(name) == "" {
		return invalid("Nama program wajib diisi")
	}
	if endsOn.Before(startsOn) {
		return invalid("Tanggal selesai tidak boleh sebelum tanggal mulai")
	}
	if requiresConsent && deadline == nil {
		return invalid("Batas persetujuan wajib diisi untuk program yang memerlukan persetujuan")
	}
	if deadline != nil && deadline.After(startsOn) {
		return invalid("Batas persetujuan harus sebelum atau sama dengan tanggal mulai")
	}
	for _, t := range targets {
		if !slices.Contains(TargetKinds, t.Kind) || strings.TrimSpace(t.Ref) == "" {
			return invalid("Sasaran program tidak valid")
		}
	}
	return nil
}

// RecapInput is one participant's countable facts.
type RecapInput struct {
	ResultStatus  string
	ConsentStatus string // "" = tidak perlu / belum dibuat
	NeedsFollowUp bool
	FollowUpDone  bool
}

// Recap is the aggregate (numbers only, no identities) for FR-129-003.
type Recap struct {
	Total            int            `json:"total"`
	ByResult         map[string]int `json:"by_result"`
	ByConsent        map[string]int `json:"by_consent"`
	NeedsFollowUp    int            `json:"needs_follow_up"`
	FollowUpOpen     int            `json:"follow_up_open"`
	FollowUpFinished int            `json:"follow_up_finished"`
}

// ProgramRecap counts participants by result, consent and follow-up.
func ProgramRecap(ps []RecapInput) Recap {
	r := Recap{ByResult: map[string]int{}, ByConsent: map[string]int{consent.StatusPending: 0, consent.StatusGranted: 0, consent.StatusDeclined: 0, consent.StatusRevoked: 0}}
	for _, s := range ResultStatuses {
		r.ByResult[s] = 0
	}
	for _, p := range ps {
		r.Total++
		r.ByResult[p.ResultStatus]++
		if p.ConsentStatus != "" {
			r.ByConsent[p.ConsentStatus]++
		}
		if p.NeedsFollowUp {
			r.NeedsFollowUp++
			if p.FollowUpDone {
				r.FollowUpFinished++
			} else {
				r.FollowUpOpen++
			}
		}
	}
	return r
}
