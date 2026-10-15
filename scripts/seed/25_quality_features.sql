-- 25_quality_features.sql
-- Data demo quality (penjaminan mutu + pelaporan pemerintah):
--   * Instrumen EDS SPMI internal yayasan lengkap (standar, butir, indikator
--     manual + berbasis metrik insight) dengan target tahun 2026/2027.
--   * Bukti mutu (tautan & laporan sekoci; tanpa file karena tidak ada objek
--     di storage) + tautan ke indikator.
--   * Evaluasi diri: finalized (v1 + snapshot), pending_finalization (terhubung
--     ke approval core a2100003-..011), draft; item penilaian + rencana tindak.
--   * Template laporan regulasi + laporan: submitted, pending_approval
--     (approval core a2100003-..012), rejected, draft.
--   * Dapodik: pemetaan kolom, impor (siswa/PTK/rombel), baris impor, selisih
--     (terbuka & sudah diputuskan), log sinkronisasi.
--
-- Aturan: hanya MENAMBAH (UUID tetap berawalan 'a25', ON CONFLICT DO NOTHING).
-- Satu transaksi, idempoten. Instrumen lama dari 06_quality.sql tidak disentuh
-- (instrumen baru dipakai agar semua relasi deterministik).
-- Sengaja kosong: audit_events (log audit mentah yang diteruskan ke core).
-- Kolom file (qa_evidence.object_key, regulatory_reports.export_object_key)
-- dibiarkan NULL karena tidak ada berkas di storage.
-- Tahun akademik: 12121212-.. (2026/2027, sama dengan 02_academic.sql).

BEGIN;

-- 1. Instrumen, standar, butir, indikator --------------------------------------
INSERT INTO qa_instruments (id, school_id, code, name, issuer, version_label, score_min, score_max, status, effective_from, created_at, updated_at)
VALUES
  ('a2500001-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'EDS-SPMI-TPGS', 'Instrumen EDS SPMI',
   'Yayasan Teratai Putih', 'v2026.1', 0, 100, 'active', '2026-07-01', now() - interval '120 days', now() - interval '120 days'),
  ('a2500001-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'EDS-SPMI-TPGS', 'Instrumen EDS SPMI',
   'Yayasan Teratai Putih', 'v2025.1', 0, 100, 'retired', '2025-07-01', now() - interval '480 days', now() - interval '120 days'),
  ('a2500001-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'SUPERVISI-KBM', 'Instrumen Supervisi Pembelajaran (draf)',
   'Kepala Sekolah', 'v2027.0', 1, 4, 'draft', NULL, now() - interval '5 days', now() - interval '5 days')
ON CONFLICT DO NOTHING;

INSERT INTO qa_standards (id, school_id, instrument_id, sort, code, name, weight_bp, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', i.id, v.sort, v.code, v.name, v.w, i.created_at, i.created_at
FROM (VALUES
  ('a2500002-0000-0000-0000-000000000001', 1, 'SKL', 'Standar Kompetensi Lulusan', 3500),
  ('a2500002-0000-0000-0000-000000000002', 2, 'PROSES', 'Standar Proses', 3500),
  ('a2500002-0000-0000-0000-000000000003', 3, 'PENGELOLAAN', 'Standar Pengelolaan & Pembiayaan', 3000)
) AS v(id, sort, code, name, w)
JOIN qa_instruments i ON i.id = 'a2500001-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO qa_items (id, school_id, standard_id, sort, code, statement, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', s.id, v.sort, v.code, v.stmt, s.created_at, s.created_at
FROM (VALUES
  ('a2500004-0000-0000-0000-000000000001', 'a2500002-0000-0000-0000-000000000001', 1, 'SKL-1', 'Peserta didik hadir dan mengikuti pembelajaran secara konsisten.'),
  ('a2500004-0000-0000-0000-000000000002', 'a2500002-0000-0000-0000-000000000001', 2, 'SKL-2', 'Peserta didik meraih prestasi akademik dan non-akademik.'),
  ('a2500004-0000-0000-0000-000000000003', 'a2500002-0000-0000-0000-000000000002', 1, 'PROSES-1', 'Guru merencanakan dan melaksanakan pembelajaran berdiferensiasi.'),
  ('a2500004-0000-0000-0000-000000000004', 'a2500002-0000-0000-0000-000000000003', 1, 'KELOLA-1', 'Sekolah mengelola pembiayaan secara tertib dan akuntabel.')
) AS v(id, std, sort, code, stmt)
JOIN qa_standards s ON s.id = v.std::uuid
ON CONFLICT DO NOTHING;

INSERT INTO qa_indicators (id, school_id, item_id, sort, code, name, weight_bp, unit, direction, source_kind, metric_key, rubric, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', it.id, v.sort, v.code, v.name, v.w, v.unit, v.dir, v.src, v.metric, v.rubric::jsonb, it.created_at, it.created_at
FROM (VALUES
  ('a2500005-0000-0000-0000-000000000001', 'a2500004-0000-0000-0000-000000000001', 1, 'SKL-1.1', 'Rata-rata kehadiran peserta didik', 2000,
   'percent_bp', 'higher_better', 'metric', 'attendance_rate', '[]'),
  ('a2500005-0000-0000-0000-000000000002', 'a2500004-0000-0000-0000-000000000001', 2, 'SKL-1.2', 'Jumlah kasus pelanggaran tata tertib berat', 1500,
   'count', 'lower_better', 'manual', NULL, '[]'),
  ('a2500005-0000-0000-0000-000000000003', 'a2500004-0000-0000-0000-000000000002', 1, 'SKL-2.1', 'Prestasi terverifikasi per semester', 1500,
   'count', 'higher_better', 'metric', 'verified_achievements', '[]'),
  ('a2500005-0000-0000-0000-000000000004', 'a2500004-0000-0000-0000-000000000003', 1, 'PROSES-1.1', 'Mutu modul ajar berdiferensiasi (hasil supervisi)', 2000,
   'score', 'higher_better', 'manual', NULL,
   '[{"score":4,"desc":"Seluruh modul memuat asesmen diagnostik dan diferensiasi"},{"score":3,"desc":">= 75% modul"},{"score":2,"desc":"50-74% modul"},{"score":1,"desc":"< 50% modul"}]'),
  ('a2500005-0000-0000-0000-000000000005', 'a2500004-0000-0000-0000-000000000004', 1, 'KELOLA-1.1', 'Kolektibilitas SPP', 2000,
   'percent_bp', 'higher_better', 'metric', 'collection_rate', '[]'),
  ('a2500005-0000-0000-0000-000000000006', 'a2500004-0000-0000-0000-000000000004', 2, 'KELOLA-1.2', 'Rata-rata hari keterlambatan laporan keuangan bulanan', 1000,
   'days', 'lower_better', 'manual', NULL, '[]')
) AS v(id, item, sort, code, name, w, unit, dir, src, metric, rubric)
JOIN qa_items it ON it.id = v.item::uuid
ON CONFLICT DO NOTHING;

INSERT INTO qa_targets (id, school_id, indicator_id, academic_year_id, target_value, owner_user_id, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', ind.id, '12121212-1212-1212-1212-121212121212', v.target, v.owner::uuid, ind.created_at, ind.created_at
FROM (VALUES
  ('a2500006-0000-0000-0000-000000000001', 'a2500005-0000-0000-0000-000000000001', 9500, '66666666-6666-6666-6666-666666666666'),
  ('a2500006-0000-0000-0000-000000000002', 'a2500005-0000-0000-0000-000000000002', 2, '77777777-7777-7777-7777-777777777777'),
  ('a2500006-0000-0000-0000-000000000003', 'a2500005-0000-0000-0000-000000000003', 10, '55555555-5555-5555-5555-555555555555'),
  ('a2500006-0000-0000-0000-000000000004', 'a2500005-0000-0000-0000-000000000004', 3, '77777777-7777-7777-7777-777777777777'),
  ('a2500006-0000-0000-0000-000000000005', 'a2500005-0000-0000-0000-000000000005', 9200, '22222222-2222-2222-2222-222222222222'),
  ('a2500006-0000-0000-0000-000000000006', 'a2500005-0000-0000-0000-000000000006', 5, '22222222-2222-2222-2222-222222222222')
) AS v(id, ind, target, owner)
JOIN qa_indicators ind ON ind.id = v.ind::uuid
ON CONFLICT DO NOTHING;

-- 2. Bukti mutu + tautan ke indikator ------------------------------------------
INSERT INTO qa_evidence (id, school_id, kind, title, object_key, mime, size_bytes, checksum, report_ref, url, version, supersedes_id,
                         uploaded_by, status, verified_by, verified_at, reject_reason, created_at, updated_at)
VALUES
  ('a2500007-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'sekoci_report', 'Rekap Kehadiran Siswa Juli-September 2026',
   NULL, NULL, NULL, NULL, '{"metric_key":"attendance_rate","period_from":"2026-07-01","period_to":"2026-09-30"}', NULL, 1, NULL,
   '66666666-6666-6666-6666-666666666666', 'verified', '77777777-7777-7777-7777-777777777777', now() - interval '12 days', NULL, now() - interval '14 days', now() - interval '12 days'),
  ('a2500007-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'link', 'Kumpulan Modul Ajar Fase D Semester Ganjil (v1)',
   NULL, NULL, NULL, NULL, NULL, 'https://drive.teratai.sch.id/kurikulum/modul-ajar-fase-d', 1, NULL,
   '55555555-5555-5555-5555-555555555555', 'rejected', '77777777-7777-7777-7777-777777777777', now() - interval '30 days',
   'Modul IPA dan IPS belum memuat asesmen diagnostik.', now() - interval '35 days', now() - interval '30 days'),
  ('a2500007-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'link', 'Kumpulan Modul Ajar Fase D Semester Ganjil (v2)',
   NULL, NULL, NULL, NULL, NULL, 'https://drive.teratai.sch.id/kurikulum/modul-ajar-fase-d-v2', 2, 'a2500007-0000-0000-0000-000000000002',
   '55555555-5555-5555-5555-555555555555', 'verified', '77777777-7777-7777-7777-777777777777', now() - interval '20 days', NULL, now() - interval '25 days', now() - interval '20 days'),
  ('a2500007-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'sekoci_report', 'Laporan Kolektibilitas SPP Triwulan III 2026',
   NULL, NULL, NULL, NULL, '{"metric_key":"collection_rate","period_from":"2026-07-01","period_to":"2026-09-30"}', NULL, 1, NULL,
   '22222222-2222-2222-2222-222222222222', 'verified', '88888888-8888-8888-8888-888888888888', now() - interval '10 days', NULL, now() - interval '11 days', now() - interval '10 days'),
  ('a2500007-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'link', 'Berita Acara Pembinaan Kasus Tata Tertib Semester Ganjil',
   NULL, NULL, NULL, NULL, NULL, 'https://drive.teratai.sch.id/kesiswaan/berita-acara-pembinaan', 1, NULL,
   '66666666-6666-6666-6666-666666666666', 'pending', NULL, NULL, NULL, now() - interval '2 days', now() - interval '2 days'),
  ('a2500007-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111', 'link', 'Daftar Sertifikat Prestasi Lomba Kota & Provinsi 2026',
   NULL, NULL, NULL, NULL, NULL, 'https://drive.teratai.sch.id/kesiswaan/prestasi-2026', 1, NULL,
   '55555555-5555-5555-5555-555555555555', 'verified', '77777777-7777-7777-7777-777777777777', now() - interval '8 days', NULL, now() - interval '9 days', now() - interval '8 days')
ON CONFLICT DO NOTHING;

INSERT INTO qa_evidence_links (evidence_id, indicator_id, academic_year_id, locked_at, created_at)
SELECT e.id, ind.id, '12121212-1212-1212-1212-121212121212', NULL, e.created_at
FROM (VALUES
  ('a2500007-0000-0000-0000-000000000001', 'a2500005-0000-0000-0000-000000000001'),
  ('a2500007-0000-0000-0000-000000000003', 'a2500005-0000-0000-0000-000000000004'),
  ('a2500007-0000-0000-0000-000000000004', 'a2500005-0000-0000-0000-000000000005'),
  ('a2500007-0000-0000-0000-000000000005', 'a2500005-0000-0000-0000-000000000002'),
  ('a2500007-0000-0000-0000-000000000006', 'a2500005-0000-0000-0000-000000000003')
) AS v(ev, ind)
JOIN qa_evidence e ON e.id = v.ev::uuid
JOIN qa_indicators ind ON ind.id = v.ind::uuid
ON CONFLICT DO NOTHING;

-- 3. Evaluasi diri ---------------------------------------------------------------
-- 001 pending_finalization -> approval core a2100003-..011 (pending)
-- 002 finalized v1 (snapshot di qa_self_eval_versions)
-- 003 draft
INSERT INTO qa_self_evaluations (id, school_id, instrument_id, academic_year_id, period_label, period_from, period_to, status,
                                 approval_id, reopen_approval_id, finalized_at, finalized_by, version, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', i.id, '12121212-1212-1212-1212-121212121212', v.label, v.pf::date, v.pt::date, v.status,
       v.appr::uuid, NULL, v.fin_at, v.fin_by::uuid, v.ver, v.created, v.updated
FROM (VALUES
  ('a2500003-0000-0000-0000-000000000001', 'Evaluasi Tengah Semester Ganjil 2026/2027', '2026-07-13', '2026-09-30', 'pending_finalization',
   'a2100003-0000-0000-0000-000000000011', NULL::timestamptz, NULL, 0, now() - interval '15 days', now() - interval '4 days'),
  ('a2500003-0000-0000-0000-000000000002', 'Evaluasi Awal Tahun 2026/2027', '2026-07-13', '2026-08-14', 'finalized',
   NULL, now() - interval '55 days', '77777777-7777-7777-7777-777777777777', 1, now() - interval '62 days', now() - interval '55 days'),
  ('a2500003-0000-0000-0000-000000000003', 'Evaluasi Akhir Semester Ganjil 2026/2027', '2026-10-01', '2026-12-18', 'draft',
   NULL, NULL, NULL, 0, now() - interval '1 day', now() - interval '1 day')
) AS v(id, label, pf, pt, status, appr, fin_at, fin_by, ver, created, updated)
JOIN qa_instruments i ON i.id = 'a2500001-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO qa_self_eval_items (id, school_id, self_evaluation_id, indicator_id, target_value, actual_value, value_source, metric_as_of, metric_fetched_at,
                                score, below_target, note, assessed_by, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', se.id, ind.id, v.target, v.actual, ind.source_kind,
       CASE WHEN ind.source_kind = 'metric' THEN se.updated_at - interval '1 day' END,
       CASE WHEN ind.source_kind = 'metric' THEN se.updated_at - interval '1 day' END,
       v.score, v.below, v.note, v.by::uuid, se.created_at, se.updated_at
FROM (VALUES
  -- 001 Tengah semester (menunggu finalisasi)
  ('a2500008-0000-0000-0000-000000000001', 'a2500003-0000-0000-0000-000000000001', 'a2500005-0000-0000-0000-000000000001', 9500::bigint, 9450::bigint, 85, true,
   'Kehadiran kelas 9 turun saat musim hujan.', '66666666-6666-6666-6666-666666666666'),
  ('a2500008-0000-0000-0000-000000000002', 'a2500003-0000-0000-0000-000000000001', 'a2500005-0000-0000-0000-000000000002', 2, 1, 90, false,
   'Satu kasus perundungan ditangani BK.', '77777777-7777-7777-7777-777777777777'),
  ('a2500008-0000-0000-0000-000000000003', 'a2500003-0000-0000-0000-000000000001', 'a2500005-0000-0000-0000-000000000003', 10, 12, 95, false,
   NULL, '55555555-5555-5555-5555-555555555555'),
  ('a2500008-0000-0000-0000-000000000004', 'a2500003-0000-0000-0000-000000000001', 'a2500005-0000-0000-0000-000000000004', 3, 3, 80, false,
   'Modul v2 sudah memuat asesmen diagnostik.', '77777777-7777-7777-7777-777777777777'),
  ('a2500008-0000-0000-0000-000000000005', 'a2500003-0000-0000-0000-000000000001', 'a2500005-0000-0000-0000-000000000005', 9200, 8996, 70, true,
   'Tunggakan > 30 hari masih 14 keluarga.', '22222222-2222-2222-2222-222222222222'),
  ('a2500008-0000-0000-0000-000000000006', 'a2500003-0000-0000-0000-000000000001', 'a2500005-0000-0000-0000-000000000006', 5, 4, 80, false,
   NULL, '22222222-2222-2222-2222-222222222222'),
  -- 002 Awal tahun (final)
  ('a2500008-0000-0000-0000-000000000007', 'a2500003-0000-0000-0000-000000000002', 'a2500005-0000-0000-0000-000000000001', 9500, 9500, 90, false,
   NULL, '66666666-6666-6666-6666-666666666666'),
  ('a2500008-0000-0000-0000-000000000008', 'a2500003-0000-0000-0000-000000000002', 'a2500005-0000-0000-0000-000000000004', 3, 2, 60, true,
   'Modul IPA dan IPS perlu direvisi.', '77777777-7777-7777-7777-777777777777'),
  ('a2500008-0000-0000-0000-000000000009', 'a2500003-0000-0000-0000-000000000002', 'a2500005-0000-0000-0000-000000000005', 9200, 9100, 80, true,
   NULL, '22222222-2222-2222-2222-222222222222'),
  ('a2500008-0000-0000-0000-000000000010', 'a2500003-0000-0000-0000-000000000002', 'a2500005-0000-0000-0000-000000000006', 5, 7, 50, true,
   'Laporan Juli terlambat karena pergantian bendahara.', '22222222-2222-2222-2222-222222222222'),
  -- 003 Akhir semester (draf, belum dinilai)
  ('a2500008-0000-0000-0000-000000000011', 'a2500003-0000-0000-0000-000000000003', 'a2500005-0000-0000-0000-000000000001', 9500, NULL, NULL, false, NULL, NULL),
  ('a2500008-0000-0000-0000-000000000012', 'a2500003-0000-0000-0000-000000000003', 'a2500005-0000-0000-0000-000000000005', 9200, NULL, NULL, false, NULL, NULL)
) AS v(id, se, ind, target, actual, score, below, note, by)
JOIN qa_self_evaluations se ON se.id = v.se::uuid
JOIN qa_indicators ind ON ind.id = v.ind::uuid
ON CONFLICT DO NOTHING;

INSERT INTO qa_action_plans (id, school_id, self_eval_item_id, action, owner_user_id, due_date, status, done_at, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', it.id, v.action, v.owner::uuid, v.due::date, v.status, v.done, it.created_at, it.updated_at
FROM (VALUES
  ('a2500009-0000-0000-0000-000000000001', 'a2500008-0000-0000-0000-000000000008', 'Revisi modul ajar IPA & IPS dengan asesmen diagnostik; supervisi ulang oleh kepala sekolah.',
   '55555555-5555-5555-5555-555555555555', '2026-09-15', 'done', now() - interval '25 days'),
  ('a2500009-0000-0000-0000-000000000002', 'a2500008-0000-0000-0000-000000000010', 'Susun SOP tutup buku bulanan dan serah terima tugas bendahara.',
   '22222222-2222-2222-2222-222222222222', '2026-10-31', 'in_progress', NULL::timestamptz),
  ('a2500009-0000-0000-0000-000000000003', 'a2500008-0000-0000-0000-000000000005', 'Hubungi 14 keluarga penunggak > 30 hari dan tawarkan skema cicilan.',
   '22222222-2222-2222-2222-222222222222', '2026-11-15', 'open', NULL),
  ('a2500009-0000-0000-0000-000000000004', 'a2500008-0000-0000-0000-000000000001', 'Program jemput sapa wali kelas untuk siswa kelas 9 dengan kehadiran < 90%.',
   '66666666-6666-6666-6666-666666666666', '2026-11-30', 'open', NULL),
  ('a2500009-0000-0000-0000-000000000005', 'a2500008-0000-0000-0000-000000000009', 'Kirim pengingat SPP otomatis tiap tanggal 5 (digantikan rencana penagihan baru).',
   '22222222-2222-2222-2222-222222222222', '2026-09-30', 'cancelled', NULL)
) AS v(id, item, action, owner, due, status, done)
JOIN qa_self_eval_items it ON it.id = v.item::uuid
ON CONFLICT DO NOTHING;

INSERT INTO qa_self_eval_versions (id, school_id, self_evaluation_id, version, snapshot, finalized_at, finalized_by, created_at, updated_at)
SELECT 'a2500010-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', se.id, 1,
       jsonb_build_object('finalized_at', se.finalized_at, 'decided_by', '77777777-7777-7777-7777-777777777777',
                          'comment', 'Disetujui; tindak lanjuti modul IPA/IPS dan laporan keuangan.', 'version', 1),
       se.finalized_at, se.finalized_by, se.finalized_at, se.finalized_at
FROM qa_self_evaluations se
WHERE se.id = 'a2500003-0000-0000-0000-000000000002'
ON CONFLICT DO NOTHING;

-- 4. Laporan regulasi -----------------------------------------------------------
INSERT INTO regulatory_templates (id, school_id, code, name, recipient, frequency, columns, export_format, status, created_at, updated_at)
VALUES
  ('a2500012-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'REKAP-PD-ROMBEL', 'Rekap Peserta Didik per Rombel',
   'Dinas Pendidikan Provinsi DKI Jakarta', 'semester',
   '[{"label":"Jumlah Peserta Didik Aktif","metric_key":"active_students"},{"label":"Rata-rata Kehadiran","metric_key":"attendance_rate","group_by":"grade"}]',
   'xlsx', 'active', now() - interval '150 days', now() - interval '150 days'),
  ('a2500012-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'KEU-BULANAN', 'Laporan Keuangan Bulanan Yayasan',
   'Pengurus Yayasan Teratai Putih', 'monthly',
   '[{"label":"Total Tagihan","metric_key":"billed_idr"},{"label":"Total Penerimaan","metric_key":"paid_idr"},{"label":"Kolektibilitas","metric_key":"collection_rate"}]',
   'xlsx', 'active', now() - interval '150 days', now() - interval '150 days'),
  ('a2500012-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'PTK-TAHUNAN', 'Laporan Tahunan Pendidik & Tenaga Kependidikan',
   'Suku Dinas Pendidikan Jakarta Selatan', 'annual',
   '[{"label":"Guru Aktif","metric_key":"active_teachers"},{"label":"Tenaga Kependidikan Aktif","metric_key":"active_staff"}]',
   'csv', 'active', now() - interval '150 days', now() - interval '150 days'),
  ('a2500012-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'PPDB-LAMA', 'Rekap PPDB Format Lama',
   'Dinas Pendidikan Provinsi DKI Jakarta', 'adhoc',
   '[{"label":"Jumlah Pendaftar","metric_key":"ppdb_applicants"}]',
   'csv', 'inactive', now() - interval '400 days', now() - interval '100 days')
ON CONFLICT DO NOTHING;

-- 001 submitted (Agustus), 002 pending_approval -> approval core a2100003-..012,
-- 003 rejected (September, salah periode), 004 draft (September)
INSERT INTO regulatory_reports (id, school_id, template_id, period_from, period_to, status, approval_id, template_snapshot, data_snapshot, snapshot_at,
                                submitted_at, submitted_by, submission_note, export_object_key, idempotency_key, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', t.id, v.pf::date, v.pt::date, v.status, v.appr::uuid,
       CASE WHEN v.snap IS NOT NULL THEN jsonb_build_object('id', t.id, 'school_id', t.school_id, 'code', t.code, 'name', t.name, 'recipient', t.recipient,
            'frequency', t.frequency, 'columns', t.columns, 'export_format', t.export_format, 'status', t.status,
            'created_at', t.created_at, 'updated_at', t.updated_at) END,
       v.snap::jsonb, v.snap_at, v.sub_at, v.sub_by::uuid, v.note, NULL, 'seed-a25:' || v.id, v.created, v.updated
FROM (VALUES
  ('a2500011-0000-0000-0000-000000000001', 'a2500012-0000-0000-0000-000000000002', '2026-08-01', '2026-08-31', 'submitted', NULL,
   '{"columns":[{"label":"Total Tagihan","metric_key":"billed_idr","value":405000000,"unit":"idr","as_of":"2026-09-01T01:00:00+07:00","unavailable":false},{"label":"Total Penerimaan","metric_key":"paid_idr","value":362880000,"unit":"idr","as_of":"2026-09-01T01:00:00+07:00","unavailable":false},{"label":"Kolektibilitas","metric_key":"collection_rate","value":8960,"unit":"percent_bp","as_of":"2026-09-01T01:00:00+07:00","unavailable":false}]}',
   now() - interval '40 days', now() - interval '38 days', '88888888-8888-8888-8888-888888888888', 'Diserahkan pada rapat pengurus yayasan 7 September 2026.',
   now() - interval '44 days', now() - interval '38 days'),
  ('a2500011-0000-0000-0000-000000000002', 'a2500012-0000-0000-0000-000000000001', '2026-07-01', '2026-09-30', 'pending_approval', 'a2100003-0000-0000-0000-000000000012',
   '{"columns":[{"label":"Jumlah Peserta Didik Aktif","metric_key":"active_students","value":96,"unit":"count","as_of":"2026-10-01T01:00:00+07:00","unavailable":false},{"label":"Rata-rata Kehadiran","metric_key":"attendance_rate","value":9450,"unit":"percent_bp","as_of":"2026-10-01T01:00:00+07:00","unavailable":false}]}',
   now() - interval '2 days', NULL::timestamptz, NULL, NULL, now() - interval '3 days', now() - interval '2 days'),
  ('a2500011-0000-0000-0000-000000000003', 'a2500012-0000-0000-0000-000000000002', '2026-09-01', '2026-09-15', 'rejected', NULL,
   '{"columns":[{"label":"Total Tagihan","metric_key":"billed_idr","unavailable":true,"unit":"","error_notice":"sumber tidak tersedia"},{"label":"Total Penerimaan","metric_key":"paid_idr","value":180000000,"unit":"idr","as_of":"2026-09-16T01:00:00+07:00","unavailable":false},{"label":"Kolektibilitas","metric_key":"collection_rate","unavailable":true,"unit":"","error_notice":"sumber tidak tersedia"}]}',
   now() - interval '28 days', NULL, NULL, NULL, now() - interval '29 days', now() - interval '27 days'),
  ('a2500011-0000-0000-0000-000000000004', 'a2500012-0000-0000-0000-000000000002', '2026-09-01', '2026-09-30', 'draft', NULL,
   NULL, NULL, NULL, NULL, NULL, now() - interval '1 day', now() - interval '1 day')
) AS v(id, tpl, pf, pt, status, appr, snap, snap_at, sub_at, sub_by, note, created, updated)
JOIN regulatory_templates t ON t.id = v.tpl::uuid
WHERE NOT EXISTS (SELECT 1 FROM regulatory_reports r
                  WHERE r.template_id = t.id AND r.period_from = v.pf::date AND r.period_to = v.pt::date
                    AND r.status <> 'rejected' AND v.status <> 'rejected' AND r.id <> v.id::uuid)
ON CONFLICT DO NOTHING;

-- 5. Dapodik ---------------------------------------------------------------------
INSERT INTO dapodik_field_mappings (id, school_id, entity, dapodik_column, sekoci_field, is_match_key, match_priority, normalize, created_at, updated_at)
VALUES
  ('a2500013-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'student', 'NISN', 'nisn', true, 1, 'digits', now() - interval '90 days', now() - interval '90 days'),
  ('a2500013-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'student', 'NIK', 'nik', true, 2, 'digits', now() - interval '90 days', now() - interval '90 days'),
  ('a2500013-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'student', 'Nama', 'name', false, NULL, 'trim_upper', now() - interval '90 days', now() - interval '90 days'),
  ('a2500013-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'student', 'Tanggal Lahir', 'birth_date', false, NULL, 'date', now() - interval '90 days', now() - interval '90 days'),
  ('a2500013-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'student', 'Rombel Saat Ini', 'classroom', false, NULL, 'trim_upper', now() - interval '90 days', now() - interval '90 days'),
  ('a2500013-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111', 'rombel', 'Nama Rombel', 'name', true, 1, 'trim_upper', now() - interval '90 days', now() - interval '90 days'),
  ('a2500013-0000-0000-0000-000000000007', '11111111-1111-1111-1111-111111111111', 'rombel', 'Tingkat Pendidikan', 'grade_level', false, NULL, 'digits', now() - interval '90 days', now() - interval '90 days'),
  ('a2500013-0000-0000-0000-000000000008', '11111111-1111-1111-1111-111111111111', 'ptk', 'NUPTK', 'nuptk', true, 1, 'digits', now() - interval '90 days', now() - interval '90 days'),
  ('a2500013-0000-0000-0000-000000000009', '11111111-1111-1111-1111-111111111111', 'ptk', 'Nama', 'name', false, NULL, 'trim_upper', now() - interval '90 days', now() - interval '90 days'),
  ('a2500013-0000-0000-0000-000000000010', '11111111-1111-1111-1111-111111111111', 'ptk', 'Status Kepegawaian', 'employment_status', false, NULL, 'none', now() - interval '90 days', now() - interval '90 days')
ON CONFLICT DO NOTHING;

INSERT INTO dapodik_imports (id, school_id, entity, file_name, file_checksum, row_count, imported_by, status, created_at, updated_at)
VALUES
  ('a2500014-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'student', 'dapodik_peserta_didik_20261001.xlsx',
   encode(sha256(convert_to('seed-a25:dapodik_peserta_didik_20261001', 'UTF8')), 'hex'), 4, '22222222-2222-2222-2222-222222222222', 'compared',
   now() - interval '14 days', now() - interval '13 days'),
  ('a2500014-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'ptk', 'dapodik_ptk_20260815.xlsx',
   encode(sha256(convert_to('seed-a25:dapodik_ptk_20260815', 'UTF8')), 'hex'), 2, '22222222-2222-2222-2222-222222222222', 'closed',
   now() - interval '61 days', now() - interval '55 days'),
  ('a2500014-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'rombel', 'dapodik_rombel_20261010.csv',
   encode(sha256(convert_to('seed-a25:dapodik_rombel_20261010', 'UTF8')), 'hex'), 2, '22222222-2222-2222-2222-222222222222', 'parsed',
   now() - interval '5 days', now() - interval '5 days')
ON CONFLICT DO NOTHING;

INSERT INTO dapodik_import_rows (id, school_id, import_id, row_no, data, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', i.id, v.n, v.data::jsonb, i.created_at, i.created_at
FROM (VALUES
  ('a2500015-0000-0000-0000-000000000001', 'a2500014-0000-0000-0000-000000000001', 1,
   '{"NISN":"1234567890","NIK":"","Nama":"BUDI SANTOSO","Tanggal Lahir":"14/03/2013","Rombel Saat Ini":"KELAS 7A"}'),
  ('a2500015-0000-0000-0000-000000000002', 'a2500014-0000-0000-0000-000000000001', 2,
   '{"NISN":"0131234501","NIK":"","Nama":"SITI RAHMAWATI","Tanggal Lahir":"02/07/2013","Rombel Saat Ini":"KELAS 7A"}'),
  ('a2500015-0000-0000-0000-000000000003', 'a2500014-0000-0000-0000-000000000001', 3,
   '{"NISN":"0131234502","NIK":"","Nama":"RIZKY PRATAMA","Tanggal Lahir":"21/11/2012","Rombel Saat Ini":"KELAS 7B"}'),
  ('a2500015-0000-0000-0000-000000000004', 'a2500014-0000-0000-0000-000000000001', 4,
   '{"NISN":"0121234588","NIK":"","Nama":"AULIA PUTRI NABILA","Tanggal Lahir":"09/05/2012","Rombel Saat Ini":"KELAS 8A"}'),
  ('a2500015-0000-0000-0000-000000000005', 'a2500014-0000-0000-0000-000000000002', 1,
   '{"NUPTK":"1234567890123456","Nama":"GURU MATEMATIKA","Status Kepegawaian":"GTY/PTY"}'),
  ('a2500015-0000-0000-0000-000000000006', 'a2500014-0000-0000-0000-000000000002', 2,
   '{"NUPTK":"6543210987654321","Nama":"WALI KELAS VII A","Status Kepegawaian":"Honor Sekolah"}'),
  ('a2500015-0000-0000-0000-000000000007', 'a2500014-0000-0000-0000-000000000003', 1, '{"Nama Rombel":"KELAS 7A","Tingkat Pendidikan":"Kelas 7"}'),
  ('a2500015-0000-0000-0000-000000000008', 'a2500014-0000-0000-0000-000000000003', 2, '{"Nama Rombel":"KELAS 7B","Tingkat Pendidikan":"Kelas 7"}')
) AS v(id, imp, n, data)
JOIN dapodik_imports i ON i.id = v.imp::uuid
ON CONFLICT DO NOTHING;

INSERT INTO dapodik_diffs (id, school_id, import_id, entity, sekoci_ref, import_row_id, kind, field, sekoci_value, dapodik_value, status,
                           decision, decision_reason, decided_by, decided_at, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', i.id, i.entity, v.ref, v.row::uuid, v.kind, v.field, v.sv, v.dv, v.status,
       v.decision, v.reason, v.by::uuid, i.created_at + v.after, i.created_at, i.created_at + coalesce(v.after, interval '0')
FROM (VALUES
  ('a2500016-0000-0000-0000-000000000001', 'a2500014-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444', 'a2500015-0000-0000-0000-000000000001',
   'field_mismatch', 'birth_date', '2013-03-15', '2013-03-14', 'decided', 'update_dapodik',
   'Akta kelahiran menyatakan 15 Maret 2013; operator akan memperbaiki di Dapodik.', '22222222-2222-2222-2222-222222222222', interval '1 day'),
  ('a2500016-0000-0000-0000-000000000002', 'a2500014-0000-0000-0000-000000000001', NULL, 'a2500015-0000-0000-0000-000000000004',
   'missing_in_sekoci', NULL, NULL, 'AULIA PUTRI NABILA', 'open', NULL, NULL, NULL, NULL),
  ('a2500016-0000-0000-0000-000000000003', 'a2500014-0000-0000-0000-000000000001', NULL, 'a2500015-0000-0000-0000-000000000003',
   'field_mismatch', 'classroom', 'KELAS 7A', 'KELAS 7B', 'open', NULL, NULL, NULL, NULL),
  ('a2500016-0000-0000-0000-000000000004', 'a2500014-0000-0000-0000-000000000001', NULL, NULL,
   'missing_in_dapodik', NULL, 'NADIA KUSUMA (siswa pindahan Oktober 2026)', NULL, 'open', NULL, NULL, NULL, NULL),
  ('a2500016-0000-0000-0000-000000000005', 'a2500014-0000-0000-0000-000000000002', '66666666-6666-6666-6666-666666666666', 'a2500015-0000-0000-0000-000000000006',
   'field_mismatch', 'employment_status', 'GTY/PTY', 'Honor Sekolah', 'decided', 'update_sekoci',
   'SK pengangkatan GTY belum terbit; data sekoci disesuaikan.', '22222222-2222-2222-2222-222222222222', interval '5 days'),
  ('a2500016-0000-0000-0000-000000000006', 'a2500014-0000-0000-0000-000000000002', '55555555-5555-5555-5555-555555555555', 'a2500015-0000-0000-0000-000000000005',
   'field_mismatch', 'name', 'GURU MATEMATIKA, S.PD.', 'GURU MATEMATIKA', 'decided', 'ignore',
   'Perbedaan gelar saja.', '22222222-2222-2222-2222-222222222222', interval '5 days')
) AS v(id, imp, ref, row, kind, field, sv, dv, status, decision, reason, by, after)
JOIN dapodik_imports i ON i.id = v.imp::uuid
ON CONFLICT DO NOTHING;

INSERT INTO dapodik_sync_logs (id, school_id, entity, action, actor_user_id, import_id, summary, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', i.entity, v.action, '22222222-2222-2222-2222-222222222222', i.id, v.summary::jsonb,
       i.created_at + v.after, i.created_at + v.after
FROM (VALUES
  ('a2500017-0000-0000-0000-000000000001', 'a2500014-0000-0000-0000-000000000002', 'import', '{"rows":2}', interval '0'),
  ('a2500017-0000-0000-0000-000000000002', 'a2500014-0000-0000-0000-000000000002', 'compare', '{"field_mismatch":2,"missing_in_dapodik":0,"missing_in_sekoci":0}', interval '1 hour'),
  ('a2500017-0000-0000-0000-000000000003', 'a2500014-0000-0000-0000-000000000002', 'decide', '{"decided":2,"update_sekoci":1,"ignore":1}', interval '5 days'),
  ('a2500017-0000-0000-0000-000000000004', 'a2500014-0000-0000-0000-000000000002', 'export', '{"format":"xlsx","rows":1}', interval '6 days'),
  ('a2500017-0000-0000-0000-000000000005', 'a2500014-0000-0000-0000-000000000001', 'import', '{"rows":4}', interval '0'),
  ('a2500017-0000-0000-0000-000000000006', 'a2500014-0000-0000-0000-000000000001', 'compare', '{"field_mismatch":2,"missing_in_dapodik":1,"missing_in_sekoci":1}', interval '2 hours'),
  ('a2500017-0000-0000-0000-000000000007', 'a2500014-0000-0000-0000-000000000003', 'import', '{"rows":2}', interval '0')
) AS v(id, imp, action, summary, after)
JOIN dapodik_imports i ON i.id = v.imp::uuid
ON CONFLICT DO NOTHING;

COMMIT;
