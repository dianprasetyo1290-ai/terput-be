ALTER TABLE student_attendances
  DROP CONSTRAINT IF EXISTS student_attendances_session_student_unique;

ALTER TABLE student_attendances
  ADD COLUMN session_date DATE;

UPDATE student_attendances sa 
  SET session_date = cs.session_date 
  FROM class_sessions cs 
  WHERE sa.class_session_id = cs.id;

-- In case there are missing session_dates due to dangling rows (shouldn't happen because of FK), default to today
UPDATE student_attendances SET session_date = CURRENT_DATE WHERE session_date IS NULL;

ALTER TABLE student_attendances
  ALTER COLUMN session_date SET NOT NULL;

-- Remove duplicates before adding unique constraint
DELETE FROM student_attendances a USING (
    SELECT MIN(ctid) as ctid, classroom_id, session_date, student_id
    FROM student_attendances 
    GROUP BY classroom_id, session_date, student_id HAVING COUNT(*) > 1
) b
WHERE a.classroom_id = b.classroom_id AND a.session_date = b.session_date AND a.student_id = b.student_id AND a.ctid <> b.ctid;

ALTER TABLE student_attendances
  ADD CONSTRAINT student_attendances_classroom_date_student_unique UNIQUE (classroom_id, session_date, student_id);

-- Optional: class_session_id is no longer the strict parent (since multiple sessions share this row)
-- But we can leave it to track WHICH session initiated it. 
