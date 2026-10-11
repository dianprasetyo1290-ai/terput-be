-- PRD v2.1 gaps (menus 09-16): two-stage billing, promo validity/quota/cap,
-- manual discount approval, staged documents, scheduled announcements.

-- FR-09-006: fee per wave + uang pangkal + minimum daftar ulang (BR-PPDB-04).
ALTER TABLE fee_schemes ADD COLUMN IF NOT EXISTS wave_id uuid REFERENCES admission_waves(id);
ALTER TABLE fee_schemes ADD COLUMN IF NOT EXISTS entrance_fee_idr bigint NOT NULL DEFAULT 0;
ALTER TABLE fee_schemes ADD COLUMN IF NOT EXISTS min_reregistration_pct integer NOT NULL DEFAULT 100;

-- Each charge belongs to one billing stage: formulir or daftar ulang.
ALTER TABLE charges ADD COLUMN IF NOT EXISTS stage text NOT NULL DEFAULT 'registration';

-- FR-09-005: required documents per stage.
ALTER TABLE document_requirements ADD COLUMN IF NOT EXISTS stage text NOT NULL DEFAULT 'registration';

-- FR-09-007 / BR-PPDB-01: promo validity, quota, target fee component.
ALTER TABLE discount_rules ADD COLUMN IF NOT EXISTS valid_from timestamptz;
ALTER TABLE discount_rules ADD COLUMN IF NOT EXISTS valid_until timestamptz;
ALTER TABLE discount_rules ADD COLUMN IF NOT EXISTS quota integer NOT NULL DEFAULT 0;
ALTER TABLE discount_rules ADD COLUMN IF NOT EXISTS applies_to text NOT NULL DEFAULT 'reregistration';

-- BR-PPDB-02: combination mode + max discount per applicant.
ALTER TABLE admission_periods ADD COLUMN IF NOT EXISTS discount_mode text NOT NULL DEFAULT 'stack';
ALTER TABLE admission_periods ADD COLUMN IF NOT EXISTS max_discount_idr bigint NOT NULL DEFAULT 0;

-- BR-PPDB-03: manual discounts need kepala unit approval. Legacy rows stay approved.
ALTER TABLE manual_discounts ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'approved';
ALTER TABLE manual_discounts ADD COLUMN IF NOT EXISTS created_by_name text;
ALTER TABLE manual_discounts ADD COLUMN IF NOT EXISTS decided_by text;
ALTER TABLE manual_discounts ADD COLUMN IF NOT EXISTS decided_by_name text;
ALTER TABLE manual_discounts ADD COLUMN IF NOT EXISTS decided_at timestamptz;
ALTER TABLE manual_discounts ADD COLUMN IF NOT EXISTS decided_note text;

-- FR-13-005 / FR-14: committee-approved, scheduled announcements.
CREATE TABLE IF NOT EXISTS ppdb_announcements(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    period_id uuid NOT NULL REFERENCES admission_periods(id),
    wave_id uuid REFERENCES admission_waves(id),
    title text NOT NULL,
    message text NOT NULL DEFAULT '',
    publish_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK(status IN('draft','approved','published','cancelled')),
    created_by text NOT NULL,
    created_by_name text,
    approved_by text,
    approved_by_name text,
    approved_at timestamptz,
    published_at timestamptz,
    notified_count integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ppdb_announcements_due_idx ON ppdb_announcements(status,publish_at);
CREATE INDEX IF NOT EXISTS ppdb_announcements_school_idx ON ppdb_announcements(school_id,period_id);
