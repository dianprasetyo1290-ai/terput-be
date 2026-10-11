package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---- leave_requests (PRD menu 42 + 48) ---------------------------------

// ErrLeaveNotPending is returned when approving/rejecting a request that was
// already reviewed -- a review is final, never silently overwritten.
var ErrLeaveNotPending = errors.New("leave request is no longer pending")

// ErrLeaveOverlap: the student already has a pending/approved request
// covering one of the requested dates.
var ErrLeaveOverlap = errors.New("leave request overlaps an existing request")

type LeaveRequest struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	AcademicYearID *string    `json:"academic_year_id"`
	ClassroomID    *string    `json:"classroom_id"`
	StudentID      string     `json:"student_id"`
	RequestedBy    string     `json:"requested_by"`
	Type           string     `json:"type"`
	StartDate      string     `json:"start_date"`
	EndDate        string     `json:"end_date"`
	Reason         string     `json:"reason"`
	AttachmentName *string    `json:"attachment_name"`
	Status         string     `json:"status"`
	ReviewedBy     *string    `json:"reviewed_by"`
	ReviewedAt     *time.Time `json:"reviewed_at"`
	ReviewNote     *string    `json:"review_note"`
	CreatedAt      time.Time  `json:"created_at"`

	// Enrichment filled in by the handler (identity/academic lookups).
	StudentName   string `json:"student_name,omitempty"`
	ClassroomName string `json:"classroom_name,omitempty"`
	ReviewerName  string `json:"reviewer_name,omitempty"`
}

const leaveColumns = `id, school_id, academic_year_id, classroom_id, student_id, requested_by, type,
	to_char(start_date, 'YYYY-MM-DD'), to_char(end_date, 'YYYY-MM-DD'), reason, attachment_name,
	status, reviewed_by, reviewed_at, review_note, created_at`

func scanLeave(sc interface{ Scan(...interface{}) error }) (*LeaveRequest, error) {
	var l LeaveRequest
	var ay, cls, att, rb, note sql.NullString
	var ra sql.NullTime
	if err := sc.Scan(&l.ID, &l.SchoolID, &ay, &cls, &l.StudentID, &l.RequestedBy, &l.Type,
		&l.StartDate, &l.EndDate, &l.Reason, &att, &l.Status, &rb, &ra, &note, &l.CreatedAt); err != nil {
		return nil, err
	}
	str := func(n sql.NullString) *string {
		if n.Valid {
			return &n.String
		}
		return nil
	}
	l.AcademicYearID, l.ClassroomID, l.AttachmentName, l.ReviewedBy, l.ReviewNote = str(ay), str(cls), str(att), str(rb), str(note)
	if ra.Valid {
		l.ReviewedAt = &ra.Time
	}
	return &l, nil
}

type CreateLeaveInput struct {
	SchoolID, AcademicYearID, ClassroomID string // AY/classroom may be "" (student not enrolled)
	StudentID, RequestedBy                string
	Type, StartDate, EndDate, Reason      string
	AttachmentName, AttachmentType        string
	Attachment                            []byte
}

func (s *Store) CreateLeaveRequest(ctx context.Context, in CreateLeaveInput) (*LeaveRequest, error) {
	var overlap bool
	if err := s.DB.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM leave_requests
		WHERE student_id = $1 AND school_id = $2 AND status IN ('pending','approved')
		  AND start_date <= $4::date AND end_date >= $3::date)
	`, in.StudentID, in.SchoolID, in.StartDate, in.EndDate).Scan(&overlap); err != nil {
		return nil, err
	}
	if overlap {
		return nil, ErrLeaveOverlap
	}
	var att interface{}
	if len(in.Attachment) > 0 {
		att = in.Attachment
	}
	return scanLeave(s.DB.QueryRowContext(ctx, `
		INSERT INTO leave_requests (school_id, academic_year_id, classroom_id, student_id, requested_by,
			type, start_date, end_date, reason, attachment_name, attachment_type, attachment)
		VALUES ($1, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid, $4, $5, $6, $7, $8, $9, NULLIF($10,''), NULLIF($11,''), $12)
		RETURNING `+leaveColumns,
		in.SchoolID, in.AcademicYearID, in.ClassroomID, in.StudentID, in.RequestedBy,
		in.Type, in.StartDate, in.EndDate, in.Reason, in.AttachmentName, in.AttachmentType, att))
}

type LeaveFilter struct {
	SchoolID    string // "" = all schools (superadmin)
	StudentID   string
	ClassroomID string
	Status      string
}

func (s *Store) ListLeaveRequests(ctx context.Context, f LeaveFilter) ([]LeaveRequest, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(cond, val string) {
		if val == "" {
			return
		}
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	add("school_id = $%d", f.SchoolID)
	add("student_id = $%d", f.StudentID)
	add("classroom_id = $%d", f.ClassroomID)
	add("status = $%d", f.Status)
	// ponytail: capped at 500 rows, add pagination if a school ever exceeds it.
	rows, err := s.DB.QueryContext(ctx, `SELECT `+leaveColumns+` FROM leave_requests WHERE `+
		strings.Join(where, " AND ")+` ORDER BY (status = 'pending') DESC, created_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeaveRequest{}
	for rows.Next() {
		l, err := scanLeave(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

func (s *Store) GetLeaveRequest(ctx context.Context, id string) (*LeaveRequest, error) {
	l, err := scanLeave(s.DB.QueryRowContext(ctx, `SELECT `+leaveColumns+` FROM leave_requests WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return l, err
}

func (s *Store) GetLeaveAttachment(ctx context.Context, id string) (name, contentType string, data []byte, err error) {
	var n, t sql.NullString
	err = s.DB.QueryRowContext(ctx, `SELECT attachment_name, attachment_type, attachment FROM leave_requests WHERE id = $1`, id).
		Scan(&n, &t, &data)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(data) == 0) {
		return "", "", nil, ErrNotFound
	}
	return n.String, t.String, data, err
}

// ReviewLeaveRequest approves or rejects a pending request. On approval it
// applies BR-ATT-02 in the same transaction: every attendance row already
// recorded for the student inside the date range becomes izin/sakit. Days
// not yet recorded get the status when the guru's session seeds/bulk-marks
// them (see ApprovedLeaveTypes).
func (s *Store) ReviewLeaveRequest(ctx context.Context, id, status, reviewerID, note string) (*LeaveRequest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	l, err := scanLeave(tx.QueryRowContext(ctx, `
		UPDATE leave_requests SET status = $2, reviewed_by = $3, reviewed_at = now(),
			review_note = NULLIF($4,''), updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING `+leaveColumns, id, status, reviewerID, note))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLeaveNotPending
	}
	if err != nil {
		return nil, err
	}
	if status == "approved" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE student_attendances SET status = $1, catatan = $2, marked_by = $3, updated_at = now()
			WHERE student_id = $4 AND school_id = $5 AND session_date BETWEEN $6::date AND $7::date
		`, l.Type, leaveCatatan(l.Type, l.Reason), reviewerID, l.StudentID, l.SchoolID, l.StartDate, l.EndDate); err != nil {
			return nil, err
		}
	}
	return l, tx.Commit()
}

func leaveCatatan(typ, reason string) string {
	return "Pengajuan " + typ + " disetujui: " + reason
}

// ApprovedLeave is the attendance status + note an approved request imposes
// on one student for one date.
type ApprovedLeave struct{ Status, Catatan string }

// ApprovedLeaveTypes returns, for the given students on sessionDate, the
// status of any approved leave covering that date (BR-ATT-02). Used when a
// guru's session seeds or bulk-marks a roster so a pilih-semua "hadir"
// can't silently overwrite an approved izin/sakit.
func (s *Store) ApprovedLeaveTypes(ctx context.Context, schoolID, sessionDate string, studentIDs []string) (map[string]ApprovedLeave, error) {
	out := map[string]ApprovedLeave{}
	if len(studentIDs) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT student_id, type, reason FROM leave_requests
		WHERE school_id = $1 AND status = 'approved' AND $2::date BETWEEN start_date AND end_date
		  AND student_id::text = ANY(string_to_array($3, ','))
	`, schoolID, sessionDate, strings.Join(studentIDs, ","))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sid, typ, reason string
		if err := rows.Scan(&sid, &typ, &reason); err != nil {
			return nil, err
		}
		out[sid] = ApprovedLeave{Status: typ, Catatan: leaveCatatan(typ, reason)}
	}
	return out, rows.Err()
}
