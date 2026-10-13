package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound       = errors.New("Data tidak ditemukan")
	ErrCountingActive = errors.New("Gudang sedang stock opname")
	ErrInvalidState   = errors.New("Status dokumen tidak valid untuk operasi ini")
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

// NextDocNumber increments sequence and returns formatted document number.
func (s *Store) NextDocNumber(ctx context.Context, schoolID, prefix string, year int, format string) (string, error) {
	var seq int
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO doc_sequences (school_id, prefix, year, last)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (school_id, prefix, year)
		DO UPDATE SET last = doc_sequences.last + 1
		RETURNING last
	`, schoolID, prefix, year).Scan(&seq)
	if err != nil {
		return "", err
	}
	return FormatDocNumber(format, year, seq), nil
}

// --- Categories ---

func (s *Store) ListCategories(ctx context.Context, schoolID string) ([]ItemCategory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, code, name, tracking, purchase_acc_code, withholding_type_id, created_at, updated_at
		FROM item_categories
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ItemCategory
	for rows.Next() {
		var c ItemCategory
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.Code, &c.Name, &c.Tracking, &c.PurchaseAccCode, &c.WithholdingTypeID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) CreateCategory(ctx context.Context, c ItemCategory) (*ItemCategory, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO item_categories (school_id, code, name, tracking, purchase_acc_code, withholding_type_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, c.SchoolID, c.Code, c.Name, c.Tracking, c.PurchaseAccCode, c.WithholdingTypeID).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) UpdateCategory(ctx context.Context, c ItemCategory) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE item_categories
		SET name = $1, tracking = $2, purchase_acc_code = $3, withholding_type_id = $4, updated_at = now()
		WHERE id = $5 AND school_id = $6 AND deleted_at IS NULL
	`, c.Name, c.Tracking, c.PurchaseAccCode, c.WithholdingTypeID, c.ID, c.SchoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCategory(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE item_categories SET deleted_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
	`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Items ---

func (s *Store) ListItems(ctx context.Context, schoolID string, categoryID string, status string) ([]Item, error) {
	query := `
		SELECT i.id, i.school_id, i.code, i.name, i.category_id, c.name, c.tracking,
		       i.unit, i.min_stock, COALESCE(SUM(b.qty), 0) as current_stock,
		       i.status, i.created_at, i.updated_at
		FROM items i
		JOIN item_categories c ON c.id = i.category_id
		LEFT JOIN stock_balances b ON b.item_id = i.id AND b.school_id = i.school_id
		WHERE i.school_id = $1 AND i.deleted_at IS NULL
	`
	args := []any{schoolID}
	if categoryID != "" {
		args = append(args, categoryID)
		query += fmt.Sprintf(" AND i.category_id = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND i.status = $%d", len(args))
	}
	query += " GROUP BY i.id, c.name, c.tracking ORDER BY i.name ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.SchoolID, &it.Code, &it.Name, &it.CategoryID, &it.CategoryName, &it.Tracking,
			&it.Unit, &it.MinStock, &it.CurrentStock, &it.Status, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		it.BelowMin = it.CurrentStock < it.MinStock
		list = append(list, it)
	}
	return list, rows.Err()
}

func (s *Store) GetItem(ctx context.Context, schoolID, id string) (*Item, error) {
	var it Item
	err := s.db.QueryRowContext(ctx, `
		SELECT i.id, i.school_id, i.code, i.name, i.category_id, c.name, c.tracking,
		       i.unit, i.min_stock, COALESCE(SUM(b.qty), 0) as current_stock,
		       i.status, i.created_at, i.updated_at
		FROM items i
		JOIN item_categories c ON c.id = i.category_id
		LEFT JOIN stock_balances b ON b.item_id = i.id AND b.school_id = i.school_id
		WHERE i.id = $1 AND i.school_id = $2 AND i.deleted_at IS NULL
		GROUP BY i.id, c.name, c.tracking
	`, id, schoolID).Scan(&it.ID, &it.SchoolID, &it.Code, &it.Name, &it.CategoryID, &it.CategoryName, &it.Tracking,
		&it.Unit, &it.MinStock, &it.CurrentStock, &it.Status, &it.CreatedAt, &it.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	it.BelowMin = it.CurrentStock < it.MinStock
	return &it, nil
}

func (s *Store) CreateItem(ctx context.Context, it Item) (*Item, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO items (school_id, code, name, category_id, unit, min_stock, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`, it.SchoolID, it.Code, it.Name, it.CategoryID, it.Unit, it.MinStock, it.Status).Scan(&it.ID, &it.CreatedAt, &it.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (s *Store) UpdateItem(ctx context.Context, it Item) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE items
		SET name = $1, category_id = $2, unit = $3, min_stock = $4, status = $5, updated_at = now()
		WHERE id = $6 AND school_id = $7 AND deleted_at IS NULL
	`, it.Name, it.CategoryID, it.Unit, it.MinStock, it.Status, it.ID, it.SchoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteItem(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE items SET deleted_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
	`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Warehouses ---

func (s *Store) ListWarehouses(ctx context.Context, schoolID string) ([]Warehouse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, code, name, room_id, status, created_at, updated_at
		FROM warehouses
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Warehouse
	for rows.Next() {
		var w Warehouse
		if err := rows.Scan(&w.ID, &w.SchoolID, &w.Code, &w.Name, &w.RoomID, &w.Status, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

func (s *Store) CreateWarehouse(ctx context.Context, w Warehouse) (*Warehouse, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO warehouses (school_id, code, name, room_id, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, w.SchoolID, w.Code, w.Name, w.RoomID, w.Status).Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *Store) UpdateWarehouse(ctx context.Context, w Warehouse) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE warehouses
		SET name = $1, room_id = $2, status = $3, updated_at = now()
		WHERE id = $4 AND school_id = $5 AND deleted_at IS NULL
	`, w.Name, w.RoomID, w.Status, w.ID, w.SchoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteWarehouse(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE warehouses SET deleted_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
	`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Stock Balances & Movements ---

func (s *Store) ListBalances(ctx context.Context, schoolID, warehouseID, itemID string, belowMin bool) ([]StockBalance, error) {
	query := `
		SELECT b.school_id, b.warehouse_id, w.name, b.item_id, i.code, i.name, i.unit, i.min_stock, b.qty, b.updated_at
		FROM stock_balances b
		JOIN warehouses w ON w.id = b.warehouse_id
		JOIN items i ON i.id = b.item_id
		WHERE b.school_id = $1 AND w.deleted_at IS NULL AND i.deleted_at IS NULL
	`
	args := []any{schoolID}
	if warehouseID != "" {
		args = append(args, warehouseID)
		query += fmt.Sprintf(" AND b.warehouse_id = $%d", len(args))
	}
	if itemID != "" {
		args = append(args, itemID)
		query += fmt.Sprintf(" AND b.item_id = $%d", len(args))
	}
	if belowMin {
		query += " AND b.qty < i.min_stock"
	}
	query += " ORDER BY i.name ASC, w.name ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []StockBalance
	for rows.Next() {
		var b StockBalance
		var updatedAt time.Time
		if err := rows.Scan(&b.SchoolID, &b.WarehouseID, &b.WarehouseName, &b.ItemID, &b.ItemCode, &b.ItemName,
			&b.Unit, &b.MinStock, &b.Qty, &updatedAt); err != nil {
			return nil, err
		}
		b.UpdatedAt = updatedAt.Format(time.RFC3339)
		b.BelowMin = b.Qty < b.MinStock
		list = append(list, b)
	}
	return list, rows.Err()
}

// Move performs a single stock movement inside a transaction with row-level locking.
// Rejects movements if the warehouse has an active stock count ('counting').
// BR-INV-01: atomic, immutable movement record.
func (s *Store) Move(ctx context.Context, tx *sql.Tx, in MoveInput) (balanceAfter int64, crossedBelow bool, err error) {
	// 1. Check if warehouse is currently being counted
	var countActive int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM stock_counts
		WHERE warehouse_id = $1 AND status = 'counting'
	`, in.WarehouseID).Scan(&countActive)
	if err != nil {
		return 0, false, err
	}
	if countActive > 0 {
		return 0, false, ErrCountingActive
	}

	// 2. Fetch min_stock
	var minStock int64
	err = tx.QueryRowContext(ctx, `SELECT min_stock FROM items WHERE id = $1`, in.ItemID).Scan(&minStock)
	if err != nil {
		return 0, false, fmt.Errorf("item not found: %w", err)
	}

	// 3. Lock balance row
	var currentBalance int64
	err = tx.QueryRowContext(ctx, `
		SELECT qty FROM stock_balances
		WHERE warehouse_id = $1 AND item_id = $2
		FOR UPDATE
	`, in.WarehouseID, in.ItemID).Scan(&currentBalance)
	if err == sql.ErrNoRows {
		currentBalance = 0
		_, err = tx.ExecContext(ctx, `
			INSERT INTO stock_balances (school_id, warehouse_id, item_id, qty, updated_at)
			VALUES ($1, $2, $3, 0, now())
			ON CONFLICT (warehouse_id, item_id) DO NOTHING
		`, in.SchoolID, in.WarehouseID, in.ItemID)
		if err != nil {
			return 0, false, err
		}
	} else if err != nil {
		return 0, false, err
	}

	// 4. Calculate new balance
	newBalance, err := ApplyMovement(currentBalance, in.Direction, in.Qty)
	if err != nil {
		return 0, false, err
	}

	// 5. Update balance
	_, err = tx.ExecContext(ctx, `
		UPDATE stock_balances
		SET qty = $1, updated_at = now()
		WHERE warehouse_id = $2 AND item_id = $3
	`, newBalance, in.WarehouseID, in.ItemID)
	if err != nil {
		return 0, false, err
	}

	// 6. Record movement
	var mID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO stock_movements (school_id, warehouse_id, item_id, direction, qty, kind, ref_type, ref_id, ref_no, transfer_id, balance_after, note, created_by, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id
	`, in.SchoolID, in.WarehouseID, in.ItemID, in.Direction, in.Qty, in.Kind, in.RefType, in.RefID, in.RefNo, in.TransferID, newBalance, in.Note, in.CreatedBy, in.IdempotencyKey).Scan(&mID)
	if err != nil {
		return 0, false, err
	}

	crossedBelow = CrossedBelowMin(currentBalance, newBalance, minStock)
	return newBalance, crossedBelow, nil
}

func (s *Store) ListMovements(ctx context.Context, schoolID, warehouseID, itemID string, limit int) ([]StockMovement, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `
		SELECT m.id, m.school_id, m.warehouse_id, w.name, m.item_id, i.code, i.name, i.unit,
		       m.direction, m.qty, m.kind, m.ref_type, m.ref_id, m.ref_no, m.transfer_id,
		       m.balance_after, m.note, m.created_by, m.created_at, m.idempotency_key
		FROM stock_movements m
		JOIN warehouses w ON w.id = m.warehouse_id
		JOIN items i ON i.id = m.item_id
		WHERE m.school_id = $1
	`
	args := []any{schoolID}
	if warehouseID != "" {
		args = append(args, warehouseID)
		query += fmt.Sprintf(" AND m.warehouse_id = $%d", len(args))
	}
	if itemID != "" {
		args = append(args, itemID)
		query += fmt.Sprintf(" AND m.item_id = $%d", len(args))
	}
	query += fmt.Sprintf(" ORDER BY m.created_at DESC LIMIT %d", limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []StockMovement
	for rows.Next() {
		var m StockMovement
		if err := rows.Scan(&m.ID, &m.SchoolID, &m.WarehouseID, &m.WarehouseName, &m.ItemID, &m.ItemCode, &m.ItemName, &m.Unit,
			&m.Direction, &m.Qty, &m.Kind, &m.RefType, &m.RefID, &m.RefNo, &m.TransferID,
			&m.BalanceAfter, &m.Note, &m.CreatedBy, &m.CreatedAt, &m.IdempotencyKey); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// GetStockCard calculates opening balance, list of movements, and closing balance for a period.
func (s *Store) GetStockCard(ctx context.Context, schoolID, warehouseID, itemID string, from, to time.Time) (*StockCardReport, error) {
	report := &StockCardReport{
		ItemID:      itemID,
		WarehouseID: warehouseID,
	}
	_ = s.db.QueryRowContext(ctx, `SELECT name FROM items WHERE id = $1`, itemID).Scan(&report.ItemName)
	_ = s.db.QueryRowContext(ctx, `SELECT name FROM warehouses WHERE id = $1`, warehouseID).Scan(&report.WarehouseName)

	// Opening balance: sum of all net movements before 'from'
	var inBefore, outBefore int64
	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE WHEN direction = 'in' THEN qty ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN direction = 'out' THEN qty ELSE 0 END), 0)
		FROM stock_movements
		WHERE school_id = $1 AND warehouse_id = $2 AND item_id = $3 AND created_at < $4
	`, schoolID, warehouseID, itemID, from).Scan(&inBefore, &outBefore)
	report.OpeningBalance = inBefore - outBefore

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, created_at, direction, qty, kind, ref_no, balance_after, COALESCE(note, '')
		FROM stock_movements
		WHERE school_id = $1 AND warehouse_id = $2 AND item_id = $3 AND created_at >= $4 AND created_at <= $5
		ORDER BY created_at ASC
	`, schoolID, warehouseID, itemID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	running := report.OpeningBalance
	for rows.Next() {
		var e StockCardEntry
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.Direction, &e.Qty, &e.Kind, &e.RefNo, &e.BalanceAfter, &e.Note); err != nil {
			return nil, err
		}
		if e.Direction == "in" {
			running += e.Qty
		} else {
			running -= e.Qty
		}
		report.Entries = append(report.Entries, e)
	}
	report.ClosingBalance = running
	return report, nil
}

// --- Stock Opname ---

func (s *Store) CreateStockCount(ctx context.Context, schoolID, warehouseID, userID, number string) (*StockCount, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var c StockCount
	c.SchoolID = schoolID
	c.WarehouseID = warehouseID
	c.StartedBy = userID
	c.Number = number
	c.Status = "counting"
	c.StartedAt = time.Now()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO stock_counts (school_id, warehouse_id, number, status, started_at, started_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, schoolID, warehouseID, number, c.Status, c.StartedAt, userID).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Freeze system quantities from current active items in category tracking='stock'
	_, err = tx.ExecContext(ctx, `
		INSERT INTO stock_count_lines (school_id, count_id, item_id, system_qty)
		SELECT $1, $2, i.id, COALESCE(b.qty, 0)
		FROM items i
		JOIN item_categories cat ON cat.id = i.category_id AND cat.tracking = 'stock'
		LEFT JOIN stock_balances b ON b.item_id = i.id AND b.warehouse_id = $3
		WHERE i.school_id = $1 AND i.deleted_at IS NULL AND i.status = 'active'
	`, schoolID, c.ID, warehouseID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetStockCount(ctx, schoolID, c.ID)
}

func (s *Store) GetStockCount(ctx context.Context, schoolID, countID string) (*StockCount, error) {
	var c StockCount
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.school_id, c.warehouse_id, w.name, c.number, c.status,
		       c.started_at, c.started_by, c.submitted_at, c.approval_id, c.report_object_key, c.error,
		       c.created_at, c.updated_at
		FROM stock_counts c
		JOIN warehouses w ON w.id = c.warehouse_id
		WHERE c.id = $1 AND c.school_id = $2
	`, countID, schoolID).Scan(&c.ID, &c.SchoolID, &c.WarehouseID, &c.WarehouseName, &c.Number, &c.Status,
		&c.StartedAt, &c.StartedBy, &c.SubmittedAt, &c.ApprovalID, &c.ReportObjectKey, &c.Error,
		&c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.school_id, l.count_id, l.item_id, i.code, i.name, i.unit,
		       l.system_qty, l.counted_qty, l.note
		FROM stock_count_lines l
		JOIN items i ON i.id = l.item_id
		WHERE l.count_id = $1
		ORDER BY i.name ASC
	`, countID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var l StockCountLine
		if err := rows.Scan(&l.ID, &l.SchoolID, &l.CountID, &l.ItemID, &l.ItemCode, &l.ItemName, &l.Unit,
			&l.SystemQty, &l.CountedQty, &l.Note); err != nil {
			return nil, err
		}
		l.Variance = CountVariance(l.SystemQty, l.CountedQty)
		c.Lines = append(c.Lines, l)
	}
	return &c, nil
}

func (s *Store) ListStockCounts(ctx context.Context, schoolID, warehouseID string) ([]StockCount, error) {
	query := `
		SELECT c.id, c.school_id, c.warehouse_id, w.name, c.number, c.status,
		       c.started_at, c.started_by, c.submitted_at, c.approval_id, c.report_object_key, c.error,
		       c.created_at, c.updated_at
		FROM stock_counts c
		JOIN warehouses w ON w.id = c.warehouse_id
		WHERE c.school_id = $1
	`
	args := []any{schoolID}
	if warehouseID != "" {
		args = append(args, warehouseID)
		query += fmt.Sprintf(" AND c.warehouse_id = $%d", len(args))
	}
	query += " ORDER BY c.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []StockCount
	for rows.Next() {
		var c StockCount
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.WarehouseID, &c.WarehouseName, &c.Number, &c.Status,
			&c.StartedAt, &c.StartedBy, &c.SubmittedAt, &c.ApprovalID, &c.ReportObjectKey, &c.Error,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) UpdateCountLines(ctx context.Context, schoolID, countID string, lines []StockCountLine) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM stock_counts WHERE id = $1 AND school_id = $2`, countID, schoolID).Scan(&status)
	if err != nil {
		return err
	}
	if status != "counting" && status != "submitted" {
		return ErrInvalidState
	}

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE stock_count_lines
		SET counted_qty = $1, note = $2
		WHERE count_id = $3 AND item_id = $4
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, l := range lines {
		if _, err := stmt.ExecContext(ctx, l.CountedQty, l.Note, countID, l.ItemID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SubmitStockCount(ctx context.Context, schoolID, countID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE stock_counts
		SET status = 'submitted', submitted_at = now(), updated_at = now()
		WHERE id = $1 AND school_id = $2 AND status = 'counting'
	`, countID, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidState
	}
	return nil
}

func (s *Store) SetCountApproval(ctx context.Context, countID, approvalID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE stock_counts
		SET status = 'pending_approval', approval_id = $1, updated_at = now()
		WHERE id = $2
	`, approvalID, countID)
	return err
}

func (s *Store) CancelStockCount(ctx context.Context, schoolID, countID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE stock_counts
		SET status = 'cancelled', updated_at = now()
		WHERE id = $1 AND school_id = $2 AND status IN ('counting', 'submitted', 'rejected')
	`, countID, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidState
	}
	return nil
}

// ApplyAdjustment executes stock adjustment movements when count is approved by Core.
// All adjustments run atomically in a single transaction. If any item balance would become negative,
// the entire transaction rollbacks and count status is set to 'adjust_failed'.
func (s *Store) ApplyAdjustment(ctx context.Context, schoolID, countID string) error {
	count, err := s.GetStockCount(ctx, schoolID, countID)
	if err != nil {
		return err
	}
	if count.Status == "adjusted" {
		return nil // already applied idempotently
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, line := range count.Lines {
		if line.CountedQty == nil {
			continue
		}
		variance := *line.CountedQty - line.SystemQty
		if variance == 0 {
			continue
		}

		direction := "in"
		qty := variance
		if variance < 0 {
			direction = "out"
			qty = -variance
		}

		in := MoveInput{
			SchoolID:       schoolID,
			WarehouseID:    count.WarehouseID,
			ItemID:         line.ItemID,
			Direction:      direction,
			Qty:            qty,
			Kind:           "adjustment",
			RefType:        "stock_count",
			RefID:          count.ID,
			RefNo:          count.Number,
			CreatedBy:      count.StartedBy,
			IdempotencyKey: fmt.Sprintf("adj:%s:%s", count.ID, line.ItemID),
		}

		// Row level lock & update balance
		var curBalance int64
		err = tx.QueryRowContext(ctx, `
			SELECT qty FROM stock_balances
			WHERE warehouse_id = $1 AND item_id = $2
			FOR UPDATE
		`, in.WarehouseID, in.ItemID).Scan(&curBalance)
		if err == sql.ErrNoRows {
			curBalance = 0
		} else if err != nil {
			return err
		}

		newBal, err := ApplyMovement(curBalance, in.Direction, in.Qty)
		if err != nil {
			// Failed due to negative stock balance! Record error and set adjust_failed
			_ = tx.Rollback()
			errMsg := fmt.Sprintf("Gagal penyesuaian untuk %s: %v", line.ItemName, err)
			_, _ = s.db.ExecContext(ctx, `
				UPDATE stock_counts
				SET status = 'adjust_failed', error = $1, updated_at = now()
				WHERE id = $2
			`, errMsg, countID)
			return err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO stock_balances (school_id, warehouse_id, item_id, qty, updated_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (warehouse_id, item_id)
			DO UPDATE SET qty = EXCLUDED.qty, updated_at = now()
		`, schoolID, in.WarehouseID, in.ItemID, newBal)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO stock_movements (school_id, warehouse_id, item_id, direction, qty, kind, ref_type, ref_id, ref_no, balance_after, created_by, idempotency_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (school_id, idempotency_key) DO NOTHING
		`, schoolID, in.WarehouseID, in.ItemID, in.Direction, in.Qty, in.Kind, in.RefType, in.RefID, in.RefNo, newBal, in.CreatedBy, in.IdempotencyKey)
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE stock_counts
		SET status = 'adjusted', error = NULL, updated_at = now()
		WHERE id = $1 AND school_id = $2
	`, countID, schoolID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) RejectStockCount(ctx context.Context, schoolID, countID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE stock_counts
		SET status = 'rejected', updated_at = now()
		WHERE id = $1 AND school_id = $2
	`, countID, schoolID)
	return err
}
