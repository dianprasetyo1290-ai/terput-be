-- 11_evaluation.sql
-- Seed RPM (Lesson Plans), Prota, Prosem, and Teacher Evaluation Periods

DO $$
DECLARE
    v_school_id UUID := '11111111-1111-1111-1111-111111111111';
    v_academic_year_id UUID := '12121212-1212-1212-1212-121212121212';
    v_guru_id UUID := '77777777-7777-7777-7777-777777777777';
    v_kepsek_id UUID := '55555555-5555-5555-5555-555555555555';
    v_rpm_id UUID := 'd1000000-0000-0000-0000-000000000001';
    v_prota_id UUID := 'd2000000-0000-0000-0000-000000000001';
    v_prosem_id UUID := 'd3000000-0000-0000-0000-000000000001';
    v_eval_id UUID := 'd4000000-0000-0000-0000-000000000001';
BEGIN
    -- 1. Lesson Plans (RPM - Rencana Pelaksanaan Mingguan)
    INSERT INTO lesson_plans (
        id, school_id, academic_year_id, guru_user_id, title, section_a, section_b, section_c, status
    ) VALUES (
        v_rpm_id, v_school_id, v_academic_year_id, v_guru_id,
        'RPM Matematika Kelas VII: Bilangan Bulat dan Pecahan',
        jsonb_build_object(
            'mata_pelajaran', 'Matematika',
            'kelas', 'VII A',
            'alokasi_waktu', '4 JP x 40 menit',
            'tujuan_pembelajaran', 'Siswa mampu memahami operasi penjumlahan dan perkalian bilangan bulat.'
        ),
        jsonb_build_object(
            'model_pembelajaran', 'Problem Based Learning (PBL)',
            'media_ajar', 'Papan tulis interaktif, Geogebra, LKPD',
            'sumber_belajar', 'Buku Siswa Kemendikbudristek 2024'
        ),
        jsonb_build_object(
            'kegiatan_pendahuluan', 'Apersepsi dan ice breaking 10 menit',
            'kegiatan_inti', 'Diskusi kelompok memecahkan permasalahan suhu udara',
            'kegiatan_penutup', 'Refleksi dan kesimpulan bersama siswa'
        ),
        'published'
    ) ON CONFLICT (id) DO UPDATE SET
        title = EXCLUDED.title,
        status = EXCLUDED.status;

    -- 2. Prota (Program Tahunan)
    INSERT INTO prota (
        id, school_id, academic_year_id, guru_user_id, mata_pelajaran, kelas
    ) VALUES (
        v_prota_id, v_school_id, v_academic_year_id, v_guru_id, 'Matematika', 'VII'
    ) ON CONFLICT (id) DO NOTHING;

    INSERT INTO prota_items (
        id, prota_id, no, semester, elemen_kode_bab, kompetensi_dasar_tp, alokasi_waktu_jp, keterangan_target_asesmen
    ) VALUES
        ('d2100000-0000-0000-0000-000000000001', v_prota_id, 1, '1', 'BAB 1', 'Bilangan Bulat dan Bilangan Rasional', 20, 'Formatif 1 & Sumatif STS'),
        ('d2100000-0000-0000-0000-000000000002', v_prota_id, 2, '1', 'BAB 2', 'Aljabar dan Persamaan Linier Satu Variabel', 24, 'Formatif 2 & Sumatif SAS'),
        ('d2100000-0000-0000-0000-000000000003', v_prota_id, 3, '2', 'BAB 3', 'Rasio dan Perbandingan Senilai/Berbalik Nilai', 20, 'Formatif 3 & Sumatif SAT')
    ON CONFLICT (id) DO NOTHING;

    -- 3. Prosem (Program Semester)
    INSERT INTO prosem (
        id, school_id, academic_year_id, guru_user_id, mata_pelajaran, kelas
    ) VALUES (
        v_prosem_id, v_school_id, v_academic_year_id, v_guru_id, 'Matematika', 'VII'
    ) ON CONFLICT (id) DO NOTHING;

    INSERT INTO prosem_items (
        id, prosem_id, no, materi_pokok_aktivitas, alokasi_jp, bulan_target, minggu_ke, estimasi_tanggal, tipe_aktivitas_lms, status_input_guru
    ) VALUES
        ('d3100000-0000-0000-0000-000000000001', v_prosem_id, 1, 'Konsep Bilangan Bulat Positif dan Negatif', 4, 'Juli', 3, '2026-07-20', 'Tatap Muka & Diskusi', 'Selesai'),
        ('d3100000-0000-0000-0000-000000000002', v_prosem_id, 2, 'Operasi Hitung Penjumlahan dan Pengurangan', 4, 'Juli', 4, '2026-07-27', 'Praktikum Mandiri', 'Selesai'),
        ('d3100000-0000-0000-0000-000000000003', v_prosem_id, 3, 'Perkalian dan Pembagian Bilangan Bulat', 4, 'Agustus', 1, '2026-08-03', 'Tatap Muka', 'Selesai')
    ON CONFLICT (id) DO NOTHING;

    -- 4. Teacher Evaluation Period
    INSERT INTO teacher_evaluation_periods (
        id, school_id, academic_year_id, title, semester, publish_start_date, publish_end_date, published,
        headmaster_user_id, target_teacher_ids, student_classroom_ids, lead_assessor_name
    ) VALUES (
        v_eval_id, v_school_id, v_academic_year_id, 'Supervisi Akademik & Kinerja Guru TP 2026/2027 Ganjil',
        'Semester 1', CURRENT_DATE - INTERVAL '10 days', CURRENT_DATE + INTERVAL '60 days', true,
        v_kepsek_id, jsonb_build_array(v_guru_id), jsonb_build_array('c1000000-0000-0000-0000-000000000001'), 'Dr. H. Ahmad Dahlan, M.Pd'
    ) ON CONFLICT (id) DO UPDATE SET
        published = true,
        publish_end_date = CURRENT_DATE + INTERVAL '60 days';

END $$;
