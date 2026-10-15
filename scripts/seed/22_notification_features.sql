-- 22_notification_features.sql
-- Data demo notification: template notifikasi (versi), template WhatsApp,
-- segmen + kampanye (draft/terkirim/dibatalkan/gagal) beserta ledger penerima,
-- persetujuan pemasaran + riwayatnya, pengaturan kampanye & chat, chat
-- guru-orang tua dengan laporan pesan, notifikasi approval & riwayat kirim.
--
-- Aturan: hanya MENAMBAH (UUID tetap berawalan 'a22', ON CONFLICT DO NOTHING).
-- Satu transaksi, idempoten.
-- Keamanan:
--   * Tidak ada kampanye 'scheduled'/'sending' dan tidak ada penerima 'queued',
--     sehingga job pengirim kampanye tidak mengirim pesan apa pun.
--   * Template hanya memakai variabel yang diizinkan ({{title}}, {{body}},
--     {{url}}, ...). Bila nilai hilang, layanan memakai teks pengirim.
--   * push_subscriptions sengaja kosong: isinya endpoint + kunci WebPush
--     perangkat nyata (setara token), tidak boleh di-seed.

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. Template notifikasi (key = jenis kejadian). Versi 2 'approval' in_app
--    adalah versi aktif; versi 1 menjadi riwayat.
-- ---------------------------------------------------------------------------
INSERT INTO notification_templates (id, school_id, key, version, channel, subject, body, created_at, updated_at)
VALUES
  ('a2200001-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'bill_issued', 1, 'in_app',
   'Tagihan baru: {{title}}', '{{body}} Silakan cek detailnya di menu Tagihan.', now() - interval '60 days', now() - interval '60 days'),
  ('a2200001-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'bill_issued', 1, 'email',
   '[Sekolah Teratai] {{title}}',
   'Yth. Bapak/Ibu Orang Tua/Wali,

{{body}}

Detail tagihan dapat dilihat di portal orang tua: {{url}}

Hormat kami,
Bagian Keuangan Sekolah Teratai', now() - interval '60 days', now() - interval '60 days'),
  ('a2200001-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'receipt', 1, 'email',
   '[Sekolah Teratai] Kuitansi pembayaran',
   'Terima kasih, pembayaran Anda telah kami terima.

{{body}}

Unduh kuitansi: {{url}}', now() - interval '60 days', now() - interval '60 days'),
  ('a2200001-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'approval', 1, 'in_app',
   'Perlu persetujuan: {{title}}', '{{body}}', now() - interval '45 days', now() - interval '45 days'),
  ('a2200001-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'approval', 2, 'in_app',
   'Menunggu persetujuan Anda: {{title}}', '{{body}} Buka kotak masuk approval untuk menyetujui atau menolak.', now() - interval '10 days', now() - interval '10 days'),
  ('a2200001-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111', 'integration_exception', 1, 'email',
   '[Galat Integrasi] {{title}}',
   'Terjadi galat integrasi yang perlu ditindaklanjuti.

{{body}}

Correlation ID: {{correlation_id}}
Buka antrean galat: {{url}}', now() - interval '30 days', now() - interval '30 days')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2. Template WhatsApp (cermin dari provider; id provider fiktif).
-- ---------------------------------------------------------------------------
INSERT INTO wa_templates (id, school_id, name, provider_template_id, language, body_preview, variables, approval_status, created_at, updated_at, deleted_at)
VALUES
  ('a2200002-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Undangan Open House PPDB', 'teratai_open_house_2027', 'id',
   'Halo {{1}}, Sekolah Teratai mengundang Anda ke Open House PPDB 2027/2028 pada {{2}}. Daftar di {{3}}.', '["nama","tanggal","tautan"]',
   'approved', now() - interval '50 days', now() - interval '48 days', NULL),
  ('a2200002-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'Pengingat Daftar Ulang', 'teratai_daftar_ulang_v2', 'id',
   'Yth. {{1}}, batas daftar ulang calon siswa {{2}} adalah {{3}}. Abaikan bila sudah menyelesaikan.', '["nama","nama_siswa","batas"]',
   'approved', now() - interval '40 days', now() - interval '39 days', NULL),
  ('a2200002-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'Info Pentas Seni Akhir Tahun', 'teratai_pentas_seni', 'id',
   'Halo {{1}}, saksikan Pentas Seni Sekolah Teratai {{2}}. Gratis untuk keluarga siswa.', '["nama","tanggal"]',
   'pending', now() - interval '3 days', now() - interval '3 days', NULL),
  ('a2200002-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'Promo Diskon Kakak Beradik', 'teratai_promo_saudara', 'id',
   'Daftarkan adik ananda sekarang dan dapatkan potongan uang pangkal {{1}}!', '["persen"]',
   'rejected', now() - interval '20 days', now() - interval '18 days', NULL),
  ('a2200002-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'Undangan Open House 2026 (lama)', 'teratai_open_house_2026', 'id',
   'Halo {{1}}, kunjungi Open House Sekolah Teratai {{2}}.', '["nama","tanggal"]',
   'approved', now() - interval '400 days', now() - interval '300 days', now() - interval '300 days')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Pengaturan kampanye & chat (hanya bila sekolah belum mengatur)
-- ---------------------------------------------------------------------------
INSERT INTO campaign_settings (school_id, monthly_wa_quota, updated_at)
VALUES ('11111111-1111-1111-1111-111111111111', 2000, now() - interval '60 days')
ON CONFLICT DO NOTHING;

INSERT INTO chat_settings (school_id, service_start, service_end, service_days, max_attachment_bytes, updated_at)
VALUES ('11111111-1111-1111-1111-111111111111', '07:00', '16:00', '{1,2,3,4,5}', 5242880, now() - interval '60 days')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 4. Segmen penerima
-- ---------------------------------------------------------------------------
INSERT INTO campaign_segments (id, school_id, name, kind, filter, created_by, created_at, updated_at, deleted_at)
VALUES
  ('a2200003-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Orang Tua Kelas 7', 'parents',
   '{"grade_levels":["7"]}', '22222222-2222-2222-2222-222222222222', now() - interval '45 days', now() - interval '45 days', NULL),
  ('a2200003-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'Orang Tua Kelas 7-9', 'parents',
   '{"grade_levels":["7","8","9"]}', '22222222-2222-2222-2222-222222222222', now() - interval '45 days', now() - interval '45 days', NULL),
  ('a2200003-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'Pendaftar PPDB Belum Daftar Ulang', 'ppdb_applicants',
   '{"statuses":["accepted"]}', '22222222-2222-2222-2222-222222222222', now() - interval '30 days', now() - interval '30 days', NULL),
  ('a2200003-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'Calon Pendaftar PPDB (semua)', 'ppdb_applicants',
   '{}', '22222222-2222-2222-2222-222222222222', now() - interval '50 days', now() - interval '50 days', NULL),
  ('a2200003-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'Orang Tua Alumni 2025', 'alumni_parents',
   '{}', '22222222-2222-2222-2222-222222222222', now() - interval '90 days', now() - interval '20 days', now() - interval '20 days')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 5. Kampanye (tanpa status scheduled/sending agar tidak ada pengiriman)
-- ---------------------------------------------------------------------------
INSERT INTO campaigns (id, school_id, name, segment_id, channel, template_id, template_vars, subject, body, scheduled_at, status,
                       last_error, created_by, created_at, updated_at, sent_at)
VALUES
  ('a2200004-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Undangan Open House PPDB 2027',
   'a2200003-0000-0000-0000-000000000004', 'whatsapp', 'a2200002-0000-0000-0000-000000000001',
   '{"tanggal":"Sabtu, 14 November 2026","tautan":"teratai.sch.id/ppdb"}', NULL, NULL, now() - interval '20 days', 'sent',
   NULL, '22222222-2222-2222-2222-222222222222', now() - interval '25 days', now() - interval '20 days', now() - interval '20 days'),
  ('a2200004-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'Info Pekan Literasi Kelas 7',
   'a2200003-0000-0000-0000-000000000001', 'in_app', NULL, '{}', 'Pekan Literasi Kelas 7',
   'Pekan Literasi berlangsung 2-6 November 2026. Ananda diminta membawa satu buku bacaan favorit setiap hari.',
   NULL, 'sent', NULL, '22222222-2222-2222-2222-222222222222', now() - interval '8 days', now() - interval '7 days', now() - interval '7 days'),
  ('a2200004-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'Pengingat Daftar Ulang Gelombang 2',
   'a2200003-0000-0000-0000-000000000003', 'whatsapp', 'a2200002-0000-0000-0000-000000000002',
   '{"batas":"31 Oktober 2026"}', NULL, NULL, NULL, 'draft', NULL,
   '22222222-2222-2222-2222-222222222222', now() - interval '1 day', now() - interval '1 day', NULL),
  ('a2200004-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'Newsletter Oktober Orang Tua',
   'a2200003-0000-0000-0000-000000000002', 'email', NULL, '{}', 'Kabar Sekolah Teratai - Oktober 2026',
   'Yth. Bapak/Ibu, berikut kabar kegiatan sekolah bulan Oktober: lomba sains, kunjungan museum, dan jadwal PTS.',
   now() - interval '5 days', 'cancelled', NULL, '22222222-2222-2222-2222-222222222222', now() - interval '9 days', now() - interval '6 days', NULL),
  ('a2200004-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'Undangan Rapat Komite Semester Ganjil',
   'a2200003-0000-0000-0000-000000000002', 'email', NULL, '{}', 'Undangan Rapat Komite Sekolah',
   'Mohon kehadiran Bapak/Ibu pada rapat komite hari Sabtu pukul 09.00 WIB di aula sekolah.',
   now() - interval '15 days', 'failed', 'Adaptor email belum dikonfigurasi untuk sekolah ini',
   '22222222-2222-2222-2222-222222222222', now() - interval '16 days', now() - interval '15 days', NULL)
ON CONFLICT DO NOTHING;

-- Ledger penerima: kontak hanya tersimpan tersamar + hash (kontak fiktif).
INSERT INTO campaign_recipients (id, school_id, campaign_id, subject_key, user_id, display_name, contact_masked, contact_hash, status,
                                 exclude_reason, provider_message_id, notification_id, last_error, created_at, sent_at, delivered_at, read_at, replied_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', c.id, v.subject_key, v.user_id::uuid, v.name, v.masked,
       CASE WHEN v.contact = '' THEN '' ELSE encode(sha256(convert_to(v.contact, 'UTF8')), 'hex') END,
       v.status, v.reason, v.provider_msg, NULL, v.err, c.created_at,
       CASE WHEN v.status IN ('sent','delivered','read','replied') THEN c.sent_at END,
       CASE WHEN v.status IN ('delivered','read','replied') THEN c.sent_at + interval '1 minute' END,
       CASE WHEN v.status IN ('read','replied') THEN c.sent_at + interval '2 hours' END,
       CASE WHEN v.status = 'replied' THEN c.sent_at + interval '3 hours' END
FROM (VALUES
  ('a2200005-0000-0000-0000-000000000001', 'a2200004-0000-0000-0000-000000000001', 'phone:' || encode(sha256(convert_to('6281200000101', 'UTF8')), 'hex'),
   NULL, 'Ibu Ratna Dewi', '6281*******01', '6281200000101', 'replied', NULL, 'wamid.seed-a22-0001', NULL),
  ('a2200005-0000-0000-0000-000000000002', 'a2200004-0000-0000-0000-000000000001', 'phone:' || encode(sha256(convert_to('6281200000102', 'UTF8')), 'hex'),
   NULL, 'Bapak Hendra Saputra', '6281*******02', '6281200000102', 'read', NULL, 'wamid.seed-a22-0002', NULL),
  ('a2200005-0000-0000-0000-000000000003', 'a2200004-0000-0000-0000-000000000001', 'phone:' || encode(sha256(convert_to('6281200000103', 'UTF8')), 'hex'),
   NULL, 'Ibu Siti Aminah', '6281*******03', '6281200000103', 'delivered', NULL, 'wamid.seed-a22-0003', NULL),
  ('a2200005-0000-0000-0000-000000000004', 'a2200004-0000-0000-0000-000000000001', 'phone:' || encode(sha256(convert_to('6281200000104', 'UTF8')), 'hex'),
   NULL, 'Bapak Yusuf Hidayat', '6281*******04', '6281200000104', 'excluded', 'opted_out', NULL, NULL),
  ('a2200005-0000-0000-0000-000000000005', 'a2200004-0000-0000-0000-000000000001', 'phone:' || encode(sha256(convert_to('6281200000105', 'UTF8')), 'hex'),
   NULL, 'Ibu Maya Lestari', '6281*******05', '6281200000105', 'failed', NULL, NULL, 'Nomor tidak terdaftar di WhatsApp'),
  ('a2200005-0000-0000-0000-000000000006', 'a2200004-0000-0000-0000-000000000001', 'phone:' || encode(sha256(convert_to('6281200000106', 'UTF8')), 'hex'),
   NULL, 'Bapak Agus Salim', '6281*******06', '6281200000106', 'excluded', 'no_consent', NULL, NULL),
  ('a2200005-0000-0000-0000-000000000007', 'a2200004-0000-0000-0000-000000000002', 'user:33333333-3333-3333-3333-333333333333',
   '33333333-3333-3333-3333-333333333333', 'Wali Budi Santoso', '', '', 'read', NULL, NULL, NULL),
  ('a2200005-0000-0000-0000-000000000008', 'a2200004-0000-0000-0000-000000000002', 'user:a2200000-0000-0000-0000-000000000099',
   NULL, 'Orang Tua Tanpa Akun', '', '', 'excluded', 'no_contact', NULL, NULL),
  ('a2200005-0000-0000-0000-000000000009', 'a2200004-0000-0000-0000-000000000005', 'user:33333333-3333-3333-3333-333333333333',
   '33333333-3333-3333-3333-333333333333', 'Wali Budi Santoso', 'w***@teratai.sch.id', 'wali@teratai.sch.id', 'failed', NULL, NULL,
   'Adaptor email belum dikonfigurasi')
) AS v(id, campaign_id, subject_key, user_id, name, masked, contact, status, reason, provider_msg, err)
JOIN campaigns c ON c.id = v.campaign_id::uuid
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 6. Persetujuan pemasaran + riwayat
-- ---------------------------------------------------------------------------
INSERT INTO marketing_consents (id, school_id, subject_key, user_id, channel, status, source, changed_at)
VALUES
  ('a2200006-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'user:33333333-3333-3333-3333-333333333333',
   '33333333-3333-3333-3333-333333333333', 'whatsapp', 'opted_in', 'preferences', now() - interval '60 days'),
  ('a2200006-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'user:33333333-3333-3333-3333-333333333333',
   '33333333-3333-3333-3333-333333333333', 'email', 'opted_out', 'unsubscribe_link', now() - interval '10 days'),
  ('a2200006-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'phone:' || encode(sha256(convert_to('6281200000101', 'UTF8')), 'hex'),
   NULL, 'whatsapp', 'opted_in', 'ppdb_form', now() - interval '40 days'),
  ('a2200006-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'phone:' || encode(sha256(convert_to('6281200000102', 'UTF8')), 'hex'),
   NULL, 'whatsapp', 'opted_in', 'ppdb_form', now() - interval '38 days'),
  ('a2200006-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'phone:' || encode(sha256(convert_to('6281200000103', 'UTF8')), 'hex'),
   NULL, 'whatsapp', 'opted_in', 'admin_import', now() - interval '35 days'),
  ('a2200006-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111', 'phone:' || encode(sha256(convert_to('6281200000104', 'UTF8')), 'hex'),
   NULL, 'whatsapp', 'opted_out', 'wa_keyword', now() - interval '22 days')
ON CONFLICT DO NOTHING;

INSERT INTO marketing_consent_events (id, school_id, subject_key, channel, status, source, actor_user_id, created_at)
VALUES
  ('a2200007-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'user:33333333-3333-3333-3333-333333333333',
   'whatsapp', 'opted_in', 'preferences', '33333333-3333-3333-3333-333333333333', now() - interval '60 days'),
  ('a2200007-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'user:33333333-3333-3333-3333-333333333333',
   'email', 'opted_in', 'preferences', '33333333-3333-3333-3333-333333333333', now() - interval '60 days'),
  ('a2200007-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'user:33333333-3333-3333-3333-333333333333',
   'email', 'opted_out', 'unsubscribe_link', NULL, now() - interval '10 days'),
  ('a2200007-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'phone:' || encode(sha256(convert_to('6281200000101', 'UTF8')), 'hex'),
   'whatsapp', 'opted_in', 'ppdb_form', NULL, now() - interval '40 days'),
  ('a2200007-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'phone:' || encode(sha256(convert_to('6281200000104', 'UTF8')), 'hex'),
   'whatsapp', 'opted_in', 'ppdb_form', NULL, now() - interval '45 days'),
  ('a2200007-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111', 'phone:' || encode(sha256(convert_to('6281200000104', 'UTF8')), 'hex'),
   'whatsapp', 'opted_out', 'wa_keyword', NULL, now() - interval '22 days'),
  ('a2200007-0000-0000-0000-000000000007', '11111111-1111-1111-1111-111111111111', 'phone:' || encode(sha256(convert_to('6281200000103', 'UTF8')), 'hex'),
   'whatsapp', 'opted_in', 'admin_import', '22222222-2222-2222-2222-222222222222', now() - interval '35 days')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 7. Chat guru-orang tua (wali kelas 7A <-> wali Budi) + laporan pesan
-- ---------------------------------------------------------------------------
INSERT INTO chat_threads (id, school_id, subject, created_by, created_at, closed_at, kind, student_user_id, teacher_user_id, guardian_user_id, archived_at)
VALUES
  ('a2200008-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Budi Santoso - Wali Kelas 7A',
   '33333333-3333-3333-3333-333333333333', now() - interval '12 days', NULL, 'teacher_parent',
   '44444444-4444-4444-4444-444444444444', '66666666-6666-6666-6666-666666666666', '33333333-3333-3333-3333-333333333333', NULL),
  ('a2200008-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'Koordinasi Panitia Pentas Seni',
   '22222222-2222-2222-2222-222222222222', now() - interval '30 days', now() - interval '5 days', 'general',
   NULL, NULL, NULL, now() - interval '5 days')
ON CONFLICT DO NOTHING;

INSERT INTO chat_thread_participants (id, thread_id, user_id, role, joined_at)
SELECT v.id::uuid, t.id, v.user_id::uuid, v.role, t.created_at
FROM (VALUES
  ('a2200009-0000-0000-0000-000000000001', 'a2200008-0000-0000-0000-000000000001', '33333333-3333-3333-3333-333333333333', 'owner'),
  ('a2200009-0000-0000-0000-000000000002', 'a2200008-0000-0000-0000-000000000001', '66666666-6666-6666-6666-666666666666', 'member'),
  ('a2200009-0000-0000-0000-000000000003', 'a2200008-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222', 'owner'),
  ('a2200009-0000-0000-0000-000000000004', 'a2200008-0000-0000-0000-000000000002', '55555555-5555-5555-5555-555555555555', 'member'),
  ('a2200009-0000-0000-0000-000000000005', 'a2200008-0000-0000-0000-000000000002', '66666666-6666-6666-6666-666666666666', 'member')
) AS v(id, thread_id, user_id, role)
JOIN chat_threads t ON t.id = v.thread_id::uuid
ON CONFLICT DO NOTHING;

INSERT INTO chat_messages (id, thread_id, sender_user_id, body, attachment_key, attachment_name, attachment_mime, attachment_size,
                           outside_service_hours, school_id, created_at)
SELECT v.id::uuid, t.id, v.sender::uuid, v.body, NULL, NULL, NULL, NULL, v.outside, t.school_id, t.created_at + v.after
FROM (VALUES
  ('a2200010-0000-0000-0000-000000000001', 'a2200008-0000-0000-0000-000000000001', '33333333-3333-3333-3333-333333333333',
   'Selamat siang Bu, Budi kemarin pulang mengeluh pusing saat pelajaran olahraga. Apakah ada kejadian di sekolah?', false, interval '0'),
  ('a2200010-0000-0000-0000-000000000002', 'a2200008-0000-0000-0000-000000000001', '66666666-6666-6666-6666-666666666666',
   'Selamat siang Pak. Budi sempat istirahat di UKS sekitar 20 menit karena belum sarapan. Kondisinya sudah membaik.', false, interval '1 hour'),
  ('a2200010-0000-0000-0000-000000000003', 'a2200008-0000-0000-0000-000000000001', '33333333-3333-3333-3333-333333333333',
   'Baik Bu, terima kasih infonya. Mulai besok Budi akan sarapan dulu sebelum berangkat.', true, interval '9 hours'),
  ('a2200010-0000-0000-0000-000000000004', 'a2200008-0000-0000-0000-000000000001', '66666666-6666-6666-6666-666666666666',
   'Sama-sama Pak. Mohon juga dibantu mengingatkan tugas proyek IPA yang dikumpulkan Jumat.', false, interval '1 day'),
  ('a2200010-0000-0000-0000-000000000005', 'a2200008-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222',
   'Rapat panitia pentas seni dipindah ke Kamis pukul 13.00 di ruang guru.', false, interval '0'),
  ('a2200010-0000-0000-0000-000000000006', 'a2200008-0000-0000-0000-000000000002', '55555555-5555-5555-5555-555555555555',
   'Siap, Pak. Daftar penampil dari kelas 8 sudah saya kirim lewat email.', false, interval '2 hours'),
  ('a2200010-0000-0000-0000-000000000007', 'a2200008-0000-0000-0000-000000000002', '66666666-6666-6666-6666-666666666666',
   'Mohon jangan kirim promosi jualan pribadi di grup panitia ya.', false, interval '3 days')
) AS v(id, thread_id, sender, body, outside, after)
JOIN chat_threads t ON t.id = v.thread_id::uuid
ON CONFLICT DO NOTHING;

INSERT INTO chat_message_reads (id, message_id, user_id, read_at)
SELECT v.id::uuid, m.id, v.user_id::uuid, m.created_at + interval '30 minutes'
FROM (VALUES
  ('a2200011-0000-0000-0000-000000000001', 'a2200010-0000-0000-0000-000000000001', '66666666-6666-6666-6666-666666666666'),
  ('a2200011-0000-0000-0000-000000000002', 'a2200010-0000-0000-0000-000000000002', '33333333-3333-3333-3333-333333333333'),
  ('a2200011-0000-0000-0000-000000000003', 'a2200010-0000-0000-0000-000000000003', '66666666-6666-6666-6666-666666666666'),
  ('a2200011-0000-0000-0000-000000000004', 'a2200010-0000-0000-0000-000000000005', '55555555-5555-5555-5555-555555555555'),
  ('a2200011-0000-0000-0000-000000000005', 'a2200010-0000-0000-0000-000000000005', '66666666-6666-6666-6666-666666666666')
) AS v(id, message_id, user_id)
JOIN chat_messages m ON m.id = v.message_id::uuid
ON CONFLICT DO NOTHING;

INSERT INTO chat_message_reports (id, school_id, message_id, reporter_user_id, reason, status, reviewed_by, review_note, reviewed_at, created_at)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', m.id, v.reporter::uuid, v.reason, v.status, v.reviewer::uuid, v.note,
       CASE WHEN v.status <> 'open' THEN m.created_at + interval '1 day' END, m.created_at + interval '2 hours'
FROM (VALUES
  ('a2200012-0000-0000-0000-000000000001', 'a2200010-0000-0000-0000-000000000007', '55555555-5555-5555-5555-555555555555',
   'Nada pesan terasa menyindir rekan panitia', 'dismissed', '22222222-2222-2222-2222-222222222222', 'Pesan berupa pengingat tata tertib grup, tidak melanggar.'),
  ('a2200012-0000-0000-0000-000000000002', 'a2200010-0000-0000-0000-000000000006', '66666666-6666-6666-6666-666666666666',
   'Pesan berisi lampiran data pribadi siswa', 'reviewed', '22222222-2222-2222-2222-222222222222', 'Pengirim diingatkan untuk berbagi data siswa lewat sistem, bukan email pribadi.'),
  ('a2200012-0000-0000-0000-000000000003', 'a2200010-0000-0000-0000-000000000003', '66666666-6666-6666-6666-666666666666',
   'Pesan dikirim di luar jam layanan', 'open', NULL, NULL)
) AS v(id, message_id, reporter, reason, status, reviewer, note)
JOIN chat_messages m ON m.id = v.message_id::uuid
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 8. Notifikasi approval & galat + riwayat pengiriman, preferensi kanal
--    (preferensi tidak menyalakan kanal email/WhatsApp untuk user demo)
-- ---------------------------------------------------------------------------
INSERT INTO notifications (id, school_id, user_id, type, title, body, data, idempotency_key, read_at, created_at)
VALUES
  ('a2200013-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777', 'approval',
   'Menunggu persetujuan Anda: Penetapan Hasil Seleksi PPDB Gelombang 2', 'Admin Teratai mengajukan penetapan hasil seleksi jalur reguler.',
   '{"approval_id":"a2100003-0000-0000-0000-000000000003","source_service":"admission","url":"/approvals/a2100003-0000-0000-0000-000000000003"}',
   'seed-a22:approval:003', NULL, now() - interval '2 days'),
  ('a2200013-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777', 'approval',
   'Menunggu persetujuan Anda: Diskon Khusus Uang Pangkal - Anak Guru', 'Tahap 1 sudah disetujui admin, menunggu keputusan kepala sekolah.',
   '{"approval_id":"a2100003-0000-0000-0000-000000000004","source_service":"admission","url":"/approvals/a2100003-0000-0000-0000-000000000004"}',
   'seed-a22:approval:004', now() - interval '1 day', now() - interval '2 days'),
  ('a2200013-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'approval',
   'Pengajuan ditolak: Pengecualian SoD Petugas Payroll', 'Kepala sekolah menolak pengecualian konflik peran.',
   '{"approval_id":"a2100003-0000-0000-0000-000000000002","source_service":"identity","url":"/approvals/a2100003-0000-0000-0000-000000000002"}',
   'seed-a22:approval:002', now() - interval '24 days', now() - interval '25 days'),
  ('a2200013-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'integration_exception',
   'Galat integrasi kritis: webhook payment gateway', 'Signature webhook tidak valid untuk VA 8808-0412.',
   '{"exception_id":"a2100006-0000-0000-0000-000000000001","url":"/integration-exceptions/a2100006-0000-0000-0000-000000000001"}',
   'seed-a22:exception:001', NULL, now() - interval '5 hours'),
  ('a2200013-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 'bill_issued',
   'Tagihan SPP November 2026', 'Tagihan SPP November 2026 sebesar Rp850.000 jatuh tempo 10 November 2026.',
   '{"url":"/tagihan"}', 'seed-a22:bill:nov', NULL, now() - interval '1 day'),
  ('a2200013-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 'receipt',
   'Pembayaran SPP Oktober diterima', 'Pembayaran SPP Oktober 2026 sebesar Rp850.000 sudah kami terima.',
   '{"url":"/tagihan/riwayat"}', 'seed-a22:receipt:oct', now() - interval '14 days', now() - interval '15 days'),
  ('a2200013-0000-0000-0000-000000000007', '11111111-1111-1111-1111-111111111111', '55555555-5555-5555-5555-555555555555', 'approval',
   'Pengajuan disetujui: Cuti Tahunan 3 Hari', 'Cuti tahunan Anda 21-23 September 2026 telah disetujui.',
   '{"approval_id":"a2100003-0000-0000-0000-000000000013","source_service":"hr","url":"/approvals/a2100003-0000-0000-0000-000000000013"}',
   'seed-a22:approval:013', now() - interval '28 days', now() - interval '29 days')
ON CONFLICT DO NOTHING;

INSERT INTO notification_deliveries (id, notification_id, channel, status, attempts, last_error, provider_message_id, created_at, updated_at)
SELECT v.id::uuid, n.id, v.channel, v.status, v.attempts, v.err, v.provider, n.created_at, n.created_at + interval '1 minute'
FROM (VALUES
  ('a2200014-0000-0000-0000-000000000001', 'a2200013-0000-0000-0000-000000000001', 'in_app', 'sent', 1, NULL, NULL),
  ('a2200014-0000-0000-0000-000000000002', 'a2200013-0000-0000-0000-000000000002', 'in_app', 'sent', 1, NULL, NULL),
  ('a2200014-0000-0000-0000-000000000003', 'a2200013-0000-0000-0000-000000000004', 'in_app', 'sent', 1, NULL, NULL),
  ('a2200014-0000-0000-0000-000000000004', 'a2200013-0000-0000-0000-000000000004', 'email', 'sent', 1, NULL, 'seed-a22-email-0004'),
  ('a2200014-0000-0000-0000-000000000005', 'a2200013-0000-0000-0000-000000000005', 'in_app', 'sent', 1, NULL, NULL),
  ('a2200014-0000-0000-0000-000000000006', 'a2200013-0000-0000-0000-000000000005', 'whatsapp', 'failed', 3, 'Nomor tujuan tidak terdaftar di WhatsApp', NULL),
  ('a2200014-0000-0000-0000-000000000007', 'a2200013-0000-0000-0000-000000000006', 'in_app', 'sent', 1, NULL, NULL),
  ('a2200014-0000-0000-0000-000000000008', 'a2200013-0000-0000-0000-000000000006', 'email', 'sent', 1, NULL, 'seed-a22-email-0008')
) AS v(id, notification_id, channel, status, attempts, err, provider)
JOIN notifications n ON n.id = v.notification_id::uuid
ON CONFLICT DO NOTHING;

INSERT INTO notification_preferences (id, school_id, user_id, channel, enabled, created_at, updated_at)
VALUES
  ('a2200015-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777', 'email', false, now() - interval '50 days', now() - interval '50 days'),
  ('a2200015-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '77777777-7777-7777-7777-777777777777', 'whatsapp', false, now() - interval '50 days', now() - interval '50 days'),
  ('a2200015-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '66666666-6666-6666-6666-666666666666', 'in_app', true, now() - interval '40 days', now() - interval '40 days'),
  ('a2200015-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', '55555555-5555-5555-5555-555555555555', 'in_app', true, now() - interval '40 days', now() - interval '40 days')
ON CONFLICT DO NOTHING;

COMMIT;
