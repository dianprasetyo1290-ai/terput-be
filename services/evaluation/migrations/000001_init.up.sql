CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- Feature 1: RPM (lesson plans)
-- ============================================================
CREATE TABLE lesson_plans (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    guru_user_id      UUID NOT NULL,
    title             TEXT NOT NULL DEFAULT '',
    -- A. Identitas & Karakteristik / B. Analisis Kesiapan & Desain / C. Langkah Pembelajaran
    section_a         JSONB NOT NULL DEFAULT '{}'::jsonb,
    section_b         JSONB NOT NULL DEFAULT '{}'::jsonb,
    section_c         JSONB NOT NULL DEFAULT '{}'::jsonb,
    status            TEXT NOT NULL DEFAULT 'draft',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ
);
CREATE INDEX idx_lesson_plans_school ON lesson_plans(school_id);
CREATE INDEX idx_lesson_plans_guru ON lesson_plans(guru_user_id, academic_year_id);

-- ============================================================
-- Feature 2: Penilaian Guru (teacher evaluation)
-- ============================================================
CREATE TABLE teacher_evaluation_periods (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id                 UUID NOT NULL,
    academic_year_id          UUID NOT NULL,
    title                     TEXT NOT NULL,
    semester                  TEXT,
    publish_start_date        DATE NOT NULL,
    publish_end_date          DATE NOT NULL,
    published                 BOOLEAN NOT NULL DEFAULT false,
    headmaster_user_id        UUID,
    curriculum_vp_user_id     UUID,
    student_affairs_vp_user_id UUID,
    -- J: guru in-scope for this period (evaluated by kepsek/waka/self/siswa)
    target_teacher_ids        JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- K(a)/K(b): peer-evaluation cross assignment
    peer_evaluatee_ids        JSONB NOT NULL DEFAULT '[]'::jsonb,
    peer_evaluator_ids        JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- classrooms whose students may evaluate the target guru
    student_classroom_ids     JSONB NOT NULL DEFAULT '[]'::jsonb,
    lead_assessor_name        TEXT,
    created_by                UUID,
    updated_by                UUID,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at                TIMESTAMPTZ
);
CREATE INDEX idx_tep_school ON teacher_evaluation_periods(school_id, academic_year_id);
CREATE INDEX idx_tep_dates ON teacher_evaluation_periods(publish_start_date, publish_end_date);

-- 6 evaluator categories (Form A..F)
CREATE TABLE teacher_evaluation_forms (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id      UUID NOT NULL,
    code           TEXT NOT NULL,           -- A, B, C, D, E, F
    title          TEXT NOT NULL,
    evaluator_type TEXT NOT NULL,           -- kepala_sekolah|waka_kurikulum|waka_kesiswaan|peer|siswa|self
    priority       SMALLINT NOT NULL DEFAULT 1,
    weight         NUMERIC(6,4) NOT NULL,   -- fraction of final 0-4 score, e.g. 0.2000 = 20%
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, evaluator_type)
);

CREATE TABLE teacher_evaluation_items (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id       UUID NOT NULL REFERENCES teacher_evaluation_forms(id) ON DELETE CASCADE,
    school_id     UUID NOT NULL,
    code          TEXT NOT NULL,
    indicator     TEXT NOT NULL,
    indicator_en  TEXT,
    weight        NUMERIC(7,5) NOT NULL DEFAULT 0,  -- fraction of final score contributed by this item
    max_score     SMALLINT NOT NULL DEFAULT 4,
    score_labels  JSONB,                             -- {"4":"...","3":"...","2":"...","1":"..."}
    sort_order    SMALLINT NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_tei_form ON teacher_evaluation_items(form_id, sort_order);

-- one task = one evaluator's assignment to rate one target guru under one form
CREATE TABLE teacher_evaluation_tasks (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    period_id              UUID NOT NULL REFERENCES teacher_evaluation_periods(id) ON DELETE CASCADE,
    form_id                UUID NOT NULL REFERENCES teacher_evaluation_forms(id) ON DELETE CASCADE,
    school_id              UUID NOT NULL,
    academic_year_id       UUID NOT NULL,
    target_teacher_user_id UUID NOT NULL,
    evaluator_user_id      UUID NOT NULL,
    evaluator_type         TEXT NOT NULL,
    classroom_id           UUID,   -- populated for evaluator_type = siswa
    status                 TEXT NOT NULL DEFAULT 'pending', -- pending|submitted
    submitted_at           TIMESTAMPTZ,
    raw_score              NUMERIC(4,2),  -- this evaluator's average item score (1-4)
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_tet_unique ON teacher_evaluation_tasks(
    period_id, form_id, target_teacher_user_id, evaluator_user_id,
    COALESCE(classroom_id, '00000000-0000-0000-0000-000000000000'::uuid)
);
CREATE INDEX idx_tet_period_target ON teacher_evaluation_tasks(period_id, target_teacher_user_id);
CREATE INDEX idx_tet_evaluator ON teacher_evaluation_tasks(evaluator_user_id, status);

CREATE TABLE teacher_evaluation_answers (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    UUID NOT NULL REFERENCES teacher_evaluation_tasks(id) ON DELETE CASCADE,
    item_id    UUID NOT NULL REFERENCES teacher_evaluation_items(id) ON DELETE CASCADE,
    school_id  UUID NOT NULL,
    score      SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, item_id)
);
