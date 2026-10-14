-- Migration 0004 (BUG-087, AC-RBAC-10): auditor access to the audit trail and
-- the integration exception queue is time-boxed per school. A superadmin
-- grants a window; outside an active, unrevoked window the auditor is denied.
CREATE TABLE audit_access_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    user_id UUID NOT NULL,
    valid_from TIMESTAMPTZ NOT NULL,
    valid_to TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL,
    granted_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ NULL,
    revoked_by UUID NULL,
    CONSTRAINT audit_access_grants_window CHECK (valid_to > valid_from),
    CONSTRAINT audit_access_grants_reason CHECK (length(btrim(reason)) > 0)
);
CREATE INDEX idx_audit_access_grants_active ON audit_access_grants (school_id, user_id, valid_to) WHERE revoked_at IS NULL;

-- BUG-117: trace one cross-module flow by its correlation id.
CREATE INDEX idx_audit_events_correlation ON audit_events (correlation_id) WHERE correlation_id IS NOT NULL;
