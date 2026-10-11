-- 0002: PPDB portal slice — applicant biodata fields the public form sends,
-- payment proof metadata, admin config columns, CBT participant state, voucher
-- redemptions, and audit actor names. Additive and reversible.

-- Applicant biodata captured by the public registration form.
ALTER TABLE applicants
    ADD COLUMN IF NOT EXISTS nickname text,
    ADD COLUMN IF NOT EXISTS nik text,
    ADD COLUMN IF NOT EXISTS previous_school text,
    ADD COLUMN IF NOT EXISTS address text,
    ADD COLUMN IF NOT EXISTS guardian_relationship text;

-- Period registration window (the admin config form edits these directly).
ALTER TABLE admission_periods
    ADD COLUMN IF NOT EXISTS registration_open_at timestamptz,
    ADD COLUMN IF NOT EXISTS registration_close_at timestamptz;

-- Track presentation fields.
ALTER TABLE admission_tracks
    ADD COLUMN IF NOT EXISTS description text,
    ADD COLUMN IF NOT EXISTS is_active boolean NOT NULL DEFAULT true;

-- Requirement description shown to guardians.
ALTER TABLE document_requirements
    ADD COLUMN IF NOT EXISTS description text;

-- Original upload filename (object_key is opaque).
ALTER TABLE application_documents
    ADD COLUMN IF NOT EXISTS file_name text;

-- Payment proof metadata sent by the guardian upload form.
ALTER TABLE payment_submissions
    ADD COLUMN IF NOT EXISTS file_name text,
    ADD COLUMN IF NOT EXISTS transfer_at timestamptz,
    ADD COLUMN IF NOT EXISTS bank_name text,
    ADD COLUMN IF NOT EXISTS account_name text,
    ADD COLUMN IF NOT EXISTS reference text,
    ADD COLUMN IF NOT EXISTS review_note text;

-- Discount rule stackability + explicit status.
ALTER TABLE discount_rules
    ADD COLUMN IF NOT EXISTS stackable boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active';

-- Voucher status + non-reversible code hint (the plaintext code is never
-- stored; only a bcrypt hash and a short masked hint for the admin list).
ALTER TABLE discount_vouchers
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS code_hint text;

-- Fee scheme shortcuts used by the simplified admin fee form.
ALTER TABLE fee_schemes
    ADD COLUMN IF NOT EXISTS registration_fee_idr bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS reregistration_fee_idr bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS installment_count integer NOT NULL DEFAULT 1;

-- Audit actor display name (identity is not queried for every row).
ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS actor_name text NOT NULL DEFAULT '';

-- Conversion saga: expose the guardian link id and current step for resume.
ALTER TABLE student_conversions
    ADD COLUMN IF NOT EXISTS guardian_link_id text,
    ADD COLUMN IF NOT EXISTS current_step text NOT NULL DEFAULT '';

-- Recommendation vocabulary now matches the frontend ('waitlisted').
ALTER TABLE ranking_entries DROP CONSTRAINT IF EXISTS ranking_entries_recommendation_check;
ALTER TABLE ranking_entries
    ADD CONSTRAINT ranking_entries_recommendation_check CHECK(recommendation IN('accepted','waitlisted'));

-- One voucher may be applied to an application at most once.
CREATE TABLE IF NOT EXISTS voucher_redemptions(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    application_id uuid NOT NULL REFERENCES applications(id),
    voucher_id uuid NOT NULL REFERENCES discount_vouchers(id),
    redeemed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(application_id)
);

-- Admission-side mirror of the exam CBT participant so the guardian portal can
-- show a schedule/score without calling exam on every page load.
CREATE TABLE IF NOT EXISTS admission_cbt_participants(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    application_id uuid NOT NULL REFERENCES applications(id),
    exam_id text NOT NULL DEFAULT '',
    participant_ref text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'scheduled' CHECK(status IN('scheduled','available','completed','missed')),
    score bigint,
    scheduled_at timestamptz,
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(application_id)
);
CREATE INDEX IF NOT EXISTS admission_cbt_school_idx ON admission_cbt_participants(school_id,status);
