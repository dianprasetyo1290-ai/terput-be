-- 0008: Accounting auto-journals (PRD menu 34, BR-ACC-05/06, IF-ACC-01..03).
-- Account mapping per unit (school) and per fee component, plus the extra
-- outbox columns the sender needs. No default account codes: they belong to
-- each school's chart of accounts (menu 33).

CREATE TABLE billing_journal_settings (
    school_id       uuid PRIMARY KEY,
    unit_div_code   text NULL,
    receivable_acc  text NULL,
    cash_acc        text NULL,
    bank_acc        text NULL,
    discount_acc    text NULL,
    scholarship_acc text NULL,
    refund_acc      text NULL,
    deposit_acc     text NULL,
    updated_by      uuid NULL,
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- component_code matches bill_items.component_code (scheme bills) or
-- student_bills.source_module (ad-hoc bills: library, transport, ...).
CREATE TABLE fee_component_accounts (
    school_id      uuid NOT NULL,
    component_code text NOT NULL CHECK (btrim(component_code) <> ''),
    name           text NOT NULL DEFAULT '',
    revenue_acc    text NOT NULL CHECK (btrim(revenue_acc) <> ''),
    receivable_acc text NULL,
    updated_by     uuid NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (school_id, component_code)
);

-- source is the accounting "source" sent with the journal; accounting is
-- idempotent on (source, reference).
UPDATE gateway_journal_postings SET source = 'payment_gateway' WHERE source = 'gateway';
ALTER TABLE gateway_journal_postings
    ALTER COLUMN source SET DEFAULT 'payment_gateway',
    ADD COLUMN kind text NOT NULL DEFAULT 'gateway_receipt',
    ADD COLUMN source_refs jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN posted_at timestamptz NULL,
    ADD COLUMN exception_raised_at timestamptz NULL;
CREATE INDEX gateway_journal_postings_school_idx ON gateway_journal_postings (school_id, created_at DESC);
CREATE INDEX gateway_journal_postings_failed_idx ON gateway_journal_postings (id) WHERE status = 'failed' AND exception_raised_at IS NULL;
