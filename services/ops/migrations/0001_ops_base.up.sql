-- Kerangka service ops (T9): outbox audit, nomor dokumen, ekstensi untuk
-- exclusion constraint booking ruang.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS btree_gist;

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
CREATE INDEX idx_audit_events_pending ON audit_events(id) WHERE forwarded_at IS NULL;
CREATE INDEX idx_audit_events_entity ON audit_events(school_id, entity_type, entity_id, created_at DESC);

-- Nomor dokumen ops (WO-2026-0001), dipakai ulang T15–T19.
CREATE TABLE doc_sequences (
    school_id UUID NOT NULL,
    prefix TEXT NOT NULL,
    year INT NOT NULL,
    last INT NOT NULL DEFAULT 0,
    PRIMARY KEY (school_id, prefix, year)
);
