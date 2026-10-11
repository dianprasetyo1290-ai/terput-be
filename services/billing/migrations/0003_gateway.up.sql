-- 0003: Payment gateway (NICEPay SNAP), checkout, receipts, cashier,
-- corrections, settlement, reconciliation and daily journal posting (T6).
-- Money is bigint rupiah, percentages are basis points.

-- Payment methods offered per school (AC-28-01, BR-PAY-02 per method).
CREATE TABLE payment_methods (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     uuid NOT NULL,
    code          text NOT NULL,
    kind          text NOT NULL CHECK (kind IN ('va','qris','ewallet')),
    provider_code text NOT NULL,
    label         text NOT NULL,
    enabled       boolean NOT NULL DEFAULT true,
    fee_kind      text NOT NULL CHECK (fee_kind IN ('flat','percent')),
    fee_flat_idr  bigint NOT NULL DEFAULT 0 CHECK (fee_flat_idr >= 0),
    fee_bp        integer NOT NULL DEFAULT 0 CHECK (fee_bp BETWEEN 0 AND 10000),
    fee_bearer    text NOT NULL CHECK (fee_bearer IN ('parent','school')),
    valid_minutes integer NOT NULL CHECK (valid_minutes > 0),
    sort          integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, code)
);
CREATE INDEX payment_methods_school_idx ON payment_methods (school_id, enabled, sort);

-- Per-school gapless transaction number ('TX-' + sequence).
CREATE TABLE gateway_tx_sequences (
    school_id uuid PRIMARY KEY,
    last      bigint NOT NULL
);

-- One online payment attempt covering one or more bills (AC-27-01).
-- partner_ref is the id NICEPay sees (trxId / partnerReferenceNo): derived
-- from the uuid so it is unique across schools sharing one merchant account.
CREATE TABLE gateway_transactions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_ref         text GENERATED ALWAYS AS (replace(id::text, '-', '')) STORED UNIQUE,
    school_id           uuid NOT NULL,
    tx_no               text NOT NULL,
    payer_user_id       uuid NOT NULL,
    student_user_id     uuid NOT NULL,
    method_id           uuid NOT NULL REFERENCES payment_methods(id),
    bills_amount_idr    bigint NOT NULL CHECK (bills_amount_idr > 0),
    fee_idr             bigint NOT NULL DEFAULT 0 CHECK (fee_idr >= 0),
    total_idr           bigint NOT NULL CHECK (total_idr > 0),
    deposit_idr         bigint NOT NULL DEFAULT 0 CHECK (deposit_idr >= 0),
    fee_bearer          text NOT NULL CHECK (fee_bearer IN ('parent','school')),
    status              text NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending','success','failed','expired','cancelled','corrected')),
    provider_ref        text NULL,
    va_number           text NULL,
    qr_content          text NULL,
    redirect_url        text NULL,
    expires_at          timestamptz NOT NULL,
    paid_at             timestamptz NULL,
    failure_reason      text NULL,
    settlement_batch_id uuid NULL,
    corrected_by_id     uuid NULL,
    renewed_from_id     uuid NULL REFERENCES gateway_transactions(id),
    idempotency_key     text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, tx_no),
    UNIQUE (school_id, idempotency_key),
    UNIQUE (provider_ref),
    CHECK (status NOT IN ('success','corrected') OR paid_at IS NOT NULL)
);
CREATE INDEX gateway_transactions_lookup_idx ON gateway_transactions (school_id, status, created_at DESC);
CREATE INDEX gateway_transactions_student_idx ON gateway_transactions (school_id, student_user_id);
CREATE INDEX gateway_transactions_paid_idx ON gateway_transactions (school_id, paid_at) WHERE paid_at IS NOT NULL;

CREATE TABLE gateway_transaction_bills (
    transaction_id uuid NOT NULL REFERENCES gateway_transactions(id) ON DELETE CASCADE,
    bill_id        uuid NOT NULL REFERENCES student_bills(id),
    position       integer NOT NULL,
    amount_idr     bigint NOT NULL CHECK (amount_idr > 0),
    PRIMARY KEY (transaction_id, bill_id)
);
CREATE INDEX gateway_tx_bills_bill_idx ON gateway_transaction_bills (bill_id);

-- Every webhook received, valid or not (BR-PAY-01, AC-IF-01). The same
-- notification body is processed once: (provider_ref, raw_hash) is unique
-- and processed_at marks completion, so a retry after a failure re-runs.
CREATE TABLE gateway_notifications (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NULL,
    provider_ref    text NOT NULL,
    signature_valid boolean NOT NULL,
    raw_hash        text NOT NULL,
    body_masked     jsonb NOT NULL,
    processed_at    timestamptz NULL,
    result          text NULL,
    received_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_ref, raw_hash)
);
CREATE INDEX gateway_notifications_ref_idx ON gateway_notifications (provider_ref, received_at DESC);

-- Digital receipts (FR-29-003/004). One receipt per source.
CREATE TABLE receipts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id    uuid NOT NULL,
    receipt_no   text NOT NULL,
    source       text NOT NULL CHECK (source IN ('gateway','cashier')),
    source_id    uuid NOT NULL,
    object_key   text NULL,
    sent_at      timestamptz NULL,
    last_error   text NULL,
    resend_count integer NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, receipt_no),
    UNIQUE (source, source_id)
);
CREATE INDEX receipts_unsent_idx ON receipts (created_at) WHERE sent_at IS NULL;

-- Gapless receipt numbering: taken with INSERT .. ON CONFLICT .. RETURNING
-- in the same DB transaction that inserts the receipt.
CREATE TABLE receipt_sequences (
    school_id uuid NOT NULL,
    year      integer NOT NULL,
    last      integer NOT NULL,
    PRIMARY KEY (school_id, year)
);

-- Cash / bank transfer received at the school office (AC-29-03).
CREATE TABLE cashier_payments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    student_user_id uuid NOT NULL,
    method          text NOT NULL CHECK (method IN ('cash','transfer')),
    amount_idr      bigint NOT NULL CHECK (amount_idr > 0),
    deposit_idr     bigint NOT NULL DEFAULT 0 CHECK (deposit_idr >= 0),
    received_by     uuid NOT NULL,
    note            text NOT NULL DEFAULT '',
    status          text NOT NULL DEFAULT 'recorded' CHECK (status IN ('recorded','corrected','cancelled')),
    corrected_by_id uuid NULL,
    idempotency_key text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX cashier_payments_school_idx ON cashier_payments (school_id, created_at DESC);

-- Corrections / cancellations, applied only after core approval
-- (BR-FIN-05, AC-29-04). The source row is kept and marked.
CREATE TABLE payment_corrections (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              uuid NOT NULL,
    source                 text NOT NULL CHECK (source IN ('gateway','cashier','manual')),
    source_id              uuid NOT NULL,
    kind                   text NOT NULL CHECK (kind IN ('reassign','cancel')),
    target_student_user_id uuid NULL,
    reason                 text NOT NULL,
    status                 text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    approval_id            text NULL,
    requested_by           uuid NOT NULL,
    decided_by             text NULL,
    decision_comment       text NULL,
    decided_at             timestamptz NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CHECK (kind <> 'reassign' OR target_student_user_id IS NOT NULL)
);
CREATE INDEX payment_corrections_school_idx ON payment_corrections (school_id, status, created_at DESC);
CREATE UNIQUE INDEX payment_corrections_one_pending ON payment_corrections (source, source_id) WHERE status = 'pending';

-- Settlement report batches from the gateway (FR-30-001).
CREATE TABLE settlement_batches (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    provider        text NOT NULL,
    channel         text NOT NULL,
    settled_on      date NOT NULL,
    gross_idr       bigint NOT NULL DEFAULT 0 CHECK (gross_idr >= 0),
    fee_idr         bigint NOT NULL DEFAULT 0 CHECK (fee_idr >= 0),
    net_idr         bigint NOT NULL DEFAULT 0,
    bank_account    text NOT NULL,
    status          text NOT NULL DEFAULT 'reported' CHECK (status IN ('reported','matched','mismatch')),
    source_file_key text NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, provider, channel, settled_on)
);
CREATE INDEX settlement_batches_school_idx ON settlement_batches (school_id, settled_on DESC);

CREATE TABLE settlement_items (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              uuid NOT NULL,
    batch_id               uuid NOT NULL REFERENCES settlement_batches(id) ON DELETE CASCADE,
    provider_ref           text NOT NULL,
    amount_idr             bigint NOT NULL CHECK (amount_idr > 0),
    fee_idr                bigint NOT NULL DEFAULT 0 CHECK (fee_idr >= 0),
    matched_transaction_id uuid NULL REFERENCES gateway_transactions(id),
    created_at             timestamptz NOT NULL DEFAULT now(),
    UNIQUE (batch_id, provider_ref)
);
CREATE INDEX settlement_items_ref_idx ON settlement_items (school_id, provider_ref);

-- Reconciliation exception report (FR-31-003, AC-31-01/02). One row per
-- (kind, ref): re-running the reconciliation updates instead of duplicating.
CREATE TABLE reconciliation_exceptions (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              uuid NOT NULL,
    kind                   text NOT NULL CHECK (kind IN ('missing_in_settlement','missing_in_sekoci','amount_mismatch','bank_unmatched')),
    ref                    text NOT NULL,
    detail                 jsonb NOT NULL DEFAULT '{}',
    status                 text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    resolution_note        text NULL,
    adjustment_lines       jsonb NULL,
    adjustment_approval_id text NULL,
    resolved_by            text NULL,
    resolved_at            timestamptz NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, kind, ref)
);
CREATE INDEX reconciliation_exceptions_school_idx ON reconciliation_exceptions (school_id, status, created_at DESC);

-- Imported bank statement lines (FR-31-002). line_hash makes re-import idempotent.
CREATE TABLE bank_statement_lines (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   uuid NOT NULL,
    account     text NOT NULL,
    posted_on   date NOT NULL,
    amount_idr  bigint NOT NULL,
    description text NOT NULL DEFAULT '',
    matched_ref text NULL,
    line_hash   text NOT NULL,
    import_id   uuid NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, line_hash)
);
CREATE INDEX bank_statement_lines_school_idx ON bank_statement_lines (school_id, account, posted_on);

-- Per-school gateway rules. Defaults follow the PRD bracket values.
CREATE TABLE gateway_settings (
    school_id                 uuid PRIMARY KEY,
    auto_allocation           text NOT NULL DEFAULT 'earliest_due' CHECK (auto_allocation IN ('earliest_due','component_priority','payer_choice')),
    receipt_deadline_minutes  integer NOT NULL DEFAULT 5 CHECK (receipt_deadline_minutes > 0),
    sub_account_mode          boolean NOT NULL DEFAULT false,
    sub_account_id            text NULL,
    bank_match_tolerance_days integer NOT NULL DEFAULT 2 CHECK (bank_match_tolerance_days >= 0),
    journal_max_attempts      integer NOT NULL DEFAULT 5 CHECK (journal_max_attempts > 0),
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now()
);

-- Chart-of-accounts mapping for the daily gateway journal. No defaults:
-- account codes belong to each school's accounting setup.
CREATE TABLE gateway_accounts (
    school_id       uuid PRIMARY KEY,
    clearing_acc    text NOT NULL,
    fee_expense_acc text NOT NULL,
    receivable_acc  text NOT NULL,
    deposit_acc     text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Journal outbox for accounting POST /api/internal/journals (AC-IF-03):
-- daily gateway postings and approved reconciliation adjustments.
CREATE TABLE gateway_journal_postings (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    reference       text NOT NULL UNIQUE,
    trans_date      date NOT NULL,
    description     text NOT NULL,
    lines           jsonb NOT NULL,
    exception_id    uuid NULL REFERENCES reconciliation_exceptions(id),
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','posted','failed')),
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text NULL,
    trans_no        text NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX gateway_journal_postings_due_idx ON gateway_journal_postings (next_attempt_at) WHERE status = 'pending';

-- Once-per-day job markers (daily reconciliation).
CREATE TABLE gateway_job_runs (
    job        text NOT NULL,
    run_on     date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job, run_on)
);

-- Overpayment deposit per student (BR-FIN-04).
CREATE TABLE student_deposits (
    school_id       uuid NOT NULL,
    student_user_id uuid NOT NULL,
    balance_idr     bigint NOT NULL DEFAULT 0 CHECK (balance_idr >= 0),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (school_id, student_user_id)
);

CREATE TABLE deposit_movements (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         uuid NOT NULL,
    student_user_id   uuid NOT NULL,
    amount_idr        bigint NOT NULL CHECK (amount_idr <> 0),
    balance_after_idr bigint NOT NULL CHECK (balance_after_idr >= 0),
    reason            text NOT NULL,
    ref               text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX deposit_movements_student_idx ON deposit_movements (school_id, student_user_id, created_at DESC);
CREATE INDEX deposit_movements_ref_idx ON deposit_movements (school_id, ref);

-- Gateway, cashier and deposit payments land in the same ledger as uploaded
-- transfer proofs, so bill totals stay one query. Only uploads carry a proof file.
ALTER TABLE bill_payments
    ADD COLUMN source text NOT NULL DEFAULT 'upload' CHECK (source IN ('upload','gateway','cashier','deposit')),
    ADD COLUMN source_id uuid NULL,
    ALTER COLUMN object_key DROP NOT NULL,
    ALTER COLUMN mime DROP NOT NULL,
    ALTER COLUMN size_bytes DROP NOT NULL,
    ALTER COLUMN sha256 DROP NOT NULL,
    ADD CONSTRAINT bill_payments_proof_required CHECK (
        source <> 'upload' OR (object_key IS NOT NULL AND mime IS NOT NULL AND size_bytes IS NOT NULL AND sha256 IS NOT NULL)
    ),
    ADD CONSTRAINT bill_payments_source_id_required CHECK (source = 'upload' OR source_id IS NOT NULL);
CREATE INDEX bill_payments_source_idx ON bill_payments (school_id, source, source_id) WHERE source <> 'upload';
