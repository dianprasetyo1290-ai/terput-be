-- 0010_inventory.up.sql: Inventory (Menu 102–103)
-- Master barang, kategori, gudang, saldo stok, mutasi stok, dan stock opname.

CREATE TABLE item_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    tracking TEXT NOT NULL CHECK (tracking IN ('stock', 'asset', 'none')),
    purchase_acc_code TEXT NULL,
    withholding_type_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_item_categories_code ON item_categories(school_id, code) WHERE deleted_at IS NULL;

CREATE TABLE items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    category_id UUID NOT NULL REFERENCES item_categories(id),
    unit TEXT NOT NULL,
    min_stock BIGINT NOT NULL DEFAULT 0 CHECK (min_stock >= 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_items_code ON items(school_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_items_category ON items(school_id, category_id);

CREATE TABLE warehouses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    room_id UUID NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_warehouses_code ON warehouses(school_id, code) WHERE deleted_at IS NULL;

CREATE TABLE stock_balances (
    school_id UUID NOT NULL,
    warehouse_id UUID NOT NULL REFERENCES warehouses(id),
    item_id UUID NOT NULL REFERENCES items(id),
    qty BIGINT NOT NULL DEFAULT 0 CHECK (qty >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (warehouse_id, item_id)
);
CREATE INDEX idx_stock_balances_school_item ON stock_balances(school_id, item_id);

CREATE TABLE stock_movements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    warehouse_id UUID NOT NULL REFERENCES warehouses(id),
    item_id UUID NOT NULL REFERENCES items(id),
    direction TEXT NOT NULL CHECK (direction IN ('in', 'out')),
    qty BIGINT NOT NULL CHECK (qty > 0),
    kind TEXT NOT NULL CHECK (kind IN ('receipt', 'issue', 'transfer_in', 'transfer_out', 'adjustment', 'return_to_vendor')),
    ref_type TEXT NOT NULL,
    ref_id UUID NOT NULL,
    ref_no TEXT NOT NULL,
    transfer_id UUID NULL,
    balance_after BIGINT NOT NULL,
    note TEXT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    idempotency_key TEXT NOT NULL,
    CONSTRAINT uq_stock_movements_idempotency UNIQUE (school_id, idempotency_key)
);
CREATE INDEX idx_stock_movements_item ON stock_movements(school_id, warehouse_id, item_id, created_at DESC);

-- Trigger: tolak UPDATE atau DELETE pada stock_movements (BR-INV-01 mutasi tidak dapat dihapus)
CREATE OR REPLACE FUNCTION trg_stock_movements_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'mutasi stok tidak dapat diubah atau dihapus';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER stock_movements_immutable_trg
BEFORE UPDATE OR DELETE ON stock_movements
FOR EACH ROW EXECUTE FUNCTION trg_stock_movements_immutable();

CREATE TABLE stock_counts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    warehouse_id UUID NOT NULL REFERENCES warehouses(id),
    number TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'counting' CHECK (status IN ('counting', 'submitted', 'pending_approval', 'approved', 'rejected', 'adjusted', 'adjust_failed', 'cancelled')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_by UUID NOT NULL,
    submitted_at TIMESTAMPTZ NULL,
    approval_id UUID NULL,
    report_object_key TEXT NULL,
    error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX one_open_count ON stock_counts(warehouse_id) WHERE status = 'counting';
CREATE INDEX idx_stock_counts_school ON stock_counts(school_id, warehouse_id, created_at DESC);

CREATE TABLE stock_count_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    count_id UUID NOT NULL REFERENCES stock_counts(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id),
    system_qty BIGINT NOT NULL,
    counted_qty BIGINT NULL,
    note TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_stock_count_line UNIQUE (count_id, item_id)
);

CREATE TABLE inventory_settings (
    school_id UUID PRIMARY KEY,
    doc_number_format JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
