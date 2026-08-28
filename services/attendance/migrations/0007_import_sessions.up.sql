-- 0007_import_sessions.up.sql
-- Admin Excel import of historical attendance (client revision, GPM School
-- 2026-08-24): bulk-imported days aren't tied to a real schedule_id slot,
-- so it must become nullable. `source` distinguishes import-created rows
-- from the guru's own check-in flow; the partial unique index is the
-- idempotent upsert target the import handler relies on (re-uploading the
-- same month must not create duplicate sessions).
ALTER TABLE class_sessions ALTER COLUMN schedule_id DROP NOT NULL;

ALTER TABLE class_sessions
  ADD COLUMN source TEXT NOT NULL DEFAULT 'checkin' CHECK (source IN ('checkin', 'import'));

CREATE UNIQUE INDEX class_sessions_import_classroom_date_unique
  ON class_sessions (classroom_id, session_date) WHERE source = 'import';
