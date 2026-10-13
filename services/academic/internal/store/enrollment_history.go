package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Class enrollment history (PRD FR-39-003). Rows are written by the
// trg_class_enrollment_history trigger (migration 0006) on every
// INSERT/UPDATE/DELETE of class_enrollments, so no writer can forget to log.
// Writers that know WHY a change happens tag it via setEnrollmentContext
// inside their transaction; untagged changes get a derived action
// (enroll / move / status_change / remove).

const (
	ActionEnroll           = "enroll"
	ActionMove             = "move"
	ActionRemove           = "remove"
	ActionStatusChange     = "status_change"
	ActionTransfer         = "transfer"
	ActionPromotion        = "promotion"
	ActionAdmission        = "admission"
	ActionWithdrawal       = "withdrawal"
	ActionUnitTransfer     = "unit_transfer"
	ActionClassroomDeleted = "classroom_deleted"
	ActionStudentDeleted   = "student_deleted"
)

// EnrollmentChange is the "why/who" context recorded alongside a change.
// Every field is optional.
type EnrollmentChange struct {
	Action      string
	Reason      string
	RequestID   string // student_status_requests.id
	ActorUserID string
}

// setEnrollmentContext stores ch in transaction-local settings read by the
// history trigger. is_local=true means they vanish at COMMIT/ROLLBACK, so
// they can never leak onto another request sharing the pooled connection.
func setEnrollmentContext(ctx context.Context, tx *sql.Tx, ch EnrollmentChange) error {
	_, err := tx.ExecContext(ctx, `
		SELECT set_config('skci.enroll_action', $1, true),
		       set_config('skci.enroll_reason', $2, true),
		       set_config('skci.enroll_request_id', $3, true),
		       set_config('skci.enroll_actor', $4, true)
	`, ch.Action, ch.Reason, ch.RequestID, ch.ActorUserID)
	return err
}

// CapacityError is returned when an enrollment would push a classroom past
// its capacity (or a capacity edit would drop below the current roster).
type CapacityError struct {
	ClassroomName string
	Capacity      int
	Enrolled      int
	Lowering      bool
}

func (e *CapacityError) Error() string {
	if e.Lowering {
		return fmt.Sprintf("Kapasitas kelas %s tidak boleh lebih kecil dari jumlah siswa aktif saat ini (%d siswa).", e.ClassroomName, e.Enrolled)
	}
	return fmt.Sprintf("Kelas %s sudah penuh (%d/%d siswa). Naikkan kapasitas kelas atau pilih kelas lain.", e.ClassroomName, e.Enrolled, e.Capacity)
}

// checkCapacity verifies classroomID can take studentUserID. It row-locks
// the classroom (FOR UPDATE) so two concurrent enrollments into the last
// seat serialize instead of both passing. A student already active in the
// classroom does not count against the seat they occupy (idempotent re-add).
func checkCapacity(ctx context.Context, tx *sql.Tx, classroomID, studentUserID string) error {
	var name string
	var capacity sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT name, capacity FROM classrooms WHERE id = $1 FOR UPDATE`, classroomID).Scan(&name, &capacity)
	if err == sql.ErrNoRows {
		return nil // caller validates existence; nothing to enforce
	}
	if err != nil {
		return err
	}
	if !capacity.Valid {
		return nil
	}
	var others int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM class_enrollments
		WHERE classroom_id = $1 AND status = 'active' AND student_user_id::text <> $2
	`, classroomID, studentUserID).Scan(&others); err != nil {
		return err
	}
	if others+1 > int(capacity.Int64) {
		return &CapacityError{ClassroomName: name, Capacity: int(capacity.Int64), Enrolled: others}
	}
	return nil
}

// CheckClassroomCapacity is the read-only (no lock) variant for request-time
// validation, e.g. creating a class-transfer request (Menu 78). The binding
// check still happens under lock when the transfer is applied.
func (s *Store) CheckClassroomCapacity(ctx context.Context, schoolID, classroomID, studentUserID string) error {
	var name string
	var capacity sql.NullInt64
	var others int
	err := s.DB.QueryRowContext(ctx, `
		SELECT c.name, c.capacity,
		       (SELECT count(*) FROM class_enrollments ce
		        WHERE ce.classroom_id = c.id AND ce.status = 'active' AND ce.student_user_id::text <> $3)
		FROM classrooms c WHERE c.id::text = $1 AND c.school_id::text = $2 AND c.deleted_at IS NULL
	`, classroomID, schoolID, studentUserID).Scan(&name, &capacity, &others)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil || !capacity.Valid {
		return err
	}
	if others+1 > int(capacity.Int64) {
		return &CapacityError{ClassroomName: name, Capacity: int(capacity.Int64), Enrolled: others}
	}
	return nil
}

// EnrollmentHistoryEntry is one row of class_enrollment_history, enriched
// with classroom names for display.
type EnrollmentHistoryEntry struct {
	ID                string    `json:"id"`
	SchoolID          string    `json:"school_id"`
	AcademicYearID    string    `json:"academic_year_id"`
	StudentUserID     string    `json:"student_user_id"`
	FromClassroomID   *string   `json:"from_classroom_id"`
	FromClassroomName *string   `json:"from_classroom_name"`
	ToClassroomID     *string   `json:"to_classroom_id"`
	ToClassroomName   *string   `json:"to_classroom_name"`
	FromStatus        *string   `json:"from_status"`
	ToStatus          *string   `json:"to_status"`
	Action            string    `json:"action"`
	Reason            *string   `json:"reason"`
	RequestID         *string   `json:"request_id"`
	ActorUserID       *string   `json:"actor_user_id"`
	CreatedAt         time.Time `json:"created_at"`
}

const historySelect = `
	SELECT h.id, h.school_id, h.academic_year_id, h.student_user_id,
	       h.from_classroom_id, fc.name, h.to_classroom_id, tc.name,
	       h.from_status, h.to_status, h.action, h.reason, h.request_id, h.actor_user_id, h.created_at
	FROM class_enrollment_history h
	LEFT JOIN classrooms fc ON fc.id = h.from_classroom_id
	LEFT JOIN classrooms tc ON tc.id = h.to_classroom_id
`

// ListStudentEnrollmentHistory: every class change of one student, newest first.
func (s *Store) ListStudentEnrollmentHistory(ctx context.Context, schoolID, studentUserID string) ([]EnrollmentHistoryEntry, error) {
	return s.queryHistory(ctx, historySelect+`
		WHERE ($1 = '' OR h.school_id::text = $1) AND h.student_user_id::text = $2
		ORDER BY h.created_at DESC, h.id`, schoolID, studentUserID)
}

// ListClassroomEnrollmentHistory: every change where a student entered or
// left the classroom, newest first.
func (s *Store) ListClassroomEnrollmentHistory(ctx context.Context, schoolID, classroomID string) ([]EnrollmentHistoryEntry, error) {
	return s.queryHistory(ctx, historySelect+`
		WHERE ($1 = '' OR h.school_id::text = $1)
		  AND (h.from_classroom_id::text = $2 OR h.to_classroom_id::text = $2)
		ORDER BY h.created_at DESC, h.id`, schoolID, classroomID)
}

func (s *Store) queryHistory(ctx context.Context, q string, args ...any) ([]EnrollmentHistoryEntry, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EnrollmentHistoryEntry{}
	for rows.Next() {
		var e EnrollmentHistoryEntry
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.StudentUserID,
			&e.FromClassroomID, &e.FromClassroomName, &e.ToClassroomID, &e.ToClassroomName,
			&e.FromStatus, &e.ToStatus, &e.Action, &e.Reason, &e.RequestID, &e.ActorUserID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
