-- Allow re-seeding item rubric text (indicator/labels) via UPSERT instead of
-- skip-if-any-item-exists, and add an evidence/notes field to answers.

-- Existing dev data was seeded with dot-separated codes (e.g. "A.1", straight
-- from the spreadsheet's own "Kode KPI" column) while internal/store/default_forms.go
-- has always used dash-separated codes ("A-1"). Normalize to dash so the
-- ON CONFLICT (form_id, code) upsert below updates these rows in place
-- instead of inserting duplicates alongside them. item_id (the answers FK)
-- is untouched, so existing teacher_evaluation_answers rows stay intact.
UPDATE teacher_evaluation_items SET code = REPLACE(code, '.', '-') WHERE code LIKE '%.%';

ALTER TABLE teacher_evaluation_items ADD CONSTRAINT ux_tei_form_code UNIQUE (form_id, code);

ALTER TABLE teacher_evaluation_answers ADD COLUMN notes TEXT NOT NULL DEFAULT '';
