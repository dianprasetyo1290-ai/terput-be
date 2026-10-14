-- QA-BILL: reminders, collection follow-ups and ledger-derived bill status.

-- Per-school reminder schedule (BUG-007). No row = no automatic reminders;
-- a school opts in from Pengaturan Tagihan. Offsets are calendar days in
-- the school time zone.
CREATE TABLE billing_reminder_settings (
    school_id   uuid PRIMARY KEY,
    enabled     boolean NOT NULL DEFAULT false,
    days_before integer[] NOT NULL DEFAULT '{}',
    on_due      boolean NOT NULL DEFAULT false,
    days_after  integer[] NOT NULL DEFAULT '{}',
    send_hour   smallint NOT NULL DEFAULT 8 CHECK (send_hour BETWEEN 0 AND 23),
    channels    text[] NOT NULL DEFAULT '{in_app}',
    updated_by  uuid NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- One row per reminder sent (or attempted) to one guardian. The
-- idempotency key makes the job safe to re-run; failed rows are retried
-- until attempts reaches the limit and then surface in core's exception
-- queue (BUG-173).
CREATE TABLE bill_reminder_log (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        uuid NOT NULL,
    student_user_id  uuid NOT NULL,
    bill_id          uuid NULL REFERENCES student_bills(id),
    guardian_user_id uuid NULL,
    kind             text NOT NULL CHECK (kind IN ('before_due','on_due','overdue','manual')),
    offset_days      integer NOT NULL DEFAULT 0,
    outstanding_idr  bigint NOT NULL DEFAULT 0 CHECK (outstanding_idr >= 0),
    status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed','skipped')),
    attempts         integer NOT NULL DEFAULT 0,
    error            text NULL,
    sent_by          uuid NULL,
    idempotency_key  text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX bill_reminder_log_student_idx ON bill_reminder_log (school_id, student_user_id, created_at DESC);
CREATE INDEX bill_reminder_log_status_idx ON bill_reminder_log (school_id, status) WHERE status IN ('pending','failed');

-- Collection follow-up log (BUG-133): calls, visits, promises to pay.
CREATE TABLE bill_followups (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          uuid NOT NULL,
    student_user_id    uuid NOT NULL,
    bill_id            uuid NULL REFERENCES student_bills(id),
    channel            text NOT NULL CHECK (channel IN ('phone','whatsapp','email','visit','letter','meeting','other')),
    outcome            text NOT NULL CHECK (length(btrim(outcome)) > 0),
    promise_date       date NULL,
    promise_amount_idr bigint NULL CHECK (promise_amount_idr IS NULL OR promise_amount_idr > 0),
    next_action_on     date NULL,
    created_by         uuid NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bill_followups_student_idx ON bill_followups (school_id, student_user_id, created_at DESC);

-- BUG-124/127/172: tariff validity window, free-component flag and the
-- fee-change approval queue (BUG-051).
ALTER TABLE billing_schemes
    ADD COLUMN effective_from date NULL,
    ADD COLUMN effective_to   date NULL,
    ADD CONSTRAINT billing_schemes_effective_window CHECK (effective_to IS NULL OR effective_from IS NULL OR effective_to >= effective_from);

ALTER TABLE billing_scheme_components ADD COLUMN is_free boolean NOT NULL DEFAULT false;
-- Existing zero tariffs were accepted as free; flag them so the new rule holds.
UPDATE billing_scheme_components SET is_free = true WHERE amount_idr = 0;
ALTER TABLE billing_scheme_components
    ADD CONSTRAINT billing_scheme_components_free_amount CHECK ((is_free AND amount_idr = 0) OR (NOT is_free AND amount_idr > 0));

-- A tariff change on an active scheme waits here until the Approval Center
-- decides (request_type fee_change). payload is the requested scheme,
-- before_data the scheme at request time.
CREATE TABLE billing_scheme_changes (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    scheme_id       uuid NOT NULL REFERENCES billing_schemes(id),
    status          text NOT NULL DEFAULT 'pending_approval'
                    CHECK (status IN ('pending_approval','applied','rejected','failed')),
    payload         jsonb NOT NULL,
    before_data     jsonb NOT NULL,
    approval_id     text NULL,
    requested_by    uuid NOT NULL,
    requested_role  text NOT NULL DEFAULT '',
    decided_by      text NULL,
    decision_note   text NULL,
    failure_reason  text NULL,
    idempotency_key text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    decided_at      timestamptz NULL,
    UNIQUE (school_id, idempotency_key)
);
CREATE UNIQUE INDEX billing_scheme_changes_one_pending ON billing_scheme_changes (scheme_id) WHERE status = 'pending_approval';
CREATE INDEX billing_scheme_changes_school_idx ON billing_scheme_changes (school_id, created_at DESC);

-- BUG-008: re-derive every stored bill status from the ledger (verified
-- allocations minus reversals vs net), the same rule as
-- model.DeriveBillStatus. Void bills are never touched. This is a data
-- repair; the down migration does not restore stale values.
WITH ledger AS (
    SELECT b.id, b.net_idr,
           COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id = al.payment_id
                      WHERE p.student_bill_id = b.id AND p.school_id = b.school_id AND p.status = 'verified'), 0)
         - COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id = rv.payment_id
                      WHERE p.student_bill_id = b.id AND p.school_id = b.school_id), 0) AS paid_idr
      FROM student_bills b
     WHERE b.status <> 'void'
), derived AS (
    SELECT id, CASE WHEN paid_idr <= 0 THEN 'unpaid' WHEN paid_idr >= net_idr THEN 'paid' ELSE 'partial' END AS status
      FROM ledger
)
UPDATE student_bills b
   SET status = d.status, updated_at = now()
  FROM derived d
 WHERE b.id = d.id AND b.status <> d.status;

-- BUG-043/044/135: refunds are decided in core's Approval Center (request
-- type 'refund', amount-based stages) and processed against a bank transfer
-- reference. Rows created before this migration keep the local decision
-- path (approval_id IS NULL) and are not required to carry a reference.
ALTER TABLE bill_refunds
    ADD COLUMN approval_id   text NULL,
    ADD COLUMN transfer_ref  text NULL,
    ADD COLUMN transfer_date date NULL,
    ADD COLUMN process_note  text NULL;
ALTER TABLE bill_refunds ADD CONSTRAINT bill_refunds_transfer_ref_required
    CHECK (status <> 'processed' OR approval_id IS NULL OR btrim(COALESCE(transfer_ref, '')) <> '');
CREATE UNIQUE INDEX bill_refunds_school_transfer_ref ON bill_refunds (school_id, transfer_ref) WHERE transfer_ref IS NOT NULL;

-- BUG-056: resolving a reconciliation exception is maker-checker through
-- core's Approval Center (request type 'recon_adjustment'). While waiting
-- the exception is 'pending_approval'; an approved adjustment journal goes
-- to the journal outbox (gateway_journal_postings.exception_id).
ALTER TABLE reconciliation_exceptions DROP CONSTRAINT IF EXISTS reconciliation_exceptions_status_check;
ALTER TABLE reconciliation_exceptions ADD CONSTRAINT reconciliation_exceptions_status_check
    CHECK (status IN ('open', 'pending_approval', 'resolved'));
ALTER TABLE reconciliation_exceptions
    ADD COLUMN requested_by  text NULL,
    ADD COLUMN requested_at  timestamptz NULL,
    ADD COLUMN decision_note text NULL;

-- BUG-163: per-school minimum gap between two resends of the same receipt.
ALTER TABLE gateway_settings ADD COLUMN receipt_resend_cooldown_minutes integer NOT NULL DEFAULT 10
    CHECK (receipt_resend_cooldown_minutes >= 0);
