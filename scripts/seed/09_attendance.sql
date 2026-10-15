-- 09_attendance.sql
-- Seed daily sessions, student attendance, daily submissions, and leave requests

DO $$
DECLARE
    v_school_id UUID := '11111111-1111-1111-1111-111111111111';
    v_academic_year_id UUID := '12121212-1212-1212-1212-121212121212';
    v_term_id UUID := 'c0000000-0000-0000-0000-000000000001';
    v_classroom_id UUID := 'c1000000-0000-0000-0000-000000000001'; -- VII A
    v_student_id UUID := '44444444-4444-4444-4444-444444444444';   -- Budi Santoso
    v_wali_id UUID := '33333333-3333-3333-3333-333333333333';      -- Wali Santoso
    v_guru_id UUID := '77777777-7777-7777-7777-777777777777';      -- Guru Teratai
    v_schedule_id UUID := 'e2000000-0000-0000-0000-000000000001';
    v_subject_id UUID := 'c2000000-0000-0000-0000-000000000001';   -- MTK
    v_sess1 UUID := 'a1000000-0000-0000-0000-000000000001';
    v_sess2 UUID := 'a1000000-0000-0000-0000-000000000002';
    v_sess3 UUID := 'a1000000-0000-0000-0000-000000000003';
BEGIN
    -- 1. Class Sessions (Agenda Pembelajaran Harian)
    INSERT INTO class_sessions (
        id, school_id, academic_year_id, term_id, schedule_id, classroom_id, subject_id, guru_id,
        session_date, checked_in_at, slot_start_time, slot_end_time, pertemuan, pembahasan, materi_pertemuan
    ) VALUES
        (v_sess1, v_school_id, v_academic_year_id, v_term_id, v_schedule_id, v_classroom_id, v_subject_id, v_guru_id,
         CURRENT_DATE - INTERVAL '2 days', (CURRENT_DATE - INTERVAL '2 days') + TIME '07:25:00', '07:30:00', '08:50:00',
         '1', 'Pengenalan Bilangan Bulat', 'Konsep bilangan positif dan negatif pada garis bilangan'),
        (v_sess2, v_school_id, v_academic_year_id, v_term_id, v_schedule_id, v_classroom_id, v_subject_id, v_guru_id,
         CURRENT_DATE - INTERVAL '1 day', (CURRENT_DATE - INTERVAL '1 day') + TIME '07:28:00', '07:30:00', '08:50:00',
         '2', 'Operasi Penjumlahan & Pengurangan', 'Sifat-sifat asosiatif dan distributif'),
        (v_sess3, v_school_id, v_academic_year_id, v_term_id, v_schedule_id, v_classroom_id, v_subject_id, v_guru_id,
         CURRENT_DATE, CURRENT_TIMESTAMP, '07:30:00', '08:50:00',
         '3', 'Latihan Soal Cerita Terapan', 'Penyelesaian soal kontekstual kehidupan sehari-hari')
    ON CONFLICT (schedule_id, session_date) DO NOTHING;

    -- 2. Student Attendances (Budi Santoso hadir)
    INSERT INTO student_attendances (
        id, class_session_id, school_id, academic_year_id, term_id, classroom_id, student_id, session_date, status, catatan, marked_by
    ) VALUES
        ('a2000000-0000-0000-0000-000000000001', v_sess1, v_school_id, v_academic_year_id, v_term_id, v_classroom_id, v_student_id, CURRENT_DATE - INTERVAL '2 days', 'hadir', 'Tepat waktu dan aktif', v_guru_id),
        ('a2000000-0000-0000-0000-000000000002', v_sess2, v_school_id, v_academic_year_id, v_term_id, v_classroom_id, v_student_id, CURRENT_DATE - INTERVAL '1 day', 'hadir', 'Mengerjakan tugas dengan baik', v_guru_id),
        ('a2000000-0000-0000-0000-000000000003', v_sess3, v_school_id, v_academic_year_id, v_term_id, v_classroom_id, v_student_id, CURRENT_DATE, 'hadir', 'Hadir tepat waktu', v_guru_id)
    ON CONFLICT (classroom_id, session_date, student_id) DO NOTHING;

    -- 3. Daily Attendance Submissions
    INSERT INTO daily_attendance_submissions (
        classroom_id, session_date, school_id, status, catatan, submitted_by, submitted_at, updated_by, updated_at
    ) VALUES
        (v_classroom_id, CURRENT_DATE - INTERVAL '2 days', v_school_id, 'submitted', 'Presensi lengkap', v_guru_id, (CURRENT_DATE - INTERVAL '2 days') + TIME '12:00:00', v_guru_id, now()),
        (v_classroom_id, CURRENT_DATE - INTERVAL '1 day', v_school_id, 'submitted', 'Presensi lengkap', v_guru_id, (CURRENT_DATE - INTERVAL '1 day') + TIME '12:00:00', v_guru_id, now()),
        (v_classroom_id, CURRENT_DATE, v_school_id, 'submitted', 'Presensi hari ini', v_guru_id, now(), v_guru_id, now())
    ON CONFLICT (classroom_id, session_date) DO NOTHING;

    -- 4. Student Leave Requests
    INSERT INTO leave_requests (
        id, school_id, academic_year_id, classroom_id, student_id, requested_by, type, start_date, end_date,
        reason, status, reviewed_by, reviewed_at, review_note, source
    ) VALUES
        ('a3000000-0000-0000-0000-000000000001', v_school_id, v_academic_year_id, v_classroom_id, v_student_id, v_wali_id, 'izin',
         CURRENT_DATE + INTERVAL '5 days', CURRENT_DATE + INTERVAL '5 days',
         'Izin menghadiri acara keluarga di luar kota', 'approved', v_guru_id, now(), 'Disetujui. Tetap pelajari materi bab 2.', 'parent')
    ON CONFLICT (id) DO NOTHING;

END $$;
