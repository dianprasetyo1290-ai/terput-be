CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    school_id UUID NOT NULL,
    module TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    action TEXT NOT NULL,
    actor_user_id UUID NULL,
    actor_role TEXT NULL,
    before JSONB NULL,
    after JSONB NULL,
    reason TEXT NULL,
    correlation_id TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    forwarded_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_quality_audit_events_pending ON audit_events(id) WHERE forwarded_at IS NULL;
CREATE INDEX idx_quality_audit_events_entity ON audit_events(school_id, entity_type, entity_id, created_at DESC);
