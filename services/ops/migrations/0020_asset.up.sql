-- 0020_asset.up.sql: Aset Tetap & Fasilitas (Menu 107, 108)
-- Register aset, label QR, mutasi, pelepasan, dan integrasi akuntansi

CREATE TABLE asset_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    useful_life_months INT NULL CHECK (useful_life_months IS NULL OR useful_life_months > 0),
    dep_start TEXT NULL CHECK (dep_start IS NULL OR dep_start IN ('acquisition_month', 'next_month')),
    asset_acc_code TEXT NULL,
    accum_dep_acc_code TEXT NULL,
    dep_expense_acc_code TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX uq_asset_categories_code ON asset_categories(school_id, code) WHERE deleted_at IS NULL;

ALTER TABLE item_categories ADD COLUMN IF NOT EXISTS asset_category_id UUID NULL REFERENCES asset_categories(id);

CREATE TABLE asset_settings (
    school_id UUID PRIMARY KEY,
    code_format TEXT NOT NULL DEFAULT '{cat}-{yyyy}-{seq:4}',
    interunit_acc_code TEXT NULL,
    disposal_gain_acc_code TEXT NULL,
    disposal_loss_acc_code TEXT NULL,
    proceeds_acc_code TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    category_id UUID NULL REFERENCES asset_categories(id),
    status TEXT NOT NULL CHECK (status IN ('candidate', 'active', 'disposed', 'transferred_out')),
    condition TEXT NULL CHECK (condition IS NULL OR condition IN ('baik', 'rusak_ringan', 'rusak_berat')),
    acquisition_cost_idr BIGINT NOT NULL DEFAULT 0 CHECK (acquisition_cost_idr >= 0),
    acquired_on DATE NULL,
    residual_idr BIGINT NOT NULL DEFAULT 0,
    room_id UUID NULL REFERENCES rooms(id),
    custodian_user_id UUID NULL,
    serial_no TEXT NULL,
    photo_object_key TEXT NULL,
    qr_token TEXT NOT NULL UNIQUE,
    source TEXT NOT NULL CHECK (source IN ('goods_receipt', 'manual', 'interunit_transfer')),
    gr_line_id UUID NULL REFERENCES goods_receipt_lines(id),
    unit_seq INT NULL,
    po_id UUID NULL REFERENCES purchase_orders(id),
    vendor_id UUID NULL,
    transferred_from_asset_id UUID NULL REFERENCES assets(id),
    offset_acc_code TEXT NULL,
    activated_at TIMESTAMPTZ NULL,
    activated_by UUID NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_assets_code ON assets(school_id, code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX one_candidate_per_unit ON assets(gr_line_id, unit_seq) WHERE gr_line_id IS NOT NULL;
CREATE INDEX idx_assets_school_status ON assets(school_id, status);
CREATE INDEX idx_assets_room ON assets(room_id);
CREATE INDEX idx_assets_category ON assets(category_id);

CREATE TABLE asset_movements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    asset_id UUID NOT NULL REFERENCES assets(id),
    kind TEXT NOT NULL CHECK (kind IN ('location', 'custodian', 'condition', 'unit')),
    from_value JSONB NOT NULL DEFAULT '{}',
    to_value JSONB NOT NULL DEFAULT '{}',
    effective_date DATE NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('done', 'pending_approval', 'approved', 'rejected')),
    approval_id UUID NULL,
    to_school_id UUID NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_asset_movements_asset ON asset_movements(asset_id);

CREATE TABLE asset_disposals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    asset_id UUID NOT NULL REFERENCES assets(id),
    kind TEXT NOT NULL CHECK (kind IN ('dijual', 'dihibahkan', 'rusak', 'hilang')),
    disposed_on DATE NOT NULL,
    proceeds_idr BIGINT NOT NULL DEFAULT 0,
    reason TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending_approval', 'approved', 'rejected', 'cancelled')),
    approval_id UUID NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX one_open_disposal ON asset_disposals(asset_id) WHERE status = 'pending_approval';
CREATE INDEX idx_asset_disposals_school ON asset_disposals(school_id, status);

CREATE TABLE asset_accounting_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    asset_id UUID NOT NULL REFERENCES assets(id),
    kind TEXT NOT NULL CHECK (kind IN ('acquisition', 'transfer', 'disposal')),
    source_id UUID NULL,
    reference TEXT UNIQUE NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'posted', 'failed')),
    attempts INT NOT NULL DEFAULT 0,
    error TEXT NULL,
    trans_nos TEXT[] NULL,
    posted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_asset_events_status ON asset_accounting_events(status);

CREATE TABLE asset_book_values (
    asset_id UUID PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
    school_id UUID NOT NULL,
    cost_idr BIGINT NOT NULL DEFAULT 0,
    accumulated_idr BIGINT NOT NULL DEFAULT 0,
    book_value_idr BIGINT NOT NULL DEFAULT 0,
    last_period CHAR(7) NULL,
    as_of TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_asset_book_values_school ON asset_book_values(school_id);

CREATE TABLE asset_depreciation_runs (
    school_id UUID NOT NULL,
    period CHAR(7) NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'done', 'failed')),
    error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (school_id, period)
);
