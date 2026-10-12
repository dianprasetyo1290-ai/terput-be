-- 0001_studentlife_base.up.sql
-- Kerangka service studentlife (T10), dipakai ulang T20/T21/T22.
-- NFR-OPS-03: log akses data sensitif + kebijakan akses rahasia per peran.
-- OQ-31: tanpa baris di confidential_access_policies = kepala_sekolah TIDAK
-- punya akses ke data kesehatan/konseling.

CREATE TABLE studentlife_settings (
    school_id  UUID PRIMARY KEY,
    timezone   TEXT NOT NULL DEFAULT 'Asia/Jakarta',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_events (
    id             BIGSERIAL PRIMARY KEY,
    school_id      UUID NOT NULL,
    module         TEXT NOT NULL,
    entity_type    TEXT NOT NULL,
    entity_id      TEXT NOT NULL,
    action         TEXT NOT NULL,
    actor_user_id  UUID NULL,
    actor_role     TEXT NULL,
    before         JSONB NULL,
    after          JSONB NULL,
    reason         TEXT NULL,
    correlation_id TEXT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    forwarded_at   TIMESTAMPTZ NULL
);
CREATE INDEX idx_audit_events_pending ON audit_events(id) WHERE forwarded_at IS NULL;
CREATE INDEX idx_audit_events_entity ON audit_events(school_id, entity_type, entity_id, created_at DESC);

CREATE TABLE sensitive_access_logs (
    id            BIGSERIAL PRIMARY KEY,
    school_id     UUID NOT NULL,
    module        TEXT NOT NULL CHECK (module IN ('health', 'counseling')),
    entity_type   TEXT NOT NULL,
    entity_id     UUID NULL,
    student_id    UUID NULL,
    actor_user_id UUID NOT NULL,
    actor_role    TEXT NOT NULL,
    action        TEXT NOT NULL CHECK (action IN ('view', 'create', 'update', 'print', 'deny')),
    reason        TEXT NULL,
    ip_hash       TEXT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sensitive_access_logs_school ON sensitive_access_logs(school_id, created_at DESC);
CREATE INDEX idx_sensitive_access_logs_student ON sensitive_access_logs(school_id, student_id, created_at DESC);

CREATE TABLE confidential_access_policies (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  UUID NOT NULL,
    module     TEXT NOT NULL CHECK (module IN ('health', 'counseling')),
    role       TEXT NOT NULL,
    granted_by UUID NOT NULL,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_by UUID NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_confidential_access_policies_active
    ON confidential_access_policies(school_id, module, role) WHERE revoked_at IS NULL;

CREATE TABLE parent_consents (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              UUID NOT NULL,
    subject_type           TEXT NOT NULL,
    subject_id             UUID NOT NULL,
    student_id             UUID NOT NULL,
    status                 TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'granted', 'declined', 'revoked')),
    decided_by_guardian_id UUID NULL,
    decided_at             TIMESTAMPTZ NULL,
    note                   TEXT NULL,
    requested_by           UUID NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (subject_type, subject_id, student_id)
);
CREATE INDEX idx_parent_consents_student ON parent_consents(school_id, student_id, status);
