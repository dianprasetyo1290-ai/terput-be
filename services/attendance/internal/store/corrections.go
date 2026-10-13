package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---- attendance corrections after lock (BR-ATT-01, AC-41-02) -------------

// ErrCorrectionPending: a pending correction already exists for the same
// student/classroom/day.
var ErrCorrectionPending = errors.New("correction already pending for this student and date")

type AttendanceCorrection struct {
	ID              string     `json:"id"`
	SchoolID        string     `json:"school_id"`
	AcademicYearID  string     `json:"academic_year_id"`
	TermID          *string    `json:"term_id"`
	ClassroomID     string     `json:"classroom_id"`
	StudentID       string     `json:"student_id"`
	SessionDate     string     `json:"session_date"`
	FromStatus      *string    `json:"from_status"`
	ToStatus        string     `json:"to_status"`
	Catatan         *string    `json:"catatan"`
	Reason          string     `json:"reason"`
	RequestedBy     string     `json:"requested_by"`
	RequestedRole   string     `json:"requested_role"`
	Status          string     `json:"status"` // pending | approved | rejected
	ApprovalID      *string    `json:"approval_id"`
	DecidedBy       *string    `json:"decided_by"`
	DecisionComment *string    `json:"decision_comment"`
	DecidedAt       *time.Time `json:"decided_at"`
	AppliedAt       *time.Time `json:"applied_at"`
	CreatedAt       time.Time  `json:"created_at"`

	// Enrichment (handler, best-effort).
	StudentName   string `json:"student_name,omitempty"`
	ClassroomName string `json:"classroom_name,omitempty"`
	RequesterName string `json:"requester_name,omitempty"`
}

const correctionColumns = `id, school_id, academic_year_id, term_id::text, classroom_id, student_id,
	to_char(session_date, 'YYYY-MM-DD'), from_status, to_status, catatan, reason, requested_by, requested_role,
	status, approval_id, decided_by, decision_comment, decided_at, applied_at, created_at`

func scanCorrection(sc interface{ Scan(...interface{}) error }) (*AttendanceCorrection, error) {
	c := &AttendanceCorrection{}
	var termID, from, catatan, approvalID, decidedBy, comment sql.NullString
	var decidedAt, appliedAt sql.NullTime
	if err := sc.Scan(&c.ID, &c.SchoolID, &c.AcademicYearID, &termID, &c.ClassroomID, &c.StudentID,
		&c.SessionDate, &from, &c.ToStatus, &catatan, &c.Reason, &c.RequestedBy, &c.RequestedRole,
		&c.Status, &approvalID, &decidedBy, &comment, &decidedAt, &appliedAt, &c.CreatedAt); err != nil {
		return nil, err
	}
	str := func(n sql.NullString) *string {
		if n.Valid {
			v := n.String
			return &v
		}
		return nil
	}
	tm := func(n sql.NullTime) *time.Time {
		if n.Valid {
			v := n.Time
			return &v
		}
		return nil
	}
	c.TermID, c.FromStatus, c.Catatan = str(termID), str(from), str(catatan)
	c.ApprovalID, c.DecidedBy, c.DecisionComment = str(approvalID), str(decidedBy), str(comment)
	c.DecidedAt, c.AppliedAt = tm(decidedAt), tm(appliedAt)
	return c, nil
}

type CreateCorrectionInput struct {
	SchoolID, AcademicYearID, TermID, ClassroomID, StudentID, SessionDate string
	FromStatus                                                            *string
	ToStatus                                                              string
	Catatan                                                               *string
	Reason, RequestedBy, RequestedRole                                    string
}

// CreateCorrection inserts a pending correction and its "requested" audit
// row in one transaction (AC-41-02: the pending change is already on the
// audit trail before anyone approves it).
func (s *Store) CreateCorrection(ctx context.Context, in CreateCorrectionInput) (*AttendanceCorrection, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	c, err := scanCorrection(tx.QueryRowContext(ctx, `
		INSERT INTO attendance_corrections (school_id, academic_year_id, term_id, classroom_id, student_id, session_date,
			from_status, to_status, catatan, reason, requested_by, requested_role)
		VALUES ($1, $2, NULLIF($3,'')::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+correctionColumns,
		in.SchoolID, in.AcademicYearID, in.TermID, in.ClassroomID, in.StudentID, in.SessionDate,
		in.FromStatus, in.ToStatus, in.Catatan, in.Reason, in.RequestedBy, in.RequestedRole))
	if err != nil {
		if strings.Contains(err.Error(), "uq_attendance_corrections_pending") {
			return nil, ErrCorrectionPending
		}
		return nil, err
	}
	if err := writeAudit(ctx, tx, AuditEntry{
		SchoolID: c.SchoolID, Module: "attendance", EntityType: "attendance_correction", EntityID: c.ID,
		Action: "attendance_correction.requested", ActorUserID: nilIfEmpty(in.RequestedBy), ActorRole: nilIfEmpty(in.RequestedRole),
		Before: map[string]any{"student_id": c.StudentID, "session_date": c.SessionDate, "status": c.FromStatus},
		After:  map[string]any{"student_id": c.StudentID, "session_date": c.SessionDate, "status": c.ToStatus, "catatan": c.Catatan},
		Reason: nilIfEmpty(in.Reason), CorrelationID: nilIfEmpty("attendance_correction:" + c.ID),
	}); err != nil {
		return nil, err
	}
	return c, tx.Commit()
}

// SetCorrectionApprovalID stores core's approval id on the correction.
func (s *Store) SetCorrectionApprovalID(ctx context.Context, id, approvalID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE attendance_corrections SET approval_id = $2, updated_at = now() WHERE id = $1`, id, approvalID)
	return err
}

// DeleteCorrection removes a correction whose approval could not be opened
// in core (nothing was decided; the requester simply retries). Its audit
// "requested" row is kept and a "withdrawn" row added so the trail stays
// complete.
func (s *Store) DeleteCorrection(ctx context.Context, c *AttendanceCorrection, reason string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM attendance_corrections WHERE id = $1 AND status = 'pending'`, c.ID); err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, AuditEntry{
		SchoolID: c.SchoolID, Module: "attendance", EntityType: "attendance_correction", EntityID: c.ID,
		Action: "attendance_correction.withdrawn", Reason: nilIfEmpty(reason),
		CorrelationID: nilIfEmpty("attendance_correction:" + c.ID),
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetCorrection(ctx context.Context, id string) (*AttendanceCorrection, error) {
	c, err := scanCorrection(s.DB.QueryRowContext(ctx, `SELECT `+correctionColumns+` FROM attendance_corrections WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

type CorrectionFilter struct {
	SchoolID, ClassroomID, Status, StudentID, RequestedBy string
}

func (s *Store) ListCorrections(ctx context.Context, f CorrectionFilter) ([]AttendanceCorrection, error) {
	where, args := []string{"1=1"}, []interface{}{}
	add := func(cond, v string) {
		if v != "" {
			args = append(args, v)
			where = append(where, fmt.Sprintf(cond, len(args)))
		}
	}
	add("school_id = $%d", f.SchoolID)
	add("classroom_id = $%d", f.ClassroomID)
	add("status = $%d", f.Status)
	add("student_id = $%d", f.StudentID)
	add("requested_by = $%d", f.RequestedBy)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+correctionColumns+` FROM attendance_corrections WHERE `+
		strings.Join(where, " AND ")+` ORDER BY created_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AttendanceCorrection{}
	for rows.Next() {
		c, err := scanCorrection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// CorrectionDecision is core's callback, reduced to what attendance needs.
type CorrectionDecision struct {
	CorrectionID string
	ApprovalID   string
	Decision     string // approved | rejected
	DecidedBy    string
	Comment      string
	DecidedAt    *time.Time
}

// ApplyCorrectionDecision applies core's decision in one transaction:
// approved -> the student's attendance for that day becomes to_status
// (bypassing the lock — this IS the authorised path), the correction is
// stamped, and an audit row records before/after. rejected -> only the
// stamp + audit. Idempotent: a correction already decided is returned
// unchanged with changed=false (core retries callbacks).
func (s *Store) ApplyCorrectionDecision(ctx context.Context, d CorrectionDecision) (c *AttendanceCorrection, changed bool, err error) {
	if d.Decision != "approved" && d.Decision != "rejected" {
		return nil, false, fmt.Errorf("invalid decision %q", d.Decision)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	c, err = scanCorrection(tx.QueryRowContext(ctx, `SELECT `+correctionColumns+` FROM attendance_corrections WHERE id = $1 FOR UPDATE`, d.CorrectionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrNotFound
	}
	if err != nil {
		return nil, false, err
	}
	if c.Status != "pending" {
		return c, false, nil
	}

	var before map[string]any
	if d.Decision == "approved" {
		var prevStatus string
		var prevCatatan sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT status, catatan FROM student_attendances
			WHERE classroom_id = $1 AND session_date = $2 AND student_id = $3 FOR UPDATE`,
			c.ClassroomID, c.SessionDate, c.StudentID).Scan(&prevStatus, &prevCatatan)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			before = map[string]any{"status": nil}
			termID := ""
			if c.TermID != nil {
				termID = *c.TermID
			}
			sessionID, err := findOrCreateManualSession(ctx, tx, c.SchoolID, c.AcademicYearID, termID, c.ClassroomID, c.SessionDate, c.RequestedBy)
			if err != nil {
				return nil, false, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO student_attendances
				(class_session_id, school_id, academic_year_id, term_id, classroom_id, session_date, student_id, status, catatan, marked_by)
				VALUES ($1, $2, $3, NULLIF($4,'')::uuid, $5, $6, $7, $8, $9, $10)`,
				sessionID, c.SchoolID, c.AcademicYearID, termID, c.ClassroomID, c.SessionDate, c.StudentID,
				c.ToStatus, c.Catatan, c.RequestedBy); err != nil {
				return nil, false, err
			}
		case err != nil:
			return nil, false, err
		default:
			before = map[string]any{"status": prevStatus, "catatan": nullStr(prevCatatan)}
			if _, err := tx.ExecContext(ctx, `UPDATE student_attendances
				SET status = $4, catatan = COALESCE($5, catatan), marked_by = $6, updated_at = now()
				WHERE classroom_id = $1 AND session_date = $2 AND student_id = $3`,
				c.ClassroomID, c.SessionDate, c.StudentID, c.ToStatus, c.Catatan, c.RequestedBy); err != nil {
				return nil, false, err
			}
		}
	} else {
		before = map[string]any{"status": c.FromStatus}
	}

	c, err = scanCorrection(tx.QueryRowContext(ctx, `
		UPDATE attendance_corrections SET status = $2, approval_id = COALESCE(NULLIF($3,''), approval_id),
			decided_by = NULLIF($4,''), decision_comment = NULLIF($5,''), decided_at = COALESCE($6, now()),
			applied_at = CASE WHEN $2 = 'approved' THEN now() END, updated_at = now()
		WHERE id = $1 RETURNING `+correctionColumns,
		c.ID, d.Decision, d.ApprovalID, d.DecidedBy, d.Comment, d.DecidedAt))
	if err != nil {
		return nil, false, err
	}
	after := map[string]any{"status": c.FromStatus, "correction_status": c.Status}
	if d.Decision == "approved" {
		after = map[string]any{"status": c.ToStatus, "catatan": c.Catatan, "correction_status": c.Status}
	}
	var decidedBy *string
	if isUUIDish(d.DecidedBy) {
		decidedBy = &d.DecidedBy
	}
	if err := writeAudit(ctx, tx, AuditEntry{
		SchoolID: c.SchoolID, Module: "attendance", EntityType: "student_attendance",
		EntityID: c.StudentID + ":" + c.SessionDate, Action: "attendance_correction." + d.Decision,
		ActorUserID: decidedBy, ActorRole: nilIfEmpty("approver"), Before: before, After: after,
		Reason: nilIfEmpty(c.Reason), CorrelationID: nilIfEmpty("attendance_correction:" + c.ID),
	}); err != nil {
		return nil, false, err
	}
	return c, true, tx.Commit()
}

func nullStr(n sql.NullString) any {
	if n.Valid {
		return n.String
	}
	return nil
}

// isUUIDish guards the uuid actor_user_id column against core sending an
// empty/non-uuid decided_by.
func isUUIDish(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, ch := range s {
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", ch) {
				return false
			}
		}
	}
	return true
}
