-- 0001: Teratai billing / SPP — schemes, student bills, scholarships and the
-- transfer-proof payment ledger for the "Portal Orang Tua".
--
-- Money is stored as integer Indonesian rupiah (bigint). All tables are
-- scoped by school_id (see docs/CONVENTIONS.md multi-tenancy rule).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Billing scheme: one per (school, academic year), optionally narrowed to a
-- grade level or classroom. Holds the bill components (SPP bulanan, uang
-- gedung, seragam, kegiatan) and whether it bills monthly (installment).
CREATE TABLE billing_schemes (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        uuid NOT NULL,
    academic_year_id uuid NOT NULL,
    grade_level      text,
    classroom_id     uuid,
    name             text NOT NULL,
    status           text NOT NULL DEFAULT 'active' CHECK (status IN ('draft','active','archived')),
    installment      boolean NOT NULL DEFAULT false,
    due_day          integer NOT NULL DEFAULT 10 CHECK (due_day BETWEEN 1 AND 31),
    notes            text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX billing_schemes_school_idx ON billing_schemes (school_id, academic_year_id, status);

CREATE TABLE billing_scheme_components (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   uuid NOT NULL,
    scheme_id   uuid NOT NULL REFERENCES billing_schemes(id),
    code        text NOT NULL,
    name        text NOT NULL,
    kind        text NOT NULL DEFAULT 'one_time' CHECK (kind IN ('monthly','one_time')),
    amount_idr  bigint NOT NULL CHECK (amount_idr >= 0),
    sort_order  integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scheme_id, code)
);
CREATE INDEX billing_scheme_components_school_idx ON billing_scheme_components (school_id, scheme_id);

-- One bill per student, scheme and period. The unique constraint is exactly
-- what makes generation idempotent per period+student.
CREATE TABLE student_bills (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         uuid NOT NULL,
    student_user_id   uuid NOT NULL,
    scheme_id         uuid NOT NULL REFERENCES billing_schemes(id),
    period_year       integer NOT NULL,
    period_month      integer NOT NULL CHECK (period_month BETWEEN 1 AND 12),
    base_idr          bigint NOT NULL CHECK (base_idr >= 0),
    discount_idr      bigint NOT NULL DEFAULT 0 CHECK (discount_idr >= 0),
    net_idr           bigint NOT NULL CHECK (net_idr >= 0),
    status            text NOT NULL DEFAULT 'unpaid' CHECK (status IN ('unpaid','partial','paid','void')),
    due_at            timestamptz,
    discount_snapshot jsonb NOT NULL DEFAULT '[]',
    void_reason       text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, student_user_id, period_year, period_month, scheme_id),
    CHECK (base_idr = discount_idr + net_idr)
);
CREATE INDEX student_bills_school_student_idx ON student_bills (school_id, student_user_id, period_year, period_month);
CREATE INDEX student_bills_school_status_idx ON student_bills (school_id, status, due_at);

CREATE TABLE bill_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id      uuid NOT NULL,
    bill_id        uuid NOT NULL REFERENCES student_bills(id) ON DELETE CASCADE,
    name           text NOT NULL,
    component_code text,
    amount_idr     bigint NOT NULL CHECK (amount_idr >= 0),
    discount_idr   bigint NOT NULL DEFAULT 0 CHECK (discount_idr >= 0),
    net_idr        bigint NOT NULL CHECK (net_idr >= 0),
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bill_items_school_bill_idx ON bill_items (school_id, bill_id);

-- Scholarships / discounts (persen bps atau nominal), applied in priority
-- order with an optional cap. Scoped to a school, scheme, grade or student.
CREATE TABLE bill_discounts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    name            text NOT NULL,
    kind            text NOT NULL CHECK (kind IN ('percent','nominal')),
    value           bigint NOT NULL CHECK (value >= 0),
    cap_idr         bigint CHECK (cap_idr IS NULL OR cap_idr >= 0),
    priority        integer NOT NULL DEFAULT 100,
    scope           text NOT NULL DEFAULT 'school' CHECK (scope IN ('school','scheme','grade','student')),
    scheme_id       uuid REFERENCES billing_schemes(id),
    grade_level     text,
    student_user_id uuid,
    scholarship     boolean NOT NULL DEFAULT false,
    active          boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (kind <> 'percent' OR value <= 10000),
    CHECK (scope <> 'scheme' OR scheme_id IS NOT NULL),
    CHECK (scope <> 'grade' OR grade_level IS NOT NULL),
    CHECK (scope <> 'student' OR student_user_id IS NOT NULL)
);
CREATE INDEX bill_discounts_school_idx ON bill_discounts (school_id, active, priority);

-- Immutable per-step audit snapshot of every discount applied to a bill.
CREATE TABLE bill_discount_steps (
    id             bigserial PRIMARY KEY,
    school_id      uuid NOT NULL,
    bill_id        uuid NOT NULL REFERENCES student_bills(id) ON DELETE CASCADE,
    discount_id    uuid REFERENCES bill_discounts(id),
    sequence       integer NOT NULL,
    source         text NOT NULL,
    label          text NOT NULL,
    priority       integer NOT NULL,
    base_before_idr bigint NOT NULL,
    requested_idr  bigint NOT NULL,
    applied_idr    bigint NOT NULL,
    base_after_idr bigint NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bill_id, sequence)
);
CREATE INDEX bill_discount_steps_school_bill_idx ON bill_discount_steps (school_id, bill_id);

-- Transfer-proof submissions. The proof itself lives in private object
-- storage (object_key); the database only keeps metadata + checksum.
CREATE TABLE bill_payments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    student_bill_id uuid NOT NULL REFERENCES student_bills(id),
    amount_idr      bigint NOT NULL CHECK (amount_idr > 0),
    transfer_at     timestamptz,
    bank_name       text,
    account_name    text,
    reference       text,
    object_key      text NOT NULL,
    file_name       text,
    mime            text NOT NULL,
    size_bytes      bigint NOT NULL CHECK (size_bytes > 0),
    sha256          text NOT NULL,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','verified','rejected')),
    review_note     text,
    idempotency_key text NOT NULL,
    uploader_user_id uuid NOT NULL,
    verified_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key),
    CHECK (status <> 'verified' OR verified_at IS NOT NULL)
);
CREATE INDEX bill_payments_school_status_idx ON bill_payments (school_id, status, created_at DESC);
CREATE INDEX bill_payments_school_bill_idx ON bill_payments (school_id, student_bill_id);

CREATE TABLE bill_payment_allocations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id    uuid NOT NULL,
    payment_id   uuid NOT NULL REFERENCES bill_payments(id),
    bill_item_id uuid REFERENCES bill_items(id),
    amount_idr   bigint NOT NULL CHECK (amount_idr > 0),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bill_payment_allocations_school_payment_idx ON bill_payment_allocations (school_id, payment_id);

-- Verified payments are immutable; a correction is an append-only reversal
-- with a mandatory reason. Outstanding = net - verified + reversals.
CREATE TABLE bill_payment_reversals (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    payment_id      uuid NOT NULL REFERENCES bill_payments(id),
    amount_idr      bigint NOT NULL CHECK (amount_idr > 0),
    reason          text NOT NULL,
    actor_id        uuid NOT NULL,
    idempotency_key text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX bill_payment_reversals_school_payment_idx ON bill_payment_reversals (school_id, payment_id);

-- Append-only financial audit trail.
CREATE TABLE billing_audit_events (
    id             bigserial PRIMARY KEY,
    school_id      uuid NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id   text NOT NULL,
    action         text NOT NULL,
    actor_type     text NOT NULL,
    actor_id       text NOT NULL,
    actor_name     text NOT NULL DEFAULT '',
    before_data    jsonb,
    after_data     jsonb,
    reason         text,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX billing_audit_events_lookup_idx ON billing_audit_events (school_id, aggregate_type, aggregate_id, created_at DESC);
