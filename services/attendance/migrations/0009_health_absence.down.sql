DROP INDEX IF EXISTS uq_leave_requests_source_ref;
ALTER TABLE leave_requests
    DROP COLUMN IF EXISTS sick_since,
    DROP COLUMN IF EXISTS source_ref,
    DROP COLUMN IF EXISTS source;
