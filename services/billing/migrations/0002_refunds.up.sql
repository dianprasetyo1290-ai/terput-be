-- 0002: Refund (PRD menu 25). A refund is requested against a bill, decided
-- (approve/reject) by a different admin than the requester (segregation of
-- duties), then processed. Processing writes append-only
-- bill_payment_reversals tagged with refund_id, so the bill's paid balance
-- drops through the same ledger math as any other correction.

CREATE TABLE IF NOT EXISTS bill_refunds (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       uuid NOT NULL,
    student_bill_id uuid NOT NULL REFERENCES student_bills(id),
    student_user_id uuid NOT NULL,
    amount_idr      bigint NOT NULL CHECK (amount_idr > 0),
    reason          text NOT NULL CHECK (btrim(reason) <> ''),
    status          text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested','approved','rejected','processed')),
    requested_by    uuid NOT NULL,
    decided_by      uuid,
    decided_at      timestamptz,
    decision_note   text,
    processed_by    uuid,
    processed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (status = 'requested' OR decided_by IS NOT NULL),
    CHECK (decided_by IS NULL OR decided_by <> requested_by),
    CHECK (status <> 'processed' OR processed_at IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS bill_refunds_school_status_idx ON bill_refunds (school_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS bill_refunds_school_bill_idx ON bill_refunds (school_id, student_bill_id);

ALTER TABLE bill_payment_reversals ADD COLUMN IF NOT EXISTS refund_id uuid REFERENCES bill_refunds(id);
CREATE INDEX IF NOT EXISTS bill_payment_reversals_refund_idx ON bill_payment_reversals (refund_id) WHERE refund_id IS NOT NULL;
