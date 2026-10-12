-- 0003_health_programs.up.sql
-- Menu 129: program imunisasi & skrining, sasaran, peserta + persetujuan.

CREATE TABLE health_programs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        UUID NOT NULL,
    kind             TEXT NOT NULL CHECK (kind IN ('immunization', 'screening')),
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    starts_on        DATE NOT NULL,
    ends_on          DATE NOT NULL,
    requires_consent BOOLEAN NOT NULL,
    consent_deadline DATE NULL,
    status           TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'completed', 'cancelled')),
    created_by       UUID NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_on >= starts_on)
);
CREATE INDEX idx_health_programs_school ON health_programs(school_id, starts_on DESC);

CREATE TABLE health_program_targets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL,
    program_id  UUID NOT NULL REFERENCES health_programs(id) ON DELETE CASCADE,
    target_kind TEXT NOT NULL CHECK (target_kind IN ('classroom', 'grade_level', 'student')),
    target_ref  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (program_id, target_kind, target_ref)
);

CREATE TABLE health_program_participants (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    program_id        UUID NOT NULL REFERENCES health_programs(id),
    student_id        UUID NOT NULL,
    consent_id        UUID NULL REFERENCES parent_consents(id),
    result_status     TEXT NOT NULL DEFAULT 'not_recorded' CHECK (result_status IN ('not_recorded', 'done', 'absent', 'excluded')),
    result_enc        BYTEA NULL,
    needs_follow_up   BOOLEAN NOT NULL DEFAULT false,
    follow_up_enc     BYTEA NULL,
    follow_up_done_at TIMESTAMPTZ NULL,
    recorded_by       UUID NULL,
    recorded_at       TIMESTAMPTZ NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (program_id, student_id)
);
CREATE INDEX idx_health_program_participants_student ON health_program_participants(school_id, student_id);
