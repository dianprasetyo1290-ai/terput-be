-- 0010_payroll gained payroll_input_changes after it had already run on
-- deployed databases, so those never got the table. Idempotent backfill.
CREATE TABLE IF NOT EXISTS payroll_input_changes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    input_id UUID NULL REFERENCES payroll_inputs(id) ON DELETE CASCADE,
    period CHAR(7) NOT NULL,
    employee_id UUID NOT NULL,
    component_id UUID NOT NULL REFERENCES pay_components(id),
    before_amount_idr BIGINT NOT NULL DEFAULT 0,
    after_amount_idr BIGINT NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending_approval' CHECK(status IN ('pending_approval', 'approved', 'rejected')),
    approval_id TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
