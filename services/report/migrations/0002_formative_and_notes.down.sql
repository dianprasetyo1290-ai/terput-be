DROP TABLE IF EXISTS report_note_templates;
ALTER TABLE report_card_subjects DROP COLUMN IF EXISTS mid_note;
ALTER TABLE report_card_subjects DROP COLUMN IF EXISTS final_note;
DROP TABLE IF EXISTS formative_grades;
