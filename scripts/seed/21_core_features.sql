-- 21_core_features.sql
-- Data demo core: alur approval untuk semua request type, permintaan approval
-- (pending/disetujui/ditolak/dibatalkan), delegasi, masa akses auditor,
-- antrean galat integrasi, panduan & FAQ, permintaan layanan + riwayatnya,
-- service desk (jam layanan, hari libur, SLA, eskalasi, tiket, timeline).
--
-- Aturan: hanya MENAMBAH (UUID tetap berawalan 'a21', ON CONFLICT DO NOTHING,
-- WHERE NOT EXISTS). Alur approval dibuat hanya bila sekolah belum punya alur
-- aktif untuk request type itu. Satu transaksi, idempoten.
--
-- Konsistensi lintas layanan:
--   * approval a2100003-..001/002 <-> identity role_conflict_exceptions a2000011-..001/002
--   * approval a2100003-..003 <-> admission selection_approvals a2300013-..001
--   * approval a2100003-..004 <-> admission manual_discounts a2300008-..001
--   * approval a2100003-..016 <-> admission manual_discounts a2300008-..002 (disetujui)
--   * approval a2100003-..017 <-> admission selection_approvals a2300013-..002 (disetujui)
--   * approval a2100003-..011 <-> quality qa_self_evaluations a2500003-..001
--   * approval a2100003-..012 <-> quality regulatory_reports a2500011-..002
--   * auditor demo a2000000-..0a1 dibuat di 20_identity_features.sql
-- Permintaan yang sudah diputus memakai callback_status='done' supaya job
-- callback tidak memanggil layanan sumber.

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. Alur approval per request type (hanya bila belum ada untuk sekolah itu)
-- ---------------------------------------------------------------------------
CREATE TEMP TABLE seed_a21_flows (id uuid, request_type text, name text) ON COMMIT DROP;
INSERT INTO seed_a21_flows VALUES
  ('a2100001-0000-0000-0000-000000000001', 'discount_special',         'Alur Diskon Khusus PPDB'),
  ('a2100001-0000-0000-0000-000000000002', 'fee_change',               'Alur Perubahan Tarif Biaya Sekolah'),
  ('a2100001-0000-0000-0000-000000000003', 'bill_adjustment',          'Alur Penyesuaian Tagihan Siswa'),
  ('a2100001-0000-0000-0000-000000000004', 'payment_correction',       'Alur Koreksi Pembayaran'),
  ('a2100001-0000-0000-0000-000000000005', 'scholarship',              'Alur Beasiswa Siswa'),
  ('a2100001-0000-0000-0000-000000000006', 'refund',                   'Alur Refund Pembayaran'),
  ('a2100001-0000-0000-0000-000000000007', 'journal',                  'Alur Penyesuaian Jurnal'),
  ('a2100001-0000-0000-0000-000000000008', 'recon_adjustment',         'Alur Penyesuaian Rekonsiliasi Bank'),
  ('a2100001-0000-0000-0000-000000000009', 'selection_result',         'Alur Penetapan Hasil Seleksi PPDB'),
  ('a2100001-0000-0000-0000-000000000010', 'student_transfer',         'Alur Mutasi Siswa'),
  ('a2100001-0000-0000-0000-000000000011', 'student_withdrawal',       'Alur Pengunduran Diri Siswa'),
  ('a2100001-0000-0000-0000-000000000012', 'leave',                    'Alur Persetujuan Cuti Pegawai'),
  ('a2100001-0000-0000-0000-000000000013', 'overtime',                 'Alur Persetujuan Lembur'),
  ('a2100001-0000-0000-0000-000000000014', 'attendance_correction',    'Alur Koreksi Presensi Pegawai'),
  ('a2100001-0000-0000-0000-000000000015', 'profile_change',           'Alur Perubahan Profil Pegawai'),
  ('a2100001-0000-0000-0000-000000000016', 'requisition',              'Alur Permintaan Tenaga Kerja'),
  ('a2100001-0000-0000-0000-000000000017', 'offer',                    'Alur Penawaran Kerja'),
  ('a2100001-0000-0000-0000-000000000018', 'payroll_component_change', 'Alur Perubahan Komponen Gaji'),
  ('a2100001-0000-0000-0000-000000000019', 'payroll_run',              'Alur Finalisasi Payroll'),
  ('a2100001-0000-0000-0000-000000000020', 'payroll_input_change',     'Alur Perubahan Input Payroll'),
  ('a2100001-0000-0000-0000-000000000021', 'training_paid',            'Alur Pelatihan Berbayar'),
  ('a2100001-0000-0000-0000-000000000022', 'room_booking',             'Alur Peminjaman Ruang'),
  ('a2100001-0000-0000-0000-000000000023', 'health_profile_update',    'Alur Pembaruan Profil Kesehatan Siswa'),
  ('a2100001-0000-0000-0000-000000000024', 'canteen_settlement',       'Alur Pencairan Dana Kantin'),
  ('a2100001-0000-0000-0000-000000000025', 'wallet_transfer',          'Alur Transfer Saldo Dompet'),
  ('a2100001-0000-0000-0000-000000000026', 'purchase_request',         'Alur Permintaan Pembelian (PR)'),
  ('a2100001-0000-0000-0000-000000000027', 'purchase_order',           'Alur Pesanan Pembelian (PO)'),
  ('a2100001-0000-0000-0000-000000000028', 'stock_adjustment',         'Alur Penyesuaian Stok Opname'),
  ('a2100001-0000-0000-0000-000000000029', 'vendor_bank_change',       'Alur Perubahan Rekening Vendor'),
  ('a2100001-0000-0000-0000-000000000030', 'discipline_appeal',        'Alur Banding Disiplin Siswa'),
  ('a2100001-0000-0000-0000-000000000031', 'event',                    'Alur Kegiatan/Event Sekolah'),
  ('a2100001-0000-0000-0000-000000000032', 'asset_disposal',           'Alur Pelepasan Aset'),
  ('a2100001-0000-0000-0000-000000000033', 'asset_transfer',           'Alur Mutasi Aset Antar-Unit'),
  ('a2100001-0000-0000-0000-000000000034', 'budget',                   'Alur Anggaran / RAPBS'),
  ('a2100001-0000-0000-0000-000000000035', 'vendor_invoice_override',  'Alur Dispensasi Faktur Vendor'),
  ('a2100001-0000-0000-0000-000000000036', 'vendor_payment',           'Alur Jadwal Pembayaran Vendor'),
  ('a2100001-0000-0000-0000-000000000037', 'qa_self_eval_finalize',    'Alur Finalisasi Evaluasi Diri Sekolah'),
  ('a2100001-0000-0000-0000-000000000038', 'qa_self_eval_reopen',      'Alur Pembukaan Kembali Evaluasi Diri'),
  ('a2100001-0000-0000-0000-000000000039', 'regulatory_report',        'Alur Persetujuan Laporan Regulasi'),
  ('a2100001-0000-0000-0000-000000000040', 'role_conflict_exception',  'Alur Pengecualian Konflik Peran (SoD)'),
  ('a2100001-0000-0000-0000-000000000041', 'discount_award',           'Alur Pemberian Program Diskon'),
  ('a2100001-0000-0000-0000-000000000042', 'procurement',              'Alur Persetujuan Pengadaan Barang Sekolah');

-- Tahap: seq, peran penyetuju, SLA (jam), batas nominal minimum, peran eskalasi.
CREATE TEMP TABLE seed_a21_flow_steps (request_type text, seq int, role text, sla int, min_amount bigint, escalate text) ON COMMIT DROP;
INSERT INTO seed_a21_flow_steps VALUES
  ('discount_special', 1, 'admin', 24, NULL, NULL), ('discount_special', 2, 'kepala_sekolah', 48, NULL, NULL),
  ('fee_change', 1, 'finance_manager', 48, NULL, NULL), ('fee_change', 2, 'kepala_yayasan', 72, NULL, NULL),
  ('bill_adjustment', 1, 'finance_manager', 24, NULL, 'kepala_sekolah'),
  ('payment_correction', 1, 'finance_manager', 24, NULL, 'kepala_sekolah'),
  ('scholarship', 1, 'kepala_sekolah', 48, NULL, NULL), ('scholarship', 2, 'kepala_yayasan', 72, NULL, NULL),
  ('refund', 1, 'finance_manager', 24, NULL, NULL), ('refund', 2, 'kepala_sekolah', 48, 5000000, NULL),
  ('journal', 1, 'finance_manager', 48, NULL, NULL),
  ('recon_adjustment', 1, 'finance_manager', 24, NULL, NULL),
  ('selection_result', 1, 'kepala_sekolah', 48, NULL, NULL),
  ('student_transfer', 1, 'admin', 24, NULL, NULL), ('student_transfer', 2, 'kepala_sekolah', 48, NULL, NULL),
  ('student_withdrawal', 1, 'kepala_sekolah', 48, NULL, NULL),
  ('leave', 1, 'hr_manager', 24, NULL, NULL),
  ('overtime', 1, 'hr_manager', 24, NULL, NULL),
  ('attendance_correction', 1, 'hr_admin', 24, NULL, NULL),
  ('profile_change', 1, 'hr_admin', 48, NULL, NULL),
  ('requisition', 1, 'hr_manager', 48, NULL, NULL), ('requisition', 2, 'kepala_yayasan', 72, NULL, NULL),
  ('offer', 1, 'hr_manager', 48, NULL, NULL), ('offer', 2, 'kepala_sekolah', 48, NULL, NULL),
  ('payroll_component_change', 1, 'hr_manager', 48, NULL, NULL), ('payroll_component_change', 2, 'finance_manager', 48, NULL, NULL),
  ('payroll_run', 1, 'finance_manager', 24, NULL, NULL), ('payroll_run', 2, 'kepala_yayasan', 48, NULL, NULL),
  ('payroll_input_change', 1, 'hr_manager', 24, NULL, NULL),
  ('training_paid', 1, 'hr_manager', 48, NULL, NULL),
  ('room_booking', 1, 'facility_officer', 24, NULL, NULL),
  ('health_profile_update', 1, 'petugas_uks', 48, NULL, NULL),
  ('canteen_settlement', 1, 'finance_manager', 24, NULL, NULL),
  ('wallet_transfer', 1, 'finance_manager', 24, NULL, NULL),
  ('purchase_request', 1, 'kepala_sekolah', 48, NULL, NULL),
  ('purchase_order', 1, 'finance_manager', 48, NULL, NULL), ('purchase_order', 2, 'kepala_yayasan', 72, 25000000, NULL),
  ('stock_adjustment', 1, 'kepala_sekolah', 48, NULL, NULL),
  ('vendor_bank_change', 1, 'finance_manager', 24, NULL, NULL),
  ('discipline_appeal', 1, 'kepala_sekolah', 72, NULL, NULL),
  ('event', 1, 'kepala_sekolah', 72, NULL, NULL),
  ('asset_disposal', 1, 'kepala_sekolah', 72, NULL, NULL), ('asset_disposal', 2, 'kepala_yayasan', 72, NULL, NULL),
  ('asset_transfer', 1, 'kepala_yayasan', 72, NULL, NULL),
  ('budget', 1, 'kepala_sekolah', 72, NULL, NULL), ('budget', 2, 'kepala_yayasan', 120, NULL, NULL),
  ('vendor_invoice_override', 1, 'finance_manager', 24, NULL, NULL),
  ('vendor_payment', 1, 'finance_manager', 24, NULL, NULL),
  ('qa_self_eval_finalize', 1, 'kepala_sekolah', 72, NULL, NULL),
  ('qa_self_eval_reopen', 1, 'kepala_sekolah', 72, NULL, NULL),
  ('regulatory_report', 1, 'kepala_sekolah', 48, NULL, NULL),
  ('role_conflict_exception', 1, 'kepala_sekolah', 48, NULL, NULL), ('role_conflict_exception', 2, 'kepala_yayasan', 72, NULL, NULL),
  ('discount_award', 1, 'kepala_sekolah', 48, NULL, NULL),
  ('procurement', 1, 'admin', 24, NULL, NULL), ('procurement', 2, 'kepala_sekolah', 48, NULL, NULL);

INSERT INTO approval_flows (id, school_id, request_type, name, active, created_by, created_at, updated_at)
SELECT f.id, '11111111-1111-1111-1111-111111111111', f.request_type, f.name, true,
       '22222222-2222-2222-2222-222222222222', now() - interval '120 days', now() - interval '120 days'
FROM seed_a21_flows f
WHERE NOT EXISTS (SELECT 1 FROM approval_flows x
                  WHERE x.school_id = '11111111-1111-1111-1111-111111111111'
                    AND x.request_type = f.request_type AND x.deleted_at IS NULL)
ON CONFLICT DO NOTHING;

-- Tahap hanya untuk alur yang dibuat seeder ini.
INSERT INTO approval_flow_steps (id, school_id, flow_id, seq, approver_kind, approver_role, min_amount_idr, sla_hours, escalate_to_role)
SELECT md5('a21-step:' || s.request_type || ':' || s.seq)::uuid, '11111111-1111-1111-1111-111111111111', f.id, s.seq, 'role',
       s.role, s.min_amount, s.sla, s.escalate
FROM seed_a21_flow_steps s
JOIN seed_a21_flows f ON f.request_type = s.request_type
JOIN approval_flows af ON af.id = f.id
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2. Permintaan approval dengan berbagai status
-- ---------------------------------------------------------------------------
CREATE TEMP TABLE seed_a21_requests (
  id uuid, request_type text, source_service text, source_id text, title text, summary jsonb, amount bigint,
  maker uuid, subject uuid, status text, current_seq int, callback_url text, created_at timestamptz, decided_at timestamptz
) ON COMMIT DROP;
INSERT INTO seed_a21_requests VALUES
  ('a2100003-0000-0000-0000-000000000001', 'role_conflict_exception', 'identity', 'a2000011-0000-0000-0000-000000000001',
   'Pengecualian SoD: HR Manager Teratai mendapat peran payroll_officer',
   '{"user":"HR Manager Teratai","role":"payroll_officer","conflicting_roles":["hr_manager"],"reason":"Petugas payroll cuti melahirkan tiga bulan"}',
   NULL, '22222222-2222-2222-2222-222222222222', NULL, 'pending', 1,
   'http://identity:8081/internal/role-conflict-exceptions/approval-callback', now() - interval '1 day', NULL),
  ('a2100003-0000-0000-0000-000000000002', 'role_conflict_exception', 'identity', 'a2000011-0000-0000-0000-000000000002',
   'Pengecualian SoD: Petugas Payroll Teratai mendapat peran finance_manager',
   '{"user":"Petugas Payroll Teratai","role":"finance_manager","conflicting_roles":["payroll_officer"],"reason":"Merangkap penyetuju pembayaran selama audit tahunan"}',
   NULL, '22222222-2222-2222-2222-222222222222', NULL, 'rejected', NULL,
   'http://identity:8081/internal/role-conflict-exceptions/approval-callback', now() - interval '27 days', now() - interval '25 days'),
  ('a2100003-0000-0000-0000-000000000003', 'selection_result', 'admission', 'a2300013-0000-0000-0000-000000000001',
   'Penetapan Hasil Seleksi PPDB Gelombang 2 - Jalur Reguler',
   '{"wave":"Gelombang 2","track":"Reguler","accepted":3,"waitlisted":1,"rejected":1}',
   NULL, '22222222-2222-2222-2222-222222222222', NULL, 'pending', 1,
   'http://admission:8088/internal/approval-callbacks', now() - interval '2 days', NULL),
  ('a2100003-0000-0000-0000-000000000004', 'discount_special', 'admission', 'a2300008-0000-0000-0000-000000000001',
   'Diskon Khusus Uang Pangkal - Anak Guru (Nadia Putri)',
   '{"applicant":"Nadia Putri","discount":"Anak pegawai","percent":50,"fee_item":"Uang Pangkal"}',
   7500000, '22222222-2222-2222-2222-222222222222', NULL, 'pending', 2,
   'http://admission:8088/internal/approval-callbacks', now() - interval '3 days', NULL),
  ('a2100003-0000-0000-0000-000000000005', 'refund', 'billing', 'REF-2026-0012',
   'Refund Biaya Study Tour Kelas 8 yang Dibatalkan',
   '{"student":"Budi Santoso","reason":"Study tour dibatalkan karena cuaca buruk","method":"transfer bank"}',
   1250000, '22222222-2222-2222-2222-222222222222', '44444444-4444-4444-4444-444444444444', 'approved', NULL,
   'http://billing:8090/internal/approval-callbacks', now() - interval '12 days', now() - interval '11 days'),
  ('a2100003-0000-0000-0000-000000000006', 'student_transfer', 'academic', 'TRF-2026-0004',
   'Mutasi Keluar: Andi Wijaya ke SMP Negeri 5 Bandung',
   '{"student":"Andi Wijaya","class":"Kelas 8A","destination":"SMP Negeri 5 Bandung","reason":"Orang tua pindah tugas"}',
   NULL, '66666666-6666-6666-6666-666666666666', NULL, 'approved', NULL,
   'http://academic:8082/internal/approval-callbacks', now() - interval '20 hours', now() - interval '1 hour'),
  ('a2100003-0000-0000-0000-000000000007', 'student_withdrawal', 'academic', 'WDR-2026-0002',
   'Pengunduran Diri: Rina Marlina (Kelas 9A)',
   '{"student":"Rina Marlina","class":"Kelas 9A","reason":"Melanjutkan homeschooling"}',
   NULL, '66666666-6666-6666-6666-666666666666', NULL, 'rejected', NULL,
   'http://academic:8082/internal/approval-callbacks', now() - interval '35 days', now() - interval '33 days'),
  ('a2100003-0000-0000-0000-000000000008', 'fee_change', 'billing', 'SCH-2026-SPP',
   'Perubahan Tarif SPP 2026/2027 menjadi Rp850.000',
   '{"scheme":"SPP Reguler 2026/2027","old_amount":800000,"new_amount":850000}',
   850000, '22222222-2222-2222-2222-222222222222', NULL, 'approved', NULL,
   'http://billing:8090/internal/approval-callbacks', now() - interval '100 days', now() - interval '96 days'),
  ('a2100003-0000-0000-0000-000000000009', 'payment_correction', 'billing', 'PAYCOR-2026-0007',
   'Koreksi Pembayaran Ganda Virtual Account September',
   '{"student":"Budi Santoso","invoice":"INV-2026-09-0412","duplicate_amount":850000}',
   850000, '22222222-2222-2222-2222-222222222222', '44444444-4444-4444-4444-444444444444', 'approved', NULL,
   'http://billing:8090/internal/approval-callbacks', now() - interval '6 hours', now() - interval '1 hour'),
  ('a2100003-0000-0000-0000-000000000010', 'bill_adjustment', 'billing', 'BADJ-2026-0003',
   'Penyesuaian Tagihan Kegiatan Ekstrakurikuler Robotik',
   '{"student":"Budi Santoso","item":"Ekskul Robotik","old_amount":350000,"new_amount":250000}',
   100000, '22222222-2222-2222-2222-222222222222', '44444444-4444-4444-4444-444444444444', 'cancelled', NULL,
   'http://billing:8090/internal/approval-callbacks', now() - interval '15 days', now() - interval '14 days'),
  ('a2100003-0000-0000-0000-000000000011', 'qa_self_eval_finalize', 'quality', 'a2500003-0000-0000-0000-000000000001',
   'Finalisasi Evaluasi Tengah Semester Ganjil 2026/2027',
   '{"period":"Evaluasi Tengah Semester Ganjil 2026/2027","instrument":"Instrumen EDS SPMI v2026.1","score":83.3}',
   NULL, '22222222-2222-2222-2222-222222222222', NULL, 'pending', 1,
   'http://quality:8095/internal/approval-callbacks', now() - interval '4 days', NULL),
  ('a2100003-0000-0000-0000-000000000012', 'regulatory_report', 'quality', 'a2500011-0000-0000-0000-000000000002',
   'Persetujuan Laporan Rekap Peserta Didik Semester Ganjil',
   '{"template":"Rekap Peserta Didik per Rombel","period_from":"2026-07-01","period_to":"2026-09-30"}',
   NULL, '22222222-2222-2222-2222-222222222222', NULL, 'pending', 1,
   'http://quality:8095/internal/approval-callbacks', now() - interval '2 days', NULL),
  ('a2100003-0000-0000-0000-000000000013', 'leave', 'hr', 'LV-2026-0031',
   'Cuti Tahunan Guru Matematika 3 Hari',
   '{"employee":"Guru Matematika","leave_type":"Cuti Tahunan","from":"2026-09-21","to":"2026-09-23","days":3}',
   NULL, '55555555-5555-5555-5555-555555555555', '55555555-5555-5555-5555-555555555555', 'approved', NULL,
   'http://hr:8092/internal/approval-callbacks', now() - interval '30 days', now() - interval '29 days'),
  ('a2100003-0000-0000-0000-000000000014', 'recon_adjustment', 'billing', 'RECON-2026-09-02',
   'Penyesuaian Rekonsiliasi: Biaya Admin Bank September',
   '{"bank":"Bank Mandiri","statement_date":"2026-09-30","difference":6500}',
   6500, '22222222-2222-2222-2222-222222222222', NULL, 'approved', NULL,
   'http://billing:8090/internal/approval-callbacks', now() - interval '9 hours', now() - interval '1 hour'),
  ('a2100003-0000-0000-0000-000000000015', 'discount_award', 'billing', 'DAW-2026-0005',
   'Pemberian Diskon Saudara Kandung 10% untuk Budi Santoso',
   '{"student":"Budi Santoso","program":"Diskon Saudara Kandung","percent":10}',
   85000, '22222222-2222-2222-2222-222222222222', '44444444-4444-4444-4444-444444444444', 'approved', NULL,
   'http://billing:8090/internal/approval-callbacks', now() - interval '60 days', now() - interval '59 days'),
  ('a2100003-0000-0000-0000-000000000016', 'discount_special', 'admission', 'a2300008-0000-0000-0000-000000000002',
   'Diskon Saudara Kandung Uang Pangkal - Budi Santoso',
   '{"applicant":"Budi Santoso","discount":"Saudara kandung","amount":1000000,"fee_item":"Uang Pangkal"}',
   1000000, '22222222-2222-2222-2222-222222222222', NULL, 'approved', NULL,
   'http://admission:8088/internal/approval-callbacks', now() - interval '229 days', now() - interval '228 days'),
  ('a2100003-0000-0000-0000-000000000017', 'selection_result', 'admission', 'a2300013-0000-0000-0000-000000000002',
   'Penetapan Hasil Seleksi PPDB Gelombang 1 - SMP Kelas 7',
   '{"wave":"Gelombang 1","accepted":4,"waitlisted":1,"rejected":1}',
   NULL, '22222222-2222-2222-2222-222222222222', NULL, 'approved', NULL,
   'http://admission:8088/internal/approval-callbacks', now() - interval '232 days', now() - interval '231 days');

-- Tahap tiap permintaan: (request, seq, role, status, decided_by, comment, umur keputusan relatif created_at)
CREATE TEMP TABLE seed_a21_req_steps (request_id uuid, seq int, role text, status text, decided_by uuid, comment text, sla int) ON COMMIT DROP;
INSERT INTO seed_a21_req_steps VALUES
  ('a2100003-0000-0000-0000-000000000001', 1, 'kepala_sekolah', 'pending', NULL, NULL, 48),
  ('a2100003-0000-0000-0000-000000000001', 2, 'kepala_yayasan', 'waiting', NULL, NULL, 72),
  ('a2100003-0000-0000-0000-000000000002', 1, 'kepala_sekolah', 'rejected', '77777777-7777-7777-7777-777777777777',
   'Ditolak: pembuat dan penyetuju gaji wajib dipisah. Gunakan delegasi approval.', 48),
  ('a2100003-0000-0000-0000-000000000002', 2, 'kepala_yayasan', 'skipped', NULL, NULL, 72),
  ('a2100003-0000-0000-0000-000000000003', 1, 'kepala_sekolah', 'pending', NULL, NULL, 48),
  ('a2100003-0000-0000-0000-000000000004', 1, 'admin', 'approved', '22222222-2222-2222-2222-222222222222',
   'Dokumen SK pegawai sudah diverifikasi.', 24),
  ('a2100003-0000-0000-0000-000000000004', 2, 'kepala_sekolah', 'pending', NULL, NULL, 48),
  ('a2100003-0000-0000-0000-000000000005', 1, 'finance_manager', 'approved', '22222222-2222-2222-2222-222222222222',
   'Bukti pembatalan dari biro perjalanan terlampir.', 24),
  ('a2100003-0000-0000-0000-000000000006', 1, 'admin', 'approved', '22222222-2222-2222-2222-222222222222', 'Berkas mutasi lengkap.', 24),
  ('a2100003-0000-0000-0000-000000000006', 2, 'kepala_sekolah', 'approved', '77777777-7777-7777-7777-777777777777', 'Disetujui.', 48),
  ('a2100003-0000-0000-0000-000000000007', 1, 'kepala_sekolah', 'rejected', '77777777-7777-7777-7777-777777777777',
   'Mohon lampirkan surat pernyataan orang tua bermeterai sebelum diajukan ulang.', 48),
  ('a2100003-0000-0000-0000-000000000008', 1, 'finance_manager', 'approved', '22222222-2222-2222-2222-222222222222',
   'Sesuai hasil rapat komite sekolah.', 48),
  ('a2100003-0000-0000-0000-000000000008', 2, 'kepala_yayasan', 'approved', '88888888-8888-8888-8888-888888888888',
   'Disetujui yayasan.', 72),
  ('a2100003-0000-0000-0000-000000000009', 1, 'finance_manager', 'approved', '22222222-2222-2222-2222-222222222222', 'Pembayaran ganda terverifikasi di mutasi bank.', 24),
  ('a2100003-0000-0000-0000-000000000010', 1, 'finance_manager', 'skipped', NULL, NULL, 24),
  ('a2100003-0000-0000-0000-000000000011', 1, 'kepala_sekolah', 'pending', NULL, NULL, 72),
  ('a2100003-0000-0000-0000-000000000012', 1, 'kepala_sekolah', 'pending', NULL, NULL, 48),
  ('a2100003-0000-0000-0000-000000000013', 1, 'hr_manager', 'approved', '77777777-7777-7777-7777-777777777777',
   'Disetujui sebagai delegasi; jadwal mengajar sudah diatur guru pengganti.', 24),
  ('a2100003-0000-0000-0000-000000000014', 1, 'finance_manager', 'approved', '22222222-2222-2222-2222-222222222222', 'Sesuai rekening koran.', 24),
  ('a2100003-0000-0000-0000-000000000015', 1, 'kepala_sekolah', 'approved', '77777777-7777-7777-7777-777777777777',
   'Kakak kandung tercatat aktif di kelas 9A.', 48),
  ('a2100003-0000-0000-0000-000000000016', 1, 'admin', 'approved', '22222222-2222-2222-2222-222222222222',
   'KK menunjukkan kakak kandung aktif di kelas 9A.', 24),
  ('a2100003-0000-0000-0000-000000000016', 2, 'kepala_sekolah', 'approved', '77777777-7777-7777-7777-777777777777',
   'Kakak kandung tercatat aktif.', 48),
  ('a2100003-0000-0000-0000-000000000017', 1, 'kepala_sekolah', 'approved', '77777777-7777-7777-7777-777777777777',
   'Hasil seleksi sesuai rapat panitia.', 48);

INSERT INTO approval_requests (id, school_id, request_type, source_service, source_id, title, summary, amount_idr, flags,
                               maker_user_id, subject_user_id, status, current_seq, callback_url, correlation_id, idempotency_key,
                               flow_snapshot, decided_at, callback_status, callback_attempts, last_callback_at, created_at, updated_at)
SELECT r.id, '11111111-1111-1111-1111-111111111111', r.request_type, r.source_service, r.source_id, r.title, r.summary, r.amount, '{}',
       r.maker, r.subject, r.status, r.current_seq, r.callback_url, 'seed-a21-' || r.source_id, 'seed-a21:' || r.id,
       (SELECT jsonb_agg(jsonb_build_object('seq', s.seq, 'approver_kind', 'role', 'approver_role', s.role, 'sla_hours', s.sla) ORDER BY s.seq)
          FROM seed_a21_req_steps s WHERE s.request_id = r.id),
       r.decided_at,
       CASE WHEN r.status = 'pending' THEN 'pending' ELSE 'done' END,
       CASE WHEN r.status IN ('approved', 'rejected') THEN 1 ELSE 0 END,
       CASE WHEN r.status IN ('approved', 'rejected') THEN r.decided_at + interval '1 minute' END,
       r.created_at, COALESCE(r.decided_at, r.created_at)
FROM seed_a21_requests r
ON CONFLICT DO NOTHING;

INSERT INTO approval_steps (id, school_id, request_id, seq, approver_kind, approver_role, status, sla_hours, due_at,
                            decided_by, comment, decided_at, created_at, updated_at)
SELECT md5('a21-req-step:' || s.request_id || ':' || s.seq)::uuid, '11111111-1111-1111-1111-111111111111', s.request_id, s.seq,
       'role', s.role, s.status, s.sla,
       CASE WHEN s.status = 'pending' THEN r.created_at + make_interval(hours => s.sla) END,
       s.decided_by, s.comment,
       CASE WHEN s.decided_by IS NOT NULL THEN COALESCE(r.decided_at, r.created_at + interval '5 hours') END,
       r.created_at, COALESCE(r.decided_at, r.created_at)
FROM seed_a21_req_steps s
JOIN seed_a21_requests r ON r.id = s.request_id
JOIN approval_requests ar ON ar.id = s.request_id AND ar.idempotency_key = 'seed-a21:' || s.request_id
-- UNIQUE (request_id, seq) bersifat DEFERRABLE sehingga tidak bisa jadi arbiter ON CONFLICT.
WHERE NOT EXISTS (SELECT 1 FROM approval_steps x WHERE x.request_id = s.request_id AND x.seq = s.seq);

-- ---------------------------------------------------------------------------
-- 3. Delegasi approval (aktif, terjadwal, kedaluwarsa, dicabut)
-- ---------------------------------------------------------------------------
INSERT INTO approval_delegations (id, school_id, from_user_id, to_user_id, valid_from, valid_to, created_at, revoked_at)
VALUES
  ('a2100004-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777',
   '55555555-5555-5555-5555-555555555555', now() - interval '1 day', now() + interval '6 days', now() - interval '2 days', NULL),
  ('a2100004-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '88888888-8888-8888-8888-888888888888',
   '77777777-7777-7777-7777-777777777777', now() + interval '10 days', now() + interval '17 days', now() - interval '1 day', NULL),
  ('a2100004-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777',
   '66666666-6666-6666-6666-666666666666', now() - interval '45 days', now() - interval '38 days', now() - interval '46 days', NULL),
  ('a2100004-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222',
   '55555555-5555-5555-5555-555555555555', now() - interval '20 days', now() - interval '5 days', now() - interval '21 days', now() - interval '15 days')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 4. Masa akses auditor (aktif, berakhir, dicabut)
-- ---------------------------------------------------------------------------
INSERT INTO audit_access_grants (id, school_id, user_id, valid_from, valid_to, reason, granted_by, created_at, revoked_at, revoked_by)
VALUES
  ('a2100005-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'a2000000-0000-0000-0000-0000000000a1',
   now() - interval '3 days', now() + interval '27 days', 'Audit eksternal laporan keuangan tahun ajaran 2025/2026 oleh KAP.',
   '22222222-2222-2222-2222-222222222222', now() - interval '3 days', NULL, NULL),
  ('a2100005-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'a2000000-0000-0000-0000-0000000000a1',
   now() - interval '200 days', now() - interval '170 days', 'Audit interim semester genap 2025/2026.',
   '22222222-2222-2222-2222-222222222222', now() - interval '200 days', NULL, NULL),
  ('a2100005-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'a2000000-0000-0000-0000-0000000000a1',
   now() - interval '90 days', now() - interval '60 days', 'Pemeriksaan khusus dana BOS triwulan II.',
   '22222222-2222-2222-2222-222222222222', now() - interval '90 days', now() - interval '80 days', '22222222-2222-2222-2222-222222222222')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 5. Antrean galat integrasi (exception report). retry_url NULL agar seeder
--    tidak memicu panggilan ulang ke layanan sumber.
-- ---------------------------------------------------------------------------
INSERT INTO integration_exceptions (id, school_id, source_service, kind, severity, correlation_id, error_message, attempts, max_attempts,
                                    payload_summary, retry_url, status, assignee_user_id, resolved_by, resolution_reason, resolved_at,
                                    idempotency_key, created_at, updated_at)
VALUES
  ('a2100006-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'billing', 'webhook', 'critical',
   'seed-a21-exc-001', 'Signature webhook payment gateway tidak valid untuk VA 8808-0412', 3, 5,
   '{"provider":"midtrans","order_id":"INV-2026-09-0412","amount":850000}', NULL, 'open',
   '22222222-2222-2222-2222-222222222222', NULL, NULL, NULL, 'seed-a21:exc:001', now() - interval '5 hours', now() - interval '2 hours'),
  ('a2100006-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'hr', 'journal_posting', 'normal',
   'seed-a21-exc-002', 'Akun COA 5101 (Beban Gaji Guru) belum dipetakan untuk unit SMP', 2, 5,
   '{"payroll_period":"2026-09","journal":"JG-2026-09","lines":14}', NULL, 'retrying',
   NULL, NULL, NULL, NULL, 'seed-a21:exc:002', now() - interval '1 day', now() - interval '20 hours'),
  ('a2100006-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'notification', 'notification', 'normal',
   'seed-a21-exc-003', 'Pengiriman WhatsApp gagal: nomor tujuan tidak terdaftar', 5, 5,
   '{"channel":"whatsapp","template":"tagihan_jatuh_tempo","recipient":"wali"}', NULL, 'resolved',
   '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', 'Nomor HP wali diperbarui dan notifikasi dikirim ulang lewat email.',
   now() - interval '3 days', 'seed-a21:exc:003', now() - interval '4 days', now() - interval '3 days'),
  ('a2100006-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'identity', 'import', 'normal',
   'seed-a21-exc-004', 'Baris 37 impor orang tua: NISN anak tidak ditemukan', 1, 3,
   '{"file":"siakad_orang_tua_2026.csv","row":37,"kind":"orang_tua"}', NULL, 'open',
   NULL, NULL, NULL, NULL, 'seed-a21:exc:004', now() - interval '2 days', now() - interval '2 days'),
  ('a2100006-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'admission', 'callback', 'critical',
   'seed-a21-exc-005', 'Callback approval hasil seleksi gagal: layanan admission timeout', 5, 5,
   '{"request_type":"selection_result","approval":"selection_result Gelombang 1"}', NULL, 'resolved',
   '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', 'Callback dikirim ulang manual setelah layanan admission pulih.',
   now() - interval '40 days', 'seed-a21:exc:005', now() - interval '41 days', now() - interval '40 days'),
  ('a2100006-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111', 'ops', 'dead_letter', 'normal',
   'seed-a21-exc-006', 'Sinkronisasi vendor ke akuntansi masuk dead letter setelah 5 percobaan', 5, 5,
   '{"vendor":"PT Sarana Edukasi Mandiri","event":"vendor.updated"}', NULL, 'open',
   NULL, NULL, NULL, NULL, 'seed-a21:exc:006', now() - interval '8 hours', now() - interval '8 hours')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 6. Panduan & FAQ sekolah (siswa + orang tua), termasuk pengganti default.
-- ---------------------------------------------------------------------------
INSERT INTO help_articles (id, school_id, audience, kind, title, body, sort_order, published, replaces_id, created_by, updated_by, created_at, updated_at)
VALUES
  ('a2100007-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'siswa', 'guide', 'Cara mengikuti ujian CBT',
   '1. Masuk ke portal siswa sebelum sesi dimulai.
2. Buka menu Ujian, pilih ujian yang aktif, lalu masukkan token dari pengawas.
3. Jawaban tersimpan otomatis. Tekan Selesai bila semua soal sudah dijawab.', 10, true, NULL,
   '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', now() - interval '60 days', now() - interval '60 days'),
  ('a2100007-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'siswa', 'guide', 'Mengumpulkan tugas dari guru',
   'Buka menu Tugas, pilih tugas, unggah berkas jawaban (PDF/JPG maksimal 10 MB), lalu tekan Kumpulkan sebelum tenggat.', 20, true, NULL,
   '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', now() - interval '60 days', now() - interval '60 days'),
  ('a2100007-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'siswa', 'faq', 'Saya lupa password, apa yang harus dilakukan?',
   'Gunakan tautan Lupa Password di halaman masuk atau hubungi wali kelas untuk reset password.', 110, true, NULL,
   '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', now() - interval '60 days', now() - interval '60 days'),
  ('a2100007-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'orang_tua', 'faq', 'Kapan batas pembayaran SPP setiap bulan?',
   'SPP dibayarkan paling lambat tanggal 10 setiap bulan melalui virtual account yang tertera di menu Tagihan.', 140, true, NULL,
   '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', now() - interval '30 days', now() - interval '30 days'),
  ('a2100007-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'orang_tua', 'guide', 'Jadwal layanan tata usaha',
   'Draf: layanan tata usaha buka Senin-Jumat 07.00-15.00 WIB dan Sabtu 07.00-12.00 WIB.', 40, false, NULL,
   '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', now() - interval '2 days', now() - interval '2 days')
ON CONFLICT DO NOTHING;

-- Pengganti artikel default "Ke mana saya mengajukan pertanyaan..." untuk sekolah ini.
INSERT INTO help_articles (id, school_id, audience, kind, title, body, sort_order, published, replaces_id, created_by, updated_by, created_at, updated_at)
SELECT 'a2100007-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111', 'orang_tua', 'faq',
       'Ke mana saya mengajukan pertanyaan atau permohonan lain?',
       'Gunakan menu Layanan Sekolah untuk permohonan surat dan data. Untuk keadaan mendesak hubungi tata usaha Sekolah Teratai di (021) 555-0123 pada jam kerja.',
       130, true, d.id, '22222222-2222-2222-2222-222222222222', '22222222-2222-2222-2222-222222222222', now() - interval '30 days', now() - interval '30 days'
FROM help_articles d
WHERE d.school_id IS NULL AND d.title = 'Ke mana saya mengajukan pertanyaan atau permohonan lain?'
  AND NOT EXISTS (SELECT 1 FROM help_articles x WHERE x.school_id = '11111111-1111-1111-1111-111111111111' AND x.replaces_id = d.id)
LIMIT 1
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 7. Permintaan layanan (menu 81): kategori, permintaan, riwayat status
-- ---------------------------------------------------------------------------
INSERT INTO service_request_categories (id, school_id, name, description, handler_role, sla_hours, requires_student, active, created_at, updated_at)
VALUES
  ('a2100008-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Legalisir Rapor dan Ijazah',
   'Legalisir salinan rapor/ijazah untuk pendaftaran sekolah lanjutan', 'admin', 48, true, true, now() - interval '90 days', now() - interval '90 days'),
  ('a2100008-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'Izin Tidak Masuk Lebih dari 3 Hari',
   'Permohonan izin siswa untuk keperluan keluarga lebih dari tiga hari', 'walikelas', 24, true, true, now() - interval '90 days', now() - interval '90 days'),
  ('a2100008-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'Perubahan Data Orang Tua',
   'Pembaruan alamat, nomor HP, atau pekerjaan orang tua', 'admin', 72, false, true, now() - interval '90 days', now() - interval '90 days'),
  ('a2100008-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'Surat Rekomendasi Beasiswa',
   'Kategori lama, diganti Permohonan Surat Keterangan Aktif', 'kepala_sekolah', 72, true, false, now() - interval '200 days', now() - interval '100 days')
ON CONFLICT DO NOTHING;

INSERT INTO service_requests (id, school_id, category_id, requester_user_id, student_user_id, subject, description, status, assignee_user_id, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.cat::uuid, '33333333-3333-3333-3333-333333333333', v.student::uuid,
       v.subject, v.descr, v.status, v.assignee::uuid, now() - v.age, now() - v.age + v.upd
FROM (VALUES
  ('a2100009-0000-0000-0000-000000000001', 'a2100008-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444',
   'Legalisir rapor semester 1 dan 2 kelas 7', 'Mohon legalisir 3 lembar untuk pendaftaran lomba olimpiade sains tingkat kota.',
   'submitted', NULL, interval '6 hours', interval '0 hours'),
  ('a2100009-0000-0000-0000-000000000002', 'a2100008-0000-0000-0000-000000000002', '44444444-4444-4444-4444-444444444444',
   'Izin 4 hari acara pernikahan keluarga di Medan', 'Budi izin tanggal 20-23 Oktober untuk menghadiri pernikahan kakak sepupu.',
   'accepted', '66666666-6666-6666-6666-666666666666', interval '2 days', interval '3 hours'),
  ('a2100009-0000-0000-0000-000000000003', 'a2100008-0000-0000-0000-000000000003', NULL,
   'Perubahan nomor HP ayah', 'Nomor HP ayah berganti menjadi 081298765432 mohon diperbarui.',
   'done', '22222222-2222-2222-2222-222222222222', interval '20 days', interval '2 days'),
  ('a2100009-0000-0000-0000-000000000004', 'a2100008-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444',
   'Legalisir ijazah SD', 'Mohon legalisir ijazah SD milik Budi.',
   'rejected', '22222222-2222-2222-2222-222222222222', interval '40 days', interval '1 day'),
  ('a2100009-0000-0000-0000-000000000005', 'a2100008-0000-0000-0000-000000000003', NULL,
   'Pembaruan alamat domisili', 'Keluarga pindah ke Jl. Melati No. 12, Kebayoran Baru, Jakarta Selatan.',
   'in_progress', '22222222-2222-2222-2222-222222222222', interval '3 days', interval '1 day')
) AS v(id, cat, student, subject, descr, status, assignee, age, upd)
JOIN service_request_categories c ON c.id = v.cat::uuid
ON CONFLICT DO NOTHING;

INSERT INTO service_request_events (id, school_id, request_id, from_status, to_status, note, actor_user_id, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', sr.id, v.from_status, v.to_status, v.note, v.actor::uuid, sr.created_at + v.after
FROM (VALUES
  ('a2100010-0000-0000-0000-000000000001', 'a2100009-0000-0000-0000-000000000001', NULL, 'submitted', 'Permintaan dibuat', '33333333-3333-3333-3333-333333333333', interval '0'),
  ('a2100010-0000-0000-0000-000000000002', 'a2100009-0000-0000-0000-000000000002', NULL, 'submitted', 'Permintaan dibuat', '33333333-3333-3333-3333-333333333333', interval '0'),
  ('a2100010-0000-0000-0000-000000000003', 'a2100009-0000-0000-0000-000000000002', 'submitted', 'accepted', 'Izin diterima wali kelas; tugas akan dikirim lewat portal.', '66666666-6666-6666-6666-666666666666', interval '3 hours'),
  ('a2100010-0000-0000-0000-000000000004', 'a2100009-0000-0000-0000-000000000003', NULL, 'submitted', 'Permintaan dibuat', '33333333-3333-3333-3333-333333333333', interval '0'),
  ('a2100010-0000-0000-0000-000000000005', 'a2100009-0000-0000-0000-000000000003', 'submitted', 'in_progress', 'Diproses tata usaha', '22222222-2222-2222-2222-222222222222', interval '1 day'),
  ('a2100010-0000-0000-0000-000000000006', 'a2100009-0000-0000-0000-000000000003', 'in_progress', 'done', 'Nomor HP sudah diperbarui di data induk.', '22222222-2222-2222-2222-222222222222', interval '18 days'),
  ('a2100010-0000-0000-0000-000000000007', 'a2100009-0000-0000-0000-000000000004', NULL, 'submitted', 'Permintaan dibuat', '33333333-3333-3333-3333-333333333333', interval '0'),
  ('a2100010-0000-0000-0000-000000000008', 'a2100009-0000-0000-0000-000000000004', 'submitted', 'rejected', 'Ijazah SD dilegalisir oleh SD asal, bukan oleh SMP.', '22222222-2222-2222-2222-222222222222', interval '39 days'),
  ('a2100010-0000-0000-0000-000000000009', 'a2100009-0000-0000-0000-000000000005', NULL, 'submitted', 'Permintaan dibuat', '33333333-3333-3333-3333-333333333333', interval '0'),
  ('a2100010-0000-0000-0000-000000000010', 'a2100009-0000-0000-0000-000000000005', 'submitted', 'in_progress', 'Menunggu salinan KK terbaru.', '22222222-2222-2222-2222-222222222222', interval '2 days'),
  ('a2100010-0000-0000-0000-000000000011', '74000000-0000-0000-0000-000000000001', NULL, 'submitted', 'Permintaan dibuat', '33333333-3333-3333-3333-333333333333', interval '0'),
  ('a2100010-0000-0000-0000-000000000012', '74000000-0000-0000-0000-000000000001', 'submitted', 'in_progress', 'Surat sedang disiapkan tata usaha.', '22222222-2222-2222-2222-222222222222', interval '2 hours')
) AS v(id, request_id, from_status, to_status, note, actor, after)
JOIN service_requests sr ON sr.id = v.request_id::uuid
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 8. Service desk: tim, kategori, jam layanan, libur, SLA, eskalasi, tiket
-- ---------------------------------------------------------------------------
INSERT INTO sd_teams (id, school_id, name, manager_user_id, created_at, updated_at)
VALUES ('a2100011-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Tim Layanan Akademik & Tata Usaha',
        '77777777-7777-7777-7777-777777777777', now() - interval '90 days', now() - interval '90 days')
ON CONFLICT DO NOTHING;

INSERT INTO sd_team_members (team_id, user_id, school_id)
SELECT t.id, v.user_id::uuid, '11111111-1111-1111-1111-111111111111'
FROM (VALUES ('22222222-2222-2222-2222-222222222222'), ('66666666-6666-6666-6666-666666666666')) AS v(user_id)
JOIN sd_teams t ON t.id = 'a2100011-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO sd_categories (id, school_id, name, description, team_id, is_facility, form_schema, reporter_kinds, is_fallback, active, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.name, v.descr, t.id, false, v.form::jsonb, v.kinds::text[], false, true,
       now() - interval '90 days', now() - interval '90 days'
FROM (VALUES
  ('a2100012-0000-0000-0000-000000000001', 'Akun & Login Portal', 'Lupa password, akun terkunci, atau tidak bisa masuk portal',
   '[{"key":"portal","label":"Portal","type":"select","options":["Orang Tua","Siswa","Pegawai"],"required":true}]', '{orang_tua,siswa,pegawai}'),
  ('a2100012-0000-0000-0000-000000000002', 'Nilai & Rapor', 'Pertanyaan atau keberatan nilai dan rapor digital',
   '[{"key":"mapel","label":"Mata pelajaran","type":"text","required":true}]', '{orang_tua,siswa}')
) AS v(id, name, descr, form, kinds)
JOIN sd_teams t ON t.id = 'a2100011-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO sd_business_hours (school_id, weekday, opens_at, closes_at)
VALUES
  ('11111111-1111-1111-1111-111111111111', 1, '07:00', '15:00'),
  ('11111111-1111-1111-1111-111111111111', 2, '07:00', '15:00'),
  ('11111111-1111-1111-1111-111111111111', 3, '07:00', '15:00'),
  ('11111111-1111-1111-1111-111111111111', 4, '07:00', '15:00'),
  ('11111111-1111-1111-1111-111111111111', 5, '07:00', '14:00'),
  ('11111111-1111-1111-1111-111111111111', 6, '07:00', '12:00')
ON CONFLICT DO NOTHING;

INSERT INTO sd_holidays (id, school_id, date, name)
VALUES
  ('a2100013-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '2026-12-25', 'Hari Raya Natal'),
  ('a2100013-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '2026-12-26', 'Cuti Bersama Natal'),
  ('a2100013-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '2027-01-01', 'Tahun Baru Masehi'),
  ('a2100013-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', '2026-08-17', 'Hari Kemerdekaan RI'),
  ('a2100013-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', '2026-08-25', 'Maulid Nabi Muhammad SAW')
ON CONFLICT DO NOTHING;

INSERT INTO sd_sla_policies (id, school_id, category_id, priority, response_value, response_unit, resolution_value, resolution_unit,
                             warn_bp, effective_from, effective_to, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.cat::uuid, v.priority, v.rv, v.ru, v.sv, v.su, v.warn,
       v.ef::date, v.et::date, now() - interval '90 days', now() - interval '90 days'
FROM (VALUES
  ('a2100014-0000-0000-0000-000000000001', NULL, 'low', 2, 'business_days', 5, 'business_days', 8000, '2026-07-01', NULL),
  ('a2100014-0000-0000-0000-000000000002', NULL, 'normal', 8, 'business_hours', 3, 'business_days', 8000, '2026-07-01', NULL),
  ('a2100014-0000-0000-0000-000000000003', NULL, 'high', 4, 'business_hours', 1, 'business_days', 7500, '2026-07-01', NULL),
  ('a2100014-0000-0000-0000-000000000004', NULL, 'urgent', 1, 'business_hours', 4, 'business_hours', 5000, '2026-07-01', NULL),
  ('a2100014-0000-0000-0000-000000000005', 'a2100012-0000-0000-0000-000000000001', 'normal', 2, 'business_hours', 1, 'business_days', 8000, '2026-07-01', NULL),
  ('a2100014-0000-0000-0000-000000000006', NULL, 'normal', 1, 'business_days', 5, 'business_days', 8000, '2025-07-01', '2026-06-30')
) AS v(id, cat, priority, rv, ru, sv, su, warn, ef, et)
WHERE v.cat IS NULL OR EXISTS (SELECT 1 FROM sd_categories c WHERE c.id = v.cat::uuid)
ON CONFLICT DO NOTHING;

INSERT INTO sd_escalation_rules (id, school_id, policy_id, trigger, level, target_kind, target_role, target_user_id)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', p.id, v.trg, v.lvl, v.kind, v.role, v.usr::uuid
FROM (VALUES
  ('a2100015-0000-0000-0000-000000000001', 'a2100014-0000-0000-0000-000000000002', 'warning', 1, 'team_manager', NULL, NULL),
  ('a2100015-0000-0000-0000-000000000002', 'a2100014-0000-0000-0000-000000000002', 'breach', 1, 'role', 'servicedesk_manager', NULL),
  ('a2100015-0000-0000-0000-000000000003', 'a2100014-0000-0000-0000-000000000003', 'breach', 1, 'team_manager', NULL, NULL),
  ('a2100015-0000-0000-0000-000000000004', 'a2100014-0000-0000-0000-000000000004', 'warning', 1, 'team_manager', NULL, NULL),
  ('a2100015-0000-0000-0000-000000000005', 'a2100014-0000-0000-0000-000000000004', 'breach', 1, 'user', NULL, '77777777-7777-7777-7777-777777777777'),
  ('a2100015-0000-0000-0000-000000000006', 'a2100014-0000-0000-0000-000000000004', 'breach', 2, 'role', 'kepala_yayasan', NULL)
) AS v(id, policy, trg, lvl, kind, role, usr)
JOIN sd_sla_policies p ON p.id = v.policy::uuid
ON CONFLICT DO NOTHING;

-- Nomor tiket mengikuti format layanan (SD-YYYY-NNNNNN). Urutan tahun 2026
-- hanya dibuat bila belum ada; tiket dilewati bila nomornya sudah terpakai.
INSERT INTO sd_ticket_sequences (school_id, year, last)
VALUES ('11111111-1111-1111-1111-111111111111', 2026, 6)
ON CONFLICT DO NOTHING;

INSERT INTO sd_tickets (id, school_id, number, category_id, team_id, priority, subject, description, form_answers, channel,
                        reporter_user_id, student_user_id, status, assignee_user_id, response_due_at, resolution_due_at,
                        first_responded_at, resolved_at, closed_at, response_breached_at, resolution_breached_at, escalation_level,
                        satisfaction_score, satisfaction_comment, rated_at, idempotency_key, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', v.num, c.id, c.team_id, v.priority, v.subject, v.descr, v.answers::jsonb, v.channel,
       v.reporter::uuid, v.student::uuid, v.status, v.assignee::uuid,
       now() - v.age + v.resp_due, now() - v.age + v.reso_due,
       CASE WHEN v.status <> 'new' THEN now() - v.age + interval '1 hour' END,
       CASE WHEN v.status IN ('resolved', 'closed') THEN now() - v.age + interval '1 day' END,
       CASE WHEN v.status = 'closed' THEN now() - v.age + interval '2 days' END,
       NULL, v.breach, v.esc, v.score, v.score_comment,
       CASE WHEN v.score IS NOT NULL THEN now() - v.age + interval '2 days' END,
       'seed-a21:' || v.id, now() - v.age, now() - v.age + interval '1 hour'
FROM (VALUES
  ('a2100016-0000-0000-0000-000000000001', 'SD-2026-000001', 'a2100012-0000-0000-0000-000000000001', 'normal',
   'Tidak bisa masuk portal orang tua', 'Muncul pesan akun terkunci setelah salah memasukkan password.', '{"portal":"Orang Tua"}', 'parent_portal',
   '33333333-3333-3333-3333-333333333333', '44444444-4444-4444-4444-444444444444', 'new', NULL,
   interval '2 hours', interval '1 day', interval '1 hour', NULL::timestamptz, 0, NULL::smallint, NULL),
  ('a2100016-0000-0000-0000-000000000002', 'SD-2026-000002', 'a2100012-0000-0000-0000-000000000002', 'high',
   'Nilai PTS Matematika belum muncul di rapor digital', 'Nilai PTS Matematika Budi belum tampil padahal teman sekelas sudah.', '{"mapel":"Matematika"}', 'parent_portal',
   '33333333-3333-3333-3333-333333333333', '44444444-4444-4444-4444-444444444444', 'assigned', '66666666-6666-6666-6666-666666666666',
   interval '4 hours', interval '1 day', interval '5 hours', NULL::timestamptz, 0, NULL::smallint, NULL),
  ('a2100016-0000-0000-0000-000000000003', 'SD-2026-000003', 'a2100012-0000-0000-0000-000000000001', 'normal',
   'Reset password akun siswa', 'Saya lupa password akun siswa setelah libur semester.', '{"portal":"Siswa"}', 'student_portal',
   '44444444-4444-4444-4444-444444444444', '44444444-4444-4444-4444-444444444444', 'in_progress', '22222222-2222-2222-2222-222222222222',
   interval '2 hours', interval '1 day', interval '1 day', NULL::timestamptz, 0, NULL::smallint, NULL),
  ('a2100016-0000-0000-0000-000000000004', 'SD-2026-000004', 'a2100012-0000-0000-0000-000000000002', 'urgent',
   'Rapor semester genap tidak bisa diunduh', 'File PDF rapor semester genap kosong saat diunduh.', '{"mapel":"Semua"}', 'parent_portal',
   '33333333-3333-3333-3333-333333333333', '44444444-4444-4444-4444-444444444444', 'closed', '22222222-2222-2222-2222-222222222222',
   interval '1 hour', interval '4 hours', interval '20 days', now() - interval '20 days' + interval '5 hours', 1, 5::smallint, 'Cepat ditangani, terima kasih.'),
  ('a2100016-0000-0000-0000-000000000005', 'SD-2026-000005', 'a2100012-0000-0000-0000-000000000001', 'low',
   'Ganti email akun pegawai', 'Mohon ganti email login ke alamat sekolah yang baru.', '{"portal":"Pegawai"}', 'employee_portal',
   '55555555-5555-5555-5555-555555555555', NULL, 'resolved', '22222222-2222-2222-2222-222222222222',
   interval '2 days', interval '5 days', interval '9 days', NULL::timestamptz, 0, NULL::smallint, NULL),
  ('a2100016-0000-0000-0000-000000000006', 'SD-2026-000006', 'a2100012-0000-0000-0000-000000000002', 'normal',
   'Minta perubahan nilai sikap', 'Mohon nilai sikap dinaikkan karena anak sudah berubah.', '{"mapel":"Sikap"}', 'parent_portal',
   '33333333-3333-3333-3333-333333333333', '44444444-4444-4444-4444-444444444444', 'rejected', '66666666-6666-6666-6666-666666666666',
   interval '8 hours', interval '3 days', interval '15 days', NULL::timestamptz, 0, NULL::smallint, NULL)
) AS v(id, num, cat, priority, subject, descr, answers, channel, reporter, student, status, assignee,
       resp_due, reso_due, age, breach, esc, score, score_comment)
JOIN sd_categories c ON c.id = v.cat::uuid
WHERE NOT EXISTS (SELECT 1 FROM sd_tickets t WHERE t.school_id = '11111111-1111-1111-1111-111111111111' AND t.number = v.num)
ON CONFLICT DO NOTHING;

INSERT INTO sd_ticket_events (id, school_id, ticket_id, kind, from_status, to_status, body, internal, actor_user_id, dedupe_key, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', t.id, v.kind, v.from_status, v.to_status, v.body, v.internal,
       v.actor::uuid, 'seed-a21:' || v.id, t.created_at + v.after
FROM (VALUES
  ('a2100017-0000-0000-0000-000000000001', 'a2100016-0000-0000-0000-000000000001', 'status', NULL, 'new', 'Tiket dibuat', false, '33333333-3333-3333-3333-333333333333', interval '0'),
  ('a2100017-0000-0000-0000-000000000002', 'a2100016-0000-0000-0000-000000000002', 'status', NULL, 'new', 'Tiket dibuat', false, '33333333-3333-3333-3333-333333333333', interval '0'),
  ('a2100017-0000-0000-0000-000000000003', 'a2100016-0000-0000-0000-000000000002', 'assign', NULL, NULL, 'Ditugaskan ke Wali Kelas VII A', false, '77777777-7777-7777-7777-777777777777', interval '1 hour'),
  ('a2100017-0000-0000-0000-000000000004', 'a2100016-0000-0000-0000-000000000002', 'status', 'new', 'assigned', NULL, false, '77777777-7777-7777-7777-777777777777', interval '1 hour'),
  ('a2100017-0000-0000-0000-000000000005', 'a2100016-0000-0000-0000-000000000003', 'status', NULL, 'new', 'Tiket dibuat', false, '44444444-4444-4444-4444-444444444444', interval '0'),
  ('a2100017-0000-0000-0000-000000000006', 'a2100016-0000-0000-0000-000000000003', 'status', 'new', 'in_progress', NULL, false, '22222222-2222-2222-2222-222222222222', interval '1 hour'),
  ('a2100017-0000-0000-0000-000000000007', 'a2100016-0000-0000-0000-000000000003', 'comment', NULL, NULL, 'Cek identitas siswa via wali kelas sebelum reset.', true, '22222222-2222-2222-2222-222222222222', interval '2 hours'),
  ('a2100017-0000-0000-0000-000000000008', 'a2100016-0000-0000-0000-000000000004', 'status', NULL, 'new', 'Tiket dibuat', false, '33333333-3333-3333-3333-333333333333', interval '0'),
  ('a2100017-0000-0000-0000-000000000009', 'a2100016-0000-0000-0000-000000000004', 'escalation', NULL, NULL, 'Eskalasi level 1 ke manajer tim', false, NULL, interval '3 hours'),
  ('a2100017-0000-0000-0000-000000000010', 'a2100016-0000-0000-0000-000000000004', 'status', 'in_progress', 'resolved', 'Template rapor diperbaiki; silakan unduh ulang.', false, '22222222-2222-2222-2222-222222222222', interval '1 day'),
  ('a2100017-0000-0000-0000-000000000011', 'a2100016-0000-0000-0000-000000000004', 'satisfaction', NULL, NULL, 'Rating 5: Cepat ditangani, terima kasih.', false, '33333333-3333-3333-3333-333333333333', interval '2 days'),
  ('a2100017-0000-0000-0000-000000000012', 'a2100016-0000-0000-0000-000000000004', 'status', 'resolved', 'closed', NULL, false, '33333333-3333-3333-3333-333333333333', interval '2 days'),
  ('a2100017-0000-0000-0000-000000000013', 'a2100016-0000-0000-0000-000000000005', 'status', 'in_progress', 'resolved', 'Email login sudah diganti.', false, '22222222-2222-2222-2222-222222222222', interval '1 day'),
  ('a2100017-0000-0000-0000-000000000014', 'a2100016-0000-0000-0000-000000000006', 'status', 'assigned', 'rejected', 'Nilai sikap ditetapkan melalui rapat dewan guru, bukan lewat tiket.', false, '66666666-6666-6666-6666-666666666666', interval '1 day')
) AS v(id, ticket_id, kind, from_status, to_status, body, internal, actor, after)
JOIN sd_tickets t ON t.id = v.ticket_id::uuid
ON CONFLICT DO NOTHING;

COMMIT;
