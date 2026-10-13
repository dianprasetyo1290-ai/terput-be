-- 0010_attendance_lock_corrections.up.sql (P4A — PRD menu 41/42/69)
--
-- FR-41-001: status "terlambat" (hadir tapi terlambat).
ALTER TABLE student_attendances DROP CONSTRAINT student_attendances_status_check;
ALTER TABLE student_attendances ADD CONSTRAINT student_attendances_status_check
    CHECK (status IN ('belum_presensi', 'hadir', 'terlambat', 'sakit', 'izin', 'tanpa_keterangan'));

-- FR-41-002 / FR-69-001: input harian manual oleh admin / wali kelas tidak
-- terikat slot jadwal, sama seperti import (migration 0007) — source baru
-- 'manual' dengan indeks unik parsialnya sendiri sebagai target upsert.
ALTER TABLE class_sessions DROP CONSTRAINT IF EXISTS class_sessions_source_check;
ALTER TABLE class_sessions ADD CONSTRAINT class_sessions_source_check
    CHECK (source IN ('checkin', 'import', 'manual'));
CREATE UNIQUE INDEX class_sessions_manual_classroom_date_unique
    ON class_sessions (classroom_id, session_date) WHERE source = 'manual';

-- FR-41-003 / BR-ATT-01 [per klien]: batas kunci absensi harian per sekolah.
-- Tanggal D terkunci mulai (D + cutoff_days) pukul cutoff_time WIB.
-- Tanpa baris = penguncian nonaktif (kompatibel dengan klien lama).
CREATE TABLE attendance_lock_settings (
    school_id    UUID PRIMARY KEY,
    enabled      BOOLEAN NOT NULL DEFAULT false,
    cutoff_days  INT NOT NULL DEFAULT 0 CHECK (cutoff_days BETWEEN 0 AND 31),
    cutoff_time  TIME NOT NULL DEFAULT '23:59',
    updated_by   UUID,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- FR-69-001: status kirim absensi harian kelas oleh wali kelas.
CREATE TABLE daily_attendance_submissions (
    classroom_id  UUID NOT NULL,
    session_date  DATE NOT NULL,
    school_id     UUID NOT NULL,
    status        TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted')),
    catatan       TEXT,
    submitted_by  UUID,
    submitted_at  TIMESTAMPTZ,
    updated_by    UUID,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (classroom_id, session_date)
);

-- BR-ATT-01: koreksi absensi yang sudah terkunci -> Approval Center core
-- (request_type attendance_correction) -> callback menerapkan perubahan.
CREATE TABLE attendance_corrections (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    term_id           UUID,
    classroom_id      UUID NOT NULL,
    student_id        UUID NOT NULL,
    session_date      DATE NOT NULL,
    from_status       TEXT,
    to_status         TEXT NOT NULL CHECK (to_status IN ('hadir', 'terlambat', 'sakit', 'izin', 'tanpa_keterangan')),
    catatan           TEXT,
    reason            TEXT NOT NULL,
    requested_by      UUID NOT NULL,
    requested_role    TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    approval_id       TEXT,
    decided_by        TEXT,
    decision_comment  TEXT,
    decided_at        TIMESTAMPTZ,
    applied_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_attendance_corrections_school ON attendance_corrections (school_id, status, created_at DESC);
CREATE UNIQUE INDEX uq_attendance_corrections_pending
    ON attendance_corrections (classroom_id, session_date, student_id) WHERE status = 'pending';

-- FR-41-004: dedupe notifikasi alpa ke orang tua — satu per siswa per hari.
CREATE TABLE alpa_notifications (
    student_id    UUID NOT NULL,
    session_date  DATE NOT NULL,
    school_id     UUID NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (student_id, session_date)
);

-- Outbox audit -> core POST /internal/audit (pola ops/hr): baris ditulis
-- bersama perubahan, forwarder latar mengirimnya dan menandai forwarded_at.
CREATE TABLE audit_outbox (
    id              BIGSERIAL PRIMARY KEY,
    school_id       UUID NOT NULL,
    module          TEXT NOT NULL,
    entity_type     TEXT NOT NULL,
    entity_id       TEXT NOT NULL,
    action          TEXT NOT NULL,
    actor_user_id   UUID,
    actor_role      TEXT,
    before          JSONB,
    after           JSONB,
    reason          TEXT,
    correlation_id  TEXT,
    attempts        INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    forwarded_at    TIMESTAMPTZ
);
CREATE INDEX idx_audit_outbox_pending ON audit_outbox (id) WHERE forwarded_at IS NULL;
