-- Migration 0030_training.up.sql: Training & Onboarding Schema

CREATE TABLE IF NOT EXISTS training_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_training_categories_school ON training_categories (school_id);

CREATE TABLE IF NOT EXISTS competencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, code)
);
CREATE INDEX IF NOT EXISTS idx_competencies_school ON competencies (school_id);

CREATE TABLE IF NOT EXISTS courses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    title TEXT NOT NULL,
    objective TEXT NOT NULL,
    duration_minutes INT NOT NULL DEFAULT 60,
    provider TEXT NOT NULL DEFAULT 'Internal',
    kind TEXT NOT NULL CHECK (kind IN ('online', 'offline')),
    category_id UUID NULL REFERENCES training_categories(id) ON DELETE SET NULL,
    cost_idr BIGINT NOT NULL DEFAULT 0,
    quota INT NULL,
    passing_score INT NULL CHECK (passing_score BETWEEN 0 AND 100),
    status TEXT NOT NULL CHECK (status IN ('draft', 'published', 'archived')),
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_courses_school_status ON courses (school_id, status);

CREATE TABLE IF NOT EXISTS course_competencies (
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    competency_id UUID NOT NULL REFERENCES competencies(id) ON DELETE CASCADE,
    PRIMARY KEY (course_id, competency_id)
);

CREATE TABLE IF NOT EXISTS course_materials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    sort INT NOT NULL DEFAULT 0,
    kind TEXT NOT NULL CHECK (kind IN ('file', 'link', 'video')),
    title TEXT NOT NULL,
    object_key TEXT NULL,
    url TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_course_materials_course ON course_materials (course_id, sort);

CREATE TABLE IF NOT EXISTS course_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    location TEXT NOT NULL,
    quota INT NOT NULL DEFAULT 30,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_course_sessions_course ON course_sessions (course_id);

CREATE TABLE IF NOT EXISTS quiz_questions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    sort INT NOT NULL DEFAULT 0,
    prompt TEXT NOT NULL,
    options JSONB NOT NULL DEFAULT '[]',
    correct_index INT NOT NULL CHECK (correct_index >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_quiz_questions_course ON quiz_questions (course_id, sort);

CREATE TABLE IF NOT EXISTS mandatory_trainings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    position_id UUID NOT NULL,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    due_days INT NOT NULL DEFAULT 30,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (position_id, course_id)
);
CREATE INDEX IF NOT EXISTS idx_mandatory_trainings_pos ON mandatory_trainings (position_id);

CREATE TABLE IF NOT EXISTS training_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    session_id UUID NULL REFERENCES course_sessions(id) ON DELETE SET NULL,
    source TEXT NOT NULL CHECK (source IN ('manual', 'position', 'unit', 'onboarding', 'self')),
    due_date DATE NULL,
    status TEXT NOT NULL CHECK (status IN ('pending_approval', 'not_started', 'in_progress', 'completed', 'rejected', 'cancelled')),
    approval_id TEXT NULL,
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    score INT NULL,
    certificate_doc_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_training_assignments_emp_course ON training_assignments (employee_id, course_id) WHERE status NOT IN ('cancelled', 'rejected');
CREATE INDEX IF NOT EXISTS idx_training_assignments_school_emp ON training_assignments (school_id, employee_id);

CREATE TABLE IF NOT EXISTS material_progress (
    assignment_id UUID NOT NULL REFERENCES training_assignments(id) ON DELETE CASCADE,
    material_id UUID NOT NULL REFERENCES course_materials(id) ON DELETE CASCADE,
    done_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (assignment_id, material_id)
);

CREATE TABLE IF NOT EXISTS quiz_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    assignment_id UUID NOT NULL REFERENCES training_assignments(id) ON DELETE CASCADE,
    answers JSONB NOT NULL DEFAULT '[]',
    score INT NOT NULL,
    passed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_quiz_attempts_assignment ON quiz_attempts (assignment_id);

-- Onboarding
CREATE TABLE IF NOT EXISTS onboarding_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    position_id UUID NULL,
    department_id UUID NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_onboarding_templates_school ON onboarding_templates (school_id);

CREATE TABLE IF NOT EXISTS onboarding_template_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    template_id UUID NOT NULL REFERENCES onboarding_templates(id) ON DELETE CASCADE,
    sort INT NOT NULL DEFAULT 0,
    title TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('dokumen', 'akun', 'perangkat', 'orientasi', 'pelatihan')),
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('role', 'supervisor', 'employee')),
    owner_role TEXT NULL,
    due_offset_days INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_onboarding_template_tasks_tpl ON onboarding_template_tasks (template_id, sort);

CREATE TABLE IF NOT EXISTS onboarding_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL UNIQUE,
    template_id UUID NOT NULL REFERENCES onboarding_templates(id),
    start_date DATE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'completed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_onboarding_plans_school ON onboarding_plans (school_id);

CREATE TABLE IF NOT EXISTS onboarding_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    plan_id UUID NOT NULL REFERENCES onboarding_plans(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('dokumen', 'akun', 'perangkat', 'orientasi', 'pelatihan')),
    owner_user_id UUID NULL,
    owner_role TEXT NULL,
    due_date DATE NOT NULL,
    done_at TIMESTAMPTZ NULL,
    done_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_onboarding_tasks_plan ON onboarding_tasks (plan_id);
