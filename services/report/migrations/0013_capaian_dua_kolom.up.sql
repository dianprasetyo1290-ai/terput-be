-- Report Sekoci V15 item 1: Deskripsi Rapor gets a SECOND capaian column for
-- each exam, so a guru can write two separate achievement sentences per
-- bucket instead of cramming both into one box.
--
-- Nullable and additive on purpose: every existing row keeps its single
-- mid_note/final_note as capaian #1 and simply has no #2 yet.
ALTER TABLE report_card_subjects ADD COLUMN IF NOT EXISTS mid_note2 TEXT;
ALTER TABLE report_card_subjects ADD COLUMN IF NOT EXISTS final_note2 TEXT;
