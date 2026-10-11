-- Revert 0005: student status requests and academic settings

DROP TABLE IF EXISTS student_status_requests;
DROP TABLE IF EXISTS academic_settings;

UPDATE class_enrollments SET status = 'transferred_out' WHERE status = 'withdrawn';
ALTER TABLE class_enrollments DROP CONSTRAINT IF EXISTS class_enrollments_status_check;
ALTER TABLE class_enrollments ADD CONSTRAINT class_enrollments_status_check
    CHECK (status IN ('active', 'graduated', 'transferred_out'));
