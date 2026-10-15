-- 33_evaluation_features.sql  (DB: teratai_evaluation)
-- Data demo fitur Evaluasi: Modul RPM (draf/terbit), Prota & Prosem tambahan,
-- periode penilaian guru (draf / berjalan / selesai) beserta tugas penilai
-- (self, kepala sekolah, siswa, sejawat, guru->kepsek) dan jawabannya.
--
-- Hanya MENAMBAH: UUID tetap berawalan 5e33 + ON CONFLICT DO NOTHING. Satu
-- transaksi. Formulir & butir penilaian dibuat aplikasi (BootstrapDefaults);
-- di DB yang belum punya formulir, file ini menambah versi ringkas dengan
-- kode butir yang sama dengan bawaan aplikasi (di server: DO NOTHING).
-- Periode berjalan d4000000-...-001 berasal dari seed 11.
-- Pengguna: guru 7777 (dinilai), kepala sekolah 5555, wali kelas 8888 (sejawat),
-- siswa Budi 4444 (kelas VII A c1000000-...-001).

BEGIN;

-- 1. Formulir & butir bawaan (hanya bila belum ada)
INSERT INTO teacher_evaluation_forms (school_id, code, title, evaluator_type, priority, weight) VALUES
  ('11111111-1111-1111-1111-111111111111', 'F', 'Self-Assessment — Guru Sendiri', 'self', 1, 0.25),
  ('11111111-1111-1111-1111-111111111111', 'E', 'Penilaian Murid — Student Assessment', 'siswa', 2, 0.22),
  ('11111111-1111-1111-1111-111111111111', 'A', 'Kepala Sekolah / Principal', 'kepala_sekolah', 3, 0.20),
  ('11111111-1111-1111-1111-111111111111', 'D', 'Teman Sejawat / Peer Teachers', 'peer', 6, 0.05),
  ('11111111-1111-1111-1111-111111111111', 'G', 'Kepala Sekolah (oleh Guru) / Principal (rated by Teacher)', 'guru_to_kepsek', 7, 0)
ON CONFLICT DO NOTHING;

INSERT INTO teacher_evaluation_items (form_id, school_id, code, indicator, weight, max_score, sort_order)
SELECT f.id, f.school_id, v.code, v.indicator, v.w, 4, v.ord
FROM (VALUES
  ('self', 'F-1', 'Kualitas dan dampak pembelajaran terhadap siswa', 0.05, 1),
  ('self', 'F-2', 'Konsistensi persiapan dan pelaksanaan mengajar', 0.04, 2),
  ('self', 'F-3', 'Inovasi pembelajaran yang diterapkan semester ini', 0.04, 3),
  ('siswa', 'E-1', 'Guru yang Helpful – mudah didekati dan responsif terhadap kebutuhan siswa', 0.03, 1),
  ('siswa', 'E-2', 'Guru yang Memotivasi – menginspirasi dan mendorong potensi terbaik siswa', 0.03, 2),
  ('siswa', 'E-5', 'Kejelasan guru dalam menyampaikan materi pelajaran', 0.03, 5),
  ('kepala_sekolah', 'A.1', 'Kehadiran dan ketepatan waktu dalam semua kewajiban sekolah', 0.02, 1),
  ('kepala_sekolah', 'A.2', 'Kesiapan mengajar tanpa perlu diingatkan atasan', 0.02, 2),
  ('kepala_sekolah', 'B.3', 'Berkontribusi pada iklim kerja yang positif dan kolaboratif', 0.02, 8),
  ('peer', 'D-1', 'Kesediaan berbagi materi, ide, dan sumber ajar', 0.01, 1),
  ('peer', 'D-2', 'Kolaborasi dalam perencanaan dan pelaksanaan program sekolah', 0.01, 2),
  ('guru_to_kepsek', 'A.1', 'Kehadiran dan ketepatan waktu dalam semua kewajiban sekolah', 0.02, 1),
  ('guru_to_kepsek', 'B.3', 'Berkontribusi pada iklim kerja yang positif dan kolaboratif', 0.02, 8)
) v(etype, code, indicator, w, ord)
JOIN teacher_evaluation_forms f ON f.school_id = '11111111-1111-1111-1111-111111111111' AND f.evaluator_type = v.etype
WHERE NOT EXISTS (SELECT 1 FROM teacher_evaluation_items i WHERE i.form_id = f.id)
ON CONFLICT DO NOTHING;

-- 2. Periode penilaian tambahan: satu sudah selesai, satu masih draf
INSERT INTO teacher_evaluation_periods (id, school_id, academic_year_id, title, semester, publish_start_date, publish_end_date,
    published, headmaster_user_id, target_teacher_ids, peer_evaluatee_ids, peer_evaluator_ids, student_classroom_ids,
    lead_assessor_name, created_by, updated_by, created_at)
VALUES
  ('5e330100-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'Supervisi Awal Semester Ganjil 2026/2027', 'Semester 1', '2026-08-17', '2026-09-12', true,
   '55555555-5555-5555-5555-555555555555',
   '["77777777-7777-7777-7777-777777777777"]', '["77777777-7777-7777-7777-777777777777"]',
   '["88888888-8888-8888-8888-888888888888"]', '["c1000000-0000-0000-0000-000000000001"]',
   'Dra. Sri Wahyuni, M.Pd', '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', '2026-08-10 09:00+07'),
  ('5e330100-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'Penilaian Kinerja Guru Semester Genap 2026/2027', 'Semester 2', '2027-03-01', '2027-04-30', false,
   '55555555-5555-5555-5555-555555555555',
   '["77777777-7777-7777-7777-777777777777","88888888-8888-8888-8888-888888888888"]', '[]', '[]',
   '["c1000000-0000-0000-0000-000000000001"]',
   NULL, '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', '2026-10-12 10:00+07')
ON CONFLICT DO NOTHING;

-- 3. Rencana jawaban (dipakai untuk raw_score dan baris jawaban)
CREATE TEMP TABLE _seed33_ans (task_id uuid, code text, score smallint, notes text) ON COMMIT DROP;
INSERT INTO _seed33_ans VALUES
  ('5e330200-0000-0000-0000-000000000001', 'F-1', 3, 'Nilai harian kelas naik rata-rata 6 poin.'),
  ('5e330200-0000-0000-0000-000000000001', 'F-2', 4, 'RPM selalu siap H-1.'),
  ('5e330200-0000-0000-0000-000000000001', 'F-3', 3, 'Mencoba Geogebra untuk aljabar.'),
  ('5e330200-0000-0000-0000-000000000002', 'A.1', 4, 'Tidak pernah terlambat.'),
  ('5e330200-0000-0000-0000-000000000002', 'A.2', 3, ''),
  ('5e330200-0000-0000-0000-000000000002', 'B.3', 4, 'Aktif di MGMP sekolah.'),
  ('5e330200-0000-0000-0000-000000000003', 'E-1', 4, ''),
  ('5e330200-0000-0000-0000-000000000003', 'E-2', 3, ''),
  ('5e330200-0000-0000-0000-000000000003', 'E-5', 4, 'Penjelasan mudah dipahami.'),
  ('5e330200-0000-0000-0000-000000000004', 'D-1', 3, 'Rutin berbagi bahan ajar di grup guru.'),
  ('5e330200-0000-0000-0000-000000000004', 'D-2', 3, ''),
  ('5e330200-0000-0000-0000-000000000005', 'A.1', 3, ''),
  ('5e330200-0000-0000-0000-000000000005', 'A.2', 4, 'Supervisi kelas dilakukan sesuai jadwal.'),
  ('5e330200-0000-0000-0000-000000000005', 'B.3', 3, '');

-- 4. Tugas penilai. Periode selesai (5e330100-...-001): semua terkirim.
--    Periode berjalan (d4000000-...-001): campuran pending & terkirim.
INSERT INTO teacher_evaluation_tasks (id, period_id, form_id, school_id, academic_year_id, target_teacher_user_id,
    evaluator_user_id, evaluator_type, classroom_id, status, submitted_at, raw_score, created_at, updated_at)
SELECT v.id::uuid, v.period::uuid, f.id, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
       v.target::uuid, v.evaluator::uuid, v.etype, v.cls::uuid, v.status, v.sub::timestamptz,
       CASE WHEN v.status = 'submitted' THEN (
         SELECT round(sum(a.score * i.weight) / NULLIF(sum(i.weight), 0), 2)
         FROM _seed33_ans a JOIN teacher_evaluation_items i ON i.form_id = f.id AND i.code = a.code
         WHERE a.task_id = v.id::uuid) END,
       v.created::timestamptz, COALESCE(v.sub, v.created)::timestamptz
FROM (VALUES
  ('5e330200-0000-0000-0000-000000000001', '5e330100-0000-0000-0000-000000000001', 'self', '77777777-7777-7777-7777-777777777777', '77777777-7777-7777-7777-777777777777', NULL, 'submitted', '2026-08-20 15:00+07', '2026-08-17 07:00+07'),
  ('5e330200-0000-0000-0000-000000000002', '5e330100-0000-0000-0000-000000000001', 'kepala_sekolah', '77777777-7777-7777-7777-777777777777', '55555555-5555-5555-5555-555555555555', NULL, 'submitted', '2026-09-02 11:00+07', '2026-08-17 07:00+07'),
  ('5e330200-0000-0000-0000-000000000003', '5e330100-0000-0000-0000-000000000001', 'siswa', '77777777-7777-7777-7777-777777777777', '44444444-4444-4444-4444-444444444444', 'c1000000-0000-0000-0000-000000000001', 'submitted', '2026-08-25 13:00+07', '2026-08-17 07:00+07'),
  ('5e330200-0000-0000-0000-000000000004', '5e330100-0000-0000-0000-000000000001', 'peer', '77777777-7777-7777-7777-777777777777', '88888888-8888-8888-8888-888888888888', NULL, 'submitted', '2026-09-05 10:00+07', '2026-08-17 07:00+07'),
  ('5e330200-0000-0000-0000-000000000005', 'd4000000-0000-0000-0000-000000000001', 'kepala_sekolah', '77777777-7777-7777-7777-777777777777', '55555555-5555-5555-5555-555555555555', NULL, 'submitted', '2026-10-09 09:30+07', '2026-10-05 07:00+07'),
  ('5e330200-0000-0000-0000-000000000006', 'd4000000-0000-0000-0000-000000000001', 'self', '77777777-7777-7777-7777-777777777777', '77777777-7777-7777-7777-777777777777', NULL, 'pending', NULL, '2026-10-05 07:00+07'),
  ('5e330200-0000-0000-0000-000000000007', 'd4000000-0000-0000-0000-000000000001', 'siswa', '77777777-7777-7777-7777-777777777777', '44444444-4444-4444-4444-444444444444', 'c1000000-0000-0000-0000-000000000001', 'pending', NULL, '2026-10-05 07:00+07'),
  ('5e330200-0000-0000-0000-000000000008', 'd4000000-0000-0000-0000-000000000001', 'peer', '77777777-7777-7777-7777-777777777777', '88888888-8888-8888-8888-888888888888', NULL, 'pending', NULL, '2026-10-05 07:00+07'),
  ('5e330200-0000-0000-0000-000000000009', 'd4000000-0000-0000-0000-000000000001', 'guru_to_kepsek', '55555555-5555-5555-5555-555555555555', '77777777-7777-7777-7777-777777777777', NULL, 'pending', NULL, '2026-10-05 07:00+07')
) v(id, period, etype, target, evaluator, cls, status, sub, created)
JOIN teacher_evaluation_forms f ON f.school_id = '11111111-1111-1111-1111-111111111111' AND f.evaluator_type = v.etype
WHERE EXISTS (SELECT 1 FROM teacher_evaluation_periods p WHERE p.id = v.period::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO teacher_evaluation_answers (id, task_id, item_id, school_id, score, notes)
SELECT md5('seed33-answer:' || a.task_id || ':' || a.code)::uuid, t.id, i.id, t.school_id, a.score, a.notes
FROM _seed33_ans a
JOIN teacher_evaluation_tasks t ON t.id = a.task_id
JOIN teacher_evaluation_items i ON i.form_id = t.form_id AND i.code = a.code
ON CONFLICT DO NOTHING;

-- 5. Modul RPM: draf & terbit
INSERT INTO lesson_plans (id, school_id, academic_year_id, guru_user_id, title, section_a, section_b, section_c, dimensi_lulusan, status, created_at)
VALUES
  ('5e330300-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', '77777777-7777-7777-7777-777777777777',
   'RPM Matematika Kelas VII: Bentuk Aljabar',
   '{"mata_pelajaran":"Matematika","kelas":"VII A","fase":"D","alokasi_waktu":"5 JP x 40 menit","karakteristik_peserta_didik":"Sebagian siswa masih kesulitan operasi bilangan negatif."}',
   '{"capaian_pembelajaran":"Peserta didik dapat menyatakan situasi ke dalam bentuk aljabar.","tujuan_pembelajaran":"Menyederhanakan bentuk aljabar dengan suku sejenis.","pemahaman_bermakna":"Aljabar membantu menyelesaikan masalah sehari-hari.","pertanyaan_pemantik":"Bagaimana menuliskan harga 3 buku dan 2 pensil tanpa mengetahui harganya?","asesmen_diagnostik":"Kuis 5 soal operasi bilangan bulat."}',
   '{"kegiatan_awal":"Apersepsi harga belanja di kantin.","kegiatan_inti":"Diskusi kelompok mengelompokkan suku sejenis memakai kartu aljabar.","kegiatan_penutup":"Refleksi dan exit ticket.","asesmen_formatif":"LKPD 10 soal","refleksi":"Siswa menuliskan bagian yang paling sulit."}',
   '{"bernalar_kritis":"Menganalisis pola suku sejenis.","bergotong_royong":"Bekerja dalam kelompok 4 orang."}',
   'published', '2026-08-20 14:00+07'),
  ('5e330300-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', '77777777-7777-7777-7777-777777777777',
   'RPM Matematika Kelas VII: Persamaan Linear Satu Variabel (draf)',
   '{"mata_pelajaran":"Matematika","kelas":"VII A","fase":"D","alokasi_waktu":"4 JP x 40 menit"}',
   '{"tujuan_pembelajaran":"Menyelesaikan PLSV dengan kesetaraan."}',
   '{}', '{}', 'draft', '2026-10-11 16:00+07'),
  ('5e330300-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', '88888888-8888-8888-8888-888888888888',
   'RPM Bahasa Indonesia Kelas VII: Teks Deskripsi',
   '{"mata_pelajaran":"Bahasa Indonesia","kelas":"VII A","fase":"D","alokasi_waktu":"4 JP x 40 menit"}',
   '{"tujuan_pembelajaran":"Menulis teks deskripsi tentang lingkungan sekolah.","pertanyaan_pemantik":"Apa yang membuat sebuah tempat mudah dibayangkan pembaca?"}',
   '{"kegiatan_awal":"Mengamati foto taman sekolah.","kegiatan_inti":"Observasi lapangan dan menulis draf.","kegiatan_penutup":"Saling menilai tulisan teman."}',
   '{"kreatif":"Memilih diksi yang hidup."}',
   'published', '2026-08-25 10:00+07')
ON CONFLICT DO NOTHING;

-- 6. Prota & Prosem tambahan
INSERT INTO prota (id, school_id, academic_year_id, guru_user_id, mata_pelajaran, kelas) VALUES
  ('5e330400-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', '88888888-8888-8888-8888-888888888888', 'Bahasa Indonesia', 'VII'),
  ('5e330400-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', '77777777-7777-7777-7777-777777777777', 'Matematika', 'VIII')
ON CONFLICT DO NOTHING;

INSERT INTO prota_items (id, prota_id, no, semester, elemen_kode_bab, kompetensi_dasar_tp, alokasi_waktu_jp, keterangan_target_asesmen)
SELECT v.id::uuid, v.p::uuid, v.no, v.sem, v.bab, v.tp, v.jp, v.ket
FROM (VALUES
  ('5e330500-0000-0000-0000-000000000001', '5e330400-0000-0000-0000-000000000001', 1, '1', 'BAB 1', 'Teks Deskripsi: Lingkungan Sekitar', 18, 'Formatif 1'),
  ('5e330500-0000-0000-0000-000000000002', '5e330400-0000-0000-0000-000000000001', 2, '1', 'BAB 2', 'Teks Narasi: Cerita Fantasi', 20, 'Formatif 2 & STS'),
  ('5e330500-0000-0000-0000-000000000003', '5e330400-0000-0000-0000-000000000001', 3, '2', 'BAB 3', 'Teks Prosedur', 16, 'Formatif 3 & SAS'),
  ('5e330500-0000-0000-0000-000000000004', '5e330400-0000-0000-0000-000000000002', 1, '1', 'BAB 1', 'Pola Bilangan', 16, 'Formatif 1'),
  ('5e330500-0000-0000-0000-000000000005', '5e330400-0000-0000-0000-000000000002', 2, '1', 'BAB 2', 'Koordinat Kartesius', 14, 'Formatif 2 & STS'),
  ('5e330500-0000-0000-0000-000000000006', '5e330400-0000-0000-0000-000000000002', 3, '2', 'BAB 3', 'Relasi dan Fungsi', 18, 'Sumatif SAS')
) v(id, p, no, sem, bab, tp, jp, ket)
WHERE EXISTS (SELECT 1 FROM prota p WHERE p.id = v.p::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO prosem (id, school_id, academic_year_id, guru_user_id, mata_pelajaran, kelas) VALUES
  ('5e330600-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', '88888888-8888-8888-8888-888888888888', 'Bahasa Indonesia', 'VII')
ON CONFLICT DO NOTHING;

INSERT INTO prosem_items (id, prosem_id, no, materi_pokok_aktivitas, alokasi_jp, bulan_target, minggu_ke, estimasi_tanggal, tipe_aktivitas_lms, status_input_guru)
SELECT v.id::uuid, '5e330600-0000-0000-0000-000000000001', v.no, v.materi, v.jp, v.bulan, v.minggu, v.tgl::date, v.tipe, v.status
FROM (VALUES
  ('5e330700-0000-0000-0000-000000000001', 1, 'Mengamati dan mendeskripsikan objek', 4, 'Juli', 3, '2026-07-20', 'Tatap Muka', 'Selesai'),
  ('5e330700-0000-0000-0000-000000000002', 2, 'Struktur dan kaidah teks deskripsi', 4, 'Agustus', 1, '2026-08-03', 'Tatap Muka & Kuis', 'Selesai'),
  ('5e330700-0000-0000-0000-000000000003', 3, 'Membaca cerita fantasi', 4, 'September', 2, '2026-09-07', 'Tugas Mandiri', 'Selesai'),
  ('5e330700-0000-0000-0000-000000000004', 4, 'Menulis cerita fantasi', 4, 'Oktober', 3, '2026-10-19', 'Proyek', 'Belum'),
  ('5e330700-0000-0000-0000-000000000005', 5, 'Sumatif Tengah Semester', 2, 'Oktober', 4, '2026-10-26', 'Ujian CBT', 'Belum')
) v(id, no, materi, jp, bulan, minggu, tgl, tipe, status)
WHERE EXISTS (SELECT 1 FROM prosem p WHERE p.id = '5e330600-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

COMMIT;
