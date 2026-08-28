-- Hide a folder from the Bank Soal list without deleting it or its
-- questions (client request, GPM School 2026-08-28: "Migrasi Ujian Lama"
-- was cluttering the folder list right as guru started inputting UTS
-- questions for the mid-semester exam -- reversible, unlike deleted_at
-- which also blocks access to the folder's own questions).
ALTER TABLE question_folders ADD COLUMN is_hidden BOOLEAN NOT NULL DEFAULT false;

UPDATE question_folders SET is_hidden = true WHERE name = 'Migrasi Ujian Lama';
