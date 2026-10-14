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
