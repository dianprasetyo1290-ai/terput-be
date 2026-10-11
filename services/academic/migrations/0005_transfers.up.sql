-- 0005: Student status requests (transfer and withdrawal, PRD Menu 78)
-- Supports class transfer, unit transfer, and withdrawal with financial snapshots
-- and academic exit policy configurations.

CREATE TABLE IF NOT EXISTS student_status_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    student_user_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('class_transfer', 'unit_transfer', 'withdrawal')),
    from_classroom_id UUID REFERENCES classrooms(id) ON DELETE SET NULL,
    to_classroom_id UUID REFERENCES classrooms(id) ON DELETE SET NULL,
    to_school_id UUID,
    effective_date DATE NOT NULL,
    reason TEXT NOT NULL CHECK (btrim(reason) <> ''),
    attachments JSONB NOT NULL DEFAULT '[]',
    finance_snapshot JSONB,
    refund_amount_idr BIGINT CHECK (refund_amount_idr IS NULL OR refund_amount_idr >= 0),
    status TEXT NOT NULL DEFAULT 'pending_approval' CHECK (status IN ('pending_approval', 'approved', 'rejected', 'cancelled', 'applied')),
    approval_id UUID,
    applied_at TIMESTAMPTZ,
    requested_by UUID NOT NULL,
    idempotency_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT student_status_requests_school_idempotency_key UNIQUE (school_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_student_status_requests_school_status ON student_status_requests (school_id, status);
CREATE INDEX IF NOT EXISTS idx_student_status_requests_student ON student_status_requests (school_id, student_user_id);
CREATE INDEX IF NOT EXISTS idx_student_status_requests_effective ON student_status_requests (school_id, effective_date);

CREATE TABLE IF NOT EXISTS academic_settings (
    school_id UUID PRIMARY KEY,
    post_exit_bill_policy TEXT NOT NULL DEFAULT 'void' CHECK (post_exit_bill_policy IN ('void', 'keep')),
    parent_grace_days INT NOT NULL DEFAULT 30 CHECK (parent_grace_days >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Expand class_enrollments status CHECK to allow 'withdrawn' alongside 'active', 'graduated', 'transferred_out'
ALTER TABLE class_enrollments DROP CONSTRAINT IF EXISTS class_enrollments_status_check;
ALTER TABLE class_enrollments ADD CONSTRAINT class_enrollments_status_check
    CHECK (status IN ('active', 'graduated', 'transferred_out', 'withdrawn'));
