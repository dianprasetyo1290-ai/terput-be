-- 0008_leave_requests.up.sql
-- PRD menu 42 (Leave) + 48 (Attendance & Leave): orang tua submits an
-- izin/sakit request for a linked child; wali kelas (own classroom) or
-- admin approves/rejects with a note. BR-ATT-02: an approved request turns
-- the student's attendance on the covered dates into izin/sakit (applied
-- by the handler to existing rows, and to rows seeded/bulk-marked later).
-- The attachment (surat dokter etc.) is stored inline -- capped at 2 MB by
-- the handler -- so no new volume/bucket is needed.
CREATE TABLE IF NOT EXISTS leave_requests (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        UUID NOT NULL,
    academic_year_id UUID,
    classroom_id     UUID,                -- student's active classroom at submit time (scopes wali kelas)
    student_id       UUID NOT NULL,       -- identity users.id
    requested_by     UUID NOT NULL,       -- orang tua users.id
    type             TEXT NOT NULL CHECK (type IN ('izin', 'sakit')),
    start_date       DATE NOT NULL,
    end_date         DATE NOT NULL,
    reason           TEXT NOT NULL,
    attachment_name  TEXT,
    attachment_type  TEXT,
    attachment       BYTEA,
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewed_by      UUID,
    reviewed_at      TIMESTAMPTZ,
    review_note      TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT leave_requests_dates CHECK (end_date >= start_date)
);

CREATE INDEX IF NOT EXISTS idx_leave_requests_school_status ON leave_requests (school_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_leave_requests_classroom ON leave_requests (classroom_id, status);
CREATE INDEX IF NOT EXISTS idx_leave_requests_student_dates ON leave_requests (student_id, start_date, end_date);
