-- T18: Perpustakaan (0040_library.up.sql)
-- Katalog & OPAC, Keanggotaan, Sirkulasi, Denda, Kunjungan

CREATE TABLE lib_settings (
    school_id UUID PRIMARY KEY,
    fine_policy TEXT NOT NULL DEFAULT 'pay_at_library' CHECK (fine_policy IN ('pay_at_library','bill')),
    reminder_days_before INT NOT NULL DEFAULT 2 CHECK (reminder_days_before >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE lib_member_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    applies_to TEXT NOT NULL CHECK (applies_to IN ('student','employee')),
    is_default BOOLEAN NOT NULL DEFAULT false,
    max_loans INT NOT NULL CHECK (max_loans > 0),
    loan_days INT NOT NULL CHECK (loan_days > 0),
    max_renewals INT NOT NULL DEFAULT 1 CHECK (max_renewals >= 0),
    reservation_hold_days INT NOT NULL DEFAULT 2 CHECK (reservation_hold_days > 0),
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_lib_member_categories_default ON lib_member_categories(school_id, applies_to) WHERE is_default AND deleted_at IS NULL;

CREATE TABLE lib_fine_rates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    member_category_id UUID NULL REFERENCES lib_member_categories(id) ON DELETE SET NULL,
    late_fee_per_day_idr BIGINT NOT NULL CHECK (late_fee_per_day_idr >= 0),
    lost_fee_idr BIGINT NULL CHECK (lost_fee_idr IS NULL OR lost_fee_idr >= 0),
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE TABLE lib_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('student','employee')),
    user_id UUID NOT NULL,
    category_id UUID NOT NULL REFERENCES lib_member_categories(id),
    member_code TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','pending_close','inactive')),
    pending_reason TEXT NULL,
    closed_at TIMESTAMPTZ NULL,
    synced_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_lib_members_user ON lib_members(school_id, user_id);
CREATE UNIQUE INDEX uq_lib_members_code ON lib_members(school_id, member_code);

CREATE TABLE lib_collection_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_lib_collection_categories_code ON lib_collection_categories(school_id, code) WHERE deleted_at IS NULL;

CREATE TABLE lib_titles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    title TEXT NOT NULL,
    author TEXT NOT NULL,
    publisher TEXT NOT NULL,
    isbn TEXT NULL,
    publish_year INT NULL,
    category_id UUID NOT NULL REFERENCES lib_collection_categories(id),
    replacement_price_idr BIGINT NULL CHECK (replacement_price_idr IS NULL OR replacement_price_idr >= 0),
    search TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(author,'') || ' ' || coalesce(publisher,'') || ' ' || coalesce(isbn,''))) STORED,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_lib_titles_search ON lib_titles USING GIN(search);

CREATE TABLE lib_copies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    title_id UUID NOT NULL REFERENCES lib_titles(id),
    barcode TEXT NOT NULL,
    shelf_location TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'available' CHECK (status IN ('available','on_loan','on_hold','lost','damaged','withdrawn')),
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_lib_copies_barcode ON lib_copies(school_id, barcode) WHERE deleted_at IS NULL;
CREATE INDEX idx_lib_copies_title ON lib_copies(title_id, status) WHERE deleted_at IS NULL;

CREATE TABLE lib_loans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    copy_id UUID NOT NULL REFERENCES lib_copies(id),
    member_id UUID NOT NULL REFERENCES lib_members(id),
    classroom_id UUID NULL,
    classroom_name TEXT NULL,
    loaned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    due_date DATE NOT NULL,
    returned_at TIMESTAMPTZ NULL,
    renew_count INT NOT NULL DEFAULT 0 CHECK (renew_count >= 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','returned','lost')),
    loaned_by UUID NOT NULL,
    returned_by UUID NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_lib_loans_idempotency ON lib_loans(school_id, idempotency_key);
CREATE UNIQUE INDEX uq_lib_loans_active_copy ON lib_loans(copy_id) WHERE status = 'active';
CREATE INDEX idx_lib_loans_member ON lib_loans(member_id, status);

CREATE TABLE lib_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    title_id UUID NOT NULL REFERENCES lib_titles(id),
    member_id UUID NOT NULL REFERENCES lib_members(id),
    status TEXT NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting','ready','fulfilled','cancelled','expired')),
    held_copy_id UUID NULL REFERENCES lib_copies(id),
    ready_until DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_lib_reservations_active ON lib_reservations(title_id, member_id) WHERE status IN ('waiting','ready');

CREATE TABLE lib_fines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    loan_id UUID NOT NULL REFERENCES lib_loans(id),
    member_id UUID NOT NULL REFERENCES lib_members(id),
    kind TEXT NOT NULL CHECK (kind IN ('late','lost')),
    late_days INT NULL,
    amount_idr BIGINT NOT NULL CHECK (amount_idr >= 0),
    rate_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    settlement TEXT NOT NULL CHECK (settlement IN ('pay_at_library','bill')),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','paid','billed','waived','void')),
    bill_id UUID NULL,
    paid_at TIMESTAMPTZ NULL,
    received_by UUID NULL,
    waived_by UUID NULL,
    waive_reason TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_lib_fines_loan_kind ON lib_fines(loan_id, kind);
CREATE INDEX idx_lib_fines_member ON lib_fines(member_id, status);

CREATE TABLE lib_visits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    member_id UUID NOT NULL REFERENCES lib_members(id),
    visited_on DATE NOT NULL,
    visited_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_lib_visits_member_day ON lib_visits(member_id, visited_on);
