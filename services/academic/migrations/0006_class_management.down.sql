DROP TRIGGER IF EXISTS trg_class_enrollment_history ON class_enrollments;
DROP FUNCTION IF EXISTS log_class_enrollment_change();
DROP TABLE IF EXISTS class_enrollment_history;
ALTER TABLE classroom_subjects DROP COLUMN IF EXISTS hours_per_week;
ALTER TABLE classrooms DROP COLUMN IF EXISTS capacity;
