-- 15_notification.sql
-- Seed in-app notifications, chat threads, and messages

DO $$
DECLARE
    v_school_id UUID := '11111111-1111-1111-1111-111111111111';
    v_admin_id UUID := '22222222-2222-2222-2222-222222222222';
    v_guru_id UUID := '77777777-7777-7777-7777-777777777777';
    v_wali_id UUID := '33333333-3333-3333-3333-333333333333';
    v_student_id UUID := '44444444-4444-4444-4444-444444444444';
    v_thread_id UUID := '81000000-0000-0000-0000-000000000001';
BEGIN
    -- 1. In-App Notifications for Admin
    INSERT INTO notifications (id, school_id, user_id, type, title, body, data, created_at)
    VALUES
        ('82000000-0000-0000-0000-000000000001', v_school_id, v_admin_id, 'system.welcome',
         'Selamat Datang di Teratai Putih Global School',
         'Seluruh fitur sistem informasi manajemen terpadu telah aktif dan siap digunakan.',
         '{"action": "/dashboard"}', now() - interval '2 days'),
        ('82000000-0000-0000-0000-000000000002', v_school_id, v_admin_id, 'billing.payment.received',
         'Pembayaran SPP Diterima',
         'Pembayaran SPP Bulan Juli 2026 atas nama Budi Santoso (Kelas VII A) telah diverifikasi lunas.',
         '{"student_id": "44444444-4444-4444-4444-444444444444"}', now() - interval '1 day'),
        ('82000000-0000-0000-0000-000000000003', v_school_id, v_admin_id, 'admission.new_application',
         'Pendaftaran Siswa Baru Masuk',
         'Calon siswa Ahmad Fauzi berhasil mendaftar melalui jalur Unggulan PPDB 2026/2027.',
         '{"menu": "ppdb"}', now() - interval '3 hours')
    ON CONFLICT (id) DO NOTHING;

    -- 2. In-App Notifications for Wali & Siswa
    INSERT INTO notifications (id, school_id, user_id, type, title, body, data, created_at)
    VALUES
        ('82000000-0000-0000-0000-000000000004', v_school_id, v_wali_id, 'academic.schedule.published',
         'Jadwal Penilaian Tengah Semester Telah Dirilis',
         'PTS Semester 1 akan dimulai pekan depan. Silakan periksa kartu peserta dan token ujian ananda.',
         '{"action": "/siswa/kegiatan"}', now() - interval '1 day'),
        ('82000000-0000-0000-0000-000000000005', v_school_id, v_student_id, 'exam.assigned',
         'Tugas Matematika Baru Tersedia',
         'Guru Teratai telah menambahkan Latihan Mandiri 1: Soal Cerita Bilangan Bulat.',
         '{"action": "/ujian"}', now() - interval '4 hours')
    ON CONFLICT (id) DO NOTHING;

    -- 3. Chat Thread (Konsultasi Guru & Wali Murid)
    INSERT INTO chat_threads (id, school_id, subject, created_by, created_at)
    VALUES (v_thread_id, v_school_id, 'Konsultasi Belajar Ananda Budi Santoso', v_wali_id, now() - interval '1 day')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO chat_thread_participants (id, thread_id, user_id, role)
    VALUES
        ('83000000-0000-0000-0000-000000000001', v_thread_id, v_wali_id, 'owner'),
        ('83000000-0000-0000-0000-000000000002', v_thread_id, v_guru_id, 'member')
    ON CONFLICT (thread_id, user_id) DO NOTHING;

    INSERT INTO chat_messages (id, thread_id, sender_user_id, body, created_at)
    VALUES
        ('84000000-0000-0000-0000-000000000001', v_thread_id, v_wali_id,
         'Selamat pagi Bapak/Ibu Guru, saya ingin menanyakan perkembangan tugas projek matematika ananda Budi.', now() - interval '1 day'),
        ('84000000-0000-0000-0000-000000000002', v_thread_id, v_guru_id,
         'Selamat pagi Bapak Wali. Perkembangan Budi sangat baik, pemahaman konsep aljabarnya sangat menonjol di kelas.', now() - interval '20 hours')
    ON CONFLICT (id) DO NOTHING;

END $$;
