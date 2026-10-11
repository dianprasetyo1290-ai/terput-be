-- ============================================================================
-- SEKOCI PPDB COMPREHENSIVE SEED SCRIPT (PRD TP-SMS v2.1)
-- ============================================================================

\c teratai_admission;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
DECLARE
    v_school_id uuid := '11111111-1111-1111-1111-111111111111';
    v_period_id uuid := 'aaaaaaaa-0000-0000-0000-000000000001';
    v_wave1_id uuid := 'aaaaaaaa-0000-0000-0000-000000000002';
    v_wave2_id uuid := 'aaaaaaaa-0000-0000-0000-000000000022';
    v_track_reg uuid := 'aaaaaaaa-0000-0000-0000-000000000003';
    v_track_prs uuid := 'aaaaaaaa-0000-0000-0000-000000000032';
    v_track_tfz uuid := 'aaaaaaaa-0000-0000-0000-000000000033';
    v_track_afr uuid := 'aaaaaaaa-0000-0000-0000-000000000034';

    -- Requirement IDs
    v_req_akta uuid := 'aaaaaaaa-0000-0000-0000-000000000004';
    v_req_kk uuid := 'aaaaaaaa-0000-0000-0000-000000000042';
    v_req_foto uuid := 'aaaaaaaa-0000-0000-0000-000000000043';
    v_req_rapor uuid := 'aaaaaaaa-0000-0000-0000-000000000044';
    v_req_piagam uuid := 'aaaaaaaa-0000-0000-0000-000000000045';

    -- Fee Scheme IDs
    v_fee_form uuid := 'bbbbbbbb-0000-0000-0000-000000000001';
    v_fee_smp uuid := 'bbbbbbbb-0000-0000-0000-000000000002';
    v_fee_sma uuid := 'bbbbbbbb-0000-0000-0000-000000000003';
    v_fee_sd uuid := 'bbbbbbbb-0000-0000-0000-000000000004';

    -- Guardians
    v_g_nurul uuid := '11110000-0000-0000-0000-000000000001';
    v_g_david uuid := '11110000-0000-0000-0000-000000000002';
    v_g_budi uuid := '11110000-0000-0000-0000-000000000003';
    v_g_ratna uuid := '11110000-0000-0000-0000-000000000004';
    v_g_hendra uuid := '11110000-0000-0000-0000-000000000005';
    v_g_siti uuid := '11110000-0000-0000-0000-000000000006';
    v_g_syamsul uuid := '11110000-0000-0000-0000-000000000007';
    v_g_dewi uuid := '11110000-0000-0000-0000-000000000008';

    -- Applicants
    v_app_ahmad uuid := '22220000-0000-0000-0000-000000000001';
    v_app_nadia uuid := '22220000-0000-0000-0000-000000000002';
    v_app_fajar uuid := '22220000-0000-0000-0000-000000000003';
    v_app_aisyah uuid := '22220000-0000-0000-0000-000000000004';
    v_app_farhan uuid := '22220000-0000-0000-0000-000000000005';
    v_app_amanda uuid := '22220000-0000-0000-0000-000000000006';
    v_app_reyhan uuid := '22220000-0000-0000-0000-000000000007';
    v_app_kayla uuid := '22220000-0000-0000-0000-000000000008';

    -- Applications
    v_appl_ahmad uuid := '33330000-0000-0000-0000-000000000001';
    v_appl_nadia uuid := '33330000-0000-0000-0000-000000000002';
    v_appl_fajar uuid := '33330000-0000-0000-0000-000000000003';
    v_appl_aisyah uuid := '33330000-0000-0000-0000-000000000004';
    v_appl_farhan uuid := '33330000-0000-0000-0000-000000000005';
    v_appl_amanda uuid := '33330000-0000-0000-0000-000000000006';
    v_appl_reyhan uuid := '33330000-0000-0000-0000-000000000007';
    v_appl_kayla uuid := '33330000-0000-0000-0000-000000000008';

    -- Selection Component IDs
    v_cmp_tpa uuid := '44440000-0000-0000-0000-000000000001';
    v_cmp_tfz uuid := '44440000-0000-0000-0000-000000000002';
    v_cmp_wwc uuid := '44440000-0000-0000-0000-000000000003';

BEGIN
    -- 1. ADMISSION PERIOD
    INSERT INTO admission_periods (id, school_id, name, academic_year, timezone, status, registration_open_at, registration_close_at, created_at, updated_at)
    VALUES (
        v_period_id,
        v_school_id,
        'PPDB 2026/2027',
        '2026/2027',
        'Asia/Jakarta',
        'open',
        now() - interval '14 days',
        now() + interval '60 days',
        now(),
        now()
    )
    ON CONFLICT (id) DO UPDATE SET
        status = 'open',
        registration_open_at = EXCLUDED.registration_open_at,
        registration_close_at = EXCLUDED.registration_close_at,
        updated_at = now();

    -- 2. ADMISSION WAVES
    INSERT INTO admission_waves (id, school_id, period_id, name, registration_start, registration_end, test_start, test_end, announcement_at, reregistration_end, created_at, updated_at)
    VALUES
    (
        v_wave1_id,
        v_school_id,
        v_period_id,
        'Gelombang 1',
        now() - interval '14 days',
        now() + interval '20 days',
        now() + interval '22 days',
        now() + interval '23 days',
        now() + interval '25 days',
        now() + interval '35 days',
        now(),
        now()
    )
    ON CONFLICT (id) DO UPDATE SET
        registration_start = EXCLUDED.registration_start,
        registration_end = EXCLUDED.registration_end,
        name = EXCLUDED.name;

    INSERT INTO admission_waves (id, school_id, period_id, name, registration_start, registration_end, test_start, test_end, announcement_at, reregistration_end, created_at, updated_at)
    VALUES
    (
        v_wave2_id,
        v_school_id,
        v_period_id,
        'Gelombang 2',
        now() + interval '21 days',
        now() + interval '55 days',
        now() + interval '57 days',
        now() + interval '58 days',
        now() + interval '60 days',
        now() + interval '70 days',
        now(),
        now()
    )
    ON CONFLICT (id) DO UPDATE SET
        registration_start = EXCLUDED.registration_start,
        registration_end = EXCLUDED.registration_end,
        name = EXCLUDED.name;

    -- 3. ADMISSION TRACKS
    INSERT INTO admission_tracks (id, school_id, period_id, code, name, quota, description, is_active, created_at, updated_at)
    VALUES
    (
        v_track_reg,
        v_school_id,
        v_period_id,
        'REG',
        'Jalur Reguler',
        120,
        'Jalur seleksi penerimaan reguler berbasis tes TPA dan wawancara kesiapan belajar.',
        true,
        now(),
        now()
    )
    ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name,
        quota = EXCLUDED.quota,
        description = EXCLUDED.description,
        is_active = true;

    INSERT INTO admission_tracks (id, school_id, period_id, code, name, quota, description, is_active, created_at, updated_at)
    VALUES
    (
        v_track_prs,
        v_school_id,
        v_period_id,
        'PRS',
        'Jalur Prestasi Akademik & Non-Akademik',
        40,
        'Jalur pendaftaran bagi siswa berprestasi peringkat kelas atau kejuaraan resmi minimal tingkat Kota/Kabupaten.',
        true,
        now(),
        now()
    )
    ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name,
        quota = EXCLUDED.quota,
        description = EXCLUDED.description,
        is_active = true;

    INSERT INTO admission_tracks (id, school_id, period_id, code, name, quota, description, is_active, created_at, updated_at)
    VALUES
    (
        v_track_tfz,
        v_school_id,
        v_period_id,
        'TFZ',
        'Jalur Beasiswa Tahfidz Al-Qur''an',
        20,
        'Jalur khusus hafalan Al-Qur''an minimal 3 Juz dengan beasiswa dan bebas biaya formulir.',
        true,
        now(),
        now()
    )
    ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name,
        quota = EXCLUDED.quota,
        description = EXCLUDED.description,
        is_active = true;

    INSERT INTO admission_tracks (id, school_id, period_id, code, name, quota, description, is_active, created_at, updated_at)
    VALUES
    (
        v_track_afr,
        v_school_id,
        v_period_id,
        'AFR',
        'Jalur Afirmasi & Kemitraan',
        30,
        'Jalur bantuan kemitraan pendidikan yayasan bagi keluarga prasejahtera dan mitra Teratai Putih.',
        true,
        now(),
        now()
    )
    ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name,
        quota = EXCLUDED.quota,
        description = EXCLUDED.description,
        is_active = true;

    -- 4. DOCUMENT REQUIREMENTS
    INSERT INTO document_requirements (id, school_id, period_id, code, name, description, required, max_bytes, allowed_mimes, version, created_at)
    VALUES
    (
        v_req_akta,
        v_school_id,
        v_period_id,
        'AKTA',
        'Akta Kelahiran',
        'Scan atau foto berwarna Akta Kelahiran resmi dari Disdukcapil (PDF/JPG/PNG, maks 5MB)',
        true,
        5242880,
        ARRAY['application/pdf','image/jpeg','image/png'],
        1,
        now()
    )
    ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

    INSERT INTO document_requirements (id, school_id, period_id, code, name, description, required, max_bytes, allowed_mimes, version, created_at)
    VALUES
    (
        v_req_kk,
        v_school_id,
        v_period_id,
        'KK',
        'Kartu Keluarga (KK)',
        'Scan Kartu Keluarga terbaru yang masih berlaku (barcode tertera jelas)',
        true,
        5242880,
        ARRAY['application/pdf','image/jpeg','image/png'],
        1,
        now()
    )
    ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

    INSERT INTO document_requirements (id, school_id, period_id, code, name, description, required, max_bytes, allowed_mimes, version, created_at)
    VALUES
    (
        v_req_foto,
        v_school_id,
        v_period_id,
        'FOTO',
        'Pas Foto Calon Siswa (3x4)',
        'Pas foto formal berseragam sekolah asal dengan latar belakang biru/merah',
        true,
        5242880,
        ARRAY['image/jpeg','image/png'],
        1,
        now()
    )
    ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

    INSERT INTO document_requirements (id, school_id, period_id, code, name, description, required, max_bytes, allowed_mimes, version, created_at)
    VALUES
    (
        v_req_rapor,
        v_school_id,
        v_period_id,
        'RAPOR',
        'Rapor Semester Terakhir',
        'Scan lembar nilai rapor 2 semester terakhir dengan legalisir sekolah asal',
        true,
        5242880,
        ARRAY['application/pdf','image/jpeg','image/png'],
        1,
        now()
    )
    ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

    INSERT INTO document_requirements (id, school_id, period_id, code, name, description, required, max_bytes, allowed_mimes, version, created_at)
    VALUES
    (
        v_req_piagam,
        v_school_id,
        v_period_id,
        'PIAGAM',
        'Sertifikat / Piagam Prestasi',
        'Scan piagam kejuaraan, sertifikat tahfidz, atau dokumen pendukung prestasi (opsional)',
        false,
        5242880,
        ARRAY['application/pdf','image/jpeg','image/png'],
        1,
        now()
    )
    ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

    -- 5. FEE SCHEMES
    INSERT INTO fee_schemes (id, school_id, period_id, track_id, name, revision, status, registration_fee_idr, reregistration_fee_idr, installment_count, created_at)
    VALUES
    (v_fee_form, v_school_id, v_period_id, v_track_reg, 'Biaya Formulir Reguler', 1, 'active', 250000, 0, 1, now()),
    (v_fee_smp, v_school_id, v_period_id, v_track_prs, 'Paket Masuk SMP Teratai Putih', 1, 'active', 150000, 7800000, 3, now()),
    (v_fee_sma, v_school_id, v_period_id, v_track_afr, 'Paket Masuk SMA MIPA Teratai Putih', 1, 'active', 100000, 8500000, 3, now()),
    (v_fee_sd, v_school_id, v_period_id, v_track_tfz, 'Paket Masuk SD Reguler & Tahfidz', 1, 'active', 0, 6500000, 2, now())
    ON CONFLICT (id) DO UPDATE SET status = 'active', registration_fee_idr = EXCLUDED.registration_fee_idr;

    -- 6. DISCOUNT RULES
    INSERT INTO discount_rules (id, school_id, period_id, name, kind, value, cap_idr, priority, stackable, status, created_at)
    VALUES
    (gen_random_uuid(), v_school_id, v_period_id, 'Early Bird Gelombang 1', 'fixed', 500000, NULL, 1, true, 'active', now()),
    (gen_random_uuid(), v_school_id, v_period_id, 'Diskon Saudara Kandung (Alumni)', 'percent', 10, 1000000, 2, true, 'active', now()),
    (gen_random_uuid(), v_school_id, v_period_id, 'Beasiswa Tahfidz Qur''an (3 Juz)', 'fixed', 1500000, NULL, 3, false, 'active', now())
    ON CONFLICT DO NOTHING;

    -- 7. SELECTION COMPONENTS
    INSERT INTO selection_components (id, school_id, period_id, code, name, source, required, max_score)
    VALUES
    (v_cmp_tpa, v_school_id, v_period_id, 'TPA', 'Tes Potensi Akademik', 'cbt', true, 10000),
    (v_cmp_tfz, v_school_id, v_period_id, 'TAHFIDZ', 'Ujian Baca & Hafalan Al-Qur''an', 'manual', true, 10000),
    (v_cmp_wwc, v_school_id, v_period_id, 'WAWANCARA', 'Wawancara Siswa & Orang Tua', 'manual', true, 10000)
    ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

    -- 8. GUARDIANS & CONTACTS
    INSERT INTO guardians (id, school_id, name, identity_user_id, status, created_at, updated_at)
    VALUES
    (v_g_nurul, v_school_id, 'Nurul Akbar Tanjung', '88880000-0000-0000-0000-000000000001', 'active', now() - interval '10 days', now()),
    (v_g_david, v_school_id, 'David Pambudi', '88880000-0000-0000-0000-000000000002', 'active', now() - interval '9 days', now()),
    (v_g_budi, v_school_id, 'Budi Santoso', '88880000-0000-0000-0000-000000000003', 'active', now() - interval '8 days', now()),
    (v_g_ratna, v_school_id, 'Ibu Ratna Sari', '88880000-0000-0000-0000-000000000004', 'active', now() - interval '7 days', now()),
    (v_g_hendra, v_school_id, 'Hendra Pratama', '88880000-0000-0000-0000-000000000005', 'active', now() - interval '6 days', now()),
    (v_g_siti, v_school_id, 'Siti Maryam', '88880000-0000-0000-0000-000000000006', 'active', now() - interval '5 days', now()),
    (v_g_syamsul, v_school_id, 'Syamsul Bahri', '88880000-0000-0000-0000-000000000007', 'active', now() - interval '4 days', now()),
    (v_g_dewi, v_school_id, 'Dewi Rahmawati', '88880000-0000-0000-0000-000000000008', 'active', now() - interval '3 days', now())
    ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, identity_user_id = EXCLUDED.identity_user_id, status = 'active';

    INSERT INTO guardian_contacts (id, school_id, guardian_id, kind, normalized_value, verified_at, created_at)
    VALUES
    (gen_random_uuid(), v_school_id, v_g_nurul, 'email', 'nurulakbartanjung@gmail.com', now() - interval '10 days', now() - interval '10 days'),
    (gen_random_uuid(), v_school_id, v_g_david, 'email', 'pambudidavid03@gmail.com', now() - interval '9 days', now() - interval '9 days'),
    (gen_random_uuid(), v_school_id, v_g_budi, 'email', 'budi.wali@gmail.com', now() - interval '8 days', now() - interval '8 days'),
    (gen_random_uuid(), v_school_id, v_g_ratna, 'email', 'ratna.sari@gmail.com', now() - interval '7 days', now() - interval '7 days'),
    (gen_random_uuid(), v_school_id, v_g_hendra, 'email', 'hendra.pratama@gmail.com', now() - interval '6 days', now() - interval '6 days'),
    (gen_random_uuid(), v_school_id, v_g_siti, 'email', 'siti.maryam@gmail.com', now() - interval '5 days', now() - interval '5 days'),
    (gen_random_uuid(), v_school_id, v_g_syamsul, 'email', 'syamsul.bahri@gmail.com', now() - interval '4 days', now() - interval '4 days'),
    (gen_random_uuid(), v_school_id, v_g_dewi, 'email', 'dewi.rahmawati@gmail.com', now() - interval '3 days', now() - interval '3 days')
    ON CONFLICT (school_id, kind, normalized_value) DO NOTHING;

    -- 9. APPLICANTS
    INSERT INTO applicants (id, school_id, guardian_id, full_name, nickname, nisn, nik, birth_date, birth_place, gender, previous_school, address, guardian_relationship, source, created_at, updated_at)
    VALUES
    (v_app_ahmad, v_school_id, v_g_nurul, 'Ahmad Rizky Pratama', 'Rizky', '0081234567', '3275011203080001', '2012-05-14', 'Bekasi', 'male', 'SDIT Al-Azhar 9 Bekasi', 'Jl. Kemang Pratama Raya No. 45, Bekasi', 'ayah', 'guardian', now() - interval '10 days', now()),
    (v_app_nadia, v_school_id, v_g_david, 'Nadia Putri Azzahra', 'Nadia', '0079876543', '3275014508070002', '2010-08-20', 'Jakarta', 'female', 'SMP Negeri 1 Bekasi', 'Jl. Boulevard Hijau Blok B3 No. 12, Harapan Indah', 'ayah', 'guardian', now() - interval '9 days', now()),
    (v_app_fajar, v_school_id, v_g_budi, 'Muhammad Fajar Ramadhan', 'Fajar', '0145678901', '3275012504140003', '2018-04-10', 'Bekasi', 'male', 'TK Islam Teratai Putih', 'Perumahan Galaxy Indah Blok A1 No. 8, Bekasi', 'ayah', 'guardian', now() - interval '8 days', now()),
    (v_app_aisyah, v_school_id, v_g_ratna, 'Siti Aisyah Humaira', 'Aisyah', '0091122334', '3275015809090004', '2012-09-18', 'Bandung', 'female', 'SDIT Nurul Fikri', 'Jl. Cikunir Raya No. 22, Jaka Mulya, Bekasi', 'ibu', 'guardian', now() - interval '7 days', now()),
    (v_app_farhan, v_school_id, v_g_hendra, 'Farhan Al-Ghifari', 'Farhan', '0067788990', '3275011707060005', '2009-07-02', 'Bekasi', 'male', 'SMP Teratai Putih Global', 'Jl. Villa Nusa Indah Blok V2 No. 15, Bojong Kulur', 'ayah', 'guardian', now() - interval '6 days', now()),
    (v_app_amanda, v_school_id, v_g_siti, 'Amanda Zahira Putri', 'Amanda', '0154321098', '3275016212150006', '2018-11-25', 'Bekasi', 'female', 'TK Aisyiyah Bustanul Athfal', 'Jl. Bintara Jaya No. 70, Bekasi Barat', 'ibu', 'guardian', now() - interval '5 days', now()),
    (v_app_reyhan, v_school_id, v_g_syamsul, 'Reyhan Putra Pratama', 'Reyhan', '0089988776', '3275011001080007', '2012-01-15', 'Jakarta', 'male', 'SDIT Thariq Bin Ziyad', 'Jl. Pondok Timur Indah Blok C No. 9, Bekasi', 'ayah', 'guardian', now() - interval '4 days', now()),
    (v_app_kayla, v_school_id, v_g_dewi, 'Kayla Putri Ramadhani', 'Kayla', '0076655443', '3275015504070008', '2010-04-12', 'Bekasi', 'female', 'SMP Islam Darussalam', 'Jl. Raya Pekayon No. 88, Bekasi Selatan', 'ibu', 'guardian', now() - interval '3 days', now())
    ON CONFLICT (id) DO UPDATE SET
        full_name = EXCLUDED.full_name,
        guardian_id = EXCLUDED.guardian_id,
        updated_at = now();

    -- Also link any other existing guardian records for the same emails
    UPDATE applicants SET guardian_id = (SELECT guardian_id FROM guardian_contacts WHERE normalized_value = 'nurulakbartanjung@gmail.com' LIMIT 1) WHERE id = v_app_ahmad;
    UPDATE applicants SET guardian_id = (SELECT guardian_id FROM guardian_contacts WHERE normalized_value = 'pambudidavid03@gmail.com' LIMIT 1) WHERE id = v_app_nadia;
    UPDATE applicants SET guardian_id = (SELECT guardian_id FROM guardian_contacts WHERE normalized_value = 'budi.wali@gmail.com' LIMIT 1) WHERE id = v_app_fajar;
    UPDATE applicants SET guardian_id = (SELECT guardian_id FROM guardian_contacts WHERE normalized_value = 'ratna.sari@gmail.com' LIMIT 1) WHERE id = v_app_aisyah;

    -- 10. APPLICATIONS
    INSERT INTO applications (id, school_id, applicant_id, period_id, wave_id, track_id, registration_number, status, version, submitted_at, created_at, updated_at)
    VALUES
    (v_appl_ahmad, v_school_id, v_app_ahmad, v_period_id, v_wave1_id, v_track_prs, 'PPDB-2026-SMP-001', 'accepted', 1, now() - interval '9 days', now() - interval '10 days', now()),
    (v_appl_nadia, v_school_id, v_app_nadia, v_period_id, v_wave1_id, v_track_reg, 'PPDB-2026-SMA-002', 'verified', 1, now() - interval '8 days', now() - interval '9 days', now()),
    (v_appl_fajar, v_school_id, v_app_fajar, v_period_id, v_wave1_id, v_track_reg, 'PPDB-2026-SD-003', 'reregistered', 1, now() - interval '7 days', now() - interval '8 days', now()),
    (v_appl_aisyah, v_school_id, v_app_aisyah, v_period_id, v_wave1_id, v_track_tfz, 'PPDB-2026-SMP-004', 'submitted', 1, now() - interval '6 days', now() - interval '7 days', now()),
    (v_appl_farhan, v_school_id, v_app_farhan, v_period_id, v_wave1_id, v_track_prs, 'PPDB-2026-SMA-005', 'verified', 1, now() - interval '5 days', now() - interval '6 days', now()),
    (v_appl_amanda, v_school_id, v_app_amanda, v_period_id, v_wave1_id, v_track_afr, 'PPDB-2026-SD-006', 'accepted', 1, now() - interval '4 days', now() - interval '5 days', now()),
    (v_appl_reyhan, v_school_id, v_app_reyhan, v_period_id, v_wave1_id, v_track_reg, 'PPDB-2026-SMP-007', 'draft', 1, NULL, now() - interval '4 days', now()),
    (v_appl_kayla, v_school_id, v_app_kayla, v_period_id, v_wave1_id, v_track_reg, 'PPDB-2026-SMA-008', 'submitted', 1, now() - interval '2 days', now() - interval '3 days', now())
    ON CONFLICT (id) DO UPDATE SET
        status = EXCLUDED.status,
        registration_number = EXCLUDED.registration_number,
        updated_at = now();

    -- 11. APPLICATION DOCUMENTS
    INSERT INTO application_documents (id, school_id, application_id, requirement_id, object_key, file_name, mime, size_bytes, sha256, scan_status, uploaded_by, created_at)
    VALUES
    (gen_random_uuid(), v_school_id, v_appl_ahmad, v_req_akta, 'docs/ahmad_akta.pdf', 'akta_kelahiran_ahmad.pdf', 'application/pdf', 1245000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'clean', 'guardian', now() - interval '9 days'),
    (gen_random_uuid(), v_school_id, v_appl_ahmad, v_req_kk, 'docs/ahmad_kk.pdf', 'kartu_keluarga_akbar.pdf', 'application/pdf', 980000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'clean', 'guardian', now() - interval '9 days'),
    (gen_random_uuid(), v_school_id, v_appl_ahmad, v_req_foto, 'docs/ahmad_foto.jpg', 'pas_foto_ahmad_3x4.jpg', 'image/jpeg', 420000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'clean', 'guardian', now() - interval '9 days'),
    (gen_random_uuid(), v_school_id, v_appl_ahmad, v_req_rapor, 'docs/ahmad_rapor.pdf', 'rapor_sd_ahmad_sm12.pdf', 'application/pdf', 2150000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'clean', 'guardian', now() - interval '9 days'),
    (gen_random_uuid(), v_school_id, v_appl_ahmad, v_req_piagam, 'docs/ahmad_piagam.pdf', 'piagam_olimpiade_matematika_juara1.pdf', 'application/pdf', 1540000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'clean', 'guardian', now() - interval '9 days'),

    (gen_random_uuid(), v_school_id, v_appl_nadia, v_req_akta, 'docs/nadia_akta.pdf', 'akta_kelahiran_nadia.pdf', 'application/pdf', 1150000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'clean', 'guardian', now() - interval '8 days'),
    (gen_random_uuid(), v_school_id, v_appl_nadia, v_req_kk, 'docs/nadia_kk.pdf', 'kartu_keluarga_david.pdf', 'application/pdf', 1020000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'clean', 'guardian', now() - interval '8 days'),
    (gen_random_uuid(), v_school_id, v_appl_nadia, v_req_foto, 'docs/nadia_foto.jpg', 'pas_foto_nadia.jpg', 'image/jpeg', 380000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'clean', 'guardian', now() - interval '8 days'),
    (gen_random_uuid(), v_school_id, v_appl_nadia, v_req_rapor, 'docs/nadia_rapor.pdf', 'rapor_smp_nadia.pdf', 'application/pdf', 2400000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'clean', 'guardian', now() - interval '8 days')
    ON CONFLICT (application_id, requirement_id) DO NOTHING;

    -- Document reviews
    INSERT INTO document_reviews (id, school_id, document_id, status, notes, reviewer_id, created_at)
    SELECT gen_random_uuid(), v_school_id, doc.id, 'accepted', 'Dokumen terverifikasi sah dan lengkap sesuai persyaratan.', 'admin-ppdb-01', now() - interval '7 days'
    FROM application_documents doc
    WHERE doc.application_id IN (v_appl_ahmad, v_appl_nadia)
    ON CONFLICT DO NOTHING;

    -- 12. CHARGES
    INSERT INTO charges (id, school_id, application_id, revision, fee_scheme_id, gross_idr, discount_idr, net_idr, items_snapshot, discount_snapshot, finalized_at)
    VALUES
    (
        gen_random_uuid(),
        v_school_id,
        v_appl_ahmad,
        1,
        v_fee_smp,
        150000,
        0,
        150000,
        '[{"code":"FORM-SMP","name":"Formulir Pendaftaran SMP","amount_idr":150000}]'::jsonb,
        '[]'::jsonb,
        now() - interval '9 days'
    ),
    (
        gen_random_uuid(),
        v_school_id,
        v_appl_nadia,
        1,
        v_fee_form,
        250000,
        0,
        250000,
        '[{"code":"FORM-REG","name":"Formulir Pendaftaran Reguler","amount_idr":250000}]'::jsonb,
        '[]'::jsonb,
        now() - interval '8 days'
    )
    ON CONFLICT (application_id, revision) DO NOTHING;

    -- 13. COMPONENT SCORES
    INSERT INTO component_scores (id, school_id, application_id, component_id, score, final, source_ref, entered_by, created_at, updated_at)
    VALUES
    (gen_random_uuid(), v_school_id, v_appl_ahmad, v_cmp_tpa, 8850, true, 'cbt-exam-smp-01', 'system-cbt', now() - interval '3 days', now()),
    (gen_random_uuid(), v_school_id, v_appl_ahmad, v_cmp_tfz, 9200, true, 'tahfidz-panel', 'ustadz-ahmad', now() - interval '3 days', now()),
    (gen_random_uuid(), v_school_id, v_appl_ahmad, v_cmp_wwc, 8500, true, 'interview-panel', 'panitia-01', now() - interval '3 days', now()),

    (gen_random_uuid(), v_school_id, v_appl_nadia, v_cmp_tpa, 8200, true, 'cbt-exam-sma-01', 'system-cbt', now() - interval '2 days', now()),
    (gen_random_uuid(), v_school_id, v_appl_nadia, v_cmp_wwc, 8400, true, 'interview-panel', 'panitia-02', now() - interval '2 days', now())
    ON CONFLICT (application_id, component_id) DO NOTHING;

    -- 14. DECISIONS
    INSERT INTO decisions (id, school_id, application_id, decision, override, reason, actor_id, version, created_at)
    VALUES
    (gen_random_uuid(), v_school_id, v_appl_ahmad, 'offered', false, NULL, 'panitia-ppdb', 1, now() - interval '2 days'),
    (gen_random_uuid(), v_school_id, v_appl_amanda, 'offered', false, NULL, 'panitia-ppdb', 1, now() - interval '2 days'),
    (gen_random_uuid(), v_school_id, v_appl_fajar, 'offered', false, NULL, 'panitia-ppdb', 1, now() - interval '2 days')
    ON CONFLICT (application_id, version) DO NOTHING;

    -- 15. CBT PARTICIPANTS
    INSERT INTO admission_cbt_participants (id, school_id, application_id, exam_id, participant_ref, status, score, scheduled_at, started_at, completed_at, created_at, updated_at)
    VALUES
    (gen_random_uuid(), v_school_id, v_appl_ahmad, 'exam-smp-01', 'PPDB-2026-SMP-001', 'completed', 8850, now() - interval '4 days', now() - interval '4 days', now() - interval '4 days' + interval '90 minutes', now() - interval '5 days', now()),
    (gen_random_uuid(), v_school_id, v_appl_nadia, 'exam-sma-01', 'PPDB-2026-SMA-002', 'available', NULL, now() + interval '1 days', NULL, NULL, now() - interval '4 days', now()),
    (gen_random_uuid(), v_school_id, v_appl_farhan, 'exam-sma-01', 'PPDB-2026-SMA-005', 'scheduled', NULL, now() + interval '2 days', NULL, NULL, now() - interval '3 days', now())
    ON CONFLICT (application_id) DO NOTHING;

END $$;
