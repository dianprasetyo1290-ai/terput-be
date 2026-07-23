-- Sekoci import template rewrite (Indonesian, 8 sheets). Two new sheets
-- (Data Orang Tua, Data Ekstrakurikuler) have no dedicated model yet, so
-- they're staged the same way Data Kelas / Data mata pelajaran already are.
ALTER TABLE import_staged_rows DROP CONSTRAINT import_staged_rows_kind_check;
ALTER TABLE import_staged_rows ADD CONSTRAINT import_staged_rows_kind_check
    CHECK (kind IN ('kelas','mata_pelajaran','orang_tua','ekstrakurikuler'));

-- Guru/Siswa import fields with no first-class column yet (NUPTK, NISN,
-- Pendidikan Terakhir, Asal Sekolah, Nama Orang Tua, per-row Tahun
-- Akademik) — catch-all JSONB instead of one narrow migration per field;
-- promote to a real column later if something needs to query on it.
ALTER TABLE users ADD COLUMN extra JSONB;
