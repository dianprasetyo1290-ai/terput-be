package counseling

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"skci/studentlife/internal/store"
)

var (
	ErrNotFound      = store.ErrNotFound
	ErrAlreadyExists = store.ErrAlreadyExists
)

type Store struct {
	DB *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{DB: db}
}

// Categories
func (s *Store) ListCategories(ctx context.Context, schoolID string) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, name, active, created_at, updated_at
		FROM counseling_categories
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.Name, &c.Active, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) CreateCategory(ctx context.Context, c Category) (*Category, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO counseling_categories (school_id, name, active)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`,
		c.SchoolID, c.Name, c.Active).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) UpdateCategory(ctx context.Context, c Category) (*Category, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE counseling_categories
		SET name = $1, active = $2, updated_at = now()
		WHERE id = $3 AND school_id = $4 AND deleted_at IS NULL`,
		c.Name, c.Active, c.ID, c.SchoolID)
	if err != nil {
		return nil, err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return nil, ErrNotFound
	}
	return &c, nil
}

func (s *Store) DeleteCategory(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE counseling_categories
		SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL`,
		id, schoolID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

// Cases
func (s *Store) ListCases(ctx context.Context, schoolID string, status, studentID, memberUserID string) ([]Case, error) {
	query := `
		SELECT c.id, c.school_id, c.student_id, c.category_id, cat.name, c.opened_by,
		       c.status, c.disclose_to_homeroom, c.opened_at, c.closed_at, c.created_at, c.updated_at
		FROM counseling_cases c
		JOIN counseling_categories cat ON c.category_id = cat.id
		WHERE c.school_id = $1`
	args := []any{schoolID}
	idx := 2

	if status != "" {
		query += ` AND c.status = $` + string(rune('0'+idx))
		args = append(args, status)
		idx++
	}
	if studentID != "" {
		query += ` AND c.student_id = $` + string(rune('0'+idx))
		args = append(args, studentID)
		idx++
	}
	if memberUserID != "" {
		query += ` AND EXISTS (
			SELECT 1 FROM counseling_case_members m
			WHERE m.case_id = c.id AND m.user_id = $` + string(rune('0'+idx)) + ` AND m.removed_at IS NULL
		)`
		args = append(args, memberUserID)
		idx++
	}

	query += ` ORDER BY c.opened_at DESC`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Case
	for rows.Next() {
		var c Case
		if err := rows.Scan(
			&c.ID, &c.SchoolID, &c.StudentID, &c.CategoryID, &c.CategoryName, &c.OpenedBy,
			&c.Status, &c.DiscloseToHomeroom, &c.OpenedAt, &c.ClosedAt, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) GetCase(ctx context.Context, schoolID, id string) (*Case, error) {
	var c Case
	err := s.DB.QueryRowContext(ctx, `
		SELECT c.id, c.school_id, c.student_id, c.category_id, cat.name, c.opened_by,
		       c.status, c.disclose_to_homeroom, c.opened_at, c.closed_at, c.close_reason_enc,
		       c.created_at, c.updated_at
		FROM counseling_cases c
		JOIN counseling_categories cat ON c.category_id = cat.id
		WHERE c.id = $1 AND c.school_id = $2`, id, schoolID).Scan(
		&c.ID, &c.SchoolID, &c.StudentID, &c.CategoryID, &c.CategoryName, &c.OpenedBy,
		&c.Status, &c.DiscloseToHomeroom, &c.OpenedAt, &c.ClosedAt, &c.CloseReasonEnc,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	members, err := s.ListCaseMembers(ctx, c.ID)
	if err == nil {
		c.Members = members
	}

	return &c, nil
}

func (s *Store) CreateCase(ctx context.Context, c Case, memberRole string) (*Case, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO counseling_cases (school_id, student_id, category_id, opened_by, status, disclose_to_homeroom)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, opened_at, created_at, updated_at`,
		c.SchoolID, c.StudentID, c.CategoryID, c.OpenedBy, CaseStatusOpen, c.DiscloseToHomeroom,
	).Scan(&c.ID, &c.OpenedAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Add creator as member
	if memberRole == "" {
		memberRole = "counselor"
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO counseling_case_members (case_id, user_id, member_role, added_by)
		VALUES ($1, $2, $3, $4)`,
		c.ID, c.OpenedBy, memberRole, c.OpenedBy)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	c.Status = CaseStatusOpen
	return &c, nil
}

func (s *Store) UpdateCase(ctx context.Context, c Case) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE counseling_cases
		SET category_id = $1, status = $2, disclose_to_homeroom = $3, updated_at = now()
		WHERE id = $4 AND school_id = $5`,
		c.CategoryID, c.Status, c.DiscloseToHomeroom, c.ID, c.SchoolID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CloseCase(ctx context.Context, schoolID, id string, closeReasonEnc []byte, closedAt time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE counseling_cases
		SET status = $1, closed_at = $2, close_reason_enc = $3, updated_at = now()
		WHERE id = $4 AND school_id = $5 AND status <> 'closed'`,
		CaseStatusClosed, closedAt, closeReasonEnc, id, schoolID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) IsCaseMember(ctx context.Context, caseID, userID string) (bool, string, error) {
	var role string
	err := s.DB.QueryRowContext(ctx, `
		SELECT member_role FROM counseling_case_members
		WHERE case_id = $1 AND user_id = $2 AND removed_at IS NULL`,
		caseID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, role, nil
}

func (s *Store) ListCaseMembers(ctx context.Context, caseID string) ([]Member, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT case_id, user_id, member_role, added_by, added_at, removed_at
		FROM counseling_case_members
		WHERE case_id = $1 AND removed_at IS NULL
		ORDER BY added_at ASC`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.CaseID, &m.UserID, &m.MemberRole, &m.AddedBy, &m.AddedAt, &m.RemovedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *Store) AddCaseMember(ctx context.Context, caseID, userID, role, addedBy string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO counseling_case_members (case_id, user_id, member_role, added_by, added_at, removed_at)
		VALUES ($1, $2, $3, $4, now(), NULL)
		ON CONFLICT (case_id, user_id) DO UPDATE
		SET member_role = EXCLUDED.member_role, removed_at = NULL, added_at = now()`,
		caseID, userID, role, addedBy)
	return err
}

func (s *Store) RemoveCaseMember(ctx context.Context, caseID, userID string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE counseling_case_members
		SET removed_at = now()
		WHERE case_id = $1 AND user_id = $2 AND removed_at IS NULL`,
		caseID, userID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetStudentCasesForHomeroom(ctx context.Context, schoolID, studentID string) ([]Case, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, status, disclose_to_homeroom
		FROM counseling_cases
		WHERE school_id = $1 AND student_id = $2 AND status IN ('open', 'monitoring')`,
		schoolID, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Case
	for rows.Next() {
		var c Case
		if err := rows.Scan(&c.ID, &c.Status, &c.DiscloseToHomeroom); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// Sessions
func (s *Store) ListSessions(ctx context.Context, schoolID, counselorUserID, caseID string, from, to *time.Time) ([]Session, error) {
	query := `
		SELECT s.id, s.school_id, s.case_id, s.kind, s.category_id, cat.name,
		       s.counselor_user_id, s.scheduled_at, s.held_at, s.status, s.reminder_sent_at,
		       s.created_at, s.updated_at
		FROM counseling_sessions s
		JOIN counseling_categories cat ON s.category_id = cat.id
		WHERE s.school_id = $1`
	args := []any{schoolID}
	idx := 2

	if counselorUserID != "" {
		query += ` AND s.counselor_user_id = $` + string(rune('0'+idx))
		args = append(args, counselorUserID)
		idx++
	}
	if caseID != "" {
		query += ` AND s.case_id = $` + string(rune('0'+idx))
		args = append(args, caseID)
		idx++
	}
	if from != nil {
		query += ` AND s.scheduled_at >= $` + string(rune('0'+idx))
		args = append(args, *from)
		idx++
	}
	if to != nil {
		query += ` AND s.scheduled_at <= $` + string(rune('0'+idx))
		args = append(args, *to)
		idx++
	}

	query += ` ORDER BY s.scheduled_at DESC`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(
			&sess.ID, &sess.SchoolID, &sess.CaseID, &sess.Kind, &sess.CategoryID, &sess.CategoryName,
			&sess.CounselorUserID, &sess.ScheduledAt, &sess.HeldAt, &sess.Status, &sess.ReminderSentAt,
			&sess.CreatedAt, &sess.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, sess)
	}

	// Fetch student IDs for each session
	for i := range list {
		stdRows, err := s.DB.QueryContext(ctx, `
			SELECT student_id FROM counseling_session_students WHERE session_id = $1`, list[i].ID)
		if err == nil {
			for stdRows.Next() {
				var stdID string
				if err := stdRows.Scan(&stdID); err == nil {
					list[i].StudentIDs = append(list[i].StudentIDs, stdID)
				}
			}
			stdRows.Close()
		}
	}

	return list, rows.Err()
}

func (s *Store) GetSession(ctx context.Context, schoolID, id string) (*Session, error) {
	var sess Session
	err := s.DB.QueryRowContext(ctx, `
		SELECT s.id, s.school_id, s.case_id, s.kind, s.category_id, cat.name,
		       s.counselor_user_id, s.scheduled_at, s.held_at, s.status, s.notes_enc, s.follow_up_plan_enc,
		       s.reminder_sent_at, s.created_at, s.updated_at
		FROM counseling_sessions s
		JOIN counseling_categories cat ON s.category_id = cat.id
		WHERE s.id = $1 AND s.school_id = $2`, id, schoolID).Scan(
		&sess.ID, &sess.SchoolID, &sess.CaseID, &sess.Kind, &sess.CategoryID, &sess.CategoryName,
		&sess.CounselorUserID, &sess.ScheduledAt, &sess.HeldAt, &sess.Status, &sess.NotesEnc, &sess.FollowUpPlanEnc,
		&sess.ReminderSentAt, &sess.CreatedAt, &sess.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT student_id FROM counseling_session_students WHERE session_id = $1`, sess.ID)
	if err == nil {
		for rows.Next() {
			var stdID string
			if err := rows.Scan(&stdID); err == nil {
				sess.StudentIDs = append(sess.StudentIDs, stdID)
			}
		}
		rows.Close()
	}

	return &sess, nil
}

func (s *Store) CreateSession(ctx context.Context, sess Session, studentIDs []string) (*Session, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO counseling_sessions
			(school_id, case_id, kind, category_id, counselor_user_id, scheduled_at, held_at, status, notes_enc, follow_up_plan_enc)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`,
		sess.SchoolID, sess.CaseID, sess.Kind, sess.CategoryID, sess.CounselorUserID,
		sess.ScheduledAt, sess.HeldAt, sess.Status, sess.NotesEnc, sess.FollowUpPlanEnc,
	).Scan(&sess.ID, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, err
	}

	for _, sid := range studentIDs {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO counseling_session_students (session_id, student_id)
			VALUES ($1, $2) ON CONFLICT DO NOTHING`, sess.ID, sid)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	sess.StudentIDs = studentIDs
	return &sess, nil
}

func (s *Store) UpdateSession(ctx context.Context, sess Session, studentIDs []string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE counseling_sessions
		SET category_id = $1, scheduled_at = $2, held_at = $3, status = $4,
		    notes_enc = COALESCE($5, notes_enc), follow_up_plan_enc = COALESCE($6, follow_up_plan_enc),
		    updated_at = now()
		WHERE id = $7 AND school_id = $8`,
		sess.CategoryID, sess.ScheduledAt, sess.HeldAt, sess.Status,
		sess.NotesEnc, sess.FollowUpPlanEnc, sess.ID, sess.SchoolID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}

	if len(studentIDs) > 0 {
		_, _ = tx.ExecContext(ctx, `DELETE FROM counseling_session_students WHERE session_id = $1`, sess.ID)
		for _, sid := range studentIDs {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO counseling_session_students (session_id, student_id)
				VALUES ($1, $2) ON CONFLICT DO NOTHING`, sess.ID, sid)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// Referrals
func (s *Store) ListReferrals(ctx context.Context, schoolID, caseID string) ([]Referral, error) {
	query := `
		SELECT r.id, r.school_id, r.case_id, r.student_id, r.target_kind,
		       r.target_name_enc, r.target_contact_enc, r.reason_enc, r.parent_summary_enc,
		       r.status, r.consent_id, r.outcome_enc, r.created_by, r.created_at, r.updated_at,
		       COALESCE(pc.status, '') as consent_status
		FROM referrals r
		LEFT JOIN parent_consents pc ON r.consent_id = pc.id
		WHERE r.school_id = $1`
	args := []any{schoolID}
	if caseID != "" {
		query += ` AND r.case_id = $2`
		args = append(args, caseID)
	}
	query += ` ORDER BY r.created_at DESC`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Referral
	for rows.Next() {
		var r Referral
		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.CaseID, &r.StudentID, &r.TargetKind,
			&r.TargetNameEnc, &r.TargetContactEnc, &r.ReasonEnc, &r.ParentSummaryEnc,
			&r.Status, &r.ConsentID, &r.OutcomeEnc, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
			&r.ConsentStatus,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *Store) GetReferral(ctx context.Context, schoolID, id string) (*Referral, error) {
	var r Referral
	err := s.DB.QueryRowContext(ctx, `
		SELECT r.id, r.school_id, r.case_id, r.student_id, r.target_kind,
		       r.target_name_enc, r.target_contact_enc, r.reason_enc, r.parent_summary_enc,
		       r.status, r.consent_id, r.outcome_enc, r.created_by, r.created_at, r.updated_at,
		       COALESCE(pc.status, '') as consent_status
		FROM referrals r
		LEFT JOIN parent_consents pc ON r.consent_id = pc.id
		WHERE r.id = $1 AND r.school_id = $2`, id, schoolID).Scan(
		&r.ID, &r.SchoolID, &r.CaseID, &r.StudentID, &r.TargetKind,
		&r.TargetNameEnc, &r.TargetContactEnc, &r.ReasonEnc, &r.ParentSummaryEnc,
		&r.Status, &r.ConsentID, &r.OutcomeEnc, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
		&r.ConsentStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetReferralByConsentID(ctx context.Context, consentID string) (*Referral, error) {
	var r Referral
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, case_id, student_id, target_kind,
		       target_name_enc, target_contact_enc, reason_enc, parent_summary_enc,
		       status, consent_id, outcome_enc, created_by, created_at, updated_at
		FROM referrals
		WHERE consent_id = $1`, consentID).Scan(
		&r.ID, &r.SchoolID, &r.CaseID, &r.StudentID, &r.TargetKind,
		&r.TargetNameEnc, &r.TargetContactEnc, &r.ReasonEnc, &r.ParentSummaryEnc,
		&r.Status, &r.ConsentID, &r.OutcomeEnc, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) CreateReferral(ctx context.Context, r Referral) (*Referral, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO referrals
			(school_id, case_id, student_id, target_kind, target_name_enc, target_contact_enc, reason_enc, parent_summary_enc, status, consent_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at`,
		r.SchoolID, r.CaseID, r.StudentID, r.TargetKind, r.TargetNameEnc, r.TargetContactEnc,
		r.ReasonEnc, r.ParentSummaryEnc, r.Status, r.ConsentID, r.CreatedBy,
	).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) UpdateReferralStatus(ctx context.Context, schoolID, id, status string, consentID *string, outcomeEnc []byte) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE referrals
		SET status = $1, consent_id = COALESCE($2, consent_id), outcome_enc = COALESCE($3, outcome_enc), updated_at = now()
		WHERE id = $4 AND school_id = $5`,
		status, consentID, outcomeEnc, id, schoolID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

// Tasks
func (s *Store) ListCaseTasks(ctx context.Context, schoolID, caseID string) ([]Task, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, case_id, title, assignee_user_id, due_date::text,
		       done_at, done_by, note_enc, created_by, created_at, updated_at
		FROM case_tasks
		WHERE school_id = $1 AND case_id = $2
		ORDER BY due_date ASC, created_at ASC`, schoolID, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(
			&t.ID, &t.SchoolID, &t.CaseID, &t.Title, &t.AssigneeUserID, &t.DueDate,
			&t.DoneAt, &t.DoneBy, &t.NoteEnc, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) ListMyTasks(ctx context.Context, schoolID, assigneeUserID string) ([]Task, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT t.id, t.school_id, t.case_id, t.title, t.assignee_user_id, t.due_date::text,
		       t.done_at, t.done_by, t.note_enc, t.created_by, t.created_at, t.updated_at
		FROM case_tasks t
		WHERE t.school_id = $1 AND t.assignee_user_id = $2
		ORDER BY t.done_at IS NOT NULL ASC, t.due_date ASC`, schoolID, assigneeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(
			&t.ID, &t.SchoolID, &t.CaseID, &t.Title, &t.AssigneeUserID, &t.DueDate,
			&t.DoneAt, &t.DoneBy, &t.NoteEnc, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) CreateTask(ctx context.Context, t Task) (*Task, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO case_tasks
			(school_id, case_id, title, assignee_user_id, due_date, note_enc, created_by)
		VALUES ($1, $2, $3, $4, $5::date, $6, $7)
		RETURNING id, created_at, updated_at`,
		t.SchoolID, t.CaseID, t.Title, t.AssigneeUserID, t.DueDate, t.NoteEnc, t.CreatedBy,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) MarkTaskDone(ctx context.Context, schoolID, id, doneBy string, doneAt time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE case_tasks
		SET done_at = $1, done_by = $2, updated_at = now()
		WHERE id = $3 AND school_id = $4 AND done_at IS NULL`,
		doneAt, doneBy, id, schoolID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

// Settings
func (s *Store) GetSettings(ctx context.Context, schoolID string) (*Settings, error) {
	var st Settings
	err := s.DB.QueryRowContext(ctx, `
		SELECT school_id, reminder_days_before, created_at, updated_at
		FROM counseling_settings
		WHERE school_id = $1`, schoolID).Scan(
		&st.SchoolID, &st.ReminderDaysBefore, &st.CreatedAt, &st.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return &Settings{SchoolID: schoolID, ReminderDaysBefore: 0}, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) PutSettings(ctx context.Context, st Settings) (*Settings, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO counseling_settings (school_id, reminder_days_before)
		VALUES ($1, $2)
		ON CONFLICT (school_id) DO UPDATE
		SET reminder_days_before = EXCLUDED.reminder_days_before, updated_at = now()
		RETURNING created_at, updated_at`,
		st.SchoolID, st.ReminderDaysBefore,
	).Scan(&st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &st, nil
}
