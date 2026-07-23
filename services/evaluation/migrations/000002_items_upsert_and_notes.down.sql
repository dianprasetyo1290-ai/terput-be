ALTER TABLE teacher_evaluation_answers DROP COLUMN IF EXISTS notes;
ALTER TABLE teacher_evaluation_items DROP CONSTRAINT IF EXISTS ux_tei_form_code;
