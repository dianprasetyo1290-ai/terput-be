-- Item 4 (Sekolah GPM 2026-08-05): "Rapor dibuat dinamis, diatur di Admin".
-- Two config tables drive both report types instead of hardcoded weights:
--
-- report_assessment_types: the "tipe penilaian" an admin defines (tugas,
-- praktek, STS, SAS, ...) each tagged with a `bucket` (which report column
-- it rolls up into: formatif/praktik/sts/sas) and its own weight_percent
-- used to pool multiple types sharing a bucket (e.g. tugas 60% + kuis 40%
-- both feeding "Nilai Formatif"). exam_linked marks STS/SAS-style types
-- whose score can be auto-populated from exam-service (item 5).
CREATE TABLE report_assessment_types (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id      UUID NOT NULL,
    code           VARCHAR(30) NOT NULL,
    name           VARCHAR(100) NOT NULL,
    bucket         VARCHAR(20) NOT NULL CHECK (bucket IN ('formatif', 'praktik', 'sts', 'sas')),
    weight_percent NUMERIC(5,2) NOT NULL DEFAULT 100,
    exam_linked    BOOLEAN NOT NULL DEFAULT false,
    sort_order     INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, code)
);

-- report_weight_settings: the top-level formula weights for Nilai Akhir
-- (semester report, from Formatif/STS/SAS buckets) and Nilai Sementara
-- (Rapot Bayangan, from Formatif/Praktik/STS buckets per the client's
-- explicit "formatif 30 / praktik 40 / sts 30" weight list -- see
-- handler/report_card.go's calcNilaiSementara comment for the source
-- text's formatif+sts-only inconsistency this resolves). One row per
-- school; a school with no row gets illustrative defaults, same
-- "simplest reasonable call" precedent as report_grade_bands.
CREATE TABLE report_weight_settings (
    school_id                 UUID PRIMARY KEY,
    akhir_formatif_weight     NUMERIC(5,2) NOT NULL DEFAULT 30,
    akhir_sts_weight          NUMERIC(5,2) NOT NULL DEFAULT 30,
    akhir_sas_weight          NUMERIC(5,2) NOT NULL DEFAULT 40,
    sementara_formatif_weight NUMERIC(5,2) NOT NULL DEFAULT 30,
    sementara_praktik_weight  NUMERIC(5,2) NOT NULL DEFAULT 40,
    sementara_sts_weight      NUMERIC(5,2) NOT NULL DEFAULT 30,
    updated_by                UUID,
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Item 5: Penilaian Formatif UX rework -- spreadsheet add/remove-column
-- grid replaced by a dropdown picking one report_assessment_types row per
-- score entry. formative_headers (per-column display name) is entirely
-- superseded by report_assessment_types.name, and the old order_index
-- grid shape no longer applies -- both dropped/recreated clean (dev-only
-- data, see docs/CONVENTIONS.md / no prod risk for this service).
DROP TABLE IF EXISTS formative_headers;
DROP TABLE IF EXISTS formative_grades;

CREATE TABLE formative_grades (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id            UUID NOT NULL,
    term_id              UUID NOT NULL,
    classroom_subject_id UUID NOT NULL,
    student_id           UUID NOT NULL,
    assessment_type_id   UUID NOT NULL REFERENCES report_assessment_types(id),
    score                NUMERIC(5,2) NOT NULL,
    created_by           UUID,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (term_id, classroom_subject_id, student_id, assessment_type_id)
);
CREATE INDEX idx_formative_grades_lookup ON formative_grades (term_id, classroom_subject_id, student_id);

-- Item 3: Rapot Bayangan reuses report_cards/report_card_subjects (same
-- header snapshot, same override/finalize/unlock lifecycle) rather than a
-- parallel table set -- report_type discriminates which weight formula
-- and column set a row represents. period_label carries the quarterly
-- label (e.g. "Triwulan 1") since academic-service's Term model only
-- knows ganjil/genap, not quarters -- NOT NULL DEFAULT '' (not nullable)
-- so the extended UNIQUE constraint below still dedupes semester rows
-- correctly (Postgres treats NULL as distinct-from-NULL in UNIQUE).
ALTER TABLE report_cards ADD COLUMN report_type VARCHAR(20) NOT NULL DEFAULT 'semester' CHECK (report_type IN ('semester', 'bayangan'));
ALTER TABLE report_cards ADD COLUMN period_label TEXT NOT NULL DEFAULT '';
ALTER TABLE report_cards DROP CONSTRAINT report_cards_student_id_classroom_id_term_id_key;
ALTER TABLE report_cards ADD CONSTRAINT report_cards_unique_key UNIQUE (student_id, classroom_id, term_id, report_type, period_label);

-- Item 3: Nilai Praktik, Rapot Bayangan's own bucket (distinct from Nilai
-- Formatif -- the client lists it as its own weighted column, not folded
-- into formatif). Unused (null) on semester-type rows.
ALTER TABLE report_card_subjects ADD COLUMN praktik_score NUMERIC(5,2);
