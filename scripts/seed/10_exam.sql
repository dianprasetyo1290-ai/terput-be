-- 10_exam.sql
-- Seed exam types, question folders, questions, exams, classrooms, and tokens

DO $$
DECLARE
    v_school_id UUID := '11111111-1111-1111-1111-111111111111';
    v_academic_year_id UUID := '12121212-1212-1212-1212-121212121212';
    v_term_id UUID := 'c0000000-0000-0000-0000-000000000001';
    v_classroom_id UUID := 'c1000000-0000-0000-0000-000000000001'; -- VII A
    v_subject_id UUID := 'c2000000-0000-0000-0000-000000000001';   -- MTK
    v_admin_id UUID := '22222222-2222-2222-2222-222222222222';
    v_guru_id UUID := '77777777-7777-7777-7777-777777777777';
    v_student_id UUID := '44444444-4444-4444-4444-444444444444';   -- Budi Santoso
    v_exam_type_id UUID := 'b1000000-0000-0000-0000-000000000001';
    v_folder_id UUID := 'b2000000-0000-0000-0000-000000000001';
    v_q1_id UUID := 'b3000000-0000-0000-0000-000000000001';
    v_q2_id UUID := 'b3000000-0000-0000-0000-000000000002';
    v_exam_id UUID := 'b4000000-0000-0000-0000-000000000001';
BEGIN
    -- 1. Exam Types
    INSERT INTO exam_types (id, school_id, academic_year_id, name, mapping_rapor, created_by)
    VALUES
        (v_exam_type_id, v_school_id, v_academic_year_id, 'Sumatif Tengah Semester (STS)', 'sts', v_admin_id),
        ('b1000000-0000-0000-0000-000000000002', v_school_id, v_academic_year_id, 'Sumatif Akhir Semester (SAS)', 'sas', v_admin_id),
        ('b1000000-0000-0000-0000-000000000003', v_school_id, v_academic_year_id, 'Ulangan Harian Formatif', 'formatif', v_admin_id)
    ON CONFLICT (id) DO NOTHING;

    -- 2. Question Folder (Bank Soal)
    INSERT INTO question_folders (id, school_id, academic_year_id, name, description, created_by)
    VALUES
        (v_folder_id, v_school_id, v_academic_year_id, 'Bank Soal Matematika Kelas 7 Ganjil', 'Kumpulan soal aljabar, bilangan bulat, dan perbandingan.', v_admin_id)
    ON CONFLICT (id) DO NOTHING;

    -- 3. Questions
    INSERT INTO questions (
        id, school_id, academic_year_id, folder_id, created_by, question_type, content, poin, is_published
    ) VALUES
        (v_q1_id, v_school_id, v_academic_year_id, v_folder_id, v_guru_id, 'pilihan_ganda',
         'Hasil dari (-15) + 28 - (-7) adalah...', 50.00, true),
        (v_q2_id, v_school_id, v_academic_year_id, v_folder_id, v_guru_id, 'pilihan_ganda',
         'Suhu di dalam lemari es mula-mula -4°C. Pada saat listrik padam, suhu naik 3°C setiap 4 menit. Berapa suhu setelah 12 menit?', 50.00, true)
    ON CONFLICT (id) DO NOTHING;

    -- 4. Answer Options
    INSERT INTO answer_options (id, question_id, content, is_correct, order_index)
    VALUES
        ('b5000000-0000-0000-0000-000000000001', v_q1_id, '20', false, 0),
        ('b5000000-0000-0000-0000-000000000002', v_q1_id, '26', false, 1),
        ('b5000000-0000-0000-0000-000000000003', v_q1_id, '35', false, 2),
        ('b5000000-0000-0000-0000-000000000004', v_q1_id, '40', false, 3),
        ('b5000000-0000-0000-0000-000000000005', v_q1_id, '20', true, 4) -- Opsi e
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO answer_options (id, question_id, content, is_correct, order_index)
    VALUES
        ('b5000000-0000-0000-0000-000000000006', v_q2_id, '5°C', true, 0),
        ('b5000000-0000-0000-0000-000000000007', v_q2_id, '9°C', false, 1),
        ('b5000000-0000-0000-0000-000000000008', v_q2_id, '1°C', false, 2),
        ('b5000000-0000-0000-0000-000000000009', v_q2_id, '-1°C', false, 3),
        ('b5000000-0000-0000-0000-000000000010', v_q2_id, '13°C', false, 4)
    ON CONFLICT (id) DO NOTHING;

    -- 5. Exams
    INSERT INTO exams (
        id, school_id, academic_year_id, term_id, exam_type_id, title, description,
        classroom_id, subject_id, teacher_id, start_at, end_at, is_shuffled, is_published, published_at, created_by, folder_id, audience
    ) VALUES
        (v_exam_id, v_school_id, v_academic_year_id, v_term_id, v_exam_type_id, 'Penilaian Tengah Semester: Matematika 7A',
         'Ujian STS Matematika materi Bilangan Bulat dan Aljabar Dasar.',
         v_classroom_id, v_subject_id, v_guru_id, now() - interval '1 hour', now() + interval '30 days', false, true, now(), v_guru_id, v_folder_id, 'academic')
    ON CONFLICT (id) DO UPDATE SET
        is_published = true,
        end_at = now() + interval '30 days';

    -- 6. Exam Classrooms
    INSERT INTO exam_classrooms (exam_id, classroom_id)
    VALUES (v_exam_id, v_classroom_id)
    ON CONFLICT (exam_id, classroom_id) DO NOTHING;

    -- 7. Exam Questions
    INSERT INTO exam_questions (id, exam_id, question_id, order_index, points)
    VALUES
        ('b6000000-0000-0000-0000-000000000001', v_exam_id, v_q1_id, 0, 50.00),
        ('b6000000-0000-0000-0000-000000000002', v_exam_id, v_q2_id, 1, 50.00)
    ON CONFLICT (exam_id, question_id) DO NOTHING;

    -- 8. Exam Token for Budi Santoso
    INSERT INTO exam_tokens (
        id, school_id, academic_year_id, user_id, code, valid_from, valid_until, created_by
    ) VALUES
        ('b7000000-0000-0000-0000-000000000001', v_school_id, v_academic_year_id, v_student_id, 'TPGS26', CURRENT_DATE, CURRENT_DATE + interval '30 days', v_admin_id)
    ON CONFLICT (school_id, user_id) DO UPDATE SET
        code = EXCLUDED.code,
        valid_until = EXCLUDED.valid_until;

END $$;
