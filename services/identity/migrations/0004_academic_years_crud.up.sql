-- Tahun Akademik: add edit/delete support. Delete is soft (deleted_at),
-- matching the schools/subjects/classrooms convention elsewhere in this
-- codebase, because academic-service's terms/classrooms/class_enrollments
-- reference academic_year_id by plain UUID (no cross-DB FK) — a hard delete
-- would silently orphan that data with no way to warn the caller.
ALTER TABLE academic_years ADD COLUMN deleted_at TIMESTAMPTZ;

-- Swap the table-level UNIQUE for a partial index so a soft-deleted year's
-- label can be reused (same pattern as idx_users_email).
ALTER TABLE academic_years DROP CONSTRAINT academic_years_school_id_label_key;
CREATE UNIQUE INDEX idx_academic_years_school_label ON academic_years(school_id, label) WHERE deleted_at IS NULL;
