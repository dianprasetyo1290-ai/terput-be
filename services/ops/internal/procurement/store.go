package procurement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"skci/ops/internal/crypto"
)

var (
	ErrNotFound      = errors.New("Data tidak ditemukan")
	ErrInvalidState  = errors.New("Status dokumen tidak valid untuk operasi ini")
	ErrVendorBlocked = errors.New("Vendor berstatus diblokir atau nonaktif")
	ErrExcessReceipt = errors.New("Melebihi sisa PO")
)

type Store struct {
	db     *sql.DB
	crypto *crypto.Service
}

func NewStore(db *sql.DB, c *crypto.Service) *Store {
	return &Store{db: db, crypto: c}
}

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

// --- Vendors ---

func (s *Store) ListVendors(ctx context.Context, schoolID, category, status string) ([]Vendor, error) {
	query := `
		SELECT id, school_id, code, name, npwp_enc, address, COALESCE(array_to_json(categories)::text, '[]'), payment_terms_days, status, duplicate_ack_by, created_at, updated_at
		FROM vendors
		WHERE school_id = $1 AND deleted_at IS NULL
	`
	args := []any{schoolID}
	if category != "" {
		args = append(args, category)
		query += fmt.Sprintf(" AND $%d = ANY(categories)", len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	query += " ORDER BY name ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Vendor
	for rows.Next() {
		var v Vendor
		var npwpEnc []byte
		var catsJSON string
		if err := rows.Scan(&v.ID, &v.SchoolID, &v.Code, &v.Name, &npwpEnc, &v.Address, &catsJSON,
			&v.PaymentTermsDays, &v.Status, &v.DuplicateAckBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(catsJSON), &v.Categories)
		if v.Categories == nil {
			v.Categories = []string{}
		}
		if len(npwpEnc) > 0 && s.crypto != nil {
			dec, err := s.crypto.Decrypt(npwpEnc, fmt.Sprintf("vendors:%s:%s", schoolID, v.ID))
			if err == nil && len(dec) >= 4 {
				v.NPWP = fmt.Sprintf("****-****-%s", dec[len(dec)-4:]) // masked
			}
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func (s *Store) GetVendor(ctx context.Context, schoolID, id string) (*Vendor, error) {
	var v Vendor
	var npwpEnc []byte
	var catsJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, code, name, npwp_enc, address, COALESCE(array_to_json(categories)::text, '[]'), payment_terms_days, status, duplicate_ack_by, created_at, updated_at
		FROM vendors
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
	`, id, schoolID).Scan(&v.ID, &v.SchoolID, &v.Code, &v.Name, &npwpEnc, &v.Address, &catsJSON,
		&v.PaymentTermsDays, &v.Status, &v.DuplicateAckBy, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(catsJSON), &v.Categories)
	if v.Categories == nil {
		v.Categories = []string{}
	}

	if len(npwpEnc) > 0 && s.crypto != nil {
		dec, err := s.crypto.Decrypt(npwpEnc, fmt.Sprintf("vendors:%s:%s", schoolID, v.ID))
		if err == nil {
			v.NPWP = dec
		}
	}

	// Contacts
	cRows, err := s.db.QueryContext(ctx, `SELECT id, school_id, vendor_id, name, phone, email, is_primary, created_at FROM vendor_contacts WHERE vendor_id = $1`, id)
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var c VendorContact
			_ = cRows.Scan(&c.ID, &c.SchoolID, &c.VendorID, &c.Name, &c.Phone, &c.Email, &c.IsPrimary, &c.CreatedAt)
			v.Contacts = append(v.Contacts, c)
		}
	}

	// Documents
	dRows, err := s.db.QueryContext(ctx, `SELECT id, school_id, vendor_id, kind, object_key, expires_on, uploaded_by, created_at FROM vendor_documents WHERE vendor_id = $1`, id)
	if err == nil {
		defer dRows.Close()
		for dRows.Next() {
			var d VendorDocument
			_ = dRows.Scan(&d.ID, &d.SchoolID, &d.VendorID, &d.Kind, &d.ObjectKey, &d.ExpiresOn, &d.UploadedBy, &d.CreatedAt)
			v.Documents = append(v.Documents, d)
		}
	}

	// Bank Accounts
	bRows, err := s.db.QueryContext(ctx, `SELECT id, school_id, vendor_id, bank_name, account_no_enc, account_holder, status, approval_id, requested_by, activated_at, created_at FROM vendor_bank_accounts WHERE vendor_id = $1 ORDER BY created_at DESC`, id)
	if err == nil {
		defer bRows.Close()
		for bRows.Next() {
			var b VendorBankAccount
			var accEnc []byte
			_ = bRows.Scan(&b.ID, &b.SchoolID, &b.VendorID, &b.BankName, &accEnc, &b.AccountHolder, &b.Status, &b.ApprovalID, &b.RequestedBy, &b.ActivatedAt, &b.CreatedAt)
			if len(accEnc) > 0 && s.crypto != nil {
				dec, err := s.crypto.Decrypt(accEnc, fmt.Sprintf("vendor_bank_accounts:%s:%s", schoolID, b.ID))
				if err == nil && len(dec) >= 4 {
					b.AccountNo = fmt.Sprintf("****%s", dec[len(dec)-4:]) // masked 4 digits
				}
			}
			v.BankAccounts = append(v.BankAccounts, b)
		}
	}

	return &v, nil
}

func (s *Store) CreateVendor(ctx context.Context, v Vendor) (*Vendor, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id string
	var createdAt, updatedAt time.Time
	catArray := "{" + strings.Join(v.Categories, ",") + "}"
	err = tx.QueryRowContext(ctx, `
		INSERT INTO vendors (school_id, code, name, address, categories, payment_terms_days, status)
		VALUES ($1, $2, $3, $4, $5::text[], $6, $7)
		RETURNING id, created_at, updated_at
	`, v.SchoolID, v.Code, v.Name, v.Address, catArray, v.PaymentTermsDays, v.Status).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	v.ID = id
	v.CreatedAt = createdAt
	v.UpdatedAt = updatedAt

	// Encrypt NPWP if provided
	if v.NPWP != "" && s.crypto != nil {
		aad := fmt.Sprintf("vendors:%s:%s", v.SchoolID, v.ID)
		enc, bidx, err := s.crypto.EncryptWithIndex(v.NPWP, aad)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE vendors SET npwp_enc = $1, npwp_bidx = $2 WHERE id = $3`, enc, bidx, v.ID)
		if err != nil {
			return nil, err
		}
	}

	// Insert into vendor_accounting_sync outbox
	payloadHash := fmt.Sprintf("v:%s:%s", v.ID, v.Code)
	_, _ = tx.ExecContext(ctx, `
		INSERT INTO vendor_accounting_sync (vendor_id, school_id, payload_hash, status)
		VALUES ($1, $2, $3, 'pending')
		ON CONFLICT (vendor_id) DO UPDATE SET status = 'pending', attempts = 0, updated_at = now()
	`, v.ID, v.SchoolID, payloadHash)

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Store) CheckDuplicates(ctx context.Context, schoolID, npwp, accountNo string) ([]string, error) {
	var existing []ExistingVendorInfo
	rows, err := s.db.QueryContext(ctx, `
		SELECT v.id, v.npwp_bidx
		FROM vendors v
		WHERE v.school_id = $1 AND v.deleted_at IS NULL
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var info ExistingVendorInfo
		_ = rows.Scan(&info.VendorID, &info.NPWPBidx)
		existing = append(existing, info)
	}

	// Fetch bank accounts
	bRows, err := s.db.QueryContext(ctx, `SELECT vendor_id, account_no_bidx FROM vendor_bank_accounts WHERE school_id = $1 AND status = 'active'`, schoolID)
	if err == nil {
		defer bRows.Close()
		accMap := make(map[string][]string)
		for bRows.Next() {
			var vid, bidx string
			_ = bRows.Scan(&vid, &bidx)
			accMap[vid] = append(accMap[vid], bidx)
		}
		for i := range existing {
			existing[i].AccountBidx = accMap[existing[i].VendorID]
		}
	}

	npwpBidx := ""
	if npwp != "" && s.crypto != nil {
		npwpBidx = s.crypto.BlindIndex(npwp, fmt.Sprintf("vendors:%s", schoolID))
	}
	accBidx := ""
	if accountNo != "" && s.crypto != nil {
		accBidx = s.crypto.BlindIndex(accountNo, fmt.Sprintf("vendor_bank_accounts:%s", schoolID))
	}

	return VendorDuplicates(npwpBidx, accBidx, existing), nil
}

func (s *Store) AddBankAccount(ctx context.Context, b VendorBankAccount, rawAccountNo string) (*VendorBankAccount, error) {
	if s.crypto == nil {
		return nil, errors.New("crypto service not initialized")
	}
	var id string
	var createdAt time.Time
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO vendor_bank_accounts (school_id, vendor_id, bank_name, account_no_enc, account_no_bidx, account_holder, status, requested_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending_approval', $7)
		RETURNING id, created_at
	`, b.SchoolID, b.VendorID, b.BankName, []byte(""), "", b.AccountHolder, b.RequestedBy).Scan(&id, &createdAt)
	if err != nil {
		return nil, err
	}
	b.ID = id
	b.CreatedAt = createdAt

	aad := fmt.Sprintf("vendor_bank_accounts:%s:%s", b.SchoolID, b.ID)
	enc, bidx, err := s.crypto.EncryptWithIndex(rawAccountNo, aad)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE vendor_bank_accounts
		SET account_no_enc = $1, account_no_bidx = $2
		WHERE id = $3
	`, enc, bidx, b.ID)
	if err != nil {
		return nil, err
	}

	if len(rawAccountNo) >= 4 {
		b.AccountNo = fmt.Sprintf("****%s", rawAccountNo[len(rawAccountNo)-4:])
	}
	return &b, nil
}

func (s *Store) ActivateBankAccount(ctx context.Context, schoolID, accountID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var vendorID string
	err = tx.QueryRowContext(ctx, `SELECT vendor_id FROM vendor_bank_accounts WHERE id = $1 AND school_id = $2`, accountID, schoolID).Scan(&vendorID)
	if err != nil {
		return err
	}

	// AC-106-01: Supersede older active accounts
	_, err = tx.ExecContext(ctx, `
		UPDATE vendor_bank_accounts
		SET status = 'superseded'
		WHERE vendor_id = $1 AND status = 'active'
	`, vendorID)
	if err != nil {
		return err
	}

	// Activate this account
	_, err = tx.ExecContext(ctx, `
		UPDATE vendor_bank_accounts
		SET status = 'active', activated_at = now()
		WHERE id = $1
	`, accountID)
	if err != nil {
		return err
	}

	// Trigger accounting sync outbox
	_, _ = tx.ExecContext(ctx, `
		INSERT INTO vendor_accounting_sync (vendor_id, school_id, payload_hash, status)
		VALUES ($1, $2, $3, 'pending')
		ON CONFLICT (vendor_id) DO UPDATE SET status = 'pending', attempts = 0, updated_at = now()
	`, vendorID, schoolID, fmt.Sprintf("act:%s", accountID))

	return tx.Commit()
}

func (s *Store) RejectBankAccount(ctx context.Context, schoolID, accountID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE vendor_bank_accounts
		SET status = 'rejected'
		WHERE id = $1 AND school_id = $2
	`, accountID, schoolID)
	return err
}

// --- Purchase Requests ---

func (s *Store) CreatePR(ctx context.Context, pr PurchaseRequest, lines []PurchaseRequestLine, idempotencyKey string) (*PurchaseRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var calcLines []PRLineCalc
	for _, l := range lines {
		calcLines = append(calcLines, PRLineCalc{
			Qty:             l.Qty,
			EstUnitPriceIDR: l.EstUnitPriceIDR,
			QtyOrdered:      0,
			BudgetLineKey:   l.BudgetLineKey,
		})
	}
	pr.TotalEstIDR = PRTotal(calcLines)

	var budgetCheckJSON []byte
	if pr.BudgetCheck != nil {
		budgetCheckJSON, _ = json.Marshal(pr.BudgetCheck)
	}

	var id string
	var createdAt, updatedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO purchase_requests (school_id, number, requester_user_id, reason, needed_by, status, total_est_idr, over_budget, budget_check, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, 'draft', $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`, pr.SchoolID, pr.Number, pr.RequesterUserID, pr.Reason, pr.NeededBy, pr.TotalEstIDR, pr.OverBudget, budgetCheckJSON, idempotencyKey).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	pr.ID = id
	pr.Status = "draft"
	pr.CreatedAt = createdAt
	pr.UpdatedAt = updatedAt

	for i := range lines {
		lines[i].PRID = id
		lines[i].SchoolID = pr.SchoolID
		err = tx.QueryRowContext(ctx, `
			INSERT INTO purchase_request_lines (school_id, pr_id, item_id, description, qty, unit, est_unit_price_idr, budget_line_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id
		`, lines[i].SchoolID, lines[i].PRID, lines[i].ItemID, lines[i].Description, lines[i].Qty, lines[i].Unit, lines[i].EstUnitPriceIDR, lines[i].BudgetLineKey).Scan(&lines[i].ID)
		if err != nil {
			return nil, err
		}
	}
	pr.Lines = lines

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (s *Store) GetPR(ctx context.Context, schoolID, id string) (*PurchaseRequest, error) {
	var pr PurchaseRequest
	var budgetCheckJSON []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, number, requester_user_id, reason, needed_by, status, total_est_idr, over_budget, budget_check, approval_id, created_at, updated_at
		FROM purchase_requests
		WHERE id = $1 AND school_id = $2
	`, id, schoolID).Scan(&pr.ID, &pr.SchoolID, &pr.Number, &pr.RequesterUserID, &pr.Reason, &pr.NeededBy,
		&pr.Status, &pr.TotalEstIDR, &pr.OverBudget, &budgetCheckJSON, &pr.ApprovalID, &pr.CreatedAt, &pr.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(budgetCheckJSON) > 0 {
		_ = json.Unmarshal(budgetCheckJSON, &pr.BudgetCheck)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.school_id, l.pr_id, l.item_id, i.code, i.name, l.description, l.qty, l.unit, l.est_unit_price_idr, l.budget_line_key, l.qty_ordered
		FROM purchase_request_lines l
		LEFT JOIN items i ON i.id = l.item_id
		WHERE l.pr_id = $1
		ORDER BY l.id ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var l PurchaseRequestLine
		if err := rows.Scan(&l.ID, &l.SchoolID, &l.PRID, &l.ItemID, &l.ItemCode, &l.ItemName, &l.Description, &l.Qty, &l.Unit, &l.EstUnitPriceIDR, &l.BudgetLineKey, &l.QtyOrdered); err != nil {
			return nil, err
		}
		pr.Lines = append(pr.Lines, l)
	}
	return &pr, nil
}

func (s *Store) ListPRs(ctx context.Context, schoolID, requesterUserID, status string) ([]PurchaseRequest, error) {
	query := `
		SELECT id, school_id, number, requester_user_id, reason, needed_by, status, total_est_idr, over_budget, approval_id, created_at, updated_at
		FROM purchase_requests
		WHERE school_id = $1
	`
	args := []any{schoolID}
	if requesterUserID != "" {
		args = append(args, requesterUserID)
		query += fmt.Sprintf(" AND requester_user_id = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PurchaseRequest
	for rows.Next() {
		var pr PurchaseRequest
		if err := rows.Scan(&pr.ID, &pr.SchoolID, &pr.Number, &pr.RequesterUserID, &pr.Reason, &pr.NeededBy,
			&pr.Status, &pr.TotalEstIDR, &pr.OverBudget, &pr.ApprovalID, &pr.CreatedAt, &pr.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, pr)
	}
	return list, rows.Err()
}

func (s *Store) SubmitPR(ctx context.Context, schoolID, id, approvalID string, overBudget bool, budgetCheck any) error {
	var budgetCheckJSON []byte
	if budgetCheck != nil {
		budgetCheckJSON, _ = json.Marshal(budgetCheck)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE purchase_requests
		SET status = 'pending_approval', approval_id = $1, over_budget = $2, budget_check = $3, updated_at = now()
		WHERE id = $4 AND school_id = $5 AND status = 'draft'
	`, approvalID, overBudget, budgetCheckJSON, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidState
	}
	return nil
}

func (s *Store) CancelPR(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE purchase_requests
		SET status = 'cancelled', updated_at = now()
		WHERE id = $1 AND school_id = $2 AND status IN ('draft', 'pending_approval', 'approved')
	`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidState
	}
	return nil
}

// --- Purchase Orders ---

func (s *Store) CreatePO(ctx context.Context, po PurchaseOrder, lines []PurchaseOrderLine, idempotencyKey string) (*PurchaseOrder, error) {
	// Validate vendor is active
	var vStatus string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM vendors WHERE id = $1 AND school_id = $2`, po.VendorID, po.SchoolID).Scan(&vStatus)
	if err != nil {
		return nil, fmt.Errorf("vendor not found: %w", err)
	}
	if vStatus != "active" {
		return nil, ErrVendorBlocked
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var totalIDR int64
	for _, l := range lines {
		totalIDR += l.Qty * l.UnitPriceIDR
	}
	po.TotalIDR = totalIDR

	var id string
	var createdAt, updatedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO purchase_orders (school_id, number, vendor_id, payment_terms_days, expected_date, status, total_idr, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, 'draft', $6, $7)
		RETURNING id, created_at, updated_at
	`, po.SchoolID, po.Number, po.VendorID, po.PaymentTermsDays, po.ExpectedDate, po.TotalIDR, idempotencyKey).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	po.ID = id
	po.Status = "draft"
	po.CreatedAt = createdAt
	po.UpdatedAt = updatedAt

	for i := range lines {
		lines[i].POID = id
		lines[i].SchoolID = po.SchoolID
		err = tx.QueryRowContext(ctx, `
			INSERT INTO purchase_order_lines (school_id, po_id, pr_line_id, item_id, description, qty, unit_price_idr, budget_line_key, withholding_type_id, warehouse_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, amount_idr
		`, lines[i].SchoolID, lines[i].POID, lines[i].PRLineID, lines[i].ItemID, lines[i].Description, lines[i].Qty, lines[i].UnitPriceIDR, lines[i].BudgetLineKey, lines[i].WithholdingTypeID, lines[i].WarehouseID).Scan(&lines[i].ID, &lines[i].AmountIDR)
		if err != nil {
			return nil, err
		}

		// Update qty_ordered on PR line if referenced
		if lines[i].PRLineID != nil {
			_, err = tx.ExecContext(ctx, `
				UPDATE purchase_request_lines
				SET qty_ordered = qty_ordered + $1
				WHERE id = $2
			`, lines[i].Qty, *lines[i].PRLineID)
			if err != nil {
				return nil, err
			}
		}
	}

	// Update PR status if lines belong to PRs
	_, _ = tx.ExecContext(ctx, `
		UPDATE purchase_requests pr
		SET status = CASE
			WHEN (SELECT bool_and(qty_ordered >= qty) FROM purchase_request_lines WHERE pr_id = pr.id) THEN 'ordered'
			WHEN (SELECT bool_or(qty_ordered > 0) FROM purchase_request_lines WHERE pr_id = pr.id) THEN 'partially_ordered'
			ELSE status
		END
		WHERE pr.id IN (SELECT DISTINCT pr_id FROM purchase_request_lines WHERE id IN (SELECT pr_line_id FROM purchase_order_lines WHERE po_id = $1))
	`, id)

	po.Lines = lines
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &po, nil
}

func (s *Store) GetPO(ctx context.Context, schoolID, id string) (*PurchaseOrder, error) {
	var po PurchaseOrder
	err := s.db.QueryRowContext(ctx, `
		SELECT p.id, p.school_id, p.number, p.vendor_id, v.name, p.payment_terms_days, p.expected_date, p.status, p.total_idr, p.approval_id, p.pdf_object_key, p.sent_at, p.closed_reason, p.created_at, p.updated_at
		FROM purchase_orders p
		JOIN vendors v ON v.id = p.vendor_id
		WHERE p.id = $1 AND p.school_id = $2
	`, id, schoolID).Scan(&po.ID, &po.SchoolID, &po.Number, &po.VendorID, &po.VendorName, &po.PaymentTermsDays,
		&po.ExpectedDate, &po.Status, &po.TotalIDR, &po.ApprovalID, &po.PDFObjectKey, &po.SentAt, &po.ClosedReason, &po.CreatedAt, &po.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Lines with accepted quantities
	rows, err := s.db.QueryContext(ctx, `
		SELECT pol.id, pol.school_id, pol.po_id, pol.pr_line_id, pol.item_id, i.code, i.name, pol.description, pol.qty, pol.unit_price_idr, pol.amount_idr, pol.budget_line_key, pol.withholding_type_id, pol.warehouse_id, w.name,
		       COALESCE(acc.qty_acc, 0) - COALESCE(ret.qty_ret, 0) as qty_accepted_net
		FROM purchase_order_lines pol
		LEFT JOIN items i ON i.id = pol.item_id
		LEFT JOIN warehouses w ON w.id = pol.warehouse_id
		LEFT JOIN (
			SELECT po_line_id, SUM(qty_accepted) as qty_acc FROM goods_receipt_lines GROUP BY po_line_id
		) acc ON acc.po_line_id = pol.id
		LEFT JOIN (
			SELECT grl.po_line_id, SUM(grt.qty) as qty_ret
			FROM goods_returns grt
			JOIN goods_receipt_lines grl ON grl.id = grt.gr_line_id
			GROUP BY grl.po_line_id
		) ret ON ret.po_line_id = pol.id
		WHERE pol.po_id = $1
		ORDER BY pol.id ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var l PurchaseOrderLine
		if err := rows.Scan(&l.ID, &l.SchoolID, &l.POID, &l.PRLineID, &l.ItemID, &l.ItemCode, &l.ItemName,
			&l.Description, &l.Qty, &l.UnitPriceIDR, &l.AmountIDR, &l.BudgetLineKey, &l.WithholdingTypeID,
			&l.WarehouseID, &l.WarehouseName, &l.QtyAcceptedNet); err != nil {
			return nil, err
		}
		rem := l.Qty - l.QtyAcceptedNet
		if rem < 0 {
			rem = 0
		}
		l.QtyRemaining = rem
		po.Lines = append(po.Lines, l)
	}
	return &po, nil
}

func (s *Store) ListPOs(ctx context.Context, schoolID, vendorID, status string) ([]PurchaseOrder, error) {
	query := `
		SELECT p.id, p.school_id, p.number, p.vendor_id, v.name, p.payment_terms_days, p.expected_date, p.status, p.total_idr, p.approval_id, p.pdf_object_key, p.sent_at, p.closed_reason, p.created_at, p.updated_at
		FROM purchase_orders p
		JOIN vendors v ON v.id = p.vendor_id
		WHERE p.school_id = $1
	`
	args := []any{schoolID}
	if vendorID != "" {
		args = append(args, vendorID)
		query += fmt.Sprintf(" AND p.vendor_id = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND p.status = $%d", len(args))
	}
	query += " ORDER BY p.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PurchaseOrder
	for rows.Next() {
		var po PurchaseOrder
		if err := rows.Scan(&po.ID, &po.SchoolID, &po.Number, &po.VendorID, &po.VendorName, &po.PaymentTermsDays,
			&po.ExpectedDate, &po.Status, &po.TotalIDR, &po.ApprovalID, &po.PDFObjectKey, &po.SentAt, &po.ClosedReason, &po.CreatedAt, &po.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, po)
	}
	return list, rows.Err()
}

func (s *Store) SubmitPO(ctx context.Context, schoolID, id, approvalID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE purchase_orders
		SET status = 'pending_approval', approval_id = $1, updated_at = now()
		WHERE id = $2 AND school_id = $3 AND status = 'draft'
	`, approvalID, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidState
	}
	return nil
}

func (s *Store) SetPOApproved(ctx context.Context, id, pdfObjectKey string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE purchase_orders
		SET status = 'approved', pdf_object_key = $1, updated_at = now()
		WHERE id = $2 AND status = 'pending_approval'
	`, pdfObjectKey, id)
	return err
}

func (s *Store) MarkPOSent(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE purchase_orders
		SET status = 'sent', sent_at = now(), updated_at = now()
		WHERE id = $1 AND school_id = $2 AND status = 'approved'
	`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidState
	}
	return nil
}

func (s *Store) ClosePO(ctx context.Context, schoolID, id, reason string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE purchase_orders
		SET status = 'closed', closed_reason = $1, updated_at = now()
		WHERE id = $2 AND school_id = $3 AND status IN ('sent', 'partially_received')
	`, reason, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidState
	}
	return nil
}

func (s *Store) CancelPO(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE purchase_orders
		SET status = 'cancelled', updated_at = now()
		WHERE id = $1 AND school_id = $2 AND status IN ('draft', 'pending_approval')
	`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidState
	}
	return nil
}

// --- Goods Receipts ---

func (s *Store) CreateReceipt(ctx context.Context, gr GoodsReceipt, lines []GoodsReceiptLine, idempotencyKey string) (*GoodsReceipt, error) {
	po, err := s.GetPO(ctx, gr.SchoolID, gr.POID)
	if err != nil {
		return nil, err
	}
	if po.Status != "approved" && po.Status != "sent" && po.Status != "partially_received" {
		return nil, ErrInvalidState
	}

	// Validate remaining
	lineMap := make(map[string]PurchaseOrderLine)
	for _, pol := range po.Lines {
		lineMap[pol.ID] = pol
	}

	for _, l := range lines {
		pol, ok := lineMap[l.POLineID]
		if !ok {
			return nil, fmt.Errorf("po line %s not found in PO", l.POLineID)
		}
		if l.QtyReceived != l.QtyAccepted+l.QtyRejected {
			return nil, errors.New("qty_received harus sama dengan qty_accepted + qty_rejected")
		}
		if l.QtyAccepted > pol.QtyRemaining {
			return nil, ErrExcessReceipt // 422 Melebihi sisa PO
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var grID string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO goods_receipts (school_id, number, po_id, received_on, received_by, note, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`, gr.SchoolID, gr.Number, gr.POID, gr.ReceivedOn, gr.ReceivedBy, gr.Note, idempotencyKey).Scan(&grID, &createdAt)
	if err != nil {
		return nil, err
	}
	gr.ID = grID
	gr.CreatedAt = createdAt

	var assetHandoffLines []GoodsReceivedLine
	var trackingMap = make(map[string]string)
	for _, l := range lines {
		pol := lineMap[l.POLineID]
		if pol.ItemID != nil {
			var tracking string
			_ = tx.QueryRowContext(ctx, `SELECT c.tracking FROM items i JOIN item_categories c ON c.id = i.category_id WHERE i.id = $1`, *pol.ItemID).Scan(&tracking)
			trackingMap[*pol.ItemID] = tracking
		}

		var lineID string
		err = tx.QueryRowContext(ctx, `
			INSERT INTO goods_receipt_lines (school_id, gr_id, po_line_id, qty_received, qty_accepted, qty_rejected, reject_reason)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`, gr.SchoolID, gr.ID, l.POLineID, l.QtyReceived, l.QtyAccepted, l.QtyRejected, l.RejectReason).Scan(&lineID)
		if err != nil {
			return nil, err
		}
		l.ID = lineID
		gr.Lines = append(gr.Lines, l)

		// Check if asset
		if pol.ItemID != nil && trackingMap[*pol.ItemID] == "asset" && l.QtyAccepted > 0 {
			name := ""
			if pol.ItemName != nil {
				name = *pol.ItemName
			}
			assetHandoffLines = append(assetHandoffLines, GoodsReceivedLine{
				GRLineID:     lineID,
				POLineID:     pol.ID,
				POID:         po.ID,
				PONumber:     po.Number,
				VendorID:     po.VendorID,
				ItemID:       *pol.ItemID,
				ItemName:     name,
				QtyAccepted:  l.QtyAccepted,
				UnitPriceIDR: pol.UnitPriceIDR,
				ReceivedOn:   gr.ReceivedOn,
			})
		}
	}

	// AC-105-02: Call OnGoodsReceived hook for asset lines
	if len(assetHandoffLines) > 0 && OnGoodsReceived != nil {
		ev := GoodsReceivedEvent{
			SchoolID:       gr.SchoolID,
			GoodsReceiptID: gr.ID,
			GRNumber:       gr.Number,
			Lines:          assetHandoffLines,
		}
		if err := OnGoodsReceived(ctx, tx, ev); err != nil {
			return nil, fmt.Errorf("asset handoff failed: %w", err)
		}
		// Mark asset_handoff_at = now()
		for _, al := range assetHandoffLines {
			_, _ = tx.ExecContext(ctx, `UPDATE goods_receipt_lines SET asset_handoff_at = now() WHERE id = $1`, al.GRLineID)
		}
	}

	// Re-calculate PO status
	var poReceiptLines []POLineReceiptCalc
	for _, pol := range po.Lines {
		var acceptedNet int64
		_ = tx.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(qty_accepted), 0) FROM goods_receipt_lines WHERE po_line_id = $1
		`, pol.ID).Scan(&acceptedNet)
		poReceiptLines = append(poReceiptLines, POLineReceiptCalc{
			POLineID:       pol.ID,
			QtyOrdered:     pol.Qty,
			QtyAcceptedNet: acceptedNet,
		})
	}
	newStatus, _ := POReceiptStatus(poReceiptLines)
	_, err = tx.ExecContext(ctx, `UPDATE purchase_orders SET status = $1, updated_at = now() WHERE id = $2`, newStatus, po.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &gr, nil
}

func (s *Store) ListReceipts(ctx context.Context, schoolID, poID string) ([]GoodsReceipt, error) {
	query := `
		SELECT gr.id, gr.school_id, gr.number, gr.po_id, po.number, gr.received_on, gr.received_by, gr.note, gr.created_at
		FROM goods_receipts gr
		JOIN purchase_orders po ON po.id = gr.po_id
		WHERE gr.school_id = $1
	`
	args := []any{schoolID}
	if poID != "" {
		args = append(args, poID)
		query += fmt.Sprintf(" AND gr.po_id = $%d", len(args))
	}
	query += " ORDER BY gr.received_on DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []GoodsReceipt
	for rows.Next() {
		var gr GoodsReceipt
		if err := rows.Scan(&gr.ID, &gr.SchoolID, &gr.Number, &gr.POID, &gr.PONumber, &gr.ReceivedOn, &gr.ReceivedBy, &gr.Note, &gr.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, gr)
	}
	return list, rows.Err()
}

func (s *Store) CreateReturn(ctx context.Context, ret GoodsReturn) (*GoodsReturn, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Validate return only on tracking='stock' items
	var poLineID string
	var poID string
	var itemID *string
	err = tx.QueryRowContext(ctx, `
		SELECT grl.po_line_id, pol.po_id, pol.item_id
		FROM goods_receipt_lines grl
		JOIN purchase_order_lines pol ON pol.id = grl.po_line_id
		WHERE grl.id = $1 AND grl.school_id = $2
	`, ret.GRLineID, ret.SchoolID).Scan(&poLineID, &poID, &itemID)
	if err != nil {
		return nil, fmt.Errorf("receipt line not found: %w", err)
	}

	if itemID != nil {
		var tracking string
		_ = tx.QueryRowContext(ctx, `SELECT c.tracking FROM items i JOIN item_categories c ON c.id = i.category_id WHERE i.id = $1`, *itemID).Scan(&tracking)
		if tracking != "stock" {
			return nil, errors.New("retur setelah penerimaan hanya diizinkan untuk barang stok")
		}
	}

	var id string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO goods_returns (school_id, gr_line_id, qty, reason, returned_on, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`, ret.SchoolID, ret.GRLineID, ret.Qty, ret.Reason, ret.ReturnedOn, ret.CreatedBy).Scan(&id, &createdAt)
	if err != nil {
		return nil, err
	}
	ret.ID = id
	ret.CreatedAt = createdAt

	// Re-evaluate PO receipt status: returning item might revert PO from 'received' to 'partially_received'
	var poLines []POLineReceiptCalc
	pRows, _ := tx.QueryContext(ctx, `
		SELECT pol.id, pol.qty,
		       COALESCE(acc.qty_acc, 0) - COALESCE(ret.qty_ret, 0) as net_acc
		FROM purchase_order_lines pol
		LEFT JOIN (SELECT po_line_id, SUM(qty_accepted) as qty_acc FROM goods_receipt_lines GROUP BY po_line_id) acc ON acc.po_line_id = pol.id
		LEFT JOIN (SELECT grl.po_line_id, SUM(grt.qty) as qty_ret FROM goods_returns grt JOIN goods_receipt_lines grl ON grl.id = grt.gr_line_id GROUP BY grl.po_line_id) ret ON ret.po_line_id = pol.id
		WHERE pol.po_id = $1
	`, poID)
	if pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var pl POLineReceiptCalc
			_ = pRows.Scan(&pl.POLineID, &pl.QtyOrdered, &pl.QtyAcceptedNet)
			poLines = append(poLines, pl)
		}
	}
	newStatus, _ := POReceiptStatus(poLines)
	_, _ = tx.ExecContext(ctx, `UPDATE purchase_orders SET status = $1, updated_at = now() WHERE id = $2`, newStatus, poID)

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ret, nil
}

func (s *Store) CreateEvaluation(ctx context.Context, eval VendorEvaluation) (*VendorEvaluation, error) {
	// Calculate on_time from PO expected_date and GR received_on
	var expectedDate *time.Time
	var receivedOn time.Time
	_ = s.db.QueryRowContext(ctx, `
		SELECT po.expected_date, gr.received_on
		FROM goods_receipts gr
		JOIN purchase_orders po ON po.id = gr.po_id
		WHERE gr.id = $1
	`, eval.GoodsReceiptID).Scan(&expectedDate, &receivedOn)

	eval.OnTime = OnTime(expectedDate, receivedOn)

	var id string
	var createdAt time.Time
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO vendor_evaluations (school_id, vendor_id, goods_receipt_id, on_time, quality_score, price_score, note, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`, eval.SchoolID, eval.VendorID, eval.GoodsReceiptID, eval.OnTime, eval.QualityScore, eval.PriceScore, eval.Note, eval.CreatedBy).Scan(&id, &createdAt)
	if err != nil {
		return nil, err
	}
	eval.ID = id
	eval.CreatedAt = createdAt
	return &eval, nil
}

func (s *Store) ListEvaluations(ctx context.Context, schoolID, vendorID string) ([]VendorEvaluation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, vendor_id, goods_receipt_id, on_time, quality_score, price_score, note, created_by, created_at
		FROM vendor_evaluations
		WHERE school_id = $1 AND vendor_id = $2
		ORDER BY created_at DESC
	`, schoolID, vendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []VendorEvaluation
	for rows.Next() {
		var e VendorEvaluation
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.VendorID, &e.GoodsReceiptID, &e.OnTime, &e.QualityScore, &e.PriceScore, &e.Note, &e.CreatedBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}
