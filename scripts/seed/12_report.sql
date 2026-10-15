-- 12_report.sql
-- Seed report cards, subject grades, formative grades, and templates

DO $$
DECLARE
    v_school_id UUID := '11111111-1111-1111-1111-111111111111';
    v_academic_year_id UUID := '12121212-1212-1212-1212-121212121212';
    v_term_id UUID := 'c0000000-0000-0000-0000-000000000001';
    v_classroom_id UUID := 'c1000000-0000-0000-0000-000000000001'; -- VII A
    v_student_id UUID := '44444444-4444-4444-4444-444444444444';   -- Budi Santoso
    v_report_id UUID := 'f1000000-0000-0000-0000-000000000001';
    v_sub_mtk UUID := 'c2000000-0000-0000-0000-000000000001';
    v_sub_indo UUID := 'c2000000-0000-0000-0000-000000000002';
    v_cs_mtk UUID := 'c3000000-0000-0000-0000-000000000001';
    v_asst_type_id UUID := 'f4000000-0000-0000-0000-000000000001';
BEGIN
    -- 1. Assessment Types
    INSERT INTO report_assessment_types (id, school_id, code, name, bucket, weight_percent, sort_order)
    VALUES
        (v_asst_type_id, v_school_id, 'FORMATIF-1', 'Penilaian Harian Formatif 1', 'formatif', 100.00, 1),
        ('f4000000-0000-0000-0000-000000000002', v_school_id, 'STS-1', 'Sumatif Tengah Semester Ganjil', 'sts', 100.00, 2),
        ('f4000000-0000-0000-0000-000000000003', v_school_id, 'SAS-1', 'Sumatif Akhir Semester Ganjil', 'sas', 100.00, 3)
    ON CONFLICT (school_id, code) DO NOTHING;

    -- 2. Report Card (Buku Rapor Budi Santoso)
    INSERT INTO report_cards (
        id, school_id, academic_year_id, term_id, classroom_id, student_id,
        status, report_type, period_label, class_teacher_note, attendance_hadir, attendance_sakit, attendance_izin, attendance_tanpa_keterangan,
        student_name, student_nisn, classroom_name, term_name
    ) VALUES (
        v_report_id, v_school_id, v_academic_year_id, v_term_id, v_classroom_id, v_student_id,
        'draft', 'semester', '', 'Ananda Budi memiliki pemahaman yang sangat baik dalam logika matematika dan nalar kritis. Pertahankan prestasinya!',
        58, 2, 1, 0,
        'Budi Santoso', '0087654321', 'Kelas VII A', 'Semester 1 (Ganjil)'
    ) ON CONFLICT (student_id, classroom_id, term_id, report_type, period_label) DO UPDATE SET
        class_teacher_note = EXCLUDED.class_teacher_note,
        attendance_hadir = EXCLUDED.attendance_hadir;

    -- 3. Report Card Subjects (Nilai Rapor per Mapel)
    INSERT INTO report_card_subjects (
        id, report_card_id, school_id, subject_id, subject_name, subject_code,
        computed_score, override_score, grade_label, notes, sts_score, sas_score, formatif_score, praktik_score
    ) VALUES
        ('f2000000-0000-0000-0000-000000000001', v_report_id, v_school_id, v_sub_mtk, 'Matematika', 'MTK-7',
         88.50, 89.00, 'A', 'Sangat mahir dalam pemecahan masalah aljabar dan bilangan bulat.', 88.00, 90.00, 87.50, 92.00),
        ('f2000000-0000-0000-0000-000000000002', v_report_id, v_school_id, v_sub_indo, 'Bahasa Indonesia', 'BIN-7',
         85.00, 85.00, 'B+', 'Mampu menyusun teks deskriptif dengan ejaan dan tanda baca yang tepat.', 84.00, 86.00, 85.00, 88.00)
    ON CONFLICT (report_card_id, subject_id) DO UPDATE SET
        computed_score = EXCLUDED.computed_score,
        grade_label = EXCLUDED.grade_label,
        notes = EXCLUDED.notes;

    -- 4. Formative Grades
    INSERT INTO formative_grades (
        id, school_id, term_id, classroom_subject_id, student_id, assessment_type_id, score, created_by
    ) VALUES
        ('f3000000-0000-0000-0000-000000000001', v_school_id, v_term_id, v_cs_mtk, v_student_id, v_asst_type_id, 92.00, '77777777-7777-7777-7777-777777777777')
    ON CONFLICT (term_id, classroom_subject_id, student_id, assessment_type_id) DO UPDATE SET
        score = EXCLUDED.score;

END $$;
