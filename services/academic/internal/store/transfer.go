package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"skci/academic/internal/model"
)

type EnrollmentInfo struct {
	ID             string    `json:"id"`
	SchoolID       string    `json:"school_id"`
	AcademicYearID string    `json:"academic_year_id"`
	ClassroomID    string    `json:"classroom_id"`
	ClassroomName  string    `json:"classroom_name"`
	GradeLevel     string    `json:"grade_level"`
	StudentUserID  string    `json:"student_user_id"`
	Status         string    `json:"status"`
	EnrolledAt     time.Time `json:"enrolled_at"`
}

// GetActiveStudentEnrollment finds the student's active enrollment and classroom.
func (s *Store) GetActiveStudentEnrollment(ctx context.Context, schoolID, studentUserID string) (*EnrollmentInfo, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT ce.id, ce.school_id, ce.academic_year_id, ce.classroom_id,
		       COALESCE(c.name, ''), COALESCE(c.grade_level, ''), ce.student_user_id, ce.status, ce.enrolled_at
		FROM class_enrollments ce
		JOIN classrooms c ON c.id = ce.classroom_id
		WHERE ce.school_id = $1 AND ce.student_user_id = $2 AND ce.status = 'active'
		LIMIT 1
	`, schoolID, studentUserID)

	var info EnrollmentInfo
	err := row.Scan(&info.ID, &info.SchoolID, &info.AcademicYearID, &info.ClassroomID,
		&info.ClassroomName, &info.GradeLevel, &info.StudentUserID, &info.Status, &info.EnrolledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// CreateStudentStatusRequest creates a new status request, respecting idempotency.
func (s *Store) CreateStudentStatusRequest(ctx context.Context, req *model.StudentStatusRequest) (*model.StudentStatusRequest, error) {
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		existing, err := s.GetStudentStatusRequestByIdempotency(ctx, req.SchoolID, *req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	attachments := req.Attachments
	if len(attachments) == 0 {
		attachments = json.RawMessage("[]")
	}

	query := `
		INSERT INTO student_status_requests (
			school_id, student_user_id, kind, from_classroom_id, to_classroom_id, to_school_id,
			effective_date, reason, attachments, finance_snapshot, refund_amount_idr,
			status, approval_id, requested_by, idempotency_key
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		) RETURNING id, created_at, updated_at
	`

	err := s.DB.QueryRowContext(ctx, query,
		req.SchoolID, req.StudentUserID, req.Kind, req.FromClassroomID, req.ToClassroomID, req.ToSchoolID,
		req.EffectiveDate, req.Reason, attachments, req.FinanceSnapshot, req.RefundAmountIDR,
		req.Status, req.ApprovalID, req.RequestedBy, req.IdempotencyKey,
	).Scan(&req.ID, &req.CreatedAt, &req.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_student_status_requests_active" {
			return nil, ErrActiveStatusRequestExists
		}
		return nil, err
	}

	return req, nil
}

// ErrActiveStatusRequestExists: the student already has a pending/approved
// request (unique index uq_student_status_requests_active, migration 0007).
var ErrActiveStatusRequestExists = errors.New("siswa sudah memiliki pengajuan mutasi yang sedang berjalan")

const statusRequestColumns = `id, school_id, student_user_id, kind, from_classroom_id, to_classroom_id, to_school_id,
		       effective_date::text, reason, attachments, finance_snapshot, refund_amount_idr,
		       status, approval_id, applied_at, requested_by, idempotency_key, created_at, updated_at`

// FindActiveStatusRequest returns the student's running (pending_approval or
// approved, not yet applied) request, or nil.
func (s *Store) FindActiveStatusRequest(ctx context.Context, schoolID, studentUserID string) (*model.StudentStatusRequest, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT `+statusRequestColumns+`
		FROM student_status_requests
		WHERE school_id::text = $1 AND student_user_id::text = $2
		  AND status IN ('pending_approval', 'approved')
		ORDER BY created_at
		LIMIT 1
	`, schoolID, studentUserID)
	return scanStatusRequest(row)
}

// DeleteUnsubmittedStatusRequest removes a request that never reached the
// Approval Center (submit failed), so the user can fix the configuration
// and submit again (same idempotency key included).
func (s *Store) DeleteUnsubmittedStatusRequest(ctx context.Context, schoolID, id string) error {
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM student_status_requests
		WHERE school_id::text = $1 AND id::text = $2
		  AND status = 'pending_approval' AND approval_id IS NULL
	`, schoolID, id)
	return err
}

// TransitionStatusRequest moves a request from one status to another only if
// it is still in `from` (compare-and-set). approvalID is kept when empty.
// Reports false when the row was not in `from` any more.
func (s *Store) TransitionStatusRequest(ctx context.Context, schoolID, id, from, to, approvalID string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE student_status_requests
		SET status = $1,
		    approval_id = COALESCE(NULLIF($2, '')::uuid, approval_id),
		    updated_at = now()
		WHERE ($3 = '' OR school_id::text = $3) AND id::text = $4 AND status = $5
	`, to, approvalID, schoolID, id, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// GetStudentStatusRequestByIdempotency looks up a request by (school_id, idempotency_key).
func (s *Store) GetStudentStatusRequestByIdempotency(ctx context.Context, schoolID, key string) (*model.StudentStatusRequest, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, student_user_id, kind, from_classroom_id, to_classroom_id, to_school_id,
		       effective_date::text, reason, attachments, finance_snapshot, refund_amount_idr,
		       status, approval_id, applied_at, requested_by, idempotency_key, created_at, updated_at
		FROM student_status_requests
		WHERE school_id = $1 AND idempotency_key = $2
	`, schoolID, key)
	return scanStatusRequest(row)
}

// GetStudentStatusRequest finds a status request by ID.
func (s *Store) GetStudentStatusRequest(ctx context.Context, schoolID, id string) (*model.StudentStatusRequest, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, student_user_id, kind, from_classroom_id, to_classroom_id, to_school_id,
		       effective_date::text, reason, attachments, finance_snapshot, refund_amount_idr,
		       status, approval_id, applied_at, requested_by, idempotency_key, created_at, updated_at
		FROM student_status_requests
		WHERE ($1 = '' OR school_id::text = $1) AND id::text = $2
	`, schoolID, id)
	return scanStatusRequest(row)
}

// ListStudentStatusRequests returns a paginated list of status requests.
func (s *Store) ListStudentStatusRequests(ctx context.Context, schoolID, kind, status, studentUserID string, limit, offset int) ([]model.StudentStatusRequest, int, error) {
	where := `WHERE ($1 = '' OR school_id::text = $1)`
	args := []any{schoolID}

	if kind != "" {
		args = append(args, kind)
		where += fmt.Sprintf(" AND kind = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if studentUserID != "" {
		args = append(args, studentUserID)
		where += fmt.Sprintf(" AND student_user_id::text = $%d", len(args))
	}

	var total int
	countQuery := `SELECT count(*) FROM student_status_requests ` + where
	if err := s.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, school_id, student_user_id, kind, from_classroom_id, to_classroom_id, to_school_id,
		       effective_date::text, reason, attachments, finance_snapshot, refund_amount_idr,
		       status, approval_id, applied_at, requested_by, idempotency_key, created_at, updated_at
		FROM student_status_requests
		` + where + `
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`
	args = append(args, limit, offset)
	query = fmt.Sprintf(query, len(args)-1, len(args))

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []model.StudentStatusRequest
	for rows.Next() {
		var r model.StudentStatusRequest
		var fromClass, toClass, toSchool, approvalID, idempotencyKey sql.NullString
		var refund sql.NullInt64
		var appliedAt sql.NullTime
		var attachBytes, snapBytes []byte

		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.StudentUserID, &r.Kind, &fromClass, &toClass, &toSchool,
			&r.EffectiveDate, &r.Reason, &attachBytes, &snapBytes, &refund,
			&r.Status, &approvalID, &appliedAt, &r.RequestedBy, &idempotencyKey, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}

		if fromClass.Valid {
			r.FromClassroomID = &fromClass.String
		}
		if toClass.Valid {
			r.ToClassroomID = &toClass.String
		}
		if toSchool.Valid {
			r.ToSchoolID = &toSchool.String
		}
		if approvalID.Valid {
			r.ApprovalID = &approvalID.String
		}
		if idempotencyKey.Valid {
			r.IdempotencyKey = &idempotencyKey.String
		}
		if refund.Valid {
			r.RefundAmountIDR = &refund.Int64
		}
		if appliedAt.Valid {
			r.AppliedAt = &appliedAt.Time
		}
		r.Attachments = json.RawMessage(attachBytes)
		if len(snapBytes) > 0 {
			r.FinanceSnapshot = json.RawMessage(snapBytes)
		}

		list = append(list, r)
	}

	return list, total, rows.Err()
}

// UpdateStatusRequestApproval updates approval status and ID.
func (s *Store) UpdateStatusRequestApproval(ctx context.Context, schoolID, id, status, approvalID string) error {
	query := `
		UPDATE student_status_requests
		SET status = $1,
		    approval_id = COALESCE(NULLIF($2, '')::uuid, approval_id),
		    updated_at = now()
		WHERE ($3 = '' OR school_id::text = $3) AND id::text = $4
	`
	_, err := s.DB.ExecContext(ctx, query, status, approvalID, schoolID, id)
	return err
}

// MarkStatusRequestApplied marks the request as applied idempotently.
func (s *Store) MarkStatusRequestApplied(ctx context.Context, schoolID, id string) error {
	query := `
		UPDATE student_status_requests
		SET status = 'applied',
		    applied_at = COALESCE(applied_at, now()),
		    updated_at = now()
		WHERE ($1 = '' OR school_id::text = $1) AND id::text = $2
	`
	_, err := s.DB.ExecContext(ctx, query, schoolID, id)
	return err
}

// ApplyClassTransfer moves the student's active enrollment to
// toClassroomID inside a transaction. The (academic_year_id, student_user_id)
// row is re-pointed in a single UPDATE, which the history trigger records as
// one 'transfer' entry (from -> to); the previous implementation flipped the
// row to transferred_out and then upserted it back, losing the old classroom.
// Refuses with *CapacityError when the target classroom is full.
func (s *Store) ApplyClassTransfer(ctx context.Context, schoolID, studentUserID, fromClassroomID, toClassroomID string, ch EnrollmentChange) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Find the active enrollment: prefer the one in fromClassroomID,
	// otherwise any active enrollment of the student.
	var enrollmentID, currentClassroomID string
	err = tx.QueryRowContext(ctx, `
		SELECT id, classroom_id FROM class_enrollments
		WHERE school_id = $1 AND student_user_id = $2 AND status = 'active'
		ORDER BY (classroom_id::text = $3) DESC, enrolled_at DESC
		LIMIT 1
	`, schoolID, studentUserID, fromClassroomID).Scan(&enrollmentID, &currentClassroomID)
	if err == sql.ErrNoRows {
		return nil // no active enrollment: nothing to move (same as before)
	}
	if err != nil {
		return err
	}
	if currentClassroomID == toClassroomID {
		return tx.Commit() // already there (idempotent replay)
	}

	if err := checkCapacity(ctx, tx, toClassroomID, studentUserID); err != nil {
		return err
	}
	if ch.Action == "" {
		ch.Action = ActionTransfer
	}
	if err := setEnrollmentContext(ctx, tx, ch); err != nil {
		return err
	}

	// 2. Close any OTHER active enrollment (other academic years) as before.
	if _, err := tx.ExecContext(ctx, `
		UPDATE class_enrollments
		SET status = 'transferred_out', updated_at = now()
		WHERE school_id = $1 AND student_user_id = $2 AND status = 'active' AND id <> $3
	`, schoolID, studentUserID, enrollmentID); err != nil {
		return err
	}

	// 3. Re-point the enrollment at the new classroom.
	if _, err := tx.ExecContext(ctx, `
		UPDATE class_enrollments
		SET classroom_id = $2, status = 'active', updated_at = now()
		WHERE id = $1
	`, enrollmentID, toClassroomID); err != nil {
		return err
	}

	return tx.Commit()
}

// setActiveEnrollmentStatus closes every active enrollment of the student
// with newStatus, tagging the history rows with ch.
func (s *Store) setActiveEnrollmentStatus(ctx context.Context, schoolID, studentUserID, newStatus string, ch EnrollmentChange) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setEnrollmentContext(ctx, tx, ch); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE class_enrollments
		SET status = $3, updated_at = now()
		WHERE school_id = $1 AND student_user_id = $2 AND status = 'active'
	`, schoolID, studentUserID, newStatus); err != nil {
		return err
	}
	return tx.Commit()
}

// ApplyWithdrawal sets active enrollment status to 'withdrawn'.
func (s *Store) ApplyWithdrawal(ctx context.Context, schoolID, studentUserID string, ch EnrollmentChange) error {
	if ch.Action == "" {
		ch.Action = ActionWithdrawal
	}
	return s.setActiveEnrollmentStatus(ctx, schoolID, studentUserID, "withdrawn", ch)
}

// ApplyUnitTransfer sets active enrollment status to 'transferred_out'.
func (s *Store) ApplyUnitTransfer(ctx context.Context, schoolID, studentUserID string, ch EnrollmentChange) error {
	if ch.Action == "" {
		ch.Action = ActionUnitTransfer
	}
	return s.setActiveEnrollmentStatus(ctx, schoolID, studentUserID, "transferred_out", ch)
}

// ListApprovedPendingRequests finds approved requests whose effective_date <= today and not yet applied.
func (s *Store) ListApprovedPendingRequests(ctx context.Context, schoolID, today string) ([]model.StudentStatusRequest, error) {
	query := `
		SELECT id, school_id, student_user_id, kind, from_classroom_id, to_classroom_id, to_school_id,
		       effective_date::text, reason, attachments, finance_snapshot, refund_amount_idr,
		       status, approval_id, applied_at, requested_by, idempotency_key, created_at, updated_at
		FROM student_status_requests
		WHERE ($1 = '' OR school_id::text = $1) AND status = 'approved' AND applied_at IS NULL AND effective_date <= $2::date
		ORDER BY effective_date ASC
	`
	rows, err := s.DB.QueryContext(ctx, query, schoolID, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.StudentStatusRequest
	for rows.Next() {
		var r model.StudentStatusRequest
		var fromClass, toClass, toSchool, approvalID, idempotencyKey sql.NullString
		var refund sql.NullInt64
		var appliedAt sql.NullTime
		var attachBytes, snapBytes []byte

		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.StudentUserID, &r.Kind, &fromClass, &toClass, &toSchool,
			&r.EffectiveDate, &r.Reason, &attachBytes, &snapBytes, &refund,
			&r.Status, &approvalID, &appliedAt, &r.RequestedBy, &idempotencyKey, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if fromClass.Valid {
			r.FromClassroomID = &fromClass.String
		}
		if toClass.Valid {
			r.ToClassroomID = &toClass.String
		}
		if toSchool.Valid {
			r.ToSchoolID = &toSchool.String
		}
		if approvalID.Valid {
			r.ApprovalID = &approvalID.String
		}
		if idempotencyKey.Valid {
			r.IdempotencyKey = &idempotencyKey.String
		}
		if refund.Valid {
			r.RefundAmountIDR = &refund.Int64
		}
		if appliedAt.Valid {
			r.AppliedAt = &appliedAt.Time
		}
		r.Attachments = json.RawMessage(attachBytes)
		if len(snapBytes) > 0 {
			r.FinanceSnapshot = json.RawMessage(snapBytes)
		}

		list = append(list, r)
	}
	return list, rows.Err()
}

// GetAcademicSettings retrieves academic settings for a school, defaulting if not found.
func (s *Store) GetAcademicSettings(ctx context.Context, schoolID string) (*model.AcademicSettings, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT school_id, post_exit_bill_policy, parent_grace_days, created_at, updated_at
		FROM academic_settings
		WHERE school_id = $1
	`, schoolID)

	var st model.AcademicSettings
	err := row.Scan(&st.SchoolID, &st.PostExitBillPolicy, &st.ParentGraceDays, &st.CreatedAt, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return &model.AcademicSettings{
			SchoolID:           schoolID,
			PostExitBillPolicy: model.PolicyVoid,
			ParentGraceDays:    30,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// UpsertAcademicSettings saves academic settings for a school.
func (s *Store) UpsertAcademicSettings(ctx context.Context, settings model.AcademicSettings) error {
	policy := settings.PostExitBillPolicy
	if policy != model.PolicyKeep {
		policy = model.PolicyVoid
	}
	grace := settings.ParentGraceDays
	if grace < 0 {
		grace = 30
	}

	query := `
		INSERT INTO academic_settings (school_id, post_exit_bill_policy, parent_grace_days, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())
		ON CONFLICT (school_id)
		DO UPDATE SET post_exit_bill_policy = EXCLUDED.post_exit_bill_policy,
		              parent_grace_days = EXCLUDED.parent_grace_days,
		              updated_at = now()
	`
	_, err := s.DB.ExecContext(ctx, query, settings.SchoolID, policy, grace)
	return err
}

func scanStatusRequest(row *sql.Row) (*model.StudentStatusRequest, error) {
	var r model.StudentStatusRequest
	var fromClass, toClass, toSchool, approvalID, idempotencyKey sql.NullString
	var refund sql.NullInt64
	var appliedAt sql.NullTime
	var attachBytes, snapBytes []byte

	err := row.Scan(
		&r.ID, &r.SchoolID, &r.StudentUserID, &r.Kind, &fromClass, &toClass, &toSchool,
		&r.EffectiveDate, &r.Reason, &attachBytes, &snapBytes, &refund,
		&r.Status, &approvalID, &appliedAt, &r.RequestedBy, &idempotencyKey, &r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if fromClass.Valid {
		r.FromClassroomID = &fromClass.String
	}
	if toClass.Valid {
		r.ToClassroomID = &toClass.String
	}
	if toSchool.Valid {
		r.ToSchoolID = &toSchool.String
	}
	if approvalID.Valid {
		r.ApprovalID = &approvalID.String
	}
	if idempotencyKey.Valid {
		r.IdempotencyKey = &idempotencyKey.String
	}
	if refund.Valid {
		r.RefundAmountIDR = &refund.Int64
	}
	if appliedAt.Valid {
		r.AppliedAt = &appliedAt.Time
	}
	r.Attachments = json.RawMessage(attachBytes)
	if len(snapBytes) > 0 {
		r.FinanceSnapshot = json.RawMessage(snapBytes)
	}

	return &r, nil
}
