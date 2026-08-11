-- Client request (Sekolah GPM, guru absensi jadwal flow):
--   1. "tugas" column on the attendance session: checkbox Tugas Individu /
--      Tugas Kelompok (whether homework was given, and which type).
--   2. "pergantian guru": a guru can hand a teaching slot off to another
--      guru who covers it instead ("reassign who's covering a jadwal").
--   3. catatan (notes) field on the session itself, e.g. "ada tugas yang
--      harus diselesaikan hari itu juga" -- distinct from the existing
--      per-student student_attendances.catatan.
--   4. jam masuk / jam keluar per pertemuan -- checked_in_at already serves
--      as jam masuk, this adds the matching clock-out timestamp.
ALTER TABLE class_sessions
    ADD COLUMN checked_out_at   TIMESTAMPTZ,      -- jam keluar (checked_in_at is jam masuk)
    ADD COLUMN catatan          TEXT,             -- session-level notes (e.g. "ada tugas hari itu juga")
    ADD COLUMN tugas_diberikan  BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN tugas_jenis      TEXT,             -- 'individu' | 'kelompok', only meaningful when tugas_diberikan
    -- pergantian guru: guru_id is reassigned to whoever actually covers the
    -- slot; original_guru_id/substituted_at/substituted_by keep the audit
    -- trail of who was originally supposed to teach and who made the call.
    ADD COLUMN original_guru_id UUID,
    ADD COLUMN substituted_at   TIMESTAMPTZ,
    ADD COLUMN substituted_by   UUID,
    ADD CONSTRAINT class_sessions_tugas_jenis_check
        CHECK (tugas_jenis IS NULL OR tugas_jenis IN ('individu', 'kelompok'));
