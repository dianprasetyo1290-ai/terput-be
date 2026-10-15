-- 23_admission_features.sql
-- Data demo PPDB lengkap untuk satu periode tersendiri ("PPDB 2026/2027 Jalur
-- Awal", status closed agar tidak muncul di formulir publik): gelombang per
-- jenjang, form builder, berkas + versi, tagihan dua tahap + cicilan per
-- pendaftar, pembayaran (verifikasi/tolak/reversal), diskon manual (approval),
-- voucher, formula & ranking, persetujuan hasil seleksi, pengumuman, daftar
-- cadangan, jadwal tes/wawancara, notifikasi wali, refund, daftar ulang,
-- konversi siswa (Budi Santoso), landing page, referral dan atribusi.
--
-- Aturan: hanya MENAMBAH (UUID tetap berawalan 'a23', ON CONFLICT DO NOTHING,
-- WHERE NOT EXISTS). Satu blok DO = satu transaksi, idempoten.
--
-- Konsistensi lintas layanan:
--   * selection_approvals a2300013-..001 (pending)  <-> core approval a2100003-..003
--   * selection_approvals a2300013-..002 (approved) <-> core approval a2100003-..017
--   * manual_discounts    a2300008-..001 (pending)  <-> core approval a2100003-..004
--   * manual_discounts    a2300008-..002 (approved) <-> core approval a2100003-..016
--   * student_conversions a2300019-..001 <-> identity admission_conversion_mappings a2000006-..001
--     (wali 3333 + siswa Budi 4444)
-- Keamanan job admission:
--   * Tidak ada pendaftar 'submitted'/'offered' yang lewat tenggat tanpa bayar
--     minimum (job gugur otomatis tidak menyentuh data ini).
--   * guardian_notifications tidak ada yang 'pending' (job email tidak mengirim).
--   * Pengumuman tidak ada yang 'approved' (job publikasi tidak berjalan).
--   * Konversi siswa 'completed' (saga tidak memanggil identity/academic).
-- Berkas: object_key/sha256 wajib NOT NULL di skema; diisi '' (kosong) karena
-- tidak ada file nyata di storage. Tidak ada path palsu.

DO $seed$
DECLARE
  v_school uuid := '11111111-1111-1111-1111-111111111111';
  v_period uuid := 'a2300000-0000-0000-0000-000000000001';
  v_w1 uuid := 'a2300000-0000-0000-0000-000000000011';
  v_w2 uuid := 'a2300000-0000-0000-0000-000000000012';
  v_reg uuid := 'a2300000-0000-0000-0000-000000000021';
  v_prs uuid := 'a2300000-0000-0000-0000-000000000022';
  v_fee uuid := 'a2300000-0000-0000-0000-000000000041';
  v_admin text := '22222222-2222-2222-2222-222222222222';
  v_kepsek text := '77777777-7777-7777-7777-777777777777';
BEGIN
  -- -------------------------------------------------------------------------
  -- 0. Periode, gelombang, jalur, persyaratan, biaya, komponen seleksi
  -- -------------------------------------------------------------------------
  INSERT INTO admission_periods (id, school_id, name, academic_year, timezone, status, registration_open_at, registration_close_at,
                                 discount_mode, max_discount_idr, max_payment_grace_days, refund_policy_bp, refund_admin_fee_idr,
                                 created_at, updated_at)
  VALUES (v_period, v_school, 'PPDB 2026/2027 Jalur Awal', '2026/2027', 'Asia/Jakarta', 'closed',
          now() - interval '280 days', now() - interval '20 days', 'stack', 5000000, 14, 7500, 100000,
          now() - interval '300 days', now() - interval '20 days')
  ON CONFLICT DO NOTHING;

  IF NOT EXISTS (SELECT 1 FROM admission_periods WHERE id = v_period) THEN
    RAISE NOTICE '23_admission_features: periode demo tidak dapat dibuat (nama sudah dipakai), seeder dilewati';
    RETURN;
  END IF;

  INSERT INTO admission_waves (id, school_id, period_id, name, registration_start, registration_end, payment_end, test_start, test_end,
                               announcement_at, reregistration_end, waitlist_reregistration_days, education_level, entry_grade, created_at, updated_at)
  VALUES
    (v_w1, v_school, v_period, 'Gelombang 1 - SMP Kelas 7', now() - interval '280 days', now() - interval '250 days', now() - interval '245 days',
     now() - interval '240 days', now() - interval '238 days', now() - interval '230 days', now() + interval '45 days', 7, 'SMP', 7,
     now() - interval '300 days', now() - interval '230 days'),
    (v_w2, v_school, v_period, 'Gelombang Susulan - SMP Kelas 7', now() - interval '45 days', now() - interval '20 days', now() - interval '18 days',
     now() - interval '15 days', now() - interval '14 days', now() + interval '5 days', now() + interval '30 days', 7, 'SMP', 7,
     now() - interval '50 days', now() - interval '14 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO admission_tracks (id, school_id, period_id, code, name, quota, eligibility_config, waitlist_policy, description, is_active, created_at, updated_at)
  VALUES
    (v_reg, v_school, v_period, 'A-REG', 'Jalur Reguler', 4, '{}', '{}', 'Seleksi tes akademik, nilai rapor dan wawancara', true,
     now() - interval '300 days', now() - interval '300 days'),
    (v_prs, v_school, v_period, 'A-PRS', 'Jalur Prestasi', 2, '{}', '{}', 'Untuk calon siswa dengan prestasi akademik/non-akademik minimal tingkat kota', true,
     now() - interval '300 days', now() - interval '300 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO document_requirements (id, school_id, period_id, track_id, code, name, required, allowed_mimes, max_bytes, version, description, stage, created_at)
  VALUES
    ('a2300000-0000-0000-0000-000000000031', v_school, v_period, NULL, 'AKTA', 'Akta Kelahiran', true, ARRAY['application/pdf','image/jpeg','image/png'], 5242880, 1,
     'Scan akta kelahiran asli', 'registration', now() - interval '300 days'),
    ('a2300000-0000-0000-0000-000000000032', v_school, v_period, NULL, 'KK', 'Kartu Keluarga', true, ARRAY['application/pdf','image/jpeg','image/png'], 5242880, 1,
     'Scan kartu keluarga terbaru', 'registration', now() - interval '300 days'),
    ('a2300000-0000-0000-0000-000000000033', v_school, v_period, NULL, 'RAPOR', 'Rapor Kelas 4-6 SD', true, ARRAY['application/pdf'], 10485760, 1,
     'Gabungkan rapor kelas 4, 5 dan 6 dalam satu PDF', 'registration', now() - interval '300 days'),
    ('a2300000-0000-0000-0000-000000000034', v_school, v_period, v_prs, 'PIAGAM', 'Piagam Prestasi', true, ARRAY['application/pdf','image/jpeg'], 5242880, 1,
     'Piagam/sertifikat lomba minimal tingkat kota', 'registration', now() - interval '300 days'),
    ('a2300000-0000-0000-0000-000000000035', v_school, v_period, NULL, 'SKL', 'Surat Keterangan Lulus SD', true, ARRAY['application/pdf'], 5242880, 1,
     'Diunggah saat daftar ulang', 'reregistration', now() - interval '300 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO fee_schemes (id, school_id, period_id, track_id, wave_id, name, revision, status, currency, registration_fee_idr, reregistration_fee_idr,
                           entrance_fee_idr, min_reregistration_pct, installment_count, created_at)
  VALUES (v_fee, v_school, v_period, NULL, NULL, 'Biaya PPDB Jalur Awal 2026/2027', 1, 'published', 'IDR', 200000, 2500000, 15000000, 50, 2,
          now() - interval '300 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO fee_items (id, school_id, fee_scheme_id, code, name, amount_idr, discountable)
  VALUES
    ('a2300000-0000-0000-0000-000000000042', v_school, v_fee, 'FORM', 'Formulir Pendaftaran', 200000, false),
    ('a2300000-0000-0000-0000-000000000043', v_school, v_fee, 'UANG_PANGKAL', 'Uang Pangkal', 15000000, true),
    ('a2300000-0000-0000-0000-000000000044', v_school, v_fee, 'DAFTAR_ULANG', 'Daftar Ulang (seragam, buku, MPLS)', 2500000, true)
  ON CONFLICT DO NOTHING;

  INSERT INTO installment_plans (id, school_id, fee_scheme_id, sequence, description, amount_idr, due_at)
  VALUES
    ('a2300000-0000-0000-0000-000000000045', v_school, v_fee, 1, 'Termin 1 (minimum daftar ulang 50%)', 8750000, now() - interval '200 days'),
    ('a2300000-0000-0000-0000-000000000046', v_school, v_fee, 2, 'Termin 2 (pelunasan)', 8750000, now() + interval '45 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO selection_components (id, school_id, period_id, code, name, source, required, max_score)
  VALUES
    ('a2300000-0000-0000-0000-000000000051', v_school, v_period, 'TES', 'Tes Potensi Akademik', 'exam', true, 10000),
    ('a2300000-0000-0000-0000-000000000052', v_school, v_period, 'RAPOR', 'Rata-rata Nilai Rapor', 'manual', true, 10000),
    ('a2300000-0000-0000-0000-000000000053', v_school, v_period, 'WAWANCARA', 'Wawancara Siswa & Orang Tua', 'manual', true, 10000)
  ON CONFLICT DO NOTHING;

  -- Form builder: definisi formulir (versi) + field tambahan
  INSERT INTO form_definitions (id, school_id, period_id, version, schema, status, created_at)
  VALUES
    ('a2300001-0000-0000-0000-000000000001', v_school, v_period, 1,
     '{"sections":[{"title":"Data Calon Siswa","fields":["full_name","nisn","birth_place","birth_date","gender","previous_school"]},{"title":"Data Orang Tua","fields":["guardian_relationship","address"]}]}',
     'published', now() - interval '290 days'),
    ('a2300001-0000-0000-0000-000000000002', v_school, v_period, 2,
     '{"sections":[{"title":"Data Calon Siswa","fields":["full_name","nisn","birth_place","birth_date","gender","previous_school","ukuran_seragam"]},{"title":"Data Orang Tua","fields":["guardian_relationship","address","pekerjaan_ayah"]}]}',
     'draft', now() - interval '30 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO admission_form_fields (id, school_id, period_id, field_key, label, field_type, options, required, help_text, sort_order, active, created_at, updated_at)
  VALUES
    ('a2300036-0000-0000-0000-000000000001', v_school, v_period, 'ukuran_seragam', 'Ukuran Seragam', 'select', '["S","M","L","XL"]', true, 'Pilih ukuran terdekat', 10, true, now() - interval '290 days', now() - interval '290 days'),
    ('a2300036-0000-0000-0000-000000000002', v_school, v_period, 'pekerjaan_ayah', 'Pekerjaan Ayah', 'text', '[]', false, '', 20, true, now() - interval '290 days', now() - interval '290 days'),
    ('a2300036-0000-0000-0000-000000000003', v_school, v_period, 'riwayat_penyakit', 'Riwayat Penyakit/Alergi', 'textarea', '[]', false, 'Kosongkan bila tidak ada', 30, true, now() - interval '290 days', now() - interval '290 days'),
    ('a2300036-0000-0000-0000-000000000004', v_school, v_period, 'jarak_rumah_km', 'Jarak Rumah ke Sekolah (km)', 'number', '[]', false, '', 40, true, now() - interval '290 days', now() - interval '290 days'),
    ('a2300036-0000-0000-0000-000000000005', v_school, v_period, 'minat_ekskul', 'Minat Ekstrakurikuler', 'select', '["Robotik","Pramuka","Futsal","Paduan Suara","Tari"]', false, '', 50, true, now() - interval '290 days', now() - interval '290 days'),
    ('a2300036-0000-0000-0000-000000000006', v_school, v_period, 'setuju_tata_tertib', 'Menyetujui Tata Tertib Sekolah', 'checkbox', '[]', true, '', 60, false, now() - interval '290 days', now() - interval '100 days')
  ON CONFLICT DO NOTHING;

  -- -------------------------------------------------------------------------
  -- 1. Wali, kontak terverifikasi, calon siswa, pendaftaran
  -- -------------------------------------------------------------------------
  INSERT INTO guardians (id, school_id, name, identity_user_id, status, created_at, updated_at)
  SELECT v.id::uuid, v_school, v.name,
         CASE WHEN v.uid IS NOT NULL AND NOT EXISTS (SELECT 1 FROM guardians g WHERE g.school_id = v_school AND g.identity_user_id = v.uid::uuid)
              THEN v.uid::uuid END,
         'active', now() - interval '280 days', now() - interval '280 days'
  FROM (VALUES
    ('a2300020-0000-0000-0000-000000000001', 'Wali Budi Santoso', '33333333-3333-3333-3333-333333333333'),
    ('a2300020-0000-0000-0000-000000000002', 'Ratna Dewi', NULL),
    ('a2300020-0000-0000-0000-000000000003', 'Hendra Saputra', NULL),
    ('a2300020-0000-0000-0000-000000000004', 'Siti Aminah', NULL),
    ('a2300020-0000-0000-0000-000000000005', 'Yusuf Hidayat', NULL),
    ('a2300020-0000-0000-0000-000000000006', 'Maya Lestari', NULL),
    ('a2300020-0000-0000-0000-000000000007', 'Agus Salim', NULL),
    ('a2300020-0000-0000-0000-000000000008', 'Lina Marlina', NULL)
  ) AS v(id, name, uid)
  ON CONFLICT DO NOTHING;

  INSERT INTO guardian_contacts (id, school_id, guardian_id, kind, normalized_value, verified_at, created_at)
  SELECT v.id::uuid, v_school, g.id, v.kind, v.val, now() - interval '280 days', now() - interval '280 days'
  FROM (VALUES
    ('a2300021-0000-0000-0000-000000000001', 'a2300020-0000-0000-0000-000000000001', 'email', 'wali.budi@ppdb-demo.test'),
    ('a2300021-0000-0000-0000-000000000002', 'a2300020-0000-0000-0000-000000000002', 'email', 'ratna.dewi@ppdb-demo.test'),
    ('a2300021-0000-0000-0000-000000000003', 'a2300020-0000-0000-0000-000000000002', 'phone', '6281200000101'),
    ('a2300021-0000-0000-0000-000000000004', 'a2300020-0000-0000-0000-000000000003', 'phone', '6281200000102'),
    ('a2300021-0000-0000-0000-000000000005', 'a2300020-0000-0000-0000-000000000004', 'phone', '6281200000103'),
    ('a2300021-0000-0000-0000-000000000006', 'a2300020-0000-0000-0000-000000000005', 'phone', '6281200000104'),
    ('a2300021-0000-0000-0000-000000000007', 'a2300020-0000-0000-0000-000000000006', 'email', 'maya.lestari@ppdb-demo.test'),
    ('a2300021-0000-0000-0000-000000000008', 'a2300020-0000-0000-0000-000000000007', 'email', 'agus.salim@ppdb-demo.test'),
    ('a2300021-0000-0000-0000-000000000009', 'a2300020-0000-0000-0000-000000000008', 'email', 'lina.marlina@ppdb-demo.test')
  ) AS v(id, guardian_id, kind, val)
  JOIN guardians g ON g.id = v.guardian_id::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO applicants (id, school_id, guardian_id, full_name, nickname, nisn, birth_date, birth_place, gender, source, previous_school, address,
                          guardian_relationship, profile, custom_fields, created_at, updated_at)
  SELECT v.id::uuid, v_school, g.id, v.name, v.nick, v.nisn, v.bdate::date, v.bplace, v.gender, v.src, v.prev, v.addr, v.rel, '{}', v.custom::jsonb,
         now() - interval '275 days', now() - interval '275 days'
  FROM (VALUES
    ('a2300022-0000-0000-0000-000000000001', 'a2300020-0000-0000-0000-000000000001', 'Budi Santoso', 'Budi', '0134567801', '2013-05-14', 'Jakarta', 'L', 'landing_page',
     'SD Negeri 01 Kebayoran Baru', 'Jl. Melati No. 12, Kebayoran Baru, Jakarta Selatan', 'ayah', '{"ukuran_seragam":"M","minat_ekskul":"Robotik"}'),
    ('a2300022-0000-0000-0000-000000000002', 'a2300020-0000-0000-0000-000000000002', 'Nadia Putri', 'Nadia', '0134567802', '2013-08-02', 'Bogor', 'P', 'referral',
     'SD Islam Al-Azhar 5', 'Jl. Kenanga No. 7, Cilandak, Jakarta Selatan', 'ibu', '{"ukuran_seragam":"S","minat_ekskul":"Paduan Suara"}'),
    ('a2300022-0000-0000-0000-000000000003', 'a2300020-0000-0000-0000-000000000003', 'Rizky Pratama', 'Rizky', '0134567803', '2013-02-21', 'Depok', 'L', 'landing_page',
     'SD Negeri 03 Pondok Labu', 'Jl. Anggrek No. 3, Pondok Labu, Jakarta Selatan', 'ayah', '{"ukuran_seragam":"L","minat_ekskul":"Futsal"}'),
    ('a2300022-0000-0000-0000-000000000004', 'a2300020-0000-0000-0000-000000000004', 'Salsabila Azzahra', 'Salsa', '0134567804', '2013-11-30', 'Jakarta', 'P', 'walk_in',
     'SD Muhammadiyah 12', 'Jl. Cempaka No. 21, Pasar Minggu, Jakarta Selatan', 'ibu', '{"ukuran_seragam":"S"}'),
    ('a2300022-0000-0000-0000-000000000005', 'a2300020-0000-0000-0000-000000000005', 'Fajar Nugroho', 'Fajar', '0134567805', '2013-04-09', 'Tangerang', 'L', 'walk_in',
     'SD Negeri 05 Cipete', 'Jl. Flamboyan No. 9, Cipete, Jakarta Selatan', 'ayah', '{}'),
    ('a2300022-0000-0000-0000-000000000006', 'a2300020-0000-0000-0000-000000000006', 'Aisyah Rahma', 'Aisyah', '0134567806', '2013-07-17', 'Bekasi', 'P', 'landing_page',
     'SD Negeri 02 Cilandak', 'Jl. Teratai No. 4, Cilandak, Jakarta Selatan', 'ibu', '{"ukuran_seragam":"M","minat_ekskul":"Tari"}'),
    ('a2300022-0000-0000-0000-000000000007', 'a2300020-0000-0000-0000-000000000007', 'Dimas Saputra', 'Dimas', '0134567807', '2013-09-25', 'Jakarta', 'L', 'whatsapp',
     'SD Kristen Penabur 7', 'Jl. Dahlia No. 15, Fatmawati, Jakarta Selatan', 'ayah', '{"ukuran_seragam":"L","minat_ekskul":"Pramuka"}'),
    ('a2300022-0000-0000-0000-000000000008', 'a2300020-0000-0000-0000-000000000008', 'Kevin Halim', 'Kevin', '0134567808', '2013-12-05', 'Jakarta', 'L', 'walk_in',
     'SD Santa Ursula', 'Jl. Kamboja No. 2, Kemang, Jakarta Selatan', 'ibu', '{"minat_ekskul":"Robotik"}'),
    ('a2300022-0000-0000-0000-000000000009', 'a2300020-0000-0000-0000-000000000008', 'Putri Ayu Lestari', 'Putri', '0134567809', '2013-03-11', 'Jakarta', 'P', 'referral',
     'SD Santa Ursula', 'Jl. Kamboja No. 2, Kemang, Jakarta Selatan', 'ibu', '{}')
  ) AS v(id, guardian_id, name, nick, nisn, bdate, bplace, gender, src, prev, addr, rel, custom)
  JOIN guardians g ON g.id = v.guardian_id::uuid
  ON CONFLICT DO NOTHING;

  -- Tidak ada status 'submitted'/'offered' yang lewat tenggat: aman dari job gugur.
  INSERT INTO applications (id, school_id, applicant_id, period_id, wave_id, track_id, registration_number, status, version, answers,
                            submitted_snapshot, submitted_at, reregistration_due_at, dropped_at, drop_reason, refund_due_idr, created_at, updated_at)
  SELECT v.id::uuid, v_school, ap.id, v_period, v.wave::uuid, v.track::uuid, v.num, v.status, v.ver, '{}', '{"source":"seed-a23"}',
         now() - v.age, v.rereg_due, v.dropped, v.drop_reason, v.refund, now() - v.age, now() - v.age + interval '1 day'
  FROM (VALUES
    ('a2300023-0000-0000-0000-000000000001', 'a2300022-0000-0000-0000-000000000001', 'a2300000-0000-0000-0000-000000000011', 'a2300000-0000-0000-0000-000000000021',
     'PPDB-2026-A1-0001', 'converted', 8, interval '275 days', now() - interval '200 days', NULL::timestamptz, NULL, 0::bigint),
    ('a2300023-0000-0000-0000-000000000002', 'a2300022-0000-0000-0000-000000000002', 'a2300000-0000-0000-0000-000000000011', 'a2300000-0000-0000-0000-000000000021',
     'PPDB-2026-A1-0002', 'reregistration', 7, interval '270 days', now() + interval '45 days', NULL, NULL, 0),
    ('a2300023-0000-0000-0000-000000000003', 'a2300022-0000-0000-0000-000000000003', 'a2300000-0000-0000-0000-000000000011', 'a2300000-0000-0000-0000-000000000022',
     'PPDB-2026-A1-0003', 'dropped', 7, interval '268 days', now() - interval '200 days', now() - interval '199 days',
     'Pembayaran minimum daftar ulang tidak terpenuhi sampai batas waktu (BR-PPDB-08)', 3650000),
    ('a2300023-0000-0000-0000-000000000004', 'a2300022-0000-0000-0000-000000000004', 'a2300000-0000-0000-0000-000000000011', 'a2300000-0000-0000-0000-000000000021',
     'PPDB-2026-A1-0004', 'waitlisted', 6, interval '265 days', NULL, NULL, NULL, 0),
    ('a2300023-0000-0000-0000-000000000005', 'a2300022-0000-0000-0000-000000000005', 'a2300000-0000-0000-0000-000000000011', 'a2300000-0000-0000-0000-000000000021',
     'PPDB-2026-A1-0005', 'rejected', 6, interval '262 days', NULL, NULL, NULL, 0),
    ('a2300023-0000-0000-0000-000000000006', 'a2300022-0000-0000-0000-000000000006', 'a2300000-0000-0000-0000-000000000012', 'a2300000-0000-0000-0000-000000000021',
     'PPDB-2026-A2-0001', 'ranked', 5, interval '40 days', NULL, NULL, NULL, 0),
    ('a2300023-0000-0000-0000-000000000007', 'a2300022-0000-0000-0000-000000000007', 'a2300000-0000-0000-0000-000000000012', 'a2300000-0000-0000-0000-000000000021',
     'PPDB-2026-A2-0002', 'ranked', 5, interval '35 days', NULL, NULL, NULL, 0),
    ('a2300023-0000-0000-0000-000000000008', 'a2300022-0000-0000-0000-000000000008', 'a2300000-0000-0000-0000-000000000012', 'a2300000-0000-0000-0000-000000000022',
     'PPDB-2026-A2-0003', 'document_review', 2, interval '22 days', NULL, NULL, NULL, 0),
    ('a2300023-0000-0000-0000-000000000009', 'a2300022-0000-0000-0000-000000000009', 'a2300000-0000-0000-0000-000000000011', 'a2300000-0000-0000-0000-000000000021',
     'PPDB-2026-A1-0006', 'withdrawn', 7, interval '260 days', now() - interval '200 days', NULL, NULL, 0)
  ) AS v(id, applicant_id, wave, track, num, status, ver, age, rereg_due, dropped, drop_reason, refund)
  JOIN applicants ap ON ap.id = v.applicant_id::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO registration_counters (wave_id, school_id, prefix, last_value)
  VALUES (v_w1, v_school, 'PPDB-2026-A1', 6), (v_w2, v_school, 'PPDB-2026-A2', 3)
  ON CONFLICT DO NOTHING;

  -- Riwayat status (bigserial: dijaga NOT EXISTS per aplikasi+status)
  INSERT INTO application_state_events (school_id, application_id, from_status, to_status, actor_type, actor_id, reason, version, created_at)
  SELECT v_school, a.id, v.from_s, v.to_s, v.actor_type, v.actor_id, v.reason, v.ver, a.created_at + v.after
  FROM (VALUES
    ('a2300023-0000-0000-0000-000000000001', 'draft', 'submitted', 'guardian', 'a2300020-0000-0000-0000-000000000001', NULL, 2, interval '0'),
    ('a2300023-0000-0000-0000-000000000001', 'ranked', 'offered', 'admin', '22222222-2222-2222-2222-222222222222', NULL, 5, interval '45 days'),
    ('a2300023-0000-0000-0000-000000000001', 'reregistration', 'converted', 'system', 'admission', 'Konversi menjadi siswa aktif', 8, interval '90 days'),
    ('a2300023-0000-0000-0000-000000000003', 'reregistration', 'dropped', 'system', 'admission',
     'Pembayaran minimum daftar ulang tidak terpenuhi sampai batas waktu (BR-PPDB-08)', 7, interval '69 days'),
    ('a2300023-0000-0000-0000-000000000009', 'reregistration', 'withdrawn', 'guardian', 'a2300020-0000-0000-0000-000000000008',
     'Keluarga pindah domisili ke Surabaya', 7, interval '55 days'),
    ('a2300023-0000-0000-0000-000000000008', 'submitted', 'document_review', 'system', 'admission', NULL, 2, interval '1 day')
  ) AS v(app, from_s, to_s, actor_type, actor_id, reason, ver, after)
  JOIN applications a ON a.id = v.app::uuid
  WHERE NOT EXISTS (SELECT 1 FROM application_state_events e WHERE e.application_id = a.id AND e.to_status = v.to_s);

  -- -------------------------------------------------------------------------
  -- 2. Berkas, versi berkas (unggah ulang), review berkas
  -- -------------------------------------------------------------------------
  INSERT INTO application_documents (id, school_id, application_id, requirement_id, object_key, mime, size_bytes, sha256, scan_status, uploaded_by,
                                     locked_at, file_name, version, lock_version, created_at)
  SELECT v.id::uuid, v_school, a.id, v.req::uuid, '', v.mime, 0, '', v.scan, v.uploader, v.locked, v.fname, v.ver, v.ver, a.created_at + interval '1 hour'
  FROM (VALUES
    ('a2300024-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000001', 'a2300000-0000-0000-0000-000000000031', 'application/pdf', 'clean', 'guardian', now() - interval '230 days', 'akta_budi.pdf', 1),
    ('a2300024-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000001', 'a2300000-0000-0000-0000-000000000032', 'application/pdf', 'clean', 'guardian', now() - interval '230 days', 'kk_santoso.pdf', 1),
    ('a2300024-0000-0000-0000-000000000003', 'a2300023-0000-0000-0000-000000000001', 'a2300000-0000-0000-0000-000000000033', 'application/pdf', 'clean', 'guardian', now() - interval '230 days', 'rapor_budi_4-6.pdf', 1),
    ('a2300024-0000-0000-0000-000000000004', 'a2300023-0000-0000-0000-000000000008', 'a2300000-0000-0000-0000-000000000031', 'image/jpeg', 'clean', 'guardian', NULL, 'akta_kevin_rev.jpg', 2),
    ('a2300024-0000-0000-0000-000000000005', 'a2300023-0000-0000-0000-000000000008', 'a2300000-0000-0000-0000-000000000034', 'application/pdf', 'pending', 'guardian', NULL, 'piagam_olimpiade_matematika.pdf', 1),
    ('a2300024-0000-0000-0000-000000000006', 'a2300023-0000-0000-0000-000000000006', 'a2300000-0000-0000-0000-000000000031', 'application/pdf', 'clean', 'guardian', NULL, 'akta_aisyah.pdf', 1)
  ) AS v(id, app, req, mime, scan, uploader, locked, fname, ver)
  JOIN applications a ON a.id = v.app::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO application_document_versions (id, school_id, document_id, application_id, version, object_key, file_name, mime, size_bytes, sha256,
                                             uploaded_by, uploaded_at, archived_at)
  SELECT 'a2300025-0000-0000-0000-000000000001', v_school, d.id, d.application_id, 1, '', 'akta_kevin_buram.jpg', 'image/jpeg', 0, '',
         'guardian', now() - interval '22 days', now() - interval '19 days'
  FROM application_documents d WHERE d.id = 'a2300024-0000-0000-0000-000000000004'
  ON CONFLICT DO NOTHING;

  INSERT INTO document_reviews (id, school_id, document_id, status, notes, reviewer_id, document_version, reviewer_name, created_at)
  SELECT v.id::uuid, v_school, d.id, v.status, v.notes, v_admin, v.ver, 'Admin Teratai', d.created_at + v.after
  FROM (VALUES
    ('a2300026-0000-0000-0000-000000000001', 'a2300024-0000-0000-0000-000000000001', 'accepted', NULL, 1, interval '2 days'),
    ('a2300026-0000-0000-0000-000000000002', 'a2300024-0000-0000-0000-000000000002', 'accepted', NULL, 1, interval '2 days'),
    ('a2300026-0000-0000-0000-000000000003', 'a2300024-0000-0000-0000-000000000003', 'accepted', 'Nilai rapor sesuai dengan data yang diisi.', 1, interval '2 days'),
    ('a2300026-0000-0000-0000-000000000004', 'a2300024-0000-0000-0000-000000000004', 'correction', 'Foto akta buram, mohon unggah ulang hasil scan.', 1, interval '-3 days'),
    ('a2300026-0000-0000-0000-000000000005', 'a2300024-0000-0000-0000-000000000006', 'accepted', NULL, 1, interval '1 day')
  ) AS v(id, doc, status, notes, ver, after)
  JOIN application_documents d ON d.id = v.doc::uuid
  ON CONFLICT DO NOTHING;

  -- -------------------------------------------------------------------------
  -- 3. Voucher, diskon manual (approval), tagihan dua tahap + cicilan
  -- -------------------------------------------------------------------------
  -- Hash kode voucher sengaja bukan bcrypt yang valid: voucher demo tidak bisa dipakai.
  INSERT INTO discount_vouchers (id, school_id, period_id, code_hash, kind, value, cap_idr, priority, usage_limit, used_count, valid_from, valid_until,
                                 status, code_hint, created_at)
  VALUES ('a2300037-0000-0000-0000-000000000001', v_school, v_period, '!seed-a23-voucher-tidak-dapat-dipakai', 'fixed', 250000, NULL, 30, 50, 1,
          now() - interval '280 days', now() - interval '200 days', 'active', 'OPEN****26', now() - interval '280 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO manual_discounts (id, school_id, application_id, kind, value, cap_idr, priority, reason, created_by, created_by_name, status,
                                decided_by, decided_by_name, decided_at, decided_note, stage, approval_id, created_at)
  SELECT v.id::uuid, v_school, a.id, v.kind, v.value, v.cap, 50, v.reason, v_admin, 'Admin Teratai', v.status,
         v.decided_by, v.decided_name, v.decided_at, v.note, 'reregistration', v.approval::uuid, v.created
  FROM (VALUES
    ('a2300008-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000002', 'percent', 5000::bigint, 7500000::bigint,
     'Anak pegawai (guru) - potongan 50% uang pangkal sesuai SK Yayasan', 'pending', NULL, NULL, NULL::timestamptz, NULL,
     'a2100003-0000-0000-0000-000000000004', now() - interval '3 days'),
    ('a2300008-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000001', 'fixed', 1000000, NULL,
     'Saudara kandung masih aktif bersekolah di Sekolah Teratai', 'approved', v_kepsek, 'Kepala Sekolah Teratai', now() - interval '228 days',
     'Kakak kandung tercatat aktif.', 'a2100003-0000-0000-0000-000000000016', now() - interval '229 days'),
    ('a2300008-0000-0000-0000-000000000003', 'a2300023-0000-0000-0000-000000000004', 'percent', 2500, NULL,
     'Permohonan keringanan karena orang tua terdampak PHK', 'rejected', v_kepsek, 'Kepala Sekolah Teratai', now() - interval '220 days',
     'Dialihkan ke program beasiswa setelah diterima.', NULL, now() - interval '222 days'),
    ('a2300008-0000-0000-0000-000000000004', 'a2300023-0000-0000-0000-000000000003', 'fixed', 500000, NULL,
     'Kompensasi keterlambatan pengumuman jalur prestasi', 'cancelled', NULL, NULL, NULL, 'Dibatalkan karena pendaftaran gugur.',
     NULL, now() - interval '205 days')
  ) AS v(id, app, kind, value, cap, reason, status, decided_by, decided_name, decided_at, note, approval, created)
  JOIN applications a ON a.id = v.app::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO voucher_redemptions (id, school_id, application_id, voucher_id, redeemed_at, stage)
  SELECT 'a2300038-0000-0000-0000-000000000001', v_school, a.id, dv.id, now() - interval '229 days', 'reregistration'
  FROM applications a JOIN discount_vouchers dv ON dv.id = 'a2300037-0000-0000-0000-000000000001'
  WHERE a.id = 'a2300023-0000-0000-0000-000000000001'
  ON CONFLICT DO NOTHING;

  INSERT INTO charges (id, school_id, application_id, revision, fee_scheme_id, gross_idr, discount_idr, net_idr, items_snapshot, discount_snapshot,
                       finalized_at, stage, cancelled_at, cancel_reason, min_payment_idr, due_at)
  SELECT v.id::uuid, v_school, a.id, CASE WHEN v.stage = 'registration' THEN 1 ELSE 2 END, v_fee, v.gross, v.disc, v.gross - v.disc, v.items::jsonb, v.discs::jsonb, a.created_at + v.after,
         v.stage, v.cancelled, v.cancel_reason, v.minpay, v.due
  FROM (VALUES
    ('a2300027-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000001', 'registration', 200000::bigint, 0::bigint,
     '[{"code":"FORM","name":"Formulir Pendaftaran","amount_idr":200000}]', '[]', 200000::bigint, now() - interval '245 days', NULL::timestamptz, NULL, interval '0'),
    ('a2300027-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000001', 'reregistration', 17500000, 1250000,
     '[{"code":"UANG_PANGKAL","name":"Uang Pangkal","amount_idr":15000000},{"code":"DAFTAR_ULANG","name":"Daftar Ulang (seragam, buku, MPLS)","amount_idr":2500000}]',
     '[{"origin":"manual","source_id":"a2300008-0000-0000-0000-000000000002","applied_idr":1000000},{"origin":"voucher","source_id":"a2300037-0000-0000-0000-000000000001","applied_idr":250000}]',
     8125000, now() - interval '200 days', NULL, NULL, interval '46 days'),
    ('a2300027-0000-0000-0000-000000000003', 'a2300023-0000-0000-0000-000000000002', 'registration', 200000, 0,
     '[{"code":"FORM","name":"Formulir Pendaftaran","amount_idr":200000}]', '[]', 200000, now() - interval '245 days', NULL, NULL, interval '0'),
    ('a2300027-0000-0000-0000-000000000004', 'a2300023-0000-0000-0000-000000000002', 'reregistration', 17500000, 0,
     '[{"code":"UANG_PANGKAL","name":"Uang Pangkal","amount_idr":15000000},{"code":"DAFTAR_ULANG","name":"Daftar Ulang (seragam, buku, MPLS)","amount_idr":2500000}]',
     '[]', 8750000, now() + interval '45 days', NULL, NULL, interval '46 days'),
    ('a2300027-0000-0000-0000-000000000005', 'a2300023-0000-0000-0000-000000000003', 'registration', 200000, 0,
     '[{"code":"FORM","name":"Formulir Pendaftaran","amount_idr":200000}]', '[]', 200000, now() - interval '245 days', NULL, NULL, interval '0'),
    ('a2300027-0000-0000-0000-000000000006', 'a2300023-0000-0000-0000-000000000003', 'reregistration', 17500000, 0,
     '[{"code":"UANG_PANGKAL","name":"Uang Pangkal","amount_idr":15000000},{"code":"DAFTAR_ULANG","name":"Daftar Ulang (seragam, buku, MPLS)","amount_idr":2500000}]',
     '[]', 8750000, now() - interval '200 days', now() - interval '199 days',
     'Pembayaran minimum daftar ulang tidak terpenuhi sampai batas waktu (BR-PPDB-08)', interval '46 days'),
    ('a2300027-0000-0000-0000-000000000007', 'a2300023-0000-0000-0000-000000000009', 'reregistration', 17500000, 0,
     '[{"code":"UANG_PANGKAL","name":"Uang Pangkal","amount_idr":15000000},{"code":"DAFTAR_ULANG","name":"Daftar Ulang (seragam, buku, MPLS)","amount_idr":2500000}]',
     '[]', 8750000, now() - interval '200 days', now() - interval '205 days', 'Pendaftaran dibatalkan oleh orang tua', interval '46 days'),
    ('a2300027-0000-0000-0000-000000000008', 'a2300023-0000-0000-0000-000000000008', 'registration', 200000, 0,
     '[{"code":"FORM","name":"Formulir Pendaftaran","amount_idr":200000}]', '[]', 200000, now() - interval '18 days', NULL, NULL, interval '0'),
    ('a2300027-0000-0000-0000-000000000009', 'a2300023-0000-0000-0000-000000000006', 'registration', 200000, 0,
     '[{"code":"FORM","name":"Formulir Pendaftaran","amount_idr":200000}]', '[]', 200000, now() - interval '18 days', NULL, NULL, interval '0')
  ) AS v(id, app, stage, gross, disc, items, discs, minpay, due, cancelled, cancel_reason, after)
  JOIN applications a ON a.id = v.app::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO charge_installments (id, school_id, charge_id, application_id, sequence, description, amount_idr, due_at, created_at)
  SELECT v.id::uuid, v_school, c.id, c.application_id, v.seq, v.descr, v.amount, v.due, c.finalized_at
  FROM (VALUES
    ('a2300028-0000-0000-0000-000000000001', 'a2300027-0000-0000-0000-000000000001', 1, 'Termin 1', 200000::bigint, now() - interval '245 days'),
    ('a2300028-0000-0000-0000-000000000002', 'a2300027-0000-0000-0000-000000000002', 1, 'Termin 1 (minimum 50%)', 8125000, now() - interval '200 days'),
    ('a2300028-0000-0000-0000-000000000003', 'a2300027-0000-0000-0000-000000000002', 2, 'Termin 2 (pelunasan)', 8125000, now() - interval '150 days'),
    ('a2300028-0000-0000-0000-000000000004', 'a2300027-0000-0000-0000-000000000003', 1, 'Termin 1', 200000, now() - interval '245 days'),
    ('a2300028-0000-0000-0000-000000000005', 'a2300027-0000-0000-0000-000000000004', 1, 'Termin 1 (minimum 50%)', 8750000, now() - interval '200 days'),
    ('a2300028-0000-0000-0000-000000000006', 'a2300027-0000-0000-0000-000000000004', 2, 'Termin 2 (pelunasan)', 8750000, now() + interval '45 days'),
    ('a2300028-0000-0000-0000-000000000007', 'a2300027-0000-0000-0000-000000000005', 1, 'Termin 1', 200000, now() - interval '245 days'),
    ('a2300028-0000-0000-0000-000000000008', 'a2300027-0000-0000-0000-000000000006', 1, 'Termin 1 (minimum 50%)', 8750000, now() - interval '200 days'),
    ('a2300028-0000-0000-0000-000000000009', 'a2300027-0000-0000-0000-000000000006', 2, 'Termin 2 (pelunasan)', 8750000, now() - interval '150 days'),
    ('a2300028-0000-0000-0000-000000000010', 'a2300027-0000-0000-0000-000000000007', 1, 'Termin 1 (minimum 50%)', 8750000, now() - interval '200 days'),
    ('a2300028-0000-0000-0000-000000000011', 'a2300027-0000-0000-0000-000000000008', 1, 'Termin 1', 200000, now() - interval '18 days'),
    ('a2300028-0000-0000-0000-000000000012', 'a2300027-0000-0000-0000-000000000009', 1, 'Termin 1', 200000, now() - interval '18 days')
  ) AS v(id, charge, seq, descr, amount, due)
  JOIN charges c ON c.id = v.charge::uuid
  ON CONFLICT DO NOTHING;

  -- -------------------------------------------------------------------------
  -- 4. Bukti bayar, review, alokasi, reversal
  -- -------------------------------------------------------------------------
  INSERT INTO payment_submissions (id, school_id, application_id, charge_id, amount_idr, object_key, mime, size_bytes, sha256, status, uploader_type,
                                   uploader_id, idempotency_key, file_name, transfer_at, bank_name, account_name, reference, review_note, created_at, updated_at)
  SELECT v.id::uuid, v_school, c.application_id, c.id, v.amount, '', 'image/jpeg', 0, '', v.status, 'guardian', v.uploader,
         'seed-a23:pay:' || v.id, v.fname, c.finalized_at + v.after, v.bank, v.acct, v.ref, v.note, c.finalized_at + v.after, c.finalized_at + v.after + interval '1 day'
  FROM (VALUES
    ('a2300029-0000-0000-0000-000000000001', 'a2300027-0000-0000-0000-000000000001', 200000::bigint, 'verified', 'a2300020-0000-0000-0000-000000000001',
     'bukti_formulir_budi.jpg', 'Bank Mandiri', 'Santoso', 'TRF-0412-001', NULL, interval '1 hour'),
    ('a2300029-0000-0000-0000-000000000002', 'a2300027-0000-0000-0000-000000000002', 16250000, 'verified', 'a2300020-0000-0000-0000-000000000001',
     'bukti_daftar_ulang_budi.jpg', 'Bank Mandiri', 'Santoso', 'TRF-0412-002', NULL, interval '3 days'),
    ('a2300029-0000-0000-0000-000000000003', 'a2300027-0000-0000-0000-000000000003', 200000, 'verified', 'a2300020-0000-0000-0000-000000000002',
     'bukti_formulir_nadia.jpg', 'BCA', 'Ratna Dewi', 'TRF-0413-001', NULL, interval '2 hours'),
    ('a2300029-0000-0000-0000-000000000004', 'a2300027-0000-0000-0000-000000000004', 8750000, 'verified', 'a2300020-0000-0000-0000-000000000002',
     'bukti_termin1_nadia.jpg', 'BCA', 'Ratna Dewi', 'TRF-0413-002', NULL, interval '-210 days'),
    ('a2300029-0000-0000-0000-000000000005', 'a2300027-0000-0000-0000-000000000004', 2000000, 'pending_review', 'a2300020-0000-0000-0000-000000000002',
     'bukti_cicilan_nadia_okt.jpg', 'BCA', 'Ratna Dewi', 'TRF-0413-003', NULL, interval '-46 days'),
    ('a2300029-0000-0000-0000-000000000006', 'a2300027-0000-0000-0000-000000000005', 200000, 'verified', 'a2300020-0000-0000-0000-000000000003',
     'bukti_formulir_rizky.jpg', 'BRI', 'Hendra Saputra', 'TRF-0414-001', NULL, interval '1 hour'),
    ('a2300029-0000-0000-0000-000000000007', 'a2300027-0000-0000-0000-000000000006', 5000000, 'verified', 'a2300020-0000-0000-0000-000000000003',
     'bukti_dp_rizky.jpg', 'BRI', 'Hendra Saputra', 'TRF-0414-002', NULL, interval '-5 days'),
    ('a2300029-0000-0000-0000-000000000008', 'a2300027-0000-0000-0000-000000000006', 5000000, 'reversed', 'a2300020-0000-0000-0000-000000000003',
     'bukti_dp_rizky_dobel.jpg', 'BRI', 'Hendra Saputra', 'TRF-0414-002', 'Bukti ganda dengan transaksi TRF-0414-002', interval '-4 days'),
    ('a2300029-0000-0000-0000-000000000009', 'a2300027-0000-0000-0000-000000000007', 3000000, 'verified', 'a2300020-0000-0000-0000-000000000008',
     'bukti_dp_putri.jpg', 'BNI', 'Lina Marlina', 'TRF-0415-001', NULL, interval '-8 days'),
    ('a2300029-0000-0000-0000-000000000010', 'a2300027-0000-0000-0000-000000000008', 150000, 'rejected', 'a2300020-0000-0000-0000-000000000008',
     'bukti_formulir_kevin.jpg', 'BNI', 'Lina Marlina', 'TRF-0416-001', 'Nominal transfer kurang Rp50.000, mohon transfer kekurangannya.', interval '2 hours'),
    ('a2300029-0000-0000-0000-000000000011', 'a2300027-0000-0000-0000-000000000008', 200000, 'verified', 'a2300020-0000-0000-0000-000000000008',
     'bukti_formulir_kevin_2.jpg', 'BNI', 'Lina Marlina', 'TRF-0416-002', NULL, interval '1 day'),
    ('a2300029-0000-0000-0000-000000000012', 'a2300027-0000-0000-0000-000000000009', 200000, 'verified', 'a2300020-0000-0000-0000-000000000006',
     'bukti_formulir_aisyah.jpg', 'Bank Syariah Indonesia', 'Maya Lestari', 'TRF-0417-001', NULL, interval '3 hours')
  ) AS v(id, charge, amount, status, uploader, fname, bank, acct, ref, note, after)
  JOIN charges c ON c.id = v.charge::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO payment_reviews (id, school_id, payment_id, action, reviewer_id, reason, created_at)
  SELECT v.id::uuid, v_school, p.id, v.action, v_admin, v.reason, p.created_at + v.after
  FROM (VALUES
    ('a2300009-0000-0000-0000-000000000001', 'a2300029-0000-0000-0000-000000000001', 'verified', NULL, interval '4 hours'),
    ('a2300009-0000-0000-0000-000000000002', 'a2300029-0000-0000-0000-000000000002', 'verified', NULL, interval '1 day'),
    ('a2300009-0000-0000-0000-000000000003', 'a2300029-0000-0000-0000-000000000003', 'verified', NULL, interval '3 hours'),
    ('a2300009-0000-0000-0000-000000000004', 'a2300029-0000-0000-0000-000000000004', 'pending_review', 'Menunggu mutasi rekening BCA', interval '2 hours'),
    ('a2300009-0000-0000-0000-000000000005', 'a2300029-0000-0000-0000-000000000004', 'verified', NULL, interval '1 day'),
    ('a2300009-0000-0000-0000-000000000006', 'a2300029-0000-0000-0000-000000000006', 'verified', NULL, interval '2 hours'),
    ('a2300009-0000-0000-0000-000000000007', 'a2300029-0000-0000-0000-000000000007', 'verified', NULL, interval '1 day'),
    ('a2300009-0000-0000-0000-000000000008', 'a2300029-0000-0000-0000-000000000008', 'verified', 'Terverifikasi sebelum diketahui ganda', interval '1 day'),
    ('a2300009-0000-0000-0000-000000000009', 'a2300029-0000-0000-0000-000000000009', 'verified', NULL, interval '1 day'),
    ('a2300009-0000-0000-0000-000000000010', 'a2300029-0000-0000-0000-000000000010', 'rejected', 'Nominal transfer kurang Rp50.000', interval '5 hours'),
    ('a2300009-0000-0000-0000-000000000011', 'a2300029-0000-0000-0000-000000000011', 'verified', NULL, interval '3 hours'),
    ('a2300009-0000-0000-0000-000000000012', 'a2300029-0000-0000-0000-000000000012', 'verified', NULL, interval '2 hours')
  ) AS v(id, pay, action, reason, after)
  JOIN payment_submissions p ON p.id = v.pay::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO payment_allocations (id, school_id, payment_id, installment_id, charge_installment_id, amount_idr, created_at)
  SELECT v.id::uuid, v_school, p.id, NULL, ci.id, v.amount, p.created_at + interval '1 day'
  FROM (VALUES
    ('a2300010-0000-0000-0000-000000000001', 'a2300029-0000-0000-0000-000000000001', 'a2300028-0000-0000-0000-000000000001', 200000::bigint),
    ('a2300010-0000-0000-0000-000000000002', 'a2300029-0000-0000-0000-000000000002', 'a2300028-0000-0000-0000-000000000002', 8125000),
    ('a2300010-0000-0000-0000-000000000003', 'a2300029-0000-0000-0000-000000000002', 'a2300028-0000-0000-0000-000000000003', 8125000),
    ('a2300010-0000-0000-0000-000000000004', 'a2300029-0000-0000-0000-000000000003', 'a2300028-0000-0000-0000-000000000004', 200000),
    ('a2300010-0000-0000-0000-000000000005', 'a2300029-0000-0000-0000-000000000004', 'a2300028-0000-0000-0000-000000000005', 8750000),
    ('a2300010-0000-0000-0000-000000000006', 'a2300029-0000-0000-0000-000000000006', 'a2300028-0000-0000-0000-000000000007', 200000),
    ('a2300010-0000-0000-0000-000000000007', 'a2300029-0000-0000-0000-000000000007', 'a2300028-0000-0000-0000-000000000008', 5000000),
    ('a2300010-0000-0000-0000-000000000008', 'a2300029-0000-0000-0000-000000000009', 'a2300028-0000-0000-0000-000000000010', 3000000),
    ('a2300010-0000-0000-0000-000000000009', 'a2300029-0000-0000-0000-000000000011', 'a2300028-0000-0000-0000-000000000011', 200000),
    ('a2300010-0000-0000-0000-000000000010', 'a2300029-0000-0000-0000-000000000012', 'a2300028-0000-0000-0000-000000000012', 200000)
  ) AS v(id, pay, inst, amount)
  JOIN payment_submissions p ON p.id = v.pay::uuid
  JOIN charge_installments ci ON ci.id = v.inst::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO payment_reversals (id, school_id, payment_id, amount_idr, reason, actor_id, idempotency_key, created_at)
  SELECT 'a2300011-0000-0000-0000-000000000001', v_school, p.id, 5000000, 'Bukti transfer ganda (referensi TRF-0414-002 sudah tercatat)',
         v_admin, 'seed-a23:reversal:001', p.created_at + interval '2 days'
  FROM payment_submissions p WHERE p.id = 'a2300029-0000-0000-0000-000000000008'
  ON CONFLICT DO NOTHING;

  -- -------------------------------------------------------------------------
  -- 5. Seleksi: skor, formula, ranking, persetujuan hasil, keputusan
  -- -------------------------------------------------------------------------
  INSERT INTO component_scores (id, school_id, application_id, component_id, score, final, source_ref, entered_by, created_at, updated_at)
  SELECT md5('a23-score:' || v.app || ':' || v.comp)::uuid, v_school, a.id, v.comp::uuid, v.score, true, NULL, v_admin,
         a.created_at + interval '30 days', a.created_at + interval '30 days'
  FROM (VALUES
    ('a2300023-0000-0000-0000-000000000001', 'a2300000-0000-0000-0000-000000000051', 8800::bigint),
    ('a2300023-0000-0000-0000-000000000001', 'a2300000-0000-0000-0000-000000000052', 8650),
    ('a2300023-0000-0000-0000-000000000001', 'a2300000-0000-0000-0000-000000000053', 8400),
    ('a2300023-0000-0000-0000-000000000002', 'a2300000-0000-0000-0000-000000000051', 8300),
    ('a2300023-0000-0000-0000-000000000002', 'a2300000-0000-0000-0000-000000000052', 8700),
    ('a2300023-0000-0000-0000-000000000002', 'a2300000-0000-0000-0000-000000000053', 8200),
    ('a2300023-0000-0000-0000-000000000004', 'a2300000-0000-0000-0000-000000000051', 7000),
    ('a2300023-0000-0000-0000-000000000004', 'a2300000-0000-0000-0000-000000000052', 7300),
    ('a2300023-0000-0000-0000-000000000004', 'a2300000-0000-0000-0000-000000000053', 7000),
    ('a2300023-0000-0000-0000-000000000006', 'a2300000-0000-0000-0000-000000000051', 8100),
    ('a2300023-0000-0000-0000-000000000006', 'a2300000-0000-0000-0000-000000000052', 8500),
    ('a2300023-0000-0000-0000-000000000006', 'a2300000-0000-0000-0000-000000000053', 8000),
    ('a2300023-0000-0000-0000-000000000007', 'a2300000-0000-0000-0000-000000000051', 7600),
    ('a2300023-0000-0000-0000-000000000007', 'a2300000-0000-0000-0000-000000000052', 7900),
    ('a2300023-0000-0000-0000-000000000007', 'a2300000-0000-0000-0000-000000000053', 7500)
  ) AS v(app, comp, score)
  JOIN applications a ON a.id = v.app::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO selection_formula_versions (id, school_id, period_id, wave_id, track_id, version, formula, status, checksum, created_at)
  SELECT v.id::uuid, v_school, v_period, v.wave::uuid, v.track::uuid, 1, v.formula::jsonb, 'active', md5(v.formula), now() - v.age
  FROM (VALUES
    ('a2300016-0000-0000-0000-000000000001', 'a2300000-0000-0000-0000-000000000011', 'a2300000-0000-0000-0000-000000000021',
     '{"name":"Formula Reguler 2026","version":1,"status":"active","minimum_total":7500,"components":[{"code":"TES","name":"Tes Potensi Akademik","source":"exam","weight":5000,"minimum":6000},{"code":"RAPOR","name":"Rata-rata Nilai Rapor","source":"manual","weight":3000,"minimum":7000},{"code":"WAWANCARA","name":"Wawancara","source":"manual","weight":2000,"minimum":0}],"tie_breakers":["TES","RAPOR"]}',
     interval '245 days'),
    ('a2300016-0000-0000-0000-000000000002', 'a2300000-0000-0000-0000-000000000011', 'a2300000-0000-0000-0000-000000000022',
     '{"name":"Formula Prestasi 2026","version":1,"status":"active","minimum_total":7000,"components":[{"code":"RAPOR","name":"Rata-rata Nilai Rapor","source":"manual","weight":6000,"minimum":8000},{"code":"WAWANCARA","name":"Wawancara","source":"manual","weight":4000,"minimum":0}],"tie_breakers":["RAPOR"]}',
     interval '245 days'),
    ('a2300016-0000-0000-0000-000000000003', 'a2300000-0000-0000-0000-000000000012', 'a2300000-0000-0000-0000-000000000021',
     '{"name":"Formula Reguler Susulan","version":1,"status":"active","minimum_total":7500,"components":[{"code":"TES","name":"Tes Potensi Akademik","source":"exam","weight":5000,"minimum":6000},{"code":"RAPOR","name":"Rata-rata Nilai Rapor","source":"manual","weight":3000,"minimum":7000},{"code":"WAWANCARA","name":"Wawancara","source":"manual","weight":2000,"minimum":0}],"tie_breakers":["TES","RAPOR"]}',
     interval '16 days')
  ) AS v(id, wave, track, formula, age)
  ON CONFLICT DO NOTHING;

  INSERT INTO ranking_runs (id, school_id, period_id, wave_id, track_id, formula_version_id, version, input_checksum, status, created_by, created_at, published_at)
  SELECT v.id::uuid, v_school, v_period, f.wave_id, f.track_id, f.id, 1, md5(v.id), 'final', v_admin, now() - v.age, v.published
  FROM (VALUES
    ('a2300014-0000-0000-0000-000000000001', 'a2300016-0000-0000-0000-000000000001', interval '232 days', now() - interval '230 days'),
    ('a2300014-0000-0000-0000-000000000002', 'a2300016-0000-0000-0000-000000000002', interval '232 days', now() - interval '230 days'),
    ('a2300014-0000-0000-0000-000000000003', 'a2300016-0000-0000-0000-000000000003', interval '3 days', NULL::timestamptz)
  ) AS v(id, formula, age, published)
  JOIN selection_formula_versions f ON f.id = v.formula::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO ranking_entries (school_id, ranking_run_id, application_id, rank, total_score, scores_snapshot, recommendation)
  SELECT v_school, r.id, a.id, v.rnk, v.total, v.snap::jsonb, v.rec
  FROM (VALUES
    ('a2300014-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000001', 1, 8675::bigint, '{"TES":8800,"RAPOR":8650,"WAWANCARA":8400}', 'accepted'),
    ('a2300014-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000002', 2, 8400, '{"TES":8300,"RAPOR":8700,"WAWANCARA":8200}', 'accepted'),
    ('a2300014-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000009', 3, 8010, '{"TES":8000,"RAPOR":8100,"WAWANCARA":7900}', 'accepted'),
    ('a2300014-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000004', 4, 7090, '{"TES":7000,"RAPOR":7300,"WAWANCARA":7000}', 'waitlisted'),
    ('a2300014-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000003', 1, 8520, '{"RAPOR":9000,"WAWANCARA":7800}', 'accepted'),
    ('a2300014-0000-0000-0000-000000000003', 'a2300023-0000-0000-0000-000000000006', 1, 8250, '{"TES":8100,"RAPOR":8500,"WAWANCARA":8000}', 'accepted'),
    ('a2300014-0000-0000-0000-000000000003', 'a2300023-0000-0000-0000-000000000007', 2, 7670, '{"TES":7600,"RAPOR":7900,"WAWANCARA":7500}', 'accepted')
  ) AS v(run, app, rnk, total, snap, rec)
  JOIN ranking_runs r ON r.id = v.run::uuid
  JOIN applications a ON a.id = v.app::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO selection_approvals (id, school_id, period_id, wave_id, status, approval_id, decision_count, created_by, created_by_name,
                                   decided_by, decided_comment, decided_at, created_at)
  VALUES
    ('a2300013-0000-0000-0000-000000000001', v_school, v_period, v_w2, 'pending', 'a2100003-0000-0000-0000-000000000003', 2, v_admin, 'Admin Teratai',
     NULL, NULL, NULL, now() - interval '2 days'),
    ('a2300013-0000-0000-0000-000000000002', v_school, v_period, v_w1, 'approved', 'a2100003-0000-0000-0000-000000000017', 6, v_admin, 'Admin Teratai',
     v_kepsek, 'Hasil seleksi sesuai rapat panitia.', now() - interval '231 days', now() - interval '232 days'),
    ('a2300013-0000-0000-0000-000000000003', v_school, v_period, v_w1, 'rejected', NULL, 6, v_admin, 'Admin Teratai',
     v_kepsek, 'Jumlah diterima jalur reguler melebihi kuota, mohon revisi.', now() - interval '233 days', now() - interval '234 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO decisions (id, school_id, application_id, ranking_entry_id, decision, override, reason, actor_id, version, created_at, status,
                         selection_approval_id, published_at)
  SELECT v.id::uuid, v_school, a.id, NULL, v.decision, v.override, v.reason, v_admin, 1, now() - v.age, v.status, v.sa::uuid, v.published
  FROM (VALUES
    ('a2300030-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000001', 'offered', false, NULL, interval '232 days', 'published', 'a2300013-0000-0000-0000-000000000002', now() - interval '230 days'),
    ('a2300030-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000002', 'offered', false, NULL, interval '232 days', 'published', 'a2300013-0000-0000-0000-000000000002', now() - interval '230 days'),
    ('a2300030-0000-0000-0000-000000000003', 'a2300023-0000-0000-0000-000000000003', 'offered', false, NULL, interval '232 days', 'published', 'a2300013-0000-0000-0000-000000000002', now() - interval '230 days'),
    ('a2300030-0000-0000-0000-000000000004', 'a2300023-0000-0000-0000-000000000004', 'waitlisted', false, NULL, interval '232 days', 'published', 'a2300013-0000-0000-0000-000000000002', now() - interval '230 days'),
    ('a2300030-0000-0000-0000-000000000005', 'a2300023-0000-0000-0000-000000000005', 'rejected', true, 'Nilai tes di bawah batas minimum 6000', interval '232 days', 'published', 'a2300013-0000-0000-0000-000000000002', now() - interval '230 days'),
    ('a2300030-0000-0000-0000-000000000006', 'a2300023-0000-0000-0000-000000000009', 'offered', false, NULL, interval '232 days', 'published', 'a2300013-0000-0000-0000-000000000002', now() - interval '230 days'),
    ('a2300030-0000-0000-0000-000000000007', 'a2300023-0000-0000-0000-000000000006', 'offered', false, NULL, interval '2 days', 'pending_approval', 'a2300013-0000-0000-0000-000000000001', NULL),
    ('a2300030-0000-0000-0000-000000000008', 'a2300023-0000-0000-0000-000000000007', 'offered', false, NULL, interval '2 days', 'pending_approval', 'a2300013-0000-0000-0000-000000000001', NULL)
  ) AS v(id, app, decision, override, reason, age, status, sa, published)
  JOIN applications a ON a.id = v.app::uuid
  JOIN selection_approvals s ON s.id = v.sa::uuid
  ON CONFLICT DO NOTHING;

  -- Callback approval yang sudah diproses (keputusan dari core)
  INSERT INTO approval_callback_events (approval_id, school_id, request_type, decision, received_at)
  VALUES
    ('a2100003-0000-0000-0000-000000000016', v_school, 'discount_special', 'approved', now() - interval '228 days'),
    ('a2100003-0000-0000-0000-000000000017', v_school, 'selection_result', 'approved', now() - interval '231 days')
  ON CONFLICT DO NOTHING;

  -- Pengumuman (tidak ada yang 'approved' agar job publikasi tidak mengirim)
  INSERT INTO ppdb_announcements (id, school_id, period_id, wave_id, title, message, publish_at, status, created_by, created_by_name, approved_by,
                                  approved_by_name, approved_at, published_at, notified_count, selection_approval_id, cancelled_reason, created_at, updated_at)
  VALUES
    ('a2300012-0000-0000-0000-000000000001', v_school, v_period, v_w1, 'Pengumuman Hasil Seleksi Gelombang 1',
     'Selamat kepada calon siswa yang diterima. Silakan lakukan daftar ulang melalui portal PPDB sebelum batas waktu.',
     now() - interval '230 days', 'published', v_admin, 'Admin Teratai', v_kepsek, 'Kepala Sekolah Teratai', now() - interval '231 days',
     now() - interval '230 days', 6, 'a2300013-0000-0000-0000-000000000002', NULL, now() - interval '231 days', now() - interval '230 days'),
    ('a2300012-0000-0000-0000-000000000002', v_school, v_period, v_w2, 'Pengumuman Hasil Seleksi Gelombang Susulan',
     'Hasil seleksi gelombang susulan akan diumumkan setelah disetujui kepala sekolah.',
     now() + interval '5 days', 'draft', v_admin, 'Admin Teratai', NULL, NULL, NULL, NULL, 0, 'a2300013-0000-0000-0000-000000000001', NULL,
     now() - interval '2 days', now() - interval '2 days'),
    ('a2300012-0000-0000-0000-000000000003', v_school, v_period, v_w1, 'Pengumuman Hasil Seleksi Gelombang 1 (revisi kuota)',
     'Draf pengumuman sebelum revisi kuota jalur reguler.',
     now() - interval '232 days', 'cancelled', v_admin, 'Admin Teratai', NULL, NULL, NULL, NULL, 0, 'a2300013-0000-0000-0000-000000000003',
     'Persetujuan hasil seleksi ditolak, diganti pengumuman baru.', now() - interval '234 days', now() - interval '233 days'),
    ('a2300012-0000-0000-0000-000000000004', v_school, v_period, NULL, 'Jadwal Tes Gelombang Susulan',
     'Tes potensi akademik gelombang susulan dilaksanakan Sabtu pukul 08.00 WIB di Lab Komputer.',
     now() - interval '17 days', 'published', v_admin, 'Admin Teratai', v_kepsek, 'Kepala Sekolah Teratai', now() - interval '18 days',
     now() - interval '17 days', 3, NULL, NULL, now() - interval '18 days', now() - interval '17 days')
  ON CONFLICT DO NOTHING;

  -- Daftar cadangan: promosi yang ditolak dan yang gagal (kursi penuh)
  INSERT INTO waitlist_promotions (id, school_id, application_id, period_id, wave_id, track_id, total_score, status, approval_id, created_by,
                                   created_by_name, decided_by, decided_comment, decided_at, created_at)
  SELECT v.id::uuid, v_school, a.id, v_period, a.wave_id, a.track_id, 7090, v.status, NULL, v_admin, 'Admin Teratai', v_kepsek, v.comment,
         now() - v.age + interval '1 day', now() - v.age
  FROM (VALUES
    ('a2300033-0000-0000-0000-000000000001', 'failed', 'Kursi sudah terisi kembali saat persetujuan diproses.', interval '205 days'),
    ('a2300033-0000-0000-0000-000000000002', 'rejected', 'Menunggu hasil daftar ulang gelombang susulan.', interval '30 days')
  ) AS v(id, status, comment, age)
  JOIN applications a ON a.id = 'a2300023-0000-0000-0000-000000000004'
  ON CONFLICT DO NOTHING;

  -- Jadwal tes/wawancara per pendaftar
  INSERT INTO selection_schedules (id, school_id, application_id, kind, scheduled_at, location, notes, created_by, created_by_name, created_at, updated_at)
  SELECT v.id::uuid, v_school, a.id, v.kind, now() + v.at, v.loc, v.notes, v_admin, 'Admin Teratai', now() - interval '20 days', now() - interval '20 days'
  FROM (VALUES
    ('a2300032-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000006', 'test', interval '-15 days', 'Lab Komputer Lt. 2', 'Bawa kartu peserta'),
    ('a2300032-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000006', 'interview', interval '-14 days', 'Ruang BK', 'Didampingi orang tua'),
    ('a2300032-0000-0000-0000-000000000003', 'a2300023-0000-0000-0000-000000000007', 'test', interval '-15 days', 'Lab Komputer Lt. 2', 'Bawa kartu peserta'),
    ('a2300032-0000-0000-0000-000000000004', 'a2300023-0000-0000-0000-000000000007', 'interview', interval '-14 days', 'Ruang BK', 'Didampingi orang tua'),
    ('a2300032-0000-0000-0000-000000000005', 'a2300023-0000-0000-0000-000000000008', 'interview', interval '6 days', 'Ruang Kepala Sekolah', 'Jalur prestasi: bawa piagam asli')
  ) AS v(id, app, kind, at, loc, notes)
  JOIN applications a ON a.id = v.app::uuid
  ON CONFLICT DO NOTHING;

  -- -------------------------------------------------------------------------
  -- 6. Refund, daftar ulang, konversi siswa
  -- -------------------------------------------------------------------------
  INSERT INTO admission_refunds (id, school_id, application_id, stage, paid_idr, refundable_idr, policy_bp, admin_fee_idr, status, method,
                                 bank_name, account_name, account_number, reference, paid_at, note, processed_by, processed_by_name, processed_at, created_at)
  SELECT v.id::uuid, v_school, a.id, 'reregistration', v.paid, v.refundable, 7500, 100000, v.status, v.method, v.bank, v.acct, v.acctno, v.ref,
         v.paid_at, v.note, v.proc, v.proc_name, v.paid_at, now() - v.age
  FROM (VALUES
    ('a2300035-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000003', 5000000::bigint, 3650000::bigint, 'due', NULL, NULL, NULL, NULL, NULL,
     NULL::timestamptz, 'Menunggu data rekening dari orang tua', NULL, NULL, interval '199 days'),
    ('a2300035-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000009', 3000000, 2150000, 'paid', 'transfer', 'BNI', 'Lina Marlina', '****4321',
     'RFD-2026-0007', now() - interval '190 days', 'Dana dikembalikan sesuai kebijakan 75% dikurangi biaya administrasi', v_admin, 'Admin Teratai', interval '204 days')
  ) AS v(id, app, paid, refundable, status, method, bank, acct, acctno, ref, paid_at, note, proc, proc_name, age)
  JOIN applications a ON a.id = v.app::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO reregistration_checklists (id, school_id, period_id, name, min_payment_idr, created_at)
  VALUES ('a2300017-0000-0000-0000-000000000001', v_school, v_period, 'Daftar Ulang Kelas 7 - 2026/2027', 8750000, now() - interval '240 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO reregistration_items (id, school_id, checklist_id, code, label, required)
  SELECT v.id::uuid, v_school, c.id, v.code, v.label, v.req
  FROM (VALUES
    ('a2300018-0000-0000-0000-000000000001', 'SKL', 'Unggah Surat Keterangan Lulus SD', true),
    ('a2300018-0000-0000-0000-000000000002', 'SERAGAM', 'Pengukuran seragam di koperasi sekolah', true),
    ('a2300018-0000-0000-0000-000000000003', 'FOTO', 'Pas foto 3x4 berlatar merah (2 lembar)', true),
    ('a2300018-0000-0000-0000-000000000004', 'MPLS', 'Konfirmasi kehadiran MPLS', true),
    ('a2300018-0000-0000-0000-000000000005', 'KESEHATAN', 'Formulir riwayat kesehatan untuk UKS', false)
  ) AS v(id, code, label, req)
  JOIN reregistration_checklists c ON c.id = 'a2300017-0000-0000-0000-000000000001'
  ON CONFLICT DO NOTHING;

  INSERT INTO application_reregistration_items (id, school_id, application_id, item_id, completed_at, completed_by, notes)
  SELECT v.id::uuid, v_school, a.id, i.id, v.done, v.by, v.notes
  FROM (VALUES
    ('a2300039-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000001', 'a2300018-0000-0000-0000-000000000001', now() - interval '195 days', '22222222-2222-2222-2222-222222222222', NULL),
    ('a2300039-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000001', 'a2300018-0000-0000-0000-000000000002', now() - interval '196 days', '22222222-2222-2222-2222-222222222222', 'Ukuran M'),
    ('a2300039-0000-0000-0000-000000000003', 'a2300023-0000-0000-0000-000000000001', 'a2300018-0000-0000-0000-000000000003', now() - interval '196 days', '22222222-2222-2222-2222-222222222222', NULL),
    ('a2300039-0000-0000-0000-000000000004', 'a2300023-0000-0000-0000-000000000001', 'a2300018-0000-0000-0000-000000000004', now() - interval '190 days', '22222222-2222-2222-2222-222222222222', NULL),
    ('a2300039-0000-0000-0000-000000000005', 'a2300023-0000-0000-0000-000000000002', 'a2300018-0000-0000-0000-000000000002', now() - interval '20 days', '22222222-2222-2222-2222-222222222222', 'Ukuran S'),
    ('a2300039-0000-0000-0000-000000000006', 'a2300023-0000-0000-0000-000000000002', 'a2300018-0000-0000-0000-000000000001', NULL, NULL, 'SKL belum terbit dari sekolah asal')
  ) AS v(id, app, item, done, by, notes)
  JOIN applications a ON a.id = v.app::uuid
  JOIN reregistration_items i ON i.id = v.item::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO student_conversions (id, school_id, application_id, idempotency_key, status, guardian_step, student_step, link_step, enrollment_step,
                                   identity_guardian_id, student_user_id, enrollment_id, last_error, attempts, guardian_link_id, current_step, created_at, updated_at)
  SELECT 'a2300019-0000-0000-0000-000000000001', v_school, a.id, 'seed-a23:conversion:budi', 'completed',
         '{"status":"done","note":"Akun wali sudah ada (wali@teratai.sch.id)"}', '{"status":"done","note":"Akun siswa Budi Santoso"}',
         '{"status":"done"}', '{"status":"done","classroom":"Kelas 7A"}',
         '33333333-3333-3333-3333-333333333333', '44444444-4444-4444-4444-444444444444', NULL, NULL, 1, NULL, 'enrollment',
         now() - interval '185 days', now() - interval '185 days'
  FROM applications a WHERE a.id = 'a2300023-0000-0000-0000-000000000001'
  ON CONFLICT DO NOTHING;

  -- -------------------------------------------------------------------------
  -- 7. Notifikasi wali PPDB (email sudah terkirim/dilewati, tidak ada 'pending')
  -- -------------------------------------------------------------------------
  INSERT INTO guardian_notifications (id, school_id, guardian_id, application_id, type, title, body, data, dedupe_key, email_status, email_attempts,
                                      email_error, read_at, created_at)
  SELECT v.id::uuid, v_school, g.id, v.app::uuid, v.type, v.title, v.body, '{}', 'seed-a23:' || v.id, v.email, v.attempts, v.err, v.read_at, now() - v.age
  FROM (VALUES
    ('a2300034-0000-0000-0000-000000000001', 'a2300020-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000001', 'decision',
     'Selamat! Budi Santoso diterima', 'Budi Santoso dinyatakan diterima di Jalur Reguler. Silakan lakukan daftar ulang.', 'sent', 1, NULL,
     now() - interval '229 days', interval '230 days'),
    ('a2300034-0000-0000-0000-000000000002', 'a2300020-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000001', 'converted',
     'Budi Santoso resmi menjadi siswa', 'Akun portal siswa dan orang tua sudah aktif. Selamat bergabung di Sekolah Teratai.', 'sent', 1, NULL,
     now() - interval '184 days', interval '185 days'),
    ('a2300034-0000-0000-0000-000000000003', 'a2300020-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000002', 'payment_verified',
     'Pembayaran termin 1 terverifikasi', 'Pembayaran Rp8.750.000 untuk daftar ulang Nadia Putri sudah diverifikasi.', 'sent', 1, NULL,
     NULL, interval '209 days'),
    ('a2300034-0000-0000-0000-000000000004', 'a2300020-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000002', 'special_discount',
     'Pengajuan diskon khusus sedang diproses', 'Pengajuan diskon anak pegawai untuk Nadia Putri menunggu persetujuan kepala sekolah.', 'skipped', 0,
     NULL, NULL, interval '3 days'),
    ('a2300034-0000-0000-0000-000000000005', 'a2300020-0000-0000-0000-000000000003', 'a2300023-0000-0000-0000-000000000003', 'dropped',
     'Pendaftaran gugur', 'Pendaftaran dinyatakan gugur: pembayaran minimum daftar ulang tidak terpenuhi. Dana Rp3.650.000 akan dikembalikan panitia.',
     'failed', 5, 'Alamat email tidak tersedia', NULL, interval '199 days'),
    ('a2300034-0000-0000-0000-000000000006', 'a2300020-0000-0000-0000-000000000006', 'a2300023-0000-0000-0000-000000000006', 'schedule',
     'Jadwal tes dan wawancara', 'Tes: Lab Komputer Lt. 2; wawancara: Ruang BK. Mohon hadir 15 menit sebelumnya.', 'sent', 1, NULL,
     now() - interval '19 days', interval '20 days'),
    ('a2300034-0000-0000-0000-000000000007', 'a2300020-0000-0000-0000-000000000008', 'a2300023-0000-0000-0000-000000000008', 'document_correction',
     'Berkas perlu diperbaiki', 'Akta kelahiran Kevin Halim buram. Mohon unggah ulang hasil scan.', 'sent', 1, NULL,
     now() - interval '18 days', interval '19 days')
  ) AS v(id, guardian, app, type, title, body, email, attempts, err, read_at, age)
  JOIN guardians g ON g.id = v.guardian::uuid
  ON CONFLICT DO NOTHING;

  -- -------------------------------------------------------------------------
  -- 8. Landing page, kunjungan, referral, atribusi
  -- -------------------------------------------------------------------------
  INSERT INTO landing_pages (id, school_id, period_id, wave_id, slug, title, template_code, content, status, published_at, created_by, created_at, updated_at, deleted_at)
  VALUES
    ('a2300002-0000-0000-0000-000000000001', v_school, v_period, v_w1, 'ppdb-teratai-jalur-awal', 'PPDB Sekolah Teratai 2026/2027', 'hero_cta',
     '{"hero":{"headline":"Bergabung bersama Sekolah Teratai","subheadline":"Pendaftaran Jalur Awal SMP kelas 7 dibuka","cta":"Daftar Sekarang"}}',
     'published', now() - interval '280 days', v_admin, now() - interval '285 days', now() - interval '280 days', NULL),
    ('a2300002-0000-0000-0000-000000000002', v_school, v_period, v_w2, 'program-unggulan-teratai', 'Program Unggulan Sekolah Teratai', 'program_grid',
     '{"programs":[{"title":"Robotik & Coding"},{"title":"Bilingual Class"},{"title":"Tahfidz"}]}',
     'published', now() - interval '45 days', v_admin, now() - interval '46 days', now() - interval '45 days', NULL),
    ('a2300002-0000-0000-0000-000000000003', v_school, v_period, NULL, 'faq-ppdb-teratai', 'Tanya Jawab PPDB', 'faq_testimonial',
     '{"faq":[{"q":"Kapan tes dilaksanakan?","a":"Sesuai jadwal gelombang."}],"testimonials":[]}',
     'draft', NULL, v_admin, now() - interval '5 days', now() - interval '5 days', NULL),
    ('a2300002-0000-0000-0000-000000000004', v_school, v_period, NULL, 'open-house-2025', 'Open House 2025', 'hero_cta',
     '{"hero":{"headline":"Open House 2025"}}', 'archived', now() - interval '400 days', v_admin, now() - interval '410 days', now() - interval '300 days', NULL)
  ON CONFLICT DO NOTHING;

  INSERT INTO referral_programs (id, school_id, period_id, name, reward_kind, reward_value, reward_cap_idr, reward_valid_days, priority, active, created_by, created_at, updated_at)
  VALUES
    ('a2300004-0000-0000-0000-000000000001', v_school, v_period, 'Ajak Teman Bergabung', 'fixed', 500000, NULL, 90, 40, true, v_admin, now() - interval '285 days', now() - interval '285 days'),
    ('a2300004-0000-0000-0000-000000000002', v_school, v_period, 'Referral Alumni 10%', 'percent', 1000, 1000000, 60, 45, false, v_admin, now() - interval '300 days', now() - interval '250 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO referral_codes (id, school_id, code, owner_user_id, owner_kind, status, created_at, revoked_at)
  VALUES
    ('a2300005-0000-0000-0000-000000000001', v_school, 'TERATAI-BUDI7A', '33333333-3333-3333-3333-333333333333', 'parent', 'active', now() - interval '280 days', NULL),
    ('a2300005-0000-0000-0000-000000000002', v_school, 'TERATAI-WK7A', '66666666-6666-6666-6666-666666666666', 'parent', 'revoked', now() - interval '280 days', now() - interval '210 days')
  ON CONFLICT DO NOTHING;

  INSERT INTO landing_visits (id, school_id, landing_page_id, visitor_hash, utm_source, utm_medium, utm_campaign, referral_code_id, visited_on, created_at)
  SELECT v.id::uuid, v_school, lp.id, md5('a23-visitor-' || v.n), v.src, v.med, v.camp, rc.id, (now() - v.age)::date, now() - v.age
  FROM (VALUES
    ('a2300003-0000-0000-0000-000000000001', 1, 'a2300002-0000-0000-0000-000000000001', 'instagram', 'social', 'ppdb2026', NULL, interval '278 days'),
    ('a2300003-0000-0000-0000-000000000002', 2, 'a2300002-0000-0000-0000-000000000001', 'google', 'cpc', 'ppdb2026', NULL, interval '275 days'),
    ('a2300003-0000-0000-0000-000000000003', 3, 'a2300002-0000-0000-0000-000000000001', 'whatsapp', 'referral', 'ajak-teman', 'a2300005-0000-0000-0000-000000000001', interval '272 days'),
    ('a2300003-0000-0000-0000-000000000004', 4, 'a2300002-0000-0000-0000-000000000001', 'facebook', 'social', 'ppdb2026', NULL, interval '270 days'),
    ('a2300003-0000-0000-0000-000000000005', 5, 'a2300002-0000-0000-0000-000000000002', 'instagram', 'social', 'susulan', NULL, interval '42 days'),
    ('a2300003-0000-0000-0000-000000000006', 6, 'a2300002-0000-0000-0000-000000000002', 'whatsapp', 'broadcast', 'susulan', NULL, interval '36 days'),
    ('a2300003-0000-0000-0000-000000000007', 1, 'a2300002-0000-0000-0000-000000000002', 'instagram', 'social', 'susulan', NULL, interval '41 days')
  ) AS v(id, n, lp, src, med, camp, rc, age)
  JOIN landing_pages lp ON lp.id = v.lp::uuid
  LEFT JOIN referral_codes rc ON rc.id = v.rc::uuid
  ON CONFLICT DO NOTHING;

  INSERT INTO application_attributions (application_id, school_id, landing_page_id, referral_code_id, utm_source, utm_medium, utm_campaign, created_at)
  SELECT a.id, v_school, lp.id, rc.id, v.src, v.med, v.camp, a.created_at
  FROM (VALUES
    ('a2300023-0000-0000-0000-000000000001', 'a2300002-0000-0000-0000-000000000001', NULL, 'instagram', 'social', 'ppdb2026'),
    ('a2300023-0000-0000-0000-000000000002', 'a2300002-0000-0000-0000-000000000001', 'a2300005-0000-0000-0000-000000000001', 'whatsapp', 'referral', 'ajak-teman'),
    ('a2300023-0000-0000-0000-000000000003', 'a2300002-0000-0000-0000-000000000001', NULL, 'google', 'cpc', 'ppdb2026'),
    ('a2300023-0000-0000-0000-000000000006', 'a2300002-0000-0000-0000-000000000002', NULL, 'instagram', 'social', 'susulan'),
    ('a2300023-0000-0000-0000-000000000007', NULL, NULL, 'whatsapp', 'broadcast', 'susulan'),
    ('a2300023-0000-0000-0000-000000000009', NULL, 'a2300005-0000-0000-0000-000000000002', NULL, NULL, NULL)
  ) AS v(app, lp, rc, src, med, camp)
  JOIN applications a ON a.id = v.app::uuid
  LEFT JOIN landing_pages lp ON lp.id = v.lp::uuid
  LEFT JOIN referral_codes rc ON rc.id = v.rc::uuid
  ON CONFLICT DO NOTHING;

  -- Hadiah referral tanpa status 'issued' (tidak menerbitkan voucher sungguhan)
  INSERT INTO referral_rewards (id, school_id, referral_code_id, application_id, program_id, status, void_reason, voucher_id, voucher_code, issued_at,
                                idempotency_key, created_at, updated_at)
  SELECT v.id::uuid, v_school, rc.id, a.id, v.prog::uuid, v.status, v.reason, NULL, NULL, NULL, 'seed-a23:reward:' || v.id, a.created_at, a.created_at + interval '60 days'
  FROM (VALUES
    ('a2300006-0000-0000-0000-000000000001', 'a2300005-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000002', 'a2300004-0000-0000-0000-000000000001', 'pending_reregistration', NULL),
    ('a2300006-0000-0000-0000-000000000002', 'a2300005-0000-0000-0000-000000000002', 'a2300023-0000-0000-0000-000000000009', 'a2300004-0000-0000-0000-000000000001', 'void', 'application_withdrawn'),
    ('a2300006-0000-0000-0000-000000000003', 'a2300005-0000-0000-0000-000000000001', 'a2300023-0000-0000-0000-000000000001', NULL, 'void', 'self_referral')
  ) AS v(id, rc, app, prog, status, reason)
  JOIN referral_codes rc ON rc.id = v.rc::uuid
  JOIN applications a ON a.id = v.app::uuid
  ON CONFLICT DO NOTHING;

  -- -------------------------------------------------------------------------
  -- 9. Jejak audit (bigserial: dijaga NOT EXISTS)
  -- -------------------------------------------------------------------------
  INSERT INTO audit_events (school_id, aggregate_type, aggregate_id, action, actor_type, actor_id, actor_name, before_data, after_data, reason, correlation_id, created_at)
  SELECT v_school, v.agg, v.agg_id, v.action, 'admin', v_admin, 'Admin Teratai', NULL, v.after_data::jsonb, v.reason, 'seed-a23', now() - v.age
  FROM (VALUES
    ('manual_discount', 'a2300008-0000-0000-0000-000000000001', 'special_discount_requested', '{"percent":50,"approval_id":"a2100003-0000-0000-0000-000000000004"}', 'Anak pegawai', interval '3 days'),
    ('selection_approval', 'a2300013-0000-0000-0000-000000000001', 'selection_result_submitted', '{"wave":"Gelombang Susulan","decisions":2}', NULL, interval '2 days'),
    ('payment', 'a2300029-0000-0000-0000-000000000008', 'payment_reversed', '{"amount_idr":5000000}', 'Bukti transfer ganda', interval '205 days'),
    ('refund', 'a2300035-0000-0000-0000-000000000002', 'refund_paid', '{"amount_idr":2150000,"method":"transfer"}', NULL, interval '190 days'),
    ('landing_page', 'a2300002-0000-0000-0000-000000000003', 'landing_page_created', '{"slug":"faq-ppdb-teratai","status":"draft"}', NULL, interval '5 days')
  ) AS v(agg, agg_id, action, after_data, reason, age)
  WHERE NOT EXISTS (SELECT 1 FROM audit_events e WHERE e.school_id = v_school AND e.aggregate_id = v.agg_id AND e.action = v.action);
END
$seed$;
