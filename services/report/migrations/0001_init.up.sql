CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- report_cards: one row per (student, classroom, term). This is the header +
-- status envelope; subject-level scores live in report_card_subjects.
CREATE TABLE report_cards (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id           UUID NOT NULL,
    academic_year_id    UUID NOT NULL,
    term_id             UUID NOT NULL,
    classroom_id        UUID NOT NULL,
    student_id          UUID NOT NULL,

    status              TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'finalized')),
    finalized_at        TIMESTAMPTZ,
    finalized_by        UUID,
    unlocked_at         TIMESTAMPTZ,
    unlocked_by         UUID,

    class_teacher_note  TEXT,

    -- attendance summary snapshot pulled from attendance-service per term
    attendance_hadir            INT,
    attendance_sakit            INT,
    attendance_izin             INT,
    attendance_tanpa_keterangan INT,
    attendance_synced_at        TIMESTAMPTZ,
    attendance_sync_error       TEXT,

    -- header snapshot pulled from identity/academic-service (cached for
    -- display resiliency if those services are briefly unreachable later)
    student_name       TEXT,
    student_nisn        TEXT,
    classroom_name       TEXT,
    term_name            TEXT,
    header_synced_at     TIMESTAMPTZ,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (student_id, classroom_id, term_id)
);

CREATE INDEX report_cards_school_idx ON report_cards (school_id);
CREATE INDEX report_cards_classroom_term_idx ON report_cards (classroom_id, term_id);
CREATE INDEX report_cards_student_idx ON report_cards (student_id);
CREATE INDEX report_cards_academic_year_idx ON report_cards (academic_year_id);

-- report_card_subjects: subject -> score rows within a report card.
-- computed_score = aggregated (average) from exam-service via ExamType
-- "Mapping Rapor". override_score, if set (not null), wins for display /
-- leger purposes, but computed_score is preserved so admin can see both.
CREATE TABLE report_card_subjects (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_card_id      UUID NOT NULL REFERENCES report_cards(id) ON DELETE CASCADE,
    school_id           UUID NOT NULL,

    subject_id          UUID NOT NULL,
    subject_name        TEXT,
    subject_code         TEXT,

    computed_score       NUMERIC(5,2),
    override_score        NUMERIC(5,2),
    source_exam_count       INT NOT NULL DEFAULT 0,
    grade_label                TEXT,

    notes                TEXT,
    overridden_by          UUID,
    overridden_at            TIMESTAMPTZ,
    last_synced_at             TIMESTAMPTZ,
    sync_error                   TEXT,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (report_card_id, subject_id)
);

CREATE INDEX report_card_subjects_report_card_idx ON report_card_subjects (report_card_id);
CREATE INDEX report_card_subjects_school_idx ON report_card_subjects (school_id);

-- report_grade_bands: per-school (NULL school_id = global default) predikat
-- thresholds used to label a numeric score (e.g. 90-100 => "A"). Admin can
-- insert school-specific rows to override the built-in default bands used
-- in application code when no row exists for a school.
CREATE TABLE report_grade_bands (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id           UUID,
    min_score           NUMERIC(5,2) NOT NULL,
    max_score           NUMERIC(5,2) NOT NULL,
    label               TEXT NOT NULL,
    description         TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX report_grade_bands_school_idx ON report_grade_bands (school_id);
