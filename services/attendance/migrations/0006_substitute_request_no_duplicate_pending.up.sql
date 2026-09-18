-- Bug Report SEKOCI GPM V10, ROLE GURU (Fitur Absensi -- "Guru A tidak
-- dapat mengassign ulang guru lain untuk jadwal yang sama selama status
-- masih pending"): a partial unique index is the DB-level guarantee behind
-- the handler-level check in CreateSubstituteRequest -- closes the race a
-- plain SELECT-then-INSERT check alone can't (two concurrent requests for
-- the same slot both passing the check before either commits).
CREATE UNIQUE INDEX idx_substitute_requests_one_pending_per_slot
    ON substitute_requests (schedule_id, session_date)
    WHERE status = 'pending';
