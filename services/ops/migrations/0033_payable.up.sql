-- 0033_payable.up.sql: Utang Usaha 3-Way Match & Pembayaran (Menu 111)
-- Pengaturan AP, faktur vendor, 3-way match (PO vs GR vs Invoice), jadwal pembayaran, dan jurnal otomatis.

CREATE TABLE payable_settings (
    school_id UUID PRIMARY KEY,
    default_ap_acc_code TEXT NOT NULL DEFAULT '2101',
    default_vat_in_acc_code TEXT NULL DEFAULT '1181',
    default_tax_payable_acc_code TEXT NULL DEFAULT '2131',
    default_bank_acc_code TEXT NULL DEFAULT '1102',
    tolerance_amount_idr BIGINT NOT NULL DEFAULT 0 CHECK (tolerance_amount_idr >= 0),
    auto_journal_on_invoice BOOLEAN NOT NULL DEFAULT true,
    auto_journal_on_payment BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE vendor_invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    number TEXT NOT NULL,
    vendor_id UUID NOT NULL REFERENCES vendors(id),
    po_id UUID NULL REFERENCES purchase_orders(id),
    invoice_date DATE NOT NULL,
    due_date DATE NOT NULL,
    subtotal_idr BIGINT NOT NULL DEFAULT 0 CHECK (subtotal_idr >= 0),
    vat_idr BIGINT NOT NULL DEFAULT 0 CHECK (vat_idr >= 0),
    withholding_tax_idr BIGINT NOT NULL DEFAULT 0 CHECK (withholding_tax_idr >= 0),
    total_idr BIGINT NOT NULL DEFAULT 0 CHECK (total_idr >= 0),
    paid_amount_idr BIGINT NOT NULL DEFAULT 0 CHECK (paid_amount_idr >= 0),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_match', 'matched', 'discrepancy', 'override_approved', 'approved', 'partially_paid', 'paid', 'cancelled')),
    match_status TEXT NOT NULL DEFAULT 'pending' CHECK (match_status IN ('pending', 'matched_ok', 'qty_mismatch', 'price_mismatch', 'over_billed', 'override_needed')),
    match_discrepancy_note TEXT NULL,
    override_approval_id UUID NULL,
    pdf_object_key TEXT NULL,
    journal_trans_no TEXT NULL,
    idempotency_key TEXT NOT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_vendor_invoices_idempotency UNIQUE (school_id, idempotency_key),
    CONSTRAINT uq_vendor_invoices_vendor_no UNIQUE (school_id, vendor_id, number)
);
CREATE INDEX idx_vendor_invoices_vendor ON vendor_invoices(school_id, vendor_id, status);
CREATE INDEX idx_vendor_invoices_due ON vendor_invoices(school_id, due_date);
CREATE INDEX idx_vendor_invoices_po ON vendor_invoices(po_id);

CREATE TABLE vendor_invoice_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    invoice_id UUID NOT NULL REFERENCES vendor_invoices(id) ON DELETE CASCADE,
    po_line_id UUID NULL REFERENCES purchase_order_lines(id),
    gr_line_id UUID NULL REFERENCES goods_receipt_lines(id),
    description TEXT NOT NULL,
    qty BIGINT NOT NULL CHECK (qty > 0),
    unit_price_idr BIGINT NOT NULL CHECK (unit_price_idr >= 0),
    amount_idr BIGINT GENERATED ALWAYS AS (qty * unit_price_idr) STORED,
    withholding_type_id UUID NULL REFERENCES withholding_tax_types(id),
    withholding_tax_idr BIGINT NOT NULL DEFAULT 0 CHECK (withholding_tax_idr >= 0),
    budget_line_key UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_vendor_invoice_lines_inv ON vendor_invoice_lines(invoice_id);

CREATE TABLE payment_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    invoice_id UUID NOT NULL REFERENCES vendor_invoices(id) ON DELETE CASCADE,
    vendor_id UUID NOT NULL REFERENCES vendors(id),
    scheduled_date DATE NOT NULL,
    amount_idr BIGINT NOT NULL CHECK (amount_idr > 0),
    payment_method TEXT NOT NULL DEFAULT 'bank_transfer' CHECK (payment_method IN ('bank_transfer', 'cash', 'giro', 'other')),
    source_bank_acc_code TEXT NULL,
    vendor_bank_account_id UUID NULL REFERENCES vendor_bank_accounts(id),
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'pending_approval', 'approved', 'executed', 'cancelled')),
    approval_id UUID NULL,
    executed_at TIMESTAMPTZ NULL,
    executed_by UUID NULL,
    reference_no TEXT NULL,
    journal_trans_no TEXT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payment_schedules_date ON payment_schedules(school_id, scheduled_date, status);
CREATE INDEX idx_payment_schedules_invoice ON payment_schedules(invoice_id);

CREATE TABLE payable_journals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('invoice', 'payment')),
    source_id UUID NOT NULL,
    trans_no TEXT NOT NULL,
    trans_date DATE NOT NULL,
    amount_idr BIGINT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'posted' CHECK (status IN ('posted', 'failed', 'reversed')),
    error_message TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payable_journals_source ON payable_journals(source_type, source_id);
