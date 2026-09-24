-- Manual attendance entry (GPM & Yadika, 2026-09-24): both schools only
-- started using Sekoci in September, so the absensi-siswa feature has no
-- history to summarise and every rapor prints Sakit/Izin/Tanpa Keterangan
-- as 0. This flag lets a wali kelas type the real figures in.
--
-- Deliberately a flag over four extra "manual_*" columns: the numbers land
-- in the existing attendance_* columns, so the rapor detail view, the print
-- view and the DOCX mail-merge all keep reading exactly one place. The flag
-- only tells GenerateReportCards to leave those columns alone, the same way
-- report_card_subjects.override_score survives a regenerate.
ALTER TABLE report_cards
    ADD COLUMN attendance_manual BOOLEAN NOT NULL DEFAULT false;
