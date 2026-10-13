package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"skci/attendance/internal/model"
)

// ---- attendance lock settings (FR-41-003, BR-ATT-01 [per klien]) --------

// GetLockSettings returns the school's lock configuration, or the disabled
// default when the school never configured one.
func (s *Store) GetLockSettings(ctx context.Context, schoolID string) (*model.LockSettings, error) {
	l := model.DefaultLockSettings(schoolID)
	var updatedBy sql.NullString
	var updatedAt sql.NullTime
	err := s.DB.QueryRowContext(ctx, `
		SELECT enabled, cutoff_days, to_char(cutoff_time, 'HH24:MI'), updated_by::text, updated_at
		FROM attendance_lock_settings WHERE school_id = $1`, schoolID).
		Scan(&l.Enabled, &l.CutoffDays, &l.CutoffTime, &updatedBy, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &l, nil
	}
	if err != nil {
		return nil, err
	}
	if updatedBy.Valid {
		l.UpdatedBy = &updatedBy.String
	}
	if updatedAt.Valid {
		l.UpdatedAt = &updatedAt.Time
	}
	return &l, nil
}

// UpsertLockSettings saves the configuration and writes an audit row in the
// same transaction (configuration changes are audit-relevant: they decide
// what BR-ATT-01 protects).
func (s *Store) UpsertLockSettings(ctx context.Context, in model.LockSettings, actorID, actorRole string) (*model.LockSettings, error) {
	before, err := s.GetLockSettings(ctx, in.SchoolID)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO attendance_lock_settings (school_id, enabled, cutoff_days, cutoff_time, updated_by, updated_at)
		VALUES ($1, $2, $3, $4::time, $5, now())
		ON CONFLICT (school_id) DO UPDATE SET enabled = EXCLUDED.enabled, cutoff_days = EXCLUDED.cutoff_days,
			cutoff_time = EXCLUDED.cutoff_time, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		in.SchoolID, in.Enabled, in.CutoffDays, in.CutoffTime, nilIfEmpty(actorID)); err != nil {
		return nil, err
	}
	if err := writeAudit(ctx, tx, AuditEntry{
		SchoolID: in.SchoolID, Module: "attendance", EntityType: "attendance_lock_settings", EntityID: in.SchoolID,
		Action: "attendance_lock.updated", ActorUserID: nilIfEmpty(actorID), ActorRole: nilIfEmpty(actorRole),
		Before: map[string]any{"enabled": before.Enabled, "cutoff_days": before.CutoffDays, "cutoff_time": before.CutoffTime},
		After:  map[string]any{"enabled": in.Enabled, "cutoff_days": in.CutoffDays, "cutoff_time": in.CutoffTime},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetLockSettings(ctx, in.SchoolID)
}

// ---- manual daily input (FR-41-002, FR-69-001) ---------------------------

// FindOrCreateManualSession returns the id of the source='manual'
// class_session for (classroom_id, session_date) — the parent row for
// attendance written by the admin / wali kelas daily input, which isn't tied
// to a schedule slot (same idea as FindOrCreateImportSession).
func (s *Store) FindOrCreateManualSession(ctx context.Context, schoolID, academicYearID, termID, classroomID, sessionDate, actorID string) (string, error) {
	return findOrCreateManualSession(ctx, s.DB, schoolID, academicYearID, termID, classroomID, sessionDate, actorID)
}

func findOrCreateManualSession(ctx context.Context, q queryRower, schoolID, academicYearID, termID, classroomID, sessionDate, actorID string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `
		INSERT INTO class_sessions (school_id, academic_year_id, term_id, classroom_id, guru_id, session_date, source)
		VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, 'manual')
		ON CONFLICT (classroom_id, session_date) WHERE source = 'manual'
		DO UPDATE SET updated_at = now()
		RETURNING id`, schoolID, academicYearID, termID, classroomID, actorID, sessionDate).Scan(&id)
	return id, err
}

// DailySubmission is the wali kelas "kirim" state for one classroom/day.
type DailySubmission struct {
	ClassroomID string     `json:"classroom_id"`
	SessionDate string     `json:"session_date"`
	Status      string     `json:"status"` // draft | submitted
	Catatan     *string    `json:"catatan"`
	SubmittedBy *string    `json:"submitted_by"`
	SubmittedAt *time.Time `json:"submitted_at"`
	UpdatedBy   *string    `json:"updated_by"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

// GetDailySubmission returns nil (no error) when nothing was saved yet.
func (s *Store) GetDailySubmission(ctx context.Context, classroomID, date string) (*DailySubmission, error) {
	d := DailySubmission{ClassroomID: classroomID, SessionDate: date}
	var catatan, subBy, updBy sql.NullString
	var subAt, updAt sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT status, catatan, submitted_by::text, submitted_at, updated_by::text, updated_at
		FROM daily_attendance_submissions WHERE classroom_id = $1 AND session_date = $2`, classroomID, date).
		Scan(&d.Status, &catatan, &subBy, &subAt, &updBy, &updAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if catatan.Valid {
		d.Catatan = &catatan.String
	}
	if subBy.Valid {
		d.SubmittedBy = &subBy.String
	}
	if subAt.Valid {
		d.SubmittedAt = &subAt.Time
	}
	if updBy.Valid {
		d.UpdatedBy = &updBy.String
	}
	if updAt.Valid {
		d.UpdatedAt = &updAt.Time
	}
	return &d, nil
}

// SaveDailySubmission records a draft save or a submit. A submitted day
// stays submitted on later draft saves (re-saving before the lock doesn't
// "un-send" it); submitted_at always reflects the latest submit.
func (s *Store) SaveDailySubmission(ctx context.Context, schoolID, classroomID, date, actorID string, submit bool, catatan *string) (*DailySubmission, error) {
	status := "draft"
	if submit {
		status = "submitted"
	}
	if _, err := s.DB.ExecContext(ctx, `
		INSERT INTO daily_attendance_submissions (classroom_id, session_date, school_id, status, catatan,
			submitted_by, submitted_at, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, CASE WHEN $4 = 'submitted' THEN $6::uuid END,
			CASE WHEN $4 = 'submitted' THEN now() END, $6, now())
		ON CONFLICT (classroom_id, session_date) DO UPDATE SET
			status = CASE WHEN EXCLUDED.status = 'submitted' THEN 'submitted' ELSE daily_attendance_submissions.status END,
			catatan = COALESCE(EXCLUDED.catatan, daily_attendance_submissions.catatan),
			submitted_by = COALESCE(EXCLUDED.submitted_by, daily_attendance_submissions.submitted_by),
			submitted_at = COALESCE(EXCLUDED.submitted_at, daily_attendance_submissions.submitted_at),
			updated_by = EXCLUDED.updated_by, updated_at = now()`,
		classroomID, date, schoolID, status, catatan, actorID); err != nil {
		return nil, err
	}
	return s.GetDailySubmission(ctx, classroomID, date)
}

// GetStudentAttendanceOn returns one student's row for a classroom/day, or
// ErrNotFound.
func (s *Store) GetStudentAttendanceOn(ctx context.Context, classroomID, date, studentID string) (*model.StudentAttendance, error) {
	sa := &model.StudentAttendance{}
	var termID, catatan, markedBy sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, class_session_id, school_id, academic_year_id, term_id, classroom_id,
			student_id, status, catatan, marked_by, created_at, updated_at
		FROM student_attendances WHERE classroom_id = $1 AND session_date = $2 AND student_id = $3`,
		classroomID, date, studentID).Scan(&sa.ID, &sa.ClassSessionID, &sa.SchoolID, &sa.AcademicYearID, &termID,
		&sa.ClassroomID, &sa.StudentID, &sa.Status, &catatan, &markedBy, &sa.CreatedAt, &sa.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if termID.Valid {
		sa.TermID = &termID.String
	}
	if catatan.Valid {
		sa.Catatan = &catatan.String
	}
	if markedBy.Valid {
		sa.MarkedBy = &markedBy.String
	}
	return sa, nil
}

// ---- alpa notification dedupe (FR-41-004) ---------------------------------

// ClaimAlpaNotification atomically claims the (student, day) notification
// slot. true means this caller must send it; false means it was already
// sent (or is being sent) by an earlier write.
func (s *Store) ClaimAlpaNotification(ctx context.Context, schoolID, studentID, date string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO alpa_notifications (student_id, session_date, school_id)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, studentID, date, schoolID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ReleaseAlpaNotification undoes a claim whose delivery failed, so the next
// write for that student/day retries.
func (s *Store) ReleaseAlpaNotification(ctx context.Context, studentID, date string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM alpa_notifications WHERE student_id = $1 AND session_date = $2`, studentID, date)
	return err
}
