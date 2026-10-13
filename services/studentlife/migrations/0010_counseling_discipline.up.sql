-- 0010_counseling_discipline.up.sql
-- T20: Konseling & Kesiswaan (BK, Rujukan, Tugas Kasus, Kedisiplinan & Poin)
-- Memenuhi Menu PRD 130, 131, 132 & NFR-OPS-03

-- 1. Counseling Categories & Settings
CREATE TABLE counseling_categories (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  UUID NOT NULL,
    name       TEXT NOT NULL,
    active     BOOLEAN NOT NULL DEFAULT true,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_counseling_categories_name
    ON counseling_categories(school_id, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_counseling_categories_school
    ON counseling_categories(school_id);

CREATE TABLE counseling_settings (
    school_id            UUID PRIMARY KEY,
    reminder_days_before INT NOT NULL DEFAULT 0 CHECK (reminder_days_before >= 0),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2. Counseling Cases & Members (BR-CNS-01, AC-130-01)
CREATE TABLE counseling_cases (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id            UUID NOT NULL,
    student_id           UUID NOT NULL,
    category_id          UUID NOT NULL REFERENCES counseling_categories(id),
    opened_by            UUID NOT NULL,
    status               TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'monitoring', 'closed')),
    disclose_to_homeroom BOOLEAN NOT NULL DEFAULT false, -- AC-130-01: hanya jika diizinkan konselor
    opened_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at            TIMESTAMPTZ NULL,
    close_reason_enc     BYTEA NULL, -- NFR-OPS-03
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_counseling_cases_student ON counseling_cases(school_id, student_id, status);

CREATE TABLE counseling_case_members (
    case_id     UUID NOT NULL REFERENCES counseling_cases(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL,
    member_role TEXT NOT NULL DEFAULT 'counselor' CHECK (member_role IN ('counselor', 'kesiswaan')),
    added_by    UUID NOT NULL,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    removed_at  TIMESTAMPTZ NULL,
    PRIMARY KEY (case_id, user_id)
);
CREATE INDEX idx_counseling_case_members_user ON counseling_case_members(user_id) WHERE removed_at IS NULL;

-- 3. Counseling Sessions & Group Students
CREATE TABLE counseling_sessions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          UUID NOT NULL,
    case_id            UUID NULL REFERENCES counseling_cases(id) ON DELETE SET NULL,
    kind               TEXT NOT NULL DEFAULT 'individual' CHECK (kind IN ('individual', 'group')),
    category_id        UUID NOT NULL REFERENCES counseling_categories(id),
    counselor_user_id  UUID NOT NULL,
    scheduled_at       TIMESTAMPTZ NOT NULL,
    held_at            TIMESTAMPTZ NULL,
    status             TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'held', 'cancelled', 'no_show')),
    notes_enc          BYTEA NULL, -- NFR-OPS-03
    follow_up_plan_enc BYTEA NULL, -- NFR-OPS-03
    reminder_sent_at   TIMESTAMPTZ NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_counseling_sessions_school ON counseling_sessions(school_id, scheduled_at);
CREATE INDEX idx_counseling_sessions_counselor ON counseling_sessions(school_id, counselor_user_id, status);

CREATE TABLE counseling_session_students (
    session_id UUID NOT NULL REFERENCES counseling_sessions(id) ON DELETE CASCADE,
    student_id UUID NOT NULL,
    PRIMARY KEY (session_id, student_id)
);
CREATE INDEX idx_counseling_session_students_std ON counseling_session_students(student_id);

-- 4. Referrals & Case Tasks (FR-132-001, AC-132-01)
CREATE TABLE referrals (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          UUID NOT NULL,
    case_id            UUID NOT NULL REFERENCES counseling_cases(id) ON DELETE CASCADE,
    student_id         UUID NOT NULL,
    target_kind        TEXT NOT NULL CHECK (target_kind IN ('internal', 'external')),
    target_name_enc    BYTEA NOT NULL,
    target_contact_enc BYTEA NULL,
    reason_enc         BYTEA NOT NULL,
    parent_summary_enc BYTEA NOT NULL,
    status             TEXT NOT NULL DEFAULT 'awaiting_consent' CHECK (status IN ('awaiting_consent', 'active', 'completed', 'declined', 'cancelled')),
    consent_id         UUID NULL REFERENCES parent_consents(id) ON DELETE SET NULL,
    outcome_enc        BYTEA NULL,
    created_by         UUID NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_referrals_case ON referrals(case_id);
CREATE INDEX idx_referrals_student ON referrals(school_id, student_id, status);

CREATE TABLE case_tasks (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        UUID NOT NULL,
    case_id          UUID NOT NULL REFERENCES counseling_cases(id) ON DELETE CASCADE,
    title            TEXT NOT NULL,
    assignee_user_id UUID NOT NULL,
    due_date         DATE NOT NULL,
    done_at          TIMESTAMPTZ NULL,
    done_by          UUID NULL,
    note_enc         BYTEA NULL,
    created_by       UUID NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_case_tasks_case ON case_tasks(case_id);
CREATE INDEX idx_case_tasks_assignee ON case_tasks(school_id, assignee_user_id, due_date) WHERE done_at IS NULL;

-- 5. Discipline Configuration (FR-131-001)
CREATE TABLE discipline_types (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  UUID NOT NULL,
    code       TEXT NOT NULL,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('violation', 'merit')),
    points     INT NOT NULL CHECK (points > 0),
    active     BOOLEAN NOT NULL DEFAULT true,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_discipline_types_code
    ON discipline_types(school_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_discipline_types_school ON discipline_types(school_id);

CREATE TABLE discipline_thresholds (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id      UUID NOT NULL,
    points         INT NOT NULL CHECK (points > 0),
    action_name    TEXT NOT NULL,
    notify_parent  BOOLEAN NOT NULL DEFAULT true,
    effective_from DATE NOT NULL,
    effective_to   DATE NULL,
    deleted_at     TIMESTAMPTZ NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_discipline_thresholds_school ON discipline_thresholds(school_id, points) WHERE deleted_at IS NULL;

CREATE TABLE discipline_settings (
    school_id          UUID PRIMARY KEY,
    accumulation_scope TEXT NOT NULL DEFAULT 'academic_year' CHECK (accumulation_scope IN ('academic_year', 'all_time')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 6. Discipline Records & Actions & Appeals (AC-131-01, BR-DIS-01, FR-131-004)
CREATE TABLE discipline_records (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    student_id        UUID NOT NULL,
    academic_year_id  UUID NULL,
    type_id           UUID NOT NULL REFERENCES discipline_types(id),
    kind              TEXT NOT NULL CHECK (kind IN ('violation', 'merit')),
    points            INT NOT NULL CHECK (points > 0),
    occurred_on       DATE NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    evidence          JSONB NOT NULL DEFAULT '[]',
    reporter_user_id  UUID NOT NULL,
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'appealed', 'annulled')),
    annulled_reason   TEXT NULL,
    idempotency_key   TEXT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_discipline_records_idemp
    ON discipline_records(school_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_discipline_records_student
    ON discipline_records(school_id, student_id, occurred_on DESC);

CREATE TABLE discipline_actions (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              UUID NOT NULL,
    student_id             UUID NOT NULL,
    academic_year_id       UUID NULL,
    threshold_id           UUID NOT NULL REFERENCES discipline_thresholds(id),
    triggered_by_record_id UUID NOT NULL REFERENCES discipline_records(id),
    points_at_trigger      INT NOT NULL,
    status                 TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done', 'cancelled')),
    assignee_user_id       UUID NULL,
    notes                  TEXT NULL,
    done_at                TIMESTAMPTZ NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- BR-DIS-01: sekali per ambang per periode (status <> 'cancelled')
CREATE UNIQUE INDEX uq_discipline_actions_once
    ON discipline_actions(student_id, threshold_id, COALESCE(academic_year_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE status <> 'cancelled';
CREATE INDEX idx_discipline_actions_school ON discipline_actions(school_id, status);

CREATE TABLE discipline_appeals (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        UUID NOT NULL,
    record_id        UUID NOT NULL REFERENCES discipline_records(id) ON DELETE CASCADE,
    guardian_user_id UUID NOT NULL,
    reason           TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    approval_id      TEXT NULL,
    idempotency_key  TEXT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_discipline_appeals_record_pending
    ON discipline_appeals(record_id) WHERE status = 'pending';
CREATE UNIQUE INDEX uq_discipline_appeals_idemp
    ON discipline_appeals(school_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_discipline_appeals_guardian
    ON discipline_appeals(school_id, guardian_user_id, status);
