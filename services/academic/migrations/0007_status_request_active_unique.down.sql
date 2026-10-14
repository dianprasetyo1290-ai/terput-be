-- Requests cancelled by the up migration stay cancelled (not reversible).
DROP INDEX IF EXISTS uq_student_status_requests_active;
