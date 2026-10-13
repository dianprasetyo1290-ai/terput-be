package asset

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	opsclient "skci/ops/internal/client"
	"skci/ops/internal/procurement"
)

type Store struct {
	db  *sql.DB
	loc *time.Location
}

func NewStore(db *sql.DB, loc *time.Location) *Store {
	return &Store{db: db, loc: loc}
}

// -------------------------------------------------------------------------
// Categories
// -------------------------------------------------------------------------

func (s *Store) ListCategories(ctx context.Context, schoolID string) ([]Category, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, code, name, useful_life_months, dep_start,
		       asset_acc_code, accum_dep_acc_code, dep_expense_acc_code,
		       created_at, updated_at
		FROM asset_categories
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY code ASC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(
			&c.ID, &c.SchoolID, &c.Code, &c.Name, &c.UsefulLifeMonths, &c.DepStart,
			&c.AssetAccCode, &c.AccumDepAccCode, &c.DepExpenseAccCode,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) GetCategory(ctx context.Context, schoolID, id string) (*Category, error) {
	var c Category
	err := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, code, name, useful_life_months, dep_start,
		       asset_acc_code, accum_dep_acc_code, dep_expense_acc_code,
		       created_at, updated_at
		FROM asset_categories
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id).Scan(
		&c.ID, &c.SchoolID, &c.Code, &c.Name, &c.UsefulLifeMonths, &c.DepStart,
		&c.AssetAccCode, &c.AccumDepAccCode, &c.DepExpenseAccCode,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (s *Store) CreateCategory(ctx context.Context, c Category) (*Category, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO asset_categories (
			school_id, code, name, useful_life_months, dep_start,
			asset_acc_code, accum_dep_acc_code, dep_expense_acc_code
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, c.SchoolID, c.Code, c.Name, c.UsefulLifeMonths, c.DepStart,
		c.AssetAccCode, c.AccumDepAccCode, c.DepExpenseAccCode,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) UpdateCategory(ctx context.Context, c Category) (*Category, error) {
	err := s.db.QueryRowContext(ctx, `
		UPDATE asset_categories
		SET code = $1, name = $2, useful_life_months = $3, dep_start = $4,
		    asset_acc_code = $5, accum_dep_acc_code = $6, dep_expense_acc_code = $7,
		    updated_at = now()
		WHERE school_id = $8 AND id = $9 AND deleted_at IS NULL
		RETURNING updated_at
	`, c.Code, c.Name, c.UsefulLifeMonths, c.DepStart,
		c.AssetAccCode, c.AccumDepAccCode, c.DepExpenseAccCode,
		c.SchoolID, c.ID,
	).Scan(&c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) DeleteCategory(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE asset_categories
		SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	cnt, _ := res.RowsAffected()
	if cnt == 0 {
		return errors.New("kategori tidak ditemukan")
	}
	return nil
}

// -------------------------------------------------------------------------
// Settings
// -------------------------------------------------------------------------

func (s *Store) GetSettings(ctx context.Context, schoolID string) (*Settings, error) {
	var st Settings
	err := s.db.QueryRowContext(ctx, `
		SELECT school_id, code_format, interunit_acc_code,
		       disposal_gain_acc_code, disposal_loss_acc_code, proceeds_acc_code,
		       created_at, updated_at
		FROM asset_settings
		WHERE school_id = $1
	`, schoolID).Scan(
		&st.SchoolID, &st.CodeFormat, &st.InterunitAccCode,
		&st.DisposalGainAccCode, &st.DisposalLossAccCode, &st.ProceedsAccCode,
		&st.CreatedAt, &st.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// default settings
			return &Settings{
				SchoolID:   schoolID,
				CodeFormat: "{cat}-{yyyy}-{seq:4}",
			}, nil
		}
		return nil, err
	}
	return &st, nil
}

func (s *Store) UpdateSettings(ctx context.Context, st Settings) (*Settings, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO asset_settings (
			school_id, code_format, interunit_acc_code,
			disposal_gain_acc_code, disposal_loss_acc_code, proceeds_acc_code,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, now(), now())
		ON CONFLICT (school_id) DO UPDATE SET
			code_format = EXCLUDED.code_format,
			interunit_acc_code = EXCLUDED.interunit_acc_code,
			disposal_gain_acc_code = EXCLUDED.disposal_gain_acc_code,
			disposal_loss_acc_code = EXCLUDED.disposal_loss_acc_code,
			proceeds_acc_code = EXCLUDED.proceeds_acc_code,
			updated_at = now()
		RETURNING created_at, updated_at
	`, st.SchoolID, st.CodeFormat, st.InterunitAccCode,
		st.DisposalGainAccCode, st.DisposalLossAccCode, st.ProceedsAccCode,
	).Scan(&st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// -------------------------------------------------------------------------
// Assets Listing & Details
// -------------------------------------------------------------------------

type AssetFilter struct {
	SchoolID        string
	Status          string
	CategoryID      string
	RoomID          string
	RoomIDs         []string
	CustodianUserID string
	Query           string
	Page            int
	PerPage         int
}

func (s *Store) ListAssets(ctx context.Context, f AssetFilter) ([]Asset, int, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PerPage <= 0 || f.PerPage > 200 {
		f.PerPage = 20
	}

	var where []string
	var args []any
	argIdx := 1

	if f.SchoolID != "" {
		where = append(where, fmt.Sprintf("a.school_id = $%d", argIdx))
		args = append(args, f.SchoolID)
		argIdx++
	}

	where = append(where, "a.deleted_at IS NULL")

	if f.Status != "" {
		where = append(where, fmt.Sprintf("a.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if f.CategoryID != "" {
		where = append(where, fmt.Sprintf("a.category_id = $%d", argIdx))
		args = append(args, f.CategoryID)
		argIdx++
	}

	if f.RoomID != "" {
		where = append(where, fmt.Sprintf("a.room_id = $%d", argIdx))
		args = append(args, f.RoomID)
		argIdx++
	}

	if len(f.RoomIDs) > 0 {
		where = append(where, fmt.Sprintf("a.room_id = ANY($%d::uuid[])", argIdx))
		args = append(args, "{"+strings.Join(f.RoomIDs, ",")+"}")
		argIdx++
	}

	if f.CustodianUserID != "" {
		where = append(where, fmt.Sprintf("a.custodian_user_id = $%d", argIdx))
		args = append(args, f.CustodianUserID)
		argIdx++
	}

	if f.Query != "" {
		q := "%" + strings.ToLower(f.Query) + "%"
		where = append(where, fmt.Sprintf("(LOWER(a.code) LIKE $%d OR LOWER(a.name) LIKE $%d)", argIdx, argIdx))
		args = append(args, q)
		argIdx++
	}

	whereClause := "WHERE " + strings.Join(where, " AND ")

	var total int
	countQuery := "SELECT count(*) FROM assets a " + whereClause
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.PerPage
	dataQuery := fmt.Sprintf(`
		SELECT a.id, a.school_id, a.code, a.name, a.category_id,
		       c.name AS category_name, c.code AS category_code,
		       a.status, a.condition, a.acquisition_cost_idr,
		       to_char(a.acquired_on, 'YYYY-MM-DD') AS acquired_on,
		       a.residual_idr, a.room_id, r.name AS room_name,
		       a.custodian_user_id, a.serial_no, a.photo_object_key,
		       a.qr_token, a.source, a.gr_line_id, a.unit_seq, a.po_id, a.vendor_id,
		       a.transferred_from_asset_id, a.offset_acc_code, a.activated_at, a.activated_by,
		       a.created_at, a.updated_at,
		       ev.status AS accounting_status,
		       bv.book_value_idr
		FROM assets a
		LEFT JOIN asset_categories c ON a.category_id = c.id
		LEFT JOIN rooms r ON a.room_id = r.id
		LEFT JOIN asset_book_values bv ON a.id = bv.asset_id
		LEFT JOIN LATERAL (
			SELECT status FROM asset_accounting_events
			WHERE asset_id = a.id
			ORDER BY created_at DESC LIMIT 1
		) ev ON true
		%s
		ORDER BY a.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	args = append(args, f.PerPage, offset)

	rows, err := s.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []Asset
	for rows.Next() {
		var a Asset
		var acquiredOn sql.NullString
		var categoryName, categoryCode, roomName sql.NullString
		var accStatus sql.NullString
		var bookVal sql.NullInt64

		if err := rows.Scan(
			&a.ID, &a.SchoolID, &a.Code, &a.Name, &a.CategoryID,
			&categoryName, &categoryCode,
			&a.Status, &a.Condition, &a.AcquisitionCostIDR,
			&acquiredOn,
			&a.ResidualIDR, &a.RoomID, &roomName,
			&a.CustodianUserID, &a.SerialNo, &a.PhotoObjectKey,
			&a.QRToken, &a.Source, &a.GRLineID, &a.UnitSeq, &a.POID, &a.VendorID,
			&a.TransferredFromAssetID, &a.OffsetAccCode, &a.ActivatedAt, &a.ActivatedBy,
			&a.CreatedAt, &a.UpdatedAt,
			&accStatus,
			&bookVal,
		); err != nil {
			return nil, 0, err
		}

		if acquiredOn.Valid {
			a.AcquiredOn = &acquiredOn.String
		}
		if categoryName.Valid {
			a.CategoryName = &categoryName.String
		}
		if categoryCode.Valid {
			a.CategoryCode = &categoryCode.String
		}
		if roomName.Valid {
			a.RoomName = &roomName.String
		}
		if accStatus.Valid {
			a.AccountingStatus = &accStatus.String
		}
		if bookVal.Valid {
			a.BookValueIDR = &bookVal.Int64
		}

		list = append(list, a)
	}

	return list, total, rows.Err()
}

func (s *Store) GetAsset(ctx context.Context, schoolID, id string) (*Asset, error) {
	var a Asset
	var acquiredOn, categoryName, categoryCode, roomName, accStatus sql.NullString
	var bookVal sql.NullInt64

	query := `
		SELECT a.id, a.school_id, a.code, a.name, a.category_id,
		       c.name AS category_name, c.code AS category_code,
		       a.status, a.condition, a.acquisition_cost_idr,
		       to_char(a.acquired_on, 'YYYY-MM-DD') AS acquired_on,
		       a.residual_idr, a.room_id, r.name AS room_name,
		       a.custodian_user_id, a.serial_no, a.photo_object_key,
		       a.qr_token, a.source, a.gr_line_id, a.unit_seq, a.po_id, a.vendor_id,
		       a.transferred_from_asset_id, a.offset_acc_code, a.activated_at, a.activated_by,
		       a.created_at, a.updated_at,
		       ev.status AS accounting_status,
		       bv.book_value_idr
		FROM assets a
		LEFT JOIN asset_categories c ON a.category_id = c.id
		LEFT JOIN rooms r ON a.room_id = r.id
		LEFT JOIN asset_book_values bv ON a.id = bv.asset_id
		LEFT JOIN LATERAL (
			SELECT status FROM asset_accounting_events
			WHERE asset_id = a.id
			ORDER BY created_at DESC LIMIT 1
		) ev ON true
		WHERE a.school_id = $1 AND a.id = $2 AND a.deleted_at IS NULL
	`

	err := s.db.QueryRowContext(ctx, query, schoolID, id).Scan(
		&a.ID, &a.SchoolID, &a.Code, &a.Name, &a.CategoryID,
		&categoryName, &categoryCode,
		&a.Status, &a.Condition, &a.AcquisitionCostIDR,
		&acquiredOn,
		&a.ResidualIDR, &a.RoomID, &roomName,
		&a.CustodianUserID, &a.SerialNo, &a.PhotoObjectKey,
		&a.QRToken, &a.Source, &a.GRLineID, &a.UnitSeq, &a.POID, &a.VendorID,
		&a.TransferredFromAssetID, &a.OffsetAccCode, &a.ActivatedAt, &a.ActivatedBy,
		&a.CreatedAt, &a.UpdatedAt,
		&accStatus,
		&bookVal,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if acquiredOn.Valid {
		a.AcquiredOn = &acquiredOn.String
	}
	if categoryName.Valid {
		a.CategoryName = &categoryName.String
	}
	if categoryCode.Valid {
		a.CategoryCode = &categoryCode.String
	}
	if roomName.Valid {
		a.RoomName = &roomName.String
	}
	if accStatus.Valid {
		a.AccountingStatus = &accStatus.String
	}
	if bookVal.Valid {
		a.BookValueIDR = &bookVal.Int64
	}

	return &a, nil
}

func (s *Store) GetAssetByToken(ctx context.Context, qrToken string) (*Asset, error) {
	var a Asset
	var acquiredOn, categoryName, categoryCode, roomName, accStatus sql.NullString
	var bookVal sql.NullInt64

	query := `
		SELECT a.id, a.school_id, a.code, a.name, a.category_id,
		       c.name AS category_name, c.code AS category_code,
		       a.status, a.condition, a.acquisition_cost_idr,
		       to_char(a.acquired_on, 'YYYY-MM-DD') AS acquired_on,
		       a.residual_idr, a.room_id, r.name AS room_name,
		       a.custodian_user_id, a.serial_no, a.photo_object_key,
		       a.qr_token, a.source, a.gr_line_id, a.unit_seq, a.po_id, a.vendor_id,
		       a.transferred_from_asset_id, a.offset_acc_code, a.activated_at, a.activated_by,
		       a.created_at, a.updated_at,
		       ev.status AS accounting_status,
		       bv.book_value_idr
		FROM assets a
		LEFT JOIN asset_categories c ON a.category_id = c.id
		LEFT JOIN rooms r ON a.room_id = r.id
		LEFT JOIN asset_book_values bv ON a.id = bv.asset_id
		LEFT JOIN LATERAL (
			SELECT status FROM asset_accounting_events
			WHERE asset_id = a.id
			ORDER BY created_at DESC LIMIT 1
		) ev ON true
		WHERE a.qr_token = $1 AND a.deleted_at IS NULL
	`

	err := s.db.QueryRowContext(ctx, query, qrToken).Scan(
		&a.ID, &a.SchoolID, &a.Code, &a.Name, &a.CategoryID,
		&categoryName, &categoryCode,
		&a.Status, &a.Condition, &a.AcquisitionCostIDR,
		&acquiredOn,
		&a.ResidualIDR, &a.RoomID, &roomName,
		&a.CustodianUserID, &a.SerialNo, &a.PhotoObjectKey,
		&a.QRToken, &a.Source, &a.GRLineID, &a.UnitSeq, &a.POID, &a.VendorID,
		&a.TransferredFromAssetID, &a.OffsetAccCode, &a.ActivatedAt, &a.ActivatedBy,
		&a.CreatedAt, &a.UpdatedAt,
		&accStatus,
		&bookVal,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if acquiredOn.Valid {
		a.AcquiredOn = &acquiredOn.String
	}
	if categoryName.Valid {
		a.CategoryName = &categoryName.String
	}
	if categoryCode.Valid {
		a.CategoryCode = &categoryCode.String
	}
	if roomName.Valid {
		a.RoomName = &roomName.String
	}
	if accStatus.Valid {
		a.AccountingStatus = &accStatus.String
	}
	if bookVal.Valid {
		a.BookValueIDR = &bookVal.Int64
	}

	return &a, nil
}

// -------------------------------------------------------------------------
// Asset Creation & Mutation
// -------------------------------------------------------------------------

func (s *Store) NextAssetCode(ctx context.Context, schoolID, catCode string, format string) (string, error) {
	year := time.Now().Year()

	// count assets for this school and year to get next sequence
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM assets
		WHERE school_id = $1 AND EXTRACT(YEAR FROM created_at) = $2
	`, schoolID, year).Scan(&count)
	if err != nil {
		return "", err
	}

	seq := count + 1
	return FormatAssetCode(format, catCode, year, seq), nil
}

func (s *Store) CreateManualAsset(ctx context.Context, a Asset) (*Asset, error) {
	if a.QRToken == "" {
		a.QRToken = GenerateQRToken()
	}
	a.Status = "candidate"
	a.Source = "manual"
	if a.Code == "" {
		a.Code = "TEMP-" + GenerateQRToken()[:8]
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO assets (
			school_id, code, name, category_id, status, condition,
			acquisition_cost_idr, acquired_on, residual_idr,
			room_id, custodian_user_id, serial_no, qr_token, source,
			offset_acc_code, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now(), now()
		)
		RETURNING id, created_at, updated_at
	`, a.SchoolID, a.Code, a.Name, a.CategoryID, a.Status, a.Condition,
		a.AcquisitionCostIDR, a.AcquiredOn, a.ResidualIDR,
		a.RoomID, a.CustodianUserID, a.SerialNo, a.QRToken, a.Source,
		a.OffsetAccCode,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) UpdateAssetPhysical(ctx context.Context, a Asset) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE assets
		SET name = COALESCE(NULLIF($1, ''), name),
		    category_id = COALESCE($2, category_id),
		    condition = COALESCE($3, condition),
		    room_id = COALESCE($4, room_id),
		    custodian_user_id = COALESCE($5, custodian_user_id),
		    serial_no = COALESCE($6, serial_no),
		    offset_acc_code = COALESCE($7, offset_acc_code),
		    residual_idr = $8,
		    updated_at = now()
		WHERE school_id = $9 AND id = $10 AND deleted_at IS NULL
	`, a.Name, a.CategoryID, a.Condition, a.RoomID, a.CustodianUserID,
		a.SerialNo, a.OffsetAccCode, a.ResidualIDR,
		a.SchoolID, a.ID,
	)
	return err
}

func (s *Store) UpdatePhoto(ctx context.Context, schoolID, id, photoKey string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE assets
		SET photo_object_key = $1, updated_at = now()
		WHERE school_id = $2 AND id = $3 AND deleted_at IS NULL
	`, photoKey, schoolID, id)
	return err
}

// ActivateAsset validates and activates a candidate asset, creating an accounting event
func (s *Store) ActivateAsset(ctx context.Context, schoolID, id, actorUserID string) (*Asset, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Fetch asset with FOR UPDATE
	var a Asset
	var acquiredOn, offsetAcc sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT id, school_id, code, name, category_id, status, condition,
		       acquisition_cost_idr, to_char(acquired_on, 'YYYY-MM-DD'), residual_idr,
		       room_id, custodian_user_id, serial_no, qr_token, source,
		       offset_acc_code
		FROM assets
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, schoolID, id).Scan(
		&a.ID, &a.SchoolID, &a.Code, &a.Name, &a.CategoryID, &a.Status, &a.Condition,
		&a.AcquisitionCostIDR, &acquiredOn, &a.ResidualIDR,
		&a.RoomID, &a.CustodianUserID, &a.SerialNo, &a.QRToken, &a.Source,
		&offsetAcc,
	)
	if err != nil {
		return nil, err
	}
	if acquiredOn.Valid {
		a.AcquiredOn = &acquiredOn.String
	}
	if offsetAcc.Valid {
		a.OffsetAccCode = &offsetAcc.String
	}

	if a.Status == "active" {
		return &a, nil // already active
	}
	if a.Status != "candidate" {
		return nil, fmt.Errorf("hanya aset kandidat yang dapat diaktifkan, status saat ini: %s", a.Status)
	}

	// 2. Fetch category
	var cat *Category
	if a.CategoryID != nil {
		var c Category
		err := tx.QueryRowContext(ctx, `
			SELECT id, school_id, code, name, useful_life_months, dep_start,
			       asset_acc_code, accum_dep_acc_code, dep_expense_acc_code
			FROM asset_categories
			WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
		`, schoolID, *a.CategoryID).Scan(
			&c.ID, &c.SchoolID, &c.Code, &c.Name, &c.UsefulLifeMonths, &c.DepStart,
			&c.AssetAccCode, &c.AccumDepAccCode, &c.DepExpenseAccCode,
		)
		if err == nil {
			cat = &c
		}
	}

	// 3. Check CanActivate
	missing := CanActivate(a, cat)
	if len(missing) > 0 {
		return nil, fmt.Errorf("tidak dapat mengaktifkan aset: %s", strings.Join(missing, ", "))
	}

	// 4. Fetch settings for code generation
	var st Settings
	err = tx.QueryRowContext(ctx, `
		SELECT code_format, interunit_acc_code, disposal_gain_acc_code, disposal_loss_acc_code, proceeds_acc_code
		FROM asset_settings WHERE school_id = $1
	`, schoolID).Scan(
		&st.CodeFormat, &st.InterunitAccCode, &st.DisposalGainAccCode, &st.DisposalLossAccCode, &st.ProceedsAccCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		st.CodeFormat = "{cat}-{yyyy}-{seq:4}"
	}

	year := time.Now().Year()
	var count int
	_ = tx.QueryRowContext(ctx, `
		SELECT count(*) FROM assets WHERE school_id = $1 AND status = 'active' AND EXTRACT(YEAR FROM activated_at) = $2
	`, schoolID, year).Scan(&count)

	newCode := FormatAssetCode(st.CodeFormat, cat.Code, year, count+1)
	now := time.Now()

	// 5. Update asset to active
	_, err = tx.ExecContext(ctx, `
		UPDATE assets
		SET status = 'active', code = $1, activated_at = $2, activated_by = $3, updated_at = now()
		WHERE id = $4
	`, newCode, now, actorUserID, a.ID)
	if err != nil {
		return nil, err
	}

	a.Status = "active"
	a.Code = newCode
	a.ActivatedAt = &now
	a.ActivatedBy = &actorUserID

	// 6. Insert Accounting Event (acquisition)
	payload := AcquisitionPayload(a, *cat, st)
	payloadJSON, _ := json.Marshal(payload)
	ref := fmt.Sprintf("asset:acq:%s", a.ID)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO asset_accounting_events (
			school_id, asset_id, kind, reference, payload, status, attempts, created_at, updated_at
		) VALUES ($1, $2, 'acquisition', $3, $4, 'pending', 0, now(), now())
		ON CONFLICT (reference) DO NOTHING
	`, schoolID, a.ID, ref, payloadJSON)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &a, nil
}

// -------------------------------------------------------------------------
// Procurement Hook & Backfill
// -------------------------------------------------------------------------

func (s *Store) OnGoodsReceived(ctx context.Context, tx *sql.Tx, ev procurement.GoodsReceivedEvent) error {
	candidates := CandidatesFromReceipt(ev)
	if len(candidates) == 0 {
		return nil
	}

	// Look up asset_category_id from item_categories for lines
	for _, c := range candidates {
		var catID sql.NullString
		if c.GRLineID != nil {
			_ = tx.QueryRowContext(ctx, `
				SELECT ic.asset_category_id
				FROM goods_receipt_lines grl
				JOIN purchase_order_lines pol ON grl.po_line_id = pol.id
				JOIN items it ON pol.item_id = it.id
				JOIN item_categories ic ON it.category_id = ic.id
				WHERE grl.id = $1
			`, *c.GRLineID).Scan(&catID)
		}

		var categoryIDVal *string
		if catID.Valid && catID.String != "" {
			categoryIDVal = &catID.String
		}

		tempCode := fmt.Sprintf("GR-%s-%d", ev.GRNumber, *c.UnitSeq)

		_, err := tx.ExecContext(ctx, `
			INSERT INTO assets (
				school_id, code, name, category_id, status, acquisition_cost_idr,
				acquired_on, residual_idr, qr_token, source, gr_line_id, unit_seq,
				po_id, vendor_id, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, 'candidate', $5, $6, $7, $8, 'goods_receipt', $9, $10,
				$11, $12, now(), now()
			)
			ON CONFLICT (gr_line_id, unit_seq) WHERE gr_line_id IS NOT NULL DO NOTHING
		`, c.SchoolID, tempCode, c.Name, categoryIDVal, c.AcquisitionCostIDR,
			c.AcquiredOn, c.ResidualIDR, c.QRToken, c.GRLineID, c.UnitSeq,
			c.POID, c.VendorID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) BackfillFromReceipts(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT grl.id, grl.school_id, gr.id, gr.number, pol.id, po.id, po.number,
		       po.vendor_id, it.id, it.name, ic.asset_category_id,
		       grl.qty_accepted, pol.unit_price_idr, gr.received_on
		FROM goods_receipt_lines grl
		JOIN goods_receipts gr ON grl.gr_id = gr.id
		JOIN purchase_order_lines pol ON grl.po_line_id = pol.id
		JOIN purchase_orders po ON gr.po_id = po.id
		JOIN items it ON pol.item_id = it.id
		JOIN item_categories ic ON it.category_id = ic.id
		WHERE ic.tracking = 'asset' AND grl.asset_handoff_at IS NULL
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type handoffRow struct {
		grLineID       string
		schoolID       string
		grID           string
		grNumber       string
		poLineID       string
		poID           string
		poNumber       string
		vendorID       string
		itemID         string
		itemName       string
		catID          sql.NullString
		qtyAccepted    int64
		unitPrice      int64
		receivedOn     time.Time
	}

	var rowsToProcess []handoffRow
	for rows.Next() {
		var r handoffRow
		if err := rows.Scan(
			&r.grLineID, &r.schoolID, &r.grID, &r.grNumber, &r.poLineID, &r.poID, &r.poNumber,
			&r.vendorID, &r.itemID, &r.itemName, &r.catID,
			&r.qtyAccepted, &r.unitPrice, &r.receivedOn,
		); err != nil {
			return 0, err
		}
		rowsToProcess = append(rowsToProcess, r)
	}

	count := 0
	for _, r := range rowsToProcess {
		ev := procurement.GoodsReceivedEvent{
			SchoolID:       r.schoolID,
			GoodsReceiptID: r.grID,
			GRNumber:       r.grNumber,
			Lines: []procurement.GoodsReceivedLine{
				{
					GRLineID:     r.grLineID,
					POLineID:     r.poLineID,
					POID:         r.poID,
					PONumber:     r.poNumber,
					VendorID:     r.vendorID,
					ItemID:       r.itemID,
					ItemName:     r.itemName,
					QtyAccepted:  r.qtyAccepted,
					UnitPriceIDR: r.unitPrice,
					ReceivedOn:   r.receivedOn,
				},
			},
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			continue
		}

		if err := s.OnGoodsReceived(ctx, tx, ev); err != nil {
			tx.Rollback()
			continue
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE goods_receipt_lines SET asset_handoff_at = now() WHERE id = $1
		`, r.grLineID)
		if err != nil {
			tx.Rollback()
			continue
		}

		if err := tx.Commit(); err == nil {
			count += int(r.qtyAccepted)
		}
	}

	return count, nil
}

// -------------------------------------------------------------------------
// Movements & Disposals
// -------------------------------------------------------------------------

func (s *Store) CreateMovement(ctx context.Context, m Movement, current Asset, coreClient *opsclient.Client) (*Movement, error) {
	beforeState := MovementState{
		RoomID:          "",
		CustodianUserID: "",
		Condition:       "",
		SchoolID:        current.SchoolID,
	}
	if current.RoomID != nil {
		beforeState.RoomID = *current.RoomID
	}
	if current.CustodianUserID != nil {
		beforeState.CustodianUserID = *current.CustodianUserID
	}
	if current.Condition != nil {
		beforeState.Condition = *current.Condition
	}

	toRoom, _ := m.ToValue["room_id"].(string)
	toCustodian, _ := m.ToValue["custodian_user_id"].(string)
	toCondition, _ := m.ToValue["condition"].(string)
	toSchool := current.SchoolID
	if m.ToSchoolID != nil && *m.ToSchoolID != "" {
		toSchool = *m.ToSchoolID
	}

	afterState := MovementState{
		RoomID:          toRoom,
		CustodianUserID: toCustodian,
		Condition:       toCondition,
		SchoolID:        toSchool,
	}

	kinds := MovementKind(beforeState, afterState)
	if len(kinds) == 0 {
		return nil, errors.New("tidak ada perubahan data fisik atau unit pada aset")
	}

	m.Kind = kinds[0]
	fromJSON, _ := json.Marshal(m.FromValue)
	toJSON, _ := json.Marshal(m.ToValue)

	if slices.Contains(kinds, "unit") {
		// Unit transfer requires approval flow
		m.Status = "pending_approval"
		var id string
		err := s.db.QueryRowContext(ctx, `
			INSERT INTO asset_movements (
				school_id, asset_id, kind, from_value, to_value, effective_date,
				reason, status, to_school_id, created_by, created_at
			) VALUES ($1, $2, 'unit', $3, $4, $5, $6, 'pending_approval', $7, $8, now())
			RETURNING id, created_at
		`, m.SchoolID, m.AssetID, fromJSON, toJSON, m.EffectiveDate, m.Reason, m.ToSchoolID, m.CreatedBy).Scan(&id, &m.CreatedAt)
		if err != nil {
			return nil, err
		}
		m.ID = id

		// Request core approval
		if coreClient != nil {
			appReq := opsclient.ApprovalRequest{
				SchoolID:       m.SchoolID,
				RequestType:    "asset_transfer",
				SourceService:  "ops",
				SourceID:       m.ID,
				Title:          fmt.Sprintf("Mutasi Aset Antar-Unit: %s (%s)", current.Name, current.Code),
				MakerUserID:    m.CreatedBy,
				Summary:        map[string]any{"asset_code": current.Code, "to_school_id": m.ToSchoolID, "reason": m.Reason},
				IdempotencyKey: fmt.Sprintf("asset:xfer:%s", m.ID),
			}
			appID, err := coreClient.CreateApproval(ctx, appReq)
			if err == nil && appID != "" {
				m.ApprovalID = &appID
				_, _ = s.db.ExecContext(ctx, `UPDATE asset_movements SET approval_id = $1 WHERE id = $2`, appID, m.ID)
			}
		}
		return &m, nil
	}

	// Normal location/custodian/condition movement is done immediately
	m.Status = "done"
	var id string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO asset_movements (
			school_id, asset_id, kind, from_value, to_value, effective_date,
			reason, status, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'done', $8, now())
		RETURNING id, created_at
	`, m.SchoolID, m.AssetID, m.Kind, fromJSON, toJSON, m.EffectiveDate, m.Reason, m.CreatedBy).Scan(&id, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	m.ID = id

	// Update asset columns
	var setClauses []string
	var args []any
	argIdx := 1

	if toRoom != "" {
		setClauses = append(setClauses, fmt.Sprintf("room_id = $%d", argIdx))
		args = append(args, toRoom)
		argIdx++
	}
	if toCustodian != "" {
		setClauses = append(setClauses, fmt.Sprintf("custodian_user_id = $%d", argIdx))
		args = append(args, toCustodian)
		argIdx++
	}
	if toCondition != "" {
		setClauses = append(setClauses, fmt.Sprintf("condition = $%d", argIdx))
		args = append(args, toCondition)
		argIdx++
	}

	if len(setClauses) > 0 {
		setClauses = append(setClauses, "updated_at = now()")
		q := fmt.Sprintf("UPDATE assets SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
		args = append(args, m.AssetID)
		_, _ = s.db.ExecContext(ctx, q, args...)
	}

	return &m, nil
}

func (s *Store) ListMovements(ctx context.Context, schoolID, assetID string) ([]Movement, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, asset_id, kind, from_value, to_value,
		       to_char(effective_date, 'YYYY-MM-DD'), reason, status, approval_id, to_school_id,
		       created_by, created_at
		FROM asset_movements
		WHERE school_id = $1 AND asset_id = $2
		ORDER BY created_at DESC
	`, schoolID, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Movement
	for rows.Next() {
		var m Movement
		var fromJSON, toJSON []byte
		if err := rows.Scan(
			&m.ID, &m.SchoolID, &m.AssetID, &m.Kind, &fromJSON, &toJSON,
			&m.EffectiveDate, &m.Reason, &m.Status, &m.ApprovalID, &m.ToSchoolID,
			&m.CreatedBy, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(fromJSON, &m.FromValue)
		_ = json.Unmarshal(toJSON, &m.ToValue)
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *Store) CreateDisposal(ctx context.Context, d Disposal, current Asset, coreClient *opsclient.Client) (*Disposal, error) {
	var openCount int
	_ = s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM asset_disposals WHERE asset_id = $1 AND status = 'pending_approval'
	`, d.AssetID).Scan(&openCount)

	if err := CanDispose(current, openCount > 0); err != nil {
		return nil, err
	}

	var id string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO asset_disposals (
			school_id, asset_id, kind, disposed_on, proceeds_idr, reason,
			status, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'pending_approval', $7, now())
		RETURNING id, created_at
	`, d.SchoolID, d.AssetID, d.Kind, d.DisposedOn, d.ProceedsIDR, d.Reason, d.CreatedBy).Scan(&id, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	d.ID = id
	d.Status = "pending_approval"

	// Request core approval
	if coreClient != nil {
		amount := current.AcquisitionCostIDR
		if current.BookValueIDR != nil && *current.BookValueIDR > 0 {
			amount = *current.BookValueIDR
		}
		appReq := opsclient.ApprovalRequest{
			SchoolID:       d.SchoolID,
			RequestType:    "asset_disposal",
			SourceService:  "ops",
			SourceID:       d.ID,
			Title:          fmt.Sprintf("Pelepasan Aset: %s (%s) - %s", current.Name, current.Code, d.Kind),
			MakerUserID:    d.CreatedBy,
			Summary:        map[string]any{"asset_code": current.Code, "kind": d.Kind, "reason": d.Reason, "amount_idr": amount},
			IdempotencyKey: fmt.Sprintf("asset:disp:%s", d.ID),
		}
		appID, err := coreClient.CreateApproval(ctx, appReq)
		if err == nil && appID != "" {
			d.ApprovalID = &appID
			_, _ = s.db.ExecContext(ctx, `UPDATE asset_disposals SET approval_id = $1 WHERE id = $2`, appID, d.ID)
		}
	}

	return &d, nil
}

func (s *Store) CancelDisposal(ctx context.Context, schoolID, id, actorUserID string, coreClient *opsclient.Client) error {
	var approvalID sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT approval_id FROM asset_disposals
		WHERE school_id = $1 AND id = $2 AND status = 'pending_approval'
	`, schoolID, id).Scan(&approvalID)
	if err != nil {
		return errors.New("pengajuan pelepasan tidak ditemukan atau tidak dalam status pending")
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE asset_disposals SET status = 'cancelled' WHERE id = $1
	`, id)
	if err != nil {
		return err
	}

	if approvalID.Valid && approvalID.String != "" && coreClient != nil {
		_ = coreClient.CancelApproval(ctx, approvalID.String, actorUserID, "Dibatalkan oleh pembuat")
	}

	return nil
}

func (s *Store) ListDisposals(ctx context.Context, schoolID, status string) ([]Disposal, error) {
	var where []string
	var args []any
	argIdx := 1

	where = append(where, fmt.Sprintf("d.school_id = $%d", argIdx))
	args = append(args, schoolID)
	argIdx++

	if status != "" {
		where = append(where, fmt.Sprintf("d.status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT d.id, d.school_id, d.asset_id, a.name AS asset_name, a.code AS asset_code,
		       d.kind, to_char(d.disposed_on, 'YYYY-MM-DD'), d.proceeds_idr,
		       d.reason, d.status, d.approval_id, d.created_by, d.created_at
		FROM asset_disposals d
		JOIN assets a ON d.asset_id = a.id
		WHERE %s
		ORDER BY d.created_at DESC
	`, strings.Join(where, " AND "))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Disposal
	for rows.Next() {
		var d Disposal
		var assetName, assetCode sql.NullString
		if err := rows.Scan(
			&d.ID, &d.SchoolID, &d.AssetID, &assetName, &assetCode,
			&d.Kind, &d.DisposedOn, &d.ProceedsIDR, &d.Reason,
			&d.Status, &d.ApprovalID, &d.CreatedBy, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		if assetName.Valid {
			d.AssetName = &assetName.String
		}
		if assetCode.Valid {
			d.AssetCode = &assetCode.String
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// -------------------------------------------------------------------------
// Room Summary for 3D Campus (T25)
// -------------------------------------------------------------------------

func (s *Store) RoomSummary(ctx context.Context, schoolID string, roomIDs []string) ([]RoomSummary, error) {
	if len(roomIDs) == 0 {
		return []RoomSummary{}, nil
	}

	roomArray := "{" + strings.Join(roomIDs, ",") + "}"

	// 1. Fetch active assets grouped by room & condition
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.room_id,
		       COALESCE(a.condition, 'tanpa_kondisi') AS cond,
		       count(*) AS cnt
		FROM assets a
		WHERE a.school_id = $1 AND a.status = 'active' AND a.room_id = ANY($2::uuid[]) AND a.deleted_at IS NULL
		GROUP BY a.room_id, cond
	`, schoolID, roomArray)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaryMap := make(map[string]*RoomSummary)
	for _, rid := range roomIDs {
		summaryMap[rid] = &RoomSummary{
			RoomID:          rid,
			Total:           0,
			ByCondition:     map[string]int{"baik": 0, "rusak_ringan": 0, "rusak_berat": 0, "tanpa_kondisi": 0},
			ByCategory:      []CategorySummary{},
			PendingDisposal: 0,
		}
	}

	for rows.Next() {
		var roomID, cond string
		var cnt int
		if err := rows.Scan(&roomID, &cond, &cnt); err == nil {
			sm, ok := summaryMap[roomID]
			if ok {
				sm.Total += cnt
				sm.ByCondition[cond] += cnt
			}
		}
	}

	// 2. Fetch categories breakdown per room
	catRows, err := s.db.QueryContext(ctx, `
		SELECT a.room_id, c.id, c.name, count(*) AS cnt
		FROM assets a
		JOIN asset_categories c ON a.category_id = c.id
		WHERE a.school_id = $1 AND a.status = 'active' AND a.room_id = ANY($2::uuid[]) AND a.deleted_at IS NULL
		GROUP BY a.room_id, c.id, c.name
	`, schoolID, roomArray)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var roomID, catID, catName string
			var cnt int
			if err := catRows.Scan(&roomID, &catID, &catName, &cnt); err == nil {
				if sm, ok := summaryMap[roomID]; ok {
					sm.ByCategory = append(sm.ByCategory, CategorySummary{
						CategoryID: catID,
						Name:       catName,
						Count:      cnt,
					})
				}
			}
		}
	}

	// 3. Fetch pending disposals count
	dispRows, err := s.db.QueryContext(ctx, `
		SELECT a.room_id, count(*) AS cnt
		FROM asset_disposals d
		JOIN assets a ON d.asset_id = a.id
		WHERE d.school_id = $1 AND d.status = 'pending_approval' AND a.room_id = ANY($2::uuid[])
		GROUP BY a.room_id
	`, schoolID, roomArray)
	if err == nil {
		defer dispRows.Close()
		for dispRows.Next() {
			var roomID string
			var cnt int
			if err := dispRows.Scan(&roomID, &cnt); err == nil {
				if sm, ok := summaryMap[roomID]; ok {
					sm.PendingDisposal = cnt
				}
			}
		}
	}

	var res []RoomSummary
	for _, rid := range roomIDs {
		res = append(res, *summaryMap[rid])
	}
	return res, nil
}

// -------------------------------------------------------------------------
// Accounting Events Outbox & Callbacks
// -------------------------------------------------------------------------

func (s *Store) DispatchPendingEvents(ctx context.Context, ac AccountingClient, coreClient *opsclient.Client) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, asset_id, kind, reference, payload, attempts
		FROM asset_accounting_events
		WHERE status = 'pending' AND attempts < 5
		ORDER BY created_at ASC
		LIMIT 20
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type eventRow struct {
		id        string
		schoolID  string
		assetID   string
		kind      string
		reference string
		payload   map[string]any
		attempts  int
	}

	var toDispatch []eventRow
	for rows.Next() {
		var r eventRow
		var rawPayload []byte
		if err := rows.Scan(&r.id, &r.schoolID, &r.assetID, &r.kind, &r.reference, &rawPayload, &r.attempts); err != nil {
			continue
		}
		_ = json.Unmarshal(rawPayload, &r.payload)
		toDispatch = append(toDispatch, r)
	}

	for _, ev := range toDispatch {
		var transNos []string
		var err error

		switch ev.kind {
		case "acquisition":
			var t string
			t, err = ac.RegisterAsset(ctx, ev.payload)
			if t != "" {
				transNos = append(transNos, t)
			}
		case "transfer":
			var tOut, tIn string
			tOut, tIn, err = ac.TransferAsset(ctx, ev.assetID, ev.payload)
			if tOut != "" {
				transNos = append(transNos, tOut)
			}
			if tIn != "" {
				transNos = append(transNos, tIn)
			}
		case "disposal":
			var t string
			var bookVal int64
			t, bookVal, err = ac.DisposeAsset(ctx, ev.assetID, ev.payload)
			if t != "" {
				transNos = append(transNos, t)
			}
			if bookVal >= 0 {
				// update cached book value
				_, _ = s.db.ExecContext(ctx, `
					UPDATE asset_book_values SET book_value_idr = 0, as_of = now() WHERE asset_id = $1
				`, ev.assetID)
			}
		}

		if err == nil {
			transNosArray := "{" + strings.Join(transNos, ",") + "}"
			_, _ = s.db.ExecContext(ctx, `
				UPDATE asset_accounting_events
				SET status = 'posted', trans_nos = $1::text[], posted_at = now(), updated_at = now()
				WHERE id = $2
			`, transNosArray, ev.id)
		} else {
			errMsg := err.Error()
			newAttempts := ev.attempts + 1
			newStatus := "pending"
			if newAttempts >= 5 {
				newStatus = "failed"
				if coreClient != nil {
					_ = coreClient.PostException(ctx, opsclient.Exception{
						SchoolID:       ev.schoolID,
						SourceService:  "ops",
						Kind:           "journal_posting",
						Severity:       "normal",
						ErrorMessage:   fmt.Sprintf("Gagal sinkronisasi event aset %s (%s): %s", ev.kind, ev.reference, errMsg),
						MaxAttempts:    5,
						IdempotencyKey: fmt.Sprintf("ex:%s", ev.reference),
						PayloadSummary: map[string]any{"event_id": ev.id, "asset_id": ev.assetID, "reference": ev.reference},
					})
				}
			}

			_, _ = s.db.ExecContext(ctx, `
				UPDATE asset_accounting_events
				SET status = $1, attempts = $2, error = $3, updated_at = now()
				WHERE id = $4
			`, newStatus, newAttempts, errMsg, ev.id)
		}
	}

	return nil
}

func (s *Store) RetryEvent(ctx context.Context, eventID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE asset_accounting_events
		SET status = 'pending', attempts = 0, error = NULL, updated_at = now()
		WHERE id = $1
	`, eventID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("event tidak ditemukan")
	}
	return nil
}

// HandleApprovalCallback processes callbacks from core approval service
func (s *Store) HandleApprovalCallback(ctx context.Context, requestType, sourceID, status string) error {
	switch requestType {
	case "asset_disposal":
		var d Disposal
		var a Asset
		err := s.db.QueryRowContext(ctx, `
			SELECT d.id, d.school_id, d.asset_id, d.kind, to_char(d.disposed_on, 'YYYY-MM-DD'), d.proceeds_idr, d.status,
			       a.id, a.code, a.name, a.status
			FROM asset_disposals d
			JOIN assets a ON d.asset_id = a.id
			WHERE d.id = $1
		`, sourceID).Scan(
			&d.ID, &d.SchoolID, &d.AssetID, &d.Kind, &d.DisposedOn, &d.ProceedsIDR, &d.Status,
			&a.ID, &a.Code, &a.Name, &a.Status,
		)
		if err != nil {
			return err
		}

		if status == "approved" {
			_, err = s.db.ExecContext(ctx, `
				UPDATE asset_disposals SET status = 'approved' WHERE id = $1
			`, d.ID)
			if err != nil {
				return err
			}

			updatedAsset, action := ApplyDisposalDecision(a, "approved")
			if action == "disposal" {
				_, err = s.db.ExecContext(ctx, `
					UPDATE assets SET status = 'disposed', updated_at = now() WHERE id = $1
				`, updatedAsset.ID)
				if err != nil {
					return err
				}

				// Create disposal accounting event
				st, _ := s.GetSettings(ctx, d.SchoolID)
				gainAcc := ""
				lossAcc := ""
				procAcc := ""
				if st != nil {
					if st.DisposalGainAccCode != nil {
						gainAcc = *st.DisposalGainAccCode
					}
					if st.DisposalLossAccCode != nil {
						lossAcc = *st.DisposalLossAccCode
					}
					if st.ProceedsAccCode != nil {
						procAcc = *st.ProceedsAccCode
					}
				}

				payload := map[string]any{
					"school_id":         d.SchoolID,
					"disposed_on":       d.DisposedOn,
					"kind":              d.Kind,
					"proceeds_idr":      d.ProceedsIDR,
					"proceeds_acc_code": procAcc,
					"gain_acc_code":     gainAcc,
					"loss_acc_code":     lossAcc,
					"reference":         fmt.Sprintf("asset:disp:%s", d.ID),
				}
				pJSON, _ := json.Marshal(payload)

				_, _ = s.db.ExecContext(ctx, `
					INSERT INTO asset_accounting_events (
						school_id, asset_id, kind, reference, payload, status, attempts, created_at, updated_at
					) VALUES ($1, $2, 'disposal', $3, $4, 'pending', 0, now(), now())
					ON CONFLICT (reference) DO NOTHING
				`, d.SchoolID, d.AssetID, fmt.Sprintf("asset:disp:%s", d.ID), pJSON)
			}
		} else if status == "rejected" {
			_, _ = s.db.ExecContext(ctx, `UPDATE asset_disposals SET status = 'rejected' WHERE id = $1`, d.ID)
		}

	case "asset_transfer":
		var m Movement
		var a Asset
		err := s.db.QueryRowContext(ctx, `
			SELECT m.id, m.school_id, m.asset_id, to_char(m.effective_date, 'YYYY-MM-DD'), m.to_school_id, m.status,
			       a.id, a.code, a.name, a.category_id, a.condition, a.acquisition_cost_idr, a.residual_idr
			FROM asset_movements m
			JOIN assets a ON m.asset_id = a.id
			WHERE m.id = $1
		`, sourceID).Scan(
			&m.ID, &m.SchoolID, &m.AssetID, &m.EffectiveDate, &m.ToSchoolID, &m.Status,
			&a.ID, &a.Code, &a.Name, &a.CategoryID, &a.Condition, &a.AcquisitionCostIDR, &a.ResidualIDR,
		)
		if err != nil {
			return err
		}

		if status == "approved" && m.ToSchoolID != nil {
			_, _ = s.db.ExecContext(ctx, `UPDATE asset_movements SET status = 'approved' WHERE id = $1`, m.ID)
			_, _ = s.db.ExecContext(ctx, `UPDATE assets SET status = 'transferred_out', updated_at = now() WHERE id = $1`, a.ID)

			// Create cloned asset in new unit
			newCode := fmt.Sprintf("%s-XFER", a.Code)
			newQR := GenerateQRToken()
			var newAssetID string
			err = s.db.QueryRowContext(ctx, `
				INSERT INTO assets (
					school_id, code, name, category_id, status, condition,
					acquisition_cost_idr, acquired_on, residual_idr,
					qr_token, source, transferred_from_asset_id, activated_at, created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, 'active', $5, $6, $7, $8, $9, 'interunit_transfer', $10, now(), now(), now()
				)
				RETURNING id
			`, *m.ToSchoolID, newCode, a.Name, a.CategoryID, a.Condition,
				a.AcquisitionCostIDR, m.EffectiveDate, a.ResidualIDR,
				newQR, a.ID,
			).Scan(&newAssetID)

			if err == nil {
				stFrom, _ := s.GetSettings(ctx, m.SchoolID)
				stTo, _ := s.GetSettings(ctx, *m.ToSchoolID)
				fromAcc := ""
				toAcc := ""
				if stFrom != nil && stFrom.InterunitAccCode != nil {
					fromAcc = *stFrom.InterunitAccCode
				}
				if stTo != nil && stTo.InterunitAccCode != nil {
					toAcc = *stTo.InterunitAccCode
				}

				payload := map[string]any{
					"school_id":               m.SchoolID,
					"to_school_id":            *m.ToSchoolID,
					"to_asset_ref":            newAssetID,
					"effective_date":          m.EffectiveDate,
					"interunit_acc_code_from": fromAcc,
					"interunit_acc_code_to":   toAcc,
					"reference":               fmt.Sprintf("asset:xfer:%s", m.ID),
				}
				pJSON, _ := json.Marshal(payload)

				_, _ = s.db.ExecContext(ctx, `
					INSERT INTO asset_accounting_events (
						school_id, asset_id, kind, reference, payload, status, attempts, created_at, updated_at
					) VALUES ($1, $2, 'transfer', $3, $4, 'pending', 0, now(), now())
					ON CONFLICT (reference) DO NOTHING
				`, m.SchoolID, a.ID, fmt.Sprintf("asset:xfer:%s", m.ID), pJSON)
			}
		} else if status == "rejected" {
			_, _ = s.db.ExecContext(ctx, `UPDATE asset_movements SET status = 'rejected' WHERE id = $1`, m.ID)
		}
	}

	return nil
}

// -------------------------------------------------------------------------
// Depreciation Runs & Book Values
// -------------------------------------------------------------------------

func (s *Store) RunMonthlyDepreciation(ctx context.Context, schoolID, period string, ac AccountingClient) (*DepreciationRun, error) {
	run := DepreciationRun{
		SchoolID: schoolID,
		Period:   period,
		Status:   "pending",
	}

	_, _, _, err := ac.Depreciate(ctx, schoolID, period)
	if err != nil {
		errMsg := err.Error()
		run.Status = "failed"
		run.Error = &errMsg
		_, _ = s.db.ExecContext(ctx, `
			INSERT INTO asset_depreciation_runs (school_id, period, status, error, created_at)
			VALUES ($1, $2, 'failed', $3, now())
			ON CONFLICT (school_id, period) DO UPDATE SET status = 'failed', error = EXCLUDED.error
		`, schoolID, period, errMsg)
		return &run, err
	}

	run.Status = "done"
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO asset_depreciation_runs (school_id, period, status, error, created_at)
		VALUES ($1, $2, 'done', NULL, now())
		ON CONFLICT (school_id, period) DO UPDATE SET status = 'done', error = NULL
	`, schoolID, period)

	// Refresh book values for active assets
	_ = s.RefreshBookValues(ctx, schoolID, ac)

	return &run, nil
}

func (s *Store) RefreshBookValues(ctx context.Context, schoolID string, ac AccountingClient) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM assets WHERE school_id = $1 AND status = 'active'`, schoolID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var refs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			refs = append(refs, id)
		}
	}

	if len(refs) == 0 {
		return nil
	}

	// Fetch from accounting in chunks of 50
	for i := 0; i < len(refs); i += 50 {
		end := i + 50
		if end > len(refs) {
			end = len(refs)
		}
		chunk := refs[i:end]

		bvs, err := ac.GetBookValues(ctx, schoolID, chunk)
		if err != nil {
			continue
		}

		for _, bv := range bvs {
			_, _ = s.db.ExecContext(ctx, `
				INSERT INTO asset_book_values (
					asset_id, school_id, cost_idr, accumulated_idr, book_value_idr, last_period, as_of
				) VALUES ($1, $2, $3, $4, $5, $6, now())
				ON CONFLICT (asset_id) DO UPDATE SET
					cost_idr = EXCLUDED.cost_idr,
					accumulated_idr = EXCLUDED.accumulated_idr,
					book_value_idr = EXCLUDED.book_value_idr,
					last_period = EXCLUDED.last_period,
					as_of = now()
			`, bv.AssetRef, schoolID, bv.CostIDR, bv.AccumulatedIDR, bv.BookValueIDR, bv.LastPeriod)
		}
	}

	return nil
}

func (s *Store) GetAssetBookValue(ctx context.Context, schoolID, assetID string) (*BookValue, error) {
	var bv BookValue
	var lastPeriod sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT asset_id, school_id, cost_idr, accumulated_idr, book_value_idr, last_period, as_of
		FROM asset_book_values
		WHERE school_id = $1 AND asset_id = $2
	`, schoolID, assetID).Scan(
		&bv.AssetID, &bv.SchoolID, &bv.CostIDR, &bv.AccumulatedIDR, &bv.BookValueIDR,
		&lastPeriod, &bv.AsOf,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if lastPeriod.Valid {
		bv.LastPeriod = &lastPeriod.String
	}
	return &bv, nil
}
