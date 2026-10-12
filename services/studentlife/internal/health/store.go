package health

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"skci/studentlife/internal/common"
	"skci/studentlife/internal/store"
)

// Table names double as the AAD prefix of their *_enc columns.
const (
	tblProfiles     = "health_profiles"
	tblChanges      = "health_profile_changes"
	tblVisits       = "clinic_visits"
	tblAddenda      = "clinic_visit_addenda"
	tblParticipants = "health_program_participants"
)

// NewID asks Postgres for a uuid so ciphertext can be bound to the row id
// before the INSERT (AAD "<tabel>:<school_id>:<id>").
func NewID(ctx context.Context, q store.Q) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT gen_random_uuid()::text`).Scan(&id)
	return id, err
}

// ---- profiles --------------------------------------------------------------

type ProfileRow struct {
	ID           string
	SchoolID     string
	StudentID    string
	ProfileEnc   []byte
	AlertEnc     []byte
	EmergencyEnc []byte
	VerifiedBy   *string
	VerifiedAt   *time.Time
	Version      int
	UpdatedAt    time.Time
}

const profileCols = `id, school_id, student_id, profile_enc, alert_enc, emergency_enc, verified_by, verified_at, version, updated_at`

func scanProfile(r interface{ Scan(...any) error }) (ProfileRow, error) {
	var p ProfileRow
	err := r.Scan(&p.ID, &p.SchoolID, &p.StudentID, &p.ProfileEnc, &p.AlertEnc, &p.EmergencyEnc, &p.VerifiedBy, &p.VerifiedAt, &p.Version, &p.UpdatedAt)
	return p, err
}

// GetProfile reads the student's profile (store.ErrNotFound when none).
func GetProfile(ctx context.Context, q store.Q, schoolID, studentID string, forUpdate bool) (ProfileRow, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE"
	}
	p, err := scanProfile(q.QueryRowContext(ctx, `SELECT `+profileCols+` FROM health_profiles WHERE school_id = $1 AND student_id = $2`+lock, schoolID, studentID))
	return p, store.One(err)
}

// ProfilesForStudents reads profiles of many students (alert projection use).
func ProfilesForStudents(ctx context.Context, q store.Q, schoolID string, studentIDs []string) ([]ProfileRow, error) {
	if len(studentIDs) == 0 {
		return []ProfileRow{}, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT `+profileCols+` FROM health_profiles WHERE school_id = $1 AND student_id::text = ANY($2)`, schoolID, studentIDs)
	return store.Collect(rows, err, func(r *sql.Rows) (ProfileRow, error) { return scanProfile(r) })
}

// SaveProfile inserts (p.Version == 1, new id) or updates the profile row.
func SaveProfile(ctx context.Context, q store.Q, p ProfileRow) error {
	_, err := q.ExecContext(ctx, `INSERT INTO health_profiles (id, school_id, student_id, profile_enc, alert_enc, emergency_enc, verified_by, verified_at, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (school_id, student_id) DO UPDATE SET profile_enc = EXCLUDED.profile_enc, alert_enc = EXCLUDED.alert_enc,
			emergency_enc = EXCLUDED.emergency_enc, verified_by = EXCLUDED.verified_by, verified_at = EXCLUDED.verified_at,
			version = EXCLUDED.version, updated_at = now()`,
		p.ID, p.SchoolID, p.StudentID, p.ProfileEnc, p.AlertEnc, p.EmergencyEnc, p.VerifiedBy, p.VerifiedAt, p.Version)
	return err
}

// ---- profile changes -------------------------------------------------------

type ChangeRow struct {
	ID          string     `json:"id"`
	SchoolID    string     `json:"school_id"`
	StudentID   string     `json:"student_id"`
	SubmittedBy string     `json:"submitted_by"`
	ProposedEnc []byte     `json:"-"`
	BaseVersion int        `json:"base_version"`
	Status      string     `json:"status"`
	ApprovalID  *string    `json:"approval_id"`
	DecidedAt   *time.Time `json:"decided_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

const changeCols = `id, school_id, student_id, submitted_by, proposed_enc, base_version, status, approval_id, decided_at, created_at`

func scanChange(r interface{ Scan(...any) error }) (ChangeRow, error) {
	var c ChangeRow
	err := r.Scan(&c.ID, &c.SchoolID, &c.StudentID, &c.SubmittedBy, &c.ProposedEnc, &c.BaseVersion, &c.Status, &c.ApprovalID, &c.DecidedAt, &c.CreatedAt)
	return c, err
}

// InsertChange stores a proposal; created=false returns the row already
// stored under the same idempotency key.
func InsertChange(ctx context.Context, q store.Q, c ChangeRow, idemKey string, profileID *string) (ChangeRow, bool, error) {
	row, err := scanChange(q.QueryRowContext(ctx, `INSERT INTO health_profile_changes
		(id, school_id, student_id, profile_id, submitted_by, proposed_enc, base_version, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (school_id, idempotency_key) DO NOTHING RETURNING `+changeCols,
		c.ID, c.SchoolID, c.StudentID, profileID, c.SubmittedBy, c.ProposedEnc, c.BaseVersion, idemKey))
	if err == nil {
		return row, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return row, false, err
	}
	row, err = scanChange(q.QueryRowContext(ctx, `SELECT `+changeCols+` FROM health_profile_changes WHERE school_id = $1 AND idempotency_key = $2`, c.SchoolID, idemKey))
	return row, false, err
}

// GetChange reads one proposal in the school.
func GetChange(ctx context.Context, q store.Q, schoolID, id string, forUpdate bool) (ChangeRow, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE"
	}
	c, err := scanChange(q.QueryRowContext(ctx, `SELECT `+changeCols+` FROM health_profile_changes WHERE school_id = $1 AND id = $2`+lock, schoolID, id))
	return c, store.One(err)
}

// GetChangeByID reads a proposal without a school scope (approval callback;
// the school is then checked against the row).
func GetChangeByID(ctx context.Context, q store.Q, id string) (ChangeRow, error) {
	c, err := scanChange(q.QueryRowContext(ctx, `SELECT `+changeCols+` FROM health_profile_changes WHERE id = $1 FOR UPDATE`, id))
	return c, store.One(err)
}

// ListChanges lists proposals; submittedBy narrows to one guardian.
func ListChanges(ctx context.Context, q store.Q, schoolID, status, submittedBy, studentID string, page, perPage int) ([]ChangeRow, int, error) {
	where := `school_id = $1 AND ($2 = '' OR status = $2) AND ($3 = '' OR submitted_by::text = $3) AND ($4 = '' OR student_id::text = $4)`
	args := []any{schoolID, status, submittedBy, studentID}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM health_profile_changes WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.QueryContext(ctx, `SELECT `+changeCols+` FROM health_profile_changes WHERE `+where+
		` ORDER BY (status = 'pending') DESC, created_at DESC LIMIT $5 OFFSET $6`, append(args, perPage, common.Offset(page, perPage))...)
	list, err := store.Collect(rows, err, func(r *sql.Rows) (ChangeRow, error) { return scanChange(r) })
	return list, total, err
}

// SetChangeApproval links the core approval id.
func SetChangeApproval(ctx context.Context, q store.Q, id, approvalID string) error {
	_, err := q.ExecContext(ctx, `UPDATE health_profile_changes SET approval_id = $2, updated_at = now() WHERE id = $1`, id, approvalID)
	return err
}

// DecideChange moves a pending proposal to its final status.
func DecideChange(ctx context.Context, q store.Q, id, status string) error {
	_, err := q.ExecContext(ctx, `UPDATE health_profile_changes SET status = $2, decided_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'pending'`, id, status)
	return err
}

// ---- medicines -------------------------------------------------------------

type Medicine struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Unit      string    `json:"unit"`
	OpsItemID *string   `json:"ops_item_id"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

const medicineCols = `id, name, unit, ops_item_id::text, active, created_at`

func scanMedicine(r interface{ Scan(...any) error }) (Medicine, error) {
	var m Medicine
	err := r.Scan(&m.ID, &m.Name, &m.Unit, &m.OpsItemID, &m.Active, &m.CreatedAt)
	return m, err
}

// ListMedicines lists non-deleted medicines; activeOnly hides inactive ones.
func ListMedicines(ctx context.Context, q store.Q, schoolID string, activeOnly bool) ([]Medicine, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+medicineCols+` FROM uks_medicines
		WHERE school_id = $1 AND deleted_at IS NULL AND (NOT $2 OR active) ORDER BY lower(name)`, schoolID, activeOnly)
	return store.Collect(rows, err, func(r *sql.Rows) (Medicine, error) { return scanMedicine(r) })
}

// ErrDuplicateName: a medicine with that name already exists.
var ErrDuplicateName = errors.New("duplicate name")

func uniqueErr(err error) error {
	if err != nil && strings.Contains(err.Error(), "uq_uks_medicines_name") {
		return ErrDuplicateName
	}
	return err
}

// CreateMedicine inserts a medicine.
func CreateMedicine(ctx context.Context, q store.Q, schoolID string, m Medicine) (Medicine, error) {
	out, err := scanMedicine(q.QueryRowContext(ctx, `INSERT INTO uks_medicines (school_id, name, unit, ops_item_id, active)
		VALUES ($1,$2,$3,$4,$5) RETURNING `+medicineCols, schoolID, m.Name, m.Unit, m.OpsItemID, m.Active))
	return out, uniqueErr(err)
}

// UpdateMedicine edits a medicine.
func UpdateMedicine(ctx context.Context, q store.Q, schoolID, id string, m Medicine) (Medicine, error) {
	out, err := scanMedicine(q.QueryRowContext(ctx, `UPDATE uks_medicines SET name = $3, unit = $4, ops_item_id = $5, active = $6, updated_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL RETURNING `+medicineCols, schoolID, id, m.Name, m.Unit, m.OpsItemID, m.Active))
	if err = uniqueErr(err); errors.Is(err, ErrDuplicateName) {
		return out, err
	}
	return out, store.One(err)
}

// DeleteMedicine soft-deletes (riwayat kunjungan tetap merujuk).
func DeleteMedicine(ctx context.Context, q store.Q, schoolID, id string) error {
	res, err := q.ExecContext(ctx, `UPDATE uks_medicines SET deleted_at = now(), active = false, updated_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`, schoolID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// MedicinesByID loads active medicines for a visit.
func MedicinesByID(ctx context.Context, q store.Q, schoolID string, ids []string) (map[string]Medicine, error) {
	out := map[string]Medicine{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT `+medicineCols+` FROM uks_medicines
		WHERE school_id = $1 AND id::text = ANY($2) AND deleted_at IS NULL AND active`, schoolID, ids)
	list, err := store.Collect(rows, err, func(r *sql.Rows) (Medicine, error) { return scanMedicine(r) })
	for _, m := range list {
		out[m.ID] = m
	}
	return out, err
}

// ---- visits ----------------------------------------------------------------

type VisitRow struct {
	ID               string
	SchoolID         string
	StudentID        string
	VisitedAt        time.Time
	LeftAt           *time.Time
	DetailsEnc       []byte
	Outcome          string
	ReferredToEnc    []byte
	HandledBy        string
	AttendanceSync   string
	AttendanceTries  int
	AttendanceError  *string
	ParentNotifiedAt *time.Time
	CreatedAt        time.Time
}

const visitCols = `id, school_id, student_id, visited_at, left_at, details_enc, outcome, referred_to_enc, handled_by,
	attendance_sync, attendance_attempts, attendance_error, parent_notified_at, created_at`

func scanVisit(r interface{ Scan(...any) error }) (VisitRow, error) {
	var v VisitRow
	err := r.Scan(&v.ID, &v.SchoolID, &v.StudentID, &v.VisitedAt, &v.LeftAt, &v.DetailsEnc, &v.Outcome, &v.ReferredToEnc, &v.HandledBy,
		&v.AttendanceSync, &v.AttendanceTries, &v.AttendanceError, &v.ParentNotifiedAt, &v.CreatedAt)
	return v, err
}

// VisitByIdempotencyKey returns the visit stored under the key, if any.
func VisitByIdempotencyKey(ctx context.Context, q store.Q, schoolID, key string) (VisitRow, error) {
	v, err := scanVisit(q.QueryRowContext(ctx, `SELECT `+visitCols+` FROM clinic_visits WHERE school_id = $1 AND idempotency_key = $2`, schoolID, key))
	return v, store.One(err)
}

// InsertVisit stores a visit; created=false when the idempotency key won a race.
func InsertVisit(ctx context.Context, q store.Q, v VisitRow, key string) (bool, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO clinic_visits (id, school_id, student_id, visited_at, left_at, details_enc, outcome,
		referred_to_enc, handled_by, attendance_sync, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (school_id, idempotency_key) DO NOTHING`,
		v.ID, v.SchoolID, v.StudentID, v.VisitedAt, v.LeftAt, v.DetailsEnc, v.Outcome, v.ReferredToEnc, v.HandledBy, v.AttendanceSync, key)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// VisitMedication is one medicine given during a visit (+ its outbox state).
type VisitMedication struct {
	ID         string  `json:"id"`
	MedicineID string  `json:"medicine_id"`
	Name       string  `json:"name"`
	Unit       string  `json:"unit"`
	Qty        float64 `json:"qty"`
	SyncStatus *string `json:"inventory_status"`
}

// InsertVisitMedication stores a medicine line and its inventory outbox row
// (FR-128-003): no ops item → skipped, else pending for the job.
func InsertVisitMedication(ctx context.Context, q store.Q, schoolID, visitID string, m Medicine, qty float64) error {
	var lineID string
	if err := q.QueryRowContext(ctx, `INSERT INTO clinic_visit_medications (school_id, visit_id, medicine_id, qty)
		VALUES ($1,$2,$3,$4) RETURNING id`, schoolID, visitID, m.ID, qty).Scan(&lineID); err != nil {
		return err
	}
	status := "pending"
	if m.OpsItemID == nil {
		status = "skipped"
	}
	_, err := q.ExecContext(ctx, `INSERT INTO inventory_issue_outbox (school_id, visit_medication_id, ops_item_id, qty, status)
		VALUES ($1,$2,$3,$4,$5)`, schoolID, lineID, m.OpsItemID, qty, status)
	return err
}

// VisitMedications lists a visit's medicine lines.
func VisitMedications(ctx context.Context, q store.Q, schoolID, visitID string) ([]VisitMedication, error) {
	rows, err := q.QueryContext(ctx, `SELECT vm.id, vm.medicine_id, m.name, m.unit, vm.qty::float8, o.status
		FROM clinic_visit_medications vm JOIN uks_medicines m ON m.id = vm.medicine_id
		LEFT JOIN inventory_issue_outbox o ON o.visit_medication_id = vm.id
		WHERE vm.school_id = $1 AND vm.visit_id = $2 ORDER BY vm.created_at`, schoolID, visitID)
	return store.Collect(rows, err, func(r *sql.Rows) (VisitMedication, error) {
		var m VisitMedication
		err := r.Scan(&m.ID, &m.MedicineID, &m.Name, &m.Unit, &m.Qty, &m.SyncStatus)
		return m, err
	})
}

// GetVisit reads one visit in the school.
func GetVisit(ctx context.Context, q store.Q, schoolID, id string) (VisitRow, error) {
	v, err := scanVisit(q.QueryRowContext(ctx, `SELECT `+visitCols+` FROM clinic_visits WHERE school_id = $1 AND id = $2`, schoolID, id))
	return v, store.One(err)
}

// VisitFilter narrows ListVisits.
type VisitFilter struct {
	SchoolID, StudentID, Outcome, From, To string
}

// ListVisits pages visits newest first.
func ListVisits(ctx context.Context, q store.Q, f VisitFilter, page, perPage int) ([]VisitRow, int, error) {
	where := `school_id = $1 AND ($2 = '' OR student_id::text = $2) AND ($3 = '' OR outcome = $3)
		AND ($4 = '' OR visited_at >= $4::date) AND ($5 = '' OR visited_at < $5::date + 1)`
	args := []any{f.SchoolID, f.StudentID, f.Outcome, f.From, f.To}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM clinic_visits WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.QueryContext(ctx, `SELECT `+visitCols+` FROM clinic_visits WHERE `+where+
		` ORDER BY visited_at DESC LIMIT $6 OFFSET $7`, append(args, perPage, common.Offset(page, perPage))...)
	list, err := store.Collect(rows, err, func(r *sql.Rows) (VisitRow, error) { return scanVisit(r) })
	return list, total, err
}

// AddendumRow is one clinic_visit_addenda row.
type AddendumRow struct {
	ID        string
	NoteEnc   []byte
	CreatedBy string
	CreatedAt time.Time
}

// InsertAddendum appends a correction note (the visit itself never changes).
func InsertAddendum(ctx context.Context, q store.Q, id, schoolID, visitID string, noteEnc []byte, actor string) (AddendumRow, error) {
	a := AddendumRow{ID: id, NoteEnc: noteEnc, CreatedBy: actor}
	err := q.QueryRowContext(ctx, `INSERT INTO clinic_visit_addenda (id, school_id, visit_id, note_enc, created_by)
		VALUES ($1,$2,$3,$4,$5) RETURNING created_at`, id, schoolID, visitID, noteEnc, actor).Scan(&a.CreatedAt)
	return a, err
}

// Addenda lists a visit's addenda oldest first.
func Addenda(ctx context.Context, q store.Q, schoolID, visitID string) ([]AddendumRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, note_enc, created_by, created_at FROM clinic_visit_addenda
		WHERE school_id = $1 AND visit_id = $2 ORDER BY created_at`, schoolID, visitID)
	return store.Collect(rows, err, func(r *sql.Rows) (AddendumRow, error) {
		var a AddendumRow
		return a, r.Scan(&a.ID, &a.NoteEnc, &a.CreatedBy, &a.CreatedAt)
	})
}

// SetAttendanceSync records an attendance sync attempt result.
func SetAttendanceSync(ctx context.Context, q store.Q, visitID, status, errMsg string) error {
	_, err := q.ExecContext(ctx, `UPDATE clinic_visits SET attendance_sync = $2, attendance_attempts = attendance_attempts + 1,
		attendance_error = NULLIF($3,''), updated_at = now() WHERE id = $1`, visitID, status, errMsg)
	return err
}

// MarkParentNotified stamps parent_notified_at once.
func MarkParentNotified(ctx context.Context, q store.Q, visitID string) error {
	_, err := q.ExecContext(ctx, `UPDATE clinic_visits SET parent_notified_at = COALESCE(parent_notified_at, now()), updated_at = now() WHERE id = $1`, visitID)
	return err
}

// PendingSyncVisits returns visits awaiting attendance sync or parent notice.
func PendingSyncVisits(ctx context.Context, q store.Q, limit int) ([]VisitRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+visitCols+` FROM clinic_visits
		WHERE attendance_sync = 'pending' OR (outcome <> 'back_to_class' AND parent_notified_at IS NULL)
		ORDER BY created_at LIMIT $1`, limit)
	return store.Collect(rows, err, func(r *sql.Rows) (VisitRow, error) { return scanVisit(r) })
}

// OutboxRow is one pending inventory issue.
type OutboxRow struct {
	ID, SchoolID, VisitMedicationID, VisitID string
	OpsItemID                                string
	Qty                                      float64
	Attempts                                 int
}

// PendingOutbox returns pending inventory issues.
func PendingOutbox(ctx context.Context, q store.Q, limit int) ([]OutboxRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT o.id, o.school_id, o.visit_medication_id, vm.visit_id, o.ops_item_id::text, o.qty::float8, o.attempts
		FROM inventory_issue_outbox o JOIN clinic_visit_medications vm ON vm.id = o.visit_medication_id
		WHERE o.status = 'pending' AND o.ops_item_id IS NOT NULL ORDER BY o.created_at LIMIT $1`, limit)
	return store.Collect(rows, err, func(r *sql.Rows) (OutboxRow, error) {
		var o OutboxRow
		return o, r.Scan(&o.ID, &o.SchoolID, &o.VisitMedicationID, &o.VisitID, &o.OpsItemID, &o.Qty, &o.Attempts)
	})
}

// SetOutbox records an inventory delivery attempt.
func SetOutbox(ctx context.Context, q store.Q, id, status, errMsg string, countAttempt bool) error {
	inc := 0
	if countAttempt {
		inc = 1
	}
	_, err := q.ExecContext(ctx, `UPDATE inventory_issue_outbox SET status = $2, last_error = NULLIF($3,''), attempts = attempts + $4, updated_at = now()
		WHERE id = $1`, id, status, errMsg, inc)
	return err
}

// ---- programs --------------------------------------------------------------

type Program struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	StartsOn        string     `json:"starts_on"`
	EndsOn          string     `json:"ends_on"`
	RequiresConsent bool       `json:"requires_consent"`
	ConsentDeadline *string    `json:"consent_deadline"`
	Status          string     `json:"status"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	Targets         []Target   `json:"targets"`
	ParticipantsN   int        `json:"participants_count"`
}

const programCols = `p.id, p.kind, p.name, p.description, to_char(p.starts_on,'YYYY-MM-DD'), to_char(p.ends_on,'YYYY-MM-DD'),
	p.requires_consent, to_char(p.consent_deadline,'YYYY-MM-DD'), p.status, p.created_by, p.created_at,
	(SELECT count(*) FROM health_program_participants pp WHERE pp.program_id = p.id)`

func scanProgram(r interface{ Scan(...any) error }) (Program, error) {
	var p Program
	err := r.Scan(&p.ID, &p.Kind, &p.Name, &p.Description, &p.StartsOn, &p.EndsOn, &p.RequiresConsent, &p.ConsentDeadline,
		&p.Status, &p.CreatedBy, &p.CreatedAt, &p.ParticipantsN)
	return p, err
}

// ProgramInput is the editable program payload.
type ProgramInput struct {
	Kind            string   `json:"kind"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	StartsOn        string   `json:"starts_on"`
	EndsOn          string   `json:"ends_on"`
	RequiresConsent bool     `json:"requires_consent"`
	ConsentDeadline *string  `json:"consent_deadline"`
	Targets         []Target `json:"targets"`
}

// InsertProgram creates a draft program.
func InsertProgram(ctx context.Context, q store.Q, schoolID, actor string, in ProgramInput) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `INSERT INTO health_programs (school_id, kind, name, description, starts_on, ends_on, requires_consent, consent_deadline, created_by)
		VALUES ($1,$2,$3,$4,$5::date,$6::date,$7,$8::date,$9) RETURNING id`,
		schoolID, in.Kind, strings.TrimSpace(in.Name), in.Description, in.StartsOn, in.EndsOn, in.RequiresConsent, in.ConsentDeadline, actor).Scan(&id)
	return id, err
}

// UpdateProgram edits a draft program.
func UpdateProgram(ctx context.Context, q store.Q, schoolID, id string, in ProgramInput) error {
	res, err := q.ExecContext(ctx, `UPDATE health_programs SET kind = $3, name = $4, description = $5, starts_on = $6::date, ends_on = $7::date,
		requires_consent = $8, consent_deadline = $9::date, updated_at = now() WHERE school_id = $1 AND id = $2 AND status = 'draft'`,
		schoolID, id, in.Kind, strings.TrimSpace(in.Name), in.Description, in.StartsOn, in.EndsOn, in.RequiresConsent, in.ConsentDeadline)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ReplaceTargets rewrites a draft program's targets.
func ReplaceTargets(ctx context.Context, q store.Q, schoolID, programID string, targets []Target) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM health_program_targets WHERE program_id = $1`, programID); err != nil {
		return err
	}
	for _, t := range targets {
		if _, err := q.ExecContext(ctx, `INSERT INTO health_program_targets (school_id, program_id, target_kind, target_ref)
			VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, schoolID, programID, t.Kind, strings.TrimSpace(t.Ref)); err != nil {
			return err
		}
	}
	return nil
}

// GetProgram reads one program with its targets.
func GetProgram(ctx context.Context, q store.Q, schoolID, id string) (Program, error) {
	p, err := scanProgram(q.QueryRowContext(ctx, `SELECT `+programCols+` FROM health_programs p WHERE p.school_id = $1 AND p.id = $2`, schoolID, id))
	if err != nil {
		return p, store.One(err)
	}
	rows, err := q.QueryContext(ctx, `SELECT target_kind, target_ref FROM health_program_targets WHERE program_id = $1 ORDER BY target_kind, target_ref`, id)
	p.Targets, err = store.Collect(rows, err, func(r *sql.Rows) (Target, error) {
		var t Target
		return t, r.Scan(&t.Kind, &t.Ref)
	})
	return p, err
}

// ListPrograms pages programs newest first.
func ListPrograms(ctx context.Context, q store.Q, schoolID, status, kind string, page, perPage int) ([]Program, int, error) {
	where := `p.school_id = $1 AND ($2 = '' OR p.status = $2) AND ($3 = '' OR p.kind = $3)`
	args := []any{schoolID, status, kind}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM health_programs p WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.QueryContext(ctx, `SELECT `+programCols+` FROM health_programs p WHERE `+where+
		` ORDER BY p.starts_on DESC, p.created_at DESC LIMIT $4 OFFSET $5`, append(args, perPage, common.Offset(page, perPage))...)
	list, err := store.Collect(rows, err, func(r *sql.Rows) (Program, error) { return scanProgram(r) })
	return list, total, err
}

// SetProgramStatus moves a program from one status to another.
func SetProgramStatus(ctx context.Context, q store.Q, schoolID, id, from, to string) error {
	res, err := q.ExecContext(ctx, `UPDATE health_programs SET status = $4, updated_at = now() WHERE school_id = $1 AND id = $2 AND status = $3`, schoolID, id, from, to)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// InsertParticipant adds a student to a program (no-op if already in).
func InsertParticipant(ctx context.Context, q store.Q, schoolID, programID, studentID string, consentID *string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO health_program_participants (school_id, program_id, student_id, consent_id)
		VALUES ($1,$2,$3,$4) ON CONFLICT (program_id, student_id) DO NOTHING`, schoolID, programID, studentID, consentID)
	return err
}

type ParticipantRow struct {
	ID             string
	ProgramID      string
	StudentID      string
	ConsentID      *string
	ConsentStatus  *string
	ResultStatus   string
	ResultEnc      []byte
	NeedsFollowUp  bool
	FollowUpEnc    []byte
	FollowUpDoneAt *time.Time
	RecordedBy     *string
	RecordedAt     *time.Time
}

const participantCols = `pp.id, pp.program_id, pp.student_id, pp.consent_id::text, pc.status, pp.result_status, pp.result_enc,
	pp.needs_follow_up, pp.follow_up_enc, pp.follow_up_done_at, pp.recorded_by::text, pp.recorded_at`

func scanParticipant(r interface{ Scan(...any) error }) (ParticipantRow, error) {
	var p ParticipantRow
	err := r.Scan(&p.ID, &p.ProgramID, &p.StudentID, &p.ConsentID, &p.ConsentStatus, &p.ResultStatus, &p.ResultEnc,
		&p.NeedsFollowUp, &p.FollowUpEnc, &p.FollowUpDoneAt, &p.RecordedBy, &p.RecordedAt)
	return p, err
}

// Participants lists a program's participants with consent status.
func Participants(ctx context.Context, q store.Q, schoolID, programID string) ([]ParticipantRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+participantCols+` FROM health_program_participants pp
		LEFT JOIN parent_consents pc ON pc.id = pp.consent_id
		WHERE pp.school_id = $1 AND pp.program_id = $2 ORDER BY pp.created_at, pp.student_id`, schoolID, programID)
	return store.Collect(rows, err, func(r *sql.Rows) (ParticipantRow, error) { return scanParticipant(r) })
}

// StudentParticipations lists one student's program rows (all programs).
func StudentParticipations(ctx context.Context, q store.Q, schoolID, studentID string) ([]ParticipantRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+participantCols+` FROM health_program_participants pp
		LEFT JOIN parent_consents pc ON pc.id = pp.consent_id
		WHERE pp.school_id = $1 AND pp.student_id = $2 ORDER BY pp.created_at DESC`, schoolID, studentID)
	return store.Collect(rows, err, func(r *sql.Rows) (ParticipantRow, error) { return scanParticipant(r) })
}

// GetParticipant reads one participant of a program (row-locked).
func GetParticipant(ctx context.Context, q store.Q, schoolID, programID, id string) (ParticipantRow, error) {
	p, err := scanParticipant(q.QueryRowContext(ctx, `SELECT `+participantCols+` FROM health_program_participants pp
		LEFT JOIN parent_consents pc ON pc.id = pp.consent_id
		WHERE pp.school_id = $1 AND pp.program_id = $2 AND pp.id = $3 FOR UPDATE OF pp`, schoolID, programID, id))
	return p, store.One(err)
}

// SaveResult records a participant result (FR-129-003).
func SaveResult(ctx context.Context, q store.Q, id, status string, resultEnc []byte, needsFollowUp bool, followUpEnc []byte, actor string) error {
	_, err := q.ExecContext(ctx, `UPDATE health_program_participants SET result_status = $2, result_enc = $3, needs_follow_up = $4,
		follow_up_enc = $5, follow_up_done_at = NULL, recorded_by = $6, recorded_at = now(), updated_at = now() WHERE id = $1`,
		id, status, resultEnc, needsFollowUp, followUpEnc, actor)
	return err
}

// MarkFollowUpDone closes a participant's follow-up.
func MarkFollowUpDone(ctx context.Context, q store.Q, id string) error {
	res, err := q.ExecContext(ctx, `UPDATE health_program_participants SET follow_up_done_at = now(), updated_at = now()
		WHERE id = $1 AND needs_follow_up AND follow_up_done_at IS NULL`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ConsentInUse: a program consent is used once a result was recorded.
func ConsentInUse(ctx context.Context, q store.Q, schoolID, consentID string) (bool, error) {
	var used bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM health_program_participants
		WHERE school_id = $1 AND consent_id = $2 AND result_status <> 'not_recorded')`, schoolID, consentID).Scan(&used)
	return used, err
}

// ReminderCandidate is a pending consent due today for one program.
type ReminderCandidate struct {
	SchoolID, ProgramID, ProgramName, ConsentID, StudentID string
}

// ConsentReminderCandidates lists pending consents of published programs.
func ConsentReminderCandidates(ctx context.Context, q store.Q, limit int) ([]ReminderCandidate, map[string]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT p.school_id, p.id, p.name, pc.id, pc.student_id, to_char(p.consent_deadline,'YYYY-MM-DD')
		FROM health_programs p JOIN parent_consents pc ON pc.subject_type = $1 AND pc.subject_id = p.id
		WHERE p.status = 'published' AND p.requires_consent AND pc.status = 'pending' AND p.consent_deadline IS NOT NULL
		  AND p.consent_deadline >= current_date - 1 AND p.consent_deadline <= current_date + 1
		ORDER BY p.id LIMIT $2`, ConsentSubjectProgram, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []ReminderCandidate{}
	deadlines := map[string]string{}
	for rows.Next() {
		var c ReminderCandidate
		var deadline string
		if err := rows.Scan(&c.SchoolID, &c.ProgramID, &c.ProgramName, &c.ConsentID, &c.StudentID, &deadline); err != nil {
			return nil, nil, err
		}
		deadlines[c.ProgramID] = deadline
		out = append(out, c)
	}
	return out, deadlines, rows.Err()
}

// ProgramSummary is the parent-facing consent subject summary (no health data).
func ProgramSummary(ctx context.Context, q store.Q, schoolID, programID string) (map[string]any, error) {
	p, err := GetProgram(ctx, q, schoolID, programID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": p.ID, "kind": p.Kind, "name": p.Name, "description": p.Description,
		"starts_on": p.StartsOn, "ends_on": p.EndsOn, "consent_deadline": p.ConsentDeadline, "status": p.Status}, nil
}

// ParseDate parses YYYY-MM-DD.
func ParseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return t, fmt.Errorf("tanggal %q tidak valid", s)
	}
	return t, nil
}
