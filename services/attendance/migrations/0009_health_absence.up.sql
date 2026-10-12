-- 0009_health_absence.up.sql (T10, menu 128 AC-128-01)
-- Absen sakit yang dicatat UKS lewat POST /internal/student-absences/health.
-- source_ref = id kunjungan UKS; UNIQUE parsial membuat panggilan ulang idempoten.
ALTER TABLE leave_requests
    ADD COLUMN source TEXT NOT NULL DEFAULT 'parent' CHECK (source IN ('parent', 'uks')),
    ADD COLUMN source_ref UUID NULL,
    ADD COLUMN sick_since TIMESTAMPTZ NULL;

CREATE UNIQUE INDEX uq_leave_requests_source_ref ON leave_requests(source, source_ref) WHERE source_ref IS NOT NULL;
