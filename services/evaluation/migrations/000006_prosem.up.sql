-- Bug Report SEKOCI (5 Agustus 2026), NEW FEATURE: Program Semester (Prosem).
-- Same parent + line-items pattern as prota (000005) -- see
-- Template_Prosem_LMS_Ready.xlsx sheet TEMPLATE_PROSEM:
-- No | Materi Pokok/Aktivitas | Alokasi (JP) | Bulan Target | Minggu Ke- | Estimasi Tanggal | Tipe Aktivitas LMS | Status Input (Guru)
CREATE TABLE prosem (
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
CREATE INDEX idx_prosem_school ON prosem(school_id);
CREATE INDEX idx_prosem_guru ON prosem(guru_user_id, academic_year_id);

CREATE TABLE prosem_items (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prosem_id               UUID NOT NULL REFERENCES prosem(id) ON DELETE CASCADE,
    no                      INT NOT NULL,
    materi_pokok_aktivitas  TEXT NOT NULL DEFAULT '',
    alokasi_jp              INT NOT NULL DEFAULT 0,
    bulan_target            TEXT NOT NULL DEFAULT '',
    minggu_ke               INT NOT NULL DEFAULT 0,
    estimasi_tanggal        DATE,
    tipe_aktivitas_lms      TEXT NOT NULL DEFAULT '',
    status_input_guru       TEXT NOT NULL DEFAULT 'Belum'
);
CREATE INDEX idx_prosem_items_prosem ON prosem_items(prosem_id, no);
