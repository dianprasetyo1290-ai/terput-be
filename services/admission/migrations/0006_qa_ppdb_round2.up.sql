-- 0006: QA PPDB round 2. Configurable registration form fields (BUG-095),
-- waves per jenjang/entry grade (BUG-093), refund policy + payout records
-- for dropped applicants (BUG-040), and one guardian per SEKOCI parent account
-- (BUG-108). Additive and reversible (see 0006_qa_ppdb_round2.down.sql).

-- ---------------------------------------------------------------------------
-- Form builder (BUG-095)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS admission_form_fields(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    period_id uuid NOT NULL REFERENCES admission_periods(id),
    field_key text NOT NULL CHECK(field_key ~ '^[a-z][a-z0-9_]{1,39}$'),
    label text NOT NULL,
    field_type text NOT NULL CHECK(field_type IN('text','textarea','number','date','select','checkbox')),
    options jsonb NOT NULL DEFAULT '[]',
    required boolean NOT NULL DEFAULT false,
    help_text text NOT NULL DEFAULT '',
    sort_order integer NOT NULL DEFAULT 0,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(school_id,period_id,field_key)
);
CREATE INDEX IF NOT EXISTS admission_form_fields_period_idx ON admission_form_fields(school_id,period_id,sort_order);

ALTER TABLE applicants ADD COLUMN IF NOT EXISTS custom_fields jsonb NOT NULL DEFAULT '{}';

-- ---------------------------------------------------------------------------
-- Waves per jenjang / entry grade (BUG-093). The unit is the school tenant
-- (school_id); a unit may run separate waves per jenjang.
-- ---------------------------------------------------------------------------
ALTER TABLE admission_waves
    ADD COLUMN IF NOT EXISTS education_level text CHECK(education_level IS NULL OR education_level IN('TK','SD','SMP','SMA','SMK')),
    ADD COLUMN IF NOT EXISTS entry_grade integer CHECK(entry_grade IS NULL OR entry_grade BETWEEN 0 AND 12);

-- ---------------------------------------------------------------------------
-- Refunds for dropped applicants (BUG-040)
-- ---------------------------------------------------------------------------
ALTER TABLE admission_periods
    ADD COLUMN IF NOT EXISTS refund_policy_bp integer NOT NULL DEFAULT 10000 CHECK(refund_policy_bp BETWEEN 0 AND 10000),
    ADD COLUMN IF NOT EXISTS refund_admin_fee_idr bigint NOT NULL DEFAULT 0 CHECK(refund_admin_fee_idr>=0);

CREATE TABLE IF NOT EXISTS admission_refunds(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    application_id uuid NOT NULL REFERENCES applications(id),
    stage text NOT NULL CHECK(stage IN('registration','reregistration')),
    paid_idr bigint NOT NULL CHECK(paid_idr>=0),
    refundable_idr bigint NOT NULL CHECK(refundable_idr>=0),
    policy_bp integer NOT NULL,
    admin_fee_idr bigint NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'due' CHECK(status IN('due','paid','waived')),
    method text CHECK(method IS NULL OR method IN('transfer','cash')),
    bank_name text,
    account_name text,
    account_number text,
    reference text,
    paid_at timestamptz,
    note text,
    processed_by text,
    processed_by_name text,
    processed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(application_id,stage)
);
CREATE INDEX IF NOT EXISTS admission_refunds_school_idx ON admission_refunds(school_id,status,created_at DESC);

-- ---------------------------------------------------------------------------
-- SEKOCI parent account <-> PPDB guardian (BUG-108)
-- ---------------------------------------------------------------------------
-- Deployed data has accounts linked to two guardian rows (an empty seed row
-- plus the row the parent registered with). Keep the link on the row with the
-- most applicants (ties: oldest) and park the others' link in
-- identity_user_id_unlinked — no row is deleted.
ALTER TABLE guardians ADD COLUMN IF NOT EXISTS identity_user_id_unlinked uuid;
WITH ranked AS (
    SELECT g.id, row_number() OVER (
        PARTITION BY g.school_id, g.identity_user_id
        ORDER BY (SELECT count(*) FROM applicants a WHERE a.guardian_id = g.id) DESC, g.created_at, g.id) AS rn
    FROM guardians g WHERE g.identity_user_id IS NOT NULL
)
UPDATE guardians g SET identity_user_id_unlinked = g.identity_user_id, identity_user_id = NULL, updated_at = now()
FROM ranked r WHERE r.id = g.id AND r.rn > 1;
CREATE UNIQUE INDEX IF NOT EXISTS guardians_identity_user_idx ON guardians(school_id,identity_user_id) WHERE identity_user_id IS NOT NULL;
