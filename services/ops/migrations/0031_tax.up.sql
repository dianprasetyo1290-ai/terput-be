-- 0031_tax.up.sql: Pajak Non-Gaji & e-Faktur (Menu 110)
-- Pengaturan pajak, jenis & tarif PPh potong/pungut (23, 4(2), dsb), bukti potong, PPN masukan & keluaran, dan ekspor DJP.

CREATE TABLE tax_settings (
    school_id UUID PRIMARY KEY,
    npwp_masked TEXT NULL,
    nitku TEXT NULL,
    efaktur_enabled BOOLEAN NOT NULL DEFAULT false,
    default_ppn_bp INT NOT NULL DEFAULT 1100 CHECK (default_ppn_bp >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE withholding_tax_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    article TEXT NOT NULL CHECK (article IN ('21', '22', '23', '4(2)', '15')),
    standard_rate_bp INT NOT NULL CHECK (standard_rate_bp >= 0),
    no_npwp_rate_bp INT NOT NULL CHECK (no_npwp_rate_bp >= 0),
    description TEXT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_withholding_types_code UNIQUE (school_id, code)
);
CREATE INDEX idx_withholding_tax_types_school ON withholding_tax_types(school_id, is_active);

CREATE TABLE withholding_tax_rates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    type_id UUID NOT NULL REFERENCES withholding_tax_types(id) ON DELETE CASCADE,
    effective_from DATE NOT NULL,
    rate_bp INT NOT NULL CHECK (rate_bp >= 0),
    no_npwp_rate_bp INT NOT NULL CHECK (no_npwp_rate_bp >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE withholding_slips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    slip_number TEXT NOT NULL,
    type_id UUID NOT NULL REFERENCES withholding_tax_types(id),
    vendor_id UUID NOT NULL REFERENCES vendors(id),
    vendor_invoice_id UUID NULL,
    tax_period TEXT NOT NULL,
    gross_amount_idr BIGINT NOT NULL CHECK (gross_amount_idr >= 0),
    rate_bp INT NOT NULL CHECK (rate_bp >= 0),
    tax_amount_idr BIGINT NOT NULL CHECK (tax_amount_idr >= 0),
    npwp_used TEXT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'issued', 'reported', 'cancelled')),
    issued_at TIMESTAMPTZ NULL,
    reported_at TIMESTAMPTZ NULL,
    pdf_object_key TEXT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_withholding_slips_number UNIQUE (school_id, slip_number)
);
CREATE INDEX idx_withholding_slips_school_period ON withholding_slips(school_id, tax_period, status);
CREATE INDEX idx_withholding_slips_vendor ON withholding_slips(vendor_id);

CREATE TABLE input_vat (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    invoice_number TEXT NOT NULL,
    vendor_id UUID NOT NULL REFERENCES vendors(id),
    vendor_invoice_id UUID NULL,
    tax_period TEXT NOT NULL,
    invoice_date DATE NOT NULL,
    dpp_idr BIGINT NOT NULL CHECK (dpp_idr >= 0),
    vat_idr BIGINT NOT NULL CHECK (vat_idr >= 0),
    creditable BOOLEAN NOT NULL DEFAULT true,
    status TEXT NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'credited', 'expired', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_input_vat_no UNIQUE (school_id, invoice_number)
);
CREATE INDEX idx_input_vat_school_period ON input_vat(school_id, tax_period, status);

CREATE TABLE output_tax_invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    invoice_number TEXT NOT NULL,
    customer_name TEXT NOT NULL,
    customer_npwp TEXT NULL,
    tax_period TEXT NOT NULL,
    invoice_date DATE NOT NULL,
    dpp_idr BIGINT NOT NULL CHECK (dpp_idr >= 0),
    vat_idr BIGINT NOT NULL CHECK (vat_idr >= 0),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'approved', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_output_tax_invoices_no UNIQUE (school_id, invoice_number)
);
CREATE INDEX idx_output_tax_invoices_school_period ON output_tax_invoices(school_id, tax_period);

CREATE TABLE tax_export_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('ebupot_23', 'ebupot_4_2', 'efaktur_input', 'efaktur_output')),
    tax_period TEXT NOT NULL,
    record_count INT NOT NULL DEFAULT 0,
    total_tax_idr BIGINT NOT NULL DEFAULT 0,
    file_object_key TEXT NULL,
    generated_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
