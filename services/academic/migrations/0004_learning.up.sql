-- PRD v2.1 menus 57/70 (Materi Pembelajaran) and 58/71 (Tugas &
-- Pengumpulan). Everything hangs off classroom_subjects (one guru x one
-- subject x one classroom), so "who may publish" and "who may see" both
-- resolve through the same row the rest of the academic module already uses.
-- Files live in the service's private STORAGE_ROOT; only metadata is here.

CREATE TABLE IF NOT EXISTS learning_materials (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id             UUID NOT NULL,
    academic_year_id      UUID NOT NULL,
    classroom_subject_id  UUID NOT NULL REFERENCES classroom_subjects(id) ON DELETE CASCADE,
    teacher_user_id       UUID NOT NULL,
    title                 TEXT NOT NULL,
    description           TEXT,
    link_url              TEXT,
    file_key              TEXT,
    file_name             TEXT,
    file_mime             TEXT,
    file_size             BIGINT,
    status                TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    published_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at            TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_learning_materials_cs ON learning_materials(classroom_subject_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_learning_materials_teacher ON learning_materials(teacher_user_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS learning_assignments (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id             UUID NOT NULL,
    academic_year_id      UUID NOT NULL,
    classroom_subject_id  UUID NOT NULL REFERENCES classroom_subjects(id) ON DELETE CASCADE,
    teacher_user_id       UUID NOT NULL,
    title                 TEXT NOT NULL,
    instructions          TEXT,
    link_url              TEXT,
    due_at                TIMESTAMPTZ NOT NULL,
    max_score             NUMERIC(6,2) NOT NULL DEFAULT 100 CHECK (max_score > 0),
    file_key              TEXT,
    file_name             TEXT,
    file_mime             TEXT,
    file_size             BIGINT,
    status                TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    published_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at            TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_learning_assignments_cs ON learning_assignments(classroom_subject_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_learning_assignments_teacher ON learning_assignments(teacher_user_id) WHERE deleted_at IS NULL;

-- One row per (tugas, siswa); a resubmission overwrites the answer and sets
-- status back to 'submitted' (resubmit_count keeps the history count).
CREATE TABLE IF NOT EXISTS learning_submissions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        UUID NOT NULL,
    assignment_id    UUID NOT NULL REFERENCES learning_assignments(id) ON DELETE CASCADE,
    student_user_id  UUID NOT NULL,
    answer_text      TEXT,
    file_key         TEXT,
    file_name        TEXT,
    file_mime        TEXT,
    file_size        BIGINT,
    submitted_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    is_late          BOOLEAN NOT NULL DEFAULT false,
    status           TEXT NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted', 'graded', 'resubmit_requested')),
    resubmit_count   INT NOT NULL DEFAULT 0,
    score            NUMERIC(6,2),
    feedback         TEXT,
    graded_at        TIMESTAMPTZ,
    graded_by        UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (assignment_id, student_user_id)
);
CREATE INDEX IF NOT EXISTS idx_learning_submissions_student ON learning_submissions(student_user_id);
