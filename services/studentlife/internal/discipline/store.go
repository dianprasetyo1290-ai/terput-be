package discipline

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Types
func (s *Store) ListTypes(ctx context.Context, schoolID string) ([]Type, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, code, name, kind, points, active, created_at, updated_at
		FROM discipline_types
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY kind DESC, points ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Type
	for rows.Next() {
		var t Type
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.Code, &t.Name, &t.Kind, &t.Points, &t.Active, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) CreateType(ctx context.Context, t Type) (*Type, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO discipline_types (school_id, code, name, kind, points, active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`,
		t.SchoolID, t.Code, t.Name, t.Kind, t.Points, t.Active).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) UpdateType(ctx context.Context, t Type) (*Type, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE discipline_types
		SET name = $1, points = $2, active = $3, updated_at = now()
		WHERE id = $4 AND school_id = $5 AND deleted_at IS NULL`,
		t.Name, t.Points, t.Active, t.ID, t.SchoolID)
	if err != nil {
		return nil, err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return nil, ErrNotFound
	}
	return &t, nil
}

func (s *Store) DeleteType(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE discipline_types
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

// Thresholds
func (s *Store) ListThresholds(ctx context.Context, schoolID string) ([]Threshold, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, points, action_name, notify_parent, effective_from::text, effective_to::text, created_at, updated_at
		FROM discipline_thresholds
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY points ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Threshold
	for rows.Next() {
		var th Threshold
		if err := rows.Scan(
			&th.ID, &th.SchoolID, &th.Points, &th.ActionName, &th.NotifyParent,
			&th.EffectiveFrom, &th.EffectiveTo, &th.CreatedAt, &th.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, th)
	}
	return list, rows.Err()
}

func (s *Store) CreateThreshold(ctx context.Context, th Threshold) (*Threshold, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO discipline_thresholds (school_id, points, action_name, notify_parent, effective_from, effective_to)
		VALUES ($1, $2, $3, $4, $5::date, $6::date)
		RETURNING id, created_at, updated_at`,
		th.SchoolID, th.Points, th.ActionName, th.NotifyParent, th.EffectiveFrom, th.EffectiveTo,
	).Scan(&th.ID, &th.CreatedAt, &th.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &th, nil
}

func (s *Store) UpdateThreshold(ctx context.Context, th Threshold) (*Threshold, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE discipline_thresholds
		SET points = $1, action_name = $2, notify_parent = $3, effective_from = $4::date, effective_to = $5::date, updated_at = now()
		WHERE id = $6 AND school_id = $7 AND deleted_at IS NULL`,
		th.Points, th.ActionName, th.NotifyParent, th.EffectiveFrom, th.EffectiveTo, th.ID, th.SchoolID)
	if err != nil {
		return nil, err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return nil, ErrNotFound
	}
	return &th, nil
}

func (s *Store) DeleteThreshold(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE discipline_thresholds
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

// Settings
func (s *Store) GetSettings(ctx context.Context, schoolID string) (*Settings, error) {
	var st Settings
	err := s.DB.QueryRowContext(ctx, `
		SELECT school_id, accumulation_scope, created_at, updated_at
		FROM discipline_settings
		WHERE school_id = $1`, schoolID).Scan(
		&st.SchoolID, &st.AccumulationScope, &st.CreatedAt, &st.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return &Settings{SchoolID: schoolID, AccumulationScope: ScopeAcademicYear}, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) PutSettings(ctx context.Context, st Settings) (*Settings, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO discipline_settings (school_id, accumulation_scope)
		VALUES ($1, $2)
		ON CONFLICT (school_id) DO UPDATE
		SET accumulation_scope = EXCLUDED.accumulation_scope, updated_at = now()
		RETURNING created_at, updated_at`,
		st.SchoolID, st.AccumulationScope,
	).Scan(&st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// Records
func (s *Store) ListRecords(ctx context.Context, schoolID, studentID, kind, status string) ([]Record, error) {
	query := `
		SELECT r.id, r.school_id, r.student_id, r.academic_year_id, r.type_id, dt.code, dt.name,
		       r.kind, r.points, r.occurred_on::text, r.description, r.evidence, r.reporter_user_id,
		       r.status, r.annulled_reason, r.idempotency_key, r.created_at, r.updated_at
		FROM discipline_records r
		JOIN discipline_types dt ON r.type_id = dt.id
		WHERE r.school_id = $1`
	args := []any{schoolID}
	idx := 2

	if studentID != "" {
		query += ` AND r.student_id = $` + string(rune('0'+idx))
		args = append(args, studentID)
		idx++
	}
	if kind != "" {
		query += ` AND r.kind = $` + string(rune('0'+idx))
		args = append(args, kind)
		idx++
	}
	if status != "" {
		query += ` AND r.status = $` + string(rune('0'+idx))
		args = append(args, status)
		idx++
	}

	query += ` ORDER BY r.occurred_on DESC, r.created_at DESC`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Record
	for rows.Next() {
		var r Record
		var evidenceRaw []byte
		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.StudentID, &r.AcademicYearID, &r.TypeID, &r.TypeCode, &r.TypeName,
			&r.Kind, &r.Points, &r.OccurredOn, &r.Description, &evidenceRaw, &r.ReporterUserID,
			&r.Status, &r.AnnulledReason, &r.IdempotencyKey, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(evidenceRaw) > 0 {
			_ = json.Unmarshal(evidenceRaw, &r.Evidence)
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *Store) GetRecord(ctx context.Context, schoolID, id string) (*Record, error) {
	var r Record
	var evidenceRaw []byte
	err := s.DB.QueryRowContext(ctx, `
		SELECT r.id, r.school_id, r.student_id, r.academic_year_id, r.type_id, dt.code, dt.name,
		       r.kind, r.points, r.occurred_on::text, r.description, r.evidence, r.reporter_user_id,
		       r.status, r.annulled_reason, r.idempotency_key, r.created_at, r.updated_at
		FROM discipline_records r
		JOIN discipline_types dt ON r.type_id = dt.id
		WHERE r.id = $1 AND r.school_id = $2`, id, schoolID).Scan(
		&r.ID, &r.SchoolID, &r.StudentID, &r.AcademicYearID, &r.TypeID, &r.TypeCode, &r.TypeName,
		&r.Kind, &r.Points, &r.OccurredOn, &r.Description, &evidenceRaw, &r.ReporterUserID,
		&r.Status, &r.AnnulledReason, &r.IdempotencyKey, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(evidenceRaw) > 0 {
		_ = json.Unmarshal(evidenceRaw, &r.Evidence)
	}
	return &r, nil
}

// CreateRecordWithActions saves record and triggers any threshold actions atomically (AC-131-01, BR-DIS-01).
func (s *Store) CreateRecordWithActions(ctx context.Context, rec Record, thresholds []Threshold, scope string, yearID *string) (*Record, int, int, []Action, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	defer tx.Rollback()

	// 1. Check idempotency if key provided
	if rec.IdempotencyKey != nil && *rec.IdempotencyKey != "" {
		var existingID string
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM discipline_records
			WHERE school_id = $1 AND idempotency_key = $2`,
			rec.SchoolID, *rec.IdempotencyKey).Scan(&existingID)
		if err == nil {
			// Already processed idempotently
			existing, err := s.GetRecord(ctx, rec.SchoolID, existingID)
			if err == nil {
				records, _ := s.ListRecords(ctx, rec.SchoolID, rec.StudentID, "", "")
				vp, mp := Accumulate(records, scope, yearID)
				return existing, vp, mp, nil, nil
			}
		}
	}

	// 2. Fetch existing student records to compute beforePoints
	existingRecords, err := s.ListRecords(ctx, rec.SchoolID, rec.StudentID, "", "")
	if err != nil {
		return nil, 0, 0, nil, err
	}
	beforeViolation, _ := Accumulate(existingRecords, scope, yearID)

	// 3. Insert record
	evidenceJSON, _ := json.Marshal(rec.Evidence)
	if len(evidenceJSON) == 0 {
		evidenceJSON = []byte("[]")
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO discipline_records
			(school_id, student_id, academic_year_id, type_id, kind, points, occurred_on, description, evidence, reporter_user_id, status, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7::date, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`,
		rec.SchoolID, rec.StudentID, rec.AcademicYearID, rec.TypeID, rec.Kind, rec.Points,
		rec.OccurredOn, rec.Description, evidenceJSON, rec.ReporterUserID, RecordStatusActive, rec.IdempotencyKey,
	).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	rec.Status = RecordStatusActive

	// 4. Compute afterPoints and newly crossed thresholds
	allRecords := append(existingRecords, rec)
	afterViolation, currentMerit := Accumulate(allRecords, scope, yearID)

	var triggeredActions []Action
	if rec.Kind == TypeKindViolation {
		// Fetch existing actions for student in period to prevent duplicate trigger (BR-DIS-01)
		triggeredIDs := make(map[string]bool)
		rows, err := tx.QueryContext(ctx, `
			SELECT threshold_id FROM discipline_actions
			WHERE student_id = $1 AND status <> 'cancelled' AND
			      COALESCE(academic_year_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE($2, '00000000-0000-0000-0000-000000000000'::uuid)`,
			rec.StudentID, rec.AcademicYearID)
		if err == nil {
			for rows.Next() {
				var thID string
				if err := rows.Scan(&thID); err == nil {
					triggeredIDs[thID] = true
				}
			}
			rows.Close()
		}

		crossed := CrossedThresholds(beforeViolation, afterViolation, thresholds, triggeredIDs, rec.OccurredOn)
		for _, th := range crossed {
			var act Action
			err := tx.QueryRowContext(ctx, `
				INSERT INTO discipline_actions
					(school_id, student_id, academic_year_id, threshold_id, triggered_by_record_id, points_at_trigger, status)
				VALUES ($1, $2, $3, $4, $5, $6, 'open')
				ON CONFLICT (student_id, threshold_id, COALESCE(academic_year_id, '00000000-0000-0000-0000-000000000000'::uuid))
				DO NOTHING
				RETURNING id, created_at, updated_at`,
				rec.SchoolID, rec.StudentID, rec.AcademicYearID, th.ID, rec.ID, afterViolation,
			).Scan(&act.ID, &act.CreatedAt, &act.UpdatedAt)
			if err == nil {
				act.SchoolID = rec.SchoolID
				act.StudentID = rec.StudentID
				act.AcademicYearID = rec.AcademicYearID
				act.ThresholdID = th.ID
				act.ActionName = th.ActionName
				act.TriggeredByRecordID = rec.ID
				act.PointsAtTrigger = afterViolation
				act.Status = ActionStatusOpen
				triggeredActions = append(triggeredActions, act)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, 0, 0, nil, err
	}

	return &rec, afterViolation, currentMerit, triggeredActions, nil
}

func (s *Store) AnnulRecord(ctx context.Context, schoolID, recordID, reason string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE discipline_records
		SET status = $1, annulled_reason = $2, updated_at = now()
		WHERE id = $3 AND school_id = $4`,
		RecordStatusAnnulled, reason, recordID, schoolID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RestoreRecord(ctx context.Context, schoolID, recordID string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE discipline_records
		SET status = $1, updated_at = now()
		WHERE id = $2 AND school_id = $3 AND status = $4`,
		RecordStatusActive, recordID, schoolID, RecordStatusAppealed)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

// Actions
func (s *Store) ListActions(ctx context.Context, schoolID, status, studentID string) ([]Action, error) {
	query := `
		SELECT a.id, a.school_id, a.student_id, a.academic_year_id, a.threshold_id, dt.action_name,
		       a.triggered_by_record_id, a.points_at_trigger, a.status, a.assignee_user_id,
		       a.notes, a.done_at, a.created_at, a.updated_at,
		       (r.status = 'annulled') as review_needed
		FROM discipline_actions a
		JOIN discipline_thresholds dt ON a.threshold_id = dt.id
		JOIN discipline_records r ON a.triggered_by_record_id = r.id
		WHERE a.school_id = $1`
	args := []any{schoolID}
	idx := 2

	if status != "" {
		query += ` AND a.status = $` + string(rune('0'+idx))
		args = append(args, status)
		idx++
	}
	if studentID != "" {
		query += ` AND a.student_id = $` + string(rune('0'+idx))
		args = append(args, studentID)
		idx++
	}

	query += ` ORDER BY a.created_at DESC`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Action
	for rows.Next() {
		var a Action
		if err := rows.Scan(
			&a.ID, &a.SchoolID, &a.StudentID, &a.AcademicYearID, &a.ThresholdID, &a.ActionName,
			&a.TriggeredByRecordID, &a.PointsAtTrigger, &a.Status, &a.AssigneeUserID,
			&a.Notes, &a.DoneAt, &a.CreatedAt, &a.UpdatedAt, &a.ReviewNeeded,
		); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Store) GetAction(ctx context.Context, schoolID, id string) (*Action, error) {
	var a Action
	err := s.DB.QueryRowContext(ctx, `
		SELECT a.id, a.school_id, a.student_id, a.academic_year_id, a.threshold_id, dt.action_name,
		       a.triggered_by_record_id, a.points_at_trigger, a.status, a.assignee_user_id,
		       a.notes, a.done_at, a.created_at, a.updated_at,
		       (r.status = 'annulled') as review_needed
		FROM discipline_actions a
		JOIN discipline_thresholds dt ON a.threshold_id = dt.id
		JOIN discipline_records r ON a.triggered_by_record_id = r.id
		WHERE a.id = $1 AND a.school_id = $2`, id, schoolID).Scan(
		&a.ID, &a.SchoolID, &a.StudentID, &a.AcademicYearID, &a.ThresholdID, &a.ActionName,
		&a.TriggeredByRecordID, &a.PointsAtTrigger, &a.Status, &a.AssigneeUserID,
		&a.Notes, &a.DoneAt, &a.CreatedAt, &a.UpdatedAt, &a.ReviewNeeded,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) UpdateAction(ctx context.Context, schoolID, id, status string, assigneeUserID, notes *string, doneAt *time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE discipline_actions
		SET status = $1, assignee_user_id = COALESCE($2, assignee_user_id),
		    notes = COALESCE($3, notes), done_at = COALESCE($4, done_at), updated_at = now()
		WHERE id = $5 AND school_id = $6`,
		status, assigneeUserID, notes, doneAt, id, schoolID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

// Appeals
func (s *Store) CreateAppeal(ctx context.Context, ap Appeal) (*Appeal, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Update record status to appealed
	_, err = tx.ExecContext(ctx, `
		UPDATE discipline_records
		SET status = $1, updated_at = now()
		WHERE id = $2 AND status = $3`,
		RecordStatusAppealed, ap.RecordID, RecordStatusActive)
	if err != nil {
		return nil, err
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO discipline_appeals
			(school_id, record_id, guardian_user_id, reason, status, approval_id, idempotency_key)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6)
		RETURNING id, created_at, updated_at`,
		ap.SchoolID, ap.RecordID, ap.GuardianUserID, ap.Reason, ap.ApprovalID, ap.IdempotencyKey,
	).Scan(&ap.ID, &ap.CreatedAt, &ap.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ap.Status = AppealStatusPending
	return &ap, nil
}

func (s *Store) ListAppeals(ctx context.Context, schoolID, guardianUserID, status string) ([]Appeal, error) {
	query := `
		SELECT id, school_id, record_id, guardian_user_id, reason, status, approval_id, idempotency_key, created_at, updated_at
		FROM discipline_appeals
		WHERE school_id = $1`
	args := []any{schoolID}
	idx := 2

	if guardianUserID != "" {
		query += ` AND guardian_user_id = $` + string(rune('0'+idx))
		args = append(args, guardianUserID)
		idx++
	}
	if status != "" {
		query += ` AND status = $` + string(rune('0'+idx))
		args = append(args, status)
		idx++
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Appeal
	for rows.Next() {
		var ap Appeal
		if err := rows.Scan(
			&ap.ID, &ap.SchoolID, &ap.RecordID, &ap.GuardianUserID, &ap.Reason,
			&ap.Status, &ap.ApprovalID, &ap.IdempotencyKey, &ap.CreatedAt, &ap.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, ap)
	}
	return list, rows.Err()
}

func (s *Store) GetAppealByRecordID(ctx context.Context, recordID string) (*Appeal, error) {
	var ap Appeal
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, record_id, guardian_user_id, reason, status, approval_id, idempotency_key, created_at, updated_at
		FROM discipline_appeals
		WHERE record_id = $1 ORDER BY created_at DESC LIMIT 1`, recordID).Scan(
		&ap.ID, &ap.SchoolID, &ap.RecordID, &ap.GuardianUserID, &ap.Reason,
		&ap.Status, &ap.ApprovalID, &ap.IdempotencyKey, &ap.CreatedAt, &ap.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ap, nil
}

func (s *Store) UpdateAppealStatus(ctx context.Context, appealID, status string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE discipline_appeals
		SET status = $1, updated_at = now()
		WHERE id = $2`, status, appealID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) PendingAppealCount(ctx context.Context, recordID string) (int, error) {
	var count int
	err := s.DB.QueryRowContext(ctx, `
		SELECT count(*) FROM discipline_appeals
		WHERE record_id = $1 AND status = 'pending'`, recordID).Scan(&count)
	return count, err
}

// Student Summary
func (s *Store) GetStudentSummary(ctx context.Context, schoolID, studentID string, scope string, yearID *string) (*StudentSummary, error) {
	records, err := s.ListRecords(ctx, schoolID, studentID, "", "")
	if err != nil {
		return nil, err
	}
	vp, mp := Accumulate(records, scope, yearID)

	actions, err := s.ListActions(ctx, schoolID, "", studentID)
	if err != nil {
		return nil, err
	}

	return &StudentSummary{
		StudentID:       studentID,
		ViolationPoints: vp,
		MeritPoints:     mp,
		Actions:         actions,
	}, nil
}
