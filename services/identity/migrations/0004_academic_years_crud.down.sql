DROP INDEX IF EXISTS idx_academic_years_school_label;
ALTER TABLE academic_years ADD CONSTRAINT academic_years_school_id_label_key UNIQUE (school_id, label);
ALTER TABLE academic_years DROP COLUMN deleted_at;
