-- 0007 (QA-PAY BUG-137): at most one active Menu 78 request per student.
-- "Active" = pending_approval or approved-but-not-applied, matching
-- model.IsActiveStatusRequest. Existing duplicates are resolved first by
-- keeping the oldest active request per student and cancelling the newer
-- ones (none of them can have been applied, so nothing else changes).

UPDATE student_status_requests r
SET status = 'cancelled', updated_at = now()
WHERE r.status IN ('pending_approval', 'approved')
  AND EXISTS (
      SELECT 1 FROM student_status_requests o
      WHERE o.school_id = r.school_id
        AND o.student_user_id = r.student_user_id
        AND o.status IN ('pending_approval', 'approved')
        AND (o.created_at, o.id) < (r.created_at, r.id)
  );

CREATE UNIQUE INDEX IF NOT EXISTS uq_student_status_requests_active
    ON student_status_requests (school_id, student_user_id)
    WHERE status IN ('pending_approval', 'approved');
