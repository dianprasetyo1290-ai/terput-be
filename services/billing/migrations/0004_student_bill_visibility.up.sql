-- 0004: Student bill visibility (Menu 82) and Student exit tracking (Menu 78)

CREATE TABLE IF NOT EXISTS student_bill_visibility (
    school_id UUID NOT NULL,
    grade_level TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (school_id, grade_level)
);

CREATE TABLE IF NOT EXISTS student_billing_exits (
    school_id UUID NOT NULL,
    student_user_id UUID NOT NULL,
    effective_date DATE NOT NULL,
    policy TEXT NOT NULL DEFAULT 'void' CHECK (policy IN ('void', 'keep')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (school_id, student_user_id)
);

-- Allow student_bill_id to be nullable in bill_refunds for student-level deposit/credit balance refunds
ALTER TABLE bill_refunds ALTER COLUMN student_bill_id DROP NOT NULL;

-- Idempotency key for bill_refunds (e.g. withdrawal:<request_id>)
ALTER TABLE bill_refunds ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS bill_refunds_school_idempotency_idx
    ON bill_refunds (school_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
