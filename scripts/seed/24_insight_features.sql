-- 24_insight_features.sql
-- Data demo insight: parameter metrik per sekolah, riwayat snapshot (tren
-- bulanan + per kelas/gelombang), riwayat refresh, dashboard yayasan, laporan
-- tersimpan + terjadwal + riwayat kirim, akses lintas unit (scope grant),
-- pengaturan asisten, panduan asisten, percakapan asisten + aksi, pengaturan
-- prediktif, aturan risiko versi 2, prediksi + tinjauan + akurasi.
--
-- Aturan: hanya MENAMBAH (UUID tetap berawalan 'a24', ON CONFLICT DO NOTHING).
-- Satu transaksi, idempoten.
-- Sengaja kosong:
--   * bi_export_tokens  : berisi hash token akses BI (kredensial), tidak di-seed.
--   * audit_events      : log audit mentah yang diteruskan ke core oleh job.
-- Keamanan job: laporan terjadwal yang aktif dijadwalkan ke depan; tidak ada
-- aksi asisten berstatus 'confirmed' yang menunggu dieksekusi.

BEGIN;

-- 1. Parameter metrik per sekolah ---------------------------------------------
INSERT INTO metric_school_params (id, school_id, metric_key, params, effective_from, effective_to, created_at, updated_at)
VALUES
  ('a2400001-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'attendance_rate',
   '{"hari_efektif_per_minggu":5,"hitung_terlambat_sebagai_hadir":true}', '2025-07-14', '2026-07-12', now() - interval '450 days', now() - interval '95 days'),
  ('a2400001-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'attendance_rate',
   '{"hari_efektif_per_minggu":5,"hitung_terlambat_sebagai_hadir":false}', '2026-07-13', NULL, now() - interval '95 days', now() - interval '95 days'),
  ('a2400001-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'collection_rate',
   '{"tanggal_jatuh_tempo":10,"abaikan_tagihan_dispensasi":true}', '2026-07-01', NULL, now() - interval '100 days', now() - interval '100 days'),
  ('a2400001-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'outstanding_idr',
   '{"aging_bucket_hari":[30,60,90]}', '2026-07-01', NULL, now() - interval '100 days', now() - interval '100 days'),
  ('a2400001-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'ppdb_conversion',
   '{"hitung_dari_status":"offered"}', '2026-01-01', NULL, now() - interval '280 days', now() - interval '280 days')
ON CONFLICT DO NOTHING;

-- 2. Snapshot metrik: tren bulan sebelumnya + rincian dimensi -------------------
INSERT INTO metric_snapshots (school_id, metric_key, period_kind, period_start, period_end, group_by, dim_value, numerator, denominator, value, source_as_of, computed_at)
VALUES
  ('11111111-1111-1111-1111-111111111111', 'attendance_rate', 'month', '2026-07-01', '2026-07-31', '', '', 1710, 1800, 9500, '2026-08-01 01:00+07', '2026-08-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'attendance_rate', 'month', '2026-08-01', '2026-08-31', '', '', 1932, 2100, 9200, '2026-09-01 01:00+07', '2026-09-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'attendance_rate', 'month', '2026-09-01', '2026-09-30', 'grade', '7', 640, 670, 9552, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'attendance_rate', 'month', '2026-09-01', '2026-09-30', 'grade', '8', 621, 660, 9409, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'attendance_rate', 'month', '2026-09-01', '2026-09-30', 'grade', '9', 629, 670, 9388, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'collection_rate', 'month', '2026-07-01', '2026-07-31', '', '', 455, 500, 9100, '2026-08-01 01:00+07', '2026-08-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'collection_rate', 'month', '2026-08-01', '2026-08-31', '', '', 448, 500, 8960, '2026-09-01 01:00+07', '2026-09-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'billed_idr', 'month', '2026-09-01', '2026-09-30', '', '', NULL, NULL, 412250000, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'paid_idr', 'month', '2026-09-01', '2026-09-30', '', '', NULL, NULL, 362780000, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'outstanding_idr', 'month', '2026-09-01', '2026-09-30', 'aging_bucket', '0-30', NULL, NULL, 31450000, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'outstanding_idr', 'month', '2026-09-01', '2026-09-30', 'aging_bucket', '31-60', NULL, NULL, 12750000, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'outstanding_idr', 'month', '2026-09-01', '2026-09-30', 'aging_bucket', '>90', NULL, NULL, 5270000, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'ppdb_applicants', 'academic_year', '2026-07-01', '2027-06-30', 'wave', 'Gelombang 1 - SMP Kelas 7', NULL, NULL, 6, '2026-10-14 01:00+07', '2026-10-14 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'ppdb_applicants', 'academic_year', '2026-07-01', '2027-06-30', 'wave', 'Gelombang Susulan - SMP Kelas 7', NULL, NULL, 3, '2026-10-14 01:00+07', '2026-10-14 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'ppdb_conversion', 'academic_year', '2026-07-01', '2027-06-30', 'wave', 'Gelombang 1 - SMP Kelas 7', 1, 6, 1667, '2026-10-14 01:00+07', '2026-10-14 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'active_staff', 'month', '2026-09-01', '2026-09-30', '', '', NULL, NULL, 21, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'verified_achievements', 'month', '2026-09-01', '2026-09-30', 'level', 'kota', NULL, NULL, 8, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'verified_achievements', 'month', '2026-09-01', '2026-09-30', 'level', 'provinsi', NULL, NULL, 3, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('11111111-1111-1111-1111-111111111111', 'verified_achievements', 'month', '2026-09-01', '2026-09-30', 'level', 'nasional', NULL, NULL, 1, '2026-10-01 01:00+07', '2026-10-01 01:05+07')
ON CONFLICT DO NOTHING;

INSERT INTO metric_refresh_runs (id, school_id, metric_key, period_start, status, error, started_at, finished_at)
VALUES
  ('a2400002-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'attendance_rate', '2026-09-01', 'ok', NULL, '2026-10-01 01:00+07', '2026-10-01 01:05+07'),
  ('a2400002-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'collection_rate', '2026-09-01', 'ok', NULL, '2026-10-01 01:00+07', '2026-10-01 01:02+07'),
  ('a2400002-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'active_staff', '2026-09-01', 'failed',
   'Layanan hr tidak merespons dalam 30 detik', '2026-10-01 01:00+07', '2026-10-01 01:00:30+07'),
  ('a2400002-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'active_staff', '2026-09-01', 'ok', NULL, '2026-10-01 02:00+07', '2026-10-01 02:01+07'),
  ('a2400002-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'ppdb_applicants', '2026-07-01', 'ok', NULL, '2026-10-14 01:00+07', '2026-10-14 01:03+07')
ON CONFLICT DO NOTHING;

-- 3. Dashboard yayasan + widget ------------------------------------------------
INSERT INTO dashboards (id, school_id, owner_user_id, title, scope_kind, shared_roles, deleted_at, created_at, updated_at)
VALUES
  ('a2400003-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '88888888-8888-8888-8888-888888888888',
   'Dashboard Yayasan - Keuangan & Akademik', 'foundation', '{kepala_yayasan,kepala_sekolah}', NULL, now() - interval '60 days', now() - interval '10 days'),
  ('a2400003-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777',
   'Pantauan PPDB Kepala Sekolah', 'unit', '{admin}', NULL, now() - interval '40 days', now() - interval '40 days'),
  ('a2400003-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222',
   'Uji Coba Dashboard (dihapus)', 'unit', '{}', now() - interval '20 days', now() - interval '50 days', now() - interval '20 days')
ON CONFLICT DO NOTHING;

INSERT INTO dashboard_widgets (id, school_id, dashboard_id, sort, title, metric_key, chart, group_by, period, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', d.id, v.sort, v.title, v.metric, v.chart, v.grp, v.period::jsonb, d.created_at, d.created_at
FROM (VALUES
  ('a2400004-0000-0000-0000-000000000001', 'a2400003-0000-0000-0000-000000000001', 1, 'Kolektibilitas SPP Bulanan', 'collection_rate', 'line', 'month', '{"kind":"last_n_months","n":3}'),
  ('a2400004-0000-0000-0000-000000000002', 'a2400003-0000-0000-0000-000000000001', 2, 'Tunggakan per Umur', 'outstanding_idr', 'bar', 'aging_bucket', '{"kind":"latest"}'),
  ('a2400004-0000-0000-0000-000000000003', 'a2400003-0000-0000-0000-000000000001', 3, 'Penerimaan Kas', 'paid_idr', 'stat', NULL, '{"kind":"current_month"}'),
  ('a2400004-0000-0000-0000-000000000004', 'a2400003-0000-0000-0000-000000000001', 4, 'Kehadiran per Jenjang', 'attendance_rate', 'bar', 'grade', '{"kind":"latest"}'),
  ('a2400004-0000-0000-0000-000000000005', 'a2400003-0000-0000-0000-000000000002', 1, 'Pendaftar per Gelombang', 'ppdb_applicants', 'bar', 'wave', '{"kind":"academic_year"}'),
  ('a2400004-0000-0000-0000-000000000006', 'a2400003-0000-0000-0000-000000000002', 2, 'Konversi PPDB', 'ppdb_conversion', 'table', 'wave', '{"kind":"academic_year"}')
) AS v(id, dash, sort, title, metric, chart, grp, period)
JOIN dashboards d ON d.id = v.dash::uuid
ON CONFLICT DO NOTHING;

-- 4. Laporan tersimpan, terjadwal, riwayat kirim -------------------------------
INSERT INTO saved_reports (id, school_id, owner_user_id, title, columns, filters, shared_roles, deleted_at, created_at, updated_at)
VALUES
  ('a2400005-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '88888888-8888-8888-8888-888888888888', 'Ringkasan Keuangan Bulanan Yayasan',
   '[{"name":"metric","label":"Metrik"},{"name":"period","label":"Periode"},{"name":"value","label":"Nilai"}]',
   '{"metric_keys":["billed_idr","paid_idr","collection_rate"],"period":"last_month"}', '{kepala_yayasan,finance_manager}', NULL, now() - interval '60 days', now() - interval '60 days'),
  ('a2400005-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777', 'Kehadiran Siswa per Kelas',
   '[{"name":"grade","label":"Jenjang"},{"name":"value","label":"Kehadiran (%)"}]',
   '{"metric_keys":["attendance_rate"],"group_by":"grade","period":"last_month"}', '{kepala_sekolah,walikelas}', NULL, now() - interval '45 days', now() - interval '45 days'),
  ('a2400005-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777', 'Rekap PPDB per Gelombang',
   '[{"name":"wave","label":"Gelombang"},{"name":"value","label":"Pendaftar"}]',
   '{"metric_keys":["ppdb_applicants"],"group_by":"wave"}', '{}', NULL, now() - interval '30 days', now() - interval '30 days')
ON CONFLICT DO NOTHING;

INSERT INTO scheduled_reports (id, school_id, saved_report_id, frequency, weekday, month_day, send_hour, recipient_user_ids, active, next_run_at, last_run_at,
                               created_by, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', r.id, v.freq, v.wd, v.md, v.hour, v.recips::uuid[], v.active, v.next_run, v.last_run,
       r.owner_user_id, r.created_at, r.created_at
FROM (VALUES
  ('a2400006-0000-0000-0000-000000000001', 'a2400005-0000-0000-0000-000000000001', 'monthly', NULL::int, 1, 7,
   '{88888888-8888-8888-8888-888888888888,77777777-7777-7777-7777-777777777777}', true, date_trunc('month', now()) + interval '1 month 7 hours', date_trunc('month', now()) + interval '7 hours'),
  ('a2400006-0000-0000-0000-000000000002', 'a2400005-0000-0000-0000-000000000002', 'weekly', 1, NULL, 6,
   '{77777777-7777-7777-7777-777777777777}', false, now() + interval '3 days', now() - interval '18 days')
) AS v(id, report, freq, wd, md, hour, recips, active, next_run, last_run)
JOIN saved_reports r ON r.id = v.report::uuid
ON CONFLICT DO NOTHING;

INSERT INTO scheduled_report_runs (id, school_id, schedule_id, recipient_user_id, status, reason, deliveries, idempotency_key, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', s.id, v.recip::uuid, v.status, v.reason, v.deliv::jsonb, 'seed-a24:' || v.id, v.at
FROM (VALUES
  ('a2400007-0000-0000-0000-000000000001', 'a2400006-0000-0000-0000-000000000001', '88888888-8888-8888-8888-888888888888', 'sent', NULL, '{"in_app":"sent","email":"sent"}', now() - interval '45 days'),
  ('a2400007-0000-0000-0000-000000000002', 'a2400006-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777', 'sent', NULL, '{"in_app":"sent"}', now() - interval '45 days'),
  ('a2400007-0000-0000-0000-000000000003', 'a2400006-0000-0000-0000-000000000001', '88888888-8888-8888-8888-888888888888', 'sent', NULL, '{"in_app":"sent","email":"sent"}', now() - interval '14 days'),
  ('a2400007-0000-0000-0000-000000000004', 'a2400006-0000-0000-0000-000000000001', '77777777-7777-7777-7777-777777777777', 'failed', 'Email penerima tidak valid', '{"in_app":"sent","email":"failed"}', now() - interval '14 days'),
  ('a2400007-0000-0000-0000-000000000005', 'a2400006-0000-0000-0000-000000000002', '77777777-7777-7777-7777-777777777777', 'skipped', 'Belum ada data kehadiran minggu ini', NULL, now() - interval '18 days')
) AS v(id, sched, recip, status, reason, deliv, at)
JOIN scheduled_reports s ON s.id = v.sched::uuid
ON CONFLICT DO NOTHING;

-- 5. Akses analitik lintas unit untuk ketua yayasan ----------------------------
INSERT INTO analytics_scope_grants (id, school_id, user_id, granted_school_id, granted_by, revoked_at, created_at, updated_at)
VALUES
  ('a2400008-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '88888888-8888-8888-8888-888888888888',
   '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', NULL, now() - interval '60 days', now() - interval '60 days'),
  ('a2400008-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777',
   '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', now() - interval '30 days', now() - interval '90 days', now() - interval '30 days')
ON CONFLICT DO NOTHING;

-- 6. Asisten: pengaturan, panduan, percakapan, aksi ---------------------------
INSERT INTO assistant_settings (school_id, log_retention_days, created_at, updated_at)
VALUES ('11111111-1111-1111-1111-111111111111', 180, now() - interval '60 days', now() - interval '60 days')
ON CONFLICT DO NOTHING;

INSERT INTO help_articles (id, school_id, title, body_md, tags, route_name, status, updated_by, created_at, updated_at)
VALUES
  ('a2400009-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Membaca Dashboard Kolektibilitas SPP',
   E'# Kolektibilitas SPP\n\nNilai kolektibilitas = pembayaran teralokasi / tagihan jatuh tempo.\nGunakan widget **Tunggakan per Umur** untuk melihat tagihan yang lewat 30, 60 dan 90 hari.',
   '{keuangan,dashboard,metrik}', 'insight-dashboards', 'published', '22222222-2222-2222-2222-222222222222', now() - interval '50 days', now() - interval '50 days'),
  ('a2400009-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'Menjadwalkan Laporan Otomatis',
   E'# Laporan Terjadwal\n\n1. Simpan laporan dari menu **Laporan**.\n2. Pilih **Jadwalkan**, tentukan frekuensi dan penerima.\n3. Riwayat pengiriman tampil di tab **Riwayat**.',
   '{laporan,jadwal}', 'insight-reports', 'published', '22222222-2222-2222-2222-222222222222', now() - interval '40 days', now() - interval '40 days'),
  ('a2400009-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'Memahami Skor Risiko Tunggakan',
   E'# Skor Risiko\n\nSkor prediksi adalah bahan tinjauan, bukan keputusan. Setiap prediksi wajib ditinjau petugas sebelum ditindaklanjuti.',
   '{prediksi,risiko}', 'insight-predictions', 'published', '22222222-2222-2222-2222-222222222222', now() - interval '30 days', now() - interval '30 days'),
  ('a2400009-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'Panduan Asisten untuk Orang Tua (draf)',
   E'# Asisten Orang Tua\n\nTanyakan "tagihan anak saya" untuk melihat status SPP.',
   '{asisten,orang_tua}', NULL, 'draft', '22222222-2222-2222-2222-222222222222', now() - interval '3 days', now() - interval '3 days')
ON CONFLICT DO NOTHING;

INSERT INTO assistant_conversations (id, school_id, user_id, active_role, started_at, last_message_at)
VALUES
  ('a2400010-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 'orang_tua', now() - interval '3 days', now() - interval '3 days' + interval '2 minutes'),
  ('a2400010-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'admin', now() - interval '2 days', now() - interval '2 days' + interval '5 minutes'),
  ('a2400010-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777', 'kepala_sekolah', now() - interval '1 day', now() - interval '1 day' + interval '3 minutes')
ON CONFLICT DO NOTHING;

INSERT INTO assistant_messages (id, school_id, conversation_id, sender, content, ai_generated, sources, tool_calls, provider, latency_ms, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', c.id, v.sender, v.content, v.ai, v.sources::jsonb, v.tools::jsonb, v.provider, v.latency, c.started_at + v.after
FROM (VALUES
  ('a2400011-0000-0000-0000-000000000001', 'a2400010-0000-0000-0000-000000000001', 'user', 'tagihan anak saya bulan ini berapa?', false, '[]', '[]', NULL, NULL, interval '0'),
  ('a2400011-0000-0000-0000-000000000002', 'a2400010-0000-0000-0000-000000000001', 'assistant',
   'Tagihan SPP November 2026 untuk Budi Santoso sebesar Rp850.000, jatuh tempo 10 November 2026. Tidak ada tunggakan bulan sebelumnya.',
   false, '[{"kind":"tool","key":"child_bills"}]', '[{"tool":"child_bills","status":"ok"}]', 'rules', 210, interval '1 minute'),
  ('a2400011-0000-0000-0000-000000000003', 'a2400010-0000-0000-0000-000000000002', 'user', 'kirim pengingat spp ke penunggak lebih dari 30 hari', false, '[]', '[]', NULL, NULL, interval '0'),
  ('a2400011-0000-0000-0000-000000000004', 'a2400010-0000-0000-0000-000000000002', 'assistant',
   'Ada 14 orang tua dengan tunggakan lebih dari 30 hari. Konfirmasi untuk mengirim pengingat lewat notifikasi aplikasi?',
   true, '[{"kind":"metric","key":"outstanding_idr"}]', '[{"tool":"send_payment_reminders","status":"proposed"}]', 'llm', 1840, interval '1 minute'),
  ('a2400011-0000-0000-0000-000000000005', 'a2400010-0000-0000-0000-000000000003', 'user', 'buat draf pengumuman libur maulid', false, '[]', '[]', NULL, NULL, interval '0'),
  ('a2400011-0000-0000-0000-000000000006', 'a2400010-0000-0000-0000-000000000003', 'assistant',
   'Berikut draf pengumuman: "Sehubungan dengan peringatan Maulid Nabi Muhammad SAW, kegiatan belajar mengajar diliburkan..." Silakan tinjau sebelum diterbitkan.',
   true, '[]', '[{"tool":"draft_announcement","status":"proposed"}]', 'llm', 2310, interval '2 minutes')
) AS v(id, conv, sender, content, ai, sources, tools, provider, latency, after)
JOIN assistant_conversations c ON c.id = v.conv::uuid
ON CONFLICT DO NOTHING;

INSERT INTO assistant_actions (id, school_id, conversation_id, message_id, action_key, payload, preview, status, proposed_by, confirmed_at, executed_at,
                               result, expires_at, idempotency_key, created_at, updated_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', m.conversation_id, m.id, v.action, v.payload::jsonb, v.preview::jsonb, v.status, v.by::uuid,
       v.confirmed, v.executed, v.result::jsonb, m.created_at + interval '15 minutes', 'seed-a24:' || v.id, m.created_at, m.created_at + interval '5 minutes'
FROM (VALUES
  ('a2400012-0000-0000-0000-000000000001', 'a2400011-0000-0000-0000-000000000004', 'send_payment_reminders', '{"min_days_overdue":30,"channel":"in_app"}',
   '{"recipients":14,"channel":"in_app"}', 'expired', '22222222-2222-2222-2222-222222222222', NULL::timestamptz, NULL::timestamptz, NULL),
  ('a2400012-0000-0000-0000-000000000002', 'a2400011-0000-0000-0000-000000000006', 'draft_announcement', '{"title":"Libur Maulid Nabi"}',
   '{"title":"Libur Maulid Nabi Muhammad SAW"}', 'cancelled', '77777777-7777-7777-7777-777777777777', NULL, NULL, '{"reason":"Dibatalkan pengguna"}')
) AS v(id, msg, action, payload, preview, status, by, confirmed, executed, result)
JOIN assistant_messages m ON m.id = v.msg::uuid
ON CONFLICT DO NOTHING;

-- 7. Prediktif: pengaturan, aturan v2, run, prediksi, tinjauan, akurasi ---------
INSERT INTO insight_settings (school_id, predictive_enabled, enabled_by, enabled_at, review_note, created_at, updated_at)
VALUES ('11111111-1111-1111-1111-111111111111', true, '88888888-8888-8888-8888-888888888888', now() - interval '60 days',
        'Diaktifkan untuk uji coba satu semester; hasil hanya sebagai bahan tinjauan wali kelas dan keuangan.', now() - interval '60 days', now() - interval '60 days')
ON CONFLICT DO NOTHING;

INSERT INTO risk_rules (id, school_id, model, version, params, effective_from, created_by, status, created_at, updated_at)
VALUES
  ('a2400013-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'ppdb_forecast', 1,
   '{"lookback_years":2,"seasonality":"wave"}', '2026-01-01', '22222222-2222-2222-2222-222222222222', 'active', now() - interval '280 days', now() - interval '280 days'),
  ('a2400013-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'cashflow_forecast', 1,
   '{"horizon_months":3,"include_ppdb":true}', '2026-07-01', '22222222-2222-2222-2222-222222222222', 'active', now() - interval '100 days', now() - interval '100 days'),
  ('a2400013-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'arrears_risk', 99,
   '{"threshold_days":45,"min_amount":850000}', '2025-07-01', '22222222-2222-2222-2222-222222222222', 'retired', now() - interval '450 days', now() - interval '100 days')
ON CONFLICT DO NOTHING;

INSERT INTO prediction_runs (id, school_id, model, rule_id, run_at, status, reason, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', r.model, r.id, now() - v.age, v.status, v.reason, now() - v.age
FROM (VALUES
  ('a2400014-0000-0000-0000-000000000001', 'a2400013-0000-0000-0000-000000000001', interval '7 days', 'ok', NULL),
  ('a2400014-0000-0000-0000-000000000002', 'a2400013-0000-0000-0000-000000000002', interval '7 days', 'ok', NULL),
  ('a2400014-0000-0000-0000-000000000003', 'a2400013-0000-0000-0000-000000000002', interval '37 days', 'failed', 'Data penerimaan billing belum tersedia untuk Agustus'),
  ('a2400014-0000-0000-0000-000000000004', 'a2400013-0000-0000-0000-000000000001', interval '14 days', 'skipped', 'Tidak ada gelombang aktif')
) AS v(id, rule, age, status, reason)
JOIN risk_rules r ON r.id = v.rule::uuid
ON CONFLICT DO NOTHING;

INSERT INTO predictions (id, school_id, run_id, model, subject_kind, subject_id, score, level, projected_value, factors, review_label, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', pr.id, pr.model, v.kind, v.subject, v.score, v.level, v.proj, v.factors::jsonb, v.label, pr.run_at
FROM (VALUES
  ('a2400015-0000-0000-0000-000000000001', 'a2400014-0000-0000-0000-000000000001', 'wave', 'Gelombang 1 2027/2028', NULL::int, NULL, 58::bigint,
   '[{"factor":"Rata-rata 2 tahun","note":"Gelombang 1 rata-rata 55 pendaftar"}]', 'perlu_tinjauan'),
  ('a2400015-0000-0000-0000-000000000002', 'a2400014-0000-0000-0000-000000000001', 'wave', 'Gelombang 2 2027/2028', NULL, NULL, 34,
   '[{"factor":"Tren kunjungan landing page","note":"Naik 12% dibanding tahun lalu"}]', 'perlu_tinjauan'),
  ('a2400015-0000-0000-0000-000000000003', 'a2400014-0000-0000-0000-000000000002', 'month', '2026-11', NULL, NULL, 371500000,
   '[{"factor":"Kolektibilitas 3 bulan","note":"Rata-rata 89,9%"}]', 'perlu_tinjauan'),
  ('a2400015-0000-0000-0000-000000000004', 'a2400014-0000-0000-0000-000000000002', 'month', '2026-12', NULL, NULL, 352000000,
   '[{"factor":"Libur semester","note":"Pembayaran Desember historis turun 5%"}]', 'sudah_ditinjau'),
  ('a2400015-0000-0000-0000-000000000005', 'a2400014-0000-0000-0000-000000000002', 'month', '2027-01', NULL, NULL, 395000000,
   '[{"factor":"Tagihan semester genap","note":"Termasuk biaya kegiatan semester"}]', 'perlu_tinjauan')
) AS v(id, run, kind, subject, score, level, proj, factors, label)
JOIN prediction_runs pr ON pr.id = v.run::uuid
ON CONFLICT DO NOTHING;

INSERT INTO prediction_reviews (id, school_id, prediction_id, reviewed_by, note, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', p.id, v.by::uuid, v.note, p.created_at + interval '1 day'
FROM (VALUES
  ('a2400016-0000-0000-0000-000000000001', 'a2400015-0000-0000-0000-000000000004', '88888888-8888-8888-8888-888888888888',
   'Sesuai pola tahun lalu; siapkan dana talangan operasional Desember.'),
  ('a2400016-0000-0000-0000-000000000002', '55000000-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222',
   'Sudah dikonfirmasi ke wali; pembayaran rutin tiap tanggal 5. Risiko rendah.'),
  ('a2400016-0000-0000-0000-000000000003', '55000000-0000-0000-0000-000000000002', '66666666-6666-6666-6666-666666666666',
   'Kehadiran Budi baik; tidak perlu tindak lanjut.')
) AS v(id, pred, by, note)
JOIN predictions p ON p.id = v.pred::uuid
ON CONFLICT DO NOTHING;

INSERT INTO prediction_accuracy (id, school_id, model, rule_id, period_start, period_end, measure, value_bp, sample_size, computed_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', r.model, r.id, v.ps::date, v.pe::date, v.measure, v.val, v.n, v.pe::date + 1
FROM (VALUES
  ('a2400017-0000-0000-0000-000000000001', 'a2400013-0000-0000-0000-000000000003', '2025-07-01', '2025-12-31', 'precision_bp', 7200, 64),
  ('a2400017-0000-0000-0000-000000000002', 'a2400013-0000-0000-0000-000000000003', '2025-07-01', '2025-12-31', 'recall_bp', 6100, 64),
  ('a2400017-0000-0000-0000-000000000003', 'a2400013-0000-0000-0000-000000000003', '2026-01-01', '2026-06-30', 'precision_bp', 7650, 71),
  ('a2400017-0000-0000-0000-000000000004', 'a2400013-0000-0000-0000-000000000001', '2026-01-01', '2026-06-30', 'mape_bp', 1150, 4),
  ('a2400017-0000-0000-0000-000000000005', 'a2400013-0000-0000-0000-000000000002', '2026-07-01', '2026-09-30', 'mape_bp', 640, 3)
) AS v(id, rule, ps, pe, measure, val, n)
JOIN risk_rules r ON r.id = v.rule::uuid
ON CONFLICT DO NOTHING;

COMMIT;
