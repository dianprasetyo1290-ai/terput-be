-- 30_academic_features.sql  (DB: teratai_academic)
-- Data demo fitur Akademik: semester, kelas, jadwal, materi/tugas/pengumpulan,
-- riwayat kelas, permintaan pindah/keluar siswa (variasi status), pengaturan.
--
-- Aturan: hanya MENAMBAH. Semua baris memakai UUID tetap berawalan 5e30 dan
-- ON CONFLICT DO NOTHING, jadi aman dijalankan berulang dan tidak pernah
-- mengubah/menghapus baris yang sudah ada. Satu file = satu transaksi.
--
-- Catatan tahun akademik: tabel academic_years (dan kolom is_current) ada di
-- DB teratai_identity, bukan di sini. File ini tidak menyentuhnya; tahun
-- ajaran aktif tetap '12121212-...' (is_current di identity).
--
-- Pengguna (sama dengan seed fitur lain): admin 2222, wali 3333,
-- siswa Budi 4444, kepala sekolah 5555, guru 7777, wali kelas 8888.

BEGIN;

-- ---------------------------------------------------------------------------
-- 0. Kerangka demo yang dirujuk seed lama (08/09/10/11/12) dan seed 31-34.
--    Di server baris-baris ini sudah ada -> DO NOTHING; di DB baru ia dibuat
--    supaya FK dan rujukan lintas layanan konsisten.
-- ---------------------------------------------------------------------------
INSERT INTO terms (id, school_id, academic_year_id, name, "order", start_date, end_date)
VALUES ('c0000000-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111',
        '12121212-1212-1212-1212-121212121212', 'Semester 1 (Ganjil)', 1, '2026-07-13', '2026-12-19')
ON CONFLICT DO NOTHING;

-- Semester 2 hanya ditambahkan bila tahun ajaran itu belum punya urutan 2.
INSERT INTO terms (id, school_id, academic_year_id, name, "order", start_date, end_date)
VALUES ('5e300100-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111',
        '12121212-1212-1212-1212-121212121212', 'Semester 2 (Genap)', 2, '2027-01-04', '2027-06-19')
ON CONFLICT DO NOTHING;

INSERT INTO subjects (id, school_id, name, description, kelompok) VALUES
  ('c2000000-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Matematika', 'Matematika Fase D Kurikulum Merdeka', 'umum'),
  ('c2000000-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'Bahasa Indonesia', 'Bahasa dan Sastra Indonesia', 'umum')
ON CONFLICT DO NOTHING;

INSERT INTO classrooms (id, school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, capacity)
VALUES ('c1000000-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111',
        '12121212-1212-1212-1212-121212121212', 'Kelas VII A', '7', '88888888-8888-8888-8888-888888888888', 32)
ON CONFLICT DO NOTHING;

INSERT INTO classroom_subjects (id, school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, hours_per_week)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
       'c1000000-0000-0000-0000-000000000001', v.subject_id::uuid, '77777777-7777-7777-7777-777777777777', v.jp
FROM (VALUES ('c3000000-0000-0000-0000-000000000001', 'c2000000-0000-0000-0000-000000000001', 5),
             ('c3000000-0000-0000-0000-000000000002', 'c2000000-0000-0000-0000-000000000002', 4)) v(id, subject_id, jp)
WHERE EXISTS (SELECT 1 FROM subjects s WHERE s.id = v.subject_id::uuid)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 1. Kelas tambahan (tujuan/asal pindah kelas)
-- ---------------------------------------------------------------------------
INSERT INTO classrooms (id, school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, capacity) VALUES
  ('5e300200-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', 'Kelas VII B', '7', '77777777-7777-7777-7777-777777777777', 32),
  ('5e300200-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', 'Kelas VIII A', '8', '55555555-5555-5555-5555-555555555555', 30)
ON CONFLICT DO NOTHING;

-- Mapel diampu di VII B supaya matriks pelajaran kelas itu tidak kosong.
INSERT INTO classroom_subjects (id, school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, hours_per_week)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
       '5e300200-0000-0000-0000-000000000001', v.subject_id::uuid, '77777777-7777-7777-7777-777777777777', v.jp
FROM (VALUES ('5e300300-0000-0000-0000-000000000001', 'c2000000-0000-0000-0000-000000000001', 5),
             ('5e300300-0000-0000-0000-000000000002', 'c2000000-0000-0000-0000-000000000002', 4)) v(id, subject_id, jp)
WHERE EXISTS (SELECT 1 FROM subjects s WHERE s.id = v.subject_id::uuid)
  AND EXISTS (SELECT 1 FROM classrooms c WHERE c.id = '5e300200-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2. Jadwal pelajaran VII A (melengkapi Senin jam 1-2 & Selasa jam 1 dari 08)
-- ---------------------------------------------------------------------------
INSERT INTO schedule_periods (id, school_id, academic_year_id, classroom_id, day_of_week, period_no, start_time, end_time)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
       'c1000000-0000-0000-0000-000000000001', v.d, v.p, v.s, v.e
FROM (VALUES ('5e300400-0000-0000-0000-000000000001', 2, 2, '09:10', '10:30'),
             ('5e300400-0000-0000-0000-000000000002', 3, 1, '07:30', '08:50'),
             ('5e300400-0000-0000-0000-000000000003', 3, 2, '09:10', '10:30'),
             ('5e300400-0000-0000-0000-000000000004', 4, 1, '07:30', '08:50'),
             ('5e300400-0000-0000-0000-000000000005', 5, 1, '07:30', '08:50')) v(id, d, p, s, e)
WHERE EXISTS (SELECT 1 FROM classrooms c WHERE c.id = 'c1000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

INSERT INTO schedules (id, school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
       'c1000000-0000-0000-0000-000000000001', v.period_id::uuid, v.cs_id::uuid
FROM (VALUES ('5e300500-0000-0000-0000-000000000001', '5e300400-0000-0000-0000-000000000001', 'c3000000-0000-0000-0000-000000000002'),
             ('5e300500-0000-0000-0000-000000000002', '5e300400-0000-0000-0000-000000000002', 'c3000000-0000-0000-0000-000000000001'),
             ('5e300500-0000-0000-0000-000000000003', '5e300400-0000-0000-0000-000000000003', 'c3000000-0000-0000-0000-000000000002'),
             ('5e300500-0000-0000-0000-000000000004', '5e300400-0000-0000-0000-000000000004', 'c3000000-0000-0000-0000-000000000001'),
             ('5e300500-0000-0000-0000-000000000005', '5e300400-0000-0000-0000-000000000005', 'c3000000-0000-0000-0000-000000000002')) v(id, period_id, cs_id)
WHERE EXISTS (SELECT 1 FROM schedule_periods p WHERE p.id = v.period_id::uuid)
  AND EXISTS (SELECT 1 FROM classroom_subjects cs WHERE cs.id = v.cs_id::uuid)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Siswa: rombel & mapel diikuti Budi
-- ---------------------------------------------------------------------------
INSERT INTO class_enrollments (id, school_id, academic_year_id, classroom_id, student_user_id, status, enrolled_at)
SELECT '5e300600-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
       'c1000000-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444', 'active', '2026-07-20 08:00+07'
WHERE EXISTS (SELECT 1 FROM classrooms c WHERE c.id = 'c1000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

INSERT INTO student_subject_enrollments (id, school_id, academic_year_id, student_user_id, classroom_subject_id, is_enrolled)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
       '44444444-4444-4444-4444-444444444444', v.cs_id::uuid, true
FROM (VALUES ('5e300700-0000-0000-0000-000000000001', 'c3000000-0000-0000-0000-000000000001'),
             ('5e300700-0000-0000-0000-000000000002', 'c3000000-0000-0000-0000-000000000002')) v(id, cs_id)
WHERE EXISTS (SELECT 1 FROM classroom_subjects cs WHERE cs.id = v.cs_id::uuid)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 4. Pindah & keluar siswa (Menu 78) — variasi status. Tidak ada status
--    pending_approval/approved karena status itu wajib punya approval_id di
--    Approval Center (core); semua di sini sudah final.
-- ---------------------------------------------------------------------------
INSERT INTO student_status_requests (id, school_id, student_user_id, kind, from_classroom_id, to_classroom_id,
    effective_date, reason, attachments, finance_snapshot, refund_amount_idr, status, applied_at, requested_by,
    idempotency_key, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', v.kind,
       v.from_c::uuid, v.to_c::uuid, v.eff::date, v.reason, '[]'::jsonb, v.fin::jsonb, v.refund, v.status,
       v.applied::timestamptz, v.req::uuid, v.idem, v.created::timestamptz, v.created::timestamptz
FROM (VALUES
  ('5e300800-0000-0000-0000-000000000001', 'class_transfer', '5e300200-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000001',
   '2026-07-20', 'Penyesuaian rombel: Budi dipindah ke VII A agar satu kelas dengan program pengayaan matematika.',
   NULL, NULL::bigint, 'applied', '2026-07-20 07:00+07', '22222222-2222-2222-2222-222222222222', 'seed30-ssr-1', '2026-07-17 10:00+07'),
  ('5e300800-0000-0000-0000-000000000002', 'withdrawal', 'c1000000-0000-0000-0000-000000000001', NULL,
   '2026-09-01', 'Orang tua berencana pindah domisili ke Surabaya (dibatalkan karena keluarga tidak jadi pindah).',
   '{"student_user_id":"44444444-4444-4444-4444-444444444444","school_id":"11111111-1111-1111-1111-111111111111","total_outstanding_idr":0,"open_bills_idr":0,"unmatured_bills_idr":0,"deposit_idr":0,"refundable_idr":0}',
   0::bigint, 'cancelled', NULL, '33333333-3333-3333-3333-333333333333', 'seed30-ssr-2', '2026-08-18 09:30+07'),
  ('5e300800-0000-0000-0000-000000000003', 'class_transfer', 'c1000000-0000-0000-0000-000000000001', '5e300200-0000-0000-0000-000000000002',
   '2026-09-14', 'Permintaan pindah ke kelas VIII A (akselerasi) — ditolak, belum memenuhi syarat kenaikan.',
   NULL, NULL::bigint, 'rejected', NULL, '88888888-8888-8888-8888-888888888888', 'seed30-ssr-3', '2026-09-08 13:15+07'),
  ('5e300800-0000-0000-0000-000000000004', 'withdrawal', 'c1000000-0000-0000-0000-000000000001', NULL,
   '2026-10-01', 'Pengajuan keluar karena alasan biaya; ditolak setelah sekolah menawarkan keringanan SPP.',
   '{"student_user_id":"44444444-4444-4444-4444-444444444444","school_id":"11111111-1111-1111-1111-111111111111","total_outstanding_idr":850000,"open_bills_idr":850000,"unmatured_bills_idr":0,"deposit_idr":0,"refundable_idr":0}',
   0::bigint, 'rejected', NULL, '33333333-3333-3333-3333-333333333333', 'seed30-ssr-4', '2026-09-24 08:45+07')
) v(id, kind, from_c, to_c, eff, reason, fin, refund, status, applied, req, idem, created)
WHERE EXISTS (SELECT 1 FROM classrooms c WHERE c.id = 'c1000000-0000-0000-0000-000000000001')
  AND EXISTS (SELECT 1 FROM classrooms c WHERE c.id = '5e300200-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- Riwayat rombel Budi (cocok dengan permintaan yang sudah diterapkan).
INSERT INTO class_enrollment_history (id, school_id, academic_year_id, student_user_id, from_classroom_id, to_classroom_id,
    from_status, to_status, action, reason, request_id, actor_user_id, created_at)
VALUES
  ('5e300900-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '44444444-4444-4444-4444-444444444444', NULL, '5e300200-0000-0000-0000-000000000001', NULL, 'active', 'admission',
   'Diterima melalui PPDB 2026 jalur reguler', NULL, '22222222-2222-2222-2222-222222222222', '2026-07-13 08:00+07'),
  ('5e300900-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '44444444-4444-4444-4444-444444444444', '5e300200-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000001', 'active', 'active', 'transfer',
   'Penyesuaian rombel: Budi dipindah ke VII A agar satu kelas dengan program pengayaan matematika.',
   '5e300800-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222', '2026-07-20 07:00+07')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 5. Materi, tugas, dan pengumpulan (LMS)
-- ---------------------------------------------------------------------------
INSERT INTO learning_materials (id, school_id, academic_year_id, classroom_subject_id, teacher_user_id, title, description, link_url, status, published_at, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', v.cs::uuid,
       '77777777-7777-7777-7777-777777777777', v.title, v.descr, v.link, v.status, v.pub::timestamptz, v.created::timestamptz
FROM (VALUES
  ('5e300a00-0000-0000-0000-000000000001', 'c3000000-0000-0000-0000-000000000001', 'Modul 2: Aljabar dan Bentuk Variabel',
   'Pengenalan variabel, koefisien, konstanta, dan suku sejenis disertai contoh soal.', 'https://teratai.sch.id/files/modul-mtk-2.pdf', 'published', '2026-08-24 07:00+07', '2026-08-21 15:00+07'),
  ('5e300a00-0000-0000-0000-000000000002', 'c3000000-0000-0000-0000-000000000001', 'Modul 3: Persamaan Linear Satu Variabel (draf)',
   'Draf materi PLSV untuk pertemuan November, masih ditinjau.', NULL, 'draft', NULL, '2026-10-10 16:00+07'),
  ('5e300a00-0000-0000-0000-000000000003', 'c3000000-0000-0000-0000-000000000002', 'Modul 2: Teks Narasi dan Unsur Intrinsik',
   'Membaca cerpen "Robohnya Surau Kami" dan mengidentifikasi tokoh, alur, latar.', 'https://teratai.sch.id/files/modul-indo-2.pdf', 'published', '2026-09-07 07:00+07', '2026-09-04 14:00+07')
) v(id, cs, title, descr, link, status, pub, created)
WHERE EXISTS (SELECT 1 FROM classroom_subjects cs WHERE cs.id = v.cs::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO learning_assignments (id, school_id, academic_year_id, classroom_subject_id, teacher_user_id, title, instructions, due_at, max_score, status, published_at, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', v.cs::uuid,
       '77777777-7777-7777-7777-777777777777', v.title, v.instr, v.due::timestamptz, 100, v.status, v.pub::timestamptz, v.created::timestamptz
FROM (VALUES
  ('5e300b00-0000-0000-0000-000000000001', 'c3000000-0000-0000-0000-000000000001', 'PR Aljabar: Menyederhanakan Bentuk Aljabar',
   'Kerjakan 10 soal pada LKPD halaman 41, unggah foto jawaban yang jelas.', '2026-09-04 23:59+07', 'published', '2026-08-28 07:00+07', '2026-08-27 15:00+07'),
  ('5e300b00-0000-0000-0000-000000000002', 'c3000000-0000-0000-0000-000000000002', 'Menulis Ringkasan Cerpen',
   'Tulis ringkasan cerpen maksimal 300 kata beserta unsur intrinsiknya.', '2026-09-18 23:59+07', 'published', '2026-09-11 07:00+07', '2026-09-10 14:00+07'),
  ('5e300b00-0000-0000-0000-000000000003', 'c3000000-0000-0000-0000-000000000001', 'Proyek Statistika: Data Tinggi Badan Kelas',
   'Kumpulkan data tinggi badan teman sekelas lalu buat diagram batang dan hitung rata-rata.', '2026-10-09 23:59+07', 'published', '2026-10-01 07:00+07', '2026-09-30 15:00+07'),
  ('5e300b00-0000-0000-0000-000000000004', 'c3000000-0000-0000-0000-000000000002', 'Puisi Bertema Lingkungan (draf)',
   'Draf tugas menulis puisi 3 bait bertema menjaga lingkungan sekolah.', '2026-11-06 23:59+07', 'draft', NULL, '2026-10-12 10:00+07')
) v(id, cs, title, instr, due, status, pub, created)
WHERE EXISTS (SELECT 1 FROM classroom_subjects cs WHERE cs.id = v.cs::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO learning_submissions (id, school_id, assignment_id, student_user_id, answer_text, submitted_at, is_late, status,
    resubmit_count, score, feedback, graded_at, graded_by)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.asg::uuid, '44444444-4444-4444-4444-444444444444', v.ans,
       v.sub::timestamptz, v.late, v.status, v.rc, v.score, v.fb, v.graded::timestamptz, v.grader::uuid
FROM (VALUES
  ('5e300c00-0000-0000-0000-000000000001', '5e300b00-0000-0000-0000-000000000001', 'Jawaban terlampir: 1) 5x+3y, 2) 2a-7, ... 10) 4p^2-p.',
   '2026-09-03 19:20+07', false, 'graded', 0, 88.00::numeric, 'Langkah penyederhanaan sudah runtut. Perhatikan tanda negatif di soal 7.', '2026-09-07 10:00+07', '77777777-7777-7777-7777-777777777777'),
  ('5e300c00-0000-0000-0000-000000000002', '5e300b00-0000-0000-0000-000000000002', 'Ringkasan cerpen "Robohnya Surau Kami": Kakek penjaga surau ...',
   '2026-09-19 06:40+07', true, 'resubmit_requested', 1, NULL::numeric, 'Ringkasan belum memuat unsur latar dan amanat. Silakan perbaiki.', NULL, NULL),
  ('5e300c00-0000-0000-0000-000000000003', '5e300b00-0000-0000-0000-000000000003', 'Diagram batang dan perhitungan rata-rata (158,4 cm) terlampir.',
   '2026-10-08 20:05+07', false, 'submitted', 0, NULL::numeric, NULL, NULL, NULL)
) v(id, asg, ans, sub, late, status, rc, score, fb, graded, grader)
WHERE EXISTS (SELECT 1 FROM learning_assignments a WHERE a.id = v.asg::uuid)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 6. Pengaturan akademik sekolah (nilai = default aplikasi)
-- ---------------------------------------------------------------------------
INSERT INTO academic_settings (school_id, post_exit_bill_policy, parent_grace_days)
VALUES ('11111111-1111-1111-1111-111111111111', 'void', 30)
ON CONFLICT DO NOTHING;

COMMIT;
