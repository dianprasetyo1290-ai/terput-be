package tax

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) GetSettings(ctx context.Context, schoolID string) (*TaxSettings, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT school_id, npwp_masked, nitku, efaktur_enabled, default_ppn_bp, updated_at
		FROM tax_settings
		WHERE school_id = $1
	`, schoolID)

	var st TaxSettings
	err := row.Scan(&st.SchoolID, &st.NPWPMasked, &st.NITKU, &st.EfakturEnabled, &st.DefaultPPNBP, &st.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &TaxSettings{
			SchoolID:       schoolID,
			EfakturEnabled: false,
			DefaultPPNBP:   1100,
			UpdatedAt:      time.Now(),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) UpdateSettings(ctx context.Context, st *TaxSettings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tax_settings (school_id, npwp_masked, nitku, efaktur_enabled, default_ppn_bp, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (school_id) DO UPDATE SET
			npwp_masked = EXCLUDED.npwp_masked,
			nitku = EXCLUDED.nitku,
			efaktur_enabled = EXCLUDED.efaktur_enabled,
			default_ppn_bp = EXCLUDED.default_ppn_bp,
			updated_at = now()
	`, st.SchoolID, st.NPWPMasked, st.NITKU, st.EfakturEnabled, st.DefaultPPNBP)
	return err
}

func (s *Store) ListTaxTypes(ctx context.Context, schoolID string) ([]WithholdingTaxType, error) {
	// Auto seed if empty
	var count int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM withholding_tax_types WHERE school_id = $1`, schoolID).Scan(&count)
	if count == 0 {
		_, _ = s.db.ExecContext(ctx, `SELECT seed_default_tax_types($1)`, schoolID)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, code, name, article, standard_rate_bp, no_npwp_rate_bp, description, is_active, created_at, updated_at
		FROM withholding_tax_types
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY article ASC, code ASC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WithholdingTaxType
	for rows.Next() {
		var t WithholdingTaxType
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.Code, &t.Name, &t.Article, &t.StandardRateBP, &t.NoNPWPRateBP, &t.Description, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

func (s *Store) GetTaxType(ctx context.Context, schoolID, id string) (*WithholdingTaxType, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, code, name, article, standard_rate_bp, no_npwp_rate_bp, description, is_active, created_at, updated_at
		FROM withholding_tax_types
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)

	var t WithholdingTaxType
	if err := row.Scan(&t.ID, &t.SchoolID, &t.Code, &t.Name, &t.Article, &t.StandardRateBP, &t.NoNPWPRateBP, &t.Description, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) CreateTaxType(ctx context.Context, t *WithholdingTaxType) (*WithholdingTaxType, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO withholding_tax_types (school_id, code, name, article, standard_rate_bp, no_npwp_rate_bp, description, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, t.SchoolID, t.Code, t.Name, t.Article, t.StandardRateBP, t.NoNPWPRateBP, t.Description, t.IsActive)

	if err := row.Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) UpdateTaxType(ctx context.Context, t *WithholdingTaxType) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE withholding_tax_types
		SET code = $3, name = $4, article = $5, standard_rate_bp = $6, no_npwp_rate_bp = $7, description = $8, is_active = $9, updated_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, t.SchoolID, t.ID, t.Code, t.Name, t.Article, t.StandardRateBP, t.NoNPWPRateBP, t.Description, t.IsActive)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tax type not found")
	}
	return nil
}

func (s *Store) ListWithholdingSlips(ctx context.Context, schoolID string, period string, status string, vendorID string) ([]WithholdingSlip, error) {
	query := `
		SELECT ws.id, ws.school_id, ws.slip_number, ws.type_id, tt.name as type_name, tt.article as type_article,
		       ws.vendor_id, v.name as vendor_name, ws.vendor_invoice_id, ws.tax_period,
		       ws.gross_amount_idr, ws.rate_bp, ws.tax_amount_idr, ws.npwp_used,
		       ws.status, ws.issued_at, ws.reported_at, ws.pdf_object_key, ws.created_by, ws.created_at, ws.updated_at
		FROM withholding_slips ws
		JOIN withholding_tax_types tt ON tt.id = ws.type_id
		JOIN vendors v ON v.id = ws.vendor_id
		WHERE ws.school_id = $1
	`
	args := []any{schoolID}
	if period != "" {
		query += fmt.Sprintf(" AND ws.tax_period = $%d", len(args)+1)
		args = append(args, period)
	}
	if status != "" {
		query += fmt.Sprintf(" AND ws.status = $%d", len(args)+1)
		args = append(args, status)
	}
	if vendorID != "" {
		query += fmt.Sprintf(" AND ws.vendor_id = $%d", len(args)+1)
		args = append(args, vendorID)
	}
	query += " ORDER BY ws.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WithholdingSlip
	for rows.Next() {
		var ws WithholdingSlip
		if err := rows.Scan(&ws.ID, &ws.SchoolID, &ws.SlipNumber, &ws.TypeID, &ws.TypeName, &ws.TypeArticle,
			&ws.VendorID, &ws.VendorName, &ws.VendorInvoiceID, &ws.TaxPeriod,
			&ws.GrossAmountIDR, &ws.RateBP, &ws.TaxAmountIDR, &ws.NPWPUsed,
			&ws.Status, &ws.IssuedAt, &ws.ReportedAt, &ws.PDFObjectKey, &ws.CreatedBy, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, ws)
	}
	return list, nil
}

func (s *Store) GetWithholdingSlip(ctx context.Context, schoolID, id string) (*WithholdingSlip, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT ws.id, ws.school_id, ws.slip_number, ws.type_id, tt.name as type_name, tt.article as type_article,
		       ws.vendor_id, v.name as vendor_name, ws.vendor_invoice_id, ws.tax_period,
		       ws.gross_amount_idr, ws.rate_bp, ws.tax_amount_idr, ws.npwp_used,
		       ws.status, ws.issued_at, ws.reported_at, ws.pdf_object_key, ws.created_by, ws.created_at, ws.updated_at
		FROM withholding_slips ws
		JOIN withholding_tax_types tt ON tt.id = ws.type_id
		JOIN vendors v ON v.id = ws.vendor_id
		WHERE ws.school_id = $1 AND ws.id = $2
	`, schoolID, id)

	var ws WithholdingSlip
	if err := row.Scan(&ws.ID, &ws.SchoolID, &ws.SlipNumber, &ws.TypeID, &ws.TypeName, &ws.TypeArticle,
		&ws.VendorID, &ws.VendorName, &ws.VendorInvoiceID, &ws.TaxPeriod,
		&ws.GrossAmountIDR, &ws.RateBP, &ws.TaxAmountIDR, &ws.NPWPUsed,
		&ws.Status, &ws.IssuedAt, &ws.ReportedAt, &ws.PDFObjectKey, &ws.CreatedBy, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
		return nil, err
	}
	return &ws, nil
}

func (s *Store) CreateWithholdingSlip(ctx context.Context, ws *WithholdingSlip) (*WithholdingSlip, error) {
	// Auto calculate tax amount if not provided
	if ws.TaxAmountIDR == 0 && ws.GrossAmountIDR > 0 && ws.RateBP > 0 {
		ws.TaxAmountIDR = WithholdingAmount(ws.GrossAmountIDR, ws.RateBP)
	}

	// Generate slip number if empty
	if ws.SlipNumber == "" {
		var seq int
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) + 1 FROM withholding_slips WHERE school_id = $1 AND tax_period = $2`, ws.SchoolID, ws.TaxPeriod).Scan(&seq)
		ws.SlipNumber = fmt.Sprintf("BUPOT/%s/%04d", ws.TaxPeriod, seq)
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO withholding_slips (school_id, slip_number, type_id, vendor_id, vendor_invoice_id, tax_period,
		                               gross_amount_idr, rate_bp, tax_amount_idr, npwp_used, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'draft', $11)
		RETURNING id, created_at, updated_at
	`, ws.SchoolID, ws.SlipNumber, ws.TypeID, ws.VendorID, ws.VendorInvoiceID, ws.TaxPeriod,
		ws.GrossAmountIDR, ws.RateBP, ws.TaxAmountIDR, ws.NPWPUsed, ws.CreatedBy)

	if err := row.Scan(&ws.ID, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
		return nil, err
	}
	ws.Status = "draft"
	return ws, nil
}

func (s *Store) IssueWithholdingSlip(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE withholding_slips
		SET status = 'issued', issued_at = now(), updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status = 'draft'
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("slip not found or already issued")
	}
	return nil
}

func (s *Store) ListInputVAT(ctx context.Context, schoolID, period, status string) ([]InputVAT, error) {
	query := `
		SELECT iv.id, iv.school_id, iv.invoice_number, iv.vendor_id, v.name as vendor_name,
		       iv.vendor_invoice_id, iv.tax_period, iv.invoice_date, iv.dpp_idr, iv.vat_idr,
		       iv.creditable, iv.status, iv.created_at, iv.updated_at
		FROM input_vat iv
		JOIN vendors v ON v.id = iv.vendor_id
		WHERE iv.school_id = $1
	`
	args := []any{schoolID}
	if period != "" {
		query += fmt.Sprintf(" AND iv.tax_period = $%d", len(args)+1)
		args = append(args, period)
	}
	if status != "" {
		query += fmt.Sprintf(" AND iv.status = $%d", len(args)+1)
		args = append(args, status)
	}
	query += " ORDER BY iv.invoice_date DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []InputVAT
	for rows.Next() {
		var v InputVAT
		var invDate time.Time
		if err := rows.Scan(&v.ID, &v.SchoolID, &v.InvoiceNumber, &v.VendorID, &v.VendorName,
			&v.VendorInvoiceID, &v.TaxPeriod, &invDate, &v.DPPIDR, &v.VATIDR,
			&v.Creditable, &v.Status, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.InvoiceDate = invDate.Format("2006-01-02")
		list = append(list, v)
	}
	return list, nil
}

func (s *Store) CreateInputVAT(ctx context.Context, v *InputVAT) (*InputVAT, error) {
	if v.VATIDR == 0 && v.DPPIDR > 0 {
		v.VATIDR = CalculateVAT(v.DPPIDR, 1100)
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO input_vat (school_id, invoice_number, vendor_id, vendor_invoice_id, tax_period, invoice_date, dpp_idr, vat_idr, creditable, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'received')
		RETURNING id, created_at, updated_at
	`, v.SchoolID, v.InvoiceNumber, v.VendorID, v.VendorInvoiceID, v.TaxPeriod, v.InvoiceDate, v.DPPIDR, v.VATIDR, v.Creditable)

	if err := row.Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Status = "received"
	return v, nil
}

func (s *Store) UpdateInputVATStatus(ctx context.Context, schoolID, id, status string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE input_vat
		SET status = $3, updated_at = now()
		WHERE school_id = $1 AND id = $2
	`, schoolID, id, status)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("input vat not found")
	}
	return nil
}

func (s *Store) ListOutputTaxInvoices(ctx context.Context, schoolID, period string) ([]OutputTaxInvoice, error) {
	query := `
		SELECT id, school_id, invoice_number, customer_name, customer_npwp, tax_period, invoice_date, dpp_idr, vat_idr, status, created_at, updated_at
		FROM output_tax_invoices
		WHERE school_id = $1
	`
	args := []any{schoolID}
	if period != "" {
		query += " AND tax_period = $2"
		args = append(args, period)
	}
	query += " ORDER BY invoice_date DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []OutputTaxInvoice
	for rows.Next() {
		var o OutputTaxInvoice
		var invDate time.Time
		if err := rows.Scan(&o.ID, &o.SchoolID, &o.InvoiceNumber, &o.CustomerName, &o.CustomerNPWP, &o.TaxPeriod, &invDate, &o.DPPIDR, &o.VATIDR, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.InvoiceDate = invDate.Format("2006-01-02")
		list = append(list, o)
	}
	return list, nil
}

func (s *Store) CreateOutputTaxInvoice(ctx context.Context, o *OutputTaxInvoice) (*OutputTaxInvoice, error) {
	if o.VATIDR == 0 && o.DPPIDR > 0 {
		o.VATIDR = CalculateVAT(o.DPPIDR, 1100)
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO output_tax_invoices (school_id, invoice_number, customer_name, customer_npwp, tax_period, invoice_date, dpp_idr, vat_idr, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'draft')
		RETURNING id, created_at, updated_at
	`, o.SchoolID, o.InvoiceNumber, o.CustomerName, o.CustomerNPWP, o.TaxPeriod, o.InvoiceDate, o.DPPIDR, o.VATIDR)

	if err := row.Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	o.Status = "draft"
	return o, nil
}

func (s *Store) GenerateExport(ctx context.Context, schoolID, kind, taxPeriod, userID string) (*TaxExportTemplate, error) {
	tpl := &TaxExportTemplate{
		SchoolID:    schoolID,
		Kind:        kind,
		TaxPeriod:   taxPeriod,
		GeneratedBy: userID,
		CreatedAt:   time.Now(),
	}

	switch kind {
	case "ebupot_23":
		slips, err := s.ListWithholdingSlips(ctx, schoolID, taxPeriod, "", "")
		if err != nil {
			return nil, err
		}
		var totTax int64
		for _, sl := range slips {
			totTax += sl.TaxAmountIDR
		}
		tpl.RecordCount = len(slips)
		tpl.TotalTaxIDR = totTax
		tpl.ContentCSV = FormatEBupot23CSV(slips)

	case "efaktur_input":
		vats, err := s.ListInputVAT(ctx, schoolID, taxPeriod, "")
		if err != nil {
			return nil, err
		}
		var totTax int64
		for _, v := range vats {
			totTax += v.VATIDR
		}
		tpl.RecordCount = len(vats)
		tpl.TotalTaxIDR = totTax
		tpl.ContentCSV = FormatEFakturInputCSV(vats)

	default:
		return nil, fmt.Errorf("jenis ekspor tidak didukung: %s", kind)
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO tax_export_templates (school_id, kind, tax_period, record_count, total_tax_idr, generated_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`, schoolID, kind, taxPeriod, tpl.RecordCount, tpl.TotalTaxIDR, userID)
	_ = row.Scan(&tpl.ID, &tpl.CreatedAt)

	return tpl, nil
}
