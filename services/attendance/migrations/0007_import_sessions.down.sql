-- ponytail: will fail with a NOT NULL violation if any source='import' rows
-- exist (their schedule_id is legitimately NULL) -- migrations here are
-- applied manually, never auto-rolled-back, so this is an accepted
-- dev-only limitation. To roll back after import data exists, first
-- DELETE FROM class_sessions WHERE source = 'import' (cascades to their
-- student_attendances rows) or backfill a real schedule_id per row.
DROP INDEX IF EXISTS class_sessions_import_classroom_date_unique;
ALTER TABLE class_sessions DROP COLUMN source;
ALTER TABLE class_sessions ALTER COLUMN schedule_id SET NOT NULL;
