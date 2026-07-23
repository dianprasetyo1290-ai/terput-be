CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- class_sessions: one row per (schedule_id, session_date). Created when the
-- guru does their self check-in for a schedule slot (academic-service owns
-- the schedule/classroom/subject records themselves — we only reference
-- their UUIDs). Also carries the "Agenda Pembelajaran Harian" fields filled
-- in right after check-in, and doubles as the teacher-attendance record for
-- the admin "Presensi Guru" view.
CREATE TABLE class_sessions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    term_id           UUID,                -- academic-service term/semester id, optional (denormalized, set at check-in)
    schedule_id       UUID NOT NULL,       -- academic-service schedules.id (jam pelajaran slot)
    classroom_id      UUID NOT NULL,       -- academic-service classrooms.id (denormalized for roster/dashboard queries)
    subject_id        UUID,                -- academic-service subjects.id (denormalized, optional)
    guru_id           UUID NOT NULL,       -- identity-service users.id (the guru who checked in)
    session_date      DATE NOT NULL,

    -- guru self check-in (step 1 of the daily flow)
    checked_in_at     TIMESTAMPTZ,
    slot_start_time   TIME,                -- schedule slot start (denormalized from academic-service at check-in time)
    slot_end_time     TIME,                -- schedule slot end; used to compute "aktif" for the dashboard

    -- Agenda Pembelajaran Harian (step 2 of the daily flow)
    pertemuan         TEXT,                -- meeting number, typed free text (e.g. "12")
    pembahasan        TEXT,
    materi_pertemuan  TEXT,
    bentuk_kegiatan   JSONB NOT NULL DEFAULT '[]'::jsonb, -- array of: teori, praktikum, formatif, sumatif_tengah_semester, sumatif_akhir_jenjang
    agenda_filled_at  TIMESTAMPTZ,

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT class_sessions_schedule_date_unique UNIQUE (schedule_id, session_date)
);

CREATE INDEX idx_class_sessions_school ON class_sessions (school_id, academic_year_id);
CREATE INDEX idx_class_sessions_guru ON class_sessions (guru_id, session_date);
CREATE INDEX idx_class_sessions_classroom ON class_sessions (classroom_id, session_date);
CREATE INDEX idx_class_sessions_active ON class_sessions (session_date, slot_end_time) WHERE checked_in_at IS NOT NULL;

-- student_attendances: per-student attendance for a class_session (step 3 of
-- the daily flow). One row per (class_session_id, student_id). Default
-- status is 'hadir' unless the guru overrides it.
CREATE TABLE student_attendances (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_session_id  UUID NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    term_id           UUID,
    classroom_id      UUID NOT NULL,
    student_id        UUID NOT NULL,       -- identity-service users.id
    status            TEXT NOT NULL DEFAULT 'hadir'
                        CHECK (status IN ('hadir', 'sakit', 'izin', 'tanpa_keterangan')),
    catatan           TEXT,
    marked_by         UUID,                -- guru user id who set/overrode this status
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT student_attendances_session_student_unique UNIQUE (class_session_id, student_id)
);

CREATE INDEX idx_student_attendances_student ON student_attendances (student_id, school_id);
CREATE INDEX idx_student_attendances_classroom ON student_attendances (classroom_id, school_id);
CREATE INDEX idx_student_attendances_session ON student_attendances (class_session_id);
