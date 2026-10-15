package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound       = errors.New("library: record not found")
	ErrConflict       = errors.New("library: record conflict")
	ErrNotPermitted   = errors.New("library: operation not permitted")
	ErrInvalidBarcode = errors.New("library: barcode tidak valid atau tidak ditemukan")
	ErrInvalidMember  = errors.New("library: kartu anggota tidak valid atau tidak ditemukan")
)

type Store struct {
	db  *sql.DB
	loc *time.Location
}

func NewStore(db *sql.DB, loc *time.Location) *Store {
	if loc == nil {
		loc = time.UTC
	}
	return &Store{db: db, loc: loc}
}

// --- Settings ---

func (s *Store) GetSettings(ctx context.Context, schoolID string) (Settings, error) {
	var st Settings
	err := s.db.QueryRowContext(ctx, `
		SELECT school_id, fine_policy, reminder_days_before, created_at, updated_at
		FROM lib_settings WHERE school_id = $1
	`, schoolID).Scan(&st.SchoolID, &st.FinePolicy, &st.ReminderDaysBefore, &st.CreatedAt, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		// Return default
		return Settings{
			SchoolID:           schoolID,
			FinePolicy:         "pay_at_library",
			ReminderDaysBefore: 2,
		}, nil
	}
	return st, err
}

func (s *Store) UpdateSettings(ctx context.Context, st Settings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO lib_settings (school_id, fine_policy, reminder_days_before, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (school_id) DO UPDATE SET
			fine_policy = EXCLUDED.fine_policy,
			reminder_days_before = EXCLUDED.reminder_days_before,
			updated_at = now()
	`, st.SchoolID, st.FinePolicy, st.ReminderDaysBefore)
	return err
}

// --- Member Categories ---

func (s *Store) ListMemberCategories(ctx context.Context, schoolID string) ([]MemberCategory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, name, applies_to, is_default, max_loans, loan_days, max_renewals, reservation_hold_days, deleted_at, created_at, updated_at
		FROM lib_member_categories
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY applies_to, name
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []MemberCategory
	for rows.Next() {
		var c MemberCategory
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.Name, &c.AppliesTo, &c.IsDefault, &c.MaxLoans, &c.LoanDays, &c.MaxRenewals, &c.ReservationHoldDays, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, nil
}

func (s *Store) GetMemberCategory(ctx context.Context, schoolID, id string) (MemberCategory, error) {
	var c MemberCategory
	err := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, name, applies_to, is_default, max_loans, loan_days, max_renewals, reservation_hold_days, deleted_at, created_at, updated_at
		FROM lib_member_categories
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id).Scan(&c.ID, &c.SchoolID, &c.Name, &c.AppliesTo, &c.IsDefault, &c.MaxLoans, &c.LoanDays, &c.MaxRenewals, &c.ReservationHoldDays, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) CreateMemberCategory(ctx context.Context, c MemberCategory) (MemberCategory, error) {
	if c.IsDefault {
		_, _ = s.db.ExecContext(ctx, `
			UPDATE lib_member_categories SET is_default = false
			WHERE school_id = $1 AND applies_to = $2 AND deleted_at IS NULL
		`, c.SchoolID, c.AppliesTo)
	}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO lib_member_categories (school_id, name, applies_to, is_default, max_loans, loan_days, max_renewals, reservation_hold_days)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, c.SchoolID, c.Name, c.AppliesTo, c.IsDefault, c.MaxLoans, c.LoanDays, c.MaxRenewals, c.ReservationHoldDays).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) UpdateMemberCategory(ctx context.Context, c MemberCategory) error {
	if c.IsDefault {
		_, _ = s.db.ExecContext(ctx, `
			UPDATE lib_member_categories SET is_default = false
			WHERE school_id = $1 AND applies_to = $2 AND id != $3 AND deleted_at IS NULL
		`, c.SchoolID, c.AppliesTo, c.ID)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_member_categories
		SET name = $1, is_default = $2, max_loans = $3, loan_days = $4, max_renewals = $5, reservation_hold_days = $6, updated_at = now()
		WHERE school_id = $7 AND id = $8 AND deleted_at IS NULL
	`, c.Name, c.IsDefault, c.MaxLoans, c.LoanDays, c.MaxRenewals, c.ReservationHoldDays, c.SchoolID, c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteMemberCategory(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_member_categories SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetDefaultCategory(ctx context.Context, schoolID, appliesTo string) (MemberCategory, error) {
	var c MemberCategory
	err := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, name, applies_to, is_default, max_loans, loan_days, max_renewals, reservation_hold_days, deleted_at, created_at, updated_at
		FROM lib_member_categories
		WHERE school_id = $1 AND applies_to = $2 AND is_default = true AND deleted_at IS NULL
	`, schoolID, appliesTo).Scan(&c.ID, &c.SchoolID, &c.Name, &c.AppliesTo, &c.IsDefault, &c.MaxLoans, &c.LoanDays, &c.MaxRenewals, &c.ReservationHoldDays, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	return c, err
}

// --- Fine Rates ---

func (s *Store) ListFineRates(ctx context.Context, schoolID string) ([]FineRate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, member_category_id, late_fee_per_day_idr, lost_fee_idr, effective_from, effective_to, created_at, updated_at
		FROM lib_fine_rates
		WHERE school_id = $1
		ORDER BY effective_from DESC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FineRate
	for rows.Next() {
		var r FineRate
		if err := rows.Scan(&r.ID, &r.SchoolID, &r.MemberCategoryID, &r.LateFeePerDayIDR, &r.LostFeeIDR, &r.EffectiveFrom, &r.EffectiveTo, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

func (s *Store) CreateFineRate(ctx context.Context, r FineRate) (FineRate, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO lib_fine_rates (school_id, member_category_id, late_fee_per_day_idr, lost_fee_idr, effective_from, effective_to)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, r.SchoolID, r.MemberCategoryID, r.LateFeePerDayIDR, r.LostFeeIDR, r.EffectiveFrom, r.EffectiveTo).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) DeleteFineRate(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM lib_fine_rates WHERE school_id = $1 AND id = $2`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetActiveRates(ctx context.Context, schoolID string, onDate time.Time) ([]FineRate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, member_category_id, late_fee_per_day_idr, lost_fee_idr, effective_from, effective_to, created_at, updated_at
		FROM lib_fine_rates
		WHERE school_id = $1 AND effective_from <= $2 AND (effective_to IS NULL OR effective_to >= $2)
	`, schoolID, onDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FineRate
	for rows.Next() {
		var r FineRate
		if err := rows.Scan(&r.ID, &r.SchoolID, &r.MemberCategoryID, &r.LateFeePerDayIDR, &r.LostFeeIDR, &r.EffectiveFrom, &r.EffectiveTo, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

// --- Collection Categories ---

func (s *Store) ListCollectionCategories(ctx context.Context, schoolID string) ([]CollectionCategory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, code, name, deleted_at, created_at, updated_at
		FROM lib_collection_categories
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY code
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []CollectionCategory
	for rows.Next() {
		var c CollectionCategory
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.Code, &c.Name, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, nil
}

func (s *Store) CreateCollectionCategory(ctx context.Context, c CollectionCategory) (CollectionCategory, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO lib_collection_categories (school_id, code, name)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`, c.SchoolID, c.Code, c.Name).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) UpdateCollectionCategory(ctx context.Context, c CollectionCategory) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_collection_categories SET name = $1, updated_at = now()
		WHERE school_id = $2 AND id = $3 AND deleted_at IS NULL
	`, c.Name, c.SchoolID, c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCollectionCategory(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_collection_categories SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Titles & Copies ---

func (s *Store) ListTitles(ctx context.Context, schoolID, q, categoryID string) ([]Title, error) {
	query := `
		SELECT t.id, t.school_id, t.title, t.author, t.publisher, t.isbn, t.publish_year,
		       t.category_id, c.name, t.replacement_price_idr, t.deleted_at, t.created_at, t.updated_at,
		       COUNT(cp.id) AS total_copies,
		       COUNT(cp.id) FILTER (WHERE cp.status = 'available' AND cp.deleted_at IS NULL) AS available_copies
		FROM lib_titles t
		JOIN lib_collection_categories c ON c.id = t.category_id
		LEFT JOIN lib_copies cp ON cp.title_id = t.id AND cp.deleted_at IS NULL
		WHERE t.school_id = $1 AND t.deleted_at IS NULL
	`
	args := []any{schoolID}
	argIdx := 2

	if categoryID != "" {
		query += fmt.Sprintf(" AND t.category_id = $%d", argIdx)
		args = append(args, categoryID)
		argIdx++
	}
	if q != "" {
		query += fmt.Sprintf(" AND (t.search @@ plainto_tsquery('simple', $%d) OR t.title ILIKE '%%' || $%d || '%%' OR t.author ILIKE '%%' || $%d || '%%')", argIdx, argIdx, argIdx)
		args = append(args, q)
		argIdx++
	}

	query += ` GROUP BY t.id, c.name ORDER BY t.title ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Title
	for rows.Next() {
		var t Title
		if err := rows.Scan(
			&t.ID, &t.SchoolID, &t.Title, &t.Author, &t.Publisher, &t.ISBN, &t.PublishYear,
			&t.CategoryID, &t.CategoryName, &t.ReplacementPriceIDR, &t.DeletedAt, &t.CreatedAt, &t.UpdatedAt,
			&t.TotalCopies, &t.AvailableCopies,
		); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

func (s *Store) GetTitle(ctx context.Context, schoolID, id string) (Title, error) {
	var t Title
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.school_id, t.title, t.author, t.publisher, t.isbn, t.publish_year,
		       t.category_id, c.name, t.replacement_price_idr, t.deleted_at, t.created_at, t.updated_at,
		       COUNT(cp.id) AS total_copies,
		       COUNT(cp.id) FILTER (WHERE cp.status = 'available' AND cp.deleted_at IS NULL) AS available_copies
		FROM lib_titles t
		JOIN lib_collection_categories c ON c.id = t.category_id
		LEFT JOIN lib_copies cp ON cp.title_id = t.id AND cp.deleted_at IS NULL
		WHERE t.school_id = $1 AND t.id = $2 AND t.deleted_at IS NULL
		GROUP BY t.id, c.name
	`, schoolID, id).Scan(
		&t.ID, &t.SchoolID, &t.Title, &t.Author, &t.Publisher, &t.ISBN, &t.PublishYear,
		&t.CategoryID, &t.CategoryName, &t.ReplacementPriceIDR, &t.DeletedAt, &t.CreatedAt, &t.UpdatedAt,
		&t.TotalCopies, &t.AvailableCopies,
	)
	if err == sql.ErrNoRows {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}

	// Fetch copies
	copies, err := s.ListCopies(ctx, schoolID, id)
	if err != nil {
		return t, err
	}
	t.Copies = copies
	return t, nil
}

func (s *Store) CreateTitle(ctx context.Context, t Title) (Title, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO lib_titles (school_id, title, author, publisher, isbn, publish_year, category_id, replacement_price_idr)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, t.SchoolID, t.Title, t.Author, t.Publisher, t.ISBN, t.PublishYear, t.CategoryID, t.ReplacementPriceIDR).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *Store) UpdateTitle(ctx context.Context, t Title) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_titles
		SET title = $1, author = $2, publisher = $3, isbn = $4, publish_year = $5, category_id = $6, replacement_price_idr = $7, updated_at = now()
		WHERE school_id = $8 AND id = $9 AND deleted_at IS NULL
	`, t.Title, t.Author, t.Publisher, t.ISBN, t.PublishYear, t.CategoryID, t.ReplacementPriceIDR, t.SchoolID, t.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTitle(ctx context.Context, schoolID, id string) error {
	// Cannot delete title if copies are currently on_loan
	var onLoanCount int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM lib_copies
		WHERE school_id = $1 AND title_id = $2 AND status = 'on_loan' AND deleted_at IS NULL
	`, schoolID, id).Scan(&onLoanCount)
	if err != nil {
		return err
	}
	if onLoanCount > 0 {
		return fmt.Errorf("tidak dapat menghapus judul: masih ada %d eksemplar yang sedang dipinjam", onLoanCount)
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_titles SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	// Soft delete copies
	_, _ = s.db.ExecContext(ctx, `UPDATE lib_copies SET deleted_at = now() WHERE school_id = $1 AND title_id = $2`, schoolID, id)
	return nil
}

// --- Copies ---

func (s *Store) ListCopies(ctx context.Context, schoolID, titleID string) ([]Copy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.school_id, c.title_id, c.barcode, c.shelf_location, c.status, c.deleted_at, c.created_at, c.updated_at, t.title
		FROM lib_copies c
		JOIN lib_titles t ON t.id = c.title_id
		WHERE c.school_id = $1 AND c.title_id = $2 AND c.deleted_at IS NULL
		ORDER BY c.barcode ASC
	`, schoolID, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Copy
	for rows.Next() {
		var cp Copy
		if err := rows.Scan(&cp.ID, &cp.SchoolID, &cp.TitleID, &cp.Barcode, &cp.ShelfLocation, &cp.Status, &cp.DeletedAt, &cp.CreatedAt, &cp.UpdatedAt, &cp.TitleName); err != nil {
			return nil, err
		}
		list = append(list, cp)
	}
	return list, nil
}

func (s *Store) GetCopyByBarcode(ctx context.Context, schoolID, barcode string) (Copy, error) {
	var cp Copy
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.school_id, c.title_id, c.barcode, c.shelf_location, c.status, c.deleted_at, c.created_at, c.updated_at, t.title
		FROM lib_copies c
		JOIN lib_titles t ON t.id = c.title_id
		WHERE c.school_id = $1 AND c.barcode = $2 AND c.deleted_at IS NULL
	`, schoolID, barcode).Scan(&cp.ID, &cp.SchoolID, &cp.TitleID, &cp.Barcode, &cp.ShelfLocation, &cp.Status, &cp.DeletedAt, &cp.CreatedAt, &cp.UpdatedAt, &cp.TitleName)
	if err == sql.ErrNoRows {
		return cp, ErrInvalidBarcode
	}
	return cp, err
}

func (s *Store) CreateCopy(ctx context.Context, cp Copy) (Copy, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO lib_copies (school_id, title_id, barcode, shelf_location, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, cp.SchoolID, cp.TitleID, cp.Barcode, cp.ShelfLocation, cp.Status).Scan(&cp.ID, &cp.CreatedAt, &cp.UpdatedAt)
	return cp, err
}

func (s *Store) UpdateCopy(ctx context.Context, cp Copy) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_copies
		SET shelf_location = $1, status = $2, updated_at = now()
		WHERE school_id = $3 AND id = $4 AND deleted_at IS NULL
	`, cp.ShelfLocation, cp.Status, cp.SchoolID, cp.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCopy(ctx context.Context, schoolID, id string) error {
	var status string
	err := s.db.QueryRowContext(ctx, `
		SELECT status FROM lib_copies WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id).Scan(&status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == "on_loan" {
		return errors.New("eksemplar yang sedang dipinjam tidak dapat dihapus")
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE lib_copies SET deleted_at = now() WHERE school_id = $1 AND id = $2
	`, schoolID, id)
	return err
}

// --- Members ---

func (s *Store) ListMembers(ctx context.Context, schoolID, status, q string) ([]Member, error) {
	query := `
		SELECT m.id, m.school_id, m.kind, m.user_id, m.category_id, c.name, m.member_code,
		       m.status, m.pending_reason, m.closed_at, m.synced_at, m.created_at, m.updated_at
		FROM lib_members m
		JOIN lib_member_categories c ON c.id = m.category_id
		WHERE m.school_id = $1
	`
	args := []any{schoolID}
	argIdx := 2

	if status != "" {
		query += fmt.Sprintf(" AND m.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if q != "" {
		query += fmt.Sprintf(" AND (m.member_code ILIKE '%%' || $%d || '%%')", argIdx)
		args = append(args, q)
		argIdx++
	}

	query += ` ORDER BY m.member_code ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(
			&m.ID, &m.SchoolID, &m.Kind, &m.UserID, &m.CategoryID, &m.CategoryName, &m.MemberCode,
			&m.Status, &m.PendingReason, &m.ClosedAt, &m.SyncedAt, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

func (s *Store) GetMember(ctx context.Context, schoolID, id string) (Member, error) {
	var m Member
	err := s.db.QueryRowContext(ctx, `
		SELECT m.id, m.school_id, m.kind, m.user_id, m.category_id, c.name, m.member_code,
		       m.status, m.pending_reason, m.closed_at, m.synced_at, m.created_at, m.updated_at
		FROM lib_members m
		JOIN lib_member_categories c ON c.id = m.category_id
		WHERE m.school_id = $1 AND m.id = $2
	`, schoolID, id).Scan(
		&m.ID, &m.SchoolID, &m.Kind, &m.UserID, &m.CategoryID, &m.CategoryName, &m.MemberCode,
		&m.Status, &m.PendingReason, &m.ClosedAt, &m.SyncedAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return m, ErrNotFound
	}
	return m, err
}

func (s *Store) GetMemberByCode(ctx context.Context, schoolID, code string) (Member, error) {
	var m Member
	err := s.db.QueryRowContext(ctx, `
		SELECT m.id, m.school_id, m.kind, m.user_id, m.category_id, c.name, m.member_code,
		       m.status, m.pending_reason, m.closed_at, m.synced_at, m.created_at, m.updated_at
		FROM lib_members m
		JOIN lib_member_categories c ON c.id = m.category_id
		WHERE m.school_id = $1 AND m.member_code = $2
	`, schoolID, code).Scan(
		&m.ID, &m.SchoolID, &m.Kind, &m.UserID, &m.CategoryID, &m.CategoryName, &m.MemberCode,
		&m.Status, &m.PendingReason, &m.ClosedAt, &m.SyncedAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return m, ErrInvalidMember
	}
	return m, err
}

func (s *Store) GetMemberByUserID(ctx context.Context, schoolID, userID string) (Member, error) {
	var m Member
	err := s.db.QueryRowContext(ctx, `
		SELECT m.id, m.school_id, m.kind, m.user_id, m.category_id, c.name, m.member_code,
		       m.status, m.pending_reason, m.closed_at, m.synced_at, m.created_at, m.updated_at
		FROM lib_members m
		JOIN lib_member_categories c ON c.id = m.category_id
		WHERE m.school_id = $1 AND m.user_id = $2
	`, schoolID, userID).Scan(
		&m.ID, &m.SchoolID, &m.Kind, &m.UserID, &m.CategoryID, &m.CategoryName, &m.MemberCode,
		&m.Status, &m.PendingReason, &m.ClosedAt, &m.SyncedAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return m, ErrNotFound
	}
	return m, err
}

func (s *Store) UpsertMember(ctx context.Context, m Member) (Member, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO lib_members (school_id, kind, user_id, category_id, member_code, status, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (school_id, user_id) DO UPDATE SET
			category_id = EXCLUDED.category_id,
			synced_at = now(),
			updated_at = now()
		RETURNING id, created_at, updated_at
	`, m.SchoolID, m.Kind, m.UserID, m.CategoryID, m.MemberCode, m.Status).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func (s *Store) UpdateMemberCategoryOnly(ctx context.Context, schoolID, id, categoryID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_members SET category_id = $1, updated_at = now()
		WHERE school_id = $2 AND id = $3
	`, categoryID, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateMemberStatus(ctx context.Context, schoolID, id, status string, pendingReason *string) error {
	query := `UPDATE lib_members SET status = $1, pending_reason = $2, updated_at = now()`
	if status == "inactive" {
		query += `, closed_at = now()`
	}
	query += ` WHERE school_id = $3 AND id = $4`
	_, err := s.db.ExecContext(ctx, query, status, pendingReason, schoolID, id)
	return err
}

func (s *Store) GetMemberStats(ctx context.Context, schoolID, memberID string, today time.Time) (activeLoans int, hasOverdue bool, hasOpenFine bool, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'active'),
			COALESCE(bool_or(status = 'active' AND due_date < $3::date), false)
		FROM lib_loans
		WHERE school_id = $1 AND member_id = $2
	`, schoolID, memberID, today).Scan(&activeLoans, &hasOverdue)
	if err != nil {
		return 0, false, false, err
	}

	var openFinesCount int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM lib_fines
		WHERE school_id = $1 AND member_id = $2 AND status = 'open' AND settlement = 'pay_at_library'
	`, schoolID, memberID).Scan(&openFinesCount)
	if err != nil {
		return 0, false, false, err
	}

	hasOpenFine = openFinesCount > 0
	return activeLoans, hasOverdue, hasOpenFine, nil
}

// --- Loans & Circulation ---

func (s *Store) BorrowCopy(ctx context.Context, schoolID, memberCode, barcode, classroomID, classroomName, loanedBy, idempotencyKey string) (*Loan, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Idempotency check
	var existing Loan
	err = tx.QueryRowContext(ctx, `
		SELECT l.id, l.school_id, l.copy_id, l.member_id, l.classroom_id, l.classroom_name,
		       l.loaned_at, l.due_date, l.returned_at, l.renew_count, l.status, l.loaned_by,
		       l.returned_by, l.idempotency_key, l.created_at, l.updated_at
		FROM lib_loans l
		WHERE l.school_id = $1 AND l.idempotency_key = $2
	`, schoolID, idempotencyKey).Scan(
		&existing.ID, &existing.SchoolID, &existing.CopyID, &existing.MemberID, &existing.ClassroomID, &existing.ClassroomName,
		&existing.LoanedAt, &existing.DueDate, &existing.ReturnedAt, &existing.RenewCount, &existing.Status, &existing.LoanedBy,
		&existing.ReturnedBy, &existing.IdempotencyKey, &existing.CreatedAt, &existing.UpdatedAt,
	)
	if err == nil {
		_ = tx.Commit()
		return &existing, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	// 1. Lock member & category
	var member Member
	var category MemberCategory
	err = tx.QueryRowContext(ctx, `
		SELECT m.id, m.school_id, m.kind, m.user_id, m.category_id, m.member_code, m.status,
		       c.id, c.max_loans, c.loan_days, c.max_renewals
		FROM lib_members m
		JOIN lib_member_categories c ON c.id = m.category_id
		WHERE m.school_id = $1 AND m.member_code = $2
		FOR UPDATE OF m
	`, schoolID, memberCode).Scan(
		&member.ID, &member.SchoolID, &member.Kind, &member.UserID, &member.CategoryID, &member.MemberCode, &member.Status,
		&category.ID, &category.MaxLoans, &category.LoanDays, &category.MaxRenewals,
	)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidMember
	}
	if err != nil {
		return nil, err
	}

	// 2. Lock copy
	var cp Copy
	err = tx.QueryRowContext(ctx, `
		SELECT id, school_id, title_id, barcode, shelf_location, status
		FROM lib_copies
		WHERE school_id = $1 AND barcode = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, schoolID, barcode).Scan(&cp.ID, &cp.SchoolID, &cp.TitleID, &cp.Barcode, &cp.ShelfLocation, &cp.Status)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidBarcode
	}
	if err != nil {
		return nil, err
	}

	if cp.Status != "available" {
		if cp.Status == "on_hold" {
			// Check if on hold for this member
			var resCount int
			_ = tx.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM lib_reservations
				WHERE school_id = $1 AND held_copy_id = $2 AND member_id = $3 AND status = 'ready'
			`, schoolID, cp.ID, member.ID).Scan(&resCount)
			if resCount == 0 {
				return nil, errors.New("eksemplar sedang di-hold untuk anggota lain")
			}
		} else {
			return nil, fmt.Errorf("eksemplar tidak tersedia untuk dipinjam (status: %s)", cp.Status)
		}
	}

	// 3. Check CanBorrow stats
	now := time.Now().In(s.loc)
	var activeLoans int
	var hasOverdue bool
	err = tx.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(bool_or(due_date < $3::date), false)
		FROM lib_loans
		WHERE school_id = $1 AND member_id = $2 AND status = 'active'
	`, schoolID, member.ID, now).Scan(&activeLoans, &hasOverdue)
	if err != nil {
		return nil, err
	}

	var openFines int
	_ = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM lib_fines
		WHERE school_id = $1 AND member_id = $2 AND status = 'open' AND settlement = 'pay_at_library'
	`, schoolID, member.ID).Scan(&openFines)

	canBorrow, reason := CanBorrow(member, category, activeLoans, hasOverdue, openFines > 0)
	if !canBorrow {
		return nil, errors.New(reason)
	}

	// 4. Create loan
	dueDate := DueDate(now, category.LoanDays, s.loc)
	var loan Loan
	loan.SchoolID = schoolID
	loan.CopyID = cp.ID
	loan.MemberID = member.ID
	if classroomID != "" {
		loan.ClassroomID = &classroomID
	}
	if classroomName != "" {
		loan.ClassroomName = &classroomName
	}
	loan.LoanedAt = now
	loan.DueDate = dueDate
	loan.RenewCount = 0
	loan.Status = "active"
	loan.LoanedBy = loanedBy
	loan.IdempotencyKey = idempotencyKey

	err = tx.QueryRowContext(ctx, `
		INSERT INTO lib_loans (school_id, copy_id, member_id, classroom_id, classroom_name, loaned_at, due_date, renew_count, status, loaned_by, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at
	`, loan.SchoolID, loan.CopyID, loan.MemberID, loan.ClassroomID, loan.ClassroomName, loan.LoanedAt, loan.DueDate, loan.RenewCount, loan.Status, loan.LoanedBy, loan.IdempotencyKey).Scan(&loan.ID, &loan.CreatedAt, &loan.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// 5. Update copy status to 'on_loan'
	_, err = tx.ExecContext(ctx, `
		UPDATE lib_copies SET status = 'on_loan', updated_at = now()
		WHERE id = $1
	`, cp.ID)
	if err != nil {
		return nil, err
	}

	// 6. If held for reservation, fulfill reservation
	_, _ = tx.ExecContext(ctx, `
		UPDATE lib_reservations SET status = 'fulfilled', updated_at = now()
		WHERE school_id = $1 AND held_copy_id = $2 AND member_id = $3 AND status = 'ready'
	`, schoolID, cp.ID, member.ID)

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &loan, nil
}

type ReturnResult struct {
	Loan         Loan  `json:"loan"`
	Fine         *Fine `json:"fine,omitempty"`
	NextReserved bool  `json:"next_reserved"`
}

func (s *Store) ReturnCopy(ctx context.Context, schoolID, barcode, returnedBy string, returnedOn time.Time) (*ReturnResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Lock copy and title
	var cp Copy
	var title Title
	err = tx.QueryRowContext(ctx, `
		SELECT c.id, c.school_id, c.title_id, c.barcode, c.status,
		       t.id, t.replacement_price_idr
		FROM lib_copies c
		JOIN lib_titles t ON t.id = c.title_id
		WHERE c.school_id = $1 AND c.barcode = $2 AND c.deleted_at IS NULL
		FOR UPDATE OF c
	`, schoolID, barcode).Scan(&cp.ID, &cp.SchoolID, &cp.TitleID, &cp.Barcode, &cp.Status, &title.ID, &title.ReplacementPriceIDR)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidBarcode
	}
	if err != nil {
		return nil, err
	}

	if cp.Status != "on_loan" {
		return nil, fmt.Errorf("eksemplar tidak dalam status dipinjam (status: %s)", cp.Status)
	}

	// 2. Lock active loan
	var loan Loan
	var member Member
	err = tx.QueryRowContext(ctx, `
		SELECT l.id, l.school_id, l.copy_id, l.member_id, l.loaned_at, l.due_date, l.renew_count, l.status,
		       m.id, m.kind, m.category_id, m.status
		FROM lib_loans l
		JOIN lib_members m ON m.id = l.member_id
		WHERE l.school_id = $1 AND l.copy_id = $2 AND l.status = 'active'
		FOR UPDATE OF l
	`, schoolID, cp.ID).Scan(
		&loan.ID, &loan.SchoolID, &loan.CopyID, &loan.MemberID, &loan.LoanedAt, &loan.DueDate, &loan.RenewCount, &loan.Status,
		&member.ID, &member.Kind, &member.CategoryID, &member.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("tidak ada pinjaman aktif untuk eksemplar ini: %w", err)
	}

	// 3. Close loan
	now := returnedOn.In(s.loc)
	loan.ReturnedAt = &now
	loan.ReturnedBy = &returnedBy
	loan.Status = "returned"

	_, err = tx.ExecContext(ctx, `
		UPDATE lib_loans SET status = 'returned', returned_at = $1, returned_by = $2, updated_at = now()
		WHERE id = $3
	`, now, returnedBy, loan.ID)
	if err != nil {
		return nil, err
	}

	// 4. Calculate fine if late
	lateDays := LateDays(loan.DueDate, now, s.loc)
	var fine *Fine
	if lateDays > 0 {
		// Resolve fine rate
		var rates []FineRate
		rRows, _ := tx.QueryContext(ctx, `
			SELECT id, school_id, member_category_id, late_fee_per_day_idr, lost_fee_idr, effective_from, effective_to
			FROM lib_fine_rates
			WHERE school_id = $1 AND effective_from <= $2 AND (effective_to IS NULL OR effective_to >= $2)
		`, schoolID, now)
		for rRows.Next() {
			var fr FineRate
			_ = rRows.Scan(&fr.ID, &fr.SchoolID, &fr.MemberCategoryID, &fr.LateFeePerDayIDR, &fr.LostFeeIDR, &fr.EffectiveFrom, &fr.EffectiveTo)
			rates = append(rates, fr)
		}
		rRows.Close()

		rate, hasRate := ResolveRate(rates, &member.CategoryID, now)
		ratePerDay := int64(1000) // fallback default
		rateSnapMap := map[string]any{"rate_per_day_idr": ratePerDay, "source": "fallback_default"}
		if hasRate {
			ratePerDay = rate.LateFeePerDayIDR
			rateSnapMap = map[string]any{
				"rate_id":          rate.ID,
				"rate_per_day_idr": ratePerDay,
				"effective_from":   rate.EffectiveFrom,
			}
		}

		fineAmount := LateFine(lateDays, ratePerDay)
		if fineAmount > 0 {
			// Get settings for policy
			var finePolicy string
			_ = tx.QueryRowContext(ctx, `SELECT fine_policy FROM lib_settings WHERE school_id = $1`, schoolID).Scan(&finePolicy)
			if finePolicy == "" {
				finePolicy = "pay_at_library"
			}
			settlement := Settlement(finePolicy, member.Kind)

			snapBytes, _ := json.Marshal(rateSnapMap)

			var f Fine
			f.SchoolID = schoolID
			f.LoanID = loan.ID
			f.MemberID = member.ID
			f.Kind = "late"
			f.LateDays = &lateDays
			f.AmountIDR = fineAmount
			f.RateSnapshot = rateSnapMap
			f.Settlement = settlement
			f.Status = "open"

			err = tx.QueryRowContext(ctx, `
				INSERT INTO lib_fines (school_id, loan_id, member_id, kind, late_days, amount_idr, rate_snapshot, settlement, status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				RETURNING id, created_at, updated_at
			`, f.SchoolID, f.LoanID, f.MemberID, f.Kind, f.LateDays, f.AmountIDR, snapBytes, f.Settlement, f.Status).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
			if err != nil {
				return nil, err
			}
			fine = &f
		}
	}

	// 5. Check waiting reservations for this title
	var nextResID string
	var nextMemberID string
	err = tx.QueryRowContext(ctx, `
		SELECT id, member_id
		FROM lib_reservations
		WHERE school_id = $1 AND title_id = $2 AND status = 'waiting'
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE
	`, schoolID, title.ID).Scan(&nextResID, &nextMemberID)

	nextReserved := false
	if err == nil {
		// Found reservation: put copy on_hold
		var holdDays int = 2
		_ = tx.QueryRowContext(ctx, `
			SELECT c.reservation_hold_days
			FROM lib_members m
			JOIN lib_member_categories c ON c.id = m.category_id
			WHERE m.id = $1
		`, nextMemberID).Scan(&holdDays)
		if holdDays <= 0 {
			holdDays = 2
		}

		readyUntil := now.AddDate(0, 0, holdDays)
		_, err = tx.ExecContext(ctx, `
			UPDATE lib_reservations
			SET status = 'ready', held_copy_id = $1, ready_until = $2, updated_at = now()
			WHERE id = $3
		`, cp.ID, readyUntil, nextResID)
		if err != nil {
			return nil, err
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE lib_copies SET status = 'on_hold', updated_at = now() WHERE id = $1
		`, cp.ID)
		if err != nil {
			return nil, err
		}
		nextReserved = true
	} else {
		// No waiting reservation: make copy available
		_, err = tx.ExecContext(ctx, `
			UPDATE lib_copies SET status = 'available', updated_at = now() WHERE id = $1
		`, cp.ID)
		if err != nil {
			return nil, err
		}
	}

	// 6. Check if member was pending_close and can now be transitioned
	if member.Status == "pending_close" {
		var activeLoansRem int
		var openFinesRem int
		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM lib_loans WHERE school_id = $1 AND member_id = $2 AND status = 'active'`, schoolID, member.ID).Scan(&activeLoansRem)
		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM lib_fines WHERE school_id = $1 AND member_id = $2 AND status = 'open'`, schoolID, member.ID).Scan(&openFinesRem)

		newSt, reason := CloseMembership(member, activeLoansRem, openFinesRem)
		if newSt == "inactive" {
			_, _ = tx.ExecContext(ctx, `
				UPDATE lib_members SET status = 'inactive', pending_reason = NULL, closed_at = now(), updated_at = now()
				WHERE id = $1
			`, member.ID)
		} else {
			_, _ = tx.ExecContext(ctx, `
				UPDATE lib_members SET pending_reason = $1, updated_at = now()
				WHERE id = $2
			`, reason, member.ID)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &ReturnResult{
		Loan:         loan,
		Fine:         fine,
		NextReserved: nextReserved,
	}, nil
}

func (s *Store) RenewLoan(ctx context.Context, schoolID, loanID string, today time.Time) (*Loan, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var loan Loan
	var cat MemberCategory
	var titleID string
	err = tx.QueryRowContext(ctx, `
		SELECT l.id, l.school_id, l.copy_id, l.member_id, l.due_date, l.renew_count, l.status,
		       c.title_id, mc.max_renewals, mc.loan_days
		FROM lib_loans l
		JOIN lib_copies c ON c.id = l.copy_id
		JOIN lib_members m ON m.id = l.member_id
		JOIN lib_member_categories mc ON mc.id = m.category_id
		WHERE l.school_id = $1 AND l.id = $2 AND l.status = 'active'
		FOR UPDATE OF l
	`, schoolID, loanID).Scan(
		&loan.ID, &loan.SchoolID, &loan.CopyID, &loan.MemberID, &loan.DueDate, &loan.RenewCount, &loan.Status,
		&titleID, &cat.MaxRenewals, &cat.LoanDays,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var waitingReservations int
	_ = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM lib_reservations
		WHERE school_id = $1 AND title_id = $2 AND status = 'waiting'
	`, schoolID, titleID).Scan(&waitingReservations)

	canRenew, reason := CanRenew(loan, cat, today, waitingReservations > 0, s.loc)
	if !canRenew {
		return nil, errors.New(reason)
	}

	newDue := DueDate(today, cat.LoanDays, s.loc)
	loan.DueDate = newDue
	loan.RenewCount++

	_, err = tx.ExecContext(ctx, `
		UPDATE lib_loans
		SET due_date = $1, renew_count = renew_count + 1, updated_at = now()
		WHERE id = $2
	`, newDue, loan.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &loan, nil
}

func (s *Store) ReportLost(ctx context.Context, schoolID, loanID, reportedBy string) (*Fine, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var loan Loan
	var member Member
	var title Title
	err = tx.QueryRowContext(ctx, `
		SELECT l.id, l.school_id, l.copy_id, l.member_id, l.status,
		       m.id, m.kind, m.category_id,
		       t.id, t.replacement_price_idr
		FROM lib_loans l
		JOIN lib_copies c ON c.id = l.copy_id
		JOIN lib_titles t ON t.id = c.title_id
		JOIN lib_members m ON m.id = l.member_id
		WHERE l.school_id = $1 AND l.id = $2 AND l.status = 'active'
		FOR UPDATE OF l
	`, schoolID, loanID).Scan(
		&loan.ID, &loan.SchoolID, &loan.CopyID, &loan.MemberID, &loan.Status,
		&member.ID, &member.Kind, &member.CategoryID,
		&title.ID, &title.ReplacementPriceIDR,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	now := time.Now().In(s.loc)

	// Close loan as lost
	_, err = tx.ExecContext(ctx, `
		UPDATE lib_loans SET status = 'lost', returned_at = $1, returned_by = $2, updated_at = now()
		WHERE id = $3
	`, now, reportedBy, loan.ID)
	if err != nil {
		return nil, err
	}

	// Update copy as lost
	_, err = tx.ExecContext(ctx, `
		UPDATE lib_copies SET status = 'lost', updated_at = now() WHERE id = $1
	`, loan.CopyID)
	if err != nil {
		return nil, err
	}

	// Calculate lost fine
	var rates []FineRate
	rRows, _ := tx.QueryContext(ctx, `
		SELECT id, school_id, member_category_id, late_fee_per_day_idr, lost_fee_idr, effective_from, effective_to
		FROM lib_fine_rates
		WHERE school_id = $1 AND effective_from <= $2 AND (effective_to IS NULL OR effective_to >= $2)
	`, schoolID, now)
	for rRows.Next() {
		var fr FineRate
		_ = rRows.Scan(&fr.ID, &fr.SchoolID, &fr.MemberCategoryID, &fr.LateFeePerDayIDR, &fr.LostFeeIDR, &fr.EffectiveFrom, &fr.EffectiveTo)
		rates = append(rates, fr)
	}
	rRows.Close()

	rate, hasRate := ResolveRate(rates, &member.CategoryID, now)
	var pRate *FineRate
	if hasRate {
		pRate = &rate
	}

	amount, err := LostFine(pRate, title)
	if err != nil {
		return nil, err
	}

	var finePolicy string
	_ = tx.QueryRowContext(ctx, `SELECT fine_policy FROM lib_settings WHERE school_id = $1`, schoolID).Scan(&finePolicy)
	if finePolicy == "" {
		finePolicy = "pay_at_library"
	}
	settlement := Settlement(finePolicy, member.Kind)

	snapMap := map[string]any{"kind": "lost", "calculated_at": now}
	snapBytes, _ := json.Marshal(snapMap)

	var f Fine
	f.SchoolID = schoolID
	f.LoanID = loan.ID
	f.MemberID = member.ID
	f.Kind = "lost"
	f.AmountIDR = amount
	f.RateSnapshot = snapMap
	f.Settlement = settlement
	f.Status = "open"

	err = tx.QueryRowContext(ctx, `
		INSERT INTO lib_fines (school_id, loan_id, member_id, kind, amount_idr, rate_snapshot, settlement, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, f.SchoolID, f.LoanID, f.MemberID, f.Kind, f.AmountIDR, snapBytes, f.Settlement, f.Status).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &f, nil
}

// --- Reservations ---

func (s *Store) ListReservations(ctx context.Context, schoolID, memberID string) ([]Reservation, error) {
	query := `
		SELECT r.id, r.school_id, r.title_id, r.member_id, r.status, r.held_copy_id, r.ready_until,
		       r.created_at, r.updated_at, t.title, m.member_code,
		       CASE WHEN r.status = 'waiting' THEN (
		           SELECT COUNT(*) FROM lib_reservations q
		           WHERE q.school_id = r.school_id AND q.title_id = r.title_id AND q.status = 'waiting'
		             AND (q.created_at, q.id) <= (r.created_at, r.id)
		       ) END AS queue_position
		FROM lib_reservations r
		JOIN lib_titles t ON t.id = r.title_id
		JOIN lib_members m ON m.id = r.member_id
		WHERE r.school_id = $1
	`
	args := []any{schoolID}
	if memberID != "" {
		query += ` AND r.member_id = $2`
		args = append(args, memberID)
	}
	query += ` ORDER BY r.created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Reservation
	for rows.Next() {
		var r Reservation
		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.TitleID, &r.MemberID, &r.Status, &r.HeldCopyID, &r.ReadyUntil,
			&r.CreatedAt, &r.UpdatedAt, &r.TitleName, &r.MemberCode, &r.QueuePosition,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

func (s *Store) CreateReservation(ctx context.Context, schoolID, titleID, memberID string) (*Reservation, error) {
	var r Reservation
	r.SchoolID = schoolID
	r.TitleID = titleID
	r.MemberID = memberID
	r.Status = "waiting"

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO lib_reservations (school_id, title_id, member_id, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`, r.SchoolID, r.TitleID, r.MemberID, r.Status).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) CancelReservation(ctx context.Context, schoolID, reservationID, memberID string) error {
	query := `
		UPDATE lib_reservations SET status = 'cancelled', updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status IN ('waiting', 'ready')
	`
	args := []any{schoolID, reservationID}
	if memberID != "" {
		query += ` AND member_id = $3`
		args = append(args, memberID)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Fines ---

func (s *Store) ListFines(ctx context.Context, schoolID, status, memberID string) ([]Fine, error) {
	query := `
		SELECT f.id, f.school_id, f.loan_id, f.member_id, f.kind, f.late_days, f.amount_idr,
		       f.rate_snapshot, f.settlement, f.status, f.bill_id, f.paid_at, f.received_by,
		       f.waived_by, f.waive_reason, f.created_at, f.updated_at,
		       m.member_code, t.title
		FROM lib_fines f
		JOIN lib_members m ON m.id = f.member_id
		JOIN lib_loans l ON l.id = f.loan_id
		JOIN lib_copies cp ON cp.id = l.copy_id
		JOIN lib_titles t ON t.id = cp.title_id
		WHERE f.school_id = $1
	`
	args := []any{schoolID}
	argIdx := 2

	if status != "" {
		query += fmt.Sprintf(" AND f.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if memberID != "" {
		query += fmt.Sprintf(" AND f.member_id = $%d", argIdx)
		args = append(args, memberID)
		argIdx++
	}
	query += ` ORDER BY f.created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Fine
	for rows.Next() {
		var f Fine
		var snap []byte
		if err := rows.Scan(
			&f.ID, &f.SchoolID, &f.LoanID, &f.MemberID, &f.Kind, &f.LateDays, &f.AmountIDR,
			&snap, &f.Settlement, &f.Status, &f.BillID, &f.PaidAt, &f.ReceivedBy,
			&f.WaivedBy, &f.WaiveReason, &f.CreatedAt, &f.UpdatedAt,
			&f.MemberCode, &f.TitleName,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(snap, &f.RateSnapshot)
		list = append(list, f)
	}
	return list, nil
}

func (s *Store) GetFine(ctx context.Context, schoolID, id string) (Fine, error) {
	var f Fine
	var snap []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT f.id, f.school_id, f.loan_id, f.member_id, f.kind, f.late_days, f.amount_idr,
		       f.rate_snapshot, f.settlement, f.status, f.bill_id, f.paid_at, f.received_by,
		       f.waived_by, f.waive_reason, f.created_at, f.updated_at,
		       m.member_code, t.title
		FROM lib_fines f
		JOIN lib_members m ON m.id = f.member_id
		JOIN lib_loans l ON l.id = f.loan_id
		JOIN lib_copies cp ON cp.id = l.copy_id
		JOIN lib_titles t ON t.id = cp.title_id
		WHERE f.school_id = $1 AND f.id = $2
	`, schoolID, id).Scan(
		&f.ID, &f.SchoolID, &f.LoanID, &f.MemberID, &f.Kind, &f.LateDays, &f.AmountIDR,
		&snap, &f.Settlement, &f.Status, &f.BillID, &f.PaidAt, &f.ReceivedBy,
		&f.WaivedBy, &f.WaiveReason, &f.CreatedAt, &f.UpdatedAt,
		&f.MemberCode, &f.TitleName,
	)
	if err == sql.ErrNoRows {
		return f, ErrNotFound
	}
	if err == nil {
		_ = json.Unmarshal(snap, &f.RateSnapshot)
	}
	return f, err
}

func (s *Store) PayFine(ctx context.Context, schoolID, id, receivedBy string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_fines
		SET status = 'paid', paid_at = now(), received_by = $1, updated_at = now()
		WHERE school_id = $2 AND id = $3 AND status IN ('open', 'billed')
	`, receivedBy, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) WaiveFine(ctx context.Context, schoolID, id, waivedBy, reason string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE lib_fines
		SET status = 'waived', waived_by = $1, waive_reason = $2, updated_at = now()
		WHERE school_id = $3 AND id = $4 AND status IN ('open', 'billed')
	`, waivedBy, reason, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateFineBilled(ctx context.Context, schoolID, id, billID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE lib_fines
		SET status = 'billed', bill_id = $1, updated_at = now()
		WHERE school_id = $2 AND id = $3
	`, billID, schoolID, id)
	return err
}

// --- Visits ---

func (s *Store) RecordVisit(ctx context.Context, schoolID, memberCode string, onDate time.Time) (*Visit, error) {
	var member Member
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM lib_members WHERE school_id = $1 AND member_code = $2
	`, schoolID, memberCode).Scan(&member.ID)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidMember
	}
	if err != nil {
		return nil, err
	}

	dateOnly := onDate.In(s.loc).Truncate(24 * time.Hour)
	var v Visit
	v.SchoolID = schoolID
	v.MemberID = member.ID
	v.VisitedOn = dateOnly
	v.VisitedAt = onDate

	err = s.db.QueryRowContext(ctx, `
		INSERT INTO lib_visits (school_id, member_id, visited_on, visited_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (member_id, visited_on) DO UPDATE SET visited_at = EXCLUDED.visited_at
		RETURNING id
	`, v.SchoolID, v.MemberID, v.VisitedOn, v.VisitedAt).Scan(&v.ID)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Store) ListVisits(ctx context.Context, schoolID string, from, to time.Time) ([]Visit, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT v.id, v.school_id, v.member_id, v.visited_on, v.visited_at, m.member_code
		FROM lib_visits v
		JOIN lib_members m ON m.id = v.member_id
		WHERE v.school_id = $1 AND v.visited_on >= $2 AND v.visited_on <= $3
		ORDER BY v.visited_at DESC
	`, schoolID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Visit
	for rows.Next() {
		var v Visit
		if err := rows.Scan(&v.ID, &v.SchoolID, &v.MemberID, &v.VisitedOn, &v.VisitedAt, &v.MemberCode); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, nil
}

// --- Reports ---

type OverdueReportItem struct {
	LoanID        string    `json:"loan_id"`
	MemberCode    string    `json:"member_code"`
	MemberKind    string    `json:"member_kind"`
	Title         string    `json:"title"`
	Barcode       string    `json:"barcode"`
	ClassroomName string    `json:"classroom_name"`
	DueDate       time.Time `json:"due_date"`
	DaysOverdue   int       `json:"days_overdue"`
	// EstFineIDR is the late fine if the copy were returned today, using the
	// fine rate active today for the member's category (general rate as
	// fallback); 0 when no rate is configured.
	EstFineIDR int64  `json:"est_fine_idr"`
	CategoryID string `json:"-"`
}

// OverdueLoans implements AC-115-01: list of members with overdue loans and days overdue.
func (s *Store) OverdueLoans(ctx context.Context, schoolID string) ([]OverdueReportItem, error) {
	now := time.Now().In(s.loc)
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, m.member_code, m.kind, m.category_id, t.title, cp.barcode,
		       COALESCE(l.classroom_name, ''), l.due_date,
		       ($2::date - l.due_date) AS days_overdue
		FROM lib_loans l
		JOIN lib_members m ON m.id = l.member_id
		JOIN lib_copies cp ON cp.id = l.copy_id
		JOIN lib_titles t ON t.id = cp.title_id
		WHERE l.school_id = $1 AND l.status = 'active' AND l.due_date < $2::date
		ORDER BY days_overdue DESC, m.member_code ASC
	`, schoolID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []OverdueReportItem
	for rows.Next() {
		var it OverdueReportItem
		if err := rows.Scan(
			&it.LoanID, &it.MemberCode, &it.MemberKind, &it.CategoryID, &it.Title, &it.Barcode,
			&it.ClassroomName, &it.DueDate, &it.DaysOverdue,
		); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(list) > 0 {
		rates, err := s.GetActiveRates(ctx, schoolID, now)
		if err != nil {
			return nil, err
		}
		for i := range list {
			catID := list[i].CategoryID
			if rate, ok := ResolveRate(rates, &catID, now); ok {
				list[i].EstFineIDR = LateFine(list[i].DaysOverdue, rate.LateFeePerDayIDR)
			}
		}
	}
	return list, nil
}

type PopularTitleItem struct {
	TitleID    string `json:"title_id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	Category   string `json:"category"`
	LoansCount int    `json:"loans_count"`
}

func (s *Store) PopularTitles(ctx context.Context, schoolID string, from, to time.Time, limit int) ([]PopularTitleItem, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.title, t.author, c.name, COUNT(l.id) AS loans_count
		FROM lib_loans l
		JOIN lib_copies cp ON cp.id = l.copy_id
		JOIN lib_titles t ON t.id = cp.title_id
		JOIN lib_collection_categories c ON c.id = t.category_id
		WHERE l.school_id = $1 AND l.loaned_at >= $2 AND l.loaned_at <= $3
		GROUP BY t.id, t.title, t.author, c.name
		ORDER BY loans_count DESC
		LIMIT $4
	`, schoolID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PopularTitleItem
	for rows.Next() {
		var it PopularTitleItem
		if err := rows.Scan(&it.TitleID, &it.Title, &it.Author, &it.Category, &it.LoansCount); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	return list, nil
}

type LoansReportItem struct {
	GroupKey   string `json:"group_key"`
	TotalLoans int    `json:"total_loans"`
	Returned   int    `json:"returned"`
	Active     int    `json:"active"`
	Lost       int    `json:"lost"`
}

func (s *Store) LoansReport(ctx context.Context, schoolID string, from, to time.Time, groupBy string) ([]LoansReportItem, error) {
	groupCol := "COALESCE(l.classroom_name, 'Staf / Lainnya')"
	if groupBy == "collection_category" {
		groupCol = "c.name"
	}

	query := fmt.Sprintf(`
		SELECT %s AS grp,
		       COUNT(l.id) AS total_loans,
		       COUNT(l.id) FILTER (WHERE l.status = 'returned') AS returned,
		       COUNT(l.id) FILTER (WHERE l.status = 'active') AS active,
		       COUNT(l.id) FILTER (WHERE l.status = 'lost') AS lost
		FROM lib_loans l
		JOIN lib_copies cp ON cp.id = l.copy_id
		JOIN lib_titles t ON t.id = cp.title_id
		JOIN lib_collection_categories c ON c.id = t.category_id
		WHERE l.school_id = $1 AND l.loaned_at >= $2 AND l.loaned_at <= $3
		GROUP BY grp
		ORDER BY total_loans DESC
	`, groupCol)

	rows, err := s.db.QueryContext(ctx, query, schoolID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []LoansReportItem
	for rows.Next() {
		var it LoansReportItem
		if err := rows.Scan(&it.GroupKey, &it.TotalLoans, &it.Returned, &it.Active, &it.Lost); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	return list, nil
}
