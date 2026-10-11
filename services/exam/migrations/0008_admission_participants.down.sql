-- Rollback is intentionally guarded: applicant data cannot be represented by
-- the legacy student-only schema and must be removed explicitly first.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM exam_attempts WHERE participant_type <> 'student')
       OR EXISTS (SELECT 1 FROM exams WHERE audience <> 'academic') THEN
        RAISE EXCEPTION 'cannot rollback admission participants while admission data exists';
    END IF;
END $$;

DROP TABLE participant_access_tokens;

ALTER TABLE exams DROP CONSTRAINT exams_audience_context_check;
ALTER TABLE exam_attempts DROP CONSTRAINT exam_attempts_typed_legacy_ref_check;
ALTER TABLE exam_attempts DROP CONSTRAINT exam_attempts_exam_participant_key;
DROP INDEX idx_exam_attempts_participant;

ALTER TABLE exam_attempts
    ALTER COLUMN academic_year_id SET NOT NULL,
    ALTER COLUMN user_id SET NOT NULL,
    ALTER COLUMN classroom_id SET NOT NULL,
    ADD CONSTRAINT exam_attempts_exam_id_user_id_key UNIQUE (exam_id, user_id),
    DROP COLUMN participant_id,
    DROP COLUMN participant_type;

DROP INDEX idx_exams_admission_external_ref;
DROP TABLE exam_participants;

ALTER TABLE exams
    ALTER COLUMN academic_year_id SET NOT NULL,
    ALTER COLUMN term_id SET NOT NULL,
    ALTER COLUMN exam_type_id SET NOT NULL,
    ALTER COLUMN classroom_id SET NOT NULL,
    ALTER COLUMN subject_id SET NOT NULL,
    ALTER COLUMN teacher_id SET NOT NULL,
    DROP COLUMN admission_context,
    DROP COLUMN audience;
