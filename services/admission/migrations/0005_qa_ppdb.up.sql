-- 0005: QA PPDB fixes. Per-applicant installments, staged charges, discount
-- origin/conditions, document versions, selection approval + scheduled
-- announcement, waitlist promotion, guardian notifications, per-applicant
-- test/interview schedules, BR-PPDB-07/08 drop bookkeeping. Additive and
-- reversible (see 0005_qa_ppdb.down.sql).

-- ---------------------------------------------------------------------------
-- Charges: per-applicant installments (BUG-003) and cancellation (BR-PPDB-07)
-- ---------------------------------------------------------------------------
ALTER TABLE charges
    ADD COLUMN IF NOT EXISTS cancelled_at timestamptz,
    ADD COLUMN IF NOT EXISTS cancel_reason text,
    ADD COLUMN IF NOT EXISTS min_payment_idr bigint NOT NULL DEFAULT 0 CHECK(min_payment_idr>=0),
    ADD COLUMN IF NOT EXISTS due_at timestamptz;
CREATE INDEX IF NOT EXISTS charges_app_stage_idx ON charges(school_id,application_id,stage,revision DESC);

CREATE TABLE IF NOT EXISTS charge_installments(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    charge_id uuid NOT NULL REFERENCES charges(id),
    application_id uuid NOT NULL REFERENCES applications(id),
    sequence integer NOT NULL CHECK(sequence>0),
    description text NOT NULL,
    amount_idr bigint NOT NULL CHECK(amount_idr>=0),
    due_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(charge_id,sequence)
);
CREATE INDEX IF NOT EXISTS charge_installments_app_idx ON charge_installments(school_id,application_id);

ALTER TABLE payment_allocations
    ADD COLUMN IF NOT EXISTS charge_installment_id uuid REFERENCES charge_installments(id);

-- Backfill: every legacy charge gets one installment for its net amount, due
-- at the wave payment deadline (or when the charge was issued).
INSERT INTO charge_installments(school_id,charge_id,application_id,sequence,description,amount_idr,due_at)
SELECT c.school_id,c.id,c.application_id,1,'Termin 1',c.net_idr,COALESCE(w.payment_end,c.finalized_at)
FROM charges c
JOIN applications a ON a.id=c.application_id
JOIN admission_waves w ON w.id=a.wave_id
ON CONFLICT (charge_id,sequence) DO NOTHING;

-- Legacy allocations pointed at the shared scheme plan; remap them to the
-- installment of the payment's own charge.
UPDATE payment_allocations pa
SET charge_installment_id=ci.id
FROM payment_submissions ps
JOIN charge_installments ci ON ci.charge_id=ps.charge_id AND ci.sequence=1
WHERE ps.id=pa.payment_id AND pa.charge_installment_id IS NULL;

-- ---------------------------------------------------------------------------
-- Discounts (BUG-011/032/092/097)
-- ---------------------------------------------------------------------------
ALTER TABLE discount_applications
    ADD COLUMN IF NOT EXISTS origin text NOT NULL DEFAULT 'rule' CHECK(origin IN('rule','voucher','manual'));
CREATE INDEX IF NOT EXISTS discount_applications_source_idx ON discount_applications(school_id,origin,source_id);

ALTER TABLE voucher_redemptions
    ADD COLUMN IF NOT EXISTS stage text NOT NULL DEFAULT 'registration' CHECK(stage IN('registration','reregistration'));

ALTER TABLE manual_discounts
    ADD COLUMN IF NOT EXISTS stage text NOT NULL DEFAULT 'reregistration' CHECK(stage IN('registration','reregistration')),
    ADD COLUMN IF NOT EXISTS approval_id uuid;
ALTER TABLE manual_discounts DROP CONSTRAINT IF EXISTS manual_discounts_status_check;
ALTER TABLE manual_discounts
    ADD CONSTRAINT manual_discounts_status_check CHECK(status IN('pending','approved','rejected','cancelled')) NOT VALID;
CREATE UNIQUE INDEX IF NOT EXISTS manual_discounts_approval_idx ON manual_discounts(approval_id) WHERE approval_id IS NOT NULL;

ALTER TABLE discount_rules DROP CONSTRAINT IF EXISTS discount_rules_applies_to_check;
ALTER TABLE discount_rules
    ADD CONSTRAINT discount_rules_applies_to_check CHECK(applies_to IN('registration','reregistration')) NOT VALID;
ALTER TABLE discount_rules DROP CONSTRAINT IF EXISTS discount_rules_quota_check;
ALTER TABLE discount_rules ADD CONSTRAINT discount_rules_quota_check CHECK(quota>=0) NOT VALID;

ALTER TABLE admission_periods DROP CONSTRAINT IF EXISTS admission_periods_discount_mode_check;
ALTER TABLE admission_periods
    ADD CONSTRAINT admission_periods_discount_mode_check CHECK(discount_mode IN('stack','max')) NOT VALID;

-- ---------------------------------------------------------------------------
-- Applications: drop (gugur) bookkeeping, deadlines, registration numbers
-- ---------------------------------------------------------------------------
ALTER TABLE applications
    ADD COLUMN IF NOT EXISTS reregistration_due_at timestamptz,
    ADD COLUMN IF NOT EXISTS dropped_at timestamptz,
    ADD COLUMN IF NOT EXISTS drop_reason text,
    ADD COLUMN IF NOT EXISTS refund_due_idr bigint NOT NULL DEFAULT 0 CHECK(refund_due_idr>=0);

ALTER TABLE admission_waves
    ADD COLUMN IF NOT EXISTS waitlist_reregistration_days integer NOT NULL DEFAULT 7 CHECK(waitlist_reregistration_days>=0);

-- TC-09-036: the form-payment deadline may trail registration close by at most
-- this many days (per school/period business rule).
ALTER TABLE admission_periods
    ADD COLUMN IF NOT EXISTS max_payment_grace_days integer NOT NULL DEFAULT 30 CHECK(max_payment_grace_days>=0);

CREATE TABLE IF NOT EXISTS registration_counters(
    wave_id uuid PRIMARY KEY REFERENCES admission_waves(id),
    school_id uuid NOT NULL,
    prefix text NOT NULL,
    last_value integer NOT NULL DEFAULT 0
);

-- ---------------------------------------------------------------------------
-- Documents: versions + optimistic lock (BUG-159/160)
-- ---------------------------------------------------------------------------
ALTER TABLE application_documents
    ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS lock_version integer NOT NULL DEFAULT 1;
ALTER TABLE document_reviews
    ADD COLUMN IF NOT EXISTS document_version integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS reviewer_name text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS application_document_versions(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    document_id uuid NOT NULL REFERENCES application_documents(id),
    application_id uuid NOT NULL REFERENCES applications(id),
    version integer NOT NULL,
    object_key text NOT NULL,
    file_name text,
    mime text NOT NULL,
    size_bytes bigint NOT NULL,
    sha256 text NOT NULL,
    uploaded_by text NOT NULL,
    uploaded_at timestamptz NOT NULL,
    archived_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(document_id,version)
);

-- ---------------------------------------------------------------------------
-- Selection result approval + publication (BUG-034/036)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS selection_approvals(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    period_id uuid NOT NULL REFERENCES admission_periods(id),
    wave_id uuid NOT NULL REFERENCES admission_waves(id),
    status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','approved','rejected','cancelled')),
    approval_id uuid,
    decision_count integer NOT NULL DEFAULT 0,
    created_by text NOT NULL,
    created_by_name text NOT NULL DEFAULT '',
    decided_by text,
    decided_comment text,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS selection_approvals_pending_idx ON selection_approvals(wave_id) WHERE status='pending';
CREATE UNIQUE INDEX IF NOT EXISTS selection_approvals_approval_idx ON selection_approvals(approval_id) WHERE approval_id IS NOT NULL;

-- Legacy decisions were already visible, so they default to published.
ALTER TABLE decisions
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'published',
    ADD COLUMN IF NOT EXISTS selection_approval_id uuid REFERENCES selection_approvals(id),
    ADD COLUMN IF NOT EXISTS published_at timestamptz;
ALTER TABLE decisions DROP CONSTRAINT IF EXISTS decisions_status_check;
ALTER TABLE decisions
    ADD CONSTRAINT decisions_status_check CHECK(status IN('draft','pending_approval','approved','published','superseded')) NOT VALID;
UPDATE decisions SET published_at=created_at WHERE published_at IS NULL AND status='published';

ALTER TABLE ppdb_announcements
    ADD COLUMN IF NOT EXISTS selection_approval_id uuid REFERENCES selection_approvals(id),
    ADD COLUMN IF NOT EXISTS cancelled_reason text;

-- ---------------------------------------------------------------------------
-- Waitlist promotion (BUG-035, BR-PPDB-09)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS waitlist_promotions(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    application_id uuid NOT NULL REFERENCES applications(id),
    period_id uuid NOT NULL REFERENCES admission_periods(id),
    wave_id uuid NOT NULL REFERENCES admission_waves(id),
    track_id uuid NOT NULL REFERENCES admission_tracks(id),
    total_score bigint,
    status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','approved','rejected','failed')),
    approval_id uuid,
    created_by text NOT NULL,
    created_by_name text NOT NULL DEFAULT '',
    decided_by text,
    decided_comment text,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS waitlist_promotions_pending_idx ON waitlist_promotions(application_id) WHERE status='pending';
CREATE UNIQUE INDEX IF NOT EXISTS waitlist_promotions_approval_idx ON waitlist_promotions(approval_id) WHERE approval_id IS NOT NULL;

-- Core approval callbacks are processed exactly once per approval.
CREATE TABLE IF NOT EXISTS approval_callback_events(
    approval_id uuid PRIMARY KEY,
    school_id uuid NOT NULL,
    request_type text NOT NULL,
    decision text NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Guardian notifications (BUG-037/104/105)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS guardian_notifications(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    guardian_id uuid NOT NULL REFERENCES guardians(id),
    application_id uuid REFERENCES applications(id),
    type text NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    data jsonb NOT NULL DEFAULT '{}',
    dedupe_key text NOT NULL,
    email_status text NOT NULL DEFAULT 'pending' CHECK(email_status IN('pending','sent','skipped','failed')),
    email_attempts integer NOT NULL DEFAULT 0,
    email_error text,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(school_id,dedupe_key)
);
CREATE INDEX IF NOT EXISTS guardian_notifications_inbox_idx ON guardian_notifications(school_id,guardian_id,created_at DESC);
CREATE INDEX IF NOT EXISTS guardian_notifications_outbox_idx ON guardian_notifications(email_status,created_at);

-- ---------------------------------------------------------------------------
-- Per-applicant test / interview schedule (BUG-105)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS selection_schedules(
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    application_id uuid NOT NULL REFERENCES applications(id),
    kind text NOT NULL CHECK(kind IN('test','interview')),
    scheduled_at timestamptz NOT NULL,
    location text NOT NULL DEFAULT '',
    notes text NOT NULL DEFAULT '',
    created_by text NOT NULL,
    created_by_name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(application_id,kind)
);

-- Score range is enforced in the handler against max_score; the floor is a
-- hard invariant (BUG-106).
ALTER TABLE component_scores DROP CONSTRAINT IF EXISTS component_scores_score_check;
ALTER TABLE component_scores ADD CONSTRAINT component_scores_score_check CHECK(score>=0) NOT VALID;
