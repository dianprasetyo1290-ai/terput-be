package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"skci/studentlife/internal/store"
)

type Store struct {
	DB *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{DB: db}
}

// -------------------------------------------------------------
// 1. Event Kinds
// -------------------------------------------------------------

func (s *Store) ListEventKinds(ctx context.Context, schoolID string) ([]EventKind, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, name, deleted_at, created_at, updated_at
		FROM act_event_kinds
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EventKind
	for rows.Next() {
		var k EventKind
		if err := rows.Scan(&k.ID, &k.SchoolID, &k.Name, &k.DeletedAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, k)
	}
	return list, nil
}

func (s *Store) CreateEventKind(ctx context.Context, k EventKind) (*EventKind, error) {
	if k.ID == "" {
		k.ID = store.NewID()
	}
	now := time.Now()
	k.CreatedAt = now
	k.UpdatedAt = now

	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO act_event_kinds (id, school_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`,
		k.ID, k.SchoolID, k.Name, k.CreatedAt, k.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (s *Store) UpdateEventKind(ctx context.Context, schoolID, id, name string) (*EventKind, error) {
	now := time.Now()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE act_event_kinds
		SET name = $1, updated_at = $2
		WHERE id = $3 AND school_id = $4 AND deleted_at IS NULL`,
		name, now, id, schoolID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, store.ErrNotFound
	}
	return &EventKind{ID: id, SchoolID: schoolID, Name: name, UpdatedAt: now}, nil
}

func (s *Store) DeleteEventKind(ctx context.Context, schoolID, id string) error {
	now := time.Now()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE act_event_kinds
		SET deleted_at = $1
		WHERE id = $2 AND school_id = $3 AND deleted_at IS NULL`,
		now, id, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// -------------------------------------------------------------
// 2. Events
// -------------------------------------------------------------

func (s *Store) ListEvents(ctx context.Context, schoolID, status, from, to string) ([]Event, error) {
	q := `
		SELECT e.id, e.school_id, e.name, e.kind_id, k.name as kind_name, e.description,
		       e.starts_at, e.ends_at, e.room_id, e.booking_id, e.location_text, e.target,
		       e.quota, e.fee_idr, e.requires_parent_consent, e.consent_text,
		       e.budget_line_id, e.budget_amount_idr, e.registration_opens_at, e.registration_closes_at,
		       e.status, e.approval_id, e.published_at, e.created_by, e.deleted_at,
		       e.idempotency_key, e.created_at, e.updated_at,
		       COALESCE((SELECT COUNT(*) FROM act_event_registrations r WHERE r.event_id = e.id AND r.status <> 'cancelled'), 0) as registered_count
		FROM act_events e
		JOIN act_event_kinds k ON e.kind_id = k.id
		WHERE e.school_id = $1 AND e.deleted_at IS NULL`

	args := []any{schoolID}
	idx := 2

	if status != "" {
		q += fmt.Sprintf(" AND e.status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if from != "" {
		q += fmt.Sprintf(" AND e.starts_at >= $%d", idx)
		args = append(args, from)
		idx++
	}
	if to != "" {
		q += fmt.Sprintf(" AND e.ends_at <= $%d", idx)
		args = append(args, to)
		idx++
	}

	q += " ORDER BY e.starts_at DESC"

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Event
	for rows.Next() {
		var e Event
		var targetRaw []byte
		if err := rows.Scan(
			&e.ID, &e.SchoolID, &e.Name, &e.KindID, &e.KindName, &e.Description,
			&e.StartsAt, &e.EndsAt, &e.RoomID, &e.BookingID, &e.LocationText, &targetRaw,
			&e.Quota, &e.FeeIDR, &e.RequiresParentConsent, &e.ConsentText,
			&e.BudgetLineID, &e.BudgetAmountIDR, &e.RegistrationOpensAt, &e.RegistrationClosesAt,
			&e.Status, &e.ApprovalID, &e.PublishedAt, &e.CreatedBy, &e.DeletedAt,
			&e.IdempotencyKey, &e.CreatedAt, &e.UpdatedAt, &e.RegisteredCount,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(targetRaw, &e.Target)
		list = append(list, e)
	}
	return list, nil
}

func (s *Store) GetEvent(ctx context.Context, schoolID, id string) (*Event, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT e.id, e.school_id, e.name, e.kind_id, k.name as kind_name, e.description,
		       e.starts_at, e.ends_at, e.room_id, e.booking_id, e.location_text, e.target,
		       e.quota, e.fee_idr, e.requires_parent_consent, e.consent_text,
		       e.budget_line_id, e.budget_amount_idr, e.registration_opens_at, e.registration_closes_at,
		       e.status, e.approval_id, e.published_at, e.created_by, e.deleted_at,
		       e.idempotency_key, e.created_at, e.updated_at,
		       COALESCE((SELECT COUNT(*) FROM act_event_registrations r WHERE r.event_id = e.id AND r.status <> 'cancelled'), 0) as registered_count
		FROM act_events e
		JOIN act_event_kinds k ON e.kind_id = k.id
		WHERE e.id = $1 AND e.school_id = $2 AND e.deleted_at IS NULL`, id, schoolID)

	var e Event
	var targetRaw []byte
	if err := row.Scan(
		&e.ID, &e.SchoolID, &e.Name, &e.KindID, &e.KindName, &e.Description,
		&e.StartsAt, &e.EndsAt, &e.RoomID, &e.BookingID, &e.LocationText, &targetRaw,
		&e.Quota, &e.FeeIDR, &e.RequiresParentConsent, &e.ConsentText,
		&e.BudgetLineID, &e.BudgetAmountIDR, &e.RegistrationOpensAt, &e.RegistrationClosesAt,
		&e.Status, &e.ApprovalID, &e.PublishedAt, &e.CreatedBy, &e.DeletedAt,
		&e.IdempotencyKey, &e.CreatedAt, &e.UpdatedAt, &e.RegisteredCount,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(targetRaw, &e.Target)
	return &e, nil
}

func (s *Store) CreateEvent(ctx context.Context, e Event, committee []string) (*Event, error) {
	if e.ID == "" {
		e.ID = store.NewID()
	}
	now := time.Now()
	e.CreatedAt = now
	e.UpdatedAt = now

	targetJSON, _ := json.Marshal(e.Target)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO act_events (
			id, school_id, name, kind_id, description, starts_at, ends_at,
			room_id, booking_id, location_text, target, quota, fee_idr,
			requires_parent_consent, consent_text, budget_line_id, budget_amount_idr,
			registration_opens_at, registration_closes_at, status, approval_id,
			published_at, created_by, idempotency_key, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17,
			$18, $19, $20, $21,
			$22, $23, $24, $25, $26
		)`,
		e.ID, e.SchoolID, e.Name, e.KindID, e.Description, e.StartsAt, e.EndsAt,
		e.RoomID, e.BookingID, e.LocationText, targetJSON, e.Quota, e.FeeIDR,
		e.RequiresParentConsent, e.ConsentText, e.BudgetLineID, e.BudgetAmountIDR,
		e.RegistrationOpensAt, e.RegistrationClosesAt, e.Status, e.ApprovalID,
		e.PublishedAt, e.CreatedBy, e.IdempotencyKey, e.CreatedAt, e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	for _, uid := range committee {
		if uid != "" {
			_, _ = tx.ExecContext(ctx, `
				INSERT INTO act_event_committee (event_id, user_id, school_id, role_label, created_at)
				VALUES ($1, $2, $3, 'panitia', $4)
				ON CONFLICT DO NOTHING`,
				e.ID, uid, e.SchoolID, now)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) UpdateEvent(ctx context.Context, e Event) error {
	now := time.Now()
	targetJSON, _ := json.Marshal(e.Target)

	res, err := s.DB.ExecContext(ctx, `
		UPDATE act_events
		SET name = $1, kind_id = $2, description = $3, starts_at = $4, ends_at = $5,
		    room_id = $6, booking_id = $7, location_text = $8, target = $9, quota = $10,
		    fee_idr = $11, requires_parent_consent = $12, consent_text = $13,
		    budget_line_id = $14, budget_amount_idr = $15, registration_opens_at = $16,
		    registration_closes_at = $17, updated_at = $18
		WHERE id = $19 AND school_id = $20 AND deleted_at IS NULL AND status = 'draft'`,
		e.Name, e.KindID, e.Description, e.StartsAt, e.EndsAt,
		e.RoomID, e.BookingID, e.LocationText, targetJSON, e.Quota,
		e.FeeIDR, e.RequiresParentConsent, e.ConsentText,
		e.BudgetLineID, e.BudgetAmountIDR, e.RegistrationOpensAt,
		e.RegistrationClosesAt, now, e.ID, e.SchoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteEvent(ctx context.Context, schoolID, id string) error {
	now := time.Now()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE act_events
		SET deleted_at = $1
		WHERE id = $2 AND school_id = $3 AND deleted_at IS NULL AND status = 'draft'`,
		now, id, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateEventStatus(ctx context.Context, schoolID, id, status string, approvalID *string, publishedAt *time.Time) error {
	now := time.Now()
	_, err := s.DB.ExecContext(ctx, `
		UPDATE act_events
		SET status = $1, approval_id = COALESCE($2, approval_id),
		    published_at = COALESCE($3, published_at), updated_at = $4
		WHERE id = $5 AND school_id = $6 AND deleted_at IS NULL`,
		status, approvalID, publishedAt, now, id, schoolID)
	return err
}

func (s *Store) ListPublishedEvents(ctx context.Context, schoolID, from, to string) ([]Event, error) {
	q := `
		SELECT e.id, e.school_id, e.name, e.kind_id, k.name as kind_name, e.description,
		       e.starts_at, e.ends_at, e.room_id, e.booking_id, e.location_text, e.target,
		       e.quota, e.fee_idr, e.requires_parent_consent, e.consent_text,
		       e.budget_line_id, e.budget_amount_idr, e.registration_opens_at, e.registration_closes_at,
		       e.status, e.approval_id, e.published_at, e.created_by, e.deleted_at,
		       e.idempotency_key, e.created_at, e.updated_at,
		       COALESCE((SELECT COUNT(*) FROM act_event_registrations r WHERE r.event_id = e.id AND r.status <> 'cancelled'), 0) as registered_count
		FROM act_events e
		JOIN act_event_kinds k ON e.kind_id = k.id
		WHERE e.school_id = $1 AND e.deleted_at IS NULL AND e.status IN ('published','completed')`

	args := []any{schoolID}
	idx := 2

	if from != "" {
		q += fmt.Sprintf(" AND e.starts_at >= $%d", idx)
		args = append(args, from)
		idx++
	}
	if to != "" {
		q += fmt.Sprintf(" AND e.ends_at <= $%d", idx)
		args = append(args, to)
		idx++
	}
	q += " ORDER BY e.starts_at ASC"

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Event
	for rows.Next() {
		var e Event
		var targetRaw []byte
		if err := rows.Scan(
			&e.ID, &e.SchoolID, &e.Name, &e.KindID, &e.KindName, &e.Description,
			&e.StartsAt, &e.EndsAt, &e.RoomID, &e.BookingID, &e.LocationText, &targetRaw,
			&e.Quota, &e.FeeIDR, &e.RequiresParentConsent, &e.ConsentText,
			&e.BudgetLineID, &e.BudgetAmountIDR, &e.RegistrationOpensAt, &e.RegistrationClosesAt,
			&e.Status, &e.ApprovalID, &e.PublishedAt, &e.CreatedBy, &e.DeletedAt,
			&e.IdempotencyKey, &e.CreatedAt, &e.UpdatedAt, &e.RegisteredCount,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(targetRaw, &e.Target)
		list = append(list, e)
	}
	return list, nil
}

// -------------------------------------------------------------
// 3. Event Registrations
// -------------------------------------------------------------

func (s *Store) ListEventRegistrations(ctx context.Context, schoolID, eventID string) ([]EventRegistration, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.id, r.school_id, r.event_id, r.student_user_id, r.registered_by,
		       r.registered_via, r.status, r.consent_id, r.consent_text_snapshot,
		       r.bill_id, r.attended_at, r.attendance_by, r.created_at, r.updated_at,
		       e.name as event_name
		FROM act_event_registrations r
		JOIN act_events e ON r.event_id = e.id
		WHERE r.school_id = $1 AND r.event_id = $2
		ORDER BY r.created_at ASC`, schoolID, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EventRegistration
	for rows.Next() {
		var r EventRegistration
		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.EventID, &r.StudentUserID, &r.RegisteredBy,
			&r.RegisteredVia, &r.Status, &r.ConsentID, &r.ConsentTextSnapshot,
			&r.BillID, &r.AttendedAt, &r.AttendanceBy, &r.CreatedAt, &r.UpdatedAt,
			&r.EventName,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

func (s *Store) GetRegistration(ctx context.Context, schoolID, id string) (*EventRegistration, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT r.id, r.school_id, r.event_id, r.student_user_id, r.registered_by,
		       r.registered_via, r.status, r.consent_id, r.consent_text_snapshot,
		       r.bill_id, r.attended_at, r.attendance_by, r.created_at, r.updated_at,
		       e.name as event_name
		FROM act_event_registrations r
		JOIN act_events e ON r.event_id = e.id
		WHERE r.id = $1 AND r.school_id = $2`, id, schoolID)

	var r EventRegistration
	if err := row.Scan(
		&r.ID, &r.SchoolID, &r.EventID, &r.StudentUserID, &r.RegisteredBy,
		&r.RegisteredVia, &r.Status, &r.ConsentID, &r.ConsentTextSnapshot,
		&r.BillID, &r.AttendedAt, &r.AttendanceBy, &r.CreatedAt, &r.UpdatedAt,
		&r.EventName,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetRegistrationByStudent(ctx context.Context, schoolID, eventID, studentUserID string) (*EventRegistration, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT r.id, r.school_id, r.event_id, r.student_user_id, r.registered_by,
		       r.registered_via, r.status, r.consent_id, r.consent_text_snapshot,
		       r.bill_id, r.attended_at, r.attendance_by, r.created_at, r.updated_at,
		       e.name as event_name
		FROM act_event_registrations r
		JOIN act_events e ON r.event_id = e.id
		WHERE r.event_id = $1 AND r.student_user_id = $2 AND r.school_id = $3 AND r.status <> 'cancelled'`,
		eventID, studentUserID, schoolID)

	var r EventRegistration
	if err := row.Scan(
		&r.ID, &r.SchoolID, &r.EventID, &r.StudentUserID, &r.RegisteredBy,
		&r.RegisteredVia, &r.Status, &r.ConsentID, &r.ConsentTextSnapshot,
		&r.BillID, &r.AttendedAt, &r.AttendanceBy, &r.CreatedAt, &r.UpdatedAt,
		&r.EventName,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) CountActiveRegistrations(ctx context.Context, schoolID, eventID string) (int, error) {
	var count int
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM act_event_registrations
		WHERE school_id = $1 AND event_id = $2 AND status IN ('confirmed','pending_consent')`,
		schoolID, eventID).Scan(&count)
	return count, err
}

func (s *Store) CreateRegistration(ctx context.Context, r EventRegistration) (*EventRegistration, error) {
	if r.ID == "" {
		r.ID = store.NewID()
	}
	now := time.Now()
	r.CreatedAt = now
	r.UpdatedAt = now

	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO act_event_registrations (
			id, school_id, event_id, student_user_id, registered_by, registered_via,
			status, consent_id, consent_text_snapshot, bill_id, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12
		)`,
		r.ID, r.SchoolID, r.EventID, r.StudentUserID, r.RegisteredBy, r.RegisteredVia,
		r.Status, r.ConsentID, r.ConsentTextSnapshot, r.BillID, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) UpdateRegistrationStatus(ctx context.Context, schoolID, id, status string, consentID, consentSnapshot, billID *string) error {
	now := time.Now()
	_, err := s.DB.ExecContext(ctx, `
		UPDATE act_event_registrations
		SET status = $1, consent_id = COALESCE($2, consent_id),
		    consent_text_snapshot = COALESCE($3, consent_text_snapshot),
		    bill_id = COALESCE($4, bill_id), updated_at = $5
		WHERE id = $6 AND school_id = $7`,
		status, consentID, consentSnapshot, billID, now, id, schoolID)
	return err
}

func (s *Store) MarkAttendance(ctx context.Context, schoolID, id, attendedBy string, attendedAt time.Time) error {
	now := time.Now()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE act_event_registrations
		SET attended_at = $1, attendance_by = $2, updated_at = $3
		WHERE id = $4 AND school_id = $5 AND status = 'confirmed'`,
		attendedAt, attendedBy, now, id, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) ListMyRegistrations(ctx context.Context, schoolID, studentUserID string) ([]EventRegistration, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.id, r.school_id, r.event_id, r.student_user_id, r.registered_by,
		       r.registered_via, r.status, r.consent_id, r.consent_text_snapshot,
		       r.bill_id, r.attended_at, r.attendance_by, r.created_at, r.updated_at,
		       e.name as event_name
		FROM act_event_registrations r
		JOIN act_events e ON r.event_id = e.id
		WHERE r.school_id = $1 AND r.student_user_id = $2
		ORDER BY r.created_at DESC`, schoolID, studentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EventRegistration
	for rows.Next() {
		var r EventRegistration
		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.EventID, &r.StudentUserID, &r.RegisteredBy,
			&r.RegisteredVia, &r.Status, &r.ConsentID, &r.ConsentTextSnapshot,
			&r.BillID, &r.AttendedAt, &r.AttendanceBy, &r.CreatedAt, &r.UpdatedAt,
			&r.EventName,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

// -------------------------------------------------------------
// 4. Extracurriculars & Periods
// -------------------------------------------------------------

func (s *Store) ListPeriods(ctx context.Context, schoolID string) ([]EccPeriod, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, name, opens_at, closes_at, created_at, updated_at
		FROM act_ecc_periods
		WHERE school_id = $1
		ORDER BY opens_at DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EccPeriod
	for rows.Next() {
		var p EccPeriod
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.Name, &p.OpensAt, &p.ClosesAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

func (s *Store) GetPeriod(ctx context.Context, schoolID, id string) (*EccPeriod, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, name, opens_at, closes_at, created_at, updated_at
		FROM act_ecc_periods
		WHERE id = $1 AND school_id = $2`, id, schoolID)

	var p EccPeriod
	if err := row.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.Name, &p.OpensAt, &p.ClosesAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreatePeriod(ctx context.Context, p EccPeriod) (*EccPeriod, error) {
	if p.ID == "" {
		p.ID = store.NewID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO act_ecc_periods (id, school_id, academic_year_id, name, opens_at, closes_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.ID, p.SchoolID, p.AcademicYearID, p.Name, p.OpensAt, p.ClosesAt, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) UpdatePeriod(ctx context.Context, p EccPeriod) error {
	now := time.Now()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE act_ecc_periods
		SET name = $1, academic_year_id = $2, opens_at = $3, closes_at = $4, updated_at = $5
		WHERE id = $6 AND school_id = $7`,
		p.Name, p.AcademicYearID, p.OpensAt, p.ClosesAt, now, p.ID, p.SchoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeletePeriod(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		DELETE FROM act_ecc_periods
		WHERE id = $1 AND school_id = $2`, id, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) ListExtracurriculars(ctx context.Context, schoolID string, activeOnly bool) ([]Extracurricular, error) {
	q := `
		SELECT e.id, e.school_id, e.name, e.description, e.schedule, e.room_id,
		       e.location_text, e.quota, e.fee_idr, e.full_policy, e.active,
		       e.deleted_at, e.created_at, e.updated_at,
		       COALESCE((SELECT COUNT(*) FROM act_ecc_members m WHERE m.ecc_id = e.id AND m.status = 'active'), 0) as active_count,
		       COALESCE((SELECT COUNT(*) FROM act_ecc_members m WHERE m.ecc_id = e.id AND m.status = 'waitlisted'), 0) as waitlisted_count
		FROM act_extracurriculars e
		WHERE e.school_id = $1 AND e.deleted_at IS NULL`

	if activeOnly {
		q += " AND e.active = true"
	}
	q += " ORDER BY e.name ASC"

	rows, err := s.DB.QueryContext(ctx, q, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Extracurricular
	for rows.Next() {
		var ecc Extracurricular
		if err := rows.Scan(
			&ecc.ID, &ecc.SchoolID, &ecc.Name, &ecc.Description, &ecc.Schedule,
			&ecc.RoomID, &ecc.LocationText, &ecc.Quota, &ecc.FeeIDR, &ecc.FullPolicy,
			&ecc.Active, &ecc.DeletedAt, &ecc.CreatedAt, &ecc.UpdatedAt,
			&ecc.ActiveMemberCount, &ecc.WaitlistedMemberCount,
		); err != nil {
			return nil, err
		}
		list = append(list, ecc)
	}
	return list, nil
}

func (s *Store) GetExtracurricular(ctx context.Context, schoolID, id string) (*Extracurricular, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT e.id, e.school_id, e.name, e.description, e.schedule, e.room_id,
		       e.location_text, e.quota, e.fee_idr, e.full_policy, e.active,
		       e.deleted_at, e.created_at, e.updated_at,
		       COALESCE((SELECT COUNT(*) FROM act_ecc_members m WHERE m.ecc_id = e.id AND m.status = 'active'), 0) as active_count,
		       COALESCE((SELECT COUNT(*) FROM act_ecc_members m WHERE m.ecc_id = e.id AND m.status = 'waitlisted'), 0) as waitlisted_count
		FROM act_extracurriculars e
		WHERE e.id = $1 AND e.school_id = $2 AND e.deleted_at IS NULL`, id, schoolID)

	var ecc Extracurricular
	if err := row.Scan(
		&ecc.ID, &ecc.SchoolID, &ecc.Name, &ecc.Description, &ecc.Schedule,
		&ecc.RoomID, &ecc.LocationText, &ecc.Quota, &ecc.FeeIDR, &ecc.FullPolicy,
		&ecc.Active, &ecc.DeletedAt, &ecc.CreatedAt, &ecc.UpdatedAt,
		&ecc.ActiveMemberCount, &ecc.WaitlistedMemberCount,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &ecc, nil
}

func (s *Store) CreateExtracurricular(ctx context.Context, ecc Extracurricular, coaches []string) (*Extracurricular, error) {
	if ecc.ID == "" {
		ecc.ID = store.NewID()
	}
	now := time.Now()
	ecc.CreatedAt = now
	ecc.UpdatedAt = now

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO act_extracurriculars (
			id, school_id, name, description, schedule, room_id, location_text,
			quota, fee_idr, full_policy, active, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13
		)`,
		ecc.ID, ecc.SchoolID, ecc.Name, ecc.Description, ecc.Schedule, ecc.RoomID, ecc.LocationText,
		ecc.Quota, ecc.FeeIDR, ecc.FullPolicy, ecc.Active, ecc.CreatedAt, ecc.UpdatedAt)
	if err != nil {
		return nil, err
	}

	for _, cid := range coaches {
		if cid != "" {
			_, _ = tx.ExecContext(ctx, `
				INSERT INTO act_ecc_coaches (ecc_id, user_id, school_id, created_at)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT DO NOTHING`,
				ecc.ID, cid, ecc.SchoolID, now)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ecc, nil
}

func (s *Store) UpdateExtracurricular(ctx context.Context, ecc Extracurricular, coaches []string) error {
	now := time.Now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE act_extracurriculars
		SET name = $1, description = $2, schedule = $3, room_id = $4,
		    location_text = $5, quota = $6, fee_idr = $7, full_policy = $8,
		    active = $9, updated_at = $10
		WHERE id = $11 AND school_id = $12 AND deleted_at IS NULL`,
		ecc.Name, ecc.Description, ecc.Schedule, ecc.RoomID,
		ecc.LocationText, ecc.Quota, ecc.FeeIDR, ecc.FullPolicy,
		ecc.Active, now, ecc.ID, ecc.SchoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}

	if coaches != nil {
		_, _ = tx.ExecContext(ctx, `DELETE FROM act_ecc_coaches WHERE ecc_id = $1`, ecc.ID)
		for _, cid := range coaches {
			if cid != "" {
				_, _ = tx.ExecContext(ctx, `
					INSERT INTO act_ecc_coaches (ecc_id, user_id, school_id, created_at)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT DO NOTHING`,
					ecc.ID, cid, ecc.SchoolID, now)
			}
		}
	}

	return tx.Commit()
}

func (s *Store) DeleteExtracurricular(ctx context.Context, schoolID, id string) error {
	now := time.Now()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE act_extracurriculars
		SET deleted_at = $1
		WHERE id = $2 AND school_id = $3 AND deleted_at IS NULL`,
		now, id, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) GetCoaches(ctx context.Context, eccID string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT user_id
		FROM act_ecc_coaches
		WHERE ecc_id = $1`, eccID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err == nil {
			list = append(list, uid)
		}
	}
	return list, nil
}

// -------------------------------------------------------------
// 5. Members & Sessions & Attendance & Assessments
// -------------------------------------------------------------

func (s *Store) ListMembers(ctx context.Context, schoolID, eccID, periodID string) ([]EccMember, error) {
	q := `
		SELECT m.id, m.school_id, m.ecc_id, m.period_id, m.student_user_id,
		       m.status, m.waitlisted_at, m.joined_at, m.left_at, m.bill_id,
		       m.created_at, m.updated_at, e.name as ecc_name
		FROM act_ecc_members m
		JOIN act_extracurriculars e ON m.ecc_id = e.id
		WHERE m.school_id = $1 AND m.ecc_id = $2`

	args := []any{schoolID, eccID}
	if periodID != "" {
		q += " AND m.period_id = $3"
		args = append(args, periodID)
	}
	q += " ORDER BY m.created_at ASC"

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EccMember
	for rows.Next() {
		var m EccMember
		if err := rows.Scan(
			&m.ID, &m.SchoolID, &m.EccID, &m.PeriodID, &m.StudentUserID,
			&m.Status, &m.WaitlistedAt, &m.JoinedAt, &m.LeftAt, &m.BillID,
			&m.CreatedAt, &m.UpdatedAt, &m.EccName,
		); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

func (s *Store) GetMember(ctx context.Context, schoolID, id string) (*EccMember, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT m.id, m.school_id, m.ecc_id, m.period_id, m.student_user_id,
		       m.status, m.waitlisted_at, m.joined_at, m.left_at, m.bill_id,
		       m.created_at, m.updated_at, e.name as ecc_name
		FROM act_ecc_members m
		JOIN act_extracurriculars e ON m.ecc_id = e.id
		WHERE m.id = $1 AND m.school_id = $2`, id, schoolID)

	var m EccMember
	if err := row.Scan(
		&m.ID, &m.SchoolID, &m.EccID, &m.PeriodID, &m.StudentUserID,
		&m.Status, &m.WaitlistedAt, &m.JoinedAt, &m.LeftAt, &m.BillID,
		&m.CreatedAt, &m.UpdatedAt, &m.EccName,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (s *Store) GetMemberByStudent(ctx context.Context, schoolID, eccID, periodID, studentUserID string) (*EccMember, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT m.id, m.school_id, m.ecc_id, m.period_id, m.student_user_id,
		       m.status, m.waitlisted_at, m.joined_at, m.left_at, m.bill_id,
		       m.created_at, m.updated_at, e.name as ecc_name
		FROM act_ecc_members m
		JOIN act_extracurriculars e ON m.ecc_id = e.id
		WHERE m.ecc_id = $1 AND m.period_id = $2 AND m.student_user_id = $3 AND m.school_id = $4`,
		eccID, periodID, studentUserID, schoolID)

	var m EccMember
	if err := row.Scan(
		&m.ID, &m.SchoolID, &m.EccID, &m.PeriodID, &m.StudentUserID,
		&m.Status, &m.WaitlistedAt, &m.JoinedAt, &m.LeftAt, &m.BillID,
		&m.CreatedAt, &m.UpdatedAt, &m.EccName,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (s *Store) CountActiveMembers(ctx context.Context, schoolID, eccID, periodID string) (int, error) {
	var count int
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM act_ecc_members
		WHERE school_id = $1 AND ecc_id = $2 AND period_id = $3 AND status = 'active'`,
		schoolID, eccID, periodID).Scan(&count)
	return count, err
}

func (s *Store) CreateMember(ctx context.Context, m EccMember) (*EccMember, error) {
	if m.ID == "" {
		m.ID = store.NewID()
	}
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now

	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO act_ecc_members (
			id, school_id, ecc_id, period_id, student_user_id, status,
			waitlisted_at, joined_at, left_at, bill_id, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12
		)`,
		m.ID, m.SchoolID, m.EccID, m.PeriodID, m.StudentUserID, m.Status,
		m.WaitlistedAt, m.JoinedAt, m.LeftAt, m.BillID, m.CreatedAt, m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) UpdateMemberStatus(ctx context.Context, schoolID, id, status string, billID *string) error {
	now := time.Now()
	var joinedAt, leftAt *time.Time
	if status == EccMemberStatusActive {
		joinedAt = &now
	} else if status == EccMemberStatusLeft {
		leftAt = &now
	}

	_, err := s.DB.ExecContext(ctx, `
		UPDATE act_ecc_members
		SET status = $1,
		    joined_at = COALESCE($2, joined_at),
		    left_at = COALESCE($3, left_at),
		    bill_id = COALESCE($4, bill_id),
		    updated_at = $5
		WHERE id = $6 AND school_id = $7`,
		status, joinedAt, leftAt, billID, now, id, schoolID)
	return err
}

func (s *Store) ListMyMemberships(ctx context.Context, schoolID, studentUserID string) ([]EccMember, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT m.id, m.school_id, m.ecc_id, m.period_id, m.student_user_id,
		       m.status, m.waitlisted_at, m.joined_at, m.left_at, m.bill_id,
		       m.created_at, m.updated_at, e.name as ecc_name
		FROM act_ecc_members m
		JOIN act_extracurriculars e ON m.ecc_id = e.id
		WHERE m.school_id = $1 AND m.student_user_id = $2
		ORDER BY m.created_at DESC`, schoolID, studentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EccMember
	for rows.Next() {
		var m EccMember
		if err := rows.Scan(
			&m.ID, &m.SchoolID, &m.EccID, &m.PeriodID, &m.StudentUserID,
			&m.Status, &m.WaitlistedAt, &m.JoinedAt, &m.LeftAt, &m.BillID,
			&m.CreatedAt, &m.UpdatedAt, &m.EccName,
		); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

func (s *Store) ListSessions(ctx context.Context, schoolID, eccID string) ([]EccSession, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, ecc_id, held_on, note, created_by, created_at
		FROM act_ecc_sessions
		WHERE school_id = $1 AND ecc_id = $2
		ORDER BY held_on DESC`, schoolID, eccID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EccSession
	for rows.Next() {
		var ses EccSession
		if err := rows.Scan(&ses.ID, &ses.SchoolID, &ses.EccID, &ses.HeldOn, &ses.Note, &ses.CreatedBy, &ses.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, ses)
	}
	return list, nil
}

type AttendanceInput struct {
	MemberID string `json:"member_id"`
	Status   string `json:"status"` // 'hadir','izin','sakit','alpa'
}

func (s *Store) CreateSessionWithAttendance(ctx context.Context, ses EccSession, atts []AttendanceInput) (*EccSession, error) {
	if ses.ID == "" {
		ses.ID = store.NewID()
	}
	now := time.Now()
	ses.CreatedAt = now

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO act_ecc_sessions (id, school_id, ecc_id, held_on, note, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		ses.ID, ses.SchoolID, ses.EccID, ses.HeldOn, ses.Note, ses.CreatedBy, ses.CreatedAt)
	if err != nil {
		return nil, err
	}

	for _, a := range atts {
		if a.MemberID != "" && a.Status != "" {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO act_ecc_attendance (session_id, member_id, school_id, status, created_at)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (session_id, member_id) DO UPDATE SET status = EXCLUDED.status`,
				ses.ID, a.MemberID, ses.SchoolID, a.Status, now)
			if err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ses, nil
}

func (s *Store) ListAssessments(ctx context.Context, schoolID, eccID, periodID string) ([]EccAssessment, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, ecc_id, period_id, member_id, predicate, description, assessed_by, created_at, updated_at
		FROM act_ecc_assessments
		WHERE school_id = $1 AND ecc_id = $2 AND period_id = $3
		ORDER BY created_at ASC`, schoolID, eccID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EccAssessment
	for rows.Next() {
		var a EccAssessment
		if err := rows.Scan(
			&a.ID, &a.SchoolID, &a.EccID, &a.PeriodID, &a.MemberID,
			&a.Predicate, &a.Description, &a.AssessedBy, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

func (s *Store) UpsertAssessment(ctx context.Context, a EccAssessment) (*EccAssessment, error) {
	if a.ID == "" {
		a.ID = store.NewID()
	}
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now

	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO act_ecc_assessments (id, school_id, ecc_id, period_id, member_id, predicate, description, assessed_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (member_id, period_id)
		DO UPDATE SET predicate = EXCLUDED.predicate, description = EXCLUDED.description,
		              assessed_by = EXCLUDED.assessed_by, updated_at = EXCLUDED.updated_at`,
		a.ID, a.SchoolID, a.EccID, a.PeriodID, a.MemberID,
		a.Predicate, a.Description, a.AssessedBy, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// -------------------------------------------------------------
// 6. Achievements
// -------------------------------------------------------------

func (s *Store) ListAchievements(ctx context.Context, schoolID, status, studentUserID string) ([]Achievement, error) {
	q := `
		SELECT id, school_id, student_user_id, activity_name, organizer, level,
		       rank_label, achieved_on, evidence, event_id, ecc_id, submitted_by,
		       submitted_via, status, verified_by, verified_at, reject_reason,
		       created_at, updated_at, deleted_at
		FROM act_achievements
		WHERE school_id = $1 AND deleted_at IS NULL`

	args := []any{schoolID}
	idx := 2

	if status != "" {
		q += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if studentUserID != "" {
		q += fmt.Sprintf(" AND student_user_id = $%d", idx)
		args = append(args, studentUserID)
		idx++
	}

	q += " ORDER BY achieved_on DESC, created_at DESC"

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Achievement
	for rows.Next() {
		var a Achievement
		var evidenceRaw []byte
		if err := rows.Scan(
			&a.ID, &a.SchoolID, &a.StudentUserID, &a.ActivityName, &a.Organizer,
			&a.Level, &a.RankLabel, &a.AchievedOn, &evidenceRaw, &a.EventID,
			&a.EccID, &a.SubmittedBy, &a.SubmittedVia, &a.Status, &a.VerifiedBy,
			&a.VerifiedAt, &a.RejectReason, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
		); err != nil {
			return nil, err
		}
		a.Evidence = evidenceRaw
		list = append(list, a)
	}
	return list, nil
}

func (s *Store) GetAchievement(ctx context.Context, schoolID, id string) (*Achievement, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, student_user_id, activity_name, organizer, level,
		       rank_label, achieved_on, evidence, event_id, ecc_id, submitted_by,
		       submitted_via, status, verified_by, verified_at, reject_reason,
		       created_at, updated_at, deleted_at
		FROM act_achievements
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL`, id, schoolID)

	var a Achievement
	var evidenceRaw []byte
	if err := row.Scan(
		&a.ID, &a.SchoolID, &a.StudentUserID, &a.ActivityName, &a.Organizer,
		&a.Level, &a.RankLabel, &a.AchievedOn, &evidenceRaw, &a.EventID,
		&a.EccID, &a.SubmittedBy, &a.SubmittedVia, &a.Status, &a.VerifiedBy,
		&a.VerifiedAt, &a.RejectReason, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	a.Evidence = evidenceRaw
	return &a, nil
}

func (s *Store) CreateAchievement(ctx context.Context, a Achievement) (*Achievement, error) {
	if a.ID == "" {
		a.ID = store.NewID()
	}
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now

	if len(a.Evidence) == 0 {
		a.Evidence = []byte("[]")
	}

	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO act_achievements (
			id, school_id, student_user_id, activity_name, organizer, level,
			rank_label, achieved_on, evidence, event_id, ecc_id, submitted_by,
			submitted_via, status, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16
		)`,
		a.ID, a.SchoolID, a.StudentUserID, a.ActivityName, a.Organizer, a.Level,
		a.RankLabel, a.AchievedOn, a.Evidence, a.EventID, a.EccID, a.SubmittedBy,
		a.SubmittedVia, a.Status, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) VerifyAchievement(ctx context.Context, schoolID, id, verifierID, status string, rejectReason *string) error {
	now := time.Now()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE act_achievements
		SET status = $1, verified_by = $2, verified_at = $3, reject_reason = $4, updated_at = $5
		WHERE id = $6 AND school_id = $7 AND deleted_at IS NULL AND submitted_by <> $8`,
		status, verifierID, now, rejectReason, now, id, schoolID, verifierID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) ListAllVerifiedAchievements(ctx context.Context, schoolID, from, to string) ([]Achievement, error) {
	q := `
		SELECT id, school_id, student_user_id, activity_name, organizer, level,
		       rank_label, achieved_on, evidence, event_id, ecc_id, submitted_by,
		       submitted_via, status, verified_by, verified_at, reject_reason,
		       created_at, updated_at, deleted_at
		FROM act_achievements
		WHERE school_id = $1 AND deleted_at IS NULL AND status = 'verified'`

	args := []any{schoolID}
	idx := 2

	if from != "" {
		q += fmt.Sprintf(" AND achieved_on >= $%d", idx)
		args = append(args, from)
		idx++
	}
	if to != "" {
		q += fmt.Sprintf(" AND achieved_on <= $%d", idx)
		args = append(args, to)
		idx++
	}
	q += " ORDER BY achieved_on DESC, created_at DESC"

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Achievement
	for rows.Next() {
		var a Achievement
		var evidenceRaw []byte
		if err := rows.Scan(
			&a.ID, &a.SchoolID, &a.StudentUserID, &a.ActivityName, &a.Organizer,
			&a.Level, &a.RankLabel, &a.AchievedOn, &evidenceRaw, &a.EventID,
			&a.EccID, &a.SubmittedBy, &a.SubmittedVia, &a.Status, &a.VerifiedBy,
			&a.VerifiedAt, &a.RejectReason, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
		); err != nil {
			return nil, err
		}
		a.Evidence = evidenceRaw
		list = append(list, a)
	}
	return list, nil
}
