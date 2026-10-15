-- 60_ops_features.sql
-- Data demo untuk SEMUA fitur layanan ops (teratai_ops) Sekolah Teratai Putih:
-- fasilitas & booking ruang, perintah kerja, inventaris & stock opname,
-- pengadaan (vendor -> PR -> PO -> penerimaan -> retur -> evaluasi), aset tetap,
-- anggaran/RAPBS, pajak non-gaji, utang usaha (AP), perpustakaan, transportasi,
-- dan kampus 3D.
--
-- Aturan:
--   * HANYA menambah data. Tidak ada DELETE/TRUNCATE/DROP/UPDATE.
--   * UUID tetap berawalan '60TT0000-0000-4000-8000-...' (TT = kode tabel),
--     ON CONFLICT DO NOTHING -> idempoten (aman dijalankan berulang).
--   * Satu blok DO = satu transaksi; satu kegagalan membatalkan seluruh file.
--   * Bergantung pada master dari 04_ops.sql (gedung, ruang, gudang WH-MAIN,
--     barang ATK, vendor VND-001, aset AST-*, katalog perpustakaan, rute RTE-01,
--     kendaraan B 7123 KAA, kru Pak Bambang, model 3D gedung A v1).
--   * Pengguna dari identity seed: admin 2222 (memegang peran
--     inventory_officer/procurement_officer/asset_officer/pustakawan/
--     facility_officer/teknisi/transport_officer), orang tua 3333, siswa Budi
--     4444, guru 5555, wali kelas 6666, kepala sekolah 7777, kepala yayasan 8888.
--     Belum ada akun ber-peran 'pengemudi' di identity seed: kru pengemudi
--     dicatat sebagai nama eksternal; pendamping Rute 01 ditautkan ke admin
--     (transport_officer) agar halaman "Tugas Perjalanan" punya isi.

BEGIN;

DO $$
DECLARE
  v_s        UUID := '11111111-1111-1111-1111-111111111111';
  v_admin    UUID := '22222222-2222-2222-2222-222222222222';
  v_ortu     UUID := '33333333-3333-3333-3333-333333333333';
  v_budi     UUID := '44444444-4444-4444-4444-444444444444';
  v_guru     UUID := '55555555-5555-5555-5555-555555555555';
  v_wali     UUID := '66666666-6666-6666-6666-666666666666';
  v_kepsek   UUID := '77777777-7777-7777-7777-777777777777';
  v_yayasan  UUID := '88888888-8888-8888-8888-888888888888';
  v_core_apr UUID := '77000000-0000-0000-0000-000000000001'; -- approval PO alat peraga (16_core.sql)

  -- master dari 04_ops.sql
  v_bldg_a UUID; v_bldg_b UUID; v_fl_a1 UUID; v_fl_a2 UUID; v_fl_b1 UUID;
  v_room_aula UUID; v_room_perpus UUID; v_room_gudang UUID; v_room_labkom UUID;
  v_wh_main UUID; v_cat_atk UUID; v_item_kertas UUID; v_item_spidol UUID;
  v_vnd1 UUID; v_astcat_elek UUID; v_ast_ltp1 UUID; v_ast_prj1 UUID;
  v_libcat_pel UUID; v_libtitle_mtk UUID; v_mcat_siswa UUID; v_mem_budi UUID;
  v_route1 UUID; v_veh1 UUID; v_crew_bambang UUID; v_model_a1 UUID;
  -- jenis PPh bawaan (seed_default_tax_types)
  v_wt_23 UUID; v_wt_22 UUID; v_wt_42rent UUID; v_wt_42con UUID;

  -- ID tetap milik file ini (ruang, gudang, barang, vendor, dst.)
  r_labipa  UUID := '60020000-0000-4000-8000-000000000001';
  r_kls7a   UUID := '60020000-0000-4000-8000-000000000002';
  r_lapang  UUID := '60020000-0000-4000-8000-000000000003';
  r_rapat   UUID := '60020000-0000-4000-8000-000000000004';
  r_uks     UUID := '60020000-0000-4000-8000-000000000005';
  r_kantin  UUID := '60020000-0000-4000-8000-000000000006';

  ic_kbr  UUID := '60100000-0000-4000-8000-000000000001';
  ic_lab  UUID := '60100000-0000-4000-8000-000000000002';
  ic_tik  UUID := '60100000-0000-4000-8000-000000000003';
  it_tinta  UUID := '60110000-0000-4000-8000-000000000001';
  it_map    UUID := '60110000-0000-4000-8000-000000000002';
  it_sabun  UUID := '60110000-0000-4000-8000-000000000003';
  it_tisu   UUID := '60110000-0000-4000-8000-000000000004';
  it_alko   UUID := '60110000-0000-4000-8000-000000000005';
  it_laptop UUID := '60110000-0000-4000-8000-000000000006';
  it_spmrh  UUID := '60110000-0000-4000-8000-000000000007';
  it_kapur  UUID := '60110000-0000-4000-8000-000000000008';
  wh_lab    UUID := '60120000-0000-4000-8000-000000000001';

  vd_gram UUID := '60200000-0000-4000-8000-000000000001';
  vd_komp UUID := '60200000-0000-4000-8000-000000000002';
  vd_bers UUID := '60200000-0000-4000-8000-000000000003';
  vd_labo UUID := '60200000-0000-4000-8000-000000000004';
  vd_mebl UUID := '60200000-0000-4000-8000-000000000005';
  vd_elek UUID := '60200000-0000-4000-8000-000000000006';

  bl_atk  UUID := '60410000-0000-4000-8000-000000000001';
  bl_lab  UUID := '60410000-0000-4000-8000-000000000002';
  bl_tik  UUID := '60410000-0000-4000-8000-000000000003';
  bl_pml  UUID := '60410000-0000-4000-8000-000000000004';
  bl_hnr  UUID := '60410000-0000-4000-8000-000000000005';
  bl_kbr  UUID := '60410000-0000-4000-8000-000000000006';

  ac_mbl UUID := '60300000-0000-4000-8000-000000000001';

  mc_gtk UUID := '60700000-0000-4000-8000-000000000001';
  mc_osn UUID := '60700000-0000-4000-8000-000000000002';
  m_guru UUID := '60720000-0000-4000-8000-000000000001';
  m_wali UUID := '60720000-0000-4000-8000-000000000002';
  m_ks   UUID := '60720000-0000-4000-8000-000000000003';
  m_adm  UUID := '60720000-0000-4000-8000-000000000004';
  m_yys  UUID := '60720000-0000-4000-8000-000000000005';
  lc_fik UUID := '60730000-0000-4000-8000-000000000001';
  lc_ref UUID := '60730000-0000-4000-8000-000000000002';
  lc_sai UUID := '60730000-0000-4000-8000-000000000003';

  z1 UUID := '60800000-0000-4000-8000-000000000001';
  z2 UUID := '60800000-0000-4000-8000-000000000002';
  z3 UUID := '60800000-0000-4000-8000-000000000003';
  rt2 UUID := '608c0000-0000-4000-8000-000000000002';
  rt3 UUID := '608c0000-0000-4000-8000-000000000003';
  veh2 UUID := '60820000-0000-4000-8000-000000000002';
  veh3 UUID := '60820000-0000-4000-8000-000000000003';
  cr_sugeng UUID := '60840000-0000-4000-8000-000000000001';
  cr_rina   UUID := '60840000-0000-4000-8000-000000000002';
  cr_admin  UUID := '60840000-0000-4000-8000-000000000003';
  cr_joko   UUID := '60840000-0000-4000-8000-000000000004';

  chk RECORD;
  ok BOOLEAN;
BEGIN
  ---------------------------------------------------------------------------
  -- 0. Prasyarat master dari 04_ops.sql
  ---------------------------------------------------------------------------
  SELECT id INTO v_bldg_a FROM buildings WHERE school_id = v_s AND code = 'BLDG-A' AND deleted_at IS NULL;
  SELECT id INTO v_bldg_b FROM buildings WHERE school_id = v_s AND code = 'BLDG-B' AND deleted_at IS NULL;
  SELECT id INTO v_fl_a1 FROM floors WHERE building_id = v_bldg_a AND level = 1 AND deleted_at IS NULL;
  SELECT id INTO v_fl_a2 FROM floors WHERE building_id = v_bldg_a AND level = 2 AND deleted_at IS NULL;
  SELECT id INTO v_room_aula   FROM rooms WHERE school_id = v_s AND code = 'R-AULA'     AND deleted_at IS NULL;
  SELECT id INTO v_room_perpus FROM rooms WHERE school_id = v_s AND code = 'R-PERPUS'   AND deleted_at IS NULL;
  SELECT id INTO v_room_gudang FROM rooms WHERE school_id = v_s AND code = 'R-GUDANG'   AND deleted_at IS NULL;
  SELECT id INTO v_room_labkom FROM rooms WHERE school_id = v_s AND code = 'R-LABKOM-1' AND deleted_at IS NULL;
  SELECT id INTO v_wh_main FROM warehouses WHERE school_id = v_s AND code = 'WH-MAIN' AND deleted_at IS NULL;
  SELECT id INTO v_cat_atk FROM item_categories WHERE school_id = v_s AND code = 'CAT-ATK' AND deleted_at IS NULL;
  SELECT id INTO v_item_kertas FROM items WHERE school_id = v_s AND code = 'ITM-HVS-A4' AND deleted_at IS NULL;
  SELECT id INTO v_item_spidol FROM items WHERE school_id = v_s AND code = 'ITM-SPIDOL-BLK' AND deleted_at IS NULL;
  SELECT id INTO v_vnd1 FROM vendors WHERE school_id = v_s AND code = 'VND-001' AND deleted_at IS NULL;
  SELECT id INTO v_astcat_elek FROM asset_categories WHERE school_id = v_s AND code = 'AST-ELEK' AND deleted_at IS NULL;
  SELECT id INTO v_ast_ltp1 FROM assets WHERE qr_token = 'QR-TPGS-AST-001';
  SELECT id INTO v_ast_prj1 FROM assets WHERE qr_token = 'QR-TPGS-AST-002';
  SELECT id INTO v_libcat_pel FROM lib_collection_categories WHERE school_id = v_s AND code = 'CAT-PELAJARAN' AND deleted_at IS NULL;
  SELECT id INTO v_libtitle_mtk FROM lib_titles
   WHERE school_id = v_s AND title = 'Matematika SMP Kelas VII Kurikulum Merdeka' AND deleted_at IS NULL
   ORDER BY created_at, id LIMIT 1;
  SELECT id INTO v_mcat_siswa FROM lib_member_categories
   WHERE school_id = v_s AND applies_to = 'student' AND is_default AND deleted_at IS NULL;
  SELECT id INTO v_mem_budi FROM lib_members WHERE school_id = v_s AND user_id = v_budi;
  SELECT id INTO v_route1 FROM transport_routes WHERE school_id = v_s AND code = 'RTE-01' AND deleted_at IS NULL;
  SELECT id INTO v_veh1 FROM transport_vehicles WHERE school_id = v_s AND plate_no = 'B 7123 KAA' AND deleted_at IS NULL;
  SELECT id INTO v_crew_bambang FROM transport_crew
   WHERE school_id = v_s AND external_name = 'Pak Bambang Supir' AND deleted_at IS NULL
   ORDER BY created_at, id LIMIT 1;
  SELECT id INTO v_model_a1 FROM campus3d_models WHERE building_id = v_bldg_a AND version = 1;

  IF v_bldg_a IS NULL OR v_bldg_b IS NULL OR v_fl_a1 IS NULL OR v_fl_a2 IS NULL
     OR v_room_aula IS NULL OR v_room_perpus IS NULL OR v_room_gudang IS NULL OR v_room_labkom IS NULL
     OR v_wh_main IS NULL OR v_cat_atk IS NULL OR v_item_kertas IS NULL OR v_item_spidol IS NULL
     OR v_vnd1 IS NULL OR v_astcat_elek IS NULL OR v_ast_ltp1 IS NULL OR v_ast_prj1 IS NULL
     OR v_libcat_pel IS NULL OR v_libtitle_mtk IS NULL OR v_mcat_siswa IS NULL OR v_mem_budi IS NULL
     OR v_route1 IS NULL OR v_veh1 IS NULL OR v_crew_bambang IS NULL OR v_model_a1 IS NULL THEN
    RAISE EXCEPTION '60_ops_features: master dari 04_ops.sql belum lengkap; jalankan 04_ops.sql dulu';
  END IF;

  ---------------------------------------------------------------------------
  -- 1. FASILITAS: lantai & ruang tambahan, fasilitas ruang, jadwal tetap,
  --    booking, perawatan berkala, perintah kerja
  ---------------------------------------------------------------------------
  INSERT INTO floors (id, school_id, building_id, level, name)
  VALUES ('60010000-0000-4000-8000-000000000001', v_s, v_bldg_b, 1, 'Lantai 1 Gedung Laboratorium')
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_fl_b1 FROM floors WHERE building_id = v_bldg_b AND level = 1 AND deleted_at IS NULL;

  INSERT INTO rooms (id, school_id, floor_id, code, name, kind, capacity, usage, requires_approval, status, available_from, available_to, notes) VALUES
    (r_labipa, v_s, v_fl_b1, 'R-LAB-IPA',  'Laboratorium IPA Terpadu',     'lab',             32, 'bookable', true,  'active',      '07:00', '16:00', 'Pemakaian di luar jadwal wajib disetujui koordinator lab'),
    (r_kls7a,  v_s, v_fl_a2, 'R-KLS-7A',   'Ruang Kelas VII A',            'kelas',           32, 'fixed',    false, 'active',      NULL, NULL, NULL),
    (r_lapang, v_s, v_fl_a1, 'R-LAPANGAN', 'Lapangan Olahraga Serbaguna',  'lapangan',       200, 'bookable', false, 'active',      '06:00', '18:00', 'Futsal, basket, upacara'),
    (r_rapat,  v_s, v_fl_a2, 'R-RAPAT',    'Ruang Rapat Guru',             'ruang_serbaguna', 20, 'bookable', false, 'active',      '07:00', '17:00', NULL),
    (r_uks,    v_s, v_fl_a1, 'R-UKS',      'Ruang UKS',                    'uks',              6, 'fixed',    false, 'maintenance', NULL, NULL, 'Perbaikan wastafel'),
    (r_kantin, v_s, v_fl_a1, 'R-KANTIN',   'Kantin Sehat Teratai',         'kantin',         120, 'fixed',    false, 'active',      NULL, NULL, NULL)
  ON CONFLICT DO NOTHING;

  INSERT INTO room_facilities (id, school_id, room_id, name, qty) VALUES
    ('60030000-0000-4000-8000-000000000001', v_s, v_room_aula,   'Proyektor & layar 150 inci', 1),
    ('60030000-0000-4000-8000-000000000002', v_s, v_room_aula,   'Sound system + 4 mikrofon nirkabel', 1),
    ('60030000-0000-4000-8000-000000000003', v_s, v_room_aula,   'Kursi lipat', 300),
    ('60030000-0000-4000-8000-000000000004', v_s, v_room_labkom, 'PC siswa', 36),
    ('60030000-0000-4000-8000-000000000005', v_s, v_room_labkom, 'AC split 2 PK', 2),
    ('60030000-0000-4000-8000-000000000006', v_s, r_labipa,      'Mikroskop binokuler', 16),
    ('60030000-0000-4000-8000-000000000007', v_s, r_labipa,      'Lemari asam', 1),
    ('60030000-0000-4000-8000-000000000008', v_s, r_rapat,       'Smart TV 65 inci', 1),
    ('60030000-0000-4000-8000-000000000009', v_s, r_kls7a,       'Meja & kursi siswa', 32)
  ON CONFLICT DO NOTHING;

  INSERT INTO room_fixed_schedules (id, school_id, room_id, weekday, starts_time, ends_time, label, valid_from, valid_to) VALUES
    ('60040000-0000-4000-8000-000000000001', v_s, v_room_labkom, 1, '07:30', '09:00', 'Informatika VII A', '2026-07-13', '2026-12-18'),
    ('60040000-0000-4000-8000-000000000002', v_s, v_room_labkom, 3, '10:00', '11:30', 'Informatika VIII B', '2026-07-13', '2026-12-18'),
    ('60040000-0000-4000-8000-000000000003', v_s, r_labipa,      2, '08:00', '09:30', 'Praktikum IPA VII A', '2026-07-13', '2026-12-18'),
    ('60040000-0000-4000-8000-000000000004', v_s, r_lapang,      5, '07:00', '08:30', 'PJOK VII A & VII B', '2026-07-13', NULL),
    ('60040000-0000-4000-8000-000000000005', v_s, r_lapang,      1, '06:45', '07:30', 'Upacara Bendera', '2026-07-13', NULL)
  ON CONFLICT DO NOTHING;

  INSERT INTO room_bookings (id, school_id, room_id, starts_at, ends_at, purpose, participants, requester_user_id, requester_name, source, status, cancel_reason, cancelled_by, idempotency_key) VALUES
    ('60050000-0000-4000-8000-000000000001', v_s, v_room_aula,   '2026-10-20 08:00+07', '2026-10-20 12:00+07', 'Rapat Pleno Komite Sekolah', 150, v_admin, 'Admin Teratai', 'internal', 'confirmed', NULL, NULL, 'seed-ops-bk-01'),
    ('60050000-0000-4000-8000-000000000002', v_s, r_labipa,      '2026-10-21 13:00+07', '2026-10-21 15:00+07', 'Praktikum tambahan persiapan OSN IPA', 20, v_guru, 'Guru Matematika', 'portal', 'pending_approval', NULL, NULL, 'seed-ops-bk-02'),
    ('60050000-0000-4000-8000-000000000003', v_s, r_lapang,      '2026-10-24 07:00+07', '2026-10-24 11:00+07', 'Class Meeting Futsal antarkelas', 120, v_wali, 'Wali Kelas VII A', 'portal', 'confirmed', NULL, NULL, 'seed-ops-bk-03'),
    ('60050000-0000-4000-8000-000000000004', v_s, r_rapat,       '2026-10-16 13:00+07', '2026-10-16 14:30+07', 'Rapat MGMP Matematika', 12, v_guru, 'Guru Matematika', 'portal', 'rejected', NULL, NULL, 'seed-ops-bk-04'),
    ('60050000-0000-4000-8000-000000000005', v_s, v_room_aula,   '2026-10-10 08:00+07', '2026-10-10 10:00+07', 'Sosialisasi Program Kerja OSIS', 200, v_wali, 'Wali Kelas VII A', 'portal', 'cancelled', 'Acara dipindah ke lapangan', v_wali, 'seed-ops-bk-05'),
    ('60050000-0000-4000-8000-000000000006', v_s, v_room_labkom, '2026-10-08 13:00+07', '2026-10-08 15:00+07', 'Pelatihan Guru: Asesmen Digital', 30, v_kepsek, 'Kepala Sekolah', 'internal', 'confirmed', NULL, NULL, 'seed-ops-bk-06')
  ON CONFLICT DO NOTHING;

  INSERT INTO maintenance_schedules (id, school_id, room_id, asset_id, title, priority, interval_unit, interval_count, next_due_date, default_assignee_user_id, default_assignee_name, active, created_by) VALUES
    ('60060000-0000-4000-8000-000000000001', v_s, v_room_labkom, NULL,       'Pembersihan filter AC Lab Komputer', 'sedang', 'month', 1, '2026-11-01', v_admin, 'Tim Teknisi Sarpras', true,  v_admin),
    ('60060000-0000-4000-8000-000000000002', v_s, v_room_aula,   v_ast_prj1, 'Servis berkala proyektor aula',      'rendah', 'month', 3, '2026-12-01', v_admin, 'Tim Teknisi Sarpras', true,  v_admin),
    ('60060000-0000-4000-8000-000000000003', v_s, v_room_gudang, NULL,       'Pengecekan APAR seluruh gedung',      'tinggi', 'month', 6, '2027-01-15', v_admin, 'Tim Teknisi Sarpras', true,  v_admin),
    ('60060000-0000-4000-8000-000000000004', v_s, r_lapang,      NULL,       'Pembersihan saluran air lapangan',    'sedang', 'week',  2, '2026-10-19', NULL,    NULL,                  true,  v_admin),
    ('60060000-0000-4000-8000-000000000005', v_s, r_uks,         NULL,       'Sterilisasi ruang UKS (dinonaktifkan)', 'rendah', 'month', 1, '2026-09-01', NULL, NULL,                  false, v_admin)
  ON CONFLICT DO NOTHING;

  INSERT INTO work_orders (id, school_id, code, source, room_id, asset_id, title, description, priority, status,
                           reporter_user_id, reporter_name, assignee_user_id, assignee_name, completion_note, completed_at,
                           cancel_reason, schedule_id, scheduled_for, ticket_ref, purchase_request_ref, created_at, updated_at) VALUES
    ('60070000-0000-4000-8000-000000000001', v_s, 'WO-2026-0001', 'report', v_room_labkom, NULL, 'AC Lab Komputer berisik dan kurang dingin',
     'Unit AC sisi kiri berbunyi keras sejak pagi, suhu ruang 29 derajat.', 'tinggi', 'selesai',
     v_guru, 'Guru Matematika', v_admin, 'Tim Teknisi Sarpras', 'Kompresor dibersihkan, freon ditambah 1 kg. Suhu kembali 22 derajat.', '2026-10-03 14:00+07',
     NULL, NULL, NULL, NULL, NULL, '2026-10-01 08:15+07', '2026-10-03 14:00+07'),
    ('60070000-0000-4000-8000-000000000002', v_s, 'WO-2026-0002', 'preventive', v_room_labkom, NULL, 'Pembersihan filter AC Lab Komputer (Oktober)',
     'Perintah kerja preventif dari jadwal perawatan bulanan.', 'sedang', 'ditutup',
     v_admin, 'Admin Teratai', v_admin, 'Tim Teknisi Sarpras', 'Filter dua unit AC sudah dicuci.', '2026-10-02 10:00+07',
     NULL, '60060000-0000-4000-8000-000000000001', '2026-10-01', NULL, NULL, '2026-10-01 00:05+07', '2026-10-02 11:00+07'),
    ('60070000-0000-4000-8000-000000000003', v_s, 'WO-2026-0003', 'report', v_room_aula, v_ast_prj1, 'Proyektor aula redup',
     'Gambar proyektor buram dan redup saat acara pagi.', 'sedang', 'diproses',
     v_wali, 'Wali Kelas VII A', v_admin, 'Tim Teknisi Sarpras', NULL, NULL,
     NULL, NULL, NULL, NULL, NULL, '2026-10-09 09:30+07', '2026-10-10 08:00+07'),
    ('60070000-0000-4000-8000-000000000004', v_s, 'WO-2026-0004', 'report', r_uks, NULL, 'Kran wastafel UKS bocor',
     'Air menetes terus dari kran wastafel, lantai licin.', 'sedang', 'dilaporkan',
     v_guru, 'Guru Matematika', NULL, NULL, NULL, NULL,
     NULL, NULL, NULL, NULL, NULL, '2026-10-14 10:20+07', '2026-10-14 10:20+07'),
    ('60070000-0000-4000-8000-000000000005', v_s, 'WO-2026-0005', 'ticket', v_room_labkom, NULL, 'Pembersihan Filter AC Lab Komputer 1',
     'Diteruskan dari Service Desk tiket TKT-2026-0002.', 'sedang', 'ditugaskan',
     v_admin, 'Admin Teratai', v_admin, 'Tim Teknisi Sarpras', NULL, NULL,
     NULL, NULL, NULL, 'TKT-2026-0002', NULL, '2026-10-12 13:00+07', '2026-10-12 13:30+07'),
    ('60070000-0000-4000-8000-000000000006', v_s, 'WO-2026-0006', 'report', r_kls7a, NULL, 'Empat lampu Ruang Kelas VII A mati',
     'Lampu TL baris belakang mati, perlu penggantian ke LED.', 'tinggi', 'menunggu_material',
     v_wali, 'Wali Kelas VII A', v_admin, 'Tim Teknisi Sarpras', NULL, NULL,
     NULL, NULL, NULL, NULL, 'PR-2026-0003', '2026-10-06 07:40+07', '2026-10-07 09:00+07'),
    ('60070000-0000-4000-8000-000000000007', v_s, 'WO-2026-0007', 'report', v_room_aula, NULL, 'Kursi aula patah',
     'Satu kursi lipat baris depan patah kaki.', 'rendah', 'dibatalkan',
     v_guru, 'Guru Matematika', NULL, NULL, NULL, NULL,
     'Kursi sudah diganti dari stok cadangan aula', NULL, NULL, NULL, NULL, '2026-09-22 11:00+07', '2026-09-22 15:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO work_order_events (id, school_id, work_order_id, from_status, to_status, note, actor_user_id, created_at) VALUES
    ('60090000-0000-4000-8000-000000000001', v_s, '60070000-0000-4000-8000-000000000001', NULL, 'dilaporkan', NULL, v_guru, '2026-10-01 08:15+07'),
    ('60090000-0000-4000-8000-000000000002', v_s, '60070000-0000-4000-8000-000000000001', 'dilaporkan', 'ditugaskan', 'Ditugaskan ke tim teknisi', v_admin, '2026-10-01 09:00+07'),
    ('60090000-0000-4000-8000-000000000003', v_s, '60070000-0000-4000-8000-000000000001', 'ditugaskan', 'diproses', NULL, v_admin, '2026-10-02 08:00+07'),
    ('60090000-0000-4000-8000-000000000004', v_s, '60070000-0000-4000-8000-000000000001', 'diproses', 'selesai', 'Kompresor dibersihkan, freon ditambah', v_admin, '2026-10-03 14:00+07'),
    ('60090000-0000-4000-8000-000000000005', v_s, '60070000-0000-4000-8000-000000000002', NULL, 'ditugaskan', 'Dibuat otomatis dari jadwal perawatan', NULL, '2026-10-01 00:05+07'),
    ('60090000-0000-4000-8000-000000000006', v_s, '60070000-0000-4000-8000-000000000002', 'ditugaskan', 'selesai', NULL, v_admin, '2026-10-02 10:00+07'),
    ('60090000-0000-4000-8000-000000000007', v_s, '60070000-0000-4000-8000-000000000002', 'selesai', 'ditutup', 'Diverifikasi koordinator sarpras', v_admin, '2026-10-02 11:00+07'),
    ('60090000-0000-4000-8000-000000000008', v_s, '60070000-0000-4000-8000-000000000003', NULL, 'dilaporkan', NULL, v_wali, '2026-10-09 09:30+07'),
    ('60090000-0000-4000-8000-000000000009', v_s, '60070000-0000-4000-8000-000000000003', 'dilaporkan', 'ditugaskan', NULL, v_admin, '2026-10-09 10:00+07'),
    ('60090000-0000-4000-8000-00000000000a', v_s, '60070000-0000-4000-8000-000000000003', 'ditugaskan', 'diproses', 'Lampu proyektor dipesan', v_admin, '2026-10-10 08:00+07'),
    ('60090000-0000-4000-8000-00000000000b', v_s, '60070000-0000-4000-8000-000000000004', NULL, 'dilaporkan', NULL, v_guru, '2026-10-14 10:20+07'),
    ('60090000-0000-4000-8000-00000000000c', v_s, '60070000-0000-4000-8000-000000000005', NULL, 'ditugaskan', 'Dari tiket Service Desk TKT-2026-0002', v_admin, '2026-10-12 13:30+07'),
    ('60090000-0000-4000-8000-00000000000d', v_s, '60070000-0000-4000-8000-000000000006', NULL, 'dilaporkan', NULL, v_wali, '2026-10-06 07:40+07'),
    ('60090000-0000-4000-8000-00000000000e', v_s, '60070000-0000-4000-8000-000000000006', 'dilaporkan', 'ditugaskan', NULL, v_admin, '2026-10-06 09:00+07'),
    ('60090000-0000-4000-8000-00000000000f', v_s, '60070000-0000-4000-8000-000000000006', 'ditugaskan', 'menunggu_material', 'Menunggu lampu LED dari PR-2026-0003', v_admin, '2026-10-07 09:00+07'),
    ('60090000-0000-4000-8000-000000000010', v_s, '60070000-0000-4000-8000-000000000007', NULL, 'dilaporkan', NULL, v_guru, '2026-09-22 11:00+07'),
    ('60090000-0000-4000-8000-000000000011', v_s, '60070000-0000-4000-8000-000000000007', 'dilaporkan', 'dibatalkan', 'Kursi sudah diganti dari stok cadangan aula', v_admin, '2026-09-22 15:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO work_order_photos (id, school_id, work_order_id, object_key, mime, size_bytes, sha256, uploaded_by, created_at) VALUES
    ('60080000-0000-4000-8000-000000000001', v_s, '60070000-0000-4000-8000-000000000001', 'ops/work-orders/WO-2026-0001/ac-sebelum.jpg', 'image/jpeg', 284512, 'seed0000000000000000000000000000000000000000000000000000000001', v_guru,  '2026-10-01 08:16+07'),
    ('60080000-0000-4000-8000-000000000002', v_s, '60070000-0000-4000-8000-000000000001', 'ops/work-orders/WO-2026-0001/ac-sesudah.jpg', 'image/jpeg', 301877, 'seed0000000000000000000000000000000000000000000000000000000002', v_admin, '2026-10-03 14:01+07'),
    ('60080000-0000-4000-8000-000000000003', v_s, '60070000-0000-4000-8000-000000000004', 'ops/work-orders/WO-2026-0004/kran-bocor.jpg', 'image/jpeg', 198220, 'seed0000000000000000000000000000000000000000000000000000000003', v_guru,  '2026-10-14 10:21+07'),
    ('60080000-0000-4000-8000-000000000004', v_s, '60070000-0000-4000-8000-000000000006', 'ops/work-orders/WO-2026-0006/lampu-mati.jpg', 'image/jpeg', 176004, 'seed0000000000000000000000000000000000000000000000000000000004', v_wali,  '2026-10-06 07:41+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO work_order_costs (id, school_id, work_order_id, kind, description, amount_idr, created_by, created_at) VALUES
    ('600a0000-0000-4000-8000-000000000001', v_s, '60070000-0000-4000-8000-000000000001', 'material', 'Freon R32 1 kg', 350000, v_admin, '2026-10-03 14:05+07'),
    ('600a0000-0000-4000-8000-000000000002', v_s, '60070000-0000-4000-8000-000000000001', 'jasa', 'Jasa servis AC (teknisi luar)', 200000, v_admin, '2026-10-03 14:06+07'),
    ('600a0000-0000-4000-8000-000000000003', v_s, '60070000-0000-4000-8000-000000000002', 'material', 'Cairan pembersih coil AC', 45000, v_admin, '2026-10-02 10:05+07'),
    ('600a0000-0000-4000-8000-000000000004', v_s, '60070000-0000-4000-8000-000000000003', 'material', 'Lampu proyektor Epson ELPLP96 (DP)', 650000, v_admin, '2026-10-10 08:10+07')
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 2. INVENTARIS: pengaturan, kategori/barang/gudang baru, saldo, mutasi,
  --    stock opname
  ---------------------------------------------------------------------------
  INSERT INTO inventory_settings (school_id, doc_number_format)
  VALUES (v_s, '{"stock_count": "SO-{yyyy}-{seq:4}"}'::jsonb)
  ON CONFLICT DO NOTHING;

  INSERT INTO asset_categories (id, school_id, code, name, useful_life_months, dep_start, asset_acc_code, accum_dep_acc_code, dep_expense_acc_code)
  VALUES (ac_mbl, v_s, 'AST-MEBEL', 'Mebel & Perabot Sekolah', 96, 'next_month', '1306', '1316', '6206')
  ON CONFLICT DO NOTHING;

  INSERT INTO item_categories (id, school_id, code, name, tracking, purchase_acc_code, asset_category_id) VALUES
    (ic_kbr, v_s, 'CAT-KBR', 'Perlengkapan Kebersihan', 'stock', '5103', NULL),
    (ic_lab, v_s, 'CAT-LAB', 'Bahan Habis Pakai Laboratorium', 'stock', '5102', NULL),
    (ic_tik, v_s, 'CAT-TIK', 'Peralatan TIK (dicatat sebagai aset)', 'asset', '1305', v_astcat_elek)
  ON CONFLICT DO NOTHING;

  INSERT INTO items (id, school_id, code, name, category_id, unit, min_stock, status) VALUES
    (it_tinta,  v_s, 'ITM-TINTA-003',   'Tinta Printer Epson 003 Hitam',      v_cat_atk, 'botol',   6,  'active'),
    (it_map,    v_s, 'ITM-MAP-FOLIO',   'Map Plastik Kancing Folio',          v_cat_atk, 'pcs',     50, 'active'),
    (it_sabun,  v_s, 'ITM-SABUN-5L',    'Sabun Cuci Tangan Cair 5 Liter',     ic_kbr,    'jerigen', 4,  'active'),
    (it_tisu,   v_s, 'ITM-TISU-GLG',    'Tisu Gulung isi 10',                 ic_kbr,    'pak',     10, 'active'),
    (it_alko,   v_s, 'ITM-ALKOHOL-70',  'Alkohol 70% 1 Liter',                ic_lab,    'botol',   5,  'active'),
    (it_laptop, v_s, 'ITM-LAPTOP-GURU', 'Laptop Guru ASUS Vivobook 14',       ic_tik,    'unit',    0,  'active'),
    (it_spmrh,  v_s, 'ITM-SPIDOL-RED',  'Spidol Whiteboard Snowman Merah',    v_cat_atk, 'pcs',     12, 'active'),
    (it_kapur,  v_s, 'ITM-KAPUR',       'Kapur Tulis Putih (tidak dipakai)',  v_cat_atk, 'kotak',   0,  'inactive')
  ON CONFLICT DO NOTHING;

  INSERT INTO warehouses (id, school_id, code, name, room_id, status)
  VALUES (wh_lab, v_s, 'WH-LAB', 'Gudang Laboratorium IPA', r_labipa, 'active')
  ON CONFLICT DO NOTHING;

  -- Saldo akhir untuk barang baru (saldo kertas & spidol milik 04_ops.sql tidak diubah;
  -- mutasinya di bawah berakhir tepat di saldo 85 dan 150).
  INSERT INTO stock_balances (school_id, warehouse_id, item_id, qty, updated_at) VALUES
    (v_s, v_wh_main, it_tinta, 20,  '2026-09-03 10:00+07'),
    (v_s, v_wh_main, it_map,   140, '2026-09-30 15:00+07'),
    (v_s, v_wh_main, it_sabun, 3,   '2026-10-05 09:00+07'),
    (v_s, v_wh_main, it_tisu,  13,  '2026-10-01 10:00+07'),
    (v_s, wh_lab,    it_tisu,  5,   '2026-10-01 10:00+07'),
    (v_s, wh_lab,    it_alko,  8,   '2026-10-10 09:00+07')
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 3. PENGADAAN: vendor, kontak, dokumen, rekening, sinkron akuntansi,
  --    PR -> PO -> penerimaan -> retur -> evaluasi
  ---------------------------------------------------------------------------
  INSERT INTO vendors (id, school_id, code, name, address, categories, payment_terms_days, status) VALUES
    (vd_gram, v_s, 'VND-002', 'PT Gramedia Asri Media',        'Jl. Ir. H. Juanda No. 88, Bekasi',        ARRAY['ATK','Buku'], 30, 'active'),
    (vd_komp, v_s, 'VND-003', 'CV Mitra Komputindo',           'Ruko Galaxy Blok C-12, Bekasi Selatan',   ARRAY['Elektronik','TIK','Jasa Jaringan'], 14, 'active'),
    (vd_bers, v_s, 'VND-004', 'UD Sumber Bersih Sentosa',      'Jl. Pekayon Raya No. 21, Bekasi',         ARRAY['Kebersihan','Jasa Kebersihan'], 7, 'active'),
    (vd_labo, v_s, 'VND-005', 'PT Labora Sains Indonesia',     'Jl. Raya Bogor Km 24, Jakarta Timur',     ARRAY['Laboratorium'], 30, 'active'),
    (vd_mebl, v_s, 'VND-006', 'CV Jaya Abadi Furniture',       'Jl. Raya Narogong Km 7, Bekasi',          ARRAY['Mebel','Konstruksi'], 30, 'inactive'),
    (vd_elek, v_s, 'VND-007', 'Toko Cahaya Elektrik',          'Pasar Baru Bekasi Lt. 2 No. 45',          ARRAY['Elektronik'], 0, 'blocked')
  ON CONFLICT DO NOTHING;

  INSERT INTO vendor_contacts (id, school_id, vendor_id, name, phone, email, is_primary) VALUES
    ('60210000-0000-4000-8000-000000000001', v_s, v_vnd1,   'Ibu Ratna Sari',     '0812-1000-2001', 'penjualan@saranaedukasi.example', true),
    ('60210000-0000-4000-8000-000000000002', v_s, vd_gram,  'Bapak Hendra Wijaya','0813-1000-2002', 'korporat@gramedia-asri.example', true),
    ('60210000-0000-4000-8000-000000000003', v_s, vd_gram,  'Sdri. Dewi Lestari', '0813-1000-2003', 'tagihan@gramedia-asri.example', false),
    ('60210000-0000-4000-8000-000000000004', v_s, vd_komp,  'Bapak Agus Pratama', '0815-1000-2004', 'sales@mitrakomputindo.example', true),
    ('60210000-0000-4000-8000-000000000005', v_s, vd_bers,  'Ibu Siti Aminah',    '0817-1000-2005', NULL, true),
    ('60210000-0000-4000-8000-000000000006', v_s, vd_labo,  'Bapak Rudi Hartono', '0811-1000-2006', 'cs@labora-sains.example', true),
    ('60210000-0000-4000-8000-000000000007', v_s, vd_mebl,  'Bapak Joni Saputra', '0819-1000-2007', NULL, true)
  ON CONFLICT DO NOTHING;

  INSERT INTO vendor_documents (id, school_id, vendor_id, kind, object_key, expires_on, uploaded_by, created_at) VALUES
    ('60220000-0000-4000-8000-000000000001', v_s, vd_gram, 'nib',  'ops/vendors/VND-002/nib.pdf',  NULL,         v_admin, '2026-07-20 10:00+07'),
    ('60220000-0000-4000-8000-000000000002', v_s, vd_gram, 'npwp', 'ops/vendors/VND-002/npwp.pdf', NULL,         v_admin, '2026-07-20 10:01+07'),
    ('60220000-0000-4000-8000-000000000003', v_s, vd_komp, 'siup', 'ops/vendors/VND-003/siup.pdf', '2027-03-31', v_admin, '2026-07-21 09:00+07'),
    ('60220000-0000-4000-8000-000000000004', v_s, vd_bers, 'kontrak', 'ops/vendors/VND-004/kontrak-kebersihan-2026.pdf', '2026-12-31', v_admin, '2026-07-01 08:00+07'),
    ('60220000-0000-4000-8000-000000000005', v_s, vd_labo, 'sertifikat_distributor', 'ops/vendors/VND-005/sertifikat.pdf', '2026-11-30', v_admin, '2026-08-01 08:00+07')
  ON CONFLICT DO NOTHING;

  -- Nomor rekening disimpan terenkripsi oleh aplikasi; seed tidak menyimpan nomor asli
  -- (placeholder kosong-semantik, tampil tanpa nomor di UI).
  INSERT INTO vendor_bank_accounts (id, school_id, vendor_id, bank_name, account_no_enc, account_no_bidx, account_holder, status, requested_by, activated_at, created_at) VALUES
    ('60230000-0000-4000-8000-000000000001', v_s, v_vnd1,  'Bank BRI',     '\x00'::bytea, 'seed-demo-60230001', 'CV Sarana Edukasi Nusantara', 'superseded',       v_admin, '2026-01-10 09:00+07', '2026-01-10 09:00+07'),
    ('60230000-0000-4000-8000-000000000002', v_s, v_vnd1,  'Bank Mandiri', '\x00'::bytea, 'seed-demo-60230002', 'CV Sarana Edukasi Nusantara', 'active',           v_admin, '2026-08-01 09:00+07', '2026-07-30 09:00+07'),
    ('60230000-0000-4000-8000-000000000003', v_s, vd_gram, 'Bank BCA',     '\x00'::bytea, 'seed-demo-60230003', 'PT Gramedia Asri Media',      'active',           v_admin, '2026-07-21 09:00+07', '2026-07-20 10:00+07'),
    ('60230000-0000-4000-8000-000000000004', v_s, vd_komp, 'Bank BNI',     '\x00'::bytea, 'seed-demo-60230004', 'CV Mitra Komputindo',         'active',           v_admin, '2026-07-22 09:00+07', '2026-07-21 09:00+07'),
    ('60230000-0000-4000-8000-000000000005', v_s, vd_bers, 'Bank BRI',     '\x00'::bytea, 'seed-demo-60230005', 'Siti Aminah',                 'pending_approval', v_admin, NULL,                  '2026-10-13 09:00+07'),
    ('60230000-0000-4000-8000-000000000006', v_s, vd_labo, 'Bank Syariah Indonesia', '\x00'::bytea, 'seed-demo-60230006', 'PT Labora Sains Indonesia', 'active', v_admin, '2026-08-02 09:00+07', '2026-08-01 09:00+07'),
    ('60230000-0000-4000-8000-000000000007', v_s, vd_mebl, 'Bank BTN',     '\x00'::bytea, 'seed-demo-60230007', 'Joni Saputra (pribadi)',      'rejected',         v_admin, NULL,                  '2026-08-15 09:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO vendor_accounting_sync (vendor_id, school_id, payload_hash, status, attempts, last_error, synced_at, updated_at) VALUES
    (v_vnd1,  v_s, 'seed-60240001', 'synced',  1, NULL, '2026-08-01 09:05+07', '2026-08-01 09:05+07'),
    (vd_gram, v_s, 'seed-60240002', 'synced',  1, NULL, '2026-07-21 09:05+07', '2026-07-21 09:05+07'),
    (vd_komp, v_s, 'seed-60240003', 'synced',  2, NULL, '2026-07-22 09:10+07', '2026-07-22 09:10+07'),
    (vd_bers, v_s, 'seed-60240004', 'pending', 0, NULL, NULL,                  '2026-10-13 09:00+07'),
    (vd_labo, v_s, 'seed-60240005', 'failed',  3, 'Akun utang vendor belum dipetakan di modul akuntansi', NULL, '2026-08-02 10:00+07')
  ON CONFLICT DO NOTHING;

  -- Purchase Request
  INSERT INTO purchase_requests (id, school_id, number, requester_user_id, reason, needed_by, status, total_est_idr, over_budget, budget_check, idempotency_key, created_at, updated_at) VALUES
    ('60240000-0000-4000-8000-000000000001', v_s, 'PR-2026-0001', v_admin,  'Kebutuhan ATK Tata Usaha semester ganjil 2026/2027', '2026-08-15', 'ordered',            2980000, false, '{"status":"ok"}'::jsonb, 'seed-ops-pr-01', '2026-08-01 09:00+07', '2026-08-10 10:00+07'),
    ('60240000-0000-4000-8000-000000000002', v_s, 'PR-2026-0002', v_guru,   'Laptop guru untuk pembelajaran berbasis proyek', '2026-09-10', 'ordered',                  26250000, false, '{"status":"ok"}'::jsonb, 'seed-ops-pr-02', '2026-08-18 10:00+07', '2026-08-28 10:00+07'),
    ('60240000-0000-4000-8000-000000000003', v_s, 'PR-2026-0003', v_admin,  'Lampu LED pengganti Ruang Kelas VII A (WO-2026-0006)', '2026-10-20', 'pending_approval',  900000, false, '{"status":"ok"}'::jsonb, 'seed-ops-pr-03', '2026-10-07 09:30+07', '2026-10-07 09:30+07'),
    ('60240000-0000-4000-8000-000000000004', v_s, 'PR-2026-0004', v_guru,   'Bahan praktikum IPA semester ganjil', '2026-10-05', 'partially_ordered',                       1006000, false, '{"status":"ok"}'::jsonb, 'seed-ops-pr-04', '2026-09-20 08:00+07', '2026-09-28 10:00+07'),
    ('60240000-0000-4000-8000-000000000005', v_s, 'PR-2026-0005', v_wali,   'Perlengkapan kebersihan kelas VII', '2026-11-01', 'draft',                                     1360000, false, NULL, 'seed-ops-pr-05', '2026-10-14 14:00+07', '2026-10-14 14:00+07'),
    ('60240000-0000-4000-8000-000000000006', v_s, 'PR-2026-0006', v_guru,   'Proyektor tambahan untuk Ruang Rapat Guru', '2026-09-30', 'rejected',                          7200000, true,  '{"status":"over_budget","line":"5.2.01"}'::jsonb, 'seed-ops-pr-06', '2026-09-02 10:00+07', '2026-09-04 15:00+07'),
    ('60240000-0000-4000-8000-000000000007', v_s, 'PR-2026-0007', v_admin,  'Spidol whiteboard merah untuk ujian tengah semester', '2026-09-15', 'cancelled',              432000, false, '{"status":"ok"}'::jsonb, 'seed-ops-pr-07', '2026-09-01 08:00+07', '2026-09-03 08:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO purchase_request_lines (id, school_id, pr_id, item_id, description, qty, unit, est_unit_price_idr, budget_line_key, qty_ordered) VALUES
    ('60250000-0000-4000-8000-000000000001', v_s, '60240000-0000-4000-8000-000000000001', it_tinta,  'Tinta Printer Epson 003 Hitam', 24, 'botol', 95000, bl_atk, 24),
    ('60250000-0000-4000-8000-000000000002', v_s, '60240000-0000-4000-8000-000000000001', it_map,    'Map Plastik Kancing Folio', 200, 'pcs', 3500, bl_atk, 200),
    ('60250000-0000-4000-8000-000000000003', v_s, '60240000-0000-4000-8000-000000000002', it_laptop, 'Laptop Guru ASUS Vivobook 14 (Core i5, 16 GB)', 3, 'unit', 8750000, bl_tik, 3),
    ('60250000-0000-4000-8000-000000000004', v_s, '60240000-0000-4000-8000-000000000003', NULL,      'Lampu LED Philips 18W T8', 20, 'pcs', 45000, bl_pml, 0),
    ('60250000-0000-4000-8000-000000000005', v_s, '60240000-0000-4000-8000-000000000004', it_alko,   'Alkohol 70% 1 Liter', 12, 'botol', 38000, bl_lab, 12),
    ('60250000-0000-4000-8000-000000000006', v_s, '60240000-0000-4000-8000-000000000004', NULL,      'Kaca preparat & kaca penutup (box)', 10, 'box', 55000, bl_lab, 0),
    ('60250000-0000-4000-8000-000000000007', v_s, '60240000-0000-4000-8000-000000000005', it_sabun,  'Sabun Cuci Tangan Cair 5 Liter', 6, 'jerigen', 120000, bl_kbr, 0),
    ('60250000-0000-4000-8000-000000000008', v_s, '60240000-0000-4000-8000-000000000005', it_tisu,   'Tisu Gulung isi 10', 20, 'pak', 32000, bl_kbr, 0),
    ('60250000-0000-4000-8000-000000000009', v_s, '60240000-0000-4000-8000-000000000006', NULL,      'Proyektor Epson EB-E01', 1, 'unit', 7200000, bl_tik, 0),
    ('60250000-0000-4000-8000-00000000000a', v_s, '60240000-0000-4000-8000-000000000007', it_spmrh,  'Spidol Whiteboard Snowman Merah', 48, 'pcs', 9000, bl_atk, 0)
  ON CONFLICT DO NOTHING;

  -- Purchase Order
  INSERT INTO purchase_orders (id, school_id, number, vendor_id, payment_terms_days, expected_date, status, total_idr, approval_id, pdf_object_key, sent_at, closed_reason, idempotency_key, created_at, updated_at) VALUES
    ('60260000-0000-4000-8000-000000000001', v_s, 'PO-2026-0001', vd_gram, 30, '2026-08-20', 'received',           2888000,  NULL,       'ops/po/PO-2026-0001.pdf', '2026-08-10 10:00+07', NULL, 'seed-ops-po-01', '2026-08-08 09:00+07', '2026-08-19 11:00+07'),
    ('60260000-0000-4000-8000-000000000002', v_s, 'PO-2026-0002', vd_komp, 14, '2026-09-05', 'received',           25800000, NULL,       'ops/po/PO-2026-0002.pdf', '2026-08-28 10:00+07', NULL, 'seed-ops-po-02', '2026-08-26 09:00+07', '2026-09-10 13:00+07'),
    ('60260000-0000-4000-8000-000000000003', v_s, 'PO-2026-0003', vd_labo, 30, '2026-10-10', 'partially_received', 610000,   NULL,       'ops/po/PO-2026-0003.pdf', '2026-09-28 10:00+07', NULL, 'seed-ops-po-03', '2026-09-26 09:00+07', '2026-10-07 10:00+07'),
    ('60260000-0000-4000-8000-000000000004', v_s, 'PO-2026-0004', v_vnd1,  30, '2026-10-30', 'pending_approval',   4500000,  v_core_apr, NULL, NULL, NULL, 'seed-ops-po-04', '2026-10-09 09:00+07', '2026-10-09 09:05+07'),
    ('60260000-0000-4000-8000-000000000005', v_s, 'PO-2026-0005', vd_bers, 7,  '2026-10-25', 'draft',              690000,   NULL,       NULL, NULL, NULL, 'seed-ops-po-05', '2026-10-14 15:00+07', '2026-10-14 15:00+07'),
    ('60260000-0000-4000-8000-000000000006', v_s, 'PO-2026-0006', vd_gram, 30, '2026-10-25', 'sent',               2400000,  NULL,       'ops/po/PO-2026-0006.pdf', '2026-10-12 10:00+07', NULL, 'seed-ops-po-06', '2026-10-10 09:00+07', '2026-10-12 10:00+07'),
    ('60260000-0000-4000-8000-000000000007', v_s, 'PO-2026-0007', vd_mebl, 30, '2026-09-15', 'cancelled',          14000000, NULL,       NULL, NULL, 'Vendor dinonaktifkan sebelum PO disetujui', 'seed-ops-po-07', '2026-08-20 09:00+07', '2026-08-25 09:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO purchase_order_lines (id, school_id, po_id, pr_line_id, item_id, description, qty, unit_price_idr, budget_line_key, withholding_type_id, warehouse_id) VALUES
    ('60270000-0000-4000-8000-000000000001', v_s, '60260000-0000-4000-8000-000000000001', '60250000-0000-4000-8000-000000000001', it_tinta,  'Tinta Printer Epson 003 Hitam', 24, 92000, bl_atk, NULL, v_wh_main),
    ('60270000-0000-4000-8000-000000000002', v_s, '60260000-0000-4000-8000-000000000001', '60250000-0000-4000-8000-000000000002', it_map,    'Map Plastik Kancing Folio', 200, 3400, bl_atk, NULL, v_wh_main),
    ('60270000-0000-4000-8000-000000000003', v_s, '60260000-0000-4000-8000-000000000002', '60250000-0000-4000-8000-000000000003', it_laptop, 'Laptop Guru ASUS Vivobook 14 (Core i5, 16 GB)', 3, 8600000, bl_tik, NULL, NULL),
    ('60270000-0000-4000-8000-000000000004', v_s, '60260000-0000-4000-8000-000000000003', '60250000-0000-4000-8000-000000000005', it_alko,   'Alkohol 70% 1 Liter', 12, 37500, bl_lab, NULL, wh_lab),
    ('60270000-0000-4000-8000-000000000005', v_s, '60260000-0000-4000-8000-000000000003', NULL,                                   NULL,      'Masker medis 3 ply (box isi 50)', 4, 40000, bl_lab, NULL, wh_lab),
    ('60270000-0000-4000-8000-000000000006', v_s, '60260000-0000-4000-8000-000000000004', NULL,                                   NULL,      'Alat peraga geometri bangun ruang (set)', 15, 120000, bl_lab, NULL, NULL),
    ('60270000-0000-4000-8000-000000000007', v_s, '60260000-0000-4000-8000-000000000004', NULL,                                   NULL,      'Mikroskop monokuler siswa', 15, 180000, bl_lab, NULL, NULL),
    ('60270000-0000-4000-8000-000000000008', v_s, '60260000-0000-4000-8000-000000000005', NULL,                                   it_sabun,  'Sabun Cuci Tangan Cair 5 Liter', 6, 115000, bl_kbr, NULL, v_wh_main),
    ('60270000-0000-4000-8000-000000000009', v_s, '60260000-0000-4000-8000-000000000006', NULL,                                   v_item_kertas, 'Kertas HVS A4 80gr Sinar Dunia', 50, 48000, bl_atk, NULL, v_wh_main),
    ('60270000-0000-4000-8000-00000000000a', v_s, '60260000-0000-4000-8000-000000000007', NULL,                                   NULL,      'Kursi siswa kayu jati', 40, 350000, bl_tik, NULL, NULL)
  ON CONFLICT DO NOTHING;

  -- Penerimaan barang (GR)
  INSERT INTO goods_receipts (id, school_id, number, po_id, received_on, received_by, note, idempotency_key, created_at) VALUES
    ('60280000-0000-4000-8000-000000000001', v_s, 'GR-2026-0001', '60260000-0000-4000-8000-000000000001', '2026-08-19', v_admin, 'Diterima lengkap di gudang utama', 'seed-ops-gr-01', '2026-08-19 11:00+07'),
    ('60280000-0000-4000-8000-000000000002', v_s, 'GR-2026-0002', '60260000-0000-4000-8000-000000000002', '2026-09-10', v_admin, 'Tiga unit laptop diterima, diteruskan ke register aset', 'seed-ops-gr-02', '2026-09-10 13:00+07'),
    ('60280000-0000-4000-8000-000000000003', v_s, 'GR-2026-0003', '60260000-0000-4000-8000-000000000003', '2026-10-07', v_admin, 'Masker belum dikirim vendor', 'seed-ops-gr-03', '2026-10-07 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO goods_receipt_lines (id, school_id, gr_id, po_line_id, qty_received, qty_accepted, qty_rejected, reject_reason, asset_handoff_at, created_at) VALUES
    ('60290000-0000-4000-8000-000000000001', v_s, '60280000-0000-4000-8000-000000000001', '60270000-0000-4000-8000-000000000001', 24, 24, 0, NULL, NULL, '2026-08-19 11:00+07'),
    ('60290000-0000-4000-8000-000000000002', v_s, '60280000-0000-4000-8000-000000000001', '60270000-0000-4000-8000-000000000002', 200, 200, 0, NULL, NULL, '2026-08-19 11:00+07'),
    ('60290000-0000-4000-8000-000000000003', v_s, '60280000-0000-4000-8000-000000000002', '60270000-0000-4000-8000-000000000003', 3, 3, 0, NULL, '2026-09-10 13:00+07', '2026-09-10 13:00+07'),
    ('60290000-0000-4000-8000-000000000004', v_s, '60280000-0000-4000-8000-000000000003', '60270000-0000-4000-8000-000000000004', 12, 10, 2, 'Dua botol pecah saat pengiriman', NULL, '2026-10-07 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO goods_returns (id, school_id, gr_line_id, qty, reason, returned_on, created_by, created_at) VALUES
    ('602a0000-0000-4000-8000-000000000001', v_s, '60290000-0000-4000-8000-000000000002', 10, 'Warna map tidak sesuai pesanan (merah, seharusnya biru)', '2026-08-22', v_admin, '2026-08-22 09:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO vendor_evaluations (id, school_id, vendor_id, goods_receipt_id, on_time, quality_score, price_score, note, created_by, created_at) VALUES
    ('602b0000-0000-4000-8000-000000000001', v_s, vd_gram, '60280000-0000-4000-8000-000000000001', true,  4, 4, 'Tepat waktu, 10 map salah warna diretur', v_admin, '2026-08-22 10:00+07'),
    ('602b0000-0000-4000-8000-000000000002', v_s, vd_komp, '60280000-0000-4000-8000-000000000002', false, 5, 4, 'Terlambat 5 hari, kualitas unit baik', v_admin, '2026-09-11 09:00+07'),
    ('602b0000-0000-4000-8000-000000000003', v_s, vd_labo, '60280000-0000-4000-8000-000000000003', true,  2, 5, 'Kemasan kurang aman, 2 botol pecah', v_admin, '2026-10-07 11:00+07')
  ON CONFLICT DO NOTHING;

  -- Mutasi stok (kartu stok). Saldo akhir sama dengan stock_balances.
  INSERT INTO stock_movements (id, school_id, warehouse_id, item_id, direction, qty, kind, ref_type, ref_id, ref_no, transfer_id, balance_after, note, created_by, created_at, idempotency_key) VALUES
    ('60130000-0000-4000-8000-000000000001', v_s, v_wh_main, v_item_kertas, 'in',  100, 'adjustment', 'manual', v_s, 'SALDO-AWAL-2026', NULL, 100, 'Saldo awal tahun ajaran 2026/2027', v_admin, '2026-07-13 08:00+07', 'seed-ops-sm-01'),
    ('60130000-0000-4000-8000-000000000002', v_s, v_wh_main, v_item_kertas, 'out', 15,  'issue', 'manual', v_s, 'BPB-2026-0912', NULL, 85, 'Pemakaian Tata Usaha untuk naskah ujian', v_admin, '2026-09-12 09:00+07', 'seed-ops-sm-02'),
    ('60130000-0000-4000-8000-000000000003', v_s, v_wh_main, v_item_spidol, 'in',  200, 'adjustment', 'manual', v_s, 'SALDO-AWAL-2026', NULL, 200, 'Saldo awal tahun ajaran 2026/2027', v_admin, '2026-07-13 08:00+07', 'seed-ops-sm-03'),
    ('60130000-0000-4000-8000-000000000004', v_s, v_wh_main, v_item_spidol, 'out', 50,  'issue', 'manual', v_s, 'BPB-2026-0715', NULL, 150, 'Distribusi ke wali kelas', v_admin, '2026-07-15 10:00+07', 'seed-ops-sm-04'),
    ('60130000-0000-4000-8000-000000000005', v_s, v_wh_main, it_tinta, 'in',  24,  'receipt', 'goods_receipt', '60280000-0000-4000-8000-000000000001', 'GR-2026-0001', NULL, 24, NULL, v_admin, '2026-08-19 11:00+07', 'seed-ops-sm-05'),
    ('60130000-0000-4000-8000-000000000006', v_s, v_wh_main, it_map,   'in',  200, 'receipt', 'goods_receipt', '60280000-0000-4000-8000-000000000001', 'GR-2026-0001', NULL, 200, NULL, v_admin, '2026-08-19 11:00+07', 'seed-ops-sm-06'),
    ('60130000-0000-4000-8000-000000000007', v_s, v_wh_main, it_map,   'out', 10,  'return_to_vendor', 'goods_return', '602a0000-0000-4000-8000-000000000001', 'RTR-GR-2026-0001', NULL, 190, 'Retur warna tidak sesuai', v_admin, '2026-08-22 09:00+07', 'seed-ops-sm-07'),
    ('60130000-0000-4000-8000-000000000008', v_s, v_wh_main, it_tinta, 'out', 4,   'issue', 'manual', v_s, 'BPB-2026-0903', NULL, 20, 'Isi ulang printer ruang guru', v_admin, '2026-09-03 10:00+07', 'seed-ops-sm-08'),
    ('60130000-0000-4000-8000-000000000009', v_s, v_wh_main, it_map,   'out', 48,  'issue', 'manual', v_s, 'BPB-2026-0905', NULL, 142, 'Map berkas rapor siswa', v_admin, '2026-09-05 10:00+07', 'seed-ops-sm-09'),
    ('60130000-0000-4000-8000-00000000000a', v_s, v_wh_main, it_map,   'out', 2,   'adjustment', 'stock_count', '60140000-0000-4000-8000-000000000001', 'SO-2026-0001', NULL, 140, 'Selisih stock opname', v_admin, '2026-09-30 15:00+07', 'seed-ops-sm-10'),
    ('60130000-0000-4000-8000-00000000000b', v_s, v_wh_main, it_sabun, 'in',  10,  'adjustment', 'manual', v_s, 'SALDO-AWAL-2026', NULL, 10, 'Saldo awal tahun ajaran 2026/2027', v_admin, '2026-07-13 08:00+07', 'seed-ops-sm-11'),
    ('60130000-0000-4000-8000-00000000000c', v_s, v_wh_main, it_sabun, 'out', 7,   'issue', 'manual', v_s, 'BPB-2026-1005', NULL, 3, 'Distribusi ke toilet siswa', v_admin, '2026-10-05 09:00+07', 'seed-ops-sm-12'),
    ('60130000-0000-4000-8000-00000000000d', v_s, v_wh_main, it_tisu,  'in',  30,  'adjustment', 'manual', v_s, 'SALDO-AWAL-2026', NULL, 30, 'Saldo awal tahun ajaran 2026/2027', v_admin, '2026-07-13 08:00+07', 'seed-ops-sm-13'),
    ('60130000-0000-4000-8000-00000000000e', v_s, v_wh_main, it_tisu,  'out', 12,  'issue', 'manual', v_s, 'BPB-2026-0920', NULL, 18, 'Distribusi ke kelas', v_admin, '2026-09-20 09:00+07', 'seed-ops-sm-14'),
    ('60130000-0000-4000-8000-00000000000f', v_s, v_wh_main, it_tisu,  'out', 5,   'transfer_out', 'transfer', '60130000-0000-4000-8000-0000000000ff', 'TRF-2026-1001', '60130000-0000-4000-8000-0000000000ff', 13, 'Transfer ke Gudang Lab IPA', v_admin, '2026-10-01 10:00+07', 'seed-ops-sm-15'),
    ('60130000-0000-4000-8000-000000000010', v_s, wh_lab,    it_tisu,  'in',  5,   'transfer_in', 'transfer', '60130000-0000-4000-8000-0000000000ff', 'TRF-2026-1001', '60130000-0000-4000-8000-0000000000ff', 5, 'Transfer dari Gudang Utama', v_admin, '2026-10-01 10:00+07', 'seed-ops-sm-16'),
    ('60130000-0000-4000-8000-000000000011', v_s, wh_lab,    it_alko,  'in',  10,  'receipt', 'goods_receipt', '60280000-0000-4000-8000-000000000003', 'GR-2026-0003', NULL, 10, NULL, v_admin, '2026-10-07 10:00+07', 'seed-ops-sm-17'),
    ('60130000-0000-4000-8000-000000000012', v_s, wh_lab,    it_alko,  'out', 2,   'issue', 'manual', v_s, 'BPB-2026-1010', NULL, 8, 'Praktikum IPA VII A', v_guru, '2026-10-10 09:00+07', 'seed-ops-sm-18')
  ON CONFLICT DO NOTHING;

  -- Stock opname: satu selesai disesuaikan (WH-MAIN), satu masih berjalan (WH-LAB)
  INSERT INTO stock_counts (id, school_id, warehouse_id, number, status, started_at, started_by, submitted_at, report_object_key, created_at, updated_at) VALUES
    ('60140000-0000-4000-8000-000000000001', v_s, v_wh_main, 'SO-2026-0001', 'adjusted', '2026-09-29 08:00+07', v_admin, '2026-09-30 11:00+07', 'ops/stock-counts/SO-2026-0001.pdf', '2026-09-29 08:00+07', '2026-09-30 15:00+07'),
    ('60140000-0000-4000-8000-000000000002', v_s, wh_lab,    'SO-2026-0002', 'counting', '2026-10-14 13:00+07', v_admin, NULL, NULL, '2026-10-14 13:00+07', '2026-10-14 13:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO stock_count_lines (id, school_id, count_id, item_id, system_qty, counted_qty, note, created_at)
  SELECT v.id::uuid, v_s, v.count_id::uuid, v.item_id, v.system_qty, v.counted_qty, v.note, v.created_at::timestamptz
  FROM (VALUES
    ('60150000-0000-4000-8000-000000000001', '60140000-0000-4000-8000-000000000001', v_item_kertas, 85::bigint, 85::bigint, NULL::text, '2026-09-29 08:00+07'),
    ('60150000-0000-4000-8000-000000000002', '60140000-0000-4000-8000-000000000001', v_item_spidol, 150, 150, NULL, '2026-09-29 08:00+07'),
    ('60150000-0000-4000-8000-000000000003', '60140000-0000-4000-8000-000000000001', it_tinta, 20, 20, NULL, '2026-09-29 08:00+07'),
    ('60150000-0000-4000-8000-000000000004', '60140000-0000-4000-8000-000000000001', it_map, 142, 140, 'Dua map rusak terkena air', '2026-09-29 08:00+07'),
    ('60150000-0000-4000-8000-000000000005', '60140000-0000-4000-8000-000000000002', it_alko, 8, NULL, NULL, '2026-10-14 13:00+07'),
    ('60150000-0000-4000-8000-000000000006', '60140000-0000-4000-8000-000000000002', it_tisu, 5, 5, NULL, '2026-10-14 13:00+07')
  ) AS v(id, count_id, item_id, system_qty, counted_qty, note, created_at)
  WHERE EXISTS (SELECT 1 FROM stock_counts c WHERE c.id = v.count_id::uuid)
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 4. ASET TETAP: pengaturan, aset baru (termasuk kandidat dari GR),
  --    mutasi, pelepasan, nilai buku, penyusutan, event akuntansi
  ---------------------------------------------------------------------------
  INSERT INTO asset_settings (school_id, code_format, interunit_acc_code, disposal_gain_acc_code, disposal_loss_acc_code, proceeds_acc_code)
  VALUES (v_s, '{cat}-{yyyy}-{seq:4}', '1990', '7201', '8201', '1101')
  ON CONFLICT DO NOTHING;

  INSERT INTO assets (id, school_id, code, name, category_id, status, condition, acquisition_cost_idr, acquired_on, residual_idr,
                      room_id, custodian_user_id, serial_no, qr_token, source, gr_line_id, unit_seq, po_id, vendor_id,
                      activated_at, activated_by, created_at, updated_at) VALUES
    ('60310000-0000-4000-8000-000000000001', v_s, 'AST-LTP-002', 'Laptop Guru ASUS Vivobook 14 #1', v_astcat_elek, 'active', 'baik', 8600000, '2026-09-10', 600000,
     r_rapat, v_guru, 'ASUS-VB14-N8A1001', 'QR-TPGS-6031-0001', 'goods_receipt', '60290000-0000-4000-8000-000000000003', 1, '60260000-0000-4000-8000-000000000002', vd_komp,
     '2026-09-12 09:00+07', v_admin, '2026-09-10 13:00+07', '2026-09-12 09:00+07'),
    ('60310000-0000-4000-8000-000000000002', v_s, 'AST-LTP-003', 'Laptop Guru ASUS Vivobook 14 #2', v_astcat_elek, 'active', 'baik', 8600000, '2026-09-10', 600000,
     r_kls7a, v_wali, 'ASUS-VB14-N8A1002', 'QR-TPGS-6031-0002', 'goods_receipt', '60290000-0000-4000-8000-000000000003', 2, '60260000-0000-4000-8000-000000000002', vd_komp,
     '2026-09-12 09:05+07', v_admin, '2026-09-10 13:00+07', '2026-09-12 09:05+07'),
    ('60310000-0000-4000-8000-000000000003', v_s, 'GR-GR-2026-0002-3', 'Laptop Guru ASUS Vivobook 14 #3', v_astcat_elek, 'candidate', NULL, 8600000, '2026-09-10', 0,
     NULL, NULL, NULL, 'QR-TPGS-6031-0003', 'goods_receipt', '60290000-0000-4000-8000-000000000003', 3, '60260000-0000-4000-8000-000000000002', vd_komp,
     NULL, NULL, '2026-09-10 13:00+07', '2026-09-10 13:00+07'),
    ('60310000-0000-4000-8000-000000000004', v_s, 'AST-MBL-001', 'Meja Guru Kayu Jati', ac_mbl, 'active', 'baik', 2750000, '2025-07-20', 250000,
     r_kls7a, NULL, NULL, 'QR-TPGS-6031-0004', 'manual', NULL, NULL, NULL, NULL,
     '2025-07-20 10:00+07', v_admin, '2025-07-20 10:00+07', '2026-07-20 10:00+07'),
    ('60310000-0000-4000-8000-000000000005', v_s, 'AST-MBL-002', 'Lemari Arsip Besi 4 Pintu', ac_mbl, 'active', 'rusak_ringan', 3200000, '2024-01-10', 200000,
     v_room_gudang, NULL, NULL, 'QR-TPGS-6031-0005', 'manual', NULL, NULL, NULL, NULL,
     '2024-01-10 10:00+07', v_admin, '2024-01-10 10:00+07', '2026-09-28 10:00+07'),
    ('60310000-0000-4000-8000-000000000006', v_s, 'AST-PRJ-000', 'Proyektor BenQ MS560 (lama)', v_astcat_elek, 'disposed', 'rusak_berat', 6800000, '2021-08-01', 400000,
     v_room_aula, NULL, 'BENQ-MS560-77812', 'QR-TPGS-6031-0006', 'manual', NULL, NULL, NULL, NULL,
     '2021-08-01 10:00+07', v_admin, '2021-08-01 10:00+07', '2026-09-20 10:00+07'),
    ('60310000-0000-4000-8000-000000000007', v_s, 'AST-PC-001', 'PC Desktop Lab Komputer (unit 07)', v_astcat_elek, 'active', 'rusak_berat', 7500000, '2020-07-15', 500000,
     v_room_labkom, NULL, 'LNV-M70-PC07', 'QR-TPGS-6031-0007', 'manual', NULL, NULL, NULL, NULL,
     '2020-07-15 10:00+07', v_admin, '2020-07-15 10:00+07', '2026-10-01 10:00+07'),
    ('60310000-0000-4000-8000-000000000008', v_s, 'AST-KMR-001', 'Kamera DSLR Canon EOS 200D', v_astcat_elek, 'disposed', 'baik', 9200000, '2022-02-14', 700000,
     v_room_gudang, NULL, 'CN200D-55120', 'QR-TPGS-6031-0008', 'manual', NULL, NULL, NULL, NULL,
     '2022-02-14 10:00+07', v_admin, '2022-02-14 10:00+07', '2026-08-30 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO asset_movements (id, school_id, asset_id, kind, from_value, to_value, effective_date, reason, status, created_by, created_at) VALUES
    ('60320000-0000-4000-8000-000000000001', v_s, '60310000-0000-4000-8000-000000000004', 'location',
     jsonb_build_object('room_id', r_rapat::text), jsonb_build_object('room_id', r_kls7a::text), '2026-07-20', 'Penataan ruang kelas baru TA 2026/2027', 'done', v_admin, '2026-07-20 10:00+07'),
    ('60320000-0000-4000-8000-000000000002', v_s, '60310000-0000-4000-8000-000000000001', 'custodian',
     jsonb_build_object('custodian_user_id', v_admin::text), jsonb_build_object('custodian_user_id', v_guru::text), '2026-09-12', 'Serah terima laptop ke guru pengampu', 'done', v_admin, '2026-09-12 09:00+07'),
    ('60320000-0000-4000-8000-000000000003', v_s, '60310000-0000-4000-8000-000000000005', 'condition',
     jsonb_build_object('condition', 'baik'), jsonb_build_object('condition', 'rusak_ringan'), '2026-09-28', 'Engsel pintu lemari longgar', 'done', v_admin, '2026-09-28 10:00+07'),
    ('60320000-0000-4000-8000-000000000004', v_s, '60310000-0000-4000-8000-000000000002', 'custodian',
     jsonb_build_object('custodian_user_id', v_wali::text), jsonb_build_object('custodian_user_id', v_kepsek::text), '2026-10-20', 'Laptop dipinjam kepala sekolah untuk akreditasi', 'pending_approval', v_admin, '2026-10-14 09:00+07'),
    ('60320000-0000-4000-8000-000000000005', v_s, '60310000-0000-4000-8000-000000000007', 'location',
     jsonb_build_object('room_id', v_room_labkom::text), jsonb_build_object('room_id', v_room_gudang::text), '2026-10-01', 'Unit rusak dipindah ke gudang', 'rejected', v_admin, '2026-10-01 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO asset_disposals (id, school_id, asset_id, kind, disposed_on, proceeds_idr, reason, status, created_by, created_at) VALUES
    ('60330000-0000-4000-8000-000000000001', v_s, '60310000-0000-4000-8000-000000000006', 'dijual', '2026-09-20', 750000, 'Lampu dan mainboard rusak, biaya servis melebihi nilai buku', 'approved', v_admin, '2026-09-15 10:00+07'),
    ('60330000-0000-4000-8000-000000000002', v_s, '60310000-0000-4000-8000-000000000008', 'hilang', '2026-08-30', 0, 'Hilang saat kegiatan study tour, berita acara kepolisian terlampir', 'approved', v_admin, '2026-08-25 10:00+07'),
    ('60330000-0000-4000-8000-000000000003', v_s, '60310000-0000-4000-8000-000000000007', 'rusak', '2026-10-31', 0, 'Mainboard terbakar, tidak ekonomis diperbaiki', 'pending_approval', v_admin, '2026-10-13 10:00+07'),
    ('60330000-0000-4000-8000-000000000004', v_s, '60310000-0000-4000-8000-000000000005', 'rusak', '2026-10-05', 0, 'Lemari berkarat', 'rejected', v_admin, '2026-10-02 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO asset_accounting_events (id, school_id, asset_id, kind, source_id, reference, payload, status, attempts, error, trans_nos, posted_at, created_at, updated_at) VALUES
    ('60340000-0000-4000-8000-000000000001', v_s, '60310000-0000-4000-8000-000000000001', 'acquisition', '60290000-0000-4000-8000-000000000003', 'seed-asset-acq-AST-LTP-002',
     '{"amount_idr": 8600000, "debit": "1305", "credit": "2101"}'::jsonb, 'posted', 1, NULL, ARRAY['JV-2026-09-0041'], '2026-09-12 09:10+07', '2026-09-12 09:00+07', '2026-09-12 09:10+07'),
    ('60340000-0000-4000-8000-000000000002', v_s, '60310000-0000-4000-8000-000000000002', 'acquisition', '60290000-0000-4000-8000-000000000003', 'seed-asset-acq-AST-LTP-003',
     '{"amount_idr": 8600000, "debit": "1305", "credit": "2101"}'::jsonb, 'posted', 1, NULL, ARRAY['JV-2026-09-0042'], '2026-09-12 09:15+07', '2026-09-12 09:05+07', '2026-09-12 09:15+07'),
    ('60340000-0000-4000-8000-000000000003', v_s, '60310000-0000-4000-8000-000000000006', 'disposal', '60330000-0000-4000-8000-000000000001', 'seed-asset-disp-AST-PRJ-000',
     '{"proceeds_idr": 750000, "book_value_idr": 400000, "gain_idr": 350000}'::jsonb, 'posted', 1, NULL, ARRAY['JV-2026-09-0063'], '2026-09-20 11:00+07', '2026-09-20 10:30+07', '2026-09-20 11:00+07'),
    ('60340000-0000-4000-8000-000000000004', v_s, '60310000-0000-4000-8000-000000000008', 'disposal', '60330000-0000-4000-8000-000000000002', 'seed-asset-disp-AST-KMR-001',
     '{"proceeds_idr": 0, "book_value_idr": 4166670, "loss_idr": 4166670}'::jsonb, 'failed', 3, 'Akun rugi pelepasan aset belum dipetakan', NULL, NULL, '2026-08-30 10:30+07', '2026-08-30 12:00+07'),
    ('60340000-0000-4000-8000-000000000005', v_s, '60310000-0000-4000-8000-000000000004', 'acquisition', NULL, 'seed-asset-acq-AST-MBL-001',
     '{"amount_idr": 2750000, "debit": "1306", "credit": "1101"}'::jsonb, 'pending', 0, NULL, NULL, NULL, '2026-10-14 09:00+07', '2026-10-14 09:00+07')
  ON CONFLICT DO NOTHING;

  -- Nilai buku s.d. periode 2026-09 (garis lurus: (harga - residu) / umur, dibulatkan ke bawah per bulan)
  INSERT INTO asset_book_values (asset_id, school_id, cost_idr, accumulated_idr, book_value_idr, last_period, as_of) VALUES
    (v_ast_ltp1,                               v_s, 12500000, 2156247, 10343753, '2026-09', '2026-10-01 01:00+07'),
    (v_ast_prj1,                               v_s, 8500000,  1333328, 7166672,  '2026-09', '2026-10-01 01:00+07'),
    ('60310000-0000-4000-8000-000000000001',   v_s, 8600000,  166666,  8433334,  '2026-09', '2026-10-01 01:00+07'),
    ('60310000-0000-4000-8000-000000000002',   v_s, 8600000,  166666,  8433334,  '2026-09', '2026-10-01 01:00+07'),
    ('60310000-0000-4000-8000-000000000004',   v_s, 2750000,  364574,  2385426,  '2026-09', '2026-10-01 01:00+07'),
    ('60310000-0000-4000-8000-000000000005',   v_s, 3200000,  1000000, 2200000,  '2026-09', '2026-10-01 01:00+07'),
    ('60310000-0000-4000-8000-000000000007',   v_s, 7500000,  7000000, 500000,   '2026-09', '2026-10-01 01:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO asset_depreciation_runs (school_id, period, status, error, created_at) VALUES
    (v_s, '2026-01', 'done', NULL, '2026-02-01 01:00+07'),
    (v_s, '2026-02', 'done', NULL, '2026-03-01 01:00+07'),
    (v_s, '2026-03', 'done', NULL, '2026-04-01 01:00+07'),
    (v_s, '2026-04', 'done', NULL, '2026-05-01 01:00+07'),
    (v_s, '2026-05', 'done', NULL, '2026-06-01 01:00+07'),
    (v_s, '2026-06', 'done', NULL, '2026-07-01 01:00+07'),
    (v_s, '2026-07', 'done', NULL, '2026-08-01 01:00+07'),
    (v_s, '2026-08', 'done', NULL, '2026-09-01 01:00+07'),
    (v_s, '2026-09', 'done', NULL, '2026-10-01 01:00+07')
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 5. ANGGARAN / RAPBS
  ---------------------------------------------------------------------------
  INSERT INTO budget_settings (school_id, fiscal_year_start_month, lock_rule, tolerance_bp, warn_threshold_bp, updated_at)
  VALUES (v_s, 7, 'warn', 500, 8000, '2026-07-01 08:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO budgets (id, school_id, code, name, fiscal_year, period_start, period_end, status, total_planned_idr, created_by, created_at, updated_at) VALUES
    ('60400000-0000-4000-8000-000000000001', v_s, 'RAPBS-2026-2027',   'RAPBS Tahun Ajaran 2026/2027',               '2026/2027', '2026-07-01', '2027-06-30', 'approved',         191000000, v_admin, '2026-05-15 09:00+07', '2026-06-20 10:00+07'),
    ('60400000-0000-4000-8000-000000000002', v_s, 'RAPBS-2025-2026',   'RAPBS Tahun Ajaran 2025/2026',               '2025/2026', '2025-07-01', '2026-06-30', 'closed',           168000000, v_admin, '2025-05-15 09:00+07', '2026-07-05 10:00+07'),
    ('60400000-0000-4000-8000-000000000003', v_s, 'RAPBS-2026-2027-P', 'Perubahan RAPBS 2026/2027 (Semester Genap)', '2026/2027', '2027-01-01', '2027-06-30', 'draft',            30000000,  v_admin, '2026-10-10 09:00+07', '2026-10-10 09:00+07'),
    ('60400000-0000-4000-8000-000000000004', v_s, 'ANG-EKSKUL-2026',   'Anggaran Kegiatan Ekstrakurikuler 2026/2027', '2026/2027', '2026-07-01', '2027-06-30', 'pending_approval', 22500000,  v_admin, '2026-10-01 09:00+07', '2026-10-02 09:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO budget_lines (id, school_id, budget_id, code, name, category, acc_code, div_code, amount_planned_idr) VALUES
    (bl_atk, v_s, '60400000-0000-4000-8000-000000000001', '5.1.01', 'ATK & Bahan Habis Pakai Kantor',     'operasional',  '5101', 'TU',     45000000),
    (bl_lab, v_s, '60400000-0000-4000-8000-000000000001', '5.1.02', 'Bahan Praktikum Laboratorium',       'operasional',  '5102', 'KUR',    18000000),
    (bl_tik, v_s, '60400000-0000-4000-8000-000000000001', '5.2.01', 'Pengadaan Peralatan TIK & Mebel',    'investasi',    '1305', 'SARPRAS', 60000000),
    (bl_pml, v_s, '60400000-0000-4000-8000-000000000001', '5.3.01', 'Pemeliharaan Gedung & Sarana',       'pemeliharaan', '5301', 'SARPRAS', 35000000),
    (bl_hnr, v_s, '60400000-0000-4000-8000-000000000001', '5.4.01', 'Honor Narasumber & Pelatihan Guru',  'personalia',   '5401', 'KUR',    24000000),
    (bl_kbr, v_s, '60400000-0000-4000-8000-000000000001', '5.1.03', 'Perlengkapan Kebersihan',            'operasional',  '5103', 'TU',      9000000),
    ('60410000-0000-4000-8000-000000000011', v_s, '60400000-0000-4000-8000-000000000002', '5.1.01', 'ATK & Bahan Habis Pakai Kantor',  'operasional',  '5101', 'TU',      40000000),
    ('60410000-0000-4000-8000-000000000012', v_s, '60400000-0000-4000-8000-000000000002', '5.2.01', 'Pengadaan Peralatan TIK',         'investasi',    '1305', 'SARPRAS', 95000000),
    ('60410000-0000-4000-8000-000000000013', v_s, '60400000-0000-4000-8000-000000000002', '5.3.01', 'Pemeliharaan Gedung & Sarana',    'pemeliharaan', '5301', 'SARPRAS', 33000000),
    ('60410000-0000-4000-8000-000000000021', v_s, '60400000-0000-4000-8000-000000000003', '5.3.02', 'Renovasi Toilet Siswa',           'pemeliharaan', '5301', 'SARPRAS', 25000000),
    ('60410000-0000-4000-8000-000000000022', v_s, '60400000-0000-4000-8000-000000000003', '5.1.03', 'Tambahan Perlengkapan Kebersihan','operasional',  '5103', 'TU',       5000000),
    ('60410000-0000-4000-8000-000000000031', v_s, '60400000-0000-4000-8000-000000000004', '6.1.01', 'Pembina & Pelatih Ekstrakurikuler','personalia',  '5402', 'KESISWAAN', 15000000),
    ('60410000-0000-4000-8000-000000000032', v_s, '60400000-0000-4000-8000-000000000004', '6.1.02', 'Perlengkapan Pramuka & Futsal',   'operasional',  '5104', 'KESISWAAN',  7500000)
  ON CONFLICT DO NOTHING;

  INSERT INTO budget_actuals_cache (budget_line_id, school_id, actual_idr, last_synced_at) VALUES
    (bl_atk, v_s, 12450000, '2026-10-15 01:00+07'),
    (bl_lab, v_s, 15300000, '2026-10-15 01:00+07'),
    (bl_tik, v_s, 25800000, '2026-10-15 01:00+07'),
    (bl_pml, v_s, 36200000, '2026-10-15 01:00+07'),
    (bl_hnr, v_s, 6000000,  '2026-10-15 01:00+07'),
    (bl_kbr, v_s, 4100000,  '2026-10-15 01:00+07'),
    ('60410000-0000-4000-8000-000000000011', v_s, 38750000, '2026-07-05 01:00+07'),
    ('60410000-0000-4000-8000-000000000012', v_s, 91200000, '2026-07-05 01:00+07'),
    ('60410000-0000-4000-8000-000000000013', v_s, 34100000, '2026-07-05 01:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO budget_alerts (id, school_id, budget_line_id, threshold_bp, current_bp, message, acknowledged_at, acknowledged_by, created_at) VALUES
    ('60420000-0000-4000-8000-000000000001', v_s, bl_lab, 8000, 8500, 'Serapan pos 5.1.02 Bahan Praktikum Laboratorium mencapai 85%', NULL, NULL, '2026-10-08 01:00+07'),
    ('60420000-0000-4000-8000-000000000002', v_s, bl_pml, 8000, 8571, 'Serapan pos 5.3.01 Pemeliharaan Gedung & Sarana mencapai 85,7%', '2026-09-16 10:00+07', v_kepsek, '2026-09-15 01:00+07'),
    ('60420000-0000-4000-8000-000000000003', v_s, bl_pml, 10000, 10343, 'Pos 5.3.01 Pemeliharaan Gedung & Sarana melebihi anggaran (103,4%)', NULL, NULL, '2026-10-12 01:00+07')
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 6. PAJAK NON-GAJI
  ---------------------------------------------------------------------------
  INSERT INTO tax_settings (school_id, npwp_masked, nitku, efaktur_enabled, default_ppn_bp, updated_at)
  VALUES (v_s, '01.***.***.*-432.000', NULL, true, 1100, '2026-07-01 08:00+07')
  ON CONFLICT DO NOTHING;

  PERFORM seed_default_tax_types(v_s);  -- PPh 23/4(2)/22 bawaan; ON CONFLICT DO NOTHING
  SELECT id INTO v_wt_23     FROM withholding_tax_types WHERE school_id = v_s AND code = 'PPH23_SERVICE';
  SELECT id INTO v_wt_22     FROM withholding_tax_types WHERE school_id = v_s AND code = 'PPH22_GOODS';
  SELECT id INTO v_wt_42rent FROM withholding_tax_types WHERE school_id = v_s AND code = 'PPH4_2_RENT';
  SELECT id INTO v_wt_42con  FROM withholding_tax_types WHERE school_id = v_s AND code = 'PPH4_2_CONSTRUCT';

  INSERT INTO withholding_tax_types (id, school_id, code, name, article, standard_rate_bp, no_npwp_rate_bp, description, is_active) VALUES
    ('60550000-0000-4000-8000-000000000001', v_s, 'PPH23_RENT_EQUIP', 'PPh 23 - Sewa Harta selain Tanah/Bangunan', '23', 200, 400, 'Sewa kendaraan, peralatan, sound system', true),
    ('60550000-0000-4000-8000-000000000002', v_s, 'PPH21_NARSUM', 'PPh 21 - Honorarium Narasumber Bukan Pegawai', '21', 250, 300, 'Tidak dipakai lagi; dipindah ke modul payroll', false)
  ON CONFLICT DO NOTHING;

  INSERT INTO withholding_tax_rates (id, school_id, type_id, effective_from, rate_bp, no_npwp_rate_bp) VALUES
    ('60500000-0000-4000-8000-000000000001', v_s, v_wt_23,     '2009-01-01', 200,  400),
    ('60500000-0000-4000-8000-000000000002', v_s, v_wt_22,     '2022-04-01', 150,  300),
    ('60500000-0000-4000-8000-000000000003', v_s, v_wt_42rent, '2018-01-01', 1000, 1000),
    ('60500000-0000-4000-8000-000000000004', v_s, v_wt_42con,  '2008-07-01', 200,  200),
    ('60500000-0000-4000-8000-000000000005', v_s, v_wt_42con,  '2022-02-21', 175,  175)
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 7. UTANG USAHA (AP): pengaturan, faktur vendor (3-way match), jadwal
  --    pembayaran, jurnal. (Dibuat sebelum bukti potong karena dirujuk.)
  ---------------------------------------------------------------------------
  INSERT INTO payable_settings (school_id, default_ap_acc_code, default_vat_in_acc_code, default_tax_payable_acc_code, default_bank_acc_code, tolerance_amount_idr, auto_journal_on_invoice, auto_journal_on_payment, updated_at)
  VALUES (v_s, '2101', '1181', '2131', '1102', 10000, true, true, '2026-07-01 08:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO vendor_invoices (id, school_id, number, vendor_id, po_id, invoice_date, due_date, subtotal_idr, vat_idr, withholding_tax_idr, total_idr, paid_amount_idr,
                               status, match_status, match_discrepancy_note, pdf_object_key, journal_trans_no, idempotency_key, created_by, created_at, updated_at) VALUES
    ('60600000-0000-4000-8000-000000000001', v_s, 'INV/GAM/2026/08/0457', vd_gram, '60260000-0000-4000-8000-000000000001', '2026-08-21', '2026-09-20', 2854000, 313940, 0, 3167940, 3167940,
     'paid', 'matched_ok', NULL, 'ops/ap/INV-GAM-2026-08-0457.pdf', 'AP-INV-2026-08-0001', 'seed-ops-inv-01', v_admin, '2026-08-21 10:00+07', '2026-09-18 10:00+07'),
    ('60600000-0000-4000-8000-000000000002', v_s, 'KMP-2609-118', vd_komp, '60260000-0000-4000-8000-000000000002', '2026-09-12', '2026-09-26', 25800000, 2838000, 387000, 28251000, 14000000,
     'partially_paid', 'matched_ok', NULL, 'ops/ap/KMP-2609-118.pdf', 'AP-INV-2026-09-0002', 'seed-ops-inv-02', v_admin, '2026-09-12 10:00+07', '2026-09-25 10:00+07'),
    ('60600000-0000-4000-8000-000000000003', v_s, 'LBR/INV/1007', vd_labo, '60260000-0000-4000-8000-000000000003', '2026-10-08', '2026-11-07', 450000, 49500, 0, 499500, 0,
     'discrepancy', 'qty_mismatch', 'Ditagih 12 botol alkohol, diterima 10 botol (2 pecah)', 'ops/ap/LBR-INV-1007.pdf', NULL, 'seed-ops-inv-03', v_admin, '2026-10-08 10:00+07', '2026-10-08 10:05+07'),
    ('60600000-0000-4000-8000-000000000004', v_s, 'INV-SBR-0921', vd_bers, NULL, '2026-09-30', '2026-10-07', 3000000, 330000, 60000, 3270000, 0,
     'approved', 'matched_ok', NULL, 'ops/ap/INV-SBR-0921.pdf', NULL, 'seed-ops-inv-04', v_admin, '2026-09-30 10:00+07', '2026-10-01 10:00+07'),
    ('60600000-0000-4000-8000-000000000005', v_s, 'SEN-2026-0912', v_vnd1, NULL, '2026-10-14', '2026-11-13', 1250000, 137500, 0, 1387500, 0,
     'draft', 'pending', NULL, NULL, NULL, 'seed-ops-inv-05', v_admin, '2026-10-14 10:00+07', '2026-10-14 10:00+07'),
    ('60600000-0000-4000-8000-000000000006', v_s, 'SEN-2026-0702', v_vnd1, NULL, '2026-07-02', '2026-08-01', 2400000, 264000, 0, 2664000, 0,
     'approved', 'matched_ok', NULL, 'ops/ap/SEN-2026-0702.pdf', 'AP-INV-2026-07-0001', 'seed-ops-inv-06', v_admin, '2026-07-02 10:00+07', '2026-07-03 10:00+07'),
    ('60600000-0000-4000-8000-000000000007', v_s, 'KMP-2610-131', vd_komp, NULL, '2026-10-10', '2026-10-24', 4500000, 495000, 90000, 4905000, 0,
     'matched', 'matched_ok', NULL, 'ops/ap/KMP-2610-131.pdf', NULL, 'seed-ops-inv-07', v_admin, '2026-10-10 10:00+07', '2026-10-10 10:30+07'),
    ('60600000-0000-4000-8000-000000000008', v_s, 'INV/GAM/2026/08/0457-B', vd_gram, '60260000-0000-4000-8000-000000000001', '2026-08-22', '2026-09-21', 2888000, 317680, 0, 3205680, 0,
     'cancelled', 'over_billed', 'Faktur ganda; 10 map sudah diretur', NULL, NULL, 'seed-ops-inv-08', v_admin, '2026-08-22 10:00+07', '2026-08-23 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO vendor_invoice_lines (id, school_id, invoice_id, po_line_id, gr_line_id, description, qty, unit_price_idr, withholding_type_id, withholding_tax_idr, budget_line_key) VALUES
    ('60610000-0000-4000-8000-000000000001', v_s, '60600000-0000-4000-8000-000000000001', '60270000-0000-4000-8000-000000000001', '60290000-0000-4000-8000-000000000001', 'Tinta Printer Epson 003 Hitam', 24, 92000, NULL, 0, bl_atk),
    ('60610000-0000-4000-8000-000000000002', v_s, '60600000-0000-4000-8000-000000000001', '60270000-0000-4000-8000-000000000002', '60290000-0000-4000-8000-000000000002', 'Map Plastik Kancing Folio (setelah retur 10)', 190, 3400, NULL, 0, bl_atk),
    ('60610000-0000-4000-8000-000000000003', v_s, '60600000-0000-4000-8000-000000000002', '60270000-0000-4000-8000-000000000003', '60290000-0000-4000-8000-000000000003', 'Laptop Guru ASUS Vivobook 14', 3, 8600000, v_wt_22, 387000, bl_tik),
    ('60610000-0000-4000-8000-000000000004', v_s, '60600000-0000-4000-8000-000000000003', '60270000-0000-4000-8000-000000000004', '60290000-0000-4000-8000-000000000004', 'Alkohol 70% 1 Liter', 12, 37500, NULL, 0, bl_lab),
    ('60610000-0000-4000-8000-000000000005', v_s, '60600000-0000-4000-8000-000000000004', NULL, NULL, 'Jasa kebersihan gedung September 2026', 1, 3000000, v_wt_23, 60000, bl_pml),
    ('60610000-0000-4000-8000-000000000006', v_s, '60600000-0000-4000-8000-000000000005', NULL, NULL, 'Buku tulis sekolah bersampul logo (pak isi 10)', 50, 25000, NULL, 0, bl_atk),
    ('60610000-0000-4000-8000-000000000007', v_s, '60600000-0000-4000-8000-000000000006', NULL, NULL, 'Buku agenda guru TA 2026/2027', 60, 40000, NULL, 0, bl_atk),
    ('60610000-0000-4000-8000-000000000008', v_s, '60600000-0000-4000-8000-000000000007', NULL, NULL, 'Jasa instalasi jaringan & access point Gedung B', 1, 4500000, v_wt_23, 90000, bl_pml),
    ('60610000-0000-4000-8000-000000000009', v_s, '60600000-0000-4000-8000-000000000008', '60270000-0000-4000-8000-000000000001', NULL, 'Tinta Printer Epson 003 Hitam', 24, 92000, NULL, 0, bl_atk),
    ('60610000-0000-4000-8000-00000000000a', v_s, '60600000-0000-4000-8000-000000000008', '60270000-0000-4000-8000-000000000002', NULL, 'Map Plastik Kancing Folio', 200, 3400, NULL, 0, bl_atk)
  ON CONFLICT DO NOTHING;

  INSERT INTO payment_schedules (id, school_id, invoice_id, vendor_id, scheduled_date, amount_idr, payment_method, source_bank_acc_code, vendor_bank_account_id,
                                 status, executed_at, executed_by, reference_no, journal_trans_no, created_by, created_at, updated_at) VALUES
    ('60620000-0000-4000-8000-000000000001', v_s, '60600000-0000-4000-8000-000000000001', vd_gram, '2026-09-18', 3167940, 'bank_transfer', '1102', '60230000-0000-4000-8000-000000000003',
     'executed', '2026-09-18 10:00+07', v_admin, 'TRF-20260918-001', 'AP-PAY-2026-09-0001', v_admin, '2026-09-15 10:00+07', '2026-09-18 10:00+07'),
    ('60620000-0000-4000-8000-000000000002', v_s, '60600000-0000-4000-8000-000000000002', vd_komp, '2026-09-25', 14000000, 'bank_transfer', '1102', '60230000-0000-4000-8000-000000000004',
     'executed', '2026-09-25 10:00+07', v_admin, 'TRF-20260925-004', 'AP-PAY-2026-09-0002', v_admin, '2026-09-20 10:00+07', '2026-09-25 10:00+07'),
    ('60620000-0000-4000-8000-000000000003', v_s, '60600000-0000-4000-8000-000000000002', vd_komp, '2026-10-20', 14251000, 'bank_transfer', '1102', '60230000-0000-4000-8000-000000000004',
     'pending_approval', NULL, NULL, NULL, NULL, v_admin, '2026-10-13 10:00+07', '2026-10-13 10:00+07'),
    ('60620000-0000-4000-8000-000000000004', v_s, '60600000-0000-4000-8000-000000000004', vd_bers, '2026-10-17', 3270000, 'cash', '1101', NULL,
     'approved', NULL, NULL, NULL, NULL, v_admin, '2026-10-08 10:00+07', '2026-10-09 10:00+07'),
    ('60620000-0000-4000-8000-000000000005', v_s, '60600000-0000-4000-8000-000000000006', v_vnd1, '2026-10-16', 2664000, 'bank_transfer', '1102', '60230000-0000-4000-8000-000000000002',
     'scheduled', NULL, NULL, NULL, NULL, v_admin, '2026-10-14 10:00+07', '2026-10-14 10:00+07'),
    ('60620000-0000-4000-8000-000000000006', v_s, '60600000-0000-4000-8000-000000000006', v_vnd1, '2026-08-01', 2664000, 'giro', '1102', NULL,
     'cancelled', NULL, NULL, NULL, NULL, v_admin, '2026-07-25 10:00+07', '2026-07-31 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO payable_journals (id, school_id, source_type, source_id, trans_no, trans_date, amount_idr, description, status, error_message, created_at) VALUES
    ('60630000-0000-4000-8000-000000000001', v_s, 'invoice', '60600000-0000-4000-8000-000000000006', 'AP-INV-2026-07-0001', '2026-07-03', 2664000,  'Utang SEN-2026-0702 CV Sarana Edukasi Nusantara', 'posted', NULL, '2026-07-03 10:00+07'),
    ('60630000-0000-4000-8000-000000000002', v_s, 'invoice', '60600000-0000-4000-8000-000000000001', 'AP-INV-2026-08-0001', '2026-08-21', 3167940,  'Utang INV/GAM/2026/08/0457 PT Gramedia Asri Media', 'posted', NULL, '2026-08-21 10:05+07'),
    ('60630000-0000-4000-8000-000000000003', v_s, 'payment', '60620000-0000-4000-8000-000000000001', 'AP-PAY-2026-09-0001', '2026-09-18', 3167940,  'Pelunasan INV/GAM/2026/08/0457', 'posted', NULL, '2026-09-18 10:05+07'),
    ('60630000-0000-4000-8000-000000000004', v_s, 'invoice', '60600000-0000-4000-8000-000000000002', 'AP-INV-2026-09-0002', '2026-09-12', 28251000, 'Utang KMP-2609-118 CV Mitra Komputindo', 'posted', NULL, '2026-09-12 10:05+07'),
    ('60630000-0000-4000-8000-000000000005', v_s, 'payment', '60620000-0000-4000-8000-000000000002', 'AP-PAY-2026-09-0002', '2026-09-25', 14000000, 'Pembayaran termin 1 KMP-2609-118', 'posted', NULL, '2026-09-25 10:05+07'),
    ('60630000-0000-4000-8000-000000000006', v_s, 'invoice', '60600000-0000-4000-8000-000000000004', 'AP-INV-2026-10-FAIL', '2026-10-01', 3270000,  'Utang INV-SBR-0921 UD Sumber Bersih Sentosa', 'failed', 'Akun beban jasa kebersihan belum dipetakan', '2026-10-01 10:05+07')
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 6b. PAJAK: bukti potong, PPN masukan/keluaran, ekspor DJP
  --     (nomor BUPOT/<periode>/NNNN berurutan per periode agar penomoran
  --     otomatis aplikasi = COUNT+1 tidak bentrok)
  ---------------------------------------------------------------------------
  INSERT INTO withholding_slips (id, school_id, slip_number, type_id, vendor_id, vendor_invoice_id, tax_period, gross_amount_idr, rate_bp, tax_amount_idr, npwp_used,
                                 status, issued_at, reported_at, pdf_object_key, created_by, created_at, updated_at) VALUES
    ('60510000-0000-4000-8000-000000000001', v_s, 'BUPOT/2026-08/0001', v_wt_23, vd_komp, NULL, '2026-08', 4500000, 200, 90000, '02.***.***.*-415.000',
     'reported', '2026-08-31 10:00+07', '2026-09-15 10:00+07', 'ops/tax/BUPOT-2026-08-0001.pdf', v_admin, '2026-08-28 10:00+07', '2026-09-15 10:00+07'),
    ('60510000-0000-4000-8000-000000000002', v_s, 'BUPOT/2026-09/0001', v_wt_22, vd_komp, '60600000-0000-4000-8000-000000000002', '2026-09', 25800000, 150, 387000, '02.***.***.*-415.000',
     'issued', '2026-09-25 10:00+07', NULL, 'ops/tax/BUPOT-2026-09-0001.pdf', v_admin, '2026-09-12 10:00+07', '2026-09-25 10:00+07'),
    ('60510000-0000-4000-8000-000000000003', v_s, 'BUPOT/2026-09/0002', v_wt_23, vd_bers, '60600000-0000-4000-8000-000000000004', '2026-09', 3000000, 200, 60000, '07.***.***.*-432.000',
     'issued', '2026-09-30 10:00+07', NULL, 'ops/tax/BUPOT-2026-09-0002.pdf', v_admin, '2026-09-30 10:00+07', '2026-09-30 10:00+07'),
    ('60510000-0000-4000-8000-000000000004', v_s, 'BUPOT/2026-10/0001', v_wt_23, vd_komp, '60600000-0000-4000-8000-000000000007', '2026-10', 4500000, 200, 90000, '02.***.***.*-415.000',
     'draft', NULL, NULL, NULL, v_admin, '2026-10-10 10:00+07', '2026-10-10 10:00+07'),
    ('60510000-0000-4000-8000-000000000005', v_s, 'BUPOT/2026-10/0002', v_wt_42con, vd_mebl, NULL, '2026-10', 15000000, 175, 262500, NULL,
     'cancelled', NULL, NULL, NULL, v_admin, '2026-10-02 10:00+07', '2026-10-03 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO input_vat (id, school_id, invoice_number, vendor_id, vendor_invoice_id, tax_period, invoice_date, dpp_idr, vat_idr, creditable, status, created_at, updated_at) VALUES
    ('60520000-0000-4000-8000-000000000001', v_s, '010.000-26.00104571', vd_gram, '60600000-0000-4000-8000-000000000001', '2026-08', '2026-08-21', 2854000, 313940, true, 'credited', '2026-08-21 10:00+07', '2026-09-15 10:00+07'),
    ('60520000-0000-4000-8000-000000000002', v_s, '010.000-26.00211802', vd_komp, '60600000-0000-4000-8000-000000000002', '2026-09', '2026-09-12', 25800000, 2838000, true, 'received', '2026-09-12 10:00+07', '2026-09-12 10:00+07'),
    ('60520000-0000-4000-8000-000000000003', v_s, '010.000-26.00310077', vd_labo, '60600000-0000-4000-8000-000000000003', '2026-10', '2026-10-08', 450000, 49500, true, 'received', '2026-10-08 10:00+07', '2026-10-08 10:00+07'),
    ('60520000-0000-4000-8000-000000000004', v_s, '010.000-26.00092100', vd_bers, '60600000-0000-4000-8000-000000000004', '2026-09', '2026-09-30', 3000000, 330000, false, 'rejected', '2026-09-30 10:00+07', '2026-10-02 10:00+07'),
    ('60520000-0000-4000-8000-000000000005', v_s, '010.000-25.00870012', v_vnd1, NULL, '2025-12', '2025-12-18', 1200000, 132000, true, 'expired', '2025-12-18 10:00+07', '2026-07-01 10:00+07'),
    ('60520000-0000-4000-8000-000000000006', v_s, '010.000-26.00330131', vd_komp, '60600000-0000-4000-8000-000000000007', '2026-10', '2026-10-10', 4500000, 495000, true, 'received', '2026-10-10 10:00+07', '2026-10-10 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO output_tax_invoices (id, school_id, invoice_number, customer_name, customer_npwp, tax_period, invoice_date, dpp_idr, vat_idr, status, created_at, updated_at) VALUES
    ('60530000-0000-4000-8000-000000000001', v_s, '010.000-26.00000100', 'Yayasan Pelita Bangsa (contoh)', NULL, '2026-08', '2026-08-12', 2000000, 220000, 'cancelled', '2026-08-12 10:00+07', '2026-08-13 10:00+07'),
    ('60530000-0000-4000-8000-000000000002', v_s, '010.000-26.00000101', 'PT Cahaya Event Organizer (contoh)', '03.***.***.*-407.000', '2026-09', '2026-09-20', 5000000, 550000, 'approved', '2026-09-20 10:00+07', '2026-09-21 10:00+07'),
    ('60530000-0000-4000-8000-000000000003', v_s, '010.000-26.00000102', 'CV Rasa Nusantara Katering (contoh)', '04.***.***.*-432.000', '2026-10', '2026-10-05', 3000000, 330000, 'approved', '2026-10-05 10:00+07', '2026-10-06 10:00+07'),
    ('60530000-0000-4000-8000-000000000004', v_s, '010.000-26.00000103', 'PT Cahaya Event Organizer (contoh)', '03.***.***.*-407.000', '2026-10', '2026-10-14', 4000000, 440000, 'draft', '2026-10-14 10:00+07', '2026-10-14 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO tax_export_templates (id, school_id, kind, tax_period, record_count, total_tax_idr, file_object_key, generated_by, created_at) VALUES
    ('60540000-0000-4000-8000-000000000001', v_s, 'ebupot_23',      '2026-08', 1, 90000,  'ops/tax/exports/ebupot23-2026-08.csv',      v_admin, '2026-09-10 10:00+07'),
    ('60540000-0000-4000-8000-000000000002', v_s, 'efaktur_input',  '2026-08', 1, 313940, 'ops/tax/exports/efaktur-masukan-2026-08.csv', v_admin, '2026-09-10 10:05+07'),
    ('60540000-0000-4000-8000-000000000003', v_s, 'efaktur_output', '2026-09', 1, 550000, 'ops/tax/exports/efaktur-keluaran-2026-09.csv', v_admin, '2026-10-10 10:00+07'),
    ('60540000-0000-4000-8000-000000000004', v_s, 'ebupot_4_2',     '2026-09', 0, 0,      'ops/tax/exports/ebupot42-2026-09.csv',      v_admin, '2026-10-10 10:05+07')
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 8. PERPUSTAKAAN
  ---------------------------------------------------------------------------
  INSERT INTO lib_settings (school_id, fine_policy, reminder_days_before)
  VALUES (v_s, 'pay_at_library', 2)
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_member_categories (id, school_id, name, applies_to, is_default, max_loans, loan_days, max_renewals, reservation_hold_days) VALUES
    (mc_gtk, v_s, 'Guru & Tenaga Kependidikan', 'employee', true,  5, 21, 2, 3),
    (mc_osn, v_s, 'Siswa Tim Olimpiade (OSN)',  'student',  false, 5, 21, 2, 2)
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_fine_rates (id, school_id, member_category_id, late_fee_per_day_idr, lost_fee_idr, effective_from, effective_to) VALUES
    ('60710000-0000-4000-8000-000000000001', v_s, v_mcat_siswa, 1000, NULL,   '2026-07-01', NULL),
    ('60710000-0000-4000-8000-000000000002', v_s, mc_gtk,       500,  NULL,   '2026-07-01', NULL),
    ('60710000-0000-4000-8000-000000000003', v_s, NULL,         1000, 100000, '2025-07-01', '2026-06-30'),
    ('60710000-0000-4000-8000-000000000004', v_s, mc_osn,       500,  NULL,   '2026-07-01', NULL)
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_members (id, school_id, kind, user_id, category_id, member_code, status, pending_reason, closed_at, synced_at) VALUES
    (m_guru, v_s, 'employee', v_guru,    mc_gtk, 'LIB-GTK-001', 'active',   NULL, NULL, '2026-07-13 08:00+07'),
    (m_wali, v_s, 'employee', v_wali,    mc_gtk, 'LIB-GTK-002', 'active',   NULL, NULL, '2026-07-13 08:00+07'),
    (m_ks,   v_s, 'employee', v_kepsek,  mc_gtk, 'LIB-GTK-003', 'active',   NULL, NULL, '2026-07-13 08:00+07'),
    (m_adm,  v_s, 'employee', v_admin,   mc_gtk, 'LIB-GTK-004', 'active',   NULL, NULL, '2026-07-13 08:00+07'),
    (m_yys,  v_s, 'employee', v_yayasan, mc_gtk, 'LIB-GTK-005', 'pending_close', 'Masih ada denda buku hilang yang belum diselesaikan', NULL, '2026-10-01 08:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_collection_categories (id, school_id, code, name) VALUES
    (lc_fik, v_s, 'CAT-FIKSI',     'Fiksi & Sastra'),
    (lc_ref, v_s, 'CAT-REFERENSI', 'Referensi & Kamus'),
    (lc_sai, v_s, 'CAT-SAINS',     'Sains & Pengetahuan Umum')
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_titles (id, school_id, title, author, publisher, isbn, publish_year, category_id, replacement_price_idr) VALUES
    ('60740000-0000-4000-8000-000000000001', v_s, 'Laskar Pelangi', 'Andrea Hirata', 'Bentang Pustaka', '978-979-3062-79-2', 2005, lc_fik, 89000),
    ('60740000-0000-4000-8000-000000000002', v_s, 'Bumi', 'Tere Liye', 'Gramedia Pustaka Utama', '978-602-03-3295-6', 2014, lc_fik, 98000),
    ('60740000-0000-4000-8000-000000000003', v_s, 'Kamus Besar Bahasa Indonesia Edisi Kelima', 'Badan Pengembangan dan Pembinaan Bahasa', 'Balai Pustaka', '978-979-690-955-7', 2016, lc_ref, 350000),
    ('60740000-0000-4000-8000-000000000004', v_s, 'Ilmu Pengetahuan Alam SMP Kelas VII', 'Kemendikbudristek', 'Pusat Perbukuan', '978-623-194-001-2', 2026, v_libcat_pel, 75000),
    ('60740000-0000-4000-8000-000000000005', v_s, 'Sapiens: Riwayat Singkat Umat Manusia', 'Yuval Noah Harari', 'Kepustakaan Populer Gramedia', '978-602-424-232-8', 2017, lc_sai, 125000),
    ('60740000-0000-4000-8000-000000000006', v_s, 'English for Nusantara SMP Kelas VII', 'Kemendikbudristek', 'Pusat Perbukuan', '978-623-194-012-8', 2026, v_libcat_pel, 70000)
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_copies (id, school_id, title_id, barcode, shelf_location, status) VALUES
    ('60750000-0000-4000-8000-000000000001', v_s, '60740000-0000-4000-8000-000000000001', 'BC-LPL-001', 'RAK-F-01', 'on_loan'),
    ('60750000-0000-4000-8000-000000000002', v_s, '60740000-0000-4000-8000-000000000001', 'BC-LPL-002', 'RAK-F-01', 'available'),
    ('60750000-0000-4000-8000-000000000003', v_s, '60740000-0000-4000-8000-000000000002', 'BC-BUMI-001', 'RAK-F-02', 'on_loan'),
    ('60750000-0000-4000-8000-000000000004', v_s, '60740000-0000-4000-8000-000000000002', 'BC-BUMI-002', 'MEJA-SIRKULASI', 'on_hold'),
    ('60750000-0000-4000-8000-000000000005', v_s, '60740000-0000-4000-8000-000000000003', 'BC-KBBI-001', 'RAK-R-01', 'available'),
    ('60750000-0000-4000-8000-000000000006', v_s, '60740000-0000-4000-8000-000000000004', 'BC-IPA7-001', 'RAK-A-02', 'on_loan'),
    ('60750000-0000-4000-8000-000000000007', v_s, '60740000-0000-4000-8000-000000000004', 'BC-IPA7-002', 'RAK-A-02', 'available'),
    ('60750000-0000-4000-8000-000000000008', v_s, '60740000-0000-4000-8000-000000000005', 'BC-SAP-001', 'RAK-S-01', 'lost'),
    ('60750000-0000-4000-8000-000000000009', v_s, '60740000-0000-4000-8000-000000000005', 'BC-SAP-002', 'RAK-S-01', 'damaged'),
    ('60750000-0000-4000-8000-00000000000a', v_s, '60740000-0000-4000-8000-000000000006', 'BC-ENG7-001', 'RAK-A-03', 'available'),
    ('60750000-0000-4000-8000-00000000000b', v_s, '60740000-0000-4000-8000-000000000006', 'BC-ENG7-002', 'GUDANG-PERPUS', 'withdrawn')
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_loans (id, school_id, copy_id, member_id, classroom_name, loaned_at, due_date, returned_at, renew_count, status, loaned_by, returned_by, idempotency_key, created_at, updated_at) VALUES
    ('60760000-0000-4000-8000-000000000001', v_s, '60750000-0000-4000-8000-000000000001', v_mem_budi, 'VII A', '2026-10-08 10:00+07', '2026-10-22', NULL, 0, 'active', v_admin, NULL, 'seed-ops-loan-01', '2026-10-08 10:00+07', '2026-10-08 10:00+07'),
    ('60760000-0000-4000-8000-000000000002', v_s, '60750000-0000-4000-8000-000000000003', v_mem_budi, 'VII A', '2026-09-24 10:00+07', '2026-10-08', NULL, 0, 'active', v_admin, NULL, 'seed-ops-loan-02', '2026-09-24 10:00+07', '2026-09-24 10:00+07'),
    ('60760000-0000-4000-8000-000000000003', v_s, '60750000-0000-4000-8000-000000000006', m_guru, NULL, '2026-09-10 09:00+07', '2026-10-22', NULL, 1, 'active', v_admin, NULL, 'seed-ops-loan-03', '2026-09-10 09:00+07', '2026-10-01 09:00+07'),
    ('60760000-0000-4000-8000-000000000004', v_s, '60750000-0000-4000-8000-000000000008', m_wali, NULL, '2026-08-10 09:00+07', '2026-08-31', NULL, 0, 'lost', v_admin, NULL, 'seed-ops-loan-04', '2026-08-10 09:00+07', '2026-09-05 09:00+07'),
    ('60760000-0000-4000-8000-000000000005', v_s, '60750000-0000-4000-8000-00000000000a', v_mem_budi, 'VII A', '2026-09-01 10:00+07', '2026-09-15', '2026-09-19 12:00+07', 0, 'returned', v_admin, v_admin, 'seed-ops-loan-05', '2026-09-01 10:00+07', '2026-09-19 12:00+07'),
    ('60760000-0000-4000-8000-000000000006', v_s, '60750000-0000-4000-8000-000000000002', m_ks, NULL, '2026-09-05 09:00+07', '2026-09-26', '2026-09-25 14:00+07', 0, 'returned', v_admin, v_admin, 'seed-ops-loan-06', '2026-09-05 09:00+07', '2026-09-25 14:00+07'),
    ('60760000-0000-4000-8000-000000000007', v_s, '60750000-0000-4000-8000-000000000007', v_mem_budi, 'VII A', '2026-08-04 10:00+07', '2026-08-18', '2026-08-25 12:00+07', 0, 'returned', v_admin, v_admin, 'seed-ops-loan-07', '2026-08-04 10:00+07', '2026-08-25 12:00+07'),
    ('60760000-0000-4000-8000-000000000008', v_s, '60750000-0000-4000-8000-000000000009', m_adm, NULL, '2026-09-10 09:00+07', '2026-10-01', '2026-10-01 15:00+07', 0, 'returned', v_admin, v_admin, 'seed-ops-loan-08', '2026-09-10 09:00+07', '2026-10-01 15:00+07'),
    ('60760000-0000-4000-8000-000000000009', v_s, '60750000-0000-4000-8000-000000000005', m_wali, NULL, '2026-09-01 09:00+07', '2026-09-22', '2026-10-02 10:00+07', 0, 'returned', v_admin, v_admin, 'seed-ops-loan-09', '2026-09-01 09:00+07', '2026-10-02 10:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_reservations (id, school_id, title_id, member_id, status, held_copy_id, ready_until, created_at, updated_at) VALUES
    ('60770000-0000-4000-8000-000000000001', v_s, '60740000-0000-4000-8000-000000000002', m_guru,     'ready',     '60750000-0000-4000-8000-000000000004', '2026-10-17', '2026-10-05 09:00+07', '2026-10-14 09:00+07'),
    ('60770000-0000-4000-8000-000000000002', v_s, '60740000-0000-4000-8000-000000000001', m_ks,       'waiting',   NULL, NULL, '2026-10-12 09:00+07', '2026-10-12 09:00+07'),
    ('60770000-0000-4000-8000-000000000003', v_s, '60740000-0000-4000-8000-000000000004', v_mem_budi, 'fulfilled', NULL, NULL, '2026-07-28 09:00+07', '2026-08-04 10:00+07'),
    ('60770000-0000-4000-8000-000000000004', v_s, '60740000-0000-4000-8000-000000000005', m_wali,     'cancelled', NULL, NULL, '2026-09-01 09:00+07', '2026-09-03 09:00+07'),
    ('60770000-0000-4000-8000-000000000005', v_s, '60740000-0000-4000-8000-000000000003', m_adm,      'expired',   NULL, '2026-09-12', '2026-09-08 09:00+07', '2026-09-13 00:10+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_fines (id, school_id, loan_id, member_id, kind, late_days, amount_idr, rate_snapshot, settlement, status, paid_at, received_by, waived_by, waive_reason, created_at, updated_at) VALUES
    ('60780000-0000-4000-8000-000000000001', v_s, '60760000-0000-4000-8000-000000000005', v_mem_budi, 'late', 4, 4000,
     '{"late_fee_per_day_idr": 1000}'::jsonb, 'pay_at_library', 'paid', '2026-09-19 12:05+07', v_admin, NULL, NULL, '2026-09-19 12:00+07', '2026-09-19 12:05+07'),
    ('60780000-0000-4000-8000-000000000002', v_s, '60760000-0000-4000-8000-000000000007', v_mem_budi, 'late', 7, 7000,
     '{"late_fee_per_day_idr": 1000}'::jsonb, 'pay_at_library', 'waived', NULL, NULL, v_admin, 'Siswa sakit (surat dokter terlampir)', '2026-08-25 12:00+07', '2026-08-26 09:00+07'),
    ('60780000-0000-4000-8000-000000000003', v_s, '60760000-0000-4000-8000-000000000009', m_wali, 'late', 10, 5000,
     '{"late_fee_per_day_idr": 500}'::jsonb, 'pay_at_library', 'open', NULL, NULL, NULL, NULL, '2026-10-02 10:00+07', '2026-10-02 10:00+07'),
    ('60780000-0000-4000-8000-000000000004', v_s, '60760000-0000-4000-8000-000000000004', m_wali, 'lost', NULL, 125000,
     '{"kind": "lost", "replacement_price_idr": 125000}'::jsonb, 'pay_at_library', 'open', NULL, NULL, NULL, NULL, '2026-09-05 09:00+07', '2026-09-05 09:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO lib_visits (id, school_id, member_id, visited_on, visited_at) VALUES
    ('60790000-0000-4000-8000-000000000001', v_s, v_mem_budi, '2026-10-01', '2026-10-01 09:40+07'),
    ('60790000-0000-4000-8000-000000000002', v_s, v_mem_budi, '2026-10-02', '2026-10-02 12:10+07'),
    ('60790000-0000-4000-8000-000000000003', v_s, v_mem_budi, '2026-10-06', '2026-10-06 09:45+07'),
    ('60790000-0000-4000-8000-000000000004', v_s, v_mem_budi, '2026-10-08', '2026-10-08 10:00+07'),
    ('60790000-0000-4000-8000-000000000005', v_s, v_mem_budi, '2026-10-13', '2026-10-13 12:15+07'),
    ('60790000-0000-4000-8000-000000000006', v_s, v_mem_budi, '2026-10-14', '2026-10-14 09:35+07'),
    ('60790000-0000-4000-8000-000000000007', v_s, m_guru,     '2026-10-01', '2026-10-01 13:00+07'),
    ('60790000-0000-4000-8000-000000000008', v_s, m_guru,     '2026-10-08', '2026-10-08 13:20+07'),
    ('60790000-0000-4000-8000-000000000009', v_s, m_wali,     '2026-10-02', '2026-10-02 10:00+07'),
    ('60790000-0000-4000-8000-00000000000a', v_s, m_ks,       '2026-10-14', '2026-10-14 14:00+07'),
    ('60790000-0000-4000-8000-00000000000b', v_s, m_adm,      '2026-10-13', '2026-10-13 07:30+07')
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 9. TRANSPORTASI
  ---------------------------------------------------------------------------
  INSERT INTO transport_settings (school_id, prorata_policy, doc_reminder_days, notify_parent_on_trip, component_code, bill_due_day)
  VALUES (v_s, 'daily', 30, true, 'TRANSPORT', 10)
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_zones (id, school_id, name) VALUES
    (z1, v_s, 'Zona 1 (0-5 km)'),
    (z2, v_s, 'Zona 2 (5-10 km)'),
    (z3, v_s, 'Zona 3 (lebih dari 10 km)')
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_routes (id, school_id, code, name, status) VALUES
    (rt2, v_s, 'RTE-02', 'Rute 02 - Harapan Indah / Medan Satria', 'active'),
    (rt3, v_s, 'RTE-03', 'Rute 03 - Jatiasih / Pondok Gede', 'inactive')
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_stops (id, school_id, route_id, seq, name, address, zone_id, pickup_time, dropoff_time) VALUES
    ('60810000-0000-4000-8000-000000000001', v_s, v_route1, 1, 'Halte Summarecon Mall Bekasi', 'Jl. Bulevar Ahmad Yani, Bekasi Utara', z2, '06:10', '15:40'),
    ('60810000-0000-4000-8000-000000000002', v_s, v_route1, 2, 'Gerbang Perumahan Galaxy',     'Jl. Pulo Sirih Utama, Bekasi Selatan', z2, '06:25', '15:25'),
    ('60810000-0000-4000-8000-000000000003', v_s, v_route1, 3, 'Taman Kota Bekasi',            'Jl. Ahmad Yani, Bekasi Selatan',       z1, '06:40', '15:10'),
    ('60810000-0000-4000-8000-000000000004', v_s, rt2,      1, 'Bundaran Harapan Indah',       'Jl. Harapan Indah Boulevard',          z3, '05:55', '15:55'),
    ('60810000-0000-4000-8000-000000000005', v_s, rt2,      2, 'Pasar Medan Satria',           'Jl. Sultan Agung, Medan Satria',       z2, '06:15', '15:35'),
    ('60810000-0000-4000-8000-000000000006', v_s, rt2,      3, 'Kantor Kelurahan Kaliabang',   'Jl. Kaliabang Tengah',                 z1, '06:35', '15:15'),
    ('60810000-0000-4000-8000-000000000007', v_s, rt3,      1, 'Gerbang Tol Jatiasih',         'Jl. Raya Jatiasih',                    z3, '05:50', '16:00'),
    ('60810000-0000-4000-8000-000000000008', v_s, rt3,      2, 'Pasar Pondok Gede',            'Jl. Raya Pondok Gede',                 z3, '06:10', '15:40')
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_vehicles (id, school_id, plate_no, name, capacity, status) VALUES
    (veh2, v_s, 'B 7456 KAB', 'Bus Sekolah Isuzu Elf 02', 19, 'active'),
    (veh3, v_s, 'B 1987 TPS', 'Toyota HiAce Commuter 03', 14, 'maintenance')
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_vehicle_documents (id, school_id, vehicle_id, kind, number, valid_until, object_key) VALUES
    ('60830000-0000-4000-8000-000000000001', v_s, v_veh1, 'stnk',     'STNK-B7123KAA-2027', '2027-03-12', 'ops/transport/B7123KAA/stnk.pdf'),
    ('60830000-0000-4000-8000-000000000002', v_s, v_veh1, 'uji_kir',  'KIR-BKS-0912-2026',  '2026-10-30', 'ops/transport/B7123KAA/kir.pdf'),
    ('60830000-0000-4000-8000-000000000003', v_s, v_veh1, 'asuransi', 'POL-ASR-77120045',   '2027-01-31', NULL),
    ('60830000-0000-4000-8000-000000000004', v_s, veh2,   'stnk',     'STNK-B7456KAB-2028', '2028-05-20', 'ops/transport/B7456KAB/stnk.pdf'),
    ('60830000-0000-4000-8000-000000000005', v_s, veh2,   'uji_kir',  'KIR-BKS-0214-2027',  '2027-02-14', NULL),
    ('60830000-0000-4000-8000-000000000006', v_s, veh3,   'uji_kir',  'KIR-BKS-0930-2026',  '2026-09-30', NULL)
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_crew (id, school_id, kind, employee_id, user_id, external_name, phone, license_no, license_valid_until) VALUES
    (cr_sugeng, v_s, 'driver',    NULL, NULL,    'Pak Sugeng Riyadi',   '0812-3456-7801', 'SIM-B1U-551203', '2027-08-01'),
    (cr_rina,   v_s, 'attendant', NULL, NULL,    'Bu Rina Marlina',     '0813-3456-7802', NULL, NULL),
    (cr_admin,  v_s, 'attendant', NULL, v_admin, 'Admin Teratai',       '0811-3456-7803', NULL, NULL),
    (cr_joko,   v_s, 'driver',    NULL, NULL,    'Pak Joko Susilo',     '0815-3456-7804', 'SIM-B1U-448190', '2026-11-05')
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_route_assignments (id, school_id, route_id, vehicle_id, driver_crew_id, attendant_crew_id, valid_from, valid_to) VALUES
    ('60850000-0000-4000-8000-000000000001', v_s, v_route1, v_veh1, cr_joko,        NULL,     '2025-07-14', '2026-07-12'),
    ('60850000-0000-4000-8000-000000000002', v_s, v_route1, v_veh1, v_crew_bambang, cr_admin, '2026-07-13', NULL),
    ('60850000-0000-4000-8000-000000000003', v_s, rt2,      veh2,   cr_sugeng,      cr_rina,  '2026-07-13', NULL)
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_fares (id, school_id, zone_id, route_id, monthly_amount_idr, effective_from, effective_to) VALUES
    ('60860000-0000-4000-8000-000000000001', v_s, z1,   NULL, 350000, '2026-07-01', NULL),
    ('60860000-0000-4000-8000-000000000002', v_s, z2,   NULL, 450000, '2026-07-01', NULL),
    ('60860000-0000-4000-8000-000000000003', v_s, z3,   NULL, 550000, '2026-07-01', NULL),
    ('60860000-0000-4000-8000-000000000004', v_s, z1,   NULL, 300000, '2025-07-01', '2026-06-30'),
    ('60860000-0000-4000-8000-000000000005', v_s, NULL, rt3,  600000, '2026-07-01', NULL)
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_allocations (id, school_id, student_user_id, route_id, stop_id, start_date, end_date, end_reason, created_by, created_at, updated_at) VALUES
    ('60870000-0000-4000-8000-000000000001', v_s, v_budi, rt2,      '60810000-0000-4000-8000-000000000005', '2026-07-13', '2026-08-31', 'Pindah rumah ke Bekasi Selatan', v_admin, '2026-07-10 09:00+07', '2026-08-28 09:00+07'),
    ('60870000-0000-4000-8000-000000000002', v_s, v_budi, v_route1, '60810000-0000-4000-8000-000000000002', '2026-09-01', NULL, NULL, v_admin, '2026-08-28 09:00+07', '2026-08-28 09:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_trips (id, school_id, route_id, trip_date, direction, vehicle_id, driver_crew_id, attendant_crew_id, status, started_at, finished_at, created_at, updated_at) VALUES
    ('60880000-0000-4000-8000-000000000001', v_s, v_route1, '2026-10-13', 'jemput', v_veh1, v_crew_bambang, cr_admin, 'done', '2026-10-13 06:00+07', '2026-10-13 07:05+07', '2026-10-13 05:30+07', '2026-10-13 07:05+07'),
    ('60880000-0000-4000-8000-000000000002', v_s, v_route1, '2026-10-13', 'antar',  v_veh1, v_crew_bambang, cr_admin, 'done', '2026-10-13 15:00+07', '2026-10-13 15:50+07', '2026-10-13 05:30+07', '2026-10-13 15:50+07'),
    ('60880000-0000-4000-8000-000000000003', v_s, v_route1, '2026-10-14', 'jemput', v_veh1, v_crew_bambang, cr_admin, 'done', '2026-10-14 06:00+07', '2026-10-14 07:00+07', '2026-10-14 05:30+07', '2026-10-14 07:00+07'),
    ('60880000-0000-4000-8000-000000000004', v_s, v_route1, '2026-10-14', 'antar',  v_veh1, v_crew_bambang, cr_admin, 'done', '2026-10-14 15:00+07', '2026-10-14 15:45+07', '2026-10-14 05:30+07', '2026-10-14 15:45+07'),
    ('60880000-0000-4000-8000-000000000005', v_s, v_route1, '2026-10-15', 'jemput', v_veh1, v_crew_bambang, cr_admin, 'done', '2026-10-15 06:00+07', '2026-10-15 07:02+07', '2026-10-15 05:30+07', '2026-10-15 07:02+07'),
    ('60880000-0000-4000-8000-000000000006', v_s, v_route1, '2026-10-15', 'antar',  v_veh1, v_crew_bambang, cr_admin, 'planned', NULL, NULL, '2026-10-15 05:30+07', '2026-10-15 05:30+07'),
    ('60880000-0000-4000-8000-000000000007', v_s, rt2,      '2026-10-14', 'jemput', veh2,   cr_sugeng,      cr_rina,  'done', '2026-10-14 05:45+07', '2026-10-14 06:55+07', '2026-10-14 05:30+07', '2026-10-14 06:55+07'),
    ('60880000-0000-4000-8000-000000000008', v_s, rt2,      '2026-10-14', 'antar',  veh2,   cr_sugeng,      cr_rina,  'done', '2026-10-14 15:00+07', '2026-10-14 16:05+07', '2026-10-14 05:30+07', '2026-10-14 16:05+07'),
    ('60880000-0000-4000-8000-000000000009', v_s, rt2,      '2026-10-15', 'jemput', veh2,   cr_sugeng,      cr_rina,  'running', '2026-10-15 05:45+07', NULL, '2026-10-15 05:30+07', '2026-10-15 05:45+07'),
    ('60880000-0000-4000-8000-00000000000a', v_s, rt2,      '2026-10-15', 'antar',  veh2,   cr_sugeng,      cr_rina,  'planned', NULL, NULL, '2026-10-15 05:30+07', '2026-10-15 05:30+07')
  ON CONFLICT DO NOTHING;

  -- Event naik/turun hanya untuk perjalanan milik seed ini (bila aplikasi sudah
  -- lebih dulu membuat perjalanan rute/tanggal yang sama, event dilewati).
  INSERT INTO transport_trip_events (id, school_id, trip_id, student_user_id, stop_id, event, source, recorded_by, recorded_at, client_event_id, created_at)
  SELECT v.id::uuid, v_s, v.trip_id::uuid, v_budi, v.stop_id::uuid, v.event, v.source, v_admin, v.at::timestamptz, v.cid, v.at::timestamptz
  FROM (VALUES
    ('60890000-0000-4000-8000-000000000001', '60880000-0000-4000-8000-000000000001', '60810000-0000-4000-8000-000000000002', 'board',  'scan',   '2026-10-13 06:26+07', 'seed-ops-te-01'),
    ('60890000-0000-4000-8000-000000000002', '60880000-0000-4000-8000-000000000001', NULL,                                   'alight', 'scan',   '2026-10-13 07:04+07', 'seed-ops-te-02'),
    ('60890000-0000-4000-8000-000000000003', '60880000-0000-4000-8000-000000000002', NULL,                                   'board',  'scan',   '2026-10-13 15:03+07', 'seed-ops-te-03'),
    ('60890000-0000-4000-8000-000000000004', '60880000-0000-4000-8000-000000000002', '60810000-0000-4000-8000-000000000002', 'alight', 'manual', '2026-10-13 15:27+07', 'seed-ops-te-04'),
    ('60890000-0000-4000-8000-000000000005', '60880000-0000-4000-8000-000000000003', '60810000-0000-4000-8000-000000000002', 'absent', 'manual', '2026-10-14 06:25+07', 'seed-ops-te-05'),
    ('60890000-0000-4000-8000-000000000006', '60880000-0000-4000-8000-000000000004', NULL,                                   'absent', 'manual', '2026-10-14 15:00+07', 'seed-ops-te-06'),
    ('60890000-0000-4000-8000-000000000007', '60880000-0000-4000-8000-000000000005', '60810000-0000-4000-8000-000000000002', 'board',  'scan',   '2026-10-15 06:27+07', 'seed-ops-te-07'),
    ('60890000-0000-4000-8000-000000000008', '60880000-0000-4000-8000-000000000005', NULL,                                   'alight', 'scan',   '2026-10-15 07:01+07', 'seed-ops-te-08')
  ) AS v(id, trip_id, stop_id, event, source, at, cid)
  WHERE EXISTS (SELECT 1 FROM transport_trips t WHERE t.id = v.trip_id::uuid)
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_absence_notices (id, school_id, student_user_id, notice_date, direction, reason, created_by, cancelled_at, created_at, updated_at) VALUES
    ('608a0000-0000-4000-8000-000000000001', v_s, v_budi, '2026-10-14', 'both',   'Sakit demam, istirahat di rumah', v_ortu, NULL, '2026-10-13 20:00+07', '2026-10-13 20:00+07'),
    ('608a0000-0000-4000-8000-000000000002', v_s, v_budi, '2026-10-20', 'antar',  'Latihan pramuka, dijemput orang tua', v_ortu, NULL, '2026-10-15 07:30+07', '2026-10-15 07:30+07'),
    ('608a0000-0000-4000-8000-000000000003', v_s, v_budi, '2026-10-09', 'jemput', 'Diantar sendiri ke sekolah', v_ortu, '2026-10-08 21:00+07', '2026-10-08 19:00+07', '2026-10-08 21:00+07')
  ON CONFLICT DO NOTHING;

  INSERT INTO transport_charges (id, school_id, student_user_id, period_year, period_month, items, amount_idr, version, bill_id, status, last_error, created_at, updated_at) VALUES
    ('608b0000-0000-4000-8000-000000000001', v_s, v_budi, 2026, 7,
     '[{"code":"TRANSPORT","name":"Antar-jemput Rute 02 (prorata 13-31 Juli 2026)","amount_idr":350000,"days":14,"rate_idr":25000}]'::jsonb,
     350000, 1, NULL, 'issued', NULL, '2026-07-01 01:00+07', '2026-07-01 01:00+07'),
    ('608b0000-0000-4000-8000-000000000002', v_s, v_budi, 2026, 8,
     '[{"code":"TRANSPORT","name":"Antar-jemput Rute 02 Agustus 2026","amount_idr":550000},{"code":"TRANSPORT","name":"Potongan libur 17 Agustus","amount_idr":-25000,"days":1,"rate_idr":25000}]'::jsonb,
     525000, 2, NULL, 'adjusted', NULL, '2026-08-01 01:00+07', '2026-08-20 09:00+07'),
    ('608b0000-0000-4000-8000-000000000003', v_s, v_budi, 2026, 9,
     '[{"code":"TRANSPORT","name":"Antar-jemput Rute 01 September 2026","amount_idr":450000}]'::jsonb,
     450000, 1, NULL, 'issued', NULL, '2026-09-01 01:00+07', '2026-09-01 01:00+07'),
    ('608b0000-0000-4000-8000-000000000004', v_s, v_budi, 2026, 10,
     '[{"code":"TRANSPORT","name":"Antar-jemput Rute 01 Oktober 2026","amount_idr":450000}]'::jsonb,
     450000, 1, NULL, 'issued', NULL, '2026-10-01 01:00+07', '2026-10-01 01:00+07')
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 10. KAMPUS 3D: node lantai, node ruang tambahan, lokasi tenant kantin
  ---------------------------------------------------------------------------
  INSERT INTO campus3d_floor_nodes (id, school_id, model_id, floor_id, node_name) VALUES
    ('60900000-0000-4000-8000-000000000001', v_s, v_model_a1, v_fl_a1, 'Node_Lantai1'),
    ('60900000-0000-4000-8000-000000000002', v_s, v_model_a1, v_fl_a2, 'Node_Lantai2')
  ON CONFLICT DO NOTHING;

  INSERT INTO campus3d_room_nodes (id, school_id, model_id, room_id, node_name)
  VALUES ('60930000-0000-4000-8000-000000000001', v_s, v_model_a1, v_room_labkom, 'Node_LabKomputer1')
  ON CONFLICT DO NOTHING;

  -- tenant_id = ID tenant kantin di teratai_billing (canteen_tenants). Seed billing
  -- perlu memakai ID yang sama agar pin tenant tampil di Peta Kantin.
  INSERT INTO campus3d_tenant_spots (id, school_id, tenant_id, room_id) VALUES
    ('60910000-0000-4000-8000-000000000001', v_s, '60920000-0000-4000-8000-000000000001', r_kantin),
    ('60910000-0000-4000-8000-000000000002', v_s, '60920000-0000-4000-8000-000000000002', r_kantin)
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 11. Nomor dokumen: urutan dimulai setelah nomor yang dipakai seed ini
  ---------------------------------------------------------------------------
  INSERT INTO doc_sequences (school_id, prefix, year, last) VALUES
    (v_s, 'WO', 2026, 7),
    (v_s, 'PR', 2026, 7),
    (v_s, 'PO', 2026, 7),
    (v_s, 'GR', 2026, 3),
    (v_s, 'SO', 2026, 2)
  ON CONFLICT DO NOTHING;

  ---------------------------------------------------------------------------
  -- 12. Verifikasi: semua master ber-ID tetap benar-benar ada. Bila salah satu
  --     dilewati karena bentrok kode/nama dengan data lain, batalkan semuanya.
  ---------------------------------------------------------------------------
  FOR chk IN SELECT * FROM (VALUES
      ('rooms', r_labipa), ('rooms', r_kls7a), ('rooms', r_lapang), ('rooms', r_rapat), ('rooms', r_uks), ('rooms', r_kantin),
      ('item_categories', ic_kbr), ('item_categories', ic_lab), ('item_categories', ic_tik),
      ('items', it_tinta), ('items', it_map), ('items', it_sabun), ('items', it_tisu), ('items', it_alko), ('items', it_laptop), ('items', it_spmrh), ('items', it_kapur),
      ('warehouses', wh_lab),
      ('vendors', vd_gram), ('vendors', vd_komp), ('vendors', vd_bers), ('vendors', vd_labo), ('vendors', vd_mebl), ('vendors', vd_elek),
      ('asset_categories', ac_mbl),
      ('budget_lines', bl_atk), ('budget_lines', bl_lab), ('budget_lines', bl_tik), ('budget_lines', bl_pml), ('budget_lines', bl_hnr), ('budget_lines', bl_kbr),
      ('lib_member_categories', mc_gtk), ('lib_member_categories', mc_osn),
      ('lib_members', m_guru), ('lib_members', m_wali), ('lib_members', m_ks), ('lib_members', m_adm), ('lib_members', m_yys),
      ('lib_collection_categories', lc_fik), ('lib_collection_categories', lc_ref), ('lib_collection_categories', lc_sai),
      ('lib_copies', '60750000-0000-4000-8000-000000000001'::uuid), ('lib_loans', '60760000-0000-4000-8000-000000000001'::uuid),
      ('transport_zones', z1), ('transport_zones', z2), ('transport_zones', z3),
      ('transport_routes', rt2), ('transport_routes', rt3),
      ('transport_vehicles', veh2), ('transport_vehicles', veh3),
      ('transport_crew', cr_sugeng), ('transport_crew', cr_rina), ('transport_crew', cr_admin), ('transport_crew', cr_joko),
      ('assets', '60310000-0000-4000-8000-000000000001'::uuid), ('work_orders', '60070000-0000-4000-8000-000000000001'::uuid),
      ('purchase_orders', '60260000-0000-4000-8000-000000000001'::uuid), ('vendor_invoices', '60600000-0000-4000-8000-000000000001'::uuid)
    ) AS t(tbl, id)
  LOOP
    EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I WHERE id = $1)', chk.tbl) INTO ok USING chk.id;
    IF NOT ok THEN
      RAISE EXCEPTION '60_ops_features: baris % di tabel % tidak ada (bentrok kunci unik dengan data lain?)', chk.id, chk.tbl;
    END IF;
  END LOOP;
END $$;

COMMIT;
