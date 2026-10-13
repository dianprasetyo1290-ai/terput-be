package payable

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Store struct {
	db  *sql.DB
	acc AccountingClient
}

func NewStore(db *sql.DB, acc AccountingClient) *Store {
	return &Store{db: db, acc: acc}
}

func (s *Store) GetSettings(ctx context.Context, schoolID string) (*PayableSettings, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT school_id, default_ap_acc_code, default_vat_in_acc_code, default_tax_payable_acc_code,
		       default_bank_acc_code, tolerance_amount_idr, auto_journal_on_invoice, auto_journal_on_payment, updated_at
		FROM payable_settings
		WHERE school_id = $1
	`, schoolID)

	var st PayableSettings
	err := row.Scan(&st.SchoolID, &st.DefaultAPAccCode, &st.DefaultVATInAccCode, &st.DefaultTaxPayableAccCode,
		&st.DefaultBankAccCode, &st.ToleranceAmountIDR, &st.AutoJournalOnInvoice, &st.AutoJournalOnPayment, &st.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &PayableSettings{
			SchoolID:                 schoolID,
			DefaultAPAccCode:         "2101",
			DefaultVATInAccCode:      "1181",
			DefaultTaxPayableAccCode: "2131",
			DefaultBankAccCode:       "1102",
			ToleranceAmountIDR:       0,
			AutoJournalOnInvoice:     true,
			AutoJournalOnPayment:     true,
			UpdatedAt:                time.Now(),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) UpdateSettings(ctx context.Context, st *PayableSettings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payable_settings (school_id, default_ap_acc_code, default_vat_in_acc_code, default_tax_payable_acc_code,
		                             default_bank_acc_code, tolerance_amount_idr, auto_journal_on_invoice, auto_journal_on_payment, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (school_id) DO UPDATE SET
			default_ap_acc_code = EXCLUDED.default_ap_acc_code,
			default_vat_in_acc_code = EXCLUDED.default_vat_in_acc_code,
			default_tax_payable_acc_code = EXCLUDED.default_tax_payable_acc_code,
			default_bank_acc_code = EXCLUDED.default_bank_acc_code,
			tolerance_amount_idr = EXCLUDED.tolerance_amount_idr,
			auto_journal_on_invoice = EXCLUDED.auto_journal_on_invoice,
			auto_journal_on_payment = EXCLUDED.auto_journal_on_payment,
			updated_at = now()
	`, st.SchoolID, st.DefaultAPAccCode, st.DefaultVATInAccCode, st.DefaultTaxPayableAccCode,
		st.DefaultBankAccCode, st.ToleranceAmountIDR, st.AutoJournalOnInvoice, st.AutoJournalOnPayment)
	return err
}

func (s *Store) ListInvoices(ctx context.Context, schoolID, status, vendorID string) ([]VendorInvoice, error) {
	query := `
		SELECT vi.id, vi.school_id, vi.number, vi.vendor_id, v.name as vendor_name,
		       vi.po_id, po.number as po_number, vi.invoice_date, vi.due_date,
		       vi.subtotal_idr, vi.vat_idr, vi.withholding_tax_idr, vi.total_idr, vi.paid_amount_idr,
		       vi.status, vi.match_status, vi.match_discrepancy_note, vi.override_approval_id,
		       vi.pdf_object_key, vi.journal_trans_no, vi.idempotency_key, vi.created_by,
		       vi.created_at, vi.updated_at
		FROM vendor_invoices vi
		JOIN vendors v ON v.id = vi.vendor_id
		LEFT JOIN purchase_orders po ON po.id = vi.po_id
		WHERE vi.school_id = $1 AND vi.deleted_at IS NULL
	`
	args := []any{schoolID}
	if status != "" {
		query += fmt.Sprintf(" AND vi.status = $%d", len(args)+1)
		args = append(args, status)
	}
	if vendorID != "" {
		query += fmt.Sprintf(" AND vi.vendor_id = $%d", len(args)+1)
		args = append(args, vendorID)
	}
	query += " ORDER BY vi.invoice_date DESC, vi.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []VendorInvoice
	for rows.Next() {
		var vi VendorInvoice
		var invDate, dueDate time.Time
		if err := rows.Scan(&vi.ID, &vi.SchoolID, &vi.Number, &vi.VendorID, &vi.VendorName,
			&vi.POID, &vi.PONumber, &invDate, &dueDate,
			&vi.SubtotalIDR, &vi.VATIDR, &vi.WithholdingTaxIDR, &vi.TotalIDR, &vi.PaidAmountIDR,
			&vi.Status, &vi.MatchStatus, &vi.MatchDiscrepancyNote, &vi.OverrideApprovalID,
			&vi.PDFObjectKey, &vi.JournalTransNo, &vi.IdempotencyKey, &vi.CreatedBy,
			&vi.CreatedAt, &vi.UpdatedAt); err != nil {
			return nil, err
		}
		vi.InvoiceDate = invDate.Format("2006-01-02")
		vi.DueDate = dueDate.Format("2006-01-02")
		list = append(list, vi)
	}
	return list, nil
}

func (s *Store) GetInvoice(ctx context.Context, schoolID, id string) (*VendorInvoice, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT vi.id, vi.school_id, vi.number, vi.vendor_id, v.name as vendor_name,
		       vi.po_id, po.number as po_number, vi.invoice_date, vi.due_date,
		       vi.subtotal_idr, vi.vat_idr, vi.withholding_tax_idr, vi.total_idr, vi.paid_amount_idr,
		       vi.status, vi.match_status, vi.match_discrepancy_note, vi.override_approval_id,
		       vi.pdf_object_key, vi.journal_trans_no, vi.idempotency_key, vi.created_by,
		       vi.created_at, vi.updated_at
		FROM vendor_invoices vi
		JOIN vendors v ON v.id = vi.vendor_id
		LEFT JOIN purchase_orders po ON po.id = vi.po_id
		WHERE vi.school_id = $1 AND vi.id = $2 AND vi.deleted_at IS NULL
	`, schoolID, id)

	var vi VendorInvoice
	var invDate, dueDate time.Time
	err := row.Scan(&vi.ID, &vi.SchoolID, &vi.Number, &vi.VendorID, &vi.VendorName,
		&vi.POID, &vi.PONumber, &invDate, &dueDate,
		&vi.SubtotalIDR, &vi.VATIDR, &vi.WithholdingTaxIDR, &vi.TotalIDR, &vi.PaidAmountIDR,
		&vi.Status, &vi.MatchStatus, &vi.MatchDiscrepancyNote, &vi.OverrideApprovalID,
		&vi.PDFObjectKey, &vi.JournalTransNo, &vi.IdempotencyKey, &vi.CreatedBy,
		&vi.CreatedAt, &vi.UpdatedAt)
	if err != nil {
		return nil, err
	}
	vi.InvoiceDate = invDate.Format("2006-01-02")
	vi.DueDate = dueDate.Format("2006-01-02")

	// Get lines
	linesRows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, invoice_id, po_line_id, gr_line_id, description, qty, unit_price_idr, amount_idr,
		       withholding_type_id, withholding_tax_idr, budget_line_key, created_at
		FROM vendor_invoice_lines
		WHERE school_id = $1 AND invoice_id = $2
	`, schoolID, id)
	if err == nil {
		defer linesRows.Close()
		for linesRows.Next() {
			var l VendorInvoiceLine
			if err := linesRows.Scan(&l.ID, &l.SchoolID, &l.InvoiceID, &l.POLineID, &l.GRLineID, &l.Description,
				&l.Qty, &l.UnitPriceIDR, &l.AmountIDR, &l.WithholdingTypeID, &l.WithholdingTaxIDR, &l.BudgetLineKey, &l.CreatedAt); err == nil {
				vi.Lines = append(vi.Lines, l)
			}
		}
	}

	return &vi, nil
}

func (s *Store) CreateInvoice(ctx context.Context, inv *VendorInvoice) (*VendorInvoice, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if inv.IdempotencyKey == "" {
		inv.IdempotencyKey = fmt.Sprintf("inv:%s:%s", inv.VendorID, inv.Number)
	}

	// Calculate subtotal and total
	var subtotal, whtTot int64
	for _, l := range inv.Lines {
		subtotal += l.Qty * l.UnitPriceIDR
		whtTot += l.WithholdingTaxIDR
	}
	inv.SubtotalIDR = subtotal
	inv.WithholdingTaxIDR = whtTot
	inv.TotalIDR = inv.SubtotalIDR + inv.VATIDR - inv.WithholdingTaxIDR

	row := tx.QueryRowContext(ctx, `
		INSERT INTO vendor_invoices (school_id, number, vendor_id, po_id, invoice_date, due_date,
		                            subtotal_idr, vat_idr, withholding_tax_idr, total_idr, paid_amount_idr,
		                            status, match_status, idempotency_key, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0, 'draft', 'pending', $11, $12)
		RETURNING id, created_at, updated_at
	`, inv.SchoolID, inv.Number, inv.VendorID, inv.POID, inv.InvoiceDate, inv.DueDate,
		inv.SubtotalIDR, inv.VATIDR, inv.WithholdingTaxIDR, inv.TotalIDR, inv.IdempotencyKey, inv.CreatedBy)

	if err := row.Scan(&inv.ID, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
		return nil, err
	}

	for i := range inv.Lines {
		line := &inv.Lines[i]
		line.SchoolID = inv.SchoolID
		line.InvoiceID = inv.ID
		line.AmountIDR = line.Qty * line.UnitPriceIDR
		lRow := tx.QueryRowContext(ctx, `
			INSERT INTO vendor_invoice_lines (school_id, invoice_id, po_line_id, gr_line_id, description,
			                                 qty, unit_price_idr, withholding_type_id, withholding_tax_idr, budget_line_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, created_at
		`, line.SchoolID, line.InvoiceID, line.POLineID, line.GRLineID, line.Description,
			line.Qty, line.UnitPriceIDR, line.WithholdingTypeID, line.WithholdingTaxIDR, line.BudgetLineKey)
		if err := lRow.Scan(&line.ID, &line.CreatedAt); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	inv.Status = "draft"
	inv.MatchStatus = "pending"
	return inv, nil
}

func (s *Store) RunThreeWayMatch(ctx context.Context, schoolID, invoiceID string) (*MatchResult, error) {
	inv, err := s.GetInvoice(ctx, schoolID, invoiceID)
	if err != nil {
		return nil, err
	}
	st, err := s.GetSettings(ctx, schoolID)
	if err != nil {
		return nil, err
	}

	var matchInputs []MatchInputLine
	for _, l := range inv.Lines {
		mi := MatchInputLine{
			Description:      l.Description,
			QtyInvoice:       l.Qty,
			UnitPriceInvoice: l.UnitPriceIDR,
		}
		if l.POLineID != nil && *l.POLineID != "" {
			mi.POLineID = *l.POLineID
			// Query po_line_match_v
			var qtyAcceptedNet int64
			var unitPricePO int64
			err := s.db.QueryRowContext(ctx, `
				SELECT COALESCE(qty_accepted_net, 0), unit_price_idr
				FROM po_line_match_v
				WHERE po_line_id = $1 AND school_id = $2
			`, *l.POLineID, schoolID).Scan(&qtyAcceptedNet, &unitPricePO)
			if err == nil {
				mi.QtyAcceptedGR = qtyAcceptedNet
				mi.UnitPricePO = unitPricePO
			}
		} else {
			// Direct invoice without PO
			mi.QtyAcceptedGR = l.Qty
			mi.UnitPricePO = l.UnitPriceIDR
		}
		matchInputs = append(matchInputs, mi)
	}

	res := ThreeWayMatch(matchInputs, st.ToleranceAmountIDR)

	newStatus := "matched"
	newMatchStatus := "matched_ok"
	if !res.OK {
		newStatus = "discrepancy"
		newMatchStatus = "discrepancy"
	}

	var discNote *string
	if res.DiscrepancyNote != "" {
		discNote = &res.DiscrepancyNote
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE vendor_invoices
		SET status = $3, match_status = $4, match_discrepancy_note = $5, updated_at = now()
		WHERE school_id = $1 AND id = $2
	`, schoolID, invoiceID, newStatus, newMatchStatus, discNote)
	if err != nil {
		return nil, err
	}

	// If matched and auto journal is on, post journal to accounting engine
	if res.OK && st.AutoJournalOnInvoice && inv.JournalTransNo == nil {
		jPayload := BuildInvoiceJournal(
			schoolID, inv.Number, inv.InvoiceDate, inv.VendorID,
			inv.SubtotalIDR, inv.VATIDR, inv.WithholdingTaxIDR, inv.TotalIDR,
			st.DefaultAPAccCode, st.DefaultVATInAccCode, "5101", st.DefaultTaxPayableAccCode,
		)
		transNo, jErr := s.acc.PostJournal(ctx, jPayload)
		if jErr == nil && transNo != "" {
			_, _ = s.db.ExecContext(ctx, `
				UPDATE vendor_invoices SET journal_trans_no = $3, updated_at = now()
				WHERE school_id = $1 AND id = $2
			`, schoolID, invoiceID, transNo)

			_, _ = s.db.ExecContext(ctx, `
				INSERT INTO payable_journals (school_id, source_type, source_id, trans_no, trans_date, amount_idr, description, status)
				VALUES ($1, 'invoice', $2, $3, $4, $5, $6, 'posted')
			`, schoolID, invoiceID, transNo, inv.InvoiceDate, inv.TotalIDR, "Faktur Vendor: "+inv.Number)
		}
	}

	return &res, nil
}

func (s *Store) RequestMatchOverride(ctx context.Context, schoolID, invoiceID, approvalID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE vendor_invoices
		SET status = 'override_needed', override_approval_id = $3, updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status = 'discrepancy'
	`, schoolID, invoiceID, approvalID)
	return err
}

func (s *Store) ApplyOverrideDecision(ctx context.Context, schoolID, invoiceID, decision string) error {
	inv, err := s.GetInvoice(ctx, schoolID, invoiceID)
	if err != nil {
		return err
	}
	st, err := s.GetSettings(ctx, schoolID)
	if err != nil {
		return err
	}

	if decision == "approved" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE vendor_invoices
			SET status = 'override_approved', updated_at = now()
			WHERE school_id = $1 AND id = $2
		`, schoolID, invoiceID)
		if err != nil {
			return err
		}

		if st.AutoJournalOnInvoice && inv.JournalTransNo == nil {
			jPayload := BuildInvoiceJournal(
				schoolID, inv.Number, inv.InvoiceDate, inv.VendorID,
				inv.SubtotalIDR, inv.VATIDR, inv.WithholdingTaxIDR, inv.TotalIDR,
				st.DefaultAPAccCode, st.DefaultVATInAccCode, "5101", st.DefaultTaxPayableAccCode,
			)
			transNo, jErr := s.acc.PostJournal(ctx, jPayload)
			if jErr == nil && transNo != "" {
				_, _ = s.db.ExecContext(ctx, `
					UPDATE vendor_invoices SET journal_trans_no = $3, updated_at = now()
					WHERE school_id = $1 AND id = $2
				`, schoolID, invoiceID, transNo)
			}
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
			UPDATE vendor_invoices
			SET status = 'discrepancy', updated_at = now()
			WHERE school_id = $1 AND id = $2
		`, schoolID, invoiceID)
		return err
	}
	return nil
}

func (s *Store) ListPaymentSchedules(ctx context.Context, schoolID, invoiceID, status string) ([]PaymentSchedule, error) {
	query := `
		SELECT ps.id, ps.school_id, ps.invoice_id, vi.number as invoice_number,
		       ps.vendor_id, v.name as vendor_name, ps.scheduled_date, ps.amount_idr,
		       ps.payment_method, ps.source_bank_acc_code, ps.vendor_bank_account_id,
		       ps.status, ps.approval_id, ps.executed_at, ps.executed_by, ps.reference_no,
		       ps.journal_trans_no, ps.created_by, ps.created_at, ps.updated_at
		FROM payment_schedules ps
		JOIN vendor_invoices vi ON vi.id = ps.invoice_id
		JOIN vendors v ON v.id = ps.vendor_id
		WHERE ps.school_id = $1
	`
	args := []any{schoolID}
	if invoiceID != "" {
		query += fmt.Sprintf(" AND ps.invoice_id = $%d", len(args)+1)
		args = append(args, invoiceID)
	}
	if status != "" {
		query += fmt.Sprintf(" AND ps.status = $%d", len(args)+1)
		args = append(args, status)
	}
	query += " ORDER BY ps.scheduled_date ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PaymentSchedule
	for rows.Next() {
		var ps PaymentSchedule
		var schDate time.Time
		if err := rows.Scan(&ps.ID, &ps.SchoolID, &ps.InvoiceID, &ps.InvoiceNumber,
			&ps.VendorID, &ps.VendorName, &schDate, &ps.AmountIDR,
			&ps.PaymentMethod, &ps.SourceBankAccCode, &ps.VendorBankAccountID,
			&ps.Status, &ps.ApprovalID, &ps.ExecutedAt, &ps.ExecutedBy, &ps.ReferenceNo,
			&ps.JournalTransNo, &ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
			return nil, err
		}
		ps.ScheduledDate = schDate.Format("2006-01-02")
		list = append(list, ps)
	}
	return list, nil
}

func (s *Store) CreatePaymentSchedule(ctx context.Context, ps *PaymentSchedule) (*PaymentSchedule, error) {
	inv, err := s.GetInvoice(ctx, ps.SchoolID, ps.InvoiceID)
	if err != nil {
		return nil, err
	}

	// Verify available unpaid balance
	if !CanSchedule(inv.TotalIDR, inv.PaidAmountIDR, ps.AmountIDR) {
		return nil, fmt.Errorf("jumlah jadwal pembayaran (Rp %d) melebihi sisa tagihan (Rp %d)", ps.AmountIDR, inv.TotalIDR-inv.PaidAmountIDR)
	}

	ps.VendorID = inv.VendorID
	if ps.PaymentMethod == "" {
		ps.PaymentMethod = "bank_transfer"
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO payment_schedules (school_id, invoice_id, vendor_id, scheduled_date, amount_idr,
		                              payment_method, source_bank_acc_code, vendor_bank_account_id,
		                              status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'scheduled', $9)
		RETURNING id, created_at, updated_at
	`, ps.SchoolID, ps.InvoiceID, ps.VendorID, ps.ScheduledDate, ps.AmountIDR,
		ps.PaymentMethod, ps.SourceBankAccCode, ps.VendorBankAccountID, ps.CreatedBy)

	if err := row.Scan(&ps.ID, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
		return nil, err
	}
	ps.Status = "scheduled"
	return ps, nil
}

func (s *Store) SubmitPaymentSchedule(ctx context.Context, schoolID, id, approvalID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE payment_schedules
		SET status = 'pending_approval', approval_id = $3, updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status = 'scheduled'
	`, schoolID, id, approvalID)
	return err
}

func (s *Store) ApprovePaymentSchedule(ctx context.Context, schoolID, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE payment_schedules
		SET status = 'approved', updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status = 'pending_approval'
	`, schoolID, id)
	return err
}

func (s *Store) ExecutePaymentSchedule(ctx context.Context, schoolID, id, executorID, referenceNo string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var invoiceID string
	var vendorID string
	var amountIDR int64
	var status string
	var sourceBank *string

	err = tx.QueryRowContext(ctx, `
		SELECT invoice_id, vendor_id, amount_idr, status, source_bank_acc_code
		FROM payment_schedules
		WHERE school_id = $1 AND id = $2
	`, schoolID, id).Scan(&invoiceID, &vendorID, &amountIDR, &status, &sourceBank)
	if err != nil {
		return err
	}
	if status != "approved" && status != "scheduled" {
		return fmt.Errorf("jadwal pembayaran tidak dalam status approved atau scheduled")
	}

	st, err := s.GetSettings(ctx, schoolID)
	if err != nil {
		return err
	}
	bankAcc := st.DefaultBankAccCode
	if sourceBank != nil && *sourceBank != "" {
		bankAcc = *sourceBank
	}

	var journalTransNo *string
	today := time.Now().Format("2006-01-02")

	if st.AutoJournalOnPayment {
		ref := referenceNo
		if ref == "" {
			ref = id
		}
		jPayload := BuildPaymentJournal(schoolID, ref, today, vendorID, amountIDR, st.DefaultAPAccCode, bankAcc)
		transNo, jErr := s.acc.PostJournal(ctx, jPayload)
		if jErr == nil && transNo != "" {
			journalTransNo = &transNo
			_, _ = tx.ExecContext(ctx, `
				INSERT INTO payable_journals (school_id, source_type, source_id, trans_no, trans_date, amount_idr, description, status)
				VALUES ($1, 'payment', $2, $3, $4, $5, $6, 'posted')
			`, schoolID, id, transNo, today, amountIDR, "Pembayaran Utang: "+ref)
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE payment_schedules
		SET status = 'executed', executed_at = now(), executed_by = $3, reference_no = $4, journal_trans_no = $5, updated_at = now()
		WHERE school_id = $1 AND id = $2
	`, schoolID, id, executorID, referenceNo, journalTransNo)
	if err != nil {
		return err
	}

	// Update invoice paid amount and status
	var newPaid int64
	var totalIDR int64
	err = tx.QueryRowContext(ctx, `
		UPDATE vendor_invoices
		SET paid_amount_idr = paid_amount_idr + $3,
		    updated_at = now()
		WHERE school_id = $1 AND id = $2
		RETURNING paid_amount_idr, total_idr
	`, schoolID, invoiceID, amountIDR).Scan(&newPaid, &totalIDR)
	if err != nil {
		return err
	}

	invStatus := "partially_paid"
	if newPaid >= totalIDR {
		invStatus = "paid"
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE vendor_invoices SET status = $3, updated_at = now()
		WHERE school_id = $1 AND id = $2
	`, schoolID, invoiceID, invStatus)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) GetAgingReport(ctx context.Context, schoolID string) (*AgingReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT vi.id, vi.number, vi.vendor_id, v.name as vendor_name, vi.due_date, vi.total_idr, vi.paid_amount_idr
		FROM vendor_invoices vi
		JOIN vendors v ON v.id = vi.vendor_id
		WHERE vi.school_id = $1 AND vi.status IN ('matched', 'override_approved', 'approved', 'partially_paid')
		ORDER BY vi.due_date ASC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	rep := &AgingReport{
		AsOf:  now.Format("2006-01-02"),
		Items: []AgingBucketItem{},
	}

	for rows.Next() {
		var item AgingBucketItem
		var dueDate time.Time
		if err := rows.Scan(&item.InvoiceID, &item.InvoiceNumber, &item.VendorID, &item.VendorName,
			&dueDate, &item.TotalIDR, &item.OutstandingIDR); err != nil {
			return nil, err
		}
		item.DueDate = dueDate.Format("2006-01-02")
		item.OutstandingIDR = item.TotalIDR - item.OutstandingIDR

		bucket, days := AgingBucket(dueDate, now)
		item.DaysOverdue = days

		rep.TotalAPIDR += item.OutstandingIDR
		switch bucket {
		case "current":
			rep.CurrentIDR += item.OutstandingIDR
		case "1_30":
			rep.Overdue1to30 += item.OutstandingIDR
		case "31_60":
			rep.Overdue31to60 += item.OutstandingIDR
		case "61_90":
			rep.Overdue61to90 += item.OutstandingIDR
		case "90_plus":
			rep.Overdue90Plus += item.OutstandingIDR
		}

		rep.Items = append(rep.Items, item)
	}

	return rep, nil
}

func (s *Store) ListPayableJournals(ctx context.Context, schoolID string) ([]PayableJournal, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, source_type, source_id, trans_no, trans_date, amount_idr, description, status, error_message, created_at
		FROM payable_journals
		WHERE school_id = $1
		ORDER BY created_at DESC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PayableJournal
	for rows.Next() {
		var pj PayableJournal
		var transDate time.Time
		if err := rows.Scan(&pj.ID, &pj.SchoolID, &pj.SourceType, &pj.SourceID, &pj.TransNo, &transDate,
			&pj.AmountIDR, &pj.Description, &pj.Status, &pj.ErrorMessage, &pj.CreatedAt); err != nil {
			return nil, err
		}
		pj.TransDate = transDate.Format("2006-01-02")
		list = append(list, pj)
	}
	return list, nil
}
