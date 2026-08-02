-- Role Admin V4: Data Siswa needs a persistent NISN field (previously only
-- existed as a bulk-import CSV column, never stored on the user record) so
-- it can display on Data Siswa and auto-fill on E-Raport.
ALTER TABLE users ADD COLUMN nisn TEXT;
