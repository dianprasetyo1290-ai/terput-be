-- Matriks Pelajaran's Auto-Enroll Agama (academic-service) needs a source of
-- truth for each student's religion to match against Agama subject variants
-- (e.g. "Islam" -> a subject named "Agama Islam"). No such field existed on
-- the student profile at all.
ALTER TABLE users ADD COLUMN religion TEXT;
