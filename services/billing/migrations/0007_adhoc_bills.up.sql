-- 0007: BILL-ADHOC (T12 §2b). Ad-hoc bills raised by other modules
-- (library fines, transport, events, extracurricular) live in the same
-- student_bills ledger with scheme_id NULL, so they show up and are paid
-- through every existing bill flow. A bill is either a scheme bill or an
-- ad-hoc bill, never both. The old UNIQUE(school_id, student_user_id,
-- period_year, period_month, scheme_id) does not apply to scheme_id NULL
-- rows (Postgres NULL semantics), so scheme bills stay protected.

ALTER TABLE student_bills
    ALTER COLUMN scheme_id DROP NOT NULL,
    ADD COLUMN source_module text NULL CHECK (source_module IN ('library','transport','event','extracurricular')),
    ADD COLUMN source_ref text NULL,
    ADD CONSTRAINT student_bills_scheme_xor_source CHECK ((scheme_id IS NOT NULL) <> (source_module IS NOT NULL)),
    ADD CONSTRAINT student_bills_source_ref_pair CHECK ((source_module IS NULL) = (source_ref IS NULL));

CREATE UNIQUE INDEX student_bills_source_uq ON student_bills (school_id, source_module, source_ref) WHERE source_module IS NOT NULL;

-- Append-only adjustment history; the bill's net is recomputed from it.
CREATE TABLE bill_adjustments (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          uuid NOT NULL,
    bill_id            uuid NOT NULL REFERENCES student_bills(id),
    kind               text NOT NULL CHECK (kind IN ('prorate','adjust')),
    delta_idr          bigint NOT NULL CHECK (delta_idr <> 0),
    net_before_idr     bigint NOT NULL,
    net_after_idr      bigint NOT NULL CHECK (net_after_idr >= 0),
    deposit_idr        bigint NOT NULL DEFAULT 0 CHECK (deposit_idr >= 0),
    reason             text NOT NULL CHECK (btrim(reason) <> ''),
    source_ref_version text NULL,
    idempotency_key    text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX bill_adjustments_bill_idx ON bill_adjustments (school_id, bill_id, created_at);

-- Adjustment lines are kept next to the original items (which are never
-- deleted); only they may carry a negative amount.
ALTER TABLE bill_items ADD COLUMN adjustment_id uuid NULL REFERENCES bill_adjustments(id);
ALTER TABLE bill_items DROP CONSTRAINT bill_items_amount_idr_check;
ALTER TABLE bill_items DROP CONSTRAINT bill_items_net_idr_check;
ALTER TABLE bill_items ADD CONSTRAINT bill_items_amount_idr_check CHECK (amount_idr >= 0 OR adjustment_id IS NOT NULL);
ALTER TABLE bill_items ADD CONSTRAINT bill_items_net_idr_check CHECK (net_idr >= 0 OR adjustment_id IS NOT NULL);
