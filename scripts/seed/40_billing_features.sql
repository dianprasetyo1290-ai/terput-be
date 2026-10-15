-- 40_billing_features.sql
-- Seed ADDITIF fitur billing (DB teratai_billing) untuk sekolah demo
-- Teratai Putih Global School. Melengkapi 03_billing.sql.
--
-- Isi: skema & komponen, tagihan Budi (cicilan, diskon, ad-hoc, batal,
-- penyesuaian), pembayaran (unggah bukti, kasir, payment gateway, deposit,
-- impor), deposit, refund, rekonsiliasi & settlement exception, promo/diskon,
-- pengingat, mapping COA per sekolah, setelan jurnal, setelan gateway
-- (tanpa kredensial), dompet uang saku & tabungan, kartu siswa, kantin
-- (tenant, menu, operator, transaksi POS, settlement), autodebit.
--
-- Aturan: hanya INSERT. Semua baris memakai UUID tetap berawalan 'b4'
-- (lihat pg_temp.u) dan ON CONFLICT DO NOTHING, sehingga aman dijalankan
-- berulang. Tidak ada DELETE/TRUNCATE/DROP/UPDATE terhadap baris lain, dan
-- tidak ada ON CONFLICT ... DO UPDATE (semua DO NOTHING).
--
-- Konsistensi angka:
--   * wallets.balance_idr = jumlah wallet_movements; balance_after berurutan.
--   * student_deposits.balance_idr = jumlah deposit_movements.
--   * status tagihan = status turunan ledger (alokasi terverifikasi - reversal).
--   * Hanya SATU promo/diskon aktif (D1, khusus skema Robotik seed ini).

BEGIN;

CREATE OR REPLACE FUNCTION pg_temp.u(tt text, n int) RETURNS uuid
LANGUAGE sql IMMUTABLE AS $f$
  SELECT ('b4' || lpad(tt, 6, '0') || '-0000-4000-8000-' || lpad(n::text, 12, '0'))::uuid
$f$;

DO $$
DECLARE
  sch   uuid := '11111111-1111-1111-1111-111111111111';
  ay    uuid := '12121212-1212-1212-1212-121212121212';
  budi  uuid := '44444444-4444-4444-4444-444444444444';
  wali  uuid := '33333333-3333-3333-3333-333333333333';
  adm   uuid := '22222222-2222-2222-2222-222222222222';
  kep   uuid := '77777777-7777-7777-7777-777777777777';

  -- skema
  s1 uuid := pg_temp.u('01',1); s2 uuid := pg_temp.u('01',2); s3 uuid := pg_temp.u('01',3);
  s4 uuid := pg_temp.u('01',4); s5 uuid := pg_temp.u('01',5);
  -- tagihan
  b1 uuid := pg_temp.u('03',1); b2 uuid := pg_temp.u('03',2); b3 uuid := pg_temp.u('03',3);
  b4 uuid := pg_temp.u('03',4); b5 uuid := pg_temp.u('03',5); b6 uuid := pg_temp.u('03',6);
  b7 uuid := pg_temp.u('03',7); b8 uuid := pg_temp.u('03',8); b9 uuid := pg_temp.u('03',9);
  b10 uuid := pg_temp.u('03',10);
  -- pembayaran (bill_payments)
  p_cp1 uuid := pg_temp.u('06',1); p_tx1 uuid := pg_temp.u('06',2); p_up1 uuid := pg_temp.u('06',3);
  p_tx2 uuid := pg_temp.u('06',4); p_up2 uuid := pg_temp.u('06',5); p_up3 uuid := pg_temp.u('06',6);
  p_dep uuid := pg_temp.u('06',7); p_cp2 uuid := pg_temp.u('06',8); p_cp3 uuid := pg_temp.u('06',9);
  -- kasir
  cp1 uuid := pg_temp.u('09',1); cp2 uuid := pg_temp.u('09',2); cp3 uuid := pg_temp.u('09',3);
  -- deposit movements
  d1 uuid := pg_temp.u('0a',1); d2 uuid := pg_temp.u('0a',2); d3 uuid := pg_temp.u('0a',3); d4 uuid := pg_temp.u('0a',4);
  -- refund
  r1 uuid := pg_temp.u('0b',1); r2 uuid := pg_temp.u('0b',2); r3 uuid := pg_temp.u('0b',3);
  r4 uuid := pg_temp.u('0b',4); r5 uuid := pg_temp.u('0b',5);
  -- penyesuaian
  a1 uuid := pg_temp.u('0d',1); a2 uuid := pg_temp.u('0d',2);
  -- diskon
  dc1 uuid := pg_temp.u('10',1); dc2 uuid := pg_temp.u('10',2); dc3 uuid := pg_temp.u('10',3);
  dc4 uuid := pg_temp.u('10',4); dc5 uuid := pg_temp.u('10',5);
  -- gateway
  m_bca uuid; m_mdr uuid; m_bri uuid; m_qris uuid; m_ovo uuid; m_spay uuid;
  tx1 uuid := pg_temp.u('15',1); tx2 uuid := pg_temp.u('15',2); tx3 uuid := pg_temp.u('15',3);
  tx4 uuid := pg_temp.u('15',4); tx5 uuid := pg_temp.u('15',5); tx6 uuid := pg_temp.u('15',6);
  sb1 uuid := pg_temp.u('16',1); sb2 uuid := pg_temp.u('16',2);
  ex1 uuid := pg_temp.u('19',1); ex2 uuid := pg_temp.u('19',2); ex3 uuid := pg_temp.u('19',3);
  bl  uuid[] := ARRAY[pg_temp.u('18',1),pg_temp.u('18',2),pg_temp.u('18',3),pg_temp.u('18',4),pg_temp.u('18',5),pg_temp.u('18',6)];
  bank_import uuid := pg_temp.u('2e',1);
  pi1 uuid := pg_temp.u('1d',1); pi2 uuid := pg_temp.u('1d',2);
  -- autodebit
  md1 uuid := pg_temp.u('1e',1); md2 uuid := pg_temp.u('1e',2); md3 uuid := pg_temp.u('1e',3);
  -- dompet
  wa uuid := pg_temp.u('20',1); ws uuid := pg_temp.u('20',2);
  tp1 uuid := pg_temp.u('22',1); tp2 uuid := pg_temp.u('22',2); tp3 uuid := pg_temp.u('22',3);
  tp4 uuid := pg_temp.u('22',4); tp5 uuid := pg_temp.u('22',5);
  sr1 uuid := pg_temp.u('23',1); sr2 uuid := pg_temp.u('23',2); sr3 uuid := pg_temp.u('23',3);
  sr4 uuid := pg_temp.u('23',4); sr5 uuid := pg_temp.u('23',5);
  -- kantin
  -- t1/t2 memakai ID yang sama dengan seed ops (pin tenant di peta kampus 3D)
  t1 uuid := '60920000-0000-4000-8000-000000000001'; t2 uuid := '60920000-0000-4000-8000-000000000002'; t3 uuid := pg_temp.u('27',3);
  cs1 uuid := pg_temp.u('28',1); cs2 uuid := pg_temp.u('28',2); cs3 uuid := pg_temp.u('28',3); cs4 uuid := pg_temp.u('28',4);
  st1 uuid := pg_temp.u('2c',1); st2 uuid := pg_temp.u('2c',2); st3 uuid := pg_temp.u('2c',3);
  st4 uuid := pg_temp.u('2c',4); st5 uuid := pg_temp.u('2c',5);

  v_mine boolean;
  v_free boolean;
  v_canteen boolean;
  v_snap jsonb;
  r record;
  v_seq int;
  v_old record;
  v_imported int := 0;
  v_total bigint := 0;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM billing_schemes WHERE school_id = sch) AND
     NOT EXISTS (SELECT 1 FROM student_bills WHERE school_id = sch) THEN
    RAISE NOTICE '40_billing_features: data sekolah demo belum ada (jalankan 03_billing.sql dulu) - tetap lanjut';
  END IF;

  -- =========================================================================
  -- 1. Setelan per sekolah (PK school_id): mapping COA, jurnal, gateway, dompet
  -- =========================================================================
  INSERT INTO billing_journal_settings (school_id, unit_div_code, receivable_acc, cash_acc, bank_acc,
         discount_acc, scholarship_acc, refund_acc, deposit_acc, updated_by, updated_at)
  VALUES (sch, 'SMP', '1201', '1101', '1102', '6402', '6401', '2205', '2203', adm, '2026-06-20 09:00+07')
  ON CONFLICT DO NOTHING;

  -- Mapping komponen -> akun pendapatan, agar generate tagihan tidak ditolak.
  INSERT INTO fee_component_accounts (school_id, component_code, name, revenue_acc, receivable_acc, updated_by, created_at, updated_at)
  VALUES
    (sch, 'SPP',             'SPP Bulanan',                    '4101', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'KEGIATAN',        'Uang Kegiatan Tahunan',          '4102', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'SERAGAM',         'Seragam Sekolah',                '4103', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'BUKU',            'Buku Paket',                     '4103', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'ROBOTIK',         'Ekstrakurikuler Robotik',        '4104', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'LES',             'Les Persiapan Olimpiade',        '4105', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'ASURANSI',        'Asuransi Kecelakaan Siswa',      '4107', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'library',         'Denda & Layanan Perpustakaan',   '4107', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'transport',       'Layanan Antar-Jemput',           '4106', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'event',           'Kegiatan & Study Tour',          '4102', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (sch, 'extracurricular', 'Ekstrakurikuler (modul)',        '4104', '1201', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07')
  ON CONFLICT DO NOTHING;
  -- Komponen lain yang sudah ada di server ikut dipetakan (SPP* -> 4101, lainnya -> 4107).
  INSERT INTO fee_component_accounts (school_id, component_code, name, revenue_acc, receivable_acc, updated_by)
  SELECT c.school_id, c.code, min(c.name),
         CASE WHEN upper(c.code) LIKE '%SPP%' THEN '4101' ELSE '4107' END, '1201', adm
    FROM billing_scheme_components c
   WHERE c.school_id = sch AND btrim(c.code) <> ''
   GROUP BY c.school_id, c.code
  ON CONFLICT DO NOTHING;

  -- Gateway: setelan default, akun jurnal. Kredensial provider TIDAK diisi
  -- (tetap "Belum dikonfigurasi" sampai env NICEPay dipasang).
  INSERT INTO gateway_settings (school_id) VALUES (sch) ON CONFLICT DO NOTHING;
  INSERT INTO gateway_accounts (school_id, clearing_acc, fee_expense_acc, receivable_acc, deposit_acc)
  VALUES (sch, '1104', '6301', '1201', '2203') ON CONFLICT DO NOTHING;

  INSERT INTO payment_methods (id, school_id, code, kind, provider_code, label, enabled, fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort, created_at, updated_at)
  VALUES
    (pg_temp.u('13',1), sch, 'va_bca',     'va',      'CENA', 'Virtual Account BCA',     true,  'flat',    4000, 0,   'parent', 1440, 1, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (pg_temp.u('13',2), sch, 'va_mandiri', 'va',      'BMRI', 'Virtual Account Mandiri', true,  'flat',    4000, 0,   'parent', 1440, 2, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (pg_temp.u('13',3), sch, 'va_bri',     'va',      'BRIN', 'Virtual Account BRI',     false, 'flat',    4000, 0,   'parent', 1440, 3, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (pg_temp.u('13',4), sch, 'qris',       'qris',    'QRIS', 'QRIS (semua e-wallet)',   true,  'percent', 0,    70,  'school', 30,   4, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (pg_temp.u('13',5), sch, 'ovo',        'ewallet', 'OVOE', 'OVO',                     true,  'percent', 0,    150, 'parent', 60,   5, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
    (pg_temp.u('13',6), sch, 'shopeepay',  'ewallet', 'ESHP', 'ShopeePay',               false, 'percent', 0,    150, 'parent', 60,   6, '2026-06-20 09:00+07', '2026-06-20 09:00+07')
  ON CONFLICT DO NOTHING;
  SELECT id INTO m_bca  FROM payment_methods WHERE school_id = sch AND code = 'va_bca';
  SELECT id INTO m_mdr  FROM payment_methods WHERE school_id = sch AND code = 'va_mandiri';
  SELECT id INTO m_bri  FROM payment_methods WHERE school_id = sch AND code = 'va_bri';
  SELECT id INTO m_qris FROM payment_methods WHERE school_id = sch AND code = 'qris';
  SELECT id INTO m_ovo  FROM payment_methods WHERE school_id = sch AND code = 'ovo';
  SELECT id INTO m_spay FROM payment_methods WHERE school_id = sch AND code = 'shopeepay';
  INSERT INTO payment_method_provider_codes (method_id, provider_code, provider_method_code)
  SELECT m.id, 'nicepay', m.provider_code FROM payment_methods m
   WHERE m.id IN (m_bca, m_mdr, m_bri, m_qris, m_ovo, m_spay)
     AND EXISTS (SELECT 1 FROM payment_providers WHERE code = 'nicepay')
  ON CONFLICT DO NOTHING;
  IF EXISTS (SELECT 1 FROM payment_providers WHERE code = 'nicepay') THEN
    INSERT INTO payment_routing_rules (id, school_id, method_kind, method_code, primary_provider, fallback_provider, failover_enabled, active, created_by, created_at, updated_at)
    VALUES
      (pg_temp.u('14',1), sch, 'va',      NULL, 'nicepay', NULL, false, true, adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
      (pg_temp.u('14',2), sch, 'qris',    NULL, 'nicepay', NULL, false, true, adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07'),
      (pg_temp.u('14',3), sch, 'ewallet', NULL, 'nicepay', NULL, false, true, adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07')
    ON CONFLICT DO NOTHING;
  END IF;

  INSERT INTO billing_reminder_settings (school_id, enabled, days_before, on_due, days_after, send_hour, channels, updated_by, created_at, updated_at)
  VALUES (sch, true, '{3}', true, '{3,7}', 8, '{in_app}', adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO autodebit_settings (school_id, max_retries, retry_interval_days, run_hour, updated_by, created_at, updated_at)
  VALUES (sch, 2, 3, 6, adm, '2026-06-20 09:00+07', '2026-06-20 09:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO wallet_settings (school_id, max_allowance_balance_idr, max_savings_balance_idr, pos_offline_max_minutes, updated_at)
  VALUES (sch, 1000000, 10000000, 30, '2026-06-20 09:00+07') ON CONFLICT DO NOTHING;
  INSERT INTO wallet_accounts (school_id, allowance_liability_acc, savings_liability_acc, cash_acc, tenant_payable_acc,
         canteen_revenue_acc, rent_revenue_acc, tenant_receivable_acc, updated_at)
  VALUES (sch, '2201', '2202', '1101', '2204', '4108', '4109', '1202', '2026-06-20 09:00+07') ON CONFLICT DO NOTHING;

  -- =========================================================================
  -- 2. Skema tagihan (aktif, draft, arsip) + komponen
  -- =========================================================================
  INSERT INTO billing_schemes (id, school_id, academic_year_id, grade_level, name, status, installment, due_day, notes,
         effective_from, effective_to, created_at, updated_at)
  VALUES
    (s1, sch, ay, '7', 'Uang Kegiatan Tahunan Kelas 7 2026/2027', 'active', true, 15,
     'Kegiatan tahunan: MPLS, peringatan hari besar, pentas seni. Dapat dicicil 3x per 3 bulan.',
     '2026-07-01', '2027-06-30', '2026-06-25 10:00+07', '2026-06-25 10:00+07'),
    (s2, sch, ay, '7', 'Seragam & Buku Paket Kelas 7 2026/2027', 'active', false, 10,
     'Ditagihkan sekali untuk siswa baru kelas 7.',
     '2026-07-01', '2027-06-30', '2026-06-25 10:05+07', '2026-06-28 14:00+07'),
    (s3, sch, ay, '7', 'Ekstrakurikuler Robotik 2026/2027', 'active', true, 10,
     'Ekskul pilihan; ditagihkan bulanan Agustus-Mei bagi peserta terdaftar.',
     '2026-08-01', '2027-05-31', '2026-07-20 10:00+07', '2026-07-20 10:00+07'),
    (s4, sch, ay, '7', 'SPP Kelas 7 Jalur Beasiswa 2026/2027 (rancangan)', 'draft', false, 10,
     'Rancangan tarif untuk penerima beasiswa yayasan, menunggu keputusan rapat yayasan.',
     '2027-01-01', '2027-06-30', '2026-10-05 13:00+07', '2026-10-05 13:00+07'),
    (s5, sch, ay, '7', 'Les Persiapan OSN Semester Ganjil 2026', 'archived', false, 10,
     'Program selesai; diarsipkan setelah seleksi OSN tingkat kota.',
     '2026-07-01', '2026-09-30', '2026-06-26 10:00+07', '2026-10-01 08:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO billing_scheme_components (id, school_id, scheme_id, code, name, kind, amount_idr, sort_order, is_free,
         installment_terms, installment_interval_months, created_at)
  VALUES
    (pg_temp.u('02',1), sch, s1, 'KEGIATAN', 'Uang Kegiatan Tahunan',      'annual',   1500000, 1, false, 3, 3, '2026-06-25 10:00+07'),
    (pg_temp.u('02',2), sch, s2, 'SERAGAM',  'Seragam Lengkap (4 stel)',   'one_time',  650000, 1, false, 1, 1, '2026-06-25 10:05+07'),
    (pg_temp.u('02',3), sch, s2, 'BUKU',     'Buku Paket Semester 1 & 2',  'one_time',  900000, 2, false, 1, 1, '2026-06-25 10:05+07'),
    (pg_temp.u('02',4), sch, s3, 'ROBOTIK',  'Ekstrakurikuler Robotik',    'monthly',   200000, 1, false, 1, 1, '2026-07-20 10:00+07'),
    (pg_temp.u('02',5), sch, s4, 'SPP',      'SPP Bulanan (beasiswa)',     'monthly',   500000, 1, false, 1, 1, '2026-10-05 13:00+07'),
    (pg_temp.u('02',6), sch, s4, 'ASURANSI', 'Asuransi Kecelakaan Siswa',  'annual',         0, 2, true,  1, 1, '2026-10-05 13:00+07'),
    (pg_temp.u('02',7), sch, s5, 'LES',      'Les Persiapan OSN',          'monthly',   300000, 1, false, 1, 1, '2026-06-26 10:00+07')
  ON CONFLICT DO NOTHING;

  -- Perubahan skema (maker-checker): menunggu, diterapkan, ditolak.
  INSERT INTO billing_scheme_changes (id, school_id, scheme_id, status, payload, before_data, approval_id, requested_by, requested_role,
         decided_by, decision_note, idempotency_key, created_at, decided_at)
  VALUES
    (pg_temp.u('0f',1), sch, s3, 'pending_approval',
     jsonb_build_object('academic_year_id', ay, 'grade_level', '7', 'name', 'Ekstrakurikuler Robotik 2026/2027', 'installment', true,
       'due_day', 10, 'status', 'active', 'effective_from', '2026-08-01', 'effective_to', '2027-05-31',
       'components', jsonb_build_array(jsonb_build_object('code','ROBOTIK','name','Ekstrakurikuler Robotik','kind','monthly','amount_idr',225000,
         'is_free',false,'installment_terms',1,'installment_interval_months',1))),
     jsonb_build_object('academic_year_id', ay, 'grade_level', '7', 'name', 'Ekstrakurikuler Robotik 2026/2027', 'installment', true,
       'due_day', 10, 'status', 'active', 'effective_from', '2026-08-01', 'effective_to', '2027-05-31',
       'components', jsonb_build_array(jsonb_build_object('code','ROBOTIK','name','Ekstrakurikuler Robotik','kind','monthly','amount_idr',200000,
         'is_free',false,'installment_terms',1,'installment_interval_months',1))),
     NULL, adm, 'admin', NULL, NULL, 'seed40-scheme-change-1', '2026-10-09 10:00+07', NULL),
    (pg_temp.u('0f',2), sch, s2, 'applied',
     jsonb_build_object('academic_year_id', ay, 'grade_level', '7', 'name', 'Seragam & Buku Paket Kelas 7 2026/2027', 'installment', false,
       'due_day', 10, 'status', 'active', 'effective_from', '2026-07-01', 'effective_to', '2027-06-30',
       'components', jsonb_build_array(
         jsonb_build_object('code','SERAGAM','name','Seragam Lengkap (4 stel)','kind','one_time','amount_idr',650000,'is_free',false,'installment_terms',1,'installment_interval_months',1),
         jsonb_build_object('code','BUKU','name','Buku Paket Semester 1 & 2','kind','one_time','amount_idr',900000,'is_free',false,'installment_terms',1,'installment_interval_months',1))),
     jsonb_build_object('academic_year_id', ay, 'grade_level', '7', 'name', 'Seragam & Buku Paket Kelas 7 2026/2027', 'installment', false,
       'due_day', 10, 'status', 'active', 'effective_from', '2026-07-01', 'effective_to', '2027-06-30',
       'components', jsonb_build_array(
         jsonb_build_object('code','SERAGAM','name','Seragam Lengkap (4 stel)','kind','one_time','amount_idr',650000,'is_free',false,'installment_terms',1,'installment_interval_months',1),
         jsonb_build_object('code','BUKU','name','Buku Paket Semester 1 & 2','kind','one_time','amount_idr',850000,'is_free',false,'installment_terms',1,'installment_interval_months',1))),
     NULL, adm, 'admin', kep::text, 'Disetujui: harga penerbit naik untuk edisi kurikulum terbaru.', 'seed40-scheme-change-2',
     '2026-06-27 09:00+07', '2026-06-28 14:00+07'),
    (pg_temp.u('0f',3), sch, s1, 'rejected',
     jsonb_build_object('academic_year_id', ay, 'grade_level', '7', 'name', 'Uang Kegiatan Tahunan Kelas 7 2026/2027', 'installment', true,
       'due_day', 15, 'status', 'active', 'effective_from', '2026-07-01', 'effective_to', '2027-06-30',
       'components', jsonb_build_array(jsonb_build_object('code','KEGIATAN','name','Uang Kegiatan Tahunan','kind','annual','amount_idr',1750000,
         'is_free',false,'installment_terms',3,'installment_interval_months',3))),
     jsonb_build_object('academic_year_id', ay, 'grade_level', '7', 'name', 'Uang Kegiatan Tahunan Kelas 7 2026/2027', 'installment', true,
       'due_day', 15, 'status', 'active', 'effective_from', '2026-07-01', 'effective_to', '2027-06-30',
       'components', jsonb_build_array(jsonb_build_object('code','KEGIATAN','name','Uang Kegiatan Tahunan','kind','annual','amount_idr',1500000,
         'is_free',false,'installment_terms',3,'installment_interval_months',3))),
     NULL, adm, 'admin', kep::text, 'Ditolak: kenaikan di tengah tahun ajaran belum disosialisasikan ke wali murid.', 'seed40-scheme-change-3',
     '2026-09-01 09:00+07', '2026-09-03 11:00+07')
  ON CONFLICT DO NOTHING;

  -- =========================================================================
  -- 3. Diskon / promo. HANYA D1 yang aktif (khusus skema Robotik seed).
  -- =========================================================================
  INSERT INTO bill_discounts (id, school_id, name, kind, value, cap_idr, priority, scope, scheme_id, grade_level, student_user_id,
         scholarship, active, status, valid_from, valid_to, reason, approval_id, requested_by, decided_by, decision_note,
         idempotency_key, created_at, updated_at)
  VALUES
    (dc1, sch, 'Potongan Ekskul Robotik Semester Ganjil', 'nominal', 50000, NULL, 10, 'scheme', s3, NULL, NULL,
     false, true, 'active', '2026-08-01', '2026-12-31', 'Subsidi yayasan untuk peserta ekskul robotik semester ganjil',
     NULL, adm, kep::text, 'Disetujui kepala sekolah', 'seed40-discount-1', '2026-07-25 10:00+07', '2026-07-27 09:00+07'),
    (dc2, sch, 'Beasiswa Prestasi OSN Matematika 50% - Budi Santoso', 'percent', 5000, NULL, 20, 'student', NULL, NULL, budi,
     true, false, 'pending_approval', '2026-11-01', '2027-06-30', 'Juara 1 OSN Matematika tingkat kota 2026',
     NULL, adm, NULL, NULL, 'seed40-discount-2', '2026-10-12 10:00+07', '2026-10-12 10:00+07'),
    (dc3, sch, 'Diskon Saudara Kandung Kelas 7', 'percent', 1000, 100000, 30, 'grade', NULL, '7', NULL,
     false, false, 'rejected', '2026-07-01', '2027-06-30', 'Usulan diskon 10% untuk siswa yang memiliki saudara kandung di sekolah',
     NULL, adm, kep::text, 'Ditolak: kebijakan saudara kandung diatur di tingkat yayasan', 'seed40-discount-3', '2026-07-10 10:00+07', '2026-07-12 09:00+07'),
    (dc4, sch, 'Promo Daftar Ulang 2025', 'nominal', 100000, NULL, 40, 'school', NULL, NULL, NULL,
     false, false, 'inactive', '2025-06-01', '2025-07-31', 'Promo daftar ulang tahun ajaran 2025/2026 (sudah berakhir)',
     NULL, adm, kep::text, 'Dinonaktifkan setelah masa promo berakhir', 'seed40-discount-4', '2025-05-20 10:00+07', '2025-08-01 08:00+07'),
    (dc5, sch, 'Beasiswa Yayasan Uang Kegiatan', 'nominal', 250000, NULL, 50, 'scheme', s1, NULL, NULL,
     true, false, 'pending_approval', '2027-01-01', '2027-06-30', 'Bantuan yayasan untuk keluarga terdampak PHK',
     NULL, adm, NULL, NULL, 'seed40-discount-5', '2026-10-13 15:00+07', '2026-10-13 15:00+07')
  ON CONFLICT DO NOTHING;

  -- =========================================================================
  -- 4. Alur uang Budi: hanya bila baris milik seed ini, atau belum ada data
  --    deposit/dompet Budi sama sekali (menghindari saldo yang tidak konsisten).
  -- =========================================================================
  v_mine := EXISTS (SELECT 1 FROM student_bills WHERE id = b1);
  v_free := NOT EXISTS (SELECT 1 FROM student_deposits WHERE school_id = sch AND student_user_id = budi)
        AND NOT EXISTS (SELECT 1 FROM deposit_movements WHERE school_id = sch AND student_user_id = budi)
        AND NOT EXISTS (SELECT 1 FROM wallets WHERE school_id = sch AND student_user_id = budi)
        AND NOT EXISTS (SELECT 1 FROM student_bills WHERE school_id = sch AND source_ref IN
              ('LIB-LOAN-2026-0912','TRN-2026-09-BUDI','EVT-STUDYTOUR-BDG-2026','LIB-LOAN-2026-1003','TRN-2026-10-BUDI'));
  v_canteen := EXISTS (SELECT 1 FROM canteen_tenants WHERE id = t1)
            OR NOT EXISTS (SELECT 1 FROM canteen_tenants WHERE school_id = sch AND code IN ('KNT-01','KNT-02','KNT-03'));
  IF NOT v_canteen THEN
    RAISE NOTICE '40_billing_features: kode tenant KNT-01..03 sudah dipakai data lain; tagihan, dompet & kantin dilewati';
  ELSIF NOT v_mine AND NOT v_free THEN
    RAISE NOTICE '40_billing_features: Budi sudah punya deposit/dompet/tagihan ad-hoc di luar seed ini; bagian tagihan, pembayaran, dompet dilewati';
  ELSE

  -- ---- 4a. Tagihan -------------------------------------------------------
  v_snap := jsonb_build_array(jsonb_build_object('id', dc1::text, 'source', 'bill_discount',
              'label', 'Potongan Ekskul Robotik Semester Ganjil', 'priority', 10,
              'base_before', 200000, 'requested', 50000, 'applied', 50000, 'base_after', 150000));

  INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, source_module, source_ref, period_year, period_month,
         base_idr, discount_idr, net_idr, status, due_at, discount_snapshot, void_reason, created_at, updated_at)
  VALUES
    (b1,  sch, budi, s1, NULL, NULL, 2026, 7, 1500000, 0, 1500000, 'partial', '2026-10-15 00:00+00', '[]', NULL, '2026-07-01 07:00+07', '2026-07-20 10:30+07'),
    (b2,  sch, budi, s2, NULL, NULL, 2026, 7, 1550000, 0, 1550000, 'paid',    '2026-07-10 00:00+00', '[]', NULL, '2026-07-01 07:00+07', '2026-07-08 10:15+07'),
    (b3,  sch, budi, s3, NULL, NULL, 2026, 8,  200000, 50000, 150000, 'paid',  '2026-08-10 00:00+00', v_snap, NULL, '2026-08-01 07:00+07', '2026-08-05 14:00+07'),
    (b4,  sch, budi, s3, NULL, NULL, 2026, 9,  200000, 50000, 150000, 'paid',  '2026-09-10 00:00+00', v_snap, NULL, '2026-09-01 07:00+07', '2026-09-07 19:20+07'),
    (b5,  sch, budi, s3, NULL, NULL, 2026, 10, 200000, 50000, 150000, 'unpaid','2026-10-10 00:00+00', v_snap, NULL, '2026-10-01 07:00+07', '2026-10-01 07:00+07'),
    (b6,  sch, budi, NULL, 'library',   'LIB-LOAN-2026-0912',     2026, 9,    15000, 0,   15000, 'paid', '2026-09-19 00:00+00', '[]', NULL, '2026-09-12 13:00+07', '2026-09-12 13:00+07'),
    (b7,  sch, budi, NULL, 'transport', 'TRN-2026-09-BUDI',       2026, 9,   350000, 0,  350000, 'paid', '2026-09-05 00:00+00', '[]', NULL, '2026-08-25 09:00+07', '2026-09-03 10:00+07'),
    (b8,  sch, budi, NULL, 'event',     'EVT-STUDYTOUR-BDG-2026', 2026, 9,  1250000, 0, 1250000, 'void', '2026-09-30 00:00+00', '[]',
     'Study tour Bandung dibatalkan sekolah karena cuaca ekstrem', '2026-09-15 09:00+07', '2026-10-02 10:00+07'),
    (b9,  sch, budi, NULL, 'library',   'LIB-LOAN-2026-1003',     2026, 10,   10000, 0,   10000, 'unpaid', '2026-10-17 00:00+00', '[]', NULL, '2026-10-03 13:00+07', '2026-10-03 13:00+07'),
    (b10, sch, budi, NULL, 'transport', 'TRN-2026-10-BUDI',       2026, 10,  245000, 0,  245000, 'unpaid', '2026-10-20 00:00+00', '[]', NULL, '2026-09-28 09:00+07', '2026-10-04 11:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO bill_items (id, school_id, bill_id, name, component_code, amount_idr, discount_idr, net_idr, adjustment_id, created_at)
  VALUES
    (pg_temp.u('04',1),  sch, b1, 'Uang Kegiatan Tahunan',     'KEGIATAN', 1500000, 0, 1500000, NULL, '2026-07-01 07:00+07'),
    (pg_temp.u('04',2),  sch, b2, 'Seragam Lengkap (4 stel)',  'SERAGAM',   650000, 0,  650000, NULL, '2026-07-01 07:00+07'),
    (pg_temp.u('04',3),  sch, b2, 'Buku Paket Semester 1 & 2', 'BUKU',      900000, 0,  900000, NULL, '2026-07-01 07:00+07'),
    (pg_temp.u('04',4),  sch, b3, 'Ekstrakurikuler Robotik',   'ROBOTIK',   200000, 0,  200000, NULL, '2026-08-01 07:00+07'),
    (pg_temp.u('04',5),  sch, b4, 'Ekstrakurikuler Robotik',   'ROBOTIK',   200000, 0,  200000, NULL, '2026-09-01 07:00+07'),
    (pg_temp.u('04',6),  sch, b5, 'Ekstrakurikuler Robotik',   'ROBOTIK',   200000, 0,  200000, NULL, '2026-10-01 07:00+07'),
    (pg_temp.u('04',7),  sch, b6, 'Denda keterlambatan buku "Laskar Pelangi" (5 hari)', NULL, 15000, 0, 15000, NULL, '2026-09-12 13:00+07'),
    (pg_temp.u('04',8),  sch, b7, 'Antar-jemput sekolah September 2026 (rute Cibubur)', NULL, 350000, 0, 350000, NULL, '2026-08-25 09:00+07'),
    (pg_temp.u('04',9),  sch, b8, 'Study tour Bandung (Museum Geologi & Saung Angklung Udjo)', NULL, 1250000, 0, 1250000, NULL, '2026-09-15 09:00+07'),
    (pg_temp.u('04',10), sch, b9, 'Denda keterlambatan buku "Bumi" (2 hari)', NULL, 10000, 0, 10000, NULL, '2026-10-03 13:00+07'),
    (pg_temp.u('04',11), sch, b10,'Antar-jemput sekolah Oktober 2026 (rute Cibubur)', NULL, 350000, 0, 350000, NULL, '2026-09-28 09:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO bill_installments (id, school_id, bill_id, seq, due_at, amount_idr, created_at)
  VALUES
    (pg_temp.u('05',1), sch, b1, 1, '2026-07-15 00:00+00', 500000, '2026-07-01 07:00+07'),
    (pg_temp.u('05',2), sch, b1, 2, '2026-10-15 00:00+00', 500000, '2026-07-01 07:00+07'),
    (pg_temp.u('05',3), sch, b1, 3, '2027-01-15 00:00+00', 500000, '2026-07-01 07:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO bill_discount_steps (school_id, bill_id, discount_id, sequence, source, label, priority,
         base_before_idr, requested_idr, applied_idr, base_after_idr, created_at)
  SELECT sch, x.bill, dc1, 1, 'bill_discount', 'Potongan Ekskul Robotik Semester Ganjil', 10, 200000, 50000, 50000, 150000, x.at
    FROM (VALUES (b3, '2026-08-01 07:00+07'::timestamptz), (b4, '2026-09-01 07:00+07'), (b5, '2026-10-01 07:00+07')) x(bill, at)
  ON CONFLICT DO NOTHING;

  -- ---- 4b. Kasir ---------------------------------------------------------
  INSERT INTO cashier_payments (id, school_id, student_user_id, method, amount_idr, deposit_idr, received_by, note, status,
         idempotency_key, created_at, updated_at)
  VALUES
    (cp1, sch, budi, 'cash',      500000,     0, adm, 'Cicilan ke-1 uang kegiatan, dibayar tunai oleh Bapak Santoso', 'recorded', 'seed40-cashier-1', '2026-07-20 10:30+07', '2026-07-20 10:30+07'),
    (cp2, sch, budi, 'transfer',  400000, 50000, adm, 'Transfer antar-jemput September, lebih Rp50.000 masuk deposit', 'recorded', 'seed40-cashier-2', '2026-09-03 10:00+07', '2026-09-03 10:00+07'),
    (cp3, sch, budi, 'transfer', 1250000,     0, adm, 'Pelunasan study tour Bandung', 'recorded', 'seed40-cashier-3', '2026-09-25 11:00+07', '2026-09-25 11:00+07')
  ON CONFLICT DO NOTHING;

  -- ---- 4c. Transaksi payment gateway -------------------------------------
  INSERT INTO gateway_transactions (id, school_id, tx_no, payer_user_id, student_user_id, method_id, bills_amount_idr,
         fee_idr, total_idr, deposit_idr, fee_bearer, status, provider_ref, va_number, qr_content, redirect_url, expires_at, paid_at,
         failure_reason, settlement_batch_id, idempotency_key, created_at, updated_at, provider, purpose, purpose_ref)
  VALUES
    (tx1, sch, 'TX-20260708094012-4821', wali, budi, m_bca, 1550000, 4000, 1554000, 0, 'parent', 'success',
     'IONPAYTEST02202607080940124821', '8848088812345678', NULL, NULL, '2026-07-09 09:40+07', '2026-07-08 10:15+07', NULL, NULL,
     'seed40-gw-1', '2026-07-08 09:40+07', '2026-07-08 10:15+07', 'nicepay', 'bills', NULL),
    (tx2, sch, 'TX-20260907191502-1177', wali, budi, m_qris, 150000, 1050, 150000, 0, 'school', 'success',
     'IONPAYTEST08202609071915021177', NULL, '00020101021226670016ID.CO.QRIS.WWW0118936000000000000001SEED', NULL,
     '2026-09-07 19:45+07', '2026-09-07 19:20+07', NULL, NULL, 'seed40-gw-2', '2026-09-07 19:15+07', '2026-09-07 19:20+07', 'nicepay', 'bills', NULL),
    (tx3, sch, 'TX-20261005080011-3302', wali, budi, m_mdr, 150000, 4000, 154000, 0, 'parent', 'expired',
     'IONPAYTEST02202610050800113302', '8960088812340001', NULL, NULL, '2026-10-06 08:00+07', NULL, NULL, NULL,
     'seed40-gw-3', '2026-10-05 08:00+07', '2026-10-06 08:01+07', 'nicepay', 'bills', NULL),
    (tx4, sch, 'TX-20261009201544-9045', wali, budi, m_ovo, 150000, 2250, 152250, 0, 'parent', 'failed',
     'IONPAYTEST05202610092015449045', NULL, NULL, 'https://ovo.example.invalid/pay/seed', '2026-10-09 21:15+07', NULL,
     'Saldo OVO tidak mencukupi', NULL, 'seed40-gw-4', '2026-10-09 20:15+07', '2026-10-09 20:17+07', 'nicepay', 'bills', NULL),
    (tx5, sch, 'TX-20260915070230-5560', wali, budi, m_bca, 200000, 4000, 204000, 0, 'parent', 'success',
     'IONPAYTEST02202609150702305560', '8848088812345679', NULL, NULL, '2026-09-16 07:02+07', '2026-09-15 07:10+07', NULL, NULL,
     'seed40-gw-5', '2026-09-15 07:02+07', '2026-09-15 07:10+07', 'nicepay', 'wallet_topup', tp2),
    (tx6, sch, 'TX-20261010180405-7712', wali, budi, m_qris, 100000, 700, 100000, 0, 'school', 'expired',
     'IONPAYTEST08202610101804057712', NULL, '00020101021226670016ID.CO.QRIS.WWW0118936000000000000002SEED', NULL,
     '2026-10-10 18:34+07', NULL, NULL, NULL, 'seed40-gw-6', '2026-10-10 18:04+07', '2026-10-10 18:35+07', 'nicepay', 'wallet_topup', tp4)
  ON CONFLICT DO NOTHING;

  INSERT INTO gateway_transaction_bills (transaction_id, bill_id, position, amount_idr)
  VALUES (tx1, b2, 1, 1550000), (tx2, b4, 1, 150000), (tx3, b5, 1, 150000), (tx4, b5, 1, 150000)
  ON CONFLICT DO NOTHING;

  INSERT INTO gateway_notifications (id, school_id, provider_ref, signature_valid, raw_hash, body_masked, processed_at, result, received_at, provider)
  VALUES
    (pg_temp.u('1a',1), sch, 'IONPAYTEST02202607080940124821', true, encode(sha256('seed40-notif-1'::bytea), 'hex'),
     '{"tXid":"IONPAYTEST02202607080940124821","referenceNo":"TX-20260708094012-4821","amt":"1554000","status":"0","payMethod":"02","bankCd":"CENA","vacctNo":"************5678"}',
     '2026-07-08 10:15+07', 'success', '2026-07-08 10:15+07', 'nicepay'),
    (pg_temp.u('1a',2), sch, 'IONPAYTEST08202609071915021177', true, encode(sha256('seed40-notif-2'::bytea), 'hex'),
     '{"tXid":"IONPAYTEST08202609071915021177","referenceNo":"TX-20260907191502-1177","amt":"150000","status":"0","payMethod":"08"}',
     '2026-09-07 19:20+07', 'success', '2026-09-07 19:20+07', 'nicepay'),
    (pg_temp.u('1a',3), NULL, 'IONPAYTEST02202609150702305560', false, encode(sha256('seed40-notif-3'::bytea), 'hex'),
     '{"tXid":"IONPAYTEST02202609150702305560","amt":"999999","status":"0","merchantToken":"***"}',
     '2026-09-15 07:05+07', 'invalid_signature', '2026-09-15 07:05+07', 'nicepay'),
    (pg_temp.u('1a',4), sch, 'IONPAYTEST02202609150702305560', true, encode(sha256('seed40-notif-4'::bytea), 'hex'),
     '{"tXid":"IONPAYTEST02202609150702305560","referenceNo":"TX-20260915070230-5560","amt":"204000","status":"0","payMethod":"02","bankCd":"CENA"}',
     '2026-09-15 07:10+07', 'success', '2026-09-15 07:10+07', 'nicepay')
  ON CONFLICT DO NOTHING;

  -- ---- 4d. Pembayaran tagihan (ledger) -----------------------------------
  INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, transfer_at, bank_name, account_name, reference,
         object_key, file_name, mime, size_bytes, sha256, status, review_note, idempotency_key, uploader_user_id, verified_at,
         created_at, updated_at, source, source_id)
  VALUES
    -- b1 cicilan 1 via kasir tunai
    (p_cp1, sch, b1, 500000, '2026-07-20 10:30+07', 'Kasir', 'Tunai (kasir)', 'KASIR-' || cp1::text, NULL, NULL, NULL, NULL, NULL,
     'verified', NULL, 'cashier-alloc:' || cp1::text || ':' || b1::text, adm, '2026-07-20 10:30+07', '2026-07-20 10:30+07', '2026-07-20 10:30+07', 'cashier', cp1),
    -- b2 lunas via VA BCA
    (p_tx1, sch, b2, 1550000, '2026-07-08 10:15+07', 'NICEPay', 'Payment Gateway', 'TX-20260708094012-4821', 'gateway/' || tx1::text,
     'gateway_receipt', 'application/json', 100, 'seed40-corr-tx1', 'verified', NULL, 'gw-alloc:' || tx1::text || ':' || b2::text, wali,
     '2026-07-08 10:15+07', '2026-07-08 10:15+07', '2026-07-08 10:15+07', 'upload', NULL),
    -- b3 lunas via unggah bukti transfer
    (p_up1, sch, b3, 150000, '2026-08-05 09:12+07', 'BCA', 'Santoso', 'BCA-0805-778812', 'billing/proofs/seed40/bukti-robotik-agustus.jpg',
     'bukti-transfer-robotik-agustus.jpg', 'image/jpeg', 184320, encode(sha256('seed40-up1'::bytea), 'hex'), 'verified',
     'Sesuai mutasi BCA 05/08', 'seed40-upload-1', wali, '2026-08-05 14:00+07', '2026-08-05 09:20+07', '2026-08-05 14:00+07', 'upload', NULL),
    -- b4 lunas via QRIS
    (p_tx2, sch, b4, 150000, '2026-09-07 19:20+07', 'NICEPay', 'Payment Gateway', 'TX-20260907191502-1177', 'gateway/' || tx2::text,
     'gateway_receipt', 'application/json', 100, 'seed40-corr-tx2', 'verified', NULL, 'gw-alloc:' || tx2::text || ':' || b4::text, wali,
     '2026-09-07 19:20+07', '2026-09-07 19:20+07', '2026-09-07 19:20+07', 'upload', NULL),
    -- b5: bukti menunggu verifikasi & bukti ditolak (tidak mengubah status)
    (p_up2, sch, b5, 150000, '2026-10-12 16:40+07', 'BCA', 'Santoso', 'BCA-1012-991204', 'billing/proofs/seed40/bukti-robotik-oktober.jpg',
     'bukti-transfer-robotik-oktober.jpg', 'image/jpeg', 201728, encode(sha256('seed40-up2'::bytea), 'hex'), 'pending',
     NULL, 'seed40-upload-2', wali, NULL, '2026-10-12 16:45+07', '2026-10-12 16:45+07', 'upload', NULL),
    (p_up3, sch, b5, 100000, '2026-10-08 07:30+07', 'BCA', 'Santoso', 'BCA-1008-120044', 'billing/proofs/seed40/bukti-buram.jpg',
     'IMG_20261008_0730.jpg', 'image/jpeg', 96512, encode(sha256('seed40-up3'::bytea), 'hex'), 'rejected',
     'Nominal tidak sesuai tagihan dan foto bukti buram, mohon unggah ulang', 'seed40-upload-3', wali, NULL,
     '2026-10-08 07:35+07', '2026-10-08 13:00+07', 'upload', NULL),
    -- b6 lunas dari deposit
    (p_dep, sch, b6, 15000, '2026-09-12 13:00+07', NULL, NULL, 'DEPOSIT', NULL, NULL, NULL, NULL, NULL, 'verified', NULL,
     'deposit-apply:' || b6::text, adm, '2026-09-12 13:00+07', '2026-09-12 13:00+07', '2026-09-12 13:00+07', 'deposit', d2),
    -- b7 lunas via kasir transfer
    (p_cp2, sch, b7, 350000, '2026-09-03 10:00+07', 'Kasir', 'Transfer (kasir)', 'KASIR-' || cp2::text, NULL, NULL, NULL, NULL, NULL,
     'verified', NULL, 'cashier-alloc:' || cp2::text || ':' || b7::text, adm, '2026-09-03 10:00+07', '2026-09-03 10:00+07', '2026-09-03 10:00+07', 'cashier', cp2),
    -- b8 dibayar lalu dibatalkan (direversal ke deposit)
    (p_cp3, sch, b8, 1250000, '2026-09-25 11:00+07', 'Kasir', 'Transfer (kasir)', 'KASIR-' || cp3::text, NULL, NULL, NULL, NULL, NULL,
     'verified', NULL, 'cashier-alloc:' || cp3::text || ':' || b8::text, adm, '2026-09-25 11:00+07', '2026-09-25 11:00+07', '2026-09-25 11:00+07', 'cashier', cp3)
  ON CONFLICT DO NOTHING;

  INSERT INTO bill_payment_allocations (id, school_id, payment_id, bill_item_id, amount_idr, created_at)
  VALUES
    (pg_temp.u('07',1), sch, p_cp1, NULL,  500000, '2026-07-20 10:30+07'),
    (pg_temp.u('07',2), sch, p_tx1, NULL, 1550000, '2026-07-08 10:15+07'),
    (pg_temp.u('07',3), sch, p_up1, NULL,  150000, '2026-08-05 14:00+07'),
    (pg_temp.u('07',4), sch, p_tx2, NULL,  150000, '2026-09-07 19:20+07'),
    (pg_temp.u('07',5), sch, p_dep, NULL,   15000, '2026-09-12 13:00+07'),
    (pg_temp.u('07',6), sch, p_cp2, NULL,  350000, '2026-09-03 10:00+07'),
    (pg_temp.u('07',7), sch, p_cp3, NULL, 1250000, '2026-09-25 11:00+07')
  ON CONFLICT DO NOTHING;

  -- ---- 4e. Perubahan tagihan: batal (b8), sesuaikan (b10), menunggu (b9), ditolak (b1)
  INSERT INTO bill_adjustments (id, school_id, bill_id, kind, delta_idr, net_before_idr, net_after_idr, deposit_idr, reason,
         source_ref_version, idempotency_key, created_at)
  VALUES
    (a1, sch, b8,  'cancel', -1250000, 1250000, 0,      1250000, 'Study tour Bandung dibatalkan sekolah karena cuaca ekstrem', NULL,
     'bill-change:' || pg_temp.u('0e',1)::text, '2026-10-02 10:00+07'),
    (a2, sch, b10, 'adjust',  -105000,  350000, 245000,       0, 'Rute dipersingkat: Budi hanya ikut antar-jemput pagi mulai Oktober', NULL,
     'bill-change:' || pg_temp.u('0e',2)::text, '2026-10-04 11:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO bill_items (id, school_id, bill_id, name, component_code, amount_idr, discount_idr, net_idr, adjustment_id, created_at)
  VALUES
    (pg_temp.u('04',12), sch, b8,  'Pembatalan: Study tour Bandung dibatalkan sekolah karena cuaca ekstrem', NULL, -1250000, 0, -1250000, a1, '2026-10-02 10:00+07'),
    (pg_temp.u('04',13), sch, b10, 'Penyesuaian: Rute dipersingkat: Budi hanya ikut antar-jemput pagi mulai Oktober', NULL, -105000, 0, -105000, a2, '2026-10-04 11:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO bill_change_requests (id, school_id, bill_id, kind, new_net_idr, reason, status, before_data, approval_id, adjustment_id,
         requested_by, requested_role, decided_by, decision_note, failure_reason, idempotency_key, created_at, decided_at)
  VALUES
    (pg_temp.u('0e',1), sch, b8, 'cancel', NULL, 'Study tour Bandung dibatalkan sekolah karena cuaca ekstrem', 'applied',
     '{"status":"paid","base_idr":1250000,"discount_idr":0,"net_idr":1250000,"paid_idr":1250000,"outstanding_idr":0}', NULL, a1,
     adm, 'admin', kep::text, 'Setuju, dana dikembalikan ke deposit siswa', NULL, 'seed40-bill-change-1', '2026-10-01 09:00+07', '2026-10-02 10:00+07'),
    (pg_temp.u('0e',2), sch, b10, 'adjust', 245000, 'Rute dipersingkat: Budi hanya ikut antar-jemput pagi mulai Oktober', 'applied',
     '{"status":"unpaid","base_idr":350000,"discount_idr":0,"net_idr":350000,"paid_idr":0,"outstanding_idr":350000}', NULL, a2,
     adm, 'admin', kep::text, 'Disetujui sesuai surat permohonan wali', NULL, 'seed40-bill-change-2', '2026-10-03 14:00+07', '2026-10-04 11:00+07'),
    (pg_temp.u('0e',3), sch, b9, 'cancel', NULL, 'Buku sudah dikembalikan tepat waktu, denda tercatat karena salah input tanggal', 'pending_approval',
     '{"status":"unpaid","base_idr":10000,"discount_idr":0,"net_idr":10000,"paid_idr":0,"outstanding_idr":10000}', NULL, NULL,
     adm, 'admin', NULL, NULL, NULL, 'seed40-bill-change-3', '2026-10-13 10:00+07', NULL),
    (pg_temp.u('0e',4), sch, b1, 'cancel', NULL, 'Wali mengajukan pembebasan uang kegiatan', 'rejected',
     '{"status":"partial","base_idr":1500000,"discount_idr":0,"net_idr":1500000,"paid_idr":500000,"outstanding_idr":1000000}', NULL, NULL,
     adm, 'admin', kep::text, 'Ditolak: siswa tetap mengikuti seluruh kegiatan; silakan ajukan beasiswa', NULL, 'seed40-bill-change-4',
     '2026-08-10 09:00+07', '2026-08-12 13:00+07')
  ON CONFLICT DO NOTHING;

  -- ---- 4f. Refund --------------------------------------------------------
  INSERT INTO bill_refunds (id, school_id, student_bill_id, student_user_id, amount_idr, reason, status, requested_by, decided_by,
         decided_at, decision_note, processed_by, processed_at, created_at, updated_at, idempotency_key, wallet_id, approval_id,
         transfer_ref, transfer_date, process_note, deposit_idr)
  VALUES
    (r1, sch, NULL, budi, 1250000, 'Pengembalian dana study tour Bandung yang dibatalkan', 'processed', adm, kep,
     '2026-10-03 09:00+07', 'Disetujui, transfer ke rekening wali', adm, '2026-10-06 10:00+07', '2026-10-02 15:00+07', '2026-10-06 10:00+07',
     'seed40-refund-1', NULL, NULL, 'TRF-BCA-061026-001', '2026-10-06', 'Ditransfer ke rekening BCA a.n. Santoso', 1250000),
    (r2, sch, b3, budi, 150000, 'Budi tidak mengikuti ekskul robotik selama Agustus karena sakit', 'requested', wali, NULL,
     NULL, NULL, NULL, NULL, '2026-10-11 19:00+07', '2026-10-11 19:00+07', 'seed40-refund-2', NULL, NULL, NULL, NULL, NULL, 0),
    (r3, sch, b2, budi, 650000, 'Seragam tidak jadi diambil karena ukuran tidak tersedia', 'rejected', wali, kep,
     '2026-07-22 10:00+07', 'Ditolak: seragam sudah diserahkan dan ditandatangani wali pada 15/07', NULL, NULL,
     '2026-07-18 08:00+07', '2026-07-22 10:00+07', 'seed40-refund-3', NULL, NULL, NULL, NULL, NULL, 0),
    (r4, sch, NULL, budi, 185000, 'Pengembalian sisa deposit dan ekskul September', 'rejected', adm, kep,
     '2026-10-09 09:00+07', 'Ditolak: ekskul September sudah berjalan penuh', NULL, NULL,
     '2026-10-08 10:00+07', '2026-10-09 09:00+07', 'seed40-refund-4', NULL, NULL, NULL, NULL, NULL, 35000),
    (r5, sch, b7, budi, 100000, 'Antar-jemput tidak berjalan 6 hari karena perbaikan bus', 'approved', adm, kep,
     '2026-10-14 11:00+07', 'Disetujui, proses transfer minggu ini', NULL, NULL,
     '2026-10-10 09:00+07', '2026-10-14 11:00+07', 'seed40-refund-5', NULL, NULL, NULL, NULL, NULL, 0)
  ON CONFLICT DO NOTHING;
  INSERT INTO bill_refund_parts (id, school_id, refund_id, student_bill_id, amount_idr)
  VALUES (pg_temp.u('0c',1), sch, r4, b4, 150000) ON CONFLICT DO NOTHING;

  INSERT INTO bill_payment_reversals (id, school_id, payment_id, amount_idr, reason, actor_id, idempotency_key, created_at, refund_id)
  VALUES (pg_temp.u('08',1), sch, p_cp3, 1250000,
          'Kelebihan bayar ke deposit: Study tour Bandung dibatalkan sekolah karena cuaca ekstrem', budi,
          'adhoc-adjust:' || a1::text || ':' || p_cp3::text, '2026-10-02 10:00+07', NULL)
  ON CONFLICT DO NOTHING;

  -- ---- 4g. Deposit (saldo akhir 35.000 = jumlah mutasi) -------------------
  INSERT INTO deposit_movements (id, school_id, student_user_id, amount_idr, balance_after_idr, reason, ref, created_at)
  VALUES
    (d1, sch, budi,    50000,   50000, 'Kelebihan pembayaran kasir',             'cashier:' || cp2::text,          '2026-09-03 10:00+07'),
    (d2, sch, budi,   -15000,   35000, 'Pemakaian deposit untuk tagihan baru',   'bill:' || b6::text,              '2026-09-12 13:00+07'),
    (d3, sch, budi,  1250000, 1285000, 'Kelebihan bayar tagihan ad-hoc',         'bill_adjustment:' || a1::text,   '2026-10-02 10:00+07'),
    (d4, sch, budi, -1250000,   35000, 'Refund deposit: Pengembalian dana study tour Bandung yang dibatalkan', 'refund:' || r1::text, '2026-10-06 10:00+07')
  ON CONFLICT DO NOTHING;
  INSERT INTO student_deposits (school_id, student_user_id, balance_idr, updated_at)
  VALUES (sch, budi, 35000, '2026-10-06 10:00+07') ON CONFLICT DO NOTHING;

  -- ---- 4h. Pengingat & tindak lanjut -------------------------------------
  INSERT INTO bill_reminder_log (id, school_id, student_user_id, bill_id, guardian_user_id, kind, offset_days, outstanding_idr, status,
         attempts, error, sent_by, idempotency_key, created_at, updated_at)
  VALUES
    (pg_temp.u('11',1), sch, budi, b5, wali, 'before_due', 3, 150000, 'sent', 1, NULL, NULL,
     'auto:' || b5::text || ':before_due:3:20261010:' || wali::text, '2026-10-07 08:00+07', '2026-10-07 08:00+07'),
    (pg_temp.u('11',2), sch, budi, b5, wali, 'on_due', 0, 150000, 'sent', 1, NULL, NULL,
     'auto:' || b5::text || ':on_due:0:20261010:' || wali::text, '2026-10-10 08:00+07', '2026-10-10 08:00+07'),
    (pg_temp.u('11',3), sch, budi, b5, wali, 'overdue', 3, 150000, 'failed', 3, 'notification service: timeout setelah 10 detik', NULL,
     'auto:' || b5::text || ':overdue:3:20261010:' || wali::text, '2026-10-13 08:00+07', '2026-10-13 10:00+07'),
    (pg_temp.u('11',4), sch, budi, b1, wali, 'manual', 0, 1000000, 'sent', 1, NULL, adm,
     'seed40-reminder-manual-1', '2026-10-12 09:30+07', '2026-10-12 09:30+07'),
    (pg_temp.u('11',5), sch, budi, b9, wali, 'before_due', 3, 10000, 'skipped', 0, 'Tagihan sedang dalam pengajuan pembatalan', NULL,
     'auto:' || b9::text || ':before_due:3:20261017:' || wali::text, '2026-10-14 08:00+07', '2026-10-14 08:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO bill_followups (id, school_id, student_user_id, bill_id, channel, outcome, promise_date, promise_amount_idr, next_action_on, created_by, created_at)
  VALUES
    (pg_temp.u('12',1), sch, budi, b5, 'phone', 'Bapak Santoso berjanji melunasi ekskul Oktober setelah gajian', '2026-10-25', 150000, '2026-10-26', adm, '2026-10-13 10:15+07'),
    (pg_temp.u('12',2), sch, budi, b1, 'whatsapp', 'Pengingat cicilan ke-2 uang kegiatan terkirim, wali mengonfirmasi akan bayar di kasir', '2026-10-15', 500000, '2026-10-16', adm, '2026-10-12 09:35+07'),
    (pg_temp.u('12',3), sch, budi, b10, 'meeting', 'Pertemuan dengan wali membahas penyesuaian biaya antar-jemput Oktober', NULL, NULL, '2026-10-20', adm, '2026-10-03 13:00+07')
  ON CONFLICT DO NOTHING;

  -- ---- 4i. Koreksi pembayaran --------------------------------------------
  INSERT INTO payment_corrections (id, school_id, source, source_id, kind, target_student_user_id, reason, status, approval_id,
         requested_by, decided_by, decision_comment, decided_at, created_at, updated_at)
  VALUES
    (pg_temp.u('1c',1), sch, 'cashier', cp1, 'cancel', NULL, 'Kasir salah input nominal cicilan', 'rejected', NULL,
     adm, kep::text, 'Ditolak: nominal sesuai kuitansi bertanda tangan wali', '2026-07-22 09:00+07', '2026-07-21 08:00+07', '2026-07-22 09:00+07'),
    (pg_temp.u('1c',2), sch, 'gateway', tx2, 'cancel', NULL, 'Wali melaporkan pembayaran QRIS terdebit dua kali', 'pending', NULL,
     adm, NULL, NULL, NULL, '2026-10-14 09:00+07', '2026-10-14 09:00+07')
  ON CONFLICT DO NOTHING;

  -- ---- 4j. Settlement & rekonsiliasi -------------------------------------
  INSERT INTO settlement_batches (id, school_id, provider, channel, settled_on, gross_idr, fee_idr, net_idr, bank_account, status,
         source_file_key, created_at, updated_at)
  VALUES
    (sb1, sch, 'nicepay', 'va',   '2026-07-09', 1554000, 4000, 1550000, 'BCA 123-456-7890', 'matched',  NULL, '2026-07-09 08:00+07', '2026-07-09 08:05+07'),
    (sb2, sch, 'nicepay', 'qris', '2026-09-08',  225000, 1575, 223425, 'BCA 123-456-7890', 'mismatch', NULL, '2026-09-08 08:00+07', '2026-09-08 08:05+07')
  ON CONFLICT DO NOTHING;
  INSERT INTO settlement_items (id, school_id, batch_id, provider_ref, amount_idr, fee_idr, matched_transaction_id, created_at)
  VALUES
    (pg_temp.u('17',1), sch, sb1, 'IONPAYTEST02202607080940124821', 1554000, 4000, tx1,  '2026-07-09 08:00+07'),
    (pg_temp.u('17',2), sch, sb2, 'IONPAYTEST08202609071915021177',  150000, 1050, tx2,  '2026-09-08 08:00+07'),
    (pg_temp.u('17',3), sch, sb2, 'IONPAYTEST08202609070000009999',   75000,  525, NULL, '2026-09-08 08:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO reconciliation_exceptions (id, school_id, kind, ref, detail, status, resolution_note, adjustment_lines,
         adjustment_approval_id, resolved_by, resolved_at, created_at, updated_at, provider, requested_by, requested_at, decision_note)
  VALUES
    (ex1, sch, 'missing_in_settlement', 'TX-20260915070230-5560',
     '{"provider_ref":"IONPAYTEST02202609150702305560","amount_idr":204000}', 'open', NULL, NULL, NULL, NULL, NULL,
     '2026-09-16 06:00+07', '2026-09-16 06:00+07', 'nicepay', NULL, NULL, NULL),
    (ex2, sch, 'missing_in_sekoci', 'IONPAYTEST08202609070000009999',
     '{"amount_idr":75000}', 'pending_approval', 'Dana dari transaksi QRIS kantin yang salah merchant; dicatat sebagai pendapatan lain-lain',
     '[{"acc_code":"1104","description":"Kliring payment gateway","debit_idr":74475,"credit_idr":0},{"acc_code":"6301","description":"Biaya payment gateway","debit_idr":525,"credit_idr":0},{"acc_code":"4107","description":"Pendapatan lain-lain","debit_idr":0,"credit_idr":75000}]',
     NULL, NULL, NULL, '2026-09-08 06:00+07', '2026-10-01 10:00+07', 'nicepay', adm::text, '2026-10-01 10:00+07', NULL),
    (ex3, sch, 'bank_unmatched', bl[6]::text,
     '{"account":"BCA 123-456-7890","posted_on":"2026-10-09","amount_idr":500000,"description":"SETORAN TUNAI TANPA KETERANGAN"}', 'resolved',
     'Setoran donatur untuk perpustakaan; dicatat manual di akuntansi', NULL, NULL, kep::text, '2026-10-10 14:00+07',
     '2026-10-10 06:00+07', '2026-10-10 14:00+07', 'bank', adm::text, '2026-10-10 09:00+07', 'Sesuai konfirmasi bendahara yayasan')
  ON CONFLICT DO NOTHING;

  INSERT INTO bank_statement_lines (id, school_id, account, posted_on, amount_idr, description, matched_ref, line_hash, import_id, created_at)
  VALUES
    (bl[1], sch, 'BCA 123-456-7890', '2026-07-09', 1550000, 'NICEPAY SETTLEMENT VA 20260708',         'settlement:' || sb1::text,   encode(sha256('seed40-bank-1'::bytea),'hex'), bank_import, '2026-10-10 06:00+07'),
    (bl[2], sch, 'BCA 123-456-7890', '2026-08-05',  150000, 'TRSF E-BANKING CR SANTOSO ROBOTIK AGT',  'bill_payment:' || p_up1::text, encode(sha256('seed40-bank-2'::bytea),'hex'), bank_import, '2026-10-10 06:00+07'),
    (bl[3], sch, 'BCA 123-456-7890', '2026-09-03',  400000, 'TRSF CR SANTOSO ANTAR JEMPUT SEPT',      'cashier:' || cp2::text,       encode(sha256('seed40-bank-3'::bytea),'hex'), bank_import, '2026-10-10 06:00+07'),
    (bl[4], sch, 'BCA 123-456-7890', '2026-09-08',  223425, 'NICEPAY SETTLEMENT QRIS 20260907',       'settlement:' || sb2::text,    encode(sha256('seed40-bank-4'::bytea),'hex'), bank_import, '2026-10-10 06:00+07'),
    (bl[5], sch, 'BCA 123-456-7890', '2026-09-25', 1250000, 'TRSF CR SANTOSO STUDY TOUR BDG',         'cashier:' || cp3::text,       encode(sha256('seed40-bank-5'::bytea),'hex'), bank_import, '2026-10-10 06:00+07'),
    (bl[6], sch, 'BCA 123-456-7890', '2026-10-09',  500000, 'SETORAN TUNAI TANPA KETERANGAN',         NULL,                          encode(sha256('seed40-bank-6'::bytea),'hex'), bank_import, '2026-10-10 06:00+07')
  ON CONFLICT DO NOTHING;

  -- Antrian jurnal: satu terposting, satu gagal (exception sudah dinaikkan).
  INSERT INTO gateway_journal_postings (id, school_id, reference, trans_date, description, lines, exception_id, status, attempts,
         next_attempt_at, last_error, trans_no, created_at, updated_at, source, kind, source_refs, posted_at, exception_raised_at)
  VALUES
    (pg_temp.u('1b',1), sch, 'pgw:' || sch::text || ':2026-07-08', '2026-07-08', 'Penerimaan payment gateway 2026-07-08',
     '[{"acc_code":"1104","description":"Penerimaan payment gateway","debit_idr":1550000,"credit_idr":0},{"acc_code":"1201","description":"Pelunasan piutang tagihan","debit_idr":0,"credit_idr":1550000}]',
     NULL, 'failed', 5, '2026-07-09 05:00+07', 'accounting: akun 1104 belum ada di bagan akun unit', NULL,
     '2026-07-09 01:00+07', '2026-07-09 05:00+07', 'payment_gateway', 'gateway_receipt',
     jsonb_build_object('gateway_transactions', jsonb_build_array(tx1::text)), NULL, '2026-07-09 05:00+07'),
    (pg_temp.u('1b',2), sch, 'pgw:' || sch::text || ':2026-09-07', '2026-09-07', 'Penerimaan payment gateway 2026-09-07',
     '[{"acc_code":"1104","description":"Penerimaan payment gateway","debit_idr":148950,"credit_idr":0},{"acc_code":"6301","description":"Biaya payment gateway","debit_idr":1050,"credit_idr":0},{"acc_code":"1201","description":"Pelunasan piutang tagihan","debit_idr":0,"credit_idr":150000}]',
     NULL, 'posted', 1, '2026-09-08 01:00+07', NULL, 'JU-2026-09-0007',
     '2026-09-08 01:00+07', '2026-09-08 01:01+07', 'payment_gateway', 'gateway_receipt',
     jsonb_build_object('gateway_transactions', jsonb_build_array(tx2::text)), '2026-09-08 01:01+07', NULL)
  ON CONFLICT DO NOTHING;

  -- ---- 4k. Autodebit (provider simulasi) ---------------------------------
  IF EXISTS (SELECT 1 FROM payment_providers WHERE code = 'sim') THEN
    INSERT INTO autodebit_mandates (id, school_id, guardian_user_id, student_user_id, provider, provider_mandate_ref, account_masked, status,
           max_amount_idr, redirect_url, activated_at, cancelled_at, cancelled_by, cancel_reason, idempotency_key, created_at, updated_at)
    VALUES
      (md1, sch, wali, budi, 'sim', 'SIM-MDT-20260801-0001', 'BCA ****7890', 'cancelled', 300000, NULL, '2026-08-01 10:05+07',
       '2026-09-15 08:00+07', wali, 'Pindah ke pembayaran manual lewat VA', 'seed40-mandate-1', '2026-08-01 10:00+07', '2026-09-15 08:00+07'),
      (md2, sch, wali, budi, 'sim', NULL, NULL, 'failed', 300000, NULL, NULL, NULL, NULL, NULL,
       'seed40-mandate-2', '2026-10-01 09:00+07', '2026-10-01 09:10+07'),
      (md3, sch, wali, budi, 'sim', NULL, NULL, 'pending_activation', 500000, 'https://sim.example.invalid/mandate/seed40-3', NULL, NULL, NULL, NULL,
       'seed40-mandate-3', '2026-10-14 20:00+07', '2026-10-14 20:00+07')
    ON CONFLICT DO NOTHING;
    INSERT INTO autodebit_mandate_schemes (mandate_id, scheme_id)
    VALUES (md1, s3), (md2, s3), (md3, s3), (md3, s1) ON CONFLICT DO NOTHING;
    INSERT INTO autodebit_attempts (id, school_id, mandate_id, bill_id, attempt_no, scheduled_for, amount_idr, status, skip_reason,
           gateway_transaction_id, failure_reason, idempotency_key, created_at, updated_at)
    VALUES
      (pg_temp.u('1f',1), sch, md1, b4, 1, '2026-09-04', 150000, 'failed',  NULL, NULL, 'Saldo rekening tidak mencukupi',
       'autodebit:' || md1::text || ':' || b4::text || ':1', '2026-09-04 06:00+07', '2026-09-04 06:01+07'),
      (pg_temp.u('1f',2), sch, md1, b4, 2, '2026-09-07', 150000, 'skipped', 'Tagihan sudah lunas lewat QRIS', NULL, NULL,
       'autodebit:' || md1::text || ':' || b4::text || ':2', '2026-09-07 06:00+07', '2026-09-08 06:00+07')
    ON CONFLICT DO NOTHING;
  END IF;

  -- ---- 4l. Dompet uang saku & tabungan -----------------------------------
  INSERT INTO wallets (id, school_id, student_user_id, kind, balance_idr, daily_limit_idr, status, created_at, updated_at)
  VALUES
    (wa, sch, budi, 'allowance', 290000, 50000, 'active', '2026-09-01 08:00+07', '2026-10-15 07:10+07'),
    (ws, sch, budi, 'savings',   650000, NULL,  'active', '2026-08-01 09:00+07', '2026-10-01 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO savings_requests (id, school_id, wallet_id, kind, amount_idr, note, proof_object_key, requested_by, requested_role, status,
         parent_user_id, parent_decided_at, completed_by, completed_at, created_at, updated_at)
  VALUES
    (sr1, sch, ws, 'deposit',    500000, 'Setoran awal tabungan', 'billing/savings/seed40/setoran-20260801.jpg', adm, 'admin', 'completed',
     NULL, NULL, adm, '2026-08-01 09:00+07', '2026-08-01 09:00+07', '2026-08-01 09:00+07'),
    (sr2, sch, ws, 'withdrawal', 100000, 'Membeli kalkulator ilmiah untuk OSN', 'billing/savings/seed40/penarikan-20261001.jpg', budi, 'siswa', 'completed',
     wali, '2026-09-30 19:00+07', adm, '2026-10-01 10:00+07', '2026-09-30 15:00+07', '2026-10-01 10:00+07'),
    (sr3, sch, ws, 'withdrawal',  75000, 'Beli buku referensi olimpiade', NULL, budi, 'siswa', 'pending_parent',
     wali, NULL, NULL, NULL, '2026-10-13 15:00+07', '2026-10-13 15:00+07'),
    (sr4, sch, ws, 'withdrawal', 300000, 'Beli sepatu futsal', NULL, budi, 'siswa', 'rejected',
     wali, '2026-09-20 20:00+07', NULL, NULL, '2026-09-20 14:00+07', '2026-09-20 20:00+07'),
    (sr5, sch, ws, 'withdrawal',  50000, 'Iuran kas kelas semester ganjil', NULL, budi, 'siswa', 'approved',
     wali, '2026-10-14 19:30+07', NULL, NULL, '2026-10-14 13:00+07', '2026-10-14 19:30+07')
  ON CONFLICT DO NOTHING;

  -- Kuitansi: nomor RCP-2026-0000n. Tanpa upsert DO UPDATE: kuitansi seed
  -- hanya dibuat bila counter tahun 2026 belum ada (lalu counter dibuat
  -- dengan nilai = jumlah kuitansi seed), atau bila kuitansi seed sudah ada
  -- (run ulang -> semua DO NOTHING). Bila sekolah sudah punya counter 2026
  -- milik data nyata, kuitansi seed dilewati agar nomor tidak bentrok.
  IF EXISTS (SELECT 1 FROM receipts WHERE id = pg_temp.u('2d',1))
     OR NOT EXISTS (SELECT 1 FROM receipt_sequences WHERE school_id = sch AND year = 2026) THEN
    v_seq := 0;
    FOR r IN
      SELECT * FROM (VALUES
        ('gateway', tx1, pg_temp.u('2d',1), '2026-07-08 10:15+07'::timestamptz),
        ('cashier', cp1, pg_temp.u('2d',2), '2026-07-20 10:30+07'::timestamptz),
        ('wallet',  tp3, pg_temp.u('2d',3), '2026-09-01 08:05+07'::timestamptz),
        ('wallet',  tp1, pg_temp.u('2d',4), '2026-09-01 08:10+07'::timestamptz),
        ('cashier', cp2, pg_temp.u('2d',5), '2026-09-03 10:00+07'::timestamptz),
        ('gateway', tx2, pg_temp.u('2d',6), '2026-09-07 19:20+07'::timestamptz),
        ('gateway', tx5, pg_temp.u('2d',7), '2026-09-15 07:10+07'::timestamptz),
        ('cashier', cp3, pg_temp.u('2d',8), '2026-09-25 11:00+07'::timestamptz)
      ) v(src, sid, rid, at) ORDER BY at
    LOOP
      v_seq := v_seq + 1;
      INSERT INTO receipts (id, school_id, receipt_no, source, source_id, object_key, sent_at, last_error, resend_count, created_at, updated_at)
      VALUES (r.rid, sch, format('RCP-2026-%s', lpad(v_seq::text, 5, '0')), r.src, r.sid, NULL, r.at + interval '1 minute', NULL, 0, r.at, r.at)
      ON CONFLICT DO NOTHING;
    END LOOP;
    INSERT INTO receipt_sequences (school_id, year, last) VALUES (sch, 2026, v_seq) ON CONFLICT DO NOTHING;
  ELSE
    RAISE NOTICE '40_billing_features: receipt_sequences 2026 sudah dipakai data nyata; kuitansi seed dilewati';
  END IF;

  INSERT INTO wallet_topups (id, school_id, student_user_id, wallet_kind, amount_idr, channel, gateway_transaction_id, receipt_id, status,
         requested_by, idempotency_key, notified_at, created_at, updated_at)
  VALUES
    (tp1, sch, budi, 'allowance', 150000, 'cashier', NULL, (SELECT id FROM receipts WHERE source = 'wallet' AND source_id = tp1), 'success',
     adm, 'cashier-topup:seed40-topup-1', '2026-09-01 08:11+07', '2026-09-01 08:10+07', '2026-09-01 08:10+07'),
    (tp2, sch, budi, 'allowance', 200000, 'gateway', tx5, NULL, 'success',
     wali, 'wallet-topup:seed40-topup-2', '2026-09-15 07:11+07', '2026-09-15 07:02+07', '2026-09-15 07:10+07'),
    (tp3, sch, budi, 'savings',   250000, 'cashier', NULL, (SELECT id FROM receipts WHERE source = 'wallet' AND source_id = tp3), 'success',
     adm, 'cashier-topup:seed40-topup-3', '2026-09-01 08:06+07', '2026-09-01 08:05+07', '2026-09-01 08:05+07'),
    (tp4, sch, budi, 'allowance', 100000, 'gateway', tx6, NULL, 'expired',
     wali, 'wallet-topup:seed40-topup-4', NULL, '2026-10-10 18:04+07', '2026-10-10 18:35+07'),
    (tp5, sch, budi, 'allowance',  50000, 'gateway', NULL, NULL, 'failed',
     wali, 'wallet-topup:seed40-topup-5', NULL, '2026-10-12 06:30+07', '2026-10-12 06:31+07')
  ON CONFLICT DO NOTHING;

  -- Kartu siswa: satu dicabut (hilang), satu aktif. token_hash adalah nilai
  -- seed, bukan HMAC asli, jadi QR kartu ini tidak bisa dipindai di POS;
  -- terbitkan ulang kartu dari menu admin untuk uji scan.
  INSERT INTO student_cards (id, school_id, student_user_id, token_hash, status, issued_by, revoked_at, created_at)
  VALUES
    (pg_temp.u('26',1), sch, budi, encode(sha256('seed40-card-1'::bytea), 'hex'), 'revoked', adm, '2026-09-10 08:00+07', '2026-07-15 09:00+07'),
    (pg_temp.u('26',2), sch, budi, encode(sha256('seed40-card-2'::bytea), 'hex'), 'active',  adm, NULL, '2026-09-10 08:05+07')
  ON CONFLICT DO NOTHING;

  -- ---- 4m. Kantin --------------------------------------------------------
  IF v_canteen THEN
    INSERT INTO canteen_tenants (id, school_id, code, name, location_label, owner_name, owner_phone, bank_name, bank_account_no,
           bank_account_name, status, is_open, open_changed_at, created_at, updated_at)
    VALUES
      (t1, sch, 'KNT-01', 'Kantin Bu Sari', 'Kantin Utama Lt. 1', 'Sari Wulandari', '0812-1111-2233', 'BRI', '0123-01-000111-50-1',
       'Sari Wulandari', 'active', true, '2026-10-15 06:30+07', '2026-06-30 10:00+07', '2026-10-15 06:30+07'),
      (t2, sch, 'KNT-02', 'Warung Sehat Pak Darto', 'Selasar Gedung B', 'Sudarto', '0813-2222-4455', 'BCA', '5550123456',
       'Sudarto', 'active', false, '2026-10-14 15:00+07', '2026-06-30 10:10+07', '2026-10-14 15:00+07'),
      (t3, sch, 'KNT-03', 'Kedai Jus Segar', 'Dekat Lapangan Basket', 'Rina Maharani', '0857-3333-6677', 'Mandiri', '1270009988776',
       'Rina Maharani', 'inactive', false, '2026-09-30 15:00+07', '2026-06-30 10:20+07', '2026-09-30 15:00+07')
    ON CONFLICT DO NOTHING;

    INSERT INTO canteen_tenant_schemes (id, school_id, tenant_id, kind, share_bp, share_base, rent_idr, effective_from, effective_to, created_by, created_at)
    VALUES
      (cs1, sch, t1, 'revenue_share', 1000, 'wallet', NULL,   '2026-07-01', '2026-08-31', adm, '2026-06-30 10:00+07'),
      (cs2, sch, t1, 'revenue_share', 1500, 'wallet', NULL,   '2026-09-01', NULL,         adm, '2026-08-25 10:00+07'),
      (cs3, sch, t2, 'rent',          NULL, NULL,     750000, '2026-07-01', NULL,         adm, '2026-06-30 10:10+07'),
      (cs4, sch, t3, 'revenue_share', 1000, 'all',    NULL,   '2026-07-01', NULL,         adm, '2026-06-30 10:20+07')
    ON CONFLICT DO NOTHING;

    -- Operator POS: akun admin demo dipetakan sebagai operator (tidak membuat user baru).
    INSERT INTO canteen_tenant_operators (tenant_id, user_id, school_id, email, full_name, active, created_at, updated_at)
    VALUES
      (t1, adm, sch, 'admin@teratai.local', 'Admin Teratai', true, '2026-06-30 11:00+07', '2026-06-30 11:00+07'),
      (t2, adm, sch, 'admin@teratai.local', 'Admin Teratai', true, '2026-06-30 11:00+07', '2026-06-30 11:00+07')
    ON CONFLICT DO NOTHING;

    INSERT INTO canteen_menu_items (id, school_id, tenant_id, name, category, price_idr, active, created_at, updated_at)
    VALUES
      (pg_temp.u('29',1),  sch, t1, 'Nasi Goreng Ayam',     'Makanan', 15000, true,  '2026-06-30 11:00+07', '2026-06-30 11:00+07'),
      (pg_temp.u('29',2),  sch, t1, 'Mie Ayam',             'Makanan', 12000, true,  '2026-06-30 11:00+07', '2026-06-30 11:00+07'),
      (pg_temp.u('29',3),  sch, t1, 'Es Teh Manis',         'Minuman',  4000, true,  '2026-06-30 11:00+07', '2026-06-30 11:00+07'),
      (pg_temp.u('29',4),  sch, t1, 'Roti Bakar Cokelat',   'Jajanan',  8000, true,  '2026-06-30 11:00+07', '2026-06-30 11:00+07'),
      (pg_temp.u('29',5),  sch, t1, 'Soto Ayam',            'Makanan', 13000, false, '2026-06-30 11:00+07', '2026-09-01 07:00+07'),
      (pg_temp.u('29',6),  sch, t2, 'Nasi Uduk Komplit',    'Makanan', 10000, true,  '2026-06-30 11:10+07', '2026-06-30 11:10+07'),
      (pg_temp.u('29',7),  sch, t2, 'Gado-gado',            'Makanan', 12000, true,  '2026-06-30 11:10+07', '2026-06-30 11:10+07'),
      (pg_temp.u('29',8),  sch, t2, 'Air Mineral 600 ml',   'Minuman',  3000, true,  '2026-06-30 11:10+07', '2026-06-30 11:10+07'),
      (pg_temp.u('29',9),  sch, t2, 'Pisang Goreng',        'Jajanan',  2000, true,  '2026-06-30 11:10+07', '2026-06-30 11:10+07'),
      (pg_temp.u('29',10), sch, t3, 'Jus Alpukat',          'Minuman', 10000, true,  '2026-06-30 11:20+07', '2026-06-30 11:20+07'),
      (pg_temp.u('29',11), sch, t3, 'Jus Jeruk',            'Minuman',  8000, true,  '2026-06-30 11:20+07', '2026-06-30 11:20+07')
    ON CONFLICT DO NOTHING;

    INSERT INTO canteen_daily_stock (item_id, stock_date, qty_start, qty_sold)
    VALUES
      (pg_temp.u('29',1), '2026-10-14', 40, 0), (pg_temp.u('29',2), '2026-10-14', 30, 1),
      (pg_temp.u('29',3), '2026-10-14', 80, 1), (pg_temp.u('29',4), '2026-10-14', 25, 0),
      (pg_temp.u('29',1), '2026-10-15', 40, 0), (pg_temp.u('29',2), '2026-10-15', 30, 0),
      (pg_temp.u('29',6), '2026-10-15', 35, 0), (pg_temp.u('29',7), '2026-10-15', 20, 1),
      (pg_temp.u('29',8), '2026-10-15', 60, 0)
    ON CONFLICT DO NOTHING;

    -- Settlement (dibuat sebelum penjualan karena FK canteen_sales.settlement_id)
    --   ST1 KNT-01 Sep  : bagi hasil 15% dari dompet 19.000 = 2.850; payout 16.150 (paid)
    --   ST2 KNT-02 Sep  : sewa 750.000; payout 13.000 - 750.000 = -737.000 (approved)
    --   ST3 KNT-03 Sep  : ditolak (salah periode skema), ST4 draft ulang: 10% x 20.000 = 2.000
    --   ST5 KNT-01 1-14 Okt: 15% x 16.000 = 2.400; payout 13.600 (pending_approval)
    INSERT INTO canteen_settlements (id, school_id, tenant_id, period_from, period_to, scheme_id, scheme_kind, wallet_sales_idr,
           cash_sales_idr, school_share_idr, tenant_payout_idr, status, approval_id, decided_by, decision_comment, paid_at, paid_ref,
           paid_by, created_by, created_at, updated_at)
    VALUES
      (st1, sch, t1, '2026-09-01', '2026-09-30', cs2, 'revenue_share', 19000, 40000, 2850, 16150, 'paid', NULL, kep::text,
       'Sesuai rekap POS', '2026-10-03 14:00+07', 'TRF-BRI-031026-KNT01', adm, adm, '2026-10-01 09:00+07', '2026-10-03 14:00+07'),
      (st2, sch, t2, '2026-09-01', '2026-09-30', cs3, 'rent', 13000, 18000, 750000, -737000, 'approved', NULL, kep::text,
       'Sewa September ditagihkan ke tenant', NULL, NULL, NULL, adm, '2026-10-01 09:10+07', '2026-10-02 10:00+07'),
      (st3, sch, t3, '2026-09-01', '2026-09-30', cs4, 'revenue_share', 0, 20000, 2000, -2000, 'rejected', NULL, kep::text,
       'Ditolak: rekap tunai belum dicocokkan dengan buku kas tenant', NULL, NULL, NULL, adm, '2026-10-01 09:20+07', '2026-10-02 10:05+07'),
      (st4, sch, t3, '2026-09-01', '2026-09-30', cs4, 'revenue_share', 0, 20000, 2000, -2000, 'draft', NULL, NULL,
       NULL, NULL, NULL, NULL, adm, '2026-10-05 09:00+07', '2026-10-05 09:00+07'),
      (st5, sch, t1, '2026-10-01', '2026-10-14', cs2, 'revenue_share', 16000, 30000, 2400, 13600, 'pending_approval', NULL, NULL,
       NULL, NULL, NULL, NULL, adm, '2026-10-15 07:00+07', '2026-10-15 07:00+07')
    ON CONFLICT DO NOTHING;

    -- Mutasi uang saku harus ada sebelum penjualan dompet (FK wallet_movement_id).
    INSERT INTO wallet_movements (id, school_id, wallet_id, kind, amount_idr, balance_after_idr, ref_type, ref_id, created_by, idempotency_key, created_at)
    VALUES
      (pg_temp.u('21',1), sch, wa, 'topup_cashier', 150000, 150000, 'wallet_topup', tp1, adm::text, 'cashier-topup:seed40-topup-1', '2026-09-01 08:10+07'),
      (pg_temp.u('21',2), sch, wa, 'topup_gateway', 200000, 350000, 'wallet_topup', tp2, 'gateway',  'wallet-topup:' || tp2::text,   '2026-09-15 07:10+07'),
      (pg_temp.u('21',3), sch, wa, 'pos_sale',      -19000, 331000, 'canteen_sale', NULL, adm::text, 'pos-sale:' || t1::text || ':' || pg_temp.u('2f',1)::text, '2026-09-16 10:05+07'),
      (pg_temp.u('21',4), sch, wa, 'pos_sale',      -13000, 318000, 'canteen_sale', NULL, adm::text, 'pos-sale:' || t2::text || ':' || pg_temp.u('2f',3)::text, '2026-09-17 10:10+07'),
      (pg_temp.u('21',5), sch, wa, 'pos_sale',      -16000, 302000, 'canteen_sale', NULL, adm::text, 'pos-sale:' || t1::text || ':' || pg_temp.u('2f',6)::text, '2026-10-14 10:00+07'),
      (pg_temp.u('21',6), sch, wa, 'pos_sale',       -8000, 294000, 'canteen_sale', NULL, adm::text, 'pos-sale:' || t1::text || ':' || pg_temp.u('2f',7)::text, '2026-10-14 12:05+07'),
      (pg_temp.u('21',7), sch, wa, 'pos_void',        8000, 302000, 'canteen_sale', pg_temp.u('2a',7), adm::text, 'pos-void:' || pg_temp.u('2a',7)::text, '2026-10-14 12:10+07'),
      (pg_temp.u('21',8), sch, wa, 'pos_sale',      -12000, 290000, 'canteen_sale', NULL, adm::text, 'pos-sale:' || t2::text || ':' || pg_temp.u('2f',9)::text, '2026-10-15 07:10+07')
    ON CONFLICT DO NOTHING;

    INSERT INTO canteen_sales (id, school_id, tenant_id, sale_no, pay_method, total_idr, student_user_id, wallet_movement_id, operator_user_id,
           device_id, client_txn_id, recorded_at, synced_at, offline, late_sync, status, void_reason, voided_by, voided_at, settlement_id, created_at)
    VALUES
      (pg_temp.u('2a',1), sch, t1, 'KNT-01-20260916-0001', 'wallet', 19000, budi, pg_temp.u('21',3), adm, 'POS-KNT01-01', pg_temp.u('2f',1),
       '2026-09-16 10:05+07', '2026-09-16 10:05+07', false, false, 'completed', NULL, NULL, NULL, st1, '2026-09-16 10:05+07'),
      (pg_temp.u('2a',2), sch, t1, 'KNT-01-20260916-0002', 'cash',   40000, NULL, NULL, adm, 'POS-KNT01-01', pg_temp.u('2f',2),
       '2026-09-16 10:20+07', '2026-09-16 10:31+07', true, false, 'completed', NULL, NULL, NULL, st1, '2026-09-16 10:31+07'),
      (pg_temp.u('2a',3), sch, t2, 'KNT-02-20260917-0001', 'wallet', 13000, budi, pg_temp.u('21',4), adm, 'POS-KNT02-01', pg_temp.u('2f',3),
       '2026-09-17 10:10+07', '2026-09-17 10:10+07', false, false, 'completed', NULL, NULL, NULL, st2, '2026-09-17 10:10+07'),
      (pg_temp.u('2a',4), sch, t2, 'KNT-02-20260917-0002', 'cash',   18000, NULL, NULL, adm, 'POS-KNT02-01', pg_temp.u('2f',4),
       '2026-09-17 10:15+07', '2026-09-17 10:15+07', false, false, 'completed', NULL, NULL, NULL, st2, '2026-09-17 10:15+07'),
      (pg_temp.u('2a',5), sch, t1, 'KNT-01-20261001-0001', 'cash',   30000, NULL, NULL, adm, 'POS-KNT01-01', pg_temp.u('2f',5),
       '2026-10-01 12:00+07', '2026-10-01 12:00+07', false, false, 'completed', NULL, NULL, NULL, st5, '2026-10-01 12:00+07'),
      (pg_temp.u('2a',6), sch, t1, 'KNT-01-20261014-0001', 'wallet', 16000, budi, pg_temp.u('21',5), adm, 'POS-KNT01-01', pg_temp.u('2f',6),
       '2026-10-14 10:00+07', '2026-10-14 10:00+07', false, false, 'completed', NULL, NULL, NULL, st5, '2026-10-14 10:00+07'),
      (pg_temp.u('2a',7), sch, t1, 'KNT-01-20261014-0002', 'wallet',  8000, budi, pg_temp.u('21',6), adm, 'POS-KNT01-01', pg_temp.u('2f',7),
       '2026-10-14 12:05+07', '2026-10-14 12:05+07', false, false, 'voided', 'Salah pilih menu, siswa batal membeli', adm, '2026-10-14 12:10+07',
       NULL, '2026-10-14 12:05+07'),
      (pg_temp.u('2a',8), sch, t3, 'KNT-03-20260918-0001', 'cash',   20000, NULL, NULL, adm, 'POS-KNT03-01', pg_temp.u('2f',8),
       '2026-09-18 13:00+07', '2026-09-18 13:00+07', false, false, 'completed', NULL, NULL, NULL, st4, '2026-09-18 13:00+07'),
      (pg_temp.u('2a',9), sch, t2, 'KNT-02-20261015-0001', 'wallet', 12000, budi, pg_temp.u('21',8), adm, 'POS-KNT02-01', pg_temp.u('2f',9),
       '2026-10-15 07:10+07', '2026-10-15 07:10+07', false, false, 'completed', NULL, NULL, NULL, NULL, '2026-10-15 07:10+07')
    ON CONFLICT DO NOTHING;

    INSERT INTO canteen_sale_items (id, sale_id, item_id, name_snapshot, qty, unit_price_idr, line_total_idr)
    VALUES
      (pg_temp.u('2b',1),  pg_temp.u('2a',1), pg_temp.u('29',1),  'Nasi Goreng Ayam',   1, 15000, 15000),
      (pg_temp.u('2b',2),  pg_temp.u('2a',1), pg_temp.u('29',3),  'Es Teh Manis',       1,  4000,  4000),
      (pg_temp.u('2b',3),  pg_temp.u('2a',2), pg_temp.u('29',2),  'Mie Ayam',           2, 12000, 24000),
      (pg_temp.u('2b',4),  pg_temp.u('2a',2), pg_temp.u('29',4),  'Roti Bakar Cokelat', 2,  8000, 16000),
      (pg_temp.u('2b',5),  pg_temp.u('2a',3), pg_temp.u('29',6),  'Nasi Uduk Komplit',  1, 10000, 10000),
      (pg_temp.u('2b',6),  pg_temp.u('2a',3), pg_temp.u('29',8),  'Air Mineral 600 ml', 1,  3000,  3000),
      (pg_temp.u('2b',7),  pg_temp.u('2a',4), pg_temp.u('29',7),  'Gado-gado',          1, 12000, 12000),
      (pg_temp.u('2b',8),  pg_temp.u('2a',4), pg_temp.u('29',8),  'Air Mineral 600 ml', 2,  3000,  6000),
      (pg_temp.u('2b',9),  pg_temp.u('2a',5), pg_temp.u('29',1),  'Nasi Goreng Ayam',   2, 15000, 30000),
      (pg_temp.u('2b',10), pg_temp.u('2a',6), pg_temp.u('29',2),  'Mie Ayam',           1, 12000, 12000),
      (pg_temp.u('2b',11), pg_temp.u('2a',6), pg_temp.u('29',3),  'Es Teh Manis',       1,  4000,  4000),
      (pg_temp.u('2b',12), pg_temp.u('2a',7), pg_temp.u('29',4),  'Roti Bakar Cokelat', 1,  8000,  8000),
      (pg_temp.u('2b',13), pg_temp.u('2a',8), pg_temp.u('29',10), 'Jus Alpukat',        2, 10000, 20000),
      (pg_temp.u('2b',14), pg_temp.u('2a',9), pg_temp.u('29',7),  'Gado-gado',          1, 12000, 12000)
    ON CONFLICT DO NOTHING;

    INSERT INTO canteen_sale_sequences (tenant_id, sale_date, last)
    VALUES (t1, '2026-09-16', 2), (t2, '2026-09-17', 2), (t3, '2026-09-18', 1),
           (t1, '2026-10-01', 1), (t1, '2026-10-14', 2), (t2, '2026-10-15', 1)
    ON CONFLICT DO NOTHING;
  END IF;

  -- Mutasi tabungan
  INSERT INTO wallet_movements (id, school_id, wallet_id, kind, amount_idr, balance_after_idr, ref_type, ref_id, created_by, idempotency_key, created_at)
  VALUES
    (pg_temp.u('21',11), sch, ws, 'savings_deposit',    500000, 500000, 'savings_request', sr1, adm::text, 'savings-deposit:' || sr1::text,    '2026-08-01 09:00+07'),
    (pg_temp.u('21',12), sch, ws, 'topup_cashier',      250000, 750000, 'wallet_topup',    tp3, adm::text, 'cashier-topup:seed40-topup-3',     '2026-09-01 08:05+07'),
    (pg_temp.u('21',13), sch, ws, 'savings_withdrawal',-100000, 650000, 'savings_request', sr2, adm::text, 'savings-withdrawal:' || sr2::text, '2026-10-01 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO wallet_reconciliations (id, school_id, run_on, wallet_kind, wallet_total_idr, ledger_idr, diff_idr, status, created_at)
  VALUES
    (pg_temp.u('24',1), sch, '2026-10-13', 'allowance', 318000, 318000, 0, 'matched', '2026-10-14 01:00+07'),
    (pg_temp.u('24',2), sch, '2026-10-13', 'savings',   650000, 650000, 0, 'matched', '2026-10-14 01:00+07'),
    (pg_temp.u('24',3), sch, '2026-10-14', 'allowance', 302000, 302000, 0, 'matched', '2026-10-15 01:00+07'),
    (pg_temp.u('24',4), sch, '2026-10-14', 'savings',   650000, 650000, 0, 'matched', '2026-10-15 01:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO wallet_closures (id, school_id, wallet_id, disposition, target_student_user_id, amount_idr, status, approval_id, refund_id,
         requested_by, created_at, updated_at)
  VALUES (pg_temp.u('25',1), sch, ws, 'refund', NULL, 750000, 'rejected', NULL, NULL, adm, '2026-09-25 09:00+07', '2026-09-26 10:00+07')
  ON CONFLICT DO NOTHING;

  END IF; -- alur uang Budi

  -- =========================================================================
  -- 5. Impor riwayat pembayaran: melengkapi tagihan SPP Juli & Agustus dari
  --    03_billing.sql yang berstatus 'paid' tanpa pembayaran (ledger kosong).
  --    Hanya untuk tagihan yang memang belum punya alokasi terverifikasi.
  -- =========================================================================
  IF NOT EXISTS (SELECT 1 FROM payment_imports WHERE id = pi1) THEN
    FOR v_old IN
      SELECT b.id, b.net_idr, b.period_month
        FROM student_bills b
        JOIN billing_schemes s ON s.id = b.scheme_id
       WHERE b.school_id = sch AND b.student_user_id = budi AND s.name = 'SPP Reguler SMP TPGS 2026/2027'
         AND b.period_year = 2026 AND b.period_month IN (7, 8) AND b.status = 'paid' AND b.net_idr > 0
         AND NOT EXISTS (SELECT 1 FROM bill_payments p WHERE p.student_bill_id = b.id AND p.status = 'verified')
       ORDER BY b.period_month
    LOOP
      INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, transfer_at, bank_name, reference, status, source, source_id,
             idempotency_key, uploader_user_id, verified_at, review_note, created_at, updated_at)
      VALUES (pg_temp.u('06', 20 + v_old.period_month), sch, v_old.id, v_old.net_idr,
              make_timestamptz(2026, v_old.period_month, 8, 12, 0, 0, 'Asia/Jakarta'), 'Mutasi BCA',
              format('BCA-%s-SPP', lpad(v_old.period_month::text, 2, '0')), 'verified', 'import', pi1,
              'import:seed40-spp-2026-' || v_old.period_month, adm, make_timestamptz(2026, v_old.period_month, 8, 12, 0, 0, 'Asia/Jakarta'),
              'Impor riwayat pembayaran', '2026-10-05 09:00+07', '2026-10-05 09:00+07')
      ON CONFLICT DO NOTHING;
      INSERT INTO bill_payment_allocations (id, school_id, payment_id, amount_idr, created_at)
      VALUES (pg_temp.u('07', 20 + v_old.period_month), sch, pg_temp.u('06', 20 + v_old.period_month), v_old.net_idr, '2026-10-05 09:00+07')
      ON CONFLICT DO NOTHING;
      v_imported := v_imported + 1;
      v_total := v_total + v_old.net_idr;
    END LOOP;
    INSERT INTO payment_imports (id, school_id, file_name, source_name, row_count, imported, skipped, total_idr, imported_by, created_at)
    VALUES (pi1, sch, 'mutasi-bca-spp-juli-agustus-2026.csv', 'Mutasi BCA', 2, v_imported, 2 - v_imported, v_total, adm, '2026-10-05 09:00+07')
    ON CONFLICT DO NOTHING;
  END IF;
  INSERT INTO payment_imports (id, school_id, file_name, source_name, row_count, imported, skipped, total_idr, imported_by, created_at)
  VALUES (pi2, sch, 'mutasi-bca-spp-juli-agustus-2026 (unggah ulang).csv', 'Mutasi BCA', 2, 0, 2, 0, adm, '2026-10-05 09:20+07')
  ON CONFLICT DO NOTHING;
END $$;

COMMIT;
