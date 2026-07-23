-- Penilaian Formatif: guru enters an arbitrary number of assignment/task
-- scores per student per (term, classroom_subject) -- classroom_subject_id
-- is academic-service's classroom_subjects.id (the specific class+subject+
-- teacher pairing), scoping this to the guru's own class. order_index is
-- the column position in the guru's grid (1..N, matching the old app's
-- addColumn/removeLastColumn spreadsheet-style UI). Pooled into
-- report_card_subjects.computed_score alongside exam scores at Generate
-- Rapor time -- matches the old Laravel app's flat pooled-average model
-- (see ReportCardService::calculateFinalScoreFromAgg).
CREATE TABLE formative_grades (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id           UUID NOT NULL,
    term_id             UUID NOT NULL,
    classroom_subject_id UUID NOT NULL,
    student_id          UUID NOT NULL,
    order_index         INT NOT NULL,
    score               NUMERIC(5,2) NOT NULL,
    created_by          UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (term_id, classroom_subject_id, student_id, order_index)
);
CREATE INDEX idx_formative_grades_lookup ON formative_grades (term_id, classroom_subject_id, student_id);

-- Deskripsi Rapor: narrative achievement notes per (report_card_subject),
-- split mid/final term like the old app's mid_term_note/final_term_note
-- (report_card_subjects only ever had one generic `notes` column so far).
ALTER TABLE report_card_subjects ADD COLUMN mid_note TEXT;
ALTER TABLE report_card_subjects ADD COLUMN final_note TEXT;

-- Per-school template text (with {{placeholders}}) used to auto-generate
-- mid/final notes for every student at once -- old app's "Atur Template
-- Deskripsi" dialog. One row per school (not per subject: the old app's
-- template dialog was global per teacher session, reused across subjects).
CREATE TABLE report_note_templates (
    school_id   UUID PRIMARY KEY,
    mid_template   TEXT NOT NULL DEFAULT '',
    final_template TEXT NOT NULL DEFAULT '',
    updated_by  UUID,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
