-- 0032_tax_seed.up.sql: Default Seed Types for Withholding Taxes
-- PPh 23 (Jasa 200 bp / no-NPWP 400 bp), PPh 4(2) (Sewa 1000 bp), PPh 22 (Barang 150 bp / 300 bp).

CREATE OR REPLACE FUNCTION seed_default_tax_types(p_school_id UUID)
RETURNS void AS $$
BEGIN
    INSERT INTO withholding_tax_types (school_id, code, name, article, standard_rate_bp, no_npwp_rate_bp, description)
    VALUES
        (p_school_id, 'PPH23_SERVICE', 'PPh 23 - Jasa Teknik, Manajemen, Konsultasi & Jasa Lain', '23', 200, 400, 'Tarif normal 2%, tanpa NPWP 4%'),
        (p_school_id, 'PPH4_2_RENT', 'PPh 4(2) - Sewa Tanah dan/atau Bangunan', '4(2)', 1000, 1000, 'PPh Final tarif 10%'),
        (p_school_id, 'PPH4_2_CONSTRUCT', 'PPh 4(2) - Jasa Pelaksanaan Konstruksi', '4(2)', 175, 175, 'PPh Final tarif 1.75% (kualifikasi kecil)'),
        (p_school_id, 'PPH22_GOODS', 'PPh 22 - Pembelian Barang / Pengadaan', '22', 150, 300, 'Tarif normal 1.5%, tanpa NPWP 3%')
    ON CONFLICT (school_id, code) DO NOTHING;
END;
$$ LANGUAGE plpgsql;

-- Jalankan untuk sekolah yang sudah terdaftar di sistem jika ada
DO $$
DECLARE
    r RECORD;
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'facility_settings') THEN
        FOR r IN EXECUTE 'SELECT DISTINCT school_id FROM facility_settings' LOOP
            PERFORM seed_default_tax_types(r.school_id);
        END LOOP;
    ELSIF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'tax_settings') THEN
        FOR r IN SELECT DISTINCT school_id FROM tax_settings LOOP
            PERFORM seed_default_tax_types(r.school_id);
        END LOOP;
    END IF;
END;
$$;
