ALTER TABLE report_card_subjects DROP COLUMN IF EXISTS praktik_score;

ALTER TABLE report_cards DROP CONSTRAINT IF EXISTS report_cards_unique_key;
ALTER TABLE report_cards ADD CONSTRAINT report_cards_student_id_classroom_id_term_id_key UNIQUE (student_id, classroom_id, term_id);
ALTER TABLE report_cards DROP COLUMN IF EXISTS period_label;
ALTER TABLE report_cards DROP COLUMN IF EXISTS report_type;

DROP TABLE IF EXISTS formative_grades;

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

CREATE TABLE formative_headers (
    classroom_subject_id UUID NOT NULL,
    term_id UUID NOT NULL,
    order_index INT NOT NULL,
    header_name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (classroom_subject_id, term_id, order_index)
);

DROP TABLE IF EXISTS report_weight_settings;
DROP TABLE IF EXISTS report_assessment_types;
