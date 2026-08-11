-- Bug Report SEKOCI (GPM 5 Agustus 2026), ROLE GURU -> Fitur Absensi ->
-- "1. Pergantian Guru / Assign Guru": supersedes the old direct-reassign
-- behavior (POST /sessions/substitute-guru immediately overwrote
-- class_sessions.guru_id). Now a guru names a colleague and it's a real
-- request/response flow -- guru_id only actually moves once the target
-- guru accepts. The old columns added in 0004 (original_guru_id/
-- substituted_at/substituted_by on class_sessions) are reused as the audit
-- trail for the reassignment that happens on accept; this table only
-- tracks the request/response state machine itself.
CREATE TABLE substitute_requests (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    term_id           UUID,
    schedule_id       UUID NOT NULL,       -- academic-service schedules.id (jam pelajaran slot)
    classroom_id      UUID NOT NULL,
    subject_id        UUID,
    session_date      DATE NOT NULL,
    slot_start_time   TIME,
    slot_end_time     TIME,

    requested_by      UUID NOT NULL,       -- Guru A (unavailable, wants a substitute)
    requested_to      UUID NOT NULL,       -- Guru B (the proposed substitute)
    reason            TEXT NOT NULL CHECK (reason IN ('sakit', 'izin', 'cuti')),
    status            TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'accepted', 'rejected')),

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at      TIMESTAMPTZ
);

-- Guru B's "Jadwal Saya" incoming-pending list.
CREATE INDEX idx_substitute_requests_requested_to ON substitute_requests (requested_to, status);
-- Guru A's own outgoing requests (so they can see a rejection and re-request).
CREATE INDEX idx_substitute_requests_requested_by ON substitute_requests (requested_by, status);
CREATE INDEX idx_substitute_requests_slot ON substitute_requests (schedule_id, session_date);
