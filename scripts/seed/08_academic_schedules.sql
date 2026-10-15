-- 08_academic_schedules.sql
-- Academic schedules, periods, materials, and assignments

DO $$
DECLARE
    v_school_id UUID := '11111111-1111-1111-1111-111111111111';
    v_academic_year_id UUID := '12121212-1212-1212-1212-121212121212';
    v_classroom_id UUID := 'c1000000-0000-0000-0000-000000000001'; -- VII A
    v_guru_id UUID := '77777777-7777-7777-7777-777777777777';
    v_period1_id UUID := 'e1000000-0000-0000-0000-000000000001';
    v_period2_id UUID := 'e1000000-0000-0000-0000-000000000002';
    v_period3_id UUID := 'e1000000-0000-0000-0000-000000000003';
    v_cs_mtk UUID := 'c3000000-0000-0000-0000-000000000001';
    v_cs_indo UUID := 'c3000000-0000-0000-0000-000000000002';
BEGIN
    -- 1. Schedule Periods for Classroom VII A (Monday - Friday)
    INSERT INTO schedule_periods (id, school_id, academic_year_id, classroom_id, day_of_week, period_no, start_time, end_time)
    VALUES
        (v_period1_id, v_school_id, v_academic_year_id, v_classroom_id, 1, 1, '07:30', '08:50'),
        (v_period2_id, v_school_id, v_academic_year_id, v_classroom_id, 1, 2, '09:10', '10:30'),
        (v_period3_id, v_school_id, v_academic_year_id, v_classroom_id, 2, 1, '07:30', '08:50')
    ON CONFLICT (classroom_id, day_of_week, period_no) DO NOTHING;

    -- 2. Schedules
    INSERT INTO schedules (id, school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id)
    VALUES
        ('e2000000-0000-0000-0000-000000000001', v_school_id, v_academic_year_id, v_classroom_id, v_period1_id, v_cs_mtk),
        ('e2000000-0000-0000-0000-000000000002', v_school_id, v_academic_year_id, v_classroom_id, v_period2_id, v_cs_indo)
    ON CONFLICT (schedule_period_id, classroom_subject_id) DO NOTHING;

    -- 3. Learning Materials
    INSERT INTO learning_materials (id, school_id, academic_year_id, classroom_subject_id, teacher_user_id, title, description, link_url, status)
    VALUES
        ('e3000000-0000-0000-0000-000000000001', v_school_id, v_academic_year_id, v_cs_mtk, v_guru_id, 'Modul 1: Bilangan Bulat dan Pecahan', 'Materi konsep dasar operasi bilangan bulat positif dan negatif beserta contoh penerapan.', 'https://teratai.sch.id/files/modul-mtk-1.pdf', 'published'),
        ('e3000000-0000-0000-0000-000000000002', v_school_id, v_academic_year_id, v_cs_indo, v_guru_id, 'Modul 1: Teks Deskripsi Lingkungan Sekitar', 'Panduan menyusun teks deskripsi objek sekolah dan alam dengan kaidah bahasa yang baik.', 'https://teratai.sch.id/files/modul-indo-1.pdf', 'published')
    ON CONFLICT (id) DO NOTHING;

    -- 4. Learning Assignments
    INSERT INTO learning_assignments (id, school_id, academic_year_id, classroom_subject_id, teacher_user_id, title, instructions, due_at, max_score, status)
    VALUES
        ('e4000000-0000-0000-0000-000000000001', v_school_id, v_academic_year_id, v_cs_mtk, v_guru_id, 'Latihan Mandiri 1: Soal Cerita Bilangan Bulat', 'Kerjakan soal latihan halaman 24 di buku tulis lalu unggah foto jawaban.', now() + interval '7 days', 100, 'published'),
        ('e4000000-0000-0000-0000-000000000002', v_school_id, v_academic_year_id, v_cs_indo, v_guru_id, 'Tugas Menulis: Deskripsi Taman Sekolah', 'Tuliskan esai 200 kata mendeskripsikan taman sekolah Teratai Putih.', now() + interval '5 days', 100, 'published')
    ON CONFLICT (id) DO NOTHING;

END $$;
