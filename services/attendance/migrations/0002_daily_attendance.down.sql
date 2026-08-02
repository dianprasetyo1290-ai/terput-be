ALTER TABLE student_attendances
  DROP CONSTRAINT IF EXISTS student_attendances_classroom_date_student_unique;

ALTER TABLE student_attendances
  DROP COLUMN IF EXISTS session_date;

ALTER TABLE student_attendances
  ADD CONSTRAINT student_attendances_session_student_unique UNIQUE (class_session_id, student_id);
