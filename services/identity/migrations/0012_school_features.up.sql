-- Saklar fitur per sekolah (ADDENDUM 2): superadmin menyalakan/mematikan
-- modul opsional untuk tiap sekolah. feature_key pertama: 'accounting'.
--
-- Default MATI secara struktural: tidak ada baris = fitur mati. Jadi tidak
-- perlu backfill untuk sekolah yang sudah ada, dan sekolah baru tidak
-- tiba-tiba punya modul keuangan.
CREATE TABLE school_features (
    school_id   UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    feature_key TEXT NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT false,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (school_id, feature_key)
);
