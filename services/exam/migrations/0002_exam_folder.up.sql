-- Remembers which Bank Soal folder an exam was created from, so the
-- "Tambah Soal Baru" builder in ExamDetailView doesn't need to re-ask for a
-- folder mid-authoring (client spec 2026-07-24: guru enters a folder, hits
-- Kelola Ujian, the folder context should stick). Nullable — exams created
-- before this migration (or via the old folder-less flow) fall back to the
-- existing manual folder picker.
ALTER TABLE exams ADD COLUMN folder_id UUID REFERENCES question_folders(id);
