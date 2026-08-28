DROP INDEX IF EXISTS class_sessions_import_classroom_date_unique;
ALTER TABLE class_sessions DROP COLUMN source;
ALTER TABLE class_sessions ALTER COLUMN schedule_id SET NOT NULL;
