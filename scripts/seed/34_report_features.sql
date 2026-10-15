-- 34_report_features.sql  (DB: teratai_report)
-- Data demo fitur Rapor: predikat nilai (grade band), bobot rapor, template
-- catatan, tipe penilaian praktik/formatif tambahan, nilai formatif per mapel,
-- serta rapor bayangan (tengah semester) Budi yang sudah difinalisasi.
--
-- Hanya MENAMBAH: UUID tetap berawalan 5e34 + ON CONFLICT DO NOTHING. Satu
-- transaksi. report_templates (file .docx biner) sengaja tidak di-seed.
-- Referensi lintas layanan (ada di server, dari seed 02/12):
--   term c0000000-...-001, kelas VII A c1000000-...-001, mapel c2000000-...-001/002,
--   classroom_subject c3000000-...-001/002, tipe penilaian f4000000-...-001/002/003.

BEGIN;

-- 1. Predikat nilai: hanya bila sekolah belum punya band sendiri
--    (nilainya sama dengan DefaultGradeBands di layanan report).
INSERT INTO report_grade_bands (id, school_id, min_score, max_score, label, description)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.mn, v.mx, v.label, v.descr
FROM (VALUES
  ('5e340100-0000-0000-0000-000000000001', 90.00, 100.00, 'A', 'Sangat baik: menguasai seluruh tujuan pembelajaran'),
  ('5e340100-0000-0000-0000-000000000002', 80.00, 89.99, 'B', 'Baik: menguasai sebagian besar tujuan pembelajaran'),
  ('5e340100-0000-0000-0000-000000000003', 70.00, 79.99, 'C', 'Cukup: perlu penguatan pada beberapa materi'),
  ('5e340100-0000-0000-0000-000000000004', 0.00, 69.99, 'D', 'Perlu bimbingan: belum mencapai KKTP')
) v(id, mn, mx, label, descr)
WHERE NOT EXISTS (
  SELECT 1 FROM report_grade_bands b
  WHERE b.school_id = '11111111-1111-1111-1111-111111111111' AND b.id NOT IN (
    '5e340100-0000-0000-0000-000000000001', '5e340100-0000-0000-0000-000000000002',
    '5e340100-0000-0000-0000-000000000003', '5e340100-0000-0000-0000-000000000004'))
ON CONFLICT DO NOTHING;

-- 2. Bobot rapor (nilai bawaan aplikasi) dan template catatan rapor
INSERT INTO report_weight_settings (school_id, akhir_formatif_weight, akhir_sts_weight, akhir_sas_weight,
    sementara_formatif_weight, sementara_praktik_weight, sementara_sts_weight, updated_by)
VALUES ('11111111-1111-1111-1111-111111111111', 30, 30, 40, 30, 40, 30, '22222222-2222-2222-2222-222222222222')
ON CONFLICT DO NOTHING;

INSERT INTO report_note_templates (school_id, mid_template, final_template, updated_by)
VALUES ('11111111-1111-1111-1111-111111111111',
  'Ananda {nama} menunjukkan perkembangan yang baik pada tengah semester ini. Tingkatkan terus semangat belajarnya.',
  'Ananda {nama} telah menyelesaikan semester dengan baik. Pertahankan prestasi dan tingkatkan kedisiplinan.',
  '22222222-2222-2222-2222-222222222222')
ON CONFLICT DO NOTHING;

-- 3. Tipe penilaian tambahan (formatif ke-2 dan praktik)
INSERT INTO report_assessment_types (id, school_id, code, name, bucket, weight_percent, exam_linked, sort_order) VALUES
  ('5e340200-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'FORMATIF-2', 'Penilaian Harian Formatif 2', 'formatif', 100.00, false, 4),
  ('5e340200-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'PRAKTIK-1', 'Penilaian Praktik / Proyek', 'praktik', 100.00, false, 5)
ON CONFLICT DO NOTHING;

-- 4. Nilai formatif/praktik/STS Budi (Matematika & Bahasa Indonesia, Semester 1)
INSERT INTO formative_grades (id, school_id, term_id, classroom_subject_id, student_id, assessment_type_id, score, created_by, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', 'c0000000-0000-0000-0000-000000000001', v.cs::uuid,
       '44444444-4444-4444-4444-444444444444', t.id, v.score, v.guru::uuid, v.at::timestamptz
FROM (VALUES
  ('5e340300-0000-0000-0000-000000000001', 'c3000000-0000-0000-0000-000000000001', 'FORMATIF-2', 84.00, '77777777-7777-7777-7777-777777777777', '2026-09-30 14:00+07'),
  ('5e340300-0000-0000-0000-000000000002', 'c3000000-0000-0000-0000-000000000001', 'PRAKTIK-1', 90.00, '77777777-7777-7777-7777-777777777777', '2026-10-02 14:00+07'),
  ('5e340300-0000-0000-0000-000000000003', 'c3000000-0000-0000-0000-000000000001', 'STS-1', 86.00, '77777777-7777-7777-7777-777777777777', '2026-10-14 15:00+07'),
  ('5e340300-0000-0000-0000-000000000004', 'c3000000-0000-0000-0000-000000000002', 'FORMATIF-1', 82.00, '77777777-7777-7777-7777-777777777777', '2026-09-15 14:00+07'),
  ('5e340300-0000-0000-0000-000000000005', 'c3000000-0000-0000-0000-000000000002', 'FORMATIF-2', 86.00, '77777777-7777-7777-7777-777777777777', '2026-10-01 14:00+07'),
  ('5e340300-0000-0000-0000-000000000006', 'c3000000-0000-0000-0000-000000000002', 'PRAKTIK-1', 88.00, '77777777-7777-7777-7777-777777777777', '2026-10-06 14:00+07')
) v(id, cs, code, score, guru, at)
JOIN report_assessment_types t ON t.school_id = '11111111-1111-1111-1111-111111111111' AND t.code = v.code
ON CONFLICT DO NOTHING;

-- 5. Rapor bayangan (tengah semester) Budi — sudah difinalisasi wali kelas
INSERT INTO report_cards (id, school_id, academic_year_id, term_id, classroom_id, student_id, status, finalized_at, finalized_by,
    class_teacher_note, attendance_hadir, attendance_sakit, attendance_izin, attendance_tanpa_keterangan, attendance_synced_at,
    student_name, student_nisn, classroom_name, term_name, header_synced_at, homeroom_teacher_name, principal_name,
    report_type, period_label, kokurikuler_note, ekstrakurikuler, prestasi, created_at)
VALUES ('5e340400-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
    'c0000000-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444',
    'finalized', '2026-10-15 10:00+07', '88888888-8888-8888-8888-888888888888',
    'Ananda Budi menunjukkan perkembangan yang baik pada tengah semester ini. Tingkatkan ketelitian dalam mengerjakan soal cerita.',
    52, 2, 1, 1, '2026-10-15 09:55+07',
    'Budi Santoso', '0087654321', 'Kelas VII A', 'Semester 1 (Ganjil)', '2026-10-15 09:55+07',
    'Wali Kelas VII A', 'Kepala Sekolah SMP Teratai',
    'bayangan', 'Tengah Semester Ganjil 2026/2027',
    'Aktif dalam projek "Gaya Hidup Berkelanjutan": memilah sampah kelas.',
    '[{"kegiatan":"Klub Robotik & Coding","predikat":"A","keterangan":"Aktif dan menjadi ketua kelompok"},{"kegiatan":"Pramuka","predikat":"B","keterangan":"Mengikuti latihan rutin"}]',
    '[{"jenis":"Juara 2 Lomba Robotik Tingkat Kota","keterangan":"September 2026"}]',
    '2026-10-14 08:00+07')
ON CONFLICT DO NOTHING;

INSERT INTO report_card_subjects (id, report_card_id, school_id, subject_id, subject_name, subject_code, computed_score,
    source_exam_count, grade_label, mid_note, formatif_score, praktik_score, sts_score, last_synced_at)
SELECT v.id::uuid, '5e340400-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', v.subj::uuid, v.nm, v.code,
       v.score, 1, v.grade, v.note, v.f, v.p, v.s, '2026-10-15 09:55+07'
FROM (VALUES
  ('5e340500-0000-0000-0000-000000000001', 'c2000000-0000-0000-0000-000000000001', 'Matematika', 'MTK-7', 88.20, 'B',
   'Menguasai operasi bilangan bulat dan bentuk aljabar; perlu latihan soal cerita.', 88.00, 90.00, 86.00),
  ('5e340500-0000-0000-0000-000000000002', 'c2000000-0000-0000-0000-000000000002', 'Bahasa Indonesia', 'BIN-7', 85.30, 'B',
   'Mampu menulis teks deskripsi yang runtut; tingkatkan penggunaan kosakata baku.', 84.00, 88.00, 83.00)
) v(id, subj, nm, code, score, grade, note, f, p, s)
WHERE EXISTS (SELECT 1 FROM report_cards r WHERE r.id = '5e340400-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

COMMIT;
