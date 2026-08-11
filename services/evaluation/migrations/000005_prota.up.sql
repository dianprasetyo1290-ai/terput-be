-- Bug Report SEKOCI (5 Agustus 2026), NEW FEATURE: Program Tahunan (Prota).
-- Not built before this migration. Excel-import-driven, parent + line-items
-- (many rows import per one Prota, unlike RPM's single-record dimensi
-- import) -- see Template_Prota_LMS_Ready.xlsx sheet TEMPLATE_PROTA:
-- No | Semester | Elemen/Kode Bab | Kompetensi Dasar/TP | Alokasi Waktu (JP) | Keterangan/Target Asesmen
CREATE TABLE prota (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    guru_user_id      UUID NOT NULL,
    mata_pelajaran    TEXT NOT NULL,
    kelas             TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ
);
CREATE INDEX idx_prota_school ON prota(school_id);
CREATE INDEX idx_prota_guru ON prota(guru_user_id, academic_year_id);

CREATE TABLE prota_items (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prota_id                    UUID NOT NULL REFERENCES prota(id) ON DELETE CASCADE,
    no                          INT NOT NULL,
    semester                    TEXT NOT NULL,
    elemen_kode_bab             TEXT NOT NULL DEFAULT '',
    kompetensi_dasar_tp         TEXT NOT NULL DEFAULT '',
    alokasi_waktu_jp            INT NOT NULL DEFAULT 0,
    keterangan_target_asesmen   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_prota_items_prota ON prota_items(prota_id, no);
