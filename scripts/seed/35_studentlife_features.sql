-- 35_studentlife_features.sql  (DB: teratai_studentlife)
-- Data demo fitur Kesiswaan: pengaturan modul, poin kedisiplinan (ambang,
-- catatan, tindak lanjut, keberatan), Bimbingan Konseling (kasus, anggota,
-- sesi, tugas), ekstrakurikuler (periode, pembina, anggota, sesi, presensi,
-- penilaian), kegiatan sekolah (jenis, kegiatan, panitia, pendaftaran,
-- persetujuan orang tua), program kesehatan UKS, dan PKL (periode, skema,
-- komponen, kuota, MoU, penempatan, presensi, jurnal, nilai, hasil).
--
-- Hanya MENAMBAH: UUID tetap berawalan 5e35 + ON CONFLICT DO NOTHING. Satu
-- transaksi. Baris induk yang sudah ada di server (dibuat seed 05 dengan UUID
-- acak) dicari lewat kunci alami: discipline_types.code, counseling_categories.name,
-- act_extracurriculars.name, majors.code, pkl_partners.name.
-- Sengaja TIDAK di-seed: tabel yang isinya terenkripsi kunci server
-- (health_profiles, health_profile_changes, clinic_visits + turunannya, referrals),
-- konfigurasi keamanan (confidential_access_policies), log (audit_events,
-- sensitive_access_logs), akun mentor industri (industry_mentors) dan berkas
-- objek (pkl_certificates, pkl_final_reports).
-- Pengguna: admin/BK/kesiswaan/pembina/koordinator PKL 2222, wali murid 3333,
-- siswa Budi 4444, kepala sekolah 5555, guru 7777, wali kelas 8888.

BEGIN;

-- 1. Pengaturan modul (nilai bawaan aplikasi)
INSERT INTO studentlife_settings (school_id, timezone) VALUES ('11111111-1111-1111-1111-111111111111', 'Asia/Jakarta') ON CONFLICT DO NOTHING;
INSERT INTO discipline_settings (school_id, accumulation_scope) VALUES ('11111111-1111-1111-1111-111111111111', 'academic_year') ON CONFLICT DO NOTHING;
INSERT INTO counseling_settings (school_id, reminder_days_before) VALUES ('11111111-1111-1111-1111-111111111111', 1) ON CONFLICT DO NOTHING;
INSERT INTO pkl_settings (school_id, certificate_prefix) VALUES ('11111111-1111-1111-1111-111111111111', 'PKL/SMPTERATAI') ON CONFLICT DO NOTHING;

-- 2. Kedisiplinan --------------------------------------------------------------
INSERT INTO discipline_thresholds (id, school_id, points, action_name, notify_parent, effective_from) VALUES
  ('5e350100-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 10, 'Teguran lisan oleh wali kelas', false, '2026-07-01'),
  ('5e350100-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 25, 'Surat Peringatan 1 dan pemanggilan orang tua', true, '2026-07-01'),
  ('5e350100-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 50, 'Surat Peringatan 2 dan pembinaan oleh BK', true, '2026-07-01'),
  ('5e350100-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 75, 'Skorsing 3 hari dengan tugas terstruktur', true, '2026-07-01')
ON CONFLICT DO NOTHING;

INSERT INTO discipline_records (id, school_id, student_id, academic_year_id, type_id, kind, points, occurred_on, description,
    reporter_user_id, status, annulled_reason, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', '12121212-1212-1212-1212-121212121212',
       t.id, t.kind, t.points, v.occ::date, v.descr, v.reporter::uuid, v.status, v.annul, (v.occ || ' 08:00+07')::timestamptz
FROM (VALUES
  ('5e350200-0000-0000-0000-000000000001', 'DISC-LATE', '2026-08-04', 'Terlambat 20 menit, gerbang sudah ditutup.', '88888888-8888-8888-8888-888888888888', 'active', NULL),
  ('5e350200-0000-0000-0000-000000000002', 'DISC-UNIFORM', '2026-08-18', 'Tidak memakai dasi saat upacara.', '22222222-2222-2222-2222-222222222222', 'active', NULL),
  ('5e350200-0000-0000-0000-000000000003', 'DISC-LATE', '2026-09-08', 'Terlambat 25 menit tanpa keterangan.', '88888888-8888-8888-8888-888888888888', 'active', NULL),
  ('5e350200-0000-0000-0000-000000000004', 'DISC-MERIT-ROBOT', '2026-09-20', 'Juara 2 Lomba Robotik Pelajar Kota Bekasi.', '22222222-2222-2222-2222-222222222222', 'active', NULL),
  ('5e350200-0000-0000-0000-000000000005', 'DISC-LATE', '2026-10-06', 'Tercatat terlambat 16 menit.', '88888888-8888-8888-8888-888888888888', 'annulled', 'Keberatan disetujui: bus sekolah mogok, ada surat keterangan sopir.'),
  ('5e350200-0000-0000-0000-000000000006', 'DISC-UNIFORM', '2026-10-13', 'Sepatu tidak berwarna hitam.', '22222222-2222-2222-2222-222222222222', 'active', NULL)
) v(id, code, occ, descr, reporter, status, annul)
JOIN discipline_types t ON t.school_id = '11111111-1111-1111-1111-111111111111' AND t.code = v.code AND t.deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- Tindak lanjut: ambang 10 poin terlewati oleh catatan 08-09 (5 + 3 + 5 = 13)
INSERT INTO discipline_actions (id, school_id, student_id, academic_year_id, threshold_id, triggered_by_record_id, points_at_trigger,
    status, assignee_user_id, notes, done_at, created_at)
SELECT '5e350300-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444',
       '12121212-1212-1212-1212-121212121212', '5e350100-0000-0000-0000-000000000001', '5e350200-0000-0000-0000-000000000003', 13,
       'done', '88888888-8888-8888-8888-888888888888', 'Teguran lisan sudah diberikan; Budi berjanji berangkat lebih awal.',
       '2026-09-10 13:00+07', '2026-09-08 08:05+07'
WHERE EXISTS (SELECT 1 FROM discipline_records WHERE id = '5e350200-0000-0000-0000-000000000003')
  AND EXISTS (SELECT 1 FROM discipline_thresholds WHERE id = '5e350100-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- Keberatan orang tua (sudah diputus; status pending butuh approval di Core)
INSERT INTO discipline_appeals (id, school_id, record_id, guardian_user_id, reason, status, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.rec::uuid, '33333333-3333-3333-3333-333333333333', v.reason, v.status, v.at::timestamptz
FROM (VALUES
  ('5e350400-0000-0000-0000-000000000001', '5e350200-0000-0000-0000-000000000005', 'Anak saya terlambat karena bus sekolah mogok di jalan. Terlampir surat keterangan dari sopir.', 'approved', '2026-10-06 19:00+07'),
  ('5e350400-0000-0000-0000-000000000002', '5e350200-0000-0000-0000-000000000006', 'Sepatu hitam sedang dicuci karena basah kehujanan.', 'rejected', '2026-10-13 18:30+07')
) v(id, rec, reason, status, at)
WHERE EXISTS (SELECT 1 FROM discipline_records r WHERE r.id = v.rec::uuid)
ON CONFLICT DO NOTHING;

-- 3. Bimbingan Konseling -------------------------------------------------------
-- Kolom *_enc (catatan rahasia) dibiarkan NULL: dienkripsi kunci server.
INSERT INTO counseling_cases (id, school_id, student_id, category_id, opened_by, status, disclose_to_homeroom, opened_at, closed_at, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', c.id,
       '22222222-2222-2222-2222-222222222222', v.status, v.disclose, v.opened::timestamptz, v.closed::timestamptz, v.opened::timestamptz
FROM (VALUES
  ('5e350500-0000-0000-0000-000000000001', 'Akademik & Motivasi Belajar', 'monitoring', true, '2026-09-09 10:00+07', NULL),
  ('5e350500-0000-0000-0000-000000000002', 'Karier & Perencanaan Studi', 'closed', false, '2026-08-11 10:00+07', '2026-09-01 14:00+07'),
  ('5e350500-0000-0000-0000-000000000003', 'Pribadi & Emosional', 'open', false, '2026-10-12 09:00+07', NULL)
) v(id, cat, status, disclose, opened, closed)
JOIN counseling_categories c ON c.school_id = '11111111-1111-1111-1111-111111111111' AND c.name = v.cat AND c.deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO counseling_case_members (case_id, user_id, member_role, added_by, added_at)
SELECT v.cid::uuid, v.uid::uuid, v.role, '22222222-2222-2222-2222-222222222222', v.at::timestamptz
FROM (VALUES
  ('5e350500-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222', 'counselor', '2026-09-09 10:00+07'),
  ('5e350500-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222', 'counselor', '2026-08-11 10:00+07'),
  ('5e350500-0000-0000-0000-000000000003', '22222222-2222-2222-2222-222222222222', 'counselor', '2026-10-12 09:00+07')
) v(cid, uid, role, at)
WHERE EXISTS (SELECT 1 FROM counseling_cases k WHERE k.id = v.cid::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO counseling_sessions (id, school_id, case_id, kind, category_id, counselor_user_id, scheduled_at, held_at, status, reminder_sent_at, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.cid::uuid, v.kind, c.id, '22222222-2222-2222-2222-222222222222',
       v.sched::timestamptz, v.held::timestamptz, v.status, v.rem::timestamptz, (v.sched::timestamptz - interval '3 days')
FROM (VALUES
  ('5e350600-0000-0000-0000-000000000001', '5e350500-0000-0000-0000-000000000001', 'individual', 'Akademik & Motivasi Belajar', '2026-09-11 10:00+07', '2026-09-11 10:05+07', 'held', '2026-09-10 07:00+07'),
  ('5e350600-0000-0000-0000-000000000002', '5e350500-0000-0000-0000-000000000001', 'individual', 'Akademik & Motivasi Belajar', '2026-09-25 10:00+07', NULL, 'no_show', '2026-09-24 07:00+07'),
  ('5e350600-0000-0000-0000-000000000003', '5e350500-0000-0000-0000-000000000001', 'individual', 'Akademik & Motivasi Belajar', '2026-10-20 10:00+07', NULL, 'scheduled', NULL),
  ('5e350600-0000-0000-0000-000000000004', '5e350500-0000-0000-0000-000000000002', 'individual', 'Karier & Perencanaan Studi', '2026-08-14 13:00+07', '2026-08-14 13:00+07', 'held', '2026-08-13 07:00+07'),
  ('5e350600-0000-0000-0000-000000000005', NULL, 'group', 'Karier & Perencanaan Studi', '2026-09-30 13:00+07', '2026-09-30 13:10+07', 'held', '2026-09-29 07:00+07'),
  ('5e350600-0000-0000-0000-000000000006', '5e350500-0000-0000-0000-000000000003', 'individual', 'Pribadi & Emosional', '2026-10-14 10:00+07', NULL, 'cancelled', NULL)
) v(id, cid, kind, cat, sched, held, status, rem)
JOIN counseling_categories c ON c.school_id = '11111111-1111-1111-1111-111111111111' AND c.name = v.cat AND c.deleted_at IS NULL
WHERE v.cid IS NULL OR EXISTS (SELECT 1 FROM counseling_cases k WHERE k.id = v.cid::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO counseling_session_students (session_id, student_id)
SELECT s.id, '44444444-4444-4444-4444-444444444444' FROM counseling_sessions s
WHERE s.id IN ('5e350600-0000-0000-0000-000000000001', '5e350600-0000-0000-0000-000000000002', '5e350600-0000-0000-0000-000000000003',
               '5e350600-0000-0000-0000-000000000004', '5e350600-0000-0000-0000-000000000005', '5e350600-0000-0000-0000-000000000006')
ON CONFLICT DO NOTHING;

INSERT INTO case_tasks (id, school_id, case_id, title, assignee_user_id, due_date, done_at, done_by, created_by, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.cid::uuid, v.title, v.assignee::uuid, v.due::date,
       v.done::timestamptz, CASE WHEN v.done IS NOT NULL THEN v.assignee::uuid END, '22222222-2222-2222-2222-222222222222', v.at::timestamptz
FROM (VALUES
  ('5e350700-0000-0000-0000-000000000001', '5e350500-0000-0000-0000-000000000001', 'Koordinasi dengan wali kelas tentang nilai harian Matematika', '88888888-8888-8888-8888-888888888888', '2026-09-18', '2026-09-17 14:00+07', '2026-09-11 11:00+07'),
  ('5e350700-0000-0000-0000-000000000002', '5e350500-0000-0000-0000-000000000001', 'Pantau kehadiran dan keterlambatan selama 2 minggu', '22222222-2222-2222-2222-222222222222', '2026-10-30', NULL, '2026-10-06 09:00+07'),
  ('5e350700-0000-0000-0000-000000000003', '5e350500-0000-0000-0000-000000000002', 'Berikan info jalur SMA/SMK unggulan di Bekasi', '22222222-2222-2222-2222-222222222222', '2026-08-28', '2026-08-27 10:00+07', '2026-08-14 14:00+07'),
  ('5e350700-0000-0000-0000-000000000004', '5e350500-0000-0000-0000-000000000003', 'Jadwalkan ulang sesi konseling individu', '22222222-2222-2222-2222-222222222222', '2026-10-21', NULL, '2026-10-14 10:30+07')
) v(id, cid, title, assignee, due, done, at)
WHERE EXISTS (SELECT 1 FROM counseling_cases k WHERE k.id = v.cid::uuid)
ON CONFLICT DO NOTHING;

-- 4. Ekstrakurikuler -----------------------------------------------------------
INSERT INTO act_ecc_periods (id, school_id, academic_year_id, name, opens_at, closes_at, created_at) VALUES
  ('5e350800-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'Pendaftaran Ekskul Semester Ganjil 2026/2027', '2026-07-20 07:00+07', '2026-08-08 23:59+07', '2026-07-15 09:00+07'),
  ('5e350800-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'Pendaftaran Ekskul Semester Genap 2026/2027', '2027-01-11 07:00+07', '2027-01-30 23:59+07', '2026-10-10 09:00+07')
ON CONFLICT DO NOTHING;

INSERT INTO act_ecc_coaches (ecc_id, user_id, school_id)
SELECT e.id, v.uid::uuid, e.school_id
FROM (VALUES
  ('Klub Robotik & Coding', '22222222-2222-2222-2222-222222222222'),
  ('Klub Robotik & Coding', '77777777-7777-7777-7777-777777777777'),
  ('Futsal Teratai Putih', '22222222-2222-2222-2222-222222222222'),
  ('Pramuka Inti', '22222222-2222-2222-2222-222222222222'),
  ('Pramuka Inti', '88888888-8888-8888-8888-888888888888')
) v(ecc, uid)
JOIN act_extracurriculars e ON e.school_id = '11111111-1111-1111-1111-111111111111' AND e.name = v.ecc AND e.deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO act_ecc_members (id, school_id, ecc_id, period_id, student_user_id, status, joined_at, left_at, created_at)
SELECT v.id::uuid, e.school_id, e.id, '5e350800-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444',
       v.status, v.joined::timestamptz, v.lft::timestamptz, v.joined::timestamptz
FROM (VALUES
  ('5e350900-0000-0000-0000-000000000001', 'Klub Robotik & Coding', 'active', '2026-07-21 10:00+07', NULL),
  ('5e350900-0000-0000-0000-000000000002', 'Pramuka Inti', 'active', '2026-07-21 10:05+07', NULL),
  ('5e350900-0000-0000-0000-000000000003', 'Futsal Teratai Putih', 'left', '2026-07-22 15:00+07', '2026-09-01 12:00+07')
) v(id, ecc, status, joined, lft)
JOIN act_extracurriculars e ON e.school_id = '11111111-1111-1111-1111-111111111111' AND e.name = v.ecc AND e.deleted_at IS NULL
WHERE EXISTS (SELECT 1 FROM act_ecc_periods p WHERE p.id = '5e350800-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

INSERT INTO act_ecc_sessions (id, school_id, ecc_id, held_on, note, created_by, created_at)
SELECT v.id::uuid, e.school_id, e.id, v.held::date, v.note, '22222222-2222-2222-2222-222222222222', (v.held || ' 17:00+07')::timestamptz
FROM (VALUES
  ('5e350a00-0000-0000-0000-000000000001', 'Klub Robotik & Coding', '2026-09-02', 'Pengenalan sensor ultrasonik'),
  ('5e350a00-0000-0000-0000-000000000002', 'Klub Robotik & Coding', '2026-09-09', 'Pemrograman line follower'),
  ('5e350a00-0000-0000-0000-000000000003', 'Klub Robotik & Coding', '2026-09-16', 'Simulasi lomba'),
  ('5e350a00-0000-0000-0000-000000000004', 'Klub Robotik & Coding', '2026-09-23', 'Evaluasi pasca lomba'),
  ('5e350a00-0000-0000-0000-000000000005', 'Pramuka Inti', '2026-09-05', 'Tali-temali dan sandi'),
  ('5e350a00-0000-0000-0000-000000000006', 'Pramuka Inti', '2026-09-19', 'Persiapan Persami')
) v(id, ecc, held, note)
JOIN act_extracurriculars e ON e.school_id = '11111111-1111-1111-1111-111111111111' AND e.name = v.ecc AND e.deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO act_ecc_attendance (session_id, member_id, school_id, status)
SELECT v.sid::uuid, v.mid::uuid, '11111111-1111-1111-1111-111111111111', v.status
FROM (VALUES
  ('5e350a00-0000-0000-0000-000000000001', '5e350900-0000-0000-0000-000000000001', 'hadir'),
  ('5e350a00-0000-0000-0000-000000000002', '5e350900-0000-0000-0000-000000000001', 'hadir'),
  ('5e350a00-0000-0000-0000-000000000003', '5e350900-0000-0000-0000-000000000001', 'izin'),
  ('5e350a00-0000-0000-0000-000000000004', '5e350900-0000-0000-0000-000000000001', 'hadir'),
  ('5e350a00-0000-0000-0000-000000000005', '5e350900-0000-0000-0000-000000000002', 'sakit'),
  ('5e350a00-0000-0000-0000-000000000006', '5e350900-0000-0000-0000-000000000002', 'alpa')
) v(sid, mid, status)
WHERE EXISTS (SELECT 1 FROM act_ecc_sessions s WHERE s.id = v.sid::uuid)
  AND EXISTS (SELECT 1 FROM act_ecc_members m WHERE m.id = v.mid::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO act_ecc_assessments (id, school_id, ecc_id, period_id, member_id, predicate, description, assessed_by, created_at)
SELECT v.id::uuid, m.school_id, m.ecc_id, m.period_id, m.id, v.pred, v.descr, '22222222-2222-2222-2222-222222222222', '2026-10-10 10:00+07'
FROM (VALUES
  ('5e350b00-0000-0000-0000-000000000001', '5e350900-0000-0000-0000-000000000001', 'A', 'Sangat aktif, memimpin tim pada lomba robotik tingkat kota.'),
  ('5e350b00-0000-0000-0000-000000000002', '5e350900-0000-0000-0000-000000000002', 'B', 'Mengikuti latihan dengan baik; perlu meningkatkan kehadiran.')
) v(id, mid, pred, descr)
JOIN act_ecc_members m ON m.id = v.mid::uuid
ON CONFLICT DO NOTHING;

-- 5. Kegiatan sekolah ----------------------------------------------------------
INSERT INTO act_event_kinds (id, school_id, name) VALUES
  ('5e350c00-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Lomba'),
  ('5e350c00-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'Perkemahan'),
  ('5e350c00-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'Seminar'),
  ('5e350c00-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'Pentas Seni'),
  ('5e350c00-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'Kunjungan Edukasi')
ON CONFLICT DO NOTHING;

-- Semua kegiatan tanpa biaya/anggaran (disetujui otomatis oleh aplikasi).
INSERT INTO act_events (id, school_id, name, kind_id, description, starts_at, ends_at, location_text, target, quota,
    requires_parent_consent, consent_text, registration_opens_at, registration_closes_at, status, published_at, created_by, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.name, k.id, v.descr, v.st::timestamptz, v.en::timestamptz, v.loc,
       v.target::jsonb, v.quota, v.consent, v.ctext, v.ro::timestamptz, v.rc::timestamptz, v.status,
       CASE WHEN v.status IN ('published', 'completed') THEN v.ro::timestamptz END,
       '22222222-2222-2222-2222-222222222222', COALESCE(v.ro::timestamptz, v.st::timestamptz) - interval '7 days'
FROM (VALUES
  ('5e350d00-0000-0000-0000-000000000001', 'Class Meeting: Futsal & Tarik Tambang', 'Lomba', 'Lomba antarkelas setelah Sumatif Tengah Semester.',
   '2026-09-24 07:30+07', '2026-09-25 12:00+07', 'Lapangan utama sekolah', '{"grade_levels":[7,8,9],"classroom_ids":[],"include_staff":false}', NULL,
   false, NULL, '2026-09-14 07:00+07', '2026-09-21 23:59+07', 'completed'),
  ('5e350d00-0000-0000-0000-000000000002', 'Perkemahan Sabtu Minggu (Persami) Pramuka', 'Perkemahan', 'Persami penggalang di Bumi Perkemahan Cibubur.',
   '2026-11-07 07:00+07', '2026-11-08 15:00+07', 'Bumi Perkemahan Cibubur', '{"grade_levels":[7],"classroom_ids":[],"include_staff":false}', 120,
   true, 'Saya mengizinkan anak saya mengikuti Persami dan menginap di lokasi perkemahan.', '2026-10-05 07:00+07', '2026-10-30 23:59+07', 'published'),
  ('5e350d00-0000-0000-0000-000000000003', 'Seminar Literasi Digital & Anti-Perundungan', 'Seminar', 'Bersama narasumber dari Dinas Kominfo Kota Bekasi.',
   '2026-10-28 08:00+07', '2026-10-28 11:00+07', 'Aula sekolah', '{"grade_levels":[7,8],"classroom_ids":[],"include_staff":true}', 200,
   false, NULL, '2026-10-12 07:00+07', '2026-10-26 23:59+07', 'published'),
  ('5e350d00-0000-0000-0000-000000000004', 'Kunjungan Edukasi Museum Nasional', 'Kunjungan Edukasi', 'Belajar sejarah Nusantara langsung di museum.',
   '2026-11-20 07:00+07', '2026-11-20 15:00+07', 'Museum Nasional, Jakarta Pusat', '{"grade_levels":[7],"classroom_ids":["c1000000-0000-0000-0000-000000000001"],"include_staff":false}', 40,
   true, 'Saya mengizinkan anak saya mengikuti kunjungan ke luar sekolah dengan bus sekolah.', '2026-10-13 07:00+07', '2026-11-10 23:59+07', 'published'),
  ('5e350d00-0000-0000-0000-000000000005', 'Pentas Seni Akhir Tahun', 'Pentas Seni', 'Penampilan seni tari, musik, dan drama tiap kelas.',
   '2026-12-17 08:00+07', '2026-12-17 13:00+07', 'Aula sekolah', '{"grade_levels":[7,8,9],"classroom_ids":[],"include_staff":true}', NULL,
   false, NULL, NULL, NULL, 'draft'),
  ('5e350d00-0000-0000-0000-000000000006', 'Lomba Cerdas Cermat Antarkelas', 'Lomba', 'Dibatalkan karena bentrok dengan jadwal ANBK.',
   '2026-10-08 08:00+07', '2026-10-08 12:00+07', 'Ruang serbaguna', '{"grade_levels":[8],"classroom_ids":[],"include_staff":false}', 30,
   false, NULL, '2026-09-21 07:00+07', '2026-10-01 23:59+07', 'cancelled')
) v(id, name, kind, descr, st, en, loc, target, quota, consent, ctext, ro, rc, status)
JOIN act_event_kinds k ON k.school_id = '11111111-1111-1111-1111-111111111111' AND k.name = v.kind AND k.deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO act_event_committee (event_id, user_id, school_id, role_label)
SELECT v.eid::uuid, v.uid::uuid, '11111111-1111-1111-1111-111111111111', v.role
FROM (VALUES
  ('5e350d00-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222', 'ketua panitia'),
  ('5e350d00-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777', 'juri'),
  ('5e350d00-0000-0000-0000-000000000002', '88888888-8888-8888-8888-888888888888', 'ketua panitia'),
  ('5e350d00-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222', 'panitia'),
  ('5e350d00-0000-0000-0000-000000000003', '22222222-2222-2222-2222-222222222222', 'panitia'),
  ('5e350d00-0000-0000-0000-000000000004', '88888888-8888-8888-8888-888888888888', 'pendamping')
) v(eid, uid, role)
WHERE EXISTS (SELECT 1 FROM act_events e WHERE e.id = v.eid::uuid)
ON CONFLICT DO NOTHING;

-- Persetujuan orang tua: subject_id = id pendaftaran (event_registration)
-- atau id program kesehatan (health_program).
INSERT INTO parent_consents (id, school_id, subject_type, subject_id, student_id, status, decided_by_guardian_id, decided_at, note, requested_by, created_at)
VALUES
  ('5e350e00-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'event_registration', '5e350f00-0000-0000-0000-000000000002',
   '44444444-4444-4444-4444-444444444444', 'granted', '33333333-3333-3333-3333-333333333333', '2026-10-07 20:00+07', 'Mohon diingatkan membawa obat asma.', '44444444-4444-4444-4444-444444444444', '2026-10-06 16:00+07'),
  ('5e350e00-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'event_registration', '5e350f00-0000-0000-0000-000000000004',
   '44444444-4444-4444-4444-444444444444', 'pending', NULL, NULL, NULL, '44444444-4444-4444-4444-444444444444', '2026-10-14 15:00+07'),
  ('5e350e00-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'health_program', '5e351100-0000-0000-0000-000000000001',
   '44444444-4444-4444-4444-444444444444', 'granted', '33333333-3333-3333-3333-333333333333', '2026-10-09 19:30+07', NULL, '22222222-2222-2222-2222-222222222222', '2026-10-05 08:00+07'),
  ('5e350e00-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'health_program', '5e351100-0000-0000-0000-000000000003',
   '44444444-4444-4444-4444-444444444444', 'declined', '33333333-3333-3333-3333-333333333333', '2026-08-03 21:00+07', 'Sudah diperiksa di dokter gigi keluarga bulan lalu.', '22222222-2222-2222-2222-222222222222', '2026-07-28 08:00+07')
ON CONFLICT DO NOTHING;

INSERT INTO act_event_registrations (id, school_id, event_id, student_user_id, registered_by, registered_via, status, consent_id,
    consent_text_snapshot, attended_at, attendance_by, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.eid::uuid, '44444444-4444-4444-4444-444444444444', v.by::uuid, v.via, v.status,
       (SELECT c.id FROM parent_consents c WHERE c.id = v.cid::uuid), v.snap, v.att::timestamptz,
       CASE WHEN v.att IS NOT NULL THEN '22222222-2222-2222-2222-222222222222'::uuid END, v.at::timestamptz
FROM (VALUES
  ('5e350f00-0000-0000-0000-000000000001', '5e350d00-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444', 'siswa', 'confirmed', NULL, NULL, '2026-09-24 07:40+07', '2026-09-15 12:00+07'),
  ('5e350f00-0000-0000-0000-000000000002', '5e350d00-0000-0000-0000-000000000002', '44444444-4444-4444-4444-444444444444', 'siswa', 'confirmed', '5e350e00-0000-0000-0000-000000000001',
   'Saya mengizinkan anak saya mengikuti Persami dan menginap di lokasi perkemahan.', NULL, '2026-10-06 16:00+07'),
  ('5e350f00-0000-0000-0000-000000000003', '5e350d00-0000-0000-0000-000000000003', '33333333-3333-3333-3333-333333333333', 'orang_tua', 'confirmed', NULL, NULL, NULL, '2026-10-13 19:00+07'),
  ('5e350f00-0000-0000-0000-000000000004', '5e350d00-0000-0000-0000-000000000004', '44444444-4444-4444-4444-444444444444', 'siswa', 'pending_consent', '5e350e00-0000-0000-0000-000000000002',
   'Saya mengizinkan anak saya mengikuti kunjungan ke luar sekolah dengan bus sekolah.', NULL, '2026-10-14 15:00+07'),
  ('5e350f00-0000-0000-0000-000000000005', '5e350d00-0000-0000-0000-000000000006', '22222222-2222-2222-2222-222222222222', 'event_coordinator', 'cancelled', NULL, NULL, NULL, '2026-09-22 09:00+07')
) v(id, eid, by, via, status, cid, snap, att, at)
WHERE EXISTS (SELECT 1 FROM act_events e WHERE e.id = v.eid::uuid)
ON CONFLICT DO NOTHING;

-- 6. Program kesehatan UKS (tanpa hasil pemeriksaan: kolom hasil terenkripsi)
INSERT INTO health_programs (id, school_id, kind, name, description, starts_on, ends_on, requires_consent, consent_deadline, status, created_by, created_at) VALUES
  ('5e351100-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'immunization', 'Bulan Imunisasi Anak Sekolah (BIAS) Td',
   'Imunisasi Td bekerja sama dengan Puskesmas Bekasi Jaya.', '2026-11-09', '2026-11-13', true, '2026-11-02', 'published', '22222222-2222-2222-2222-222222222222', '2026-10-05 08:00+07'),
  ('5e351100-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'screening', 'Skrining Kesehatan Mata (Tajam Penglihatan)',
   'Pemeriksaan visus oleh petugas UKS untuk siswa baru.', '2026-08-10', '2026-08-14', false, NULL, 'completed', '22222222-2222-2222-2222-222222222222', '2026-07-27 08:00+07'),
  ('5e351100-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'screening', 'Pemeriksaan Gigi dan Mulut',
   'Pemeriksaan oleh dokter gigi Puskesmas.', '2026-08-17', '2026-08-21', true, '2026-08-10', 'completed', '22222222-2222-2222-2222-222222222222', '2026-07-28 08:00+07'),
  ('5e351100-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'screening', 'Pengukuran Tinggi & Berat Badan Semester Genap',
   'Pemantauan status gizi siswa.', '2027-01-18', '2027-01-22', false, NULL, 'draft', '22222222-2222-2222-2222-222222222222', '2026-10-12 08:00+07')
ON CONFLICT DO NOTHING;

INSERT INTO health_program_targets (id, school_id, program_id, target_kind, target_ref)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.pid::uuid, v.kind, v.ref
FROM (VALUES
  ('5e351200-0000-0000-0000-000000000001', '5e351100-0000-0000-0000-000000000001', 'grade_level', '7'),
  ('5e351200-0000-0000-0000-000000000002', '5e351100-0000-0000-0000-000000000002', 'classroom', 'c1000000-0000-0000-0000-000000000001'),
  ('5e351200-0000-0000-0000-000000000003', '5e351100-0000-0000-0000-000000000003', 'grade_level', '7'),
  ('5e351200-0000-0000-0000-000000000004', '5e351100-0000-0000-0000-000000000004', 'grade_level', '7'),
  ('5e351200-0000-0000-0000-000000000005', '5e351100-0000-0000-0000-000000000004', 'grade_level', '8')
) v(id, pid, kind, ref)
WHERE EXISTS (SELECT 1 FROM health_programs p WHERE p.id = v.pid::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO health_program_participants (id, school_id, program_id, student_id, consent_id, result_status, recorded_by, recorded_at, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.pid::uuid, '44444444-4444-4444-4444-444444444444',
       (SELECT c.id FROM parent_consents c WHERE c.id = v.cid::uuid), v.status, v.by::uuid, v.rec::timestamptz, v.at::timestamptz
FROM (VALUES
  ('5e351300-0000-0000-0000-000000000001', '5e351100-0000-0000-0000-000000000001', '5e350e00-0000-0000-0000-000000000003', 'not_recorded', NULL, NULL, '2026-10-05 08:00+07'),
  ('5e351300-0000-0000-0000-000000000002', '5e351100-0000-0000-0000-000000000002', NULL, 'absent', '22222222-2222-2222-2222-222222222222', '2026-08-14 12:00+07', '2026-07-27 08:00+07'),
  ('5e351300-0000-0000-0000-000000000003', '5e351100-0000-0000-0000-000000000003', '5e350e00-0000-0000-0000-000000000004', 'excluded', '22222222-2222-2222-2222-222222222222', '2026-08-17 08:00+07', '2026-07-28 08:00+07')
) v(id, pid, cid, status, by, rec, at)
WHERE EXISTS (SELECT 1 FROM health_programs p WHERE p.id = v.pid::uuid)
ON CONFLICT DO NOTHING;

-- 7. PKL -----------------------------------------------------------------------
-- Kelas VII A dipetakan ke jurusan RPL (kebutuhan demo penempatan PKL).
INSERT INTO classroom_majors (classroom_id, school_id, major_id)
SELECT 'c1000000-0000-0000-0000-000000000001', m.school_id, m.id FROM majors m
WHERE m.school_id = '11111111-1111-1111-1111-111111111111' AND m.code = 'RPL' AND m.deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO pkl_periods (id, school_id, academic_year_id, name, starts_on, ends_on, status, created_at) VALUES
  ('5e351400-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', NULL, 'PKL Gelombang 2 TA 2025/2026', '2026-02-02', '2026-04-30', 'closed', '2026-01-10 09:00+07'),
  ('5e351400-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', 'PKL Gelombang 1 TA 2026/2027', '2026-09-01', '2026-12-11', 'ongoing', '2026-08-01 09:00+07'),
  ('5e351400-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', 'PKL Gelombang 2 TA 2026/2027', '2027-02-01', '2027-04-30', 'planning', '2026-10-10 09:00+07')
ON CONFLICT DO NOTHING;

INSERT INTO pkl_schemes (id, school_id, period_id, industry_weight_bp, school_weight_bp, created_at) VALUES
  ('5e351500-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '5e351400-0000-0000-0000-000000000001', 6000, 4000, '2026-01-10 09:00+07'),
  ('5e351500-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '5e351400-0000-0000-0000-000000000002', 6000, 4000, '2026-08-01 09:00+07')
ON CONFLICT DO NOTHING;

INSERT INTO pkl_components (id, school_id, scheme_id, assessor, name, weight_bp, sort)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.sid::uuid, v.assessor, v.name, v.w, v.sort
FROM (VALUES
  ('5e351600-0000-0000-0000-000000000001', '5e351500-0000-0000-0000-000000000001', 'industry', 'Sikap kerja dan kedisiplinan', 4000, 1),
  ('5e351600-0000-0000-0000-000000000002', '5e351500-0000-0000-0000-000000000001', 'industry', 'Keterampilan teknis', 6000, 2),
  ('5e351600-0000-0000-0000-000000000003', '5e351500-0000-0000-0000-000000000001', 'school', 'Laporan akhir PKL', 6000, 3),
  ('5e351600-0000-0000-0000-000000000004', '5e351500-0000-0000-0000-000000000001', 'school', 'Presentasi hasil PKL', 4000, 4),
  ('5e351600-0000-0000-0000-000000000005', '5e351500-0000-0000-0000-000000000002', 'industry', 'Sikap kerja dan kedisiplinan', 4000, 1),
  ('5e351600-0000-0000-0000-000000000006', '5e351500-0000-0000-0000-000000000002', 'industry', 'Keterampilan teknis', 6000, 2),
  ('5e351600-0000-0000-0000-000000000007', '5e351500-0000-0000-0000-000000000002', 'school', 'Laporan akhir PKL', 6000, 3),
  ('5e351600-0000-0000-0000-000000000008', '5e351500-0000-0000-0000-000000000002', 'school', 'Presentasi hasil PKL', 4000, 4)
) v(id, sid, assessor, name, w, sort)
WHERE EXISTS (SELECT 1 FROM pkl_schemes s WHERE s.id = v.sid::uuid)
ON CONFLICT DO NOTHING;

-- Mitra PKL dicari berdasarkan nama (baris tertua bila ada duplikat dari seed 05)
CREATE TEMP TABLE _seed35_partner ON COMMIT DROP AS
SELECT DISTINCT ON (name) name, id FROM pkl_partners
WHERE school_id = '11111111-1111-1111-1111-111111111111' AND deleted_at IS NULL
  AND name IN ('PT Telkom Indonesia (Witel Bekasi)', 'PT Astra Graphia Information Technology')
ORDER BY name, created_at, id;

CREATE TEMP TABLE _seed35_major ON COMMIT DROP AS
SELECT code, id FROM majors WHERE school_id = '11111111-1111-1111-1111-111111111111' AND deleted_at IS NULL AND code IN ('RPL', 'TKJ');

INSERT INTO pkl_partner_mous (id, school_id, partner_id, mou_number, valid_from, valid_to, uploaded_by, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', p.id, v.no, v.vf::date, v.vt::date, '22222222-2222-2222-2222-222222222222', v.at::timestamptz
FROM (VALUES
  ('5e351700-0000-0000-0000-000000000001', 'PT Telkom Indonesia (Witel Bekasi)', '045/MOU/SMPT-TLK/I/2026', '2026-01-05', '2028-01-04', '2026-01-05 10:00+07'),
  ('5e351700-0000-0000-0000-000000000002', 'PT Astra Graphia Information Technology', '046/MOU/SMPT-AGIT/VII/2026', '2026-07-15', '2027-07-14', '2026-07-15 10:00+07')
) v(id, partner, no, vf, vt, at)
JOIN _seed35_partner p ON p.name = v.partner
ON CONFLICT DO NOTHING;

INSERT INTO pkl_quotas (id, school_id, partner_id, period_id, major_id, quota)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', p.id, v.per::uuid, m.id, v.q
FROM (VALUES
  ('5e351800-0000-0000-0000-000000000001', 'PT Telkom Indonesia (Witel Bekasi)', '5e351400-0000-0000-0000-000000000001', 'RPL', 4),
  ('5e351800-0000-0000-0000-000000000002', 'PT Telkom Indonesia (Witel Bekasi)', '5e351400-0000-0000-0000-000000000002', 'RPL', 5),
  ('5e351800-0000-0000-0000-000000000003', 'PT Telkom Indonesia (Witel Bekasi)', '5e351400-0000-0000-0000-000000000002', 'TKJ', 3),
  ('5e351800-0000-0000-0000-000000000004', 'PT Astra Graphia Information Technology', '5e351400-0000-0000-0000-000000000002', 'RPL', 6),
  ('5e351800-0000-0000-0000-000000000005', 'PT Astra Graphia Information Technology', '5e351400-0000-0000-0000-000000000003', 'RPL', 6)
) v(id, partner, per, major, q)
JOIN _seed35_partner p ON p.name = v.partner
JOIN _seed35_major m ON m.code = v.major
WHERE EXISTS (SELECT 1 FROM pkl_periods x WHERE x.id = v.per::uuid)
ON CONFLICT DO NOTHING;

-- Penempatan Budi: selesai (periode lalu), aktif (periode berjalan), dibatalkan.
INSERT INTO pkl_placements (id, school_id, period_id, partner_id, major_id, student_id, school_mentor_user_id, starts_on, ends_on,
    status, cancel_reason, created_by, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.per::uuid, p.id, m.id, '44444444-4444-4444-4444-444444444444',
       '22222222-2222-2222-2222-222222222222', v.st::date, v.en::date, v.status, v.reason, '22222222-2222-2222-2222-222222222222', v.at::timestamptz
FROM (VALUES
  ('5e351900-0000-0000-0000-000000000001', '5e351400-0000-0000-0000-000000000001', 'PT Telkom Indonesia (Witel Bekasi)', '2026-02-02', '2026-04-30', 'completed', NULL, '2026-01-20 09:00+07'),
  ('5e351900-0000-0000-0000-000000000002', '5e351400-0000-0000-0000-000000000002', 'PT Telkom Indonesia (Witel Bekasi)', '2026-09-01', '2026-12-11', 'cancelled',
   'Mitra meminta pengurangan kuota; siswa dipindahkan ke mitra lain.', '2026-08-10 09:00+07'),
  ('5e351900-0000-0000-0000-000000000003', '5e351400-0000-0000-0000-000000000002', 'PT Astra Graphia Information Technology', '2026-09-01', '2026-12-11', 'active', NULL, '2026-08-20 09:00+07')
) v(id, per, partner, st, en, status, reason, at)
JOIN _seed35_partner p ON p.name = v.partner
JOIN _seed35_major m ON m.code = 'RPL'
WHERE EXISTS (SELECT 1 FROM pkl_periods x WHERE x.id = v.per::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO pkl_attendance (id, school_id, placement_id, work_date, status, check_in_at, check_out_at, note, confirmation, confirmed_by, confirmed_at, reject_reason)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '5e351900-0000-0000-0000-000000000003', v.d::date, v.status,
       CASE WHEN v.status = 'hadir' THEN (v.d || ' 07:55+07')::timestamptz END,
       CASE WHEN v.status = 'hadir' THEN (v.d || ' 16:05+07')::timestamptz END,
       v.note, v.conf, CASE WHEN v.conf <> 'pending' THEN '22222222-2222-2222-2222-222222222222'::uuid END,
       CASE WHEN v.conf <> 'pending' THEN (v.d || ' 18:00+07')::timestamptz END, v.rej
FROM (VALUES
  ('5e351a00-0000-0000-0000-000000000001', '2026-10-05', 'hadir', NULL, 'confirmed', NULL),
  ('5e351a00-0000-0000-0000-000000000002', '2026-10-06', 'hadir', NULL, 'confirmed', NULL),
  ('5e351a00-0000-0000-0000-000000000003', '2026-10-07', 'sakit', 'Demam, ada surat dokter.', 'confirmed', NULL),
  ('5e351a00-0000-0000-0000-000000000004', '2026-10-08', 'izin', 'Acara keluarga.', 'rejected', 'Izin tidak diajukan sebelumnya ke pembimbing industri.'),
  ('5e351a00-0000-0000-0000-000000000005', '2026-10-14', 'hadir', NULL, 'pending', NULL)
) v(id, d, status, note, conf, rej)
WHERE EXISTS (SELECT 1 FROM pkl_placements x WHERE x.id = '5e351900-0000-0000-0000-000000000003')
ON CONFLICT DO NOTHING;

INSERT INTO pkl_journals (id, school_id, placement_id, entry_date, activity, mentor_note, mentor_note_by, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '5e351900-0000-0000-0000-000000000003', v.d::date, v.act, v.note,
       CASE WHEN v.note IS NOT NULL THEN '22222222-2222-2222-2222-222222222222'::uuid END, (v.d || ' 16:30+07')::timestamptz
FROM (VALUES
  ('5e351b00-0000-0000-0000-000000000001', '2026-10-05', 'Orientasi tim dan pengenalan alur kerja helpdesk TI.', 'Bagus, catat istilah teknis yang baru dipelajari.'),
  ('5e351b00-0000-0000-0000-000000000002', '2026-10-06', 'Membantu instalasi ulang 5 laptop karyawan.', NULL),
  ('5e351b00-0000-0000-0000-000000000003', '2026-10-09', 'Membuat dokumentasi inventaris perangkat jaringan.', 'Rapi. Lengkapi dengan foto label perangkat.'),
  ('5e351b00-0000-0000-0000-000000000004', '2026-10-14', 'Belajar membuat halaman web sederhana dengan HTML dan CSS.', NULL)
) v(id, d, act, note)
WHERE EXISTS (SELECT 1 FROM pkl_placements x WHERE x.id = '5e351900-0000-0000-0000-000000000003')
ON CONFLICT DO NOTHING;

-- Nilai & hasil penempatan yang sudah selesai:
-- industri 0.4*85 + 0.6*90 = 88.00; sekolah 0.6*80 + 0.4*88 = 83.20;
-- akhir 0.6*88.00 + 0.4*83.20 = 86.08
INSERT INTO pkl_scores (id, school_id, placement_id, component_id, score_x100, scored_by, scored_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '5e351900-0000-0000-0000-000000000001', v.cid::uuid, v.s,
       '22222222-2222-2222-2222-222222222222', v.at::timestamptz
FROM (VALUES
  ('5e351c00-0000-0000-0000-000000000001', '5e351600-0000-0000-0000-000000000001', 8500, '2026-04-28 10:00+07'),
  ('5e351c00-0000-0000-0000-000000000002', '5e351600-0000-0000-0000-000000000002', 9000, '2026-04-28 10:00+07'),
  ('5e351c00-0000-0000-0000-000000000003', '5e351600-0000-0000-0000-000000000003', 8000, '2026-05-06 10:00+07'),
  ('5e351c00-0000-0000-0000-000000000004', '5e351600-0000-0000-0000-000000000004', 8800, '2026-05-06 10:00+07')
) v(id, cid, s, at)
WHERE EXISTS (SELECT 1 FROM pkl_placements x WHERE x.id = '5e351900-0000-0000-0000-000000000001')
  AND EXISTS (SELECT 1 FROM pkl_components c WHERE c.id = v.cid::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO pkl_results (placement_id, school_id, industry_x100, school_x100, final_x100, computed_at, finalized_by, finalized_at)
SELECT '5e351900-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 8800, 8320, 8608,
       '2026-05-06 10:05+07', '22222222-2222-2222-2222-222222222222', '2026-05-08 09:00+07'
WHERE EXISTS (SELECT 1 FROM pkl_placements x WHERE x.id = '5e351900-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

COMMIT;
