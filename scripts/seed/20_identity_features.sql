-- 20_identity_features.sql
-- Data demo identity untuk fitur yang masih kosong di server dan tabel baru
-- QA-IDN (tata kelola, data referensi, matriks izin, permintaan data pribadi,
-- akses darurat, migrasi data lama, arsip, pengumuman ber-audiens & terjadwal).
--
-- Aturan:
--   * Hanya MENAMBAH data: UUID tetap berawalan 'a20', ON CONFLICT DO NOTHING
--     atau INSERT ... SELECT ... WHERE EXISTS/NOT EXISTS. Tidak ada UPDATE/DELETE.
--   * Idempoten dan satu transaksi (BEGIN/COMMIT, jalankan dengan ON_ERROR_STOP=1).
--   * Baris yang merujuk pengguna di-JOIN ke users, jadi pengguna yang tidak ada
--     di server membuat baris itu dilewati, bukan menggagalkan file.
--   * MFA tidak diaktifkan untuk akun demo (mfa_secret_enc/mfa_enabled_at tidak
--     disentuh; mfa_challenges, user_sessions, password_reset_tokens dibiarkan kosong).
--
-- Pengguna acuan (scripts/seed/identity.sql):
--   2222.. Admin Teratai, 3333.. Wali Santoso, 4444.. Budi Santoso (siswa),
--   5555.. Guru Matematika, 6666.. Wali Kelas VII A, 7777.. Kepala Sekolah,
--   8888.. Kepala Yayasan, 9999.. Pengawas Ujian.

BEGIN;

-- ---------------------------------------------------------------------------
-- 00. Tahun akademik berjalan: pastikan sekolah Teratai punya tepat satu
--     is_current. Hanya INSERT bila sekolah BELUM punya tahun berjalan dan
--     label 2026/2027 belum ada; baris yang sudah ada tidak di-UPDATE.
--     (schools.current_academic_year_id tidak disentuh.)
-- ---------------------------------------------------------------------------
INSERT INTO academic_years (id, school_id, label, is_current, start_date, end_date)
SELECT 'a2000000-0000-0000-0000-0000000000d1', s.id, '2026/2027', true, DATE '2026-07-13', DATE '2027-06-30'
FROM schools s
WHERE s.id = '11111111-1111-1111-1111-111111111111'
  AND NOT EXISTS (SELECT 1 FROM academic_years y
                  WHERE y.school_id = s.id AND y.is_current AND y.deleted_at IS NULL)
  AND NOT EXISTS (SELECT 1 FROM academic_years y
                  WHERE y.school_id = s.id AND y.label = '2026/2027' AND y.deleted_at IS NULL)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 0. Akun auditor demo (UUID tetap) untuk masa akses auditor di core.
--    Password tidak dapat dipakai masuk (hash tidak valid) -- akun ini hanya
--    subjek data demo. Peran auditor tidak bentrok dengan peran lain karena
--    akun ini hanya memegang satu peran.
-- ---------------------------------------------------------------------------
INSERT INTO users (id, name, email, password_hash, password_changed, is_superadmin)
SELECT 'a2000000-0000-0000-0000-0000000000a1', 'Rina Kartika (Auditor Eksternal)', 'auditor.demo@teratai.local',
       '!akun-demo-tidak-dapat-login', true, false
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

INSERT INTO user_school_roles (id, user_id, school_id, role, status)
SELECT 'a2000000-0000-0000-0000-0000000000a2', u.id, '11111111-1111-1111-1111-111111111111', 'auditor', 'active'
FROM users u WHERE u.id = 'a2000000-0000-0000-0000-0000000000a1'
ON CONFLICT DO NOTHING;

-- User demo transport: pengemudi + pendamping_transport. Hash password disalin
-- dari user demo admin@teratai.sch.id (tidak membuat hash baru); dilewati bila
-- user admin demo belum ada.
INSERT INTO users (id, name, email, password_hash, password_changed, is_superadmin)
SELECT v.id::uuid, v.name, v.email, src.password_hash, true, false
FROM (VALUES
  ('a2000000-0000-0000-0000-0000000000b1', 'Pak Darmawan (Pengemudi Bus Sekolah)', 'pengemudi@teratai.sch.id'),
  ('a2000000-0000-0000-0000-0000000000c1', 'Ibu Sulastri (Pendamping Transport)', 'pendamping@teratai.sch.id')
) AS v(id, name, email)
JOIN users src ON src.email = 'admin@teratai.sch.id'
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

INSERT INTO user_school_roles (id, user_id, school_id, role, status)
SELECT v.id::uuid, u.id, '11111111-1111-1111-1111-111111111111', v.role, 'active'
FROM (VALUES
  ('a2000000-0000-0000-0000-0000000000b2', 'a2000000-0000-0000-0000-0000000000b1', 'pengemudi'),
  ('a2000000-0000-0000-0000-0000000000c2', 'a2000000-0000-0000-0000-0000000000c1', 'pendamping_transport')
) AS v(id, user_id, role)
JOIN users u ON u.id = v.user_id::uuid
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 1. Data Jabatan / Peran + Organisasi Sekolah
-- ---------------------------------------------------------------------------
INSERT INTO positions (id, school_id, name, priority)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.name, v.priority
FROM (VALUES
  ('a2000001-0000-0000-0000-000000000001', 'Kepala Sekolah', 1),
  ('a2000001-0000-0000-0000-000000000002', 'Wakil Kepala Sekolah Bidang Kurikulum', 2),
  ('a2000001-0000-0000-0000-000000000003', 'Wakil Kepala Sekolah Bidang Kesiswaan', 3),
  ('a2000001-0000-0000-0000-000000000004', 'Bendahara Sekolah', 4),
  ('a2000001-0000-0000-0000-000000000005', 'Wali Kelas 7A', 10),
  ('a2000001-0000-0000-0000-000000000006', 'Koordinator Ujian', 11),
  ('a2000001-0000-0000-0000-000000000007', 'Operator Sekolah', 20)
) AS v(id, name, priority)
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

INSERT INTO position_assignments (id, position_id, user_id, school_id)
SELECT v.id::uuid, p.id, u.id, '11111111-1111-1111-1111-111111111111'
FROM (VALUES
  ('a2000002-0000-0000-0000-000000000001', 'a2000001-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777'),
  ('a2000002-0000-0000-0000-000000000002', 'a2000001-0000-0000-0000-000000000002', '55555555-5555-5555-5555-555555555555'),
  ('a2000002-0000-0000-0000-000000000003', 'a2000001-0000-0000-0000-000000000003', '66666666-6666-6666-6666-666666666666'),
  ('a2000002-0000-0000-0000-000000000004', 'a2000001-0000-0000-0000-000000000005', '66666666-6666-6666-6666-666666666666'),
  ('a2000002-0000-0000-0000-000000000005', 'a2000001-0000-0000-0000-000000000006', '99999999-9999-9999-9999-999999999999'),
  ('a2000002-0000-0000-0000-000000000006', 'a2000001-0000-0000-0000-000000000007', '22222222-2222-2222-2222-222222222222')
) AS v(id, position_id, user_id)
JOIN positions p ON p.id = v.position_id::uuid
JOIN users u ON u.id = v.user_id::uuid
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2. Informasi (pengumuman & acara) dengan audiens wajib dan jadwal terbit
-- ---------------------------------------------------------------------------
INSERT INTO announcements (id, school_id, type, title, message, target_roles, location, url, jenis,
                           published, audience_all, target_school_ids, publish_at, created_by, created_at, updated_at)
SELECT v.id::uuid, v.school_id::uuid, v.type, v.title, v.message, v.target_roles::text[], v.location, v.url, v.jenis,
       v.published, v.audience_all, v.target_school_ids::uuid[], v.publish_at,
       (SELECT id FROM users WHERE id = '22222222-2222-2222-2222-222222222222'), v.created_at, v.created_at
FROM (VALUES
  ('a2000003-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'pengumuman',
   'Libur Peringatan Maulid Nabi Muhammad SAW',
   'Kegiatan belajar mengajar diliburkan pada hari Kamis. Siswa kembali masuk sekolah pada hari Jumat pukul 07.00 WIB.',
   '{}', NULL, NULL, 'umum', true, true, '{}', NULL::timestamptz, now() - interval '10 days'),
  ('a2000003-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'pengumuman',
   'Jadwal Penilaian Tengah Semester Ganjil 2026/2027',
   'Penilaian Tengah Semester dilaksanakan berbasis komputer (CBT). Siswa wajib membawa kartu peserta dan hadir 30 menit sebelum sesi dimulai.',
   '{siswa,orang_tua}', NULL, NULL, 'ujian', true, false, '{}', NULL::timestamptz, now() - interval '6 days'),
  ('a2000003-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'event',
   'Pentas Seni dan Gelar Karya P5',
   'Siswa kelas 7 dan 8 menampilkan hasil Projek Penguatan Profil Pelajar Pancasila bertema Kearifan Lokal. Orang tua diundang hadir.',
   '{siswa,orang_tua,guru}', 'Aula Utama Sekolah Teratai', 'https://terataiputih.sch.id/pentas-seni', NULL, true, false, '{}', NULL::timestamptz, now() - interval '3 days'),
  ('a2000003-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'pengumuman',
   'Rapat Koordinasi Guru Persiapan Asesmen Sumatif',
   'Draf: rapat koordinasi seluruh guru mata pelajaran untuk menyelaraskan kisi-kisi asesmen sumatif akhir semester.',
   '{guru,walikelas}', NULL, NULL, 'umum', false, false, '{}', NULL::timestamptz, now() - interval '1 day'),
  ('a2000003-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'pengumuman',
   'Pengambilan Rapor Tengah Semester oleh Orang Tua',
   'Rapor tengah semester dapat diambil orang tua/wali di ruang kelas masing-masing. Mohon membawa bukti klaim anak di portal orang tua.',
   '{orang_tua}', NULL, NULL, 'umum', true, false, '{}', now() + interval '7 days', now() - interval '2 hours'),
  ('a2000003-0000-0000-0000-000000000006', NULL, 'pengumuman',
   'Pemeliharaan Sistem Sekoci Hari Minggu',
   'Layanan Sekoci tidak dapat diakses pada hari Minggu pukul 00.00-04.00 WIB karena pemeliharaan server terjadwal.',
   '{}', NULL, NULL, 'umum', true, true, '{11111111-1111-1111-1111-111111111111}', NULL::timestamptz, now() - interval '5 days')
) AS v(id, school_id, type, title, message, target_roles, location, url, jenis, published, audience_all,
       target_school_ids, publish_at, created_at)
WHERE v.school_id IS NULL OR EXISTS (SELECT 1 FROM schools s WHERE s.id = v.school_id::uuid)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Aplikasi white-label: status listing toko aplikasi
-- ---------------------------------------------------------------------------
INSERT INTO app_store_listings (id, school_id, store, account_owner, package_id, status, listing_url, note, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.store, v.owner, v.pkg, v.status, v.url, v.note, now() - interval '20 days', now() - interval '2 days'
FROM (VALUES
  ('a2000004-0000-0000-0000-000000000001', 'google_play', 'fullcircle', 'id.sch.terataiputih.app', 'published',
   'https://play.google.com/store/apps/details?id=id.sch.terataiputih.app', 'Versi 1.2.0 terbit; rating awal 4,6.'),
  ('a2000004-0000-0000-0000-000000000002', 'app_store', 'client', 'id.sch.terataiputih.app', 'in_review',
   NULL, 'Menunggu review Apple; akun developer atas nama Yayasan Teratai Putih.')
) AS v(id, store, owner, pkg, status, url, note)
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 4. Klaim anak: riwayat kode klaim Budi (terpakai, kedaluwarsa, dicabut).
--    Hash sengaja tidak cocok dengan kode apa pun.
-- ---------------------------------------------------------------------------
INSERT INTO guardian_claim_codes (id, school_id, student_user_id, code_hash, expires_at, used_at, used_by, created_by, created_at, revoked_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', s.id, v.hash, v.expires_at, v.used_at,
       (SELECT id FROM users WHERE id = v.used_by::uuid), (SELECT id FROM users WHERE id = '22222222-2222-2222-2222-222222222222'),
       v.created_at, v.revoked_at
FROM (VALUES
  ('a2000005-0000-0000-0000-000000000001', 'seed-demo:tidak-berlaku-1', now() - interval '83 days', now() - interval '89 days',
   '33333333-3333-3333-3333-333333333333', now() - interval '90 days', NULL::timestamptz),
  ('a2000005-0000-0000-0000-000000000002', 'seed-demo:tidak-berlaku-2', now() - interval '33 days', NULL::timestamptz,
   NULL, now() - interval '40 days', NULL::timestamptz),
  ('a2000005-0000-0000-0000-000000000003', 'seed-demo:tidak-berlaku-3', now() - interval '8 days', NULL::timestamptz,
   NULL, now() - interval '15 days', now() - interval '14 days')
) AS v(id, hash, expires_at, used_at, used_by, created_at, revoked_at)
JOIN users s ON s.id = '44444444-4444-4444-4444-444444444444'
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 5. Konversi PPDB -> akun (cocok dengan student_conversions di 23_admission)
-- ---------------------------------------------------------------------------
INSERT INTO admission_conversion_mappings (id, school_id, source, conversion_ref, guardian_user_id, student_user_id, guardian_link_id, created_at, updated_at)
SELECT 'a2000006-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'admission',
       'a2300019-0000-0000-0000-000000000001', g.id, s.id,
       (SELECT gl.id FROM guardian_links gl WHERE gl.school_id = '11111111-1111-1111-1111-111111111111'
          AND gl.guardian_user_id = g.id AND gl.student_user_id = s.id LIMIT 1),
       now() - interval '95 days', now() - interval '95 days'
FROM users g, users s
WHERE g.id = '33333333-3333-3333-3333-333333333333' AND s.id = '44444444-4444-4444-4444-444444444444'
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 6. Impor template lama: baris yang menunggu dikirim ke layanan lain
-- ---------------------------------------------------------------------------
INSERT INTO import_staged_rows (id, school_id, kind, payload, dispatched, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.kind, v.payload::jsonb, v.dispatched, now() - v.age
FROM (VALUES
  ('a2000007-0000-0000-0000-000000000001', 'kelas', '{"nama_kelas":"Kelas 7C","tingkat":"7","wali_kelas":"Guru Matematika","kapasitas":32}', true, interval '60 days'),
  ('a2000007-0000-0000-0000-000000000002', 'mata_pelajaran', '{"nama":"Seni Budaya","kelompok":"umum","kkm":75}', true, interval '60 days'),
  ('a2000007-0000-0000-0000-000000000003', 'orang_tua', '{"nama":"Siti Rahmawati","hubungan":"ibu","nisn_anak":"1234567890","no_hp":"081234500011"}', false, interval '5 days'),
  ('a2000007-0000-0000-0000-000000000004', 'ekstrakurikuler', '{"nama":"Pramuka Penggalang","pembina":"Wali Kelas VII A","hari":"Jumat"}', false, interval '5 days'),
  ('a2000007-0000-0000-0000-000000000005', 'ekstrakurikuler', '{"nama":"Robotik","pembina":"Guru Matematika","hari":"Rabu"}', false, interval '5 days')
) AS v(id, kind, payload, dispatched, age)
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 7. Status siswa: riwayat Budi (calon -> aktif)
-- ---------------------------------------------------------------------------
INSERT INTO student_status_history (id, user_id, school_id, from_status, to_status, reason, correction, changed_by, changed_at)
SELECT v.id::uuid, u.id, '11111111-1111-1111-1111-111111111111', v.from_status, v.to_status, v.reason, v.correction,
       '22222222-2222-2222-2222-222222222222', now() - v.age
FROM (VALUES
  ('a2000008-0000-0000-0000-000000000001', NULL, 'calon', 'Dikonversi dari PPDB Gelombang 1 2026/2027', false, interval '100 days'),
  ('a2000008-0000-0000-0000-000000000002', 'calon', 'aktif', 'Daftar ulang dan pembayaran uang pangkal lunas', false, interval '95 days')
) AS v(id, from_status, to_status, reason, correction, age)
JOIN users u ON u.id = '44444444-4444-4444-4444-444444444444'
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 8. Data Referensi: program/jurusan unit (nasional sudah diisi migrasi 0022)
-- ---------------------------------------------------------------------------
INSERT INTO reference_values (id, school_id, category, code, label, sort_order, active)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.category, v.code, v.label, v.sort_order, v.active
FROM (VALUES
  ('a2000009-0000-0000-0000-000000000001', 'program', 'reguler', 'Reguler', 1, true),
  ('a2000009-0000-0000-0000-000000000002', 'program', 'bilingual', 'Kelas Bilingual', 2, true),
  ('a2000009-0000-0000-0000-000000000003', 'program', 'tahfidz', 'Kelas Tahfidz', 3, true),
  ('a2000009-0000-0000-0000-000000000004', 'program', 'olimpiade', 'Kelas Olimpiade Sains', 4, true),
  ('a2000009-0000-0000-0000-000000000005', 'program', 'akselerasi', 'Akselerasi (tidak dibuka lagi)', 9, false),
  ('a2000009-0000-0000-0000-000000000006', 'pekerjaan_ortu', 'guru_dosen', 'Guru/Dosen', 15, true)
) AS v(id, category, code, label, sort_order, active)
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 9. Matriks izin: hanya MENAMBAH hak (tidak mencabut hak bawaan) untuk unit.
-- ---------------------------------------------------------------------------
INSERT INTO role_permissions (id, school_id, role, module, can_view, can_create, can_edit, can_delete, can_approve, updated_by, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.role, v.module, v.cv, v.cc, v.ce, v.cd, v.ca,
       (SELECT id FROM users WHERE email = 'superadmin@sekoci.local'), now() - interval '7 days'
FROM (VALUES
  ('a2000010-0000-0000-0000-000000000001', 'kepala_sekolah', 'announcements', true, true, true, false, true),
  ('a2000010-0000-0000-0000-000000000002', 'kepala_sekolah', 'guardians', true, false, false, false, false),
  ('a2000010-0000-0000-0000-000000000003', 'kepala_sekolah', 'privacy_requests', true, false, false, false, false),
  ('a2000010-0000-0000-0000-000000000004', 'kepala_yayasan', 'privacy_requests', true, false, false, false, false),
  ('a2000010-0000-0000-0000-000000000005', 'walikelas', 'guardians', true, false, false, false, false)
) AS v(id, role, module, cv, cc, ce, cd, ca)
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 10. Pengecualian SoD (BUG-023). Tidak ada yang berstatus approved, jadi
--     tidak ada peran yang bertambah. Pengajuan pending terhubung ke
--     approval core a2100003-...-001 (lihat 21_core_features.sql).
-- ---------------------------------------------------------------------------
INSERT INTO role_conflict_exceptions (id, school_id, user_id, role, conflicting_roles, reason, valid_until, status, approval_id,
                                      status_message, requested_by, decided_by, decision_comment, decided_at, revoked_by, revoked_at,
                                      created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', u.id, v.role, v.conflicting::text[], v.reason, v.valid_until::date,
       v.status, v.approval_id, v.status_message,
       (SELECT id FROM users WHERE id = '22222222-2222-2222-2222-222222222222'),
       v.decided_by, v.decision_comment, v.decided_at,
       CASE WHEN v.status = 'revoked' THEN (SELECT id FROM users WHERE id = '22222222-2222-2222-2222-222222222222') END,
       v.revoked_at, v.created_at, COALESCE(v.decided_at, v.created_at)
FROM (VALUES
  ('a2000011-0000-0000-0000-000000000001', 'hrmanager@teratai.sch.id', 'payroll_officer', '{hr_manager}',
   'Petugas payroll cuti melahirkan tiga bulan; HR Manager merangkap sementara pemrosesan gaji dengan pengawasan auditor.',
   (CURRENT_DATE + 60)::text, 'pending', 'a2100003-0000-0000-0000-000000000001', NULL,
   NULL, NULL, NULL::timestamptz, NULL::timestamptz, now() - interval '1 day'),
  ('a2000011-0000-0000-0000-000000000002', 'payroll@teratai.sch.id', 'finance_manager', '{payroll_officer}',
   'Permintaan merangkap penyetuju pembayaran selama audit tahunan karena manajer keuangan sedang dinas luar.',
   (CURRENT_DATE - 10)::text, 'rejected', 'a2100003-0000-0000-0000-000000000002', NULL,
   '77777777-7777-7777-7777-777777777777', 'Ditolak: pembuat dan penyetuju gaji wajib dipisah. Gunakan delegasi approval.',
   now() - interval '25 days', NULL::timestamptz, now() - interval '27 days'),
  ('a2000011-0000-0000-0000-000000000003', 'financemanager@teratai.sch.id', 'payroll_officer', '{finance_manager}',
   'Masa transisi sistem penggajian baru; manajer keuangan membantu input komponen gaji selama dua minggu.',
   (CURRENT_DATE - 40)::text, 'revoked', NULL, NULL,
   '77777777-7777-7777-7777-777777777777', 'Disetujui dengan masa berlaku dua minggu.',
   now() - interval '70 days', now() - interval '45 days', now() - interval '72 days'),
  ('a2000011-0000-0000-0000-000000000004', 'recruiter@teratai.sch.id', 'hr_manager', '{auditor}',
   'Pengajuan otomatis dari impor peran lama; perlu dikaji ulang setelah layanan Approval Center pulih.',
   NULL, 'failed', NULL, 'Pengajuan tersimpan tetapi belum terkirim ke Approval Center. Layanan Approval Center tidak dapat dihubungi.',
   NULL, NULL, NULL::timestamptz, NULL::timestamptz, now() - interval '3 days')
) AS v(id, email, role, conflicting, reason, valid_until, status, approval_id, status_message,
       decided_by, decision_comment, decided_at, revoked_at, created_at)
JOIN users u ON u.email = v.email
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
  AND NOT EXISTS (SELECT 1 FROM role_conflict_exceptions e
                  WHERE e.user_id = u.id AND e.role = v.role AND e.status IN ('pending', 'approved') AND v.status IN ('pending', 'approved'))
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 11. Akses darurat (break-glass): hanya riwayat yang sudah berakhir.
-- ---------------------------------------------------------------------------
INSERT INTO break_glass_grants (id, school_id, user_id, role, reason, incident_ref, granted_by, starts_at, expires_at,
                                role_row_created, ended_at, end_reason, ended_by, summary, summary_sent_at, summary_status, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', u.id, v.role, v.reason, v.incident_ref,
       (SELECT id FROM users WHERE id = '88888888-8888-8888-8888-888888888888'),
       v.starts_at, v.starts_at + v.dur, true, v.ended_at, v.end_reason,
       CASE WHEN v.end_reason = 'revoked' THEN (SELECT id FROM users WHERE id = '88888888-8888-8888-8888-888888888888') END,
       jsonb_build_object('started_at', v.starts_at, 'ended_at', v.ended_at, 'successful_logins', v.ok,
                          'failed_logins', 0, 'sessions_opened', v.ok, 'ip_addresses', jsonb_build_array('10.10.1.25'),
                          'end_reason', v.end_reason),
       v.ended_at + interval '1 minute', 'sent', v.starts_at
FROM (VALUES
  ('a2000012-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777', 'admin',
   'Admin sekolah sakit saat batas unggah nilai rapor; kepala sekolah perlu membuka kunci periode nilai.',
   'INC-2026-014', now() - interval '40 days', interval '8 hours', now() - interval '40 days' + interval '8 hours', 'expired', 2),
  ('a2000012-0000-0000-0000-000000000002', '55555555-5555-5555-5555-555555555555', 'admin',
   'Perbaikan data siswa ganda menjelang ANBK; operator sekolah tidak dapat dihubungi pada akhir pekan.',
   'INC-2026-021', now() - interval '18 days', interval '12 hours', now() - interval '18 days' + interval '3 hours', 'revoked', 1),
  ('a2000012-0000-0000-0000-000000000003', '77777777-7777-7777-7777-777777777777', 'finance_manager',
   'Persetujuan darurat refund biaya study tour yang dibatalkan karena cuaca buruk, manajer keuangan sedang cuti.',
   'INC-2026-030', now() - interval '6 days', interval '4 hours', now() - interval '6 days' + interval '4 hours', 'expired', 1)
) AS v(id, user_id, role, reason, incident_ref, starts_at, dur, ended_at, end_reason, ok)
JOIN users u ON u.id = v.user_id::uuid
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 12. Permintaan data pribadi (UU PDP). Permintaan penghapusan hanya
--     berstatus ditolak supaya tidak ada akun demo yang dianonimkan.
-- ---------------------------------------------------------------------------
INSERT INTO data_subject_requests (id, school_id, subject_user_id, request_type, details, corrections, status, requested_by,
                                   requested_at, due_at, handled_by, handled_at, resolution, export_payload, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', u.id, v.rtype, v.details, v.corrections::jsonb, v.status,
       u.id, v.requested_at, v.requested_at + interval '72 hours',
       CASE WHEN v.status <> 'submitted' THEN (SELECT id FROM users WHERE id = '22222222-2222-2222-2222-222222222222') END,
       v.handled_at, v.resolution, v.export_payload::jsonb, COALESCE(v.handled_at, v.requested_at)
FROM (VALUES
  ('a2000013-0000-0000-0000-000000000001', '33333333-3333-3333-3333-333333333333', 'access',
   'Saya ingin mengetahui data pribadi saya dan anak yang disimpan sekolah.', NULL, 'submitted',
   now() - interval '20 hours', NULL::timestamptz, NULL, NULL),
  ('a2000013-0000-0000-0000-000000000002', '33333333-3333-3333-3333-333333333333', 'correction',
   'Nomor HP saya sudah berganti, mohon diperbarui agar notifikasi tagihan sampai.', '{"phone":"081298765432"}', 'in_review',
   now() - interval '2 days', NULL::timestamptz, NULL, NULL),
  ('a2000013-0000-0000-0000-000000000003', '55555555-5555-5555-5555-555555555555', 'access',
   'Mohon salinan data kepegawaian dan riwayat login akun saya.', NULL, 'completed',
   now() - interval '30 days', now() - interval '29 days', 'Data diekspor dan dapat diunduh dari halaman Data Pribadi Saya.',
   '{"generated_at":"2026-09-16T09:00:00+07:00","profile":{"name":"Guru Matematika","email":"guru@teratai.local"},"roles":[{"unit":"Sekolah Teratai","role":"guru","status":"active"}],"login_history":[],"note":"Data dari layanan lain dapat diminta melalui admin unit."}'),
  ('a2000013-0000-0000-0000-000000000004', '66666666-6666-6666-6666-666666666666', 'deletion',
   'Saya meminta akun saya dihapus karena akan pindah tugas.', NULL, 'rejected',
   now() - interval '15 days', now() - interval '14 days',
   'Ditolak: akun masih aktif sebagai wali kelas tahun ajaran berjalan dan data wajib disimpan sesuai ketentuan arsip sekolah.', NULL),
  ('a2000013-0000-0000-0000-000000000005', '44444444-4444-4444-4444-444444444444', 'correction',
   'Tempat lahir tertulis salah, seharusnya Bandung.', '{"birth_place":"Bandung"}', 'completed',
   now() - interval '50 days', now() - interval '48 days', 'Koreksi diterapkan sesuai akta kelahiran.', NULL)
) AS v(id, user_id, rtype, details, corrections, status, requested_at, handled_at, resolution, export_payload)
JOIN users u ON u.id = v.user_id::uuid
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
  AND NOT EXISTS (SELECT 1 FROM data_subject_requests d
                  WHERE d.subject_user_id = u.id AND d.request_type = v.rtype
                    AND d.status IN ('submitted', 'in_review') AND v.status IN ('submitted', 'in_review'))
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 13. Migrasi data lama: run percobaan (signed off), run produksi (open),
--     run percobaan yang dibuang; control totals; batch impor; peta ID; arsip.
-- ---------------------------------------------------------------------------
INSERT INTO migration_runs (id, school_id, title, mode, status, trial_run_id, notes, created_by, created_at,
                            signed_off_by, signed_off_at, sign_off_note, discarded_by, discarded_at, discard_reason)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.title, v.mode, v.status, v.trial_run_id::uuid, v.notes,
       a.id, v.created_at,
       CASE WHEN v.status = 'signed_off' THEN k.id END, v.signed_off_at, v.sign_off_note,
       CASE WHEN v.status = 'discarded' THEN a.id END, v.discarded_at, v.discard_reason
FROM (VALUES
  ('a2000014-0000-0000-0000-000000000001', 'Percobaan migrasi data SIAKAD lama (Juni 2026)', 'trial', 'signed_off', NULL,
   'Uji muat seluruh siswa, guru, dan orang tua dari ekspor SIAKAD 2025/2026.', now() - interval '120 days',
   now() - interval '115 days', 'Selisih sudah dijelaskan; lanjut ke migrasi produksi.', NULL::timestamptz, NULL),
  ('a2000014-0000-0000-0000-000000000002', 'Migrasi produksi data SIAKAD lama', 'production', 'open', 'a2000014-0000-0000-0000-000000000001',
   'Migrasi produksi mengikuti hasil percobaan Juni 2026.', now() - interval '110 days',
   NULL::timestamptz, NULL, NULL::timestamptz, NULL),
  ('a2000014-0000-0000-0000-000000000003', 'Percobaan impor nilai rapor 2019-2023', 'trial', 'discarded', NULL,
   'Format file rapor lama tidak seragam antar-tahun.', now() - interval '80 days',
   NULL::timestamptz, NULL, now() - interval '78 days', 'File sumber salah versi; diunggah ulang sebagai arsip.')
) AS v(id, title, mode, status, trial_run_id, notes, created_at, signed_off_at, sign_off_note, discarded_at, discard_reason)
JOIN users a ON a.id = '22222222-2222-2222-2222-222222222222'
JOIN users k ON k.id = '77777777-7777-7777-7777-777777777777'
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ORDER BY v.mode DESC -- trial dulu agar FK trial_run_id terpenuhi
ON CONFLICT DO NOTHING;

INSERT INTO migration_control_totals (id, run_id, category, metric, label, source_value, target_value, target_source, explanation, updated_at)
SELECT v.id::uuid, v.run_id::uuid, v.category, v.metric, v.label, v.src, v.tgt, v.tsrc, v.expl, now() - interval '100 days'
FROM (VALUES
  ('a2000015-0000-0000-0000-000000000001', 'a2000014-0000-0000-0000-000000000001', 'siswa', 'count', 'Siswa aktif', 412::numeric, 410::numeric, 'auto',
   'Dua siswa ganda di sistem lama digabung.'),
  ('a2000015-0000-0000-0000-000000000002', 'a2000014-0000-0000-0000-000000000001', 'guru', 'count', 'Guru', 36, 36, 'auto', NULL),
  ('a2000015-0000-0000-0000-000000000003', 'a2000014-0000-0000-0000-000000000001', 'orang_tua', 'count', 'Akun orang tua', 389, 385, 'auto',
   'Empat orang tua memiliki dua anak sehingga satu akun.'),
  ('a2000015-0000-0000-0000-000000000004', 'a2000014-0000-0000-0000-000000000001', 'saldo_tunggakan', 'sum_amount', 'Saldo tunggakan SPP', 48750000, 48750000, 'manual',
   'Dicocokkan dengan laporan bendahara per 30 Juni 2026.'),
  ('a2000015-0000-0000-0000-000000000005', 'a2000014-0000-0000-0000-000000000002', 'siswa', 'count', 'Siswa aktif', 410, 409, 'auto', NULL),
  ('a2000015-0000-0000-0000-000000000006', 'a2000014-0000-0000-0000-000000000002', 'guru', 'count', 'Guru', 36, 36, 'auto', NULL),
  ('a2000015-0000-0000-0000-000000000007', 'a2000014-0000-0000-0000-000000000002', 'kelas', 'count', 'Kelas (dari impor)', 12, 12, 'auto', NULL),
  ('a2000015-0000-0000-0000-000000000008', 'a2000014-0000-0000-0000-000000000002', 'saldo_tunggakan', 'sum_amount', 'Saldo tunggakan SPP', 48750000, NULL, 'manual', NULL)
) AS v(id, run_id, category, metric, label, src, tgt, tsrc, expl)
JOIN migration_runs r ON r.id = v.run_id::uuid
ON CONFLICT DO NOTHING;

INSERT INTO import_batches (id, school_id, kind, file_name, file_format, total_rows, created_rows, skipped_rows, created_by, created_at,
                            rolled_back_at, rolled_back_by, rollback_reason, run_id, trial, category_counts, trial_refs)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.kind, v.file_name, v.fmt, v.total, v.created, v.skipped,
       (SELECT id FROM users WHERE id = '22222222-2222-2222-2222-222222222222'), now() - v.age,
       v.rb_at, CASE WHEN v.rb_at IS NOT NULL THEN (SELECT id FROM users WHERE id = '22222222-2222-2222-2222-222222222222') END, v.rb_reason,
       (SELECT id FROM migration_runs WHERE id = v.run_id::uuid), v.trial, v.counts::jsonb, v.refs::text[]
FROM (VALUES
  ('a2000016-0000-0000-0000-000000000001', 'users', 'siakad_siswa_guru_2026.xlsx', 'xlsx', 448, 446, 2, interval '119 days',
   NULL::timestamptz, NULL, 'a2000014-0000-0000-0000-000000000001', true, '{"siswa":410,"guru":36}', '{SIS-0001,SIS-0002,SIS-0003}'),
  ('a2000016-0000-0000-0000-000000000002', 'orang_tua', 'siakad_orang_tua_2026.csv', 'csv', 389, 385, 4, interval '118 days',
   NULL::timestamptz, NULL, 'a2000014-0000-0000-0000-000000000001', true, '{"orang_tua":385,"relasi_ortu":410}', '{}'),
  ('a2000016-0000-0000-0000-000000000003', 'kelas', 'pembagian_kelas_2026_2027.xlsx', 'xlsx', 1, 1, 0, interval '108 days',
   NULL::timestamptz, NULL, 'a2000014-0000-0000-0000-000000000002', false, '{"kelas":1,"anggota_kelas":1}', '{}'),
  ('a2000016-0000-0000-0000-000000000004', 'users', 'guru_honorer_salah_format.xlsx', 'xlsx', 6, 6, 0, interval '90 days',
   now() - interval '89 days', 'Kolom NIK dan NUPTK tertukar; impor diulang dengan file yang benar.', NULL, false, '{"guru":6}', '{}')
) AS v(id, kind, file_name, fmt, total, created, skipped, age, rb_at, rb_reason, run_id, trial, counts, refs)
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

-- Batch kelas (tidak dapat di-rollback otomatis): Budi ditempatkan di kelas 7A.
INSERT INTO import_batch_items (batch_id, row_number, user_id, role, email, source_ref, entity_type, entity_id, created_account)
SELECT b.id, 2, u.id, 'siswa', u.email, 'SIS-0001', 'enrollment', NULL, false
FROM import_batches b JOIN users u ON u.id = '44444444-4444-4444-4444-444444444444'
WHERE b.id = 'a2000016-0000-0000-0000-000000000003'
ON CONFLICT DO NOTHING;

INSERT INTO source_id_map (id, school_id, entity_type, source_ref, target_id, batch_id, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.entity, v.ref, u.id::text, NULL, now() - interval '108 days'
FROM (VALUES
  ('a2000017-0000-0000-0000-000000000001', 'siswa', 'SIS-0001', '44444444-4444-4444-4444-444444444444'),
  ('a2000017-0000-0000-0000-000000000002', 'orang_tua', 'ORT-0001', '33333333-3333-3333-3333-333333333333'),
  ('a2000017-0000-0000-0000-000000000003', 'guru', 'GTK-0007', '55555555-5555-5555-5555-555555555555'),
  ('a2000017-0000-0000-0000-000000000004', 'guru', 'GTK-0012', '66666666-6666-6666-6666-666666666666'),
  ('a2000017-0000-0000-0000-000000000005', 'guru', 'GTK-0001', '77777777-7777-7777-7777-777777777777')
) AS v(id, entity, ref, user_id)
JOIN users u ON u.id = v.user_id::uuid
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

INSERT INTO archive_records (id, school_id, run_id, category, source_ref, title, payload, file_name, imported_by, imported_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', (SELECT id FROM migration_runs WHERE id = 'a2000014-0000-0000-0000-000000000002'),
       v.category, v.ref, v.title, v.payload::jsonb, v.file_name,
       (SELECT id FROM users WHERE id = '22222222-2222-2222-2222-222222222222'), now() - interval '75 days'
FROM (VALUES
  ('a2000018-0000-0000-0000-000000000001', 'rapor_2019', 'R19-0101', 'Rapor 2019 - Ahmad Fauzi (VII A)',
   '{"nama":"Ahmad Fauzi","kelas":"VII A","semester":"Genap","rata_rata":84.5,"peringkat":5}', 'rapor_2019.xlsx'),
  ('a2000018-0000-0000-0000-000000000002', 'rapor_2019', 'R19-0102', 'Rapor 2019 - Dewi Lestari (VII A)',
   '{"nama":"Dewi Lestari","kelas":"VII A","semester":"Genap","rata_rata":88.2,"peringkat":2}', 'rapor_2019.xlsx'),
  ('a2000018-0000-0000-0000-000000000003', 'rapor_2019', 'R19-0103', 'Rapor 2019 - Rizky Pratama (VII B)',
   '{"nama":"Rizky Pratama","kelas":"VII B","semester":"Genap","rata_rata":79.0,"peringkat":14}', 'rapor_2019.xlsx'),
  ('a2000018-0000-0000-0000-000000000004', 'buku_induk_2018', 'BI18-0450', 'Buku Induk 2018 - Nur Aisyah',
   '{"nama":"Nur Aisyah","nis":"18450","tahun_masuk":2018,"tahun_lulus":2021,"asal_sekolah":"SD Negeri 3 Jakarta"}', 'buku_induk_2018.csv'),
  ('a2000018-0000-0000-0000-000000000005', 'buku_induk_2018', 'BI18-0451', 'Buku Induk 2018 - Fajar Nugroho',
   '{"nama":"Fajar Nugroho","nis":"18451","tahun_masuk":2018,"tahun_lulus":2021,"asal_sekolah":"SD Islam Al-Azhar"}', 'buku_induk_2018.csv')
) AS v(id, category, ref, title, payload, file_name)
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 14. Riwayat login (Keamanan & Tata Kelola). Tanpa session_id.
-- ---------------------------------------------------------------------------
INSERT INTO login_events (id, user_id, email, success, reason, ip, user_agent, created_at)
SELECT v.id::uuid, u.id, u.email, v.success, v.reason, v.ip, v.ua, now() - v.age
FROM (VALUES
  ('a2000019-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222', true, 'success', '10.10.1.12', 'Mozilla/5.0 (Windows NT 10.0) Chrome/128', interval '3 hours'),
  ('a2000019-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222', false, 'wrong_password', '10.10.1.12', 'Mozilla/5.0 (Windows NT 10.0) Chrome/128', interval '3 hours 2 minutes'),
  ('a2000019-0000-0000-0000-000000000003', '33333333-3333-3333-3333-333333333333', true, 'success', '114.124.50.8', 'Mozilla/5.0 (Linux; Android 14) Chrome/128 Mobile', interval '1 day'),
  ('a2000019-0000-0000-0000-000000000004', '44444444-4444-4444-4444-444444444444', true, 'success', '114.124.50.9', 'Mozilla/5.0 (Linux; Android 13) Chrome/127 Mobile', interval '2 days'),
  ('a2000019-0000-0000-0000-000000000005', '55555555-5555-5555-5555-555555555555', false, 'wrong_password', '10.10.2.40', 'Mozilla/5.0 (Macintosh) Safari/17', interval '2 days 1 hour'),
  ('a2000019-0000-0000-0000-000000000006', '55555555-5555-5555-5555-555555555555', true, 'success', '10.10.2.40', 'Mozilla/5.0 (Macintosh) Safari/17', interval '2 days 58 minutes'),
  ('a2000019-0000-0000-0000-000000000007', '77777777-7777-7777-7777-777777777777', true, 'success', '10.10.1.5', 'Mozilla/5.0 (Windows NT 10.0) Edge/128', interval '5 hours')
) AS v(id, user_id, success, reason, ip, ua, age)
JOIN users u ON u.id = v.user_id::uuid
ON CONFLICT DO NOTHING;

INSERT INTO login_events (id, user_id, email, success, reason, ip, user_agent, created_at)
VALUES ('a2000019-0000-0000-0000-000000000008', NULL, 'admin@teratai.co.id', false, 'unknown_email', '182.253.11.4',
        'python-requests/2.31', now() - interval '6 hours')
ON CONFLICT DO NOTHING;

COMMIT;
