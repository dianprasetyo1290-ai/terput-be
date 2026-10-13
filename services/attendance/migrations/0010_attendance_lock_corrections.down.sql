-- ponytail: data input manual (source='manual') dan status terlambat ikut
-- hilang/menjadi hadir saat rollback -- sama seperti 0007, rollback hanya
-- untuk dev.
DROP TABLE IF EXISTS audit_outbox;
DROP TABLE IF EXISTS alpa_notifications;
DROP TABLE IF EXISTS attendance_corrections;
DROP TABLE IF EXISTS daily_attendance_submissions;
DROP TABLE IF EXISTS attendance_lock_settings;
DELETE FROM class_sessions WHERE source = 'manual';
DROP INDEX IF EXISTS class_sessions_manual_classroom_date_unique;
ALTER TABLE class_sessions DROP CONSTRAINT IF EXISTS class_sessions_source_check;
ALTER TABLE class_sessions ADD CONSTRAINT class_sessions_source_check CHECK (source IN ('checkin', 'import'));
UPDATE student_attendances SET status = 'hadir' WHERE status = 'terlambat';
ALTER TABLE student_attendances DROP CONSTRAINT student_attendances_status_check;
ALTER TABLE student_attendances ADD CONSTRAINT student_attendances_status_check
    CHECK (status IN ('belum_presensi', 'hadir', 'sakit', 'izin', 'tanpa_keterangan'));
