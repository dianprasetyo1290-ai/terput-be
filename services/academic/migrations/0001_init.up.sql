-- academic-service schema
-- Owns everything "under" an academic year: terms/semesters, classrooms,
-- subjects, classroom<->subject<->guru mapping (+ subject matrix), schedules,
-- and enrollments. identity-service owns schools/users/academic_years
-- (bare id/school_id/label/is_current) — referenced here only by UUID, no
-- cross-DB foreign keys.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ==========================================================================
-- Terms / Semesters
-- ==========================================================================

-- Two rows (Semester 1, Semester 2) are expected per academic_year_id; see
-- POST /academic-years/{academic_year_id}/terms/bootstrap in
-- internal/handler/term.go and README "Tahun Akademik / Term" section for
-- how these get created.
CREATE TABLE terms (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    name              TEXT NOT NULL,        -- e.g. "Semester 1"
    "order"           SMALLINT NOT NULL,    -- 1 or 2
    start_date        DATE,
    end_date          DATE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (academic_year_id, "order")
);

CREATE INDEX idx_terms_school_year ON terms(school_id, academic_year_id);

-- ==========================================================================
-- Classrooms (Data Kelas)
-- ==========================================================================

-- Flattened single-level classroom (e.g. "X IPA 1") rather than the old
-- Laravel schema's two-level classrooms/sub_classrooms split — see README
-- Assumptions for why.
CREATE TABLE classrooms (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id                 UUID NOT NULL,
    academic_year_id          UUID NOT NULL,
    name                      TEXT NOT NULL,   -- e.g. "X IPA 1"
    grade_level               TEXT,            -- e.g. "10", "12" — used to detect graduating classes (9/12) at promotion time
    homeroom_teacher_user_id  UUID,            -- wali kelas; identity-service user id, no FK (cross-DB)
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at                TIMESTAMPTZ
);

CREATE INDEX idx_classrooms_school_year ON classrooms(school_id, academic_year_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_classrooms_homeroom ON classrooms(homeroom_teacher_user_id) WHERE deleted_at IS NULL;

-- Roster: which siswa is enrolled in which classroom, for which academic
-- year. This is the table the "kelas 12/9 lulus tidak boleh nyampur ke
-- tahun berikutnya" rule and the kenaikan-kelas bulk-promotion endpoint
-- operate on. A student has at most one active enrollment row per
-- academic_year_id.
CREATE TABLE class_enrollments (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    classroom_id      UUID NOT NULL REFERENCES classrooms(id) ON DELETE CASCADE,
    student_user_id   UUID NOT NULL,  -- identity-service user id, no FK (cross-DB)
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','graduated','transferred_out')),
    enrolled_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (academic_year_id, student_user_id)
);

CREATE INDEX idx_class_enrollments_classroom ON class_enrollments(classroom_id);
CREATE INDEX idx_class_enrollments_school_year ON class_enrollments(school_id, academic_year_id);

-- ==========================================================================
-- Subjects (Data Mata Pelajaran)
-- ==========================================================================

-- Subject catalog is school-scoped but NOT academic_year-scoped (a subject
-- name like "Matematika" persists across years unchanged in the reference
-- Laravel schema too — subjects table there has no academic_year_id). The
-- year-scoping happens one level down, at classroom_subjects. See README
-- Assumptions.
CREATE TABLE subjects (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id      UUID NOT NULL,
    name           TEXT NOT NULL,
    description    TEXT,
    subject_group  TEXT,  -- e.g. "Agama" — groups elective variants (Agama Islam/Katolik/Buddha/Konghucu) that are mutually-exclusive alternatives of each other for subject-matrix purposes
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ
);

CREATE INDEX idx_subjects_school ON subjects(school_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_subjects_group ON subjects(subject_group) WHERE deleted_at IS NULL;

-- classroom_subjects is the (subject x classroom x guru) mapping from "Data
-- Kelas", editable per-pairing (one guru can teach multiple subjects;
-- subjects differ per class/jurusan). is_active doubles as the "Matriks
-- Pelajaran" enable/disable toggle for that subject in that classroom (e.g.
-- turning off "Agama Katolik" for a classroom that has no Catholic
-- students, while leaving "Agama Islam" on).
CREATE TABLE classroom_subjects (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    classroom_id      UUID NOT NULL REFERENCES classrooms(id) ON DELETE CASCADE,
    subject_id        UUID NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    teacher_user_id   UUID,  -- guru pengampu; identity-service user id, no FK (cross-DB)
    is_active         BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ,
    UNIQUE (classroom_id, subject_id)
);

CREATE INDEX idx_classroom_subjects_classroom ON classroom_subjects(classroom_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_classroom_subjects_teacher ON classroom_subjects(teacher_user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_classroom_subjects_school_year ON classroom_subjects(school_id, academic_year_id);

-- Resolves, per student, which specific classroom_subjects row (i.e. which
-- subject-group variant) they are actually enrolled in — this is what
-- GET /students/{id}/active-subjects reads. Needed because classroom_subjects
-- may list several mutually-exclusive variants of the same subject_group
-- (Agama Islam / Agama Katolik / ...) all "active" for a classroom at once;
-- this table picks the one each individual student takes.
CREATE TABLE student_subject_enrollments (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              UUID NOT NULL,
    academic_year_id       UUID NOT NULL,
    student_user_id        UUID NOT NULL,  -- identity-service user id, no FK (cross-DB)
    classroom_subject_id   UUID NOT NULL REFERENCES classroom_subjects(id) ON DELETE CASCADE,
    is_enrolled            BOOLEAN NOT NULL DEFAULT true,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (student_user_id, classroom_subject_id)
);

CREATE INDEX idx_sse_student ON student_subject_enrollments(student_user_id);
CREATE INDEX idx_sse_classroom_subject ON student_subject_enrollments(classroom_subject_id);

-- ==========================================================================
-- Schedules (Jadwal Pelajaran)
-- ==========================================================================

-- "jam pelajaran" — flexible per classroom (different classes have
-- different period start/end times, e.g. shorter Friday periods).
CREATE TABLE schedule_periods (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    classroom_id      UUID NOT NULL REFERENCES classrooms(id) ON DELETE CASCADE,
    day_of_week       SMALLINT NOT NULL CHECK (day_of_week BETWEEN 1 AND 7), -- 1=Monday .. 7=Sunday
    period_no         SMALLINT NOT NULL,
    start_time        TEXT NOT NULL,  -- "HH:MM", stored as text (not TIME) to keep pgx/database-sql scanning simple; validated at the handler layer
    end_time          TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (classroom_id, day_of_week, period_no)
);

CREATE INDEX idx_schedule_periods_classroom_day ON schedule_periods(classroom_id, day_of_week);

-- "jadwal" slot — assigns a classroom_subject (subject+guru pairing,
-- already scoped to this classroom) into a schedule_period (day+time slot).
-- The handler layer enforces that the subject dropdown only offers subjects
-- already present in classroom_subjects for this classroom (see
-- internal/handler/schedule.go).
CREATE TABLE schedules (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              UUID NOT NULL,
    academic_year_id       UUID NOT NULL,
    classroom_id           UUID NOT NULL REFERENCES classrooms(id) ON DELETE CASCADE,
    schedule_period_id     UUID NOT NULL REFERENCES schedule_periods(id) ON DELETE CASCADE,
    classroom_subject_id   UUID NOT NULL REFERENCES classroom_subjects(id) ON DELETE CASCADE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (schedule_period_id)  -- one subject per period slot
);

CREATE INDEX idx_schedules_classroom ON schedules(classroom_id);
CREATE INDEX idx_schedules_classroom_subject ON schedules(classroom_subject_id);
