-- 32_exam_features.sql  (DB: teratai_exam)
-- Data demo fitur Ujian CBT: bank soal (PG, benar/salah, esai + kunci esai,
-- folder tersembunyi, soal draf), ujian dengan jadwal lampau/berjalan/akan
-- datang, peserta siswa & pendaftar PPDB, pengerjaan (selesai/aktif/belum
-- mulai/blokir), jawaban, riwayat ujian susulan (sitting) dan nilai ACC.
--
-- Hanya MENAMBAH: UUID tetap berawalan 5e32 + ON CONFLICT DO NOTHING; baris
-- anak lewat SELECT ... WHERE EXISTS induknya. Satu transaksi.
-- participant_access_tokens sengaja kosong (hash token akses pendaftar,
-- hanya diterbitkan oleh layanan saat login peserta).
-- Rujukan: tipe ujian b1000000-...-001/002/003 (seed 10), kelas VII A
-- c1000000-...-001, mapel B. Indonesia c2000000-...-002, semester c0000000-...-001,
-- periode PPDB aaaaaaaa-...-001 (seed admission).

BEGIN;

-- 1. Folder bank soal
INSERT INTO question_folders (id, school_id, academic_year_id, name, description, created_by, is_hidden) VALUES
  ('5e320100-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'Bank Soal Bahasa Indonesia Kelas 7 Ganjil', 'Teks narasi, teks deskripsi, dan puisi rakyat.', '77777777-7777-7777-7777-777777777777', false),
  ('5e320100-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'Tes Potensi Akademik PPDB', 'Soal seleksi calon siswa baru (penalaran verbal & numerik).', '22222222-2222-2222-2222-222222222222', false),
  ('5e320100-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'Arsip Soal Remedial 2025', 'Arsip lama, disembunyikan dari daftar guru.', '77777777-7777-7777-7777-777777777777', true)
ON CONFLICT DO NOTHING;

-- 2. Soal
INSERT INTO questions (id, school_id, academic_year_id, folder_id, created_by, question_type, content, no_kd, konten_kd,
    indikator_pencapaian, level_kognitif, poin, is_published)
VALUES
  ('5e320200-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '5e320100-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777', 'pilihan_ganda',
   'Bagian cerita yang berisi pengenalan tokoh dan latar disebut ...', '7.1', 'Teks narasi',
   'Menentukan struktur teks narasi', 'C1', 25, true),
  ('5e320200-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '5e320100-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777', 'pilihan_ganda',
   'Amanat yang paling tepat dari cerita "Si Kancil dan Buaya" adalah ...', '7.1', 'Teks narasi',
   'Menyimpulkan amanat cerita', 'C2', 25, true),
  ('5e320200-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '5e320100-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777', 'benar_salah',
   'Teks deskripsi bertujuan menggambarkan objek sehingga pembaca seolah-olah melihat sendiri.', '7.2', 'Teks deskripsi',
   'Menjelaskan tujuan teks deskripsi', 'C1', 20, true),
  ('5e320200-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '5e320100-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777', 'essay',
   'Tuliskan tiga unsur intrinsik cerpen beserta contohnya dari cerpen yang kamu baca.', '7.1', 'Teks narasi',
   'Mengidentifikasi unsur intrinsik', 'C3', 30, true),
  ('5e320200-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '5e320100-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777', 'pilihan_ganda',
   'Ciri kebahasaan puisi rakyat (pantun) yang benar adalah ... (draf, belum ditinjau)', '7.3', 'Puisi rakyat',
   'Mengidentifikasi ciri pantun', 'C1', 10, false),
  ('5e320200-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '5e320100-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222', 'pilihan_ganda',
   'Jika 3 buku harganya Rp36.000, maka harga 5 buku adalah ...', NULL, 'Penalaran numerik',
   'Menyelesaikan perbandingan senilai', 'C3', 50, true),
  ('5e320200-0000-0000-0000-000000000007', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '5e320100-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222', 'pilihan_ganda',
   'SEPATU : KAKI = TOPI : ...', NULL, 'Penalaran verbal', 'Analogi kata', 'C2', 50, true),
  ('5e320200-0000-0000-0000-000000000008', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   '5e320100-0000-0000-0000-000000000003', '77777777-7777-7777-7777-777777777777', 'benar_salah',
   'Kata baku dari "apotik" adalah "apotek".', '7.2', 'Ejaan', 'Menentukan kata baku', 'C1', 10, true)
ON CONFLICT DO NOTHING;

INSERT INTO answer_options (id, question_id, content, is_correct, order_index)
SELECT v.id::uuid, v.q::uuid, v.content, v.ok, v.ord
FROM (VALUES
  ('5e320300-0000-0000-0000-000000000001', '5e320200-0000-0000-0000-000000000001', 'Orientasi', true, 0),
  ('5e320300-0000-0000-0000-000000000002', '5e320200-0000-0000-0000-000000000001', 'Komplikasi', false, 1),
  ('5e320300-0000-0000-0000-000000000003', '5e320200-0000-0000-0000-000000000001', 'Resolusi', false, 2),
  ('5e320300-0000-0000-0000-000000000004', '5e320200-0000-0000-0000-000000000001', 'Koda', false, 3),
  ('5e320300-0000-0000-0000-000000000005', '5e320200-0000-0000-0000-000000000002', 'Kecerdikan harus digunakan untuk menolong, bukan menipu', true, 0),
  ('5e320300-0000-0000-0000-000000000006', '5e320200-0000-0000-0000-000000000002', 'Buaya adalah hewan yang kuat', false, 1),
  ('5e320300-0000-0000-0000-000000000007', '5e320200-0000-0000-0000-000000000002', 'Sungai berbahaya untuk diseberangi', false, 2),
  ('5e320300-0000-0000-0000-000000000008', '5e320200-0000-0000-0000-000000000002', 'Kancil selalu menang', false, 3),
  ('5e320300-0000-0000-0000-000000000009', '5e320200-0000-0000-0000-000000000003', 'Benar', true, 0),
  ('5e320300-0000-0000-0000-000000000010', '5e320200-0000-0000-0000-000000000003', 'Salah', false, 1),
  ('5e320300-0000-0000-0000-000000000011', '5e320200-0000-0000-0000-000000000005', 'Bersajak a-b-a-b', true, 0),
  ('5e320300-0000-0000-0000-000000000012', '5e320200-0000-0000-0000-000000000005', 'Terdiri atas 2 baris', false, 1),
  ('5e320300-0000-0000-0000-000000000013', '5e320200-0000-0000-0000-000000000006', 'Rp55.000', false, 0),
  ('5e320300-0000-0000-0000-000000000014', '5e320200-0000-0000-0000-000000000006', 'Rp60.000', true, 1),
  ('5e320300-0000-0000-0000-000000000015', '5e320200-0000-0000-0000-000000000006', 'Rp65.000', false, 2),
  ('5e320300-0000-0000-0000-000000000016', '5e320200-0000-0000-0000-000000000006', 'Rp72.000', false, 3),
  ('5e320300-0000-0000-0000-000000000017', '5e320200-0000-0000-0000-000000000007', 'Rambut', false, 0),
  ('5e320300-0000-0000-0000-000000000018', '5e320200-0000-0000-0000-000000000007', 'Kepala', true, 1),
  ('5e320300-0000-0000-0000-000000000019', '5e320200-0000-0000-0000-000000000007', 'Tangan', false, 2),
  ('5e320300-0000-0000-0000-000000000020', '5e320200-0000-0000-0000-000000000007', 'Leher', false, 3),
  ('5e320300-0000-0000-0000-000000000021', '5e320200-0000-0000-0000-000000000008', 'Benar', true, 0),
  ('5e320300-0000-0000-0000-000000000022', '5e320200-0000-0000-0000-000000000008', 'Salah', false, 1)
) v(id, q, content, ok, ord)
WHERE EXISTS (SELECT 1 FROM questions q WHERE q.id = v.q::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO essay_answer_options (id, question_id, answer_text)
SELECT v.id::uuid, v.q::uuid, v.t
FROM (VALUES
  ('5e320400-0000-0000-0000-000000000001', '5e320200-0000-0000-0000-000000000004', 'tokoh dan penokohan'),
  ('5e320400-0000-0000-0000-000000000002', '5e320200-0000-0000-0000-000000000004', 'alur'),
  ('5e320400-0000-0000-0000-000000000003', '5e320200-0000-0000-0000-000000000004', 'latar'),
  ('5e320400-0000-0000-0000-000000000004', '5e320200-0000-0000-0000-000000000004', 'amanat')
) v(id, q, t)
WHERE EXISTS (SELECT 1 FROM questions q WHERE q.id = v.q::uuid)
ON CONFLICT DO NOTHING;

-- 3. Ujian akademik (selesai / sedang berlangsung / terjadwal-draf)
INSERT INTO exams (id, school_id, academic_year_id, term_id, exam_type_id, title, description, classroom_id, subject_id, teacher_id,
    start_at, end_at, is_shuffled, is_published, published_at, created_by, folder_id, audience)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', 'c0000000-0000-0000-0000-000000000001',
       v.type_id::uuid, v.title, v.descr, 'c1000000-0000-0000-0000-000000000001', 'c2000000-0000-0000-0000-000000000002',
       '77777777-7777-7777-7777-777777777777', v.s::timestamptz, v.e::timestamptz, v.shuffle, v.pub, v.pub_at::timestamptz,
       '77777777-7777-7777-7777-777777777777', '5e320100-0000-0000-0000-000000000001', 'academic'
FROM (VALUES
  ('5e320500-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000003', 'Ulangan Harian: Teks Narasi 7A',
   'Formatif bab teks narasi dan deskripsi.', '2026-09-29 07:30+07', '2026-09-30 09:00+07', false, true, '2026-09-25 10:00+07'),
  ('5e320500-0000-0000-0000-000000000002', 'b1000000-0000-0000-0000-000000000001', 'STS Bahasa Indonesia 7A',
   'Sumatif tengah semester ganjil, materi teks narasi s.d. teks deskripsi.', '2026-10-13 07:30+07', '2026-10-20 15:00+07', true, true, '2026-10-09 09:00+07'),
  ('5e320500-0000-0000-0000-000000000003', 'b1000000-0000-0000-0000-000000000002', 'SAS Bahasa Indonesia 7A',
   'Sumatif akhir semester ganjil (draf, belum dipublikasikan).', '2026-12-01 07:30+07', '2026-12-01 09:30+07', true, false, NULL)
) v(id, type_id, title, descr, s, e, shuffle, pub, pub_at)
WHERE EXISTS (SELECT 1 FROM exam_types t WHERE t.id = v.type_id::uuid)
ON CONFLICT DO NOTHING;

-- 4. Ujian seleksi PPDB (audiens pendaftar)
INSERT INTO exams (id, school_id, title, description, start_at, end_at, is_shuffled, is_published, published_at, created_by,
    folder_id, audience, admission_context)
VALUES ('5e320500-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111',
        'Tes Potensi Akademik PPDB 2027 Gelombang 1', 'Tes seleksi calon siswa baru jalur reguler.',
        '2026-10-10 08:00+07', '2026-10-10 10:00+07', true, true, '2026-10-05 09:00+07',
        '22222222-2222-2222-2222-222222222222', '5e320100-0000-0000-0000-000000000002', 'admission',
        '{"external_reference":"seed32-ppdb-tpa-gel1","period_id":"aaaaaaaa-0000-0000-0000-000000000001","wave_id":"aaaaaaaa-0000-0000-0000-000000000002"}')
ON CONFLICT DO NOTHING;

INSERT INTO exam_classrooms (exam_id, classroom_id)
SELECT e.id, 'c1000000-0000-0000-0000-000000000001' FROM exams e
WHERE e.id IN ('5e320500-0000-0000-0000-000000000001', '5e320500-0000-0000-0000-000000000002', '5e320500-0000-0000-0000-000000000003')
ON CONFLICT DO NOTHING;

INSERT INTO exam_questions (id, exam_id, question_id, order_index, points)
SELECT v.id::uuid, v.ex::uuid, v.q::uuid, v.ord, v.pts
FROM (VALUES
  ('5e320600-0000-0000-0000-000000000001', '5e320500-0000-0000-0000-000000000001', '5e320200-0000-0000-0000-000000000001', 0, 25),
  ('5e320600-0000-0000-0000-000000000002', '5e320500-0000-0000-0000-000000000001', '5e320200-0000-0000-0000-000000000002', 1, 25),
  ('5e320600-0000-0000-0000-000000000003', '5e320500-0000-0000-0000-000000000001', '5e320200-0000-0000-0000-000000000003', 2, 20),
  ('5e320600-0000-0000-0000-000000000004', '5e320500-0000-0000-0000-000000000001', '5e320200-0000-0000-0000-000000000004', 3, 30),
  ('5e320600-0000-0000-0000-000000000005', '5e320500-0000-0000-0000-000000000002', '5e320200-0000-0000-0000-000000000002', 0, 30),
  ('5e320600-0000-0000-0000-000000000006', '5e320500-0000-0000-0000-000000000002', '5e320200-0000-0000-0000-000000000003', 1, 30),
  ('5e320600-0000-0000-0000-000000000007', '5e320500-0000-0000-0000-000000000002', '5e320200-0000-0000-0000-000000000004', 2, 40),
  ('5e320600-0000-0000-0000-000000000008', '5e320500-0000-0000-0000-000000000003', '5e320200-0000-0000-0000-000000000001', 0, 50),
  ('5e320600-0000-0000-0000-000000000009', '5e320500-0000-0000-0000-000000000003', '5e320200-0000-0000-0000-000000000004', 1, 50),
  ('5e320600-0000-0000-0000-000000000010', '5e320500-0000-0000-0000-000000000004', '5e320200-0000-0000-0000-000000000006', 0, 50),
  ('5e320600-0000-0000-0000-000000000011', '5e320500-0000-0000-0000-000000000004', '5e320200-0000-0000-0000-000000000007', 1, 50)
) v(id, ex, q, ord, pts)
WHERE EXISTS (SELECT 1 FROM exams e WHERE e.id = v.ex::uuid)
  AND EXISTS (SELECT 1 FROM questions q WHERE q.id = v.q::uuid)
ON CONFLICT DO NOTHING;

-- 5. Peserta: Budi (siswa) dan tiga pendaftar PPDB
INSERT INTO exam_participants (id, school_id, exam_id, participant_type, participant_id, display_name, external_reference)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.ex::uuid, v.ptype, v.pid::uuid, v.name, v.ref
FROM (VALUES
  ('5e320700-0000-0000-0000-000000000001', '5e320500-0000-0000-0000-000000000001', 'student', '44444444-4444-4444-4444-444444444444', 'Budi Santoso', NULL),
  ('5e320700-0000-0000-0000-000000000002', '5e320500-0000-0000-0000-000000000002', 'student', '44444444-4444-4444-4444-444444444444', 'Budi Santoso', NULL),
  ('5e320700-0000-0000-0000-000000000003', '5e320500-0000-0000-0000-000000000004', 'applicant', '5e32a000-0000-0000-0000-000000000001', 'Aisyah Putri Ramadhani', 'PPDB27-REG-0001'),
  ('5e320700-0000-0000-0000-000000000004', '5e320500-0000-0000-0000-000000000004', 'applicant', '5e32a000-0000-0000-0000-000000000002', 'Rizky Pratama Wibowo', 'PPDB27-REG-0002'),
  ('5e320700-0000-0000-0000-000000000005', '5e320500-0000-0000-0000-000000000004', 'applicant', '5e32a000-0000-0000-0000-000000000003', 'Nadia Salsabila', 'PPDB27-REG-0003')
) v(id, ex, ptype, pid, name, ref)
WHERE EXISTS (SELECT 1 FROM exams e WHERE e.id = v.ex::uuid)
ON CONFLICT DO NOTHING;

-- 6. Pengerjaan ujian
INSERT INTO exam_attempts (id, school_id, academic_year_id, exam_id, user_id, classroom_id, token_id, status, question_order,
    current_position, focus_lost_count, started_at, submitted_at, blocked_at, blocked_reason, blocked_by, score, correct_count,
    graded_at, final_score, final_acc_at, final_acc_by, participant_type, participant_id)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.ay::uuid, v.ex::uuid, v.uid::uuid, v.cls::uuid, NULL, v.status,
       v.qorder::jsonb, v.pos, v.focus, v.started::timestamptz, v.submitted::timestamptz, v.blocked::timestamptz, v.breason,
       v.bby::uuid, v.score, v.correct, v.graded::timestamptz, v.final, v.acc_at::timestamptz, v.acc_by::uuid, v.ptype, v.pid::uuid
FROM (VALUES
  -- Budi, UH Teks Narasi: selesai, dinilai, nilai di-ACC guru (pengerjaan ke-2 setelah reset)
  ('5e320800-0000-0000-0000-000000000001', '12121212-1212-1212-1212-121212121212', '5e320500-0000-0000-0000-000000000001',
   '44444444-4444-4444-4444-444444444444', 'c1000000-0000-0000-0000-000000000001', 'selesai',
   '["5e320600-0000-0000-0000-000000000001","5e320600-0000-0000-0000-000000000002","5e320600-0000-0000-0000-000000000003","5e320600-0000-0000-0000-000000000004"]',
   4, 1, '2026-09-30 07:35+07', '2026-09-30 08:40+07', NULL, NULL, NULL, 69.00::numeric, 2,
   '2026-10-01 10:00+07', 69::float8, '2026-10-01 10:05+07', '77777777-7777-7777-7777-777777777777', 'student', '44444444-4444-4444-4444-444444444444'),
  -- Budi, STS: sedang mengerjakan
  ('5e320800-0000-0000-0000-000000000002', '12121212-1212-1212-1212-121212121212', '5e320500-0000-0000-0000-000000000002',
   '44444444-4444-4444-4444-444444444444', 'c1000000-0000-0000-0000-000000000001', 'aktif',
   '["5e320600-0000-0000-0000-000000000006","5e320600-0000-0000-0000-000000000005","5e320600-0000-0000-0000-000000000007"]',
   1, 0, '2026-10-15 07:40+07', NULL, NULL, NULL, NULL, NULL::numeric, NULL,
   NULL, NULL::float8, NULL, NULL, 'student', '44444444-4444-4444-4444-444444444444'),
  -- Pendaftar 1: selesai
  ('5e320800-0000-0000-0000-000000000003', NULL, '5e320500-0000-0000-0000-000000000004', NULL, NULL, 'selesai',
   '["5e320600-0000-0000-0000-000000000011","5e320600-0000-0000-0000-000000000010"]',
   2, 0, '2026-10-10 08:03+07', '2026-10-10 08:41+07', NULL, NULL, NULL, 100.00::numeric, 2,
   '2026-10-10 08:41+07', NULL::float8, NULL, NULL, 'applicant', '5e32a000-0000-0000-0000-000000000001'),
  -- Pendaftar 2: tidak hadir (belum mulai)
  ('5e320800-0000-0000-0000-000000000004', NULL, '5e320500-0000-0000-0000-000000000004', NULL, NULL, 'belum_mulai',
   NULL, 0, 0, NULL, NULL, NULL, NULL, NULL, NULL::numeric, NULL,
   NULL, NULL::float8, NULL, NULL, 'applicant', '5e32a000-0000-0000-0000-000000000002'),
  -- Pendaftar 3: diblokir pengawas karena berpindah tab berulang kali
  ('5e320800-0000-0000-0000-000000000005', NULL, '5e320500-0000-0000-0000-000000000004', NULL, NULL, 'blokir',
   '["5e320600-0000-0000-0000-000000000010","5e320600-0000-0000-0000-000000000011"]',
   1, 4, '2026-10-10 08:05+07', NULL, '2026-10-10 08:22+07', 'Berpindah tab/jendela 4 kali', '22222222-2222-2222-2222-222222222222',
   NULL::numeric, NULL, NULL, NULL::float8, NULL, NULL, 'applicant', '5e32a000-0000-0000-0000-000000000003')
) v(id, ay, ex, uid, cls, status, qorder, pos, focus, started, submitted, blocked, breason, bby, score, correct,
    graded, final, acc_at, acc_by, ptype, pid)
WHERE EXISTS (SELECT 1 FROM exams e WHERE e.id = v.ex::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO exam_attempt_answers (id, attempt_id, exam_question_id, selected_option_id, essay_answer, is_correct, points_awarded,
    manually_graded, answered_at)
SELECT v.id::uuid, v.att::uuid, v.eq::uuid, v.opt::uuid, v.essay, v.ok, v.pts, v.manual, v.at::timestamptz
FROM (VALUES
  ('5e320900-0000-0000-0000-000000000001', '5e320800-0000-0000-0000-000000000001', '5e320600-0000-0000-0000-000000000001', '5e320300-0000-0000-0000-000000000001', NULL, true, 25.00::numeric, false, '2026-09-30 07:41+07'),
  ('5e320900-0000-0000-0000-000000000002', '5e320800-0000-0000-0000-000000000001', '5e320600-0000-0000-0000-000000000002', '5e320300-0000-0000-0000-000000000006', NULL, false, 0.00::numeric, false, '2026-09-30 07:50+07'),
  ('5e320900-0000-0000-0000-000000000003', '5e320800-0000-0000-0000-000000000001', '5e320600-0000-0000-0000-000000000003', '5e320300-0000-0000-0000-000000000009', NULL, true, 20.00::numeric, false, '2026-09-30 07:55+07'),
  ('5e320900-0000-0000-0000-000000000004', '5e320800-0000-0000-0000-000000000001', '5e320600-0000-0000-0000-000000000004', NULL,
   'Tokoh: Kakek penjaga surau. Latar: surau di kampung. Alur: maju mundur.', NULL, 24.00::numeric, true, '2026-09-30 08:35+07'),
  ('5e320900-0000-0000-0000-000000000005', '5e320800-0000-0000-0000-000000000002', '5e320600-0000-0000-0000-000000000006', '5e320300-0000-0000-0000-000000000009', NULL, true, 30.00::numeric, false, '2026-10-15 07:44+07'),
  ('5e320900-0000-0000-0000-000000000006', '5e320800-0000-0000-0000-000000000003', '5e320600-0000-0000-0000-000000000011', '5e320300-0000-0000-0000-000000000018', NULL, true, 50.00::numeric, false, '2026-10-10 08:15+07'),
  ('5e320900-0000-0000-0000-000000000007', '5e320800-0000-0000-0000-000000000003', '5e320600-0000-0000-0000-000000000010', '5e320300-0000-0000-0000-000000000014', NULL, true, 50.00::numeric, false, '2026-10-10 08:30+07'),
  ('5e320900-0000-0000-0000-000000000008', '5e320800-0000-0000-0000-000000000005', '5e320600-0000-0000-0000-000000000010', '5e320300-0000-0000-0000-000000000013', NULL, false, 0.00::numeric, false, '2026-10-10 08:12+07')
) v(id, att, eq, opt, essay, ok, pts, manual, at)
WHERE EXISTS (SELECT 1 FROM exam_attempts a WHERE a.id = v.att::uuid)
  AND EXISTS (SELECT 1 FROM exam_questions q WHERE q.id = v.eq::uuid)
ON CONFLICT DO NOTHING;

-- 7. Riwayat sitting (Budi sempat di-reset karena koneksi putus -> susulan)
INSERT INTO exam_attempt_sittings (id, school_id, exam_id, user_id, attempt_id, sitting, status, score, correct_count,
    started_at, submitted_at, answers, reset_by, created_at)
SELECT '5e320a00-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '5e320500-0000-0000-0000-000000000001',
       '44444444-4444-4444-4444-444444444444', '5e320800-0000-0000-0000-000000000001', 1, 'selesai', 25, 1,
       '2026-09-29 07:35+07', '2026-09-29 07:52+07',
       '[{"id":"5e320a10-0000-0000-0000-000000000001","attempt_id":"5e320800-0000-0000-0000-000000000001","exam_question_id":"5e320600-0000-0000-0000-000000000001","selected_option_id":"5e320300-0000-0000-0000-000000000001","essay_answer":null,"is_correct":true,"points_awarded":25,"manually_graded":false,"answered_at":"2026-09-29T07:40:00+07:00"}]'::jsonb,
       '77777777-7777-7777-7777-777777777777', '2026-09-29 09:00+07'
WHERE EXISTS (SELECT 1 FROM exam_attempts a WHERE a.id = '5e320800-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

COMMIT;
