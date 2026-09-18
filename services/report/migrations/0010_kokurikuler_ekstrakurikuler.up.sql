-- SEKOCI V10 item B: the printed rapor gains "II. Kokurikuler" (one
-- free-text "Deskripsi Perkembangan Siswa") and "III. Ekstrakurikuler" (a
-- variable-length kegiatan/predikat/keterangan list) below "C. Muatan
-- Lokal". "IV. Kehadiran" and "V. Tanggapan Wali Kelas" need no new
-- storage -- attendance_* and class_teacher_note are already on this table,
-- they were simply never printed.
--
-- Both ride report_cards rather than earning their own tables: they are
-- per-report-card, follow the same draft/finalize/unlock lifecycle as the
-- scores, and are only ever read or written as a whole (the print view
-- renders the list, the admin form replaces it) -- so there is nothing to
-- join, filter or aggregate that would pay for a second table.
ALTER TABLE report_cards
    ADD COLUMN kokurikuler_note TEXT,
    ADD COLUMN ekstrakurikuler  JSONB NOT NULL DEFAULT '[]'::jsonb;
