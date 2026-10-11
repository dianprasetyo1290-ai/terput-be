-- Migration 0001_core.up.sql: Core Service Initial Schema

-- Konfigurasi alur approval (FR-79-001/002)
CREATE TABLE approval_flows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    request_type TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_approval_flows_tenant_type ON approval_flows (school_id, request_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_approval_flows_tenant ON approval_flows (school_id);

CREATE TABLE approval_flow_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    flow_id UUID NOT NULL REFERENCES approval_flows(id) ON DELETE CASCADE,
    seq INT NOT NULL CHECK (seq > 0),
    approver_kind TEXT NOT NULL CHECK (approver_kind IN ('role', 'direct_supervisor', 'user')),
    approver_role TEXT NULL,
    approver_user_id UUID NULL,
    min_amount_idr BIGINT NULL,
    condition_flag TEXT NULL,
    sla_hours INT NULL CHECK (sla_hours > 0),
    escalate_to_role TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (flow_id, seq),
    CHECK ((approver_kind = 'role') = (approver_role IS NOT NULL)),
    CHECK ((approver_kind = 'user') = (approver_user_id IS NOT NULL))
);
CREATE INDEX idx_approval_flow_steps_flow ON approval_flow_steps (flow_id, seq);

-- Permintaan approval (instance)
CREATE TABLE approval_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    request_type TEXT NOT NULL,
    source_service TEXT NOT NULL,
    source_id TEXT NOT NULL,
    title TEXT NOT NULL,
    summary JSONB NOT NULL,
    amount_idr BIGINT NULL,
    flags TEXT[] NOT NULL DEFAULT '{}',
    maker_user_id UUID NOT NULL,
    subject_user_id UUID NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    current_seq INT NULL,
    callback_url TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    flow_snapshot JSONB NOT NULL,
    decided_at TIMESTAMPTZ NULL,
    callback_status TEXT NOT NULL DEFAULT 'pending' CHECK (callback_status IN ('pending', 'done', 'failed')),
    callback_attempts INT NOT NULL DEFAULT 0,
    last_callback_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX idx_approval_requests_tenant_status ON approval_requests (school_id, status, created_at DESC);
CREATE INDEX idx_approval_requests_source ON approval_requests (school_id, source_service, source_id);
CREATE INDEX idx_approval_requests_maker ON approval_requests (maker_user_id, created_at DESC);
CREATE INDEX idx_approval_requests_callback ON approval_requests (callback_status) WHERE callback_status != 'done';

CREATE TABLE approval_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    request_id UUID NOT NULL REFERENCES approval_requests(id) ON DELETE CASCADE,
    seq INT NOT NULL CHECK (seq > 0),
    approver_kind TEXT NOT NULL CHECK (approver_kind IN ('role', 'direct_supervisor', 'user')),
    approver_role TEXT NULL,
    approver_user_id UUID NULL,
    status TEXT NOT NULL CHECK (status IN ('waiting', 'pending', 'approved', 'rejected', 'skipped', 'escalated')),
    -- sla_hours / escalate_to_role disalin dari flow saat request dibuat, supaya
    -- eskalasi dan due_at tahap berikutnya tidak bergantung pada pemetaan seq
    -- flow_snapshot (seq bergeser saat tahap pengganti eskalasi disisipkan).
    sla_hours INT NULL CHECK (sla_hours > 0),
    escalate_to_role TEXT NULL,
    due_at TIMESTAMPTZ NULL,
    decided_by UUID NULL,
    decided_as_delegate_of UUID NULL,
    comment TEXT NULL,
    decided_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- DEFERRABLE: eskalasi menggeser seq tahap berikutnya +1 dalam satu transaksi.
    CONSTRAINT approval_steps_request_seq_key UNIQUE (request_id, seq) DEFERRABLE INITIALLY IMMEDIATE
);
CREATE INDEX idx_approval_steps_request ON approval_steps (request_id, seq);
CREATE INDEX idx_approval_steps_pending_role ON approval_steps (school_id, status, approver_role) WHERE status = 'pending';
CREATE INDEX idx_approval_steps_pending_user ON approval_steps (school_id, status, approver_user_id) WHERE status = 'pending';
CREATE INDEX idx_approval_steps_due ON approval_steps (status, due_at) WHERE status = 'pending' AND due_at IS NOT NULL;

-- Delegasi approval (FR-79-005)
CREATE TABLE approval_delegations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    from_user_id UUID NOT NULL,
    to_user_id UUID NOT NULL,
    valid_from TIMESTAMPTZ NOT NULL,
    valid_to TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ NULL,
    CHECK (valid_to > valid_from),
    CHECK (from_user_id <> to_user_id)
);
CREATE INDEX idx_approval_delegations_lookup ON approval_delegations (school_id, to_user_id, valid_from, valid_to) WHERE revoked_at IS NULL;
CREATE INDEX idx_approval_delegations_from ON approval_delegations (school_id, from_user_id, valid_from, valid_to) WHERE revoked_at IS NULL;

-- Exception queue (FR-80-*)
CREATE TABLE integration_exceptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    source_service TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('webhook', 'journal_posting', 'notification', 'dead_letter', 'import', 'callback')),
    severity TEXT NOT NULL CHECK (severity IN ('normal', 'critical')),
    correlation_id TEXT NOT NULL,
    error_message TEXT NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL,
    payload_summary JSONB NOT NULL,
    retry_url TEXT NULL,
    status TEXT NOT NULL CHECK (status IN ('open', 'retrying', 'resolved')),
    assignee_user_id UUID NULL,
    resolved_by UUID NULL,
    resolution_reason TEXT NULL,
    resolved_at TIMESTAMPTZ NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX idx_integration_exceptions_tenant ON integration_exceptions (school_id, status, kind, created_at DESC);

-- Audit pusat (FR-06-*)
CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    school_id UUID NULL,
    service TEXT NOT NULL,
    module TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    action TEXT NOT NULL,
    actor_user_id UUID NULL,
    actor_role TEXT NULL,
    before_data JSONB NULL,
    after_data JSONB NULL,
    reason TEXT NULL,
    correlation_id TEXT NULL,
    ip_hash TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_events_tenant_created ON audit_events (school_id, created_at DESC);
CREATE INDEX idx_audit_events_tenant_module ON audit_events (school_id, module, created_at DESC);
CREATE INDEX idx_audit_events_actor ON audit_events (actor_user_id, created_at DESC);
