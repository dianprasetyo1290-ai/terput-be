package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"skci/academic/internal/model"
)

type EnrollmentInfo struct {
	ID             string `json:"id"`
	SchoolID       string `json:"school_id"`
	AcademicYearID string `json:"academic_year_id"`
	ClassroomID    string `json:"classroom_id"`
	ClassroomName  string `json:"classroom_name"`
	GradeLevel     string `json:"grade_level"`
	StudentUserID  string `json:"student_user_id"`
	Status         string `json:"status"`
}

// GetActiveStudentEnrollment finds the student's active enrollment and classroom.
func (s *Store) GetActiveStudentEnrollment(ctx context.Context, schoolID, studentUserID string) (*EnrollmentInfo, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT ce.id, ce.school_id, ce.academic_year_id, ce.classroom_id,
		       COALESCE(c.name, ''), COALESCE(c.grade_level, ''), ce.student_user_id, ce.status
		FROM class_enrollments ce
		JOIN classrooms c ON c.id = ce.classroom_id
		WHERE ce.school_id = $1 AND ce.student_user_id = $2 AND ce.status = 'active'
		LIMIT 1
	`, schoolID, studentUserID)

	var info EnrollmentInfo
	err := row.Scan(&info.ID, &info.SchoolID, &info.AcademicYearID, &info.ClassroomID,
		&info.ClassroomName, &info.GradeLevel, &info.StudentUserID, &info.Status)
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
		return nil, err
	}

	return req, nil
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

// ApplyClassTransfer executes the class transfer mutation inside a transaction.
func (s *Store) ApplyClassTransfer(ctx context.Context, schoolID, studentUserID, fromClassroomID, toClassroomID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Get the current active enrollment's academic_year_id
	var academicYearID string
	err = tx.QueryRowContext(ctx, `
		SELECT academic_year_id FROM class_enrollments
		WHERE school_id = $1 AND student_user_id = $2 AND classroom_id = $3 AND status = 'active'
		LIMIT 1
	`, schoolID, studentUserID, fromClassroomID).Scan(&academicYearID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if academicYearID == "" {
		// Try finding any active enrollment for student
		_ = tx.QueryRowContext(ctx, `
			SELECT academic_year_id FROM class_enrollments
			WHERE school_id = $1 AND student_user_id = $2 AND status = 'active'
			LIMIT 1
		`, schoolID, studentUserID).Scan(&academicYearID)
	}

	// 2. Close old enrollment: set status = 'transferred_out'
	_, err = tx.ExecContext(ctx, `
		UPDATE class_enrollments
		SET status = 'transferred_out', updated_at = now()
		WHERE school_id = $1 AND student_user_id = $2 AND status = 'active'
	`, schoolID, studentUserID)
	if err != nil {
		return err
	}

	// 3. Open new enrollment in toClassroomID
	if academicYearID != "" {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
			VALUES ($1, $2, $3, $4, 'active')
			ON CONFLICT (academic_year_id, student_user_id)
			DO UPDATE SET classroom_id = EXCLUDED.classroom_id, status = 'active', updated_at = now()
		`, schoolID, academicYearID, toClassroomID, studentUserID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ApplyWithdrawal sets active enrollment status to 'withdrawn'.
func (s *Store) ApplyWithdrawal(ctx context.Context, schoolID, studentUserID string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE class_enrollments
		SET status = 'withdrawn', updated_at = now()
		WHERE school_id = $1 AND student_user_id = $2 AND status = 'active'
	`, schoolID, studentUserID)
	return err
}

// ApplyUnitTransfer sets active enrollment status to 'transferred_out'.
func (s *Store) ApplyUnitTransfer(ctx context.Context, schoolID, studentUserID string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE class_enrollments
		SET status = 'transferred_out', updated_at = now()
		WHERE school_id = $1 AND student_user_id = $2 AND status = 'active'
	`, schoolID, studentUserID)
	return err
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
