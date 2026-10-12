-- 0005: Kantin, POS & dompet siswa (T12, PRD menu 123-126).
-- Money is bigint rupiah, percentages are basis points. Wallet balances are
-- school liabilities (dana titipan, BR-WAL-02): every change is an
-- append-only wallet_movements row in the same DB transaction.

-- ---------------------------------------------------------------------------
-- Tenant & skema (FR-123-001/002/003)
-- ---------------------------------------------------------------------------
CREATE TABLE canteen_tenants (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         uuid NOT NULL,
    code              text NOT NULL,
    name              text NOT NULL,
    location_label    text NOT NULL DEFAULT '',
    room_id           uuid NULL,
    owner_name        text NOT NULL DEFAULT '',
    owner_phone       text NOT NULL DEFAULT '',
    bank_name         text NOT NULL DEFAULT '',
    bank_account_no   text NOT NULL DEFAULT '',
    bank_account_name text NOT NULL DEFAULT '',
    status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
    is_open           boolean NOT NULL DEFAULT false,
    open_changed_at   timestamptz NULL,
    deleted_at        timestamptz NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, code)
);
CREATE INDEX canteen_tenants_school_idx ON canteen_tenants (school_id, status) WHERE deleted_at IS NULL;

-- Revenue-share / rent history. A change is a new row (BR-HR-02); periods of
-- one tenant never overlap (trigger below).
CREATE TABLE canteen_tenant_schemes (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id      uuid NOT NULL,
    tenant_id      uuid NOT NULL REFERENCES canteen_tenants(id),
    kind           text NOT NULL CHECK (kind IN ('revenue_share','rent')),
    share_bp       integer NULL CHECK (share_bp BETWEEN 0 AND 10000),
    share_base     text NULL CHECK (share_base IN ('wallet','all')),
    rent_idr       bigint NULL CHECK (rent_idr >= 0),
    effective_from date NOT NULL,
    effective_to   date NULL,
    created_by     uuid NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'revenue_share') = (share_bp IS NOT NULL AND share_base IS NOT NULL)),
    CHECK ((kind = 'rent') = (rent_idr IS NOT NULL)),
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);
CREATE INDEX canteen_tenant_schemes_tenant_idx ON canteen_tenant_schemes (tenant_id, effective_from);

CREATE FUNCTION canteen_scheme_no_overlap() RETURNS trigger AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM canteen_tenant_schemes s
         WHERE s.tenant_id = NEW.tenant_id AND s.id <> NEW.id
           AND daterange(s.effective_from, COALESCE(s.effective_to, 'infinity'::date), '[]')
            && daterange(NEW.effective_from, COALESCE(NEW.effective_to, 'infinity'::date), '[]')
    ) THEN
        RAISE EXCEPTION 'canteen scheme period overlaps an existing scheme' USING ERRCODE = '23P01';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER canteen_scheme_no_overlap_trg
    BEFORE INSERT OR UPDATE ON canteen_tenant_schemes
    FOR EACH ROW EXECUTE FUNCTION canteen_scheme_no_overlap();

CREATE TABLE canteen_tenant_operators (
    tenant_id  uuid NOT NULL REFERENCES canteen_tenants(id),
    user_id    uuid NOT NULL,
    school_id  uuid NOT NULL,
    email      text NOT NULL DEFAULT '',
    full_name  text NOT NULL DEFAULT '',
    active     boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id)
);
CREATE INDEX canteen_tenant_operators_user_idx ON canteen_tenant_operators (school_id, user_id) WHERE active;

-- ---------------------------------------------------------------------------
-- Menu & POS (FR-124-*)
-- ---------------------------------------------------------------------------
CREATE TABLE canteen_menu_items (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  uuid NOT NULL,
    tenant_id  uuid NOT NULL REFERENCES canteen_tenants(id),
    name       text NOT NULL,
    category   text NOT NULL DEFAULT '',
    price_idr  bigint NOT NULL CHECK (price_idr > 0),
    active     boolean NOT NULL DEFAULT true,
    deleted_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX canteen_menu_items_tenant_idx ON canteen_menu_items (tenant_id) WHERE deleted_at IS NULL;

CREATE TABLE canteen_daily_stock (
    item_id    uuid NOT NULL REFERENCES canteen_menu_items(id),
    stock_date date NOT NULL,
    qty_start  integer NOT NULL CHECK (qty_start >= 0),
    qty_sold   integer NOT NULL DEFAULT 0 CHECK (qty_sold >= 0),
    PRIMARY KEY (item_id, stock_date)
);

CREATE TABLE canteen_sale_sequences (
    tenant_id uuid NOT NULL REFERENCES canteen_tenants(id),
    sale_date date NOT NULL,
    last      integer NOT NULL,
    PRIMARY KEY (tenant_id, sale_date)
);

-- ---------------------------------------------------------------------------
-- Dompet (FR-125-*)
-- ---------------------------------------------------------------------------
CREATE TABLE wallet_settings (
    school_id                 uuid PRIMARY KEY,
    max_allowance_balance_idr bigint NULL CHECK (max_allowance_balance_idr > 0),
    max_savings_balance_idr   bigint NULL CHECK (max_savings_balance_idr > 0),
    pos_offline_max_minutes   integer NOT NULL DEFAULT 30 CHECK (pos_offline_max_minutes > 0),
    updated_at                timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE wallets (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    student_user_id uuid NOT NULL,
    kind            text NOT NULL CHECK (kind IN ('allowance','savings')),
    balance_idr     bigint NOT NULL DEFAULT 0 CHECK (balance_idr >= 0),
    daily_limit_idr bigint NULL CHECK (daily_limit_idr > 0),
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','closing','closed')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, student_user_id, kind),
    CHECK (kind = 'allowance' OR daily_limit_idr IS NULL)
);

CREATE TABLE wallet_movements (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         uuid NOT NULL,
    wallet_id         uuid NOT NULL REFERENCES wallets(id),
    kind              text NOT NULL CHECK (kind IN ('topup_gateway','topup_cashier','pos_sale','pos_void','savings_deposit',
                                                    'savings_withdrawal','transfer_in','transfer_out','refund_exit')),
    amount_idr        bigint NOT NULL CHECK (amount_idr <> 0),
    balance_after_idr bigint NOT NULL CHECK (balance_after_idr >= 0),
    ref_type          text NOT NULL DEFAULT '',
    ref_id            uuid NULL,
    created_by        text NOT NULL DEFAULT '',
    idempotency_key   text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX wallet_movements_wallet_idx ON wallet_movements (wallet_id, created_at);
CREATE INDEX wallet_movements_school_day_idx ON wallet_movements (school_id, created_at);

CREATE FUNCTION wallet_movements_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'wallet_movements is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER wallet_movements_append_only_trg
    BEFORE UPDATE OR DELETE ON wallet_movements
    FOR EACH ROW EXECUTE FUNCTION wallet_movements_append_only();

CREATE TABLE canteen_sales (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          uuid NOT NULL,
    tenant_id          uuid NOT NULL REFERENCES canteen_tenants(id),
    sale_no            text NOT NULL,
    pay_method         text NOT NULL CHECK (pay_method IN ('wallet','cash')),
    total_idr          bigint NOT NULL CHECK (total_idr > 0),
    student_user_id    uuid NULL,
    wallet_movement_id uuid NULL REFERENCES wallet_movements(id),
    operator_user_id   uuid NOT NULL,
    device_id          text NOT NULL DEFAULT '',
    client_txn_id      uuid NOT NULL,
    recorded_at        timestamptz NOT NULL,
    synced_at          timestamptz NOT NULL DEFAULT now(),
    offline            boolean NOT NULL DEFAULT false,
    late_sync          boolean NOT NULL DEFAULT false,
    status             text NOT NULL DEFAULT 'completed' CHECK (status IN ('completed','voided')),
    void_reason        text NULL,
    voided_by          uuid NULL,
    voided_at          timestamptz NULL,
    settlement_id      uuid NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, tenant_id, client_txn_id),
    UNIQUE (tenant_id, sale_no),
    CHECK ((pay_method = 'wallet') = (student_user_id IS NOT NULL)),
    CHECK (pay_method = 'wallet' OR wallet_movement_id IS NULL),
    CHECK (pay_method <> 'wallet' OR NOT offline),
    CHECK (status <> 'voided' OR (void_reason IS NOT NULL AND voided_at IS NOT NULL))
);
CREATE INDEX canteen_sales_report_idx ON canteen_sales (school_id, tenant_id, recorded_at);

CREATE TABLE canteen_sale_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id        uuid NOT NULL REFERENCES canteen_sales(id) ON DELETE CASCADE,
    item_id        uuid NOT NULL REFERENCES canteen_menu_items(id),
    name_snapshot  text NOT NULL,
    qty            integer NOT NULL CHECK (qty > 0),
    unit_price_idr bigint NOT NULL CHECK (unit_price_idr > 0),
    line_total_idr bigint NOT NULL CHECK (line_total_idr > 0)
);
CREATE INDEX canteen_sale_items_sale_idx ON canteen_sale_items (sale_id);

CREATE TABLE canteen_settlements (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         uuid NOT NULL,
    tenant_id         uuid NOT NULL REFERENCES canteen_tenants(id),
    period_from       date NOT NULL,
    period_to         date NOT NULL,
    scheme_id         uuid NOT NULL REFERENCES canteen_tenant_schemes(id),
    scheme_kind       text NOT NULL CHECK (scheme_kind IN ('revenue_share','rent')),
    wallet_sales_idr  bigint NOT NULL CHECK (wallet_sales_idr >= 0),
    cash_sales_idr    bigint NOT NULL CHECK (cash_sales_idr >= 0),
    school_share_idr  bigint NOT NULL CHECK (school_share_idr >= 0),
    tenant_payout_idr bigint NOT NULL,
    status            text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','pending_approval','approved','rejected','paid')),
    approval_id       text NULL,
    decided_by        text NULL,
    decision_comment  text NULL,
    paid_at           timestamptz NULL,
    paid_ref          text NULL,
    paid_by           uuid NULL,
    created_by        uuid NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (period_to >= period_from),
    CHECK (status <> 'paid' OR paid_at IS NOT NULL)
);
CREATE UNIQUE INDEX canteen_settlements_period_uq ON canteen_settlements (tenant_id, period_from, period_to) WHERE status <> 'rejected';
CREATE INDEX canteen_settlements_school_idx ON canteen_settlements (school_id, status, created_at DESC);

ALTER TABLE canteen_sales ADD CONSTRAINT canteen_sales_settlement_fk FOREIGN KEY (settlement_id) REFERENCES canteen_settlements(id);

-- Shared student card (OQ-30: static QR, revocable). Used by POS, library,
-- transport; not wallet-specific. Only the sha256 of the code is stored.
CREATE TABLE student_cards (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    student_user_id uuid NOT NULL,
    token_hash      text NOT NULL UNIQUE,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    issued_by       uuid NOT NULL,
    revoked_at      timestamptz NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'revoked' OR revoked_at IS NOT NULL)
);
CREATE UNIQUE INDEX student_cards_one_active ON student_cards (school_id, student_user_id) WHERE status = 'active';

CREATE TABLE wallet_topups (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              uuid NOT NULL,
    student_user_id        uuid NOT NULL,
    wallet_kind            text NOT NULL CHECK (wallet_kind IN ('allowance','savings')),
    amount_idr             bigint NOT NULL CHECK (amount_idr > 0),
    channel                text NOT NULL CHECK (channel IN ('gateway','cashier')),
    gateway_transaction_id uuid NULL,
    receipt_id             uuid NULL,
    status                 text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','success','failed','expired')),
    requested_by           uuid NOT NULL,
    idempotency_key        text NOT NULL,
    notified_at            timestamptz NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX wallet_topups_pending_idx ON wallet_topups (school_id, status) WHERE status = 'pending';
CREATE INDEX wallet_topups_notify_idx ON wallet_topups (created_at) WHERE status = 'success' AND notified_at IS NULL AND channel = 'gateway';

CREATE TABLE savings_requests (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         uuid NOT NULL,
    wallet_id         uuid NOT NULL REFERENCES wallets(id),
    kind              text NOT NULL CHECK (kind IN ('deposit','withdrawal')),
    amount_idr        bigint NOT NULL CHECK (amount_idr > 0),
    note              text NOT NULL DEFAULT '',
    proof_object_key  text NULL,
    requested_by      uuid NOT NULL,
    requested_role    text NOT NULL,
    status            text NOT NULL CHECK (status IN ('pending_parent','approved','rejected','completed')),
    parent_user_id    uuid NULL,
    parent_decided_at timestamptz NULL,
    completed_by      uuid NULL,
    completed_at      timestamptz NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'completed' OR (completed_at IS NOT NULL AND proof_object_key IS NOT NULL))
);
CREATE INDEX savings_requests_school_idx ON savings_requests (school_id, status, created_at DESC);

-- Wallet closure on graduation/exit (FR-125-005). Transfers to a sibling go
-- through core approval 'wallet_transfer'; refunds go through menu 25.
CREATE TABLE wallet_closures (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              uuid NOT NULL,
    wallet_id              uuid NOT NULL REFERENCES wallets(id),
    disposition            text NOT NULL CHECK (disposition IN ('refund','transfer')),
    target_student_user_id uuid NULL,
    amount_idr             bigint NOT NULL CHECK (amount_idr >= 0),
    status                 text NOT NULL CHECK (status IN ('pending_approval','approved','rejected','completed')),
    approval_id            text NULL,
    refund_id              uuid NULL,
    requested_by           uuid NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CHECK ((disposition = 'transfer') = (target_student_user_id IS NOT NULL))
);
CREATE UNIQUE INDEX wallet_closures_one_open ON wallet_closures (wallet_id) WHERE status IN ('pending_approval','approved');

-- Chart-of-accounts mapping (no defaults: account codes belong to each
-- school's COA, created by the accountant through menu 33).
CREATE TABLE wallet_accounts (
    school_id               uuid PRIMARY KEY,
    allowance_liability_acc text NOT NULL,
    savings_liability_acc   text NOT NULL,
    cash_acc                text NOT NULL,
    tenant_payable_acc      text NOT NULL,
    canteen_revenue_acc     text NOT NULL,
    rent_revenue_acc        text NULL,
    tenant_receivable_acc   text NULL,
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE wallet_reconciliations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    run_on          date NOT NULL,
    wallet_kind     text NOT NULL CHECK (wallet_kind IN ('allowance','savings')),
    wallet_total_idr bigint NOT NULL,
    ledger_idr      bigint NOT NULL,
    diff_idr        bigint NOT NULL,
    status          text NOT NULL CHECK (status IN ('matched','mismatch')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, run_on, wallet_kind)
);

-- ---------------------------------------------------------------------------
-- Additive changes to T6 tables
-- ---------------------------------------------------------------------------
ALTER TABLE bill_refunds ADD COLUMN wallet_id uuid NULL REFERENCES wallets(id);
CREATE INDEX bill_refunds_wallet_idx ON bill_refunds (wallet_id) WHERE wallet_id IS NOT NULL;

ALTER TABLE receipts DROP CONSTRAINT receipts_source_check;
ALTER TABLE receipts ADD CONSTRAINT receipts_source_check CHECK (source IN ('gateway','cashier','wallet'));

ALTER TABLE gateway_journal_postings ADD COLUMN source text NOT NULL DEFAULT 'gateway';
