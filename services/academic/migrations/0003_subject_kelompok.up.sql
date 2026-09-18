-- SEKOCI V7 #1 (SMK GPM): the printed E-Rapor groups its subject table as
-- "A. Kelompok Mata Pelajaran Umum / B. Kelompok Mata Pelajaran Kejuruan /
-- C. Muatan Lokal". Its own column, not subject_group: subject_group already
-- means "mutually-exclusive elective variants" (Agama Islam/Kristen/...) for
-- the per-student subject filter, so reusing it would break that.
ALTER TABLE subjects ADD COLUMN kelompok TEXT
    CHECK (kelompok IN ('umum', 'kejuruan', 'muatan_lokal'));
