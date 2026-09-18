-- Format Rapor (3).xlsx, sheet "Form Raport Akhir Smstr(6 bln)": the akhir-
-- semester form has two sections the tengah-semester form doesn't --
-- "IV. PRESTASI" (No / Jenis Prestasi / Keterangan) and "VII. KENAIKAN
-- KELAS" ("dinyatakan: Naik / Tidak Naik ke Kelas ..."), the latter printed
-- only on a semester genap. Same shape and lifecycle as kokurikuler_note /
-- ekstrakurikuler in migration 0010, so it lives on the same row.
ALTER TABLE report_cards
    ADD COLUMN prestasi        JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN kenaikan_status TEXT,
    ADD COLUMN kenaikan_kelas  TEXT;
