-- 0011_procurement.up.sql: Pengadaan (Menu 104–106)
-- Vendor, PR (Purchase Request), PO (Purchase Order), Penerimaan Barang (Goods Receipt), Retur, dan Evaluasi.

CREATE TABLE vendors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    npwp_enc BYTEA NULL,
    npwp_bidx TEXT NULL,
    address TEXT NULL,
    categories TEXT[] NOT NULL DEFAULT '{}',
    payment_terms_days INT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'blocked')),
    duplicate_ack_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_vendors_code ON vendors(school_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_vendors_npwp_bidx ON vendors(school_id, npwp_bidx);

CREATE TABLE vendor_contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    vendor_id UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    phone TEXT NULL,
    email TEXT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE vendor_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    vendor_id UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    object_key TEXT NOT NULL,
    expires_on DATE NULL,
    uploaded_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE vendor_bank_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    vendor_id UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    bank_name TEXT NOT NULL,
    account_no_enc BYTEA NOT NULL,
    account_no_bidx TEXT NOT NULL,
    account_holder TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending_approval' CHECK (status IN ('pending_approval', 'active', 'rejected', 'superseded')),
    approval_id UUID NULL,
    requested_by UUID NOT NULL,
    activated_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX one_active_bank ON vendor_bank_accounts(vendor_id) WHERE status = 'active';

CREATE TABLE vendor_accounting_sync (
    vendor_id UUID PRIMARY KEY REFERENCES vendors(id) ON DELETE CASCADE,
    school_id UUID NOT NULL,
    payload_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'synced', 'failed')),
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT NULL,
    synced_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE purchase_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    number TEXT NOT NULL,
    requester_user_id UUID NOT NULL,
    reason TEXT NOT NULL,
    needed_by DATE NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_approval', 'approved', 'rejected', 'cancelled', 'partially_ordered', 'ordered')),
    total_est_idr BIGINT NOT NULL DEFAULT 0,
    over_budget BOOLEAN NOT NULL DEFAULT false,
    budget_check JSONB NULL,
    approval_id UUID NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_purchase_requests_idempotency UNIQUE (school_id, idempotency_key)
);
CREATE INDEX idx_purchase_requests_school_status ON purchase_requests(school_id, status);

CREATE TABLE purchase_request_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    pr_id UUID NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    item_id UUID NULL REFERENCES items(id),
    description TEXT NOT NULL,
    qty BIGINT NOT NULL CHECK (qty > 0),
    unit TEXT NOT NULL,
    est_unit_price_idr BIGINT NOT NULL CHECK (est_unit_price_idr >= 0),
    budget_line_key UUID NULL,
    qty_ordered BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_pr_lines_pr ON purchase_request_lines(pr_id);

CREATE TABLE purchase_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    number TEXT NOT NULL,
    vendor_id UUID NOT NULL REFERENCES vendors(id),
    payment_terms_days INT NOT NULL DEFAULT 30,
    expected_date DATE NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_approval', 'approved', 'rejected', 'sent', 'partially_received', 'received', 'closed', 'cancelled')),
    total_idr BIGINT NOT NULL DEFAULT 0,
    approval_id UUID NULL,
    pdf_object_key TEXT NULL,
    sent_at TIMESTAMPTZ NULL,
    closed_reason TEXT NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_purchase_orders_idempotency UNIQUE (school_id, idempotency_key)
);
CREATE INDEX idx_purchase_orders_school_status ON purchase_orders(school_id, status);

CREATE TABLE purchase_order_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    po_id UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    pr_line_id UUID NULL REFERENCES purchase_request_lines(id),
    item_id UUID NULL REFERENCES items(id),
    description TEXT NOT NULL,
    qty BIGINT NOT NULL CHECK (qty > 0),
    unit_price_idr BIGINT NOT NULL CHECK (unit_price_idr >= 0),
    amount_idr BIGINT GENERATED ALWAYS AS (qty * unit_price_idr) STORED,
    budget_line_key UUID NULL,
    withholding_type_id UUID NULL,
    warehouse_id UUID NULL REFERENCES warehouses(id)
);
CREATE INDEX idx_po_lines_po ON purchase_order_lines(po_id);

CREATE TABLE goods_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    number TEXT NOT NULL,
    po_id UUID NOT NULL REFERENCES purchase_orders(id),
    received_on DATE NOT NULL,
    received_by UUID NOT NULL,
    note TEXT NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_goods_receipts_idempotency UNIQUE (school_id, idempotency_key)
);
CREATE INDEX idx_goods_receipts_po ON goods_receipts(po_id);

CREATE TABLE goods_receipt_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    gr_id UUID NOT NULL REFERENCES goods_receipts(id) ON DELETE CASCADE,
    po_line_id UUID NOT NULL REFERENCES purchase_order_lines(id),
    qty_received BIGINT NOT NULL,
    qty_accepted BIGINT NOT NULL,
    qty_rejected BIGINT NOT NULL,
    reject_reason TEXT NULL,
    asset_handoff_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_gr_lines_qty CHECK (qty_received = qty_accepted + qty_rejected)
);
CREATE INDEX idx_gr_lines_gr ON goods_receipt_lines(gr_id);
CREATE INDEX idx_gr_lines_po_line ON goods_receipt_lines(po_line_id);

CREATE TABLE goods_returns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    gr_line_id UUID NOT NULL REFERENCES goods_receipt_lines(id),
    qty BIGINT NOT NULL CHECK (qty > 0),
    reason TEXT NOT NULL,
    returned_on DATE NOT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE vendor_evaluations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    vendor_id UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    goods_receipt_id UUID NOT NULL UNIQUE,
    on_time BOOLEAN NULL,
    quality_score SMALLINT NOT NULL CHECK (quality_score BETWEEN 1 AND 5),
    price_score SMALLINT NOT NULL CHECK (price_score BETWEEN 1 AND 5),
    note TEXT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- KONTRAK BACA untuk T17 (3-way match & komitmen)
CREATE OR REPLACE VIEW po_line_match_v AS
  SELECT pol.school_id, pol.po_id, pol.id AS po_line_id, po.vendor_id, po.status AS po_status, pol.item_id, pol.budget_line_key,
         pol.withholding_type_id, pol.qty AS qty_ordered, pol.unit_price_idr, pol.amount_idr,
         COALESCE(acc.qty_acc, 0) - COALESCE(ret.qty_ret, 0) AS qty_accepted_net
  FROM purchase_order_lines pol
  JOIN purchase_orders po ON po.id = pol.po_id
  LEFT JOIN (
    SELECT po_line_id, SUM(qty_accepted) AS qty_acc
    FROM goods_receipt_lines
    GROUP BY po_line_id
  ) acc ON acc.po_line_id = pol.id
  LEFT JOIN (
    SELECT grl.po_line_id, SUM(grt.qty) AS qty_ret
    FROM goods_returns grt
    JOIN goods_receipt_lines grl ON grl.id = grt.gr_line_id
    GROUP BY grl.po_line_id
  ) ret ON ret.po_line_id = pol.id;

CREATE OR REPLACE VIEW pr_open_commitment_v AS
  SELECT prl.school_id, prl.budget_line_key, pr.id AS pr_id,
         (prl.qty - prl.qty_ordered) * prl.est_unit_price_idr AS amount_idr
  FROM purchase_request_lines prl
  JOIN purchase_requests pr ON pr.id = prl.pr_id
  WHERE pr.status IN ('approved', 'partially_ordered') AND prl.qty > prl.qty_ordered;
