-- 31_attendance_features.sql  (DB: teratai_attendance)
-- Data demo fitur Presensi: agenda/check-in guru, presensi harian Budi dengan
-- semua variasi status, kiriman presensi wali kelas (draft/terkirim), izin
-- siswa (pending/disetujui/ditolak), koreksi presensi, guru pengganti,
-- pengaturan kunci presensi, dan dedupe notifikasi alpa.
--
-- Hanya MENAMBAH: UUID tetap berawalan 5e31 + ON CONFLICT DO NOTHING; baris
-- anak dimasukkan lewat SELECT ... WHERE EXISTS induknya. Satu transaksi.
-- Rujukan lintas layanan: kelas VII A c1000000-...-001, jadwal Senin
-- e2000000-...-001 (MTK) / -002 (B. Indonesia), semester c0000000-...-001.
-- audit_outbox sengaja kosong (antrean teknis).

BEGIN;

-- 1. Pengaturan kunci presensi. Tanpa baris = penguncian nonaktif; baris ini
--    tetap nonaktif (enabled=false) sehingga perilaku tidak berubah, hanya
--    halaman pengaturan menampilkan nilai usulan H+2 pukul 15:00.
INSERT INTO attendance_lock_settings (school_id, enabled, cutoff_days, cutoff_time, updated_by)
VALUES ('11111111-1111-1111-1111-111111111111', false, 2, '15:00', '22222222-2222-2222-2222-222222222222')
ON CONFLICT DO NOTHING;

-- 2. Agenda kelas / check-in guru (Senin, September-Oktober 2026)
INSERT INTO class_sessions (id, school_id, academic_year_id, term_id, schedule_id, classroom_id, subject_id, guru_id,
    session_date, checked_in_at, checked_out_at, slot_start_time, slot_end_time, pertemuan, pembahasan, materi_pertemuan,
    bentuk_kegiatan, agenda_filled_at, catatan, tugas_diberikan, tugas_jenis, source)
SELECT v.id::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
       'c0000000-0000-0000-0000-000000000001', v.sched::uuid, 'c1000000-0000-0000-0000-000000000001', v.subj::uuid,
       '77777777-7777-7777-7777-777777777777', v.d::date, (v.d || ' ' || v.cin || '+07')::timestamptz,
       (v.d || ' ' || v.cout || '+07')::timestamptz, v.s::time, v.e::time, v.pert, v.bahas, v.materi, v.bentuk::jsonb,
       (v.d || ' ' || v.cout || '+07')::timestamptz, v.cat, v.tugas, v.tjenis, 'checkin'
FROM (VALUES
  ('5e310100-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000001', 'c2000000-0000-0000-0000-000000000001', '2026-09-07', '07:26', '08:52', '07:30', '08:50',
   '4', 'Bentuk Aljabar', 'Variabel, koefisien, konstanta, suku sejenis', '["teori"]', 'Kelas kondusif', true, 'individu'),
  ('5e310100-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000002', 'c2000000-0000-0000-0000-000000000002', '2026-09-07', '09:08', '10:31', '09:10', '10:30',
   '4', 'Teks Narasi', 'Membaca cerpen dan menentukan unsur intrinsik', '["teori"]', NULL, false, NULL),
  ('5e310100-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000001', 'c2000000-0000-0000-0000-000000000001', '2026-09-14', '07:29', '08:50', '07:30', '08:50',
   '5', 'Operasi Bentuk Aljabar', 'Penjumlahan dan pengurangan bentuk aljabar', '["teori","praktikum"]', '2 siswa terlambat', false, NULL),
  ('5e310100-0000-0000-0000-000000000004', 'e2000000-0000-0000-0000-000000000001', 'c2000000-0000-0000-0000-000000000001', '2026-09-21', '07:25', '08:50', '07:30', '08:50',
   '6', 'Perkalian Bentuk Aljabar', 'Sifat distributif pada perkalian suku dua', '["teori","praktikum"]', NULL, true, 'kelompok'),
  ('5e310100-0000-0000-0000-000000000005', 'e2000000-0000-0000-0000-000000000001', 'c2000000-0000-0000-0000-000000000001', '2026-09-28', '07:27', '08:51', '07:30', '08:50',
   '7', 'Pemfaktoran Sederhana', 'Faktor persekutuan dan pemfaktoran selisih kuadrat', '["praktikum"]', NULL, false, NULL),
  ('5e310100-0000-0000-0000-000000000006', 'e2000000-0000-0000-0000-000000000001', 'c2000000-0000-0000-0000-000000000001', '2026-10-05', '07:31', '08:50', '07:30', '08:50',
   '8', 'Ulangan Harian Aljabar', 'Penilaian formatif bab aljabar', '["formatif"]', '1 siswa alpa', false, NULL)
) v(id, sched, subj, d, cin, cout, s, e, pert, bahas, materi, bentuk, cat, tugas, tjenis)
ON CONFLICT DO NOTHING;

-- 3. Presensi harian Budi: hadir, terlambat, sakit, izin, tanpa keterangan
INSERT INTO student_attendances (id, class_session_id, school_id, academic_year_id, term_id, classroom_id, student_id,
    session_date, status, catatan, marked_by)
SELECT v.id::uuid, v.sess::uuid, '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
       'c0000000-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444',
       v.d::date, v.status, v.cat, '77777777-7777-7777-7777-777777777777'
FROM (VALUES
  ('5e310200-0000-0000-0000-000000000001', '5e310100-0000-0000-0000-000000000001', '2026-09-07', 'hadir', 'Aktif bertanya'),
  ('5e310200-0000-0000-0000-000000000002', '5e310100-0000-0000-0000-000000000003', '2026-09-14', 'terlambat', 'Datang 07.45, ban sepeda bocor'),
  ('5e310200-0000-0000-0000-000000000003', '5e310100-0000-0000-0000-000000000004', '2026-09-21', 'sakit', 'Surat dokter: demam'),
  ('5e310200-0000-0000-0000-000000000004', '5e310100-0000-0000-0000-000000000005', '2026-09-28', 'izin', 'Acara keluarga (izin orang tua)'),
  ('5e310200-0000-0000-0000-000000000005', '5e310100-0000-0000-0000-000000000006', '2026-10-05', 'tanpa_keterangan', 'Tidak ada kabar dari orang tua')
) v(id, sess, d, status, cat)
WHERE EXISTS (SELECT 1 FROM class_sessions cs WHERE cs.id = v.sess::uuid)
ON CONFLICT DO NOTHING;

-- 4. Kiriman presensi harian wali kelas (terkirim + satu masih draf)
INSERT INTO daily_attendance_submissions (classroom_id, session_date, school_id, status, catatan, submitted_by, submitted_at, updated_by, updated_at)
VALUES
  ('c1000000-0000-0000-0000-000000000001', '2026-09-07', '11111111-1111-1111-1111-111111111111', 'submitted', 'Presensi lengkap', '88888888-8888-8888-8888-888888888888', '2026-09-07 12:00+07', '88888888-8888-8888-8888-888888888888', '2026-09-07 12:00+07'),
  ('c1000000-0000-0000-0000-000000000001', '2026-09-14', '11111111-1111-1111-1111-111111111111', 'submitted', '2 siswa terlambat', '88888888-8888-8888-8888-888888888888', '2026-09-14 12:05+07', '88888888-8888-8888-8888-888888888888', '2026-09-14 12:05+07'),
  ('c1000000-0000-0000-0000-000000000001', '2026-09-21', '11111111-1111-1111-1111-111111111111', 'submitted', '1 siswa sakit', '88888888-8888-8888-8888-888888888888', '2026-09-21 12:10+07', '88888888-8888-8888-8888-888888888888', '2026-09-21 12:10+07'),
  ('c1000000-0000-0000-0000-000000000001', '2026-09-28', '11111111-1111-1111-1111-111111111111', 'submitted', '1 siswa izin', '88888888-8888-8888-8888-888888888888', '2026-09-28 12:00+07', '88888888-8888-8888-8888-888888888888', '2026-09-28 12:00+07'),
  ('c1000000-0000-0000-0000-000000000001', '2026-10-05', '11111111-1111-1111-1111-111111111111', 'draft', 'Menunggu konfirmasi orang tua siswa alpa', NULL, NULL, '88888888-8888-8888-8888-888888888888', '2026-10-05 12:30+07')
ON CONFLICT DO NOTHING;

-- 5. Izin siswa dari orang tua: disetujui, ditolak, menunggu
INSERT INTO leave_requests (id, school_id, academic_year_id, classroom_id, student_id, requested_by, type, start_date, end_date,
    reason, status, reviewed_by, reviewed_at, review_note, source, sick_since, created_at, updated_at)
VALUES
  ('5e310300-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', 'c1000000-0000-0000-0000-000000000001',
   '44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333', 'sakit', '2026-09-21', '2026-09-22',
   'Demam tinggi sejak Minggu malam, sudah periksa ke dokter.', 'approved', '88888888-8888-8888-8888-888888888888', '2026-09-21 08:10+07',
   'Semoga lekas sembuh.', 'parent', '2026-09-20 20:00+07', '2026-09-21 06:30+07', '2026-09-21 08:10+07'),
  ('5e310300-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', 'c1000000-0000-0000-0000-000000000001',
   '44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333', 'izin', '2026-09-28', '2026-09-28',
   'Menghadiri pernikahan kakak di Bandung.', 'approved', '88888888-8888-8888-8888-888888888888', '2026-09-25 14:00+07',
   'Disetujui, mohon kejar materi aljabar.', 'parent', NULL, '2026-09-24 19:00+07', '2026-09-25 14:00+07'),
  ('5e310300-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', 'c1000000-0000-0000-0000-000000000001',
   '44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333', 'izin', '2026-10-09', '2026-10-09',
   'Ingin menonton pertandingan sepak bola di stadion.', 'rejected', '88888888-8888-8888-8888-888888888888', '2026-10-08 09:00+07',
   'Alasan tidak termasuk izin yang dapat disetujui pada hari efektif.', 'parent', NULL, '2026-10-07 20:15+07', '2026-10-08 09:00+07'),
  ('5e310300-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212', 'c1000000-0000-0000-0000-000000000001',
   '44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333', 'izin', '2026-10-22', '2026-10-23',
   'Mengikuti seleksi Olimpiade Sains Nasional tingkat kota.', 'pending', NULL, NULL,
   NULL, 'parent', NULL, '2026-10-14 18:40+07', '2026-10-14 18:40+07')
ON CONFLICT DO NOTHING;

-- 6. Koreksi presensi (Approval Center): disetujui & ditolak
INSERT INTO attendance_corrections (id, school_id, academic_year_id, term_id, classroom_id, student_id, session_date,
    from_status, to_status, catatan, reason, requested_by, requested_role, status, decided_by, decision_comment,
    decided_at, applied_at, created_at, updated_at)
VALUES
  ('5e310400-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'c0000000-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444', '2026-09-14',
   'tanpa_keterangan', 'terlambat', 'Datang 07.45, ban sepeda bocor',
   'Siswa hadir terlambat, tadi tercatat alpa karena guru menginput sebelum siswa tiba.',
   '88888888-8888-8888-8888-888888888888', 'walikelas', 'approved', '55555555-5555-5555-5555-555555555555',
   'Disetujui sesuai keterangan satpam.', '2026-09-17 10:00+07', '2026-09-17 10:00+07', '2026-09-16 13:00+07', '2026-09-17 10:00+07'),
  ('5e310400-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'c0000000-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000001', '44444444-4444-4444-4444-444444444444', '2026-10-05',
   'tanpa_keterangan', 'izin', NULL,
   'Orang tua menyampaikan izin lisan setelah presensi dikunci.',
   '88888888-8888-8888-8888-888888888888', 'walikelas', 'rejected', '55555555-5555-5555-5555-555555555555',
   'Ditolak: tidak ada surat izin tertulis.', '2026-10-08 11:00+07', NULL, '2026-10-07 09:30+07', '2026-10-08 11:00+07')
ON CONFLICT DO NOTHING;

-- 7. Permintaan guru pengganti: menunggu, diterima, ditolak
INSERT INTO substitute_requests (id, school_id, academic_year_id, term_id, schedule_id, classroom_id, subject_id, session_date,
    slot_start_time, slot_end_time, requested_by, requested_to, reason, status, created_at, responded_at)
VALUES
  ('5e310500-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'c0000000-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000001',
   'c2000000-0000-0000-0000-000000000001', '2026-10-19', '07:30', '08:50', '77777777-7777-7777-7777-777777777777',
   '88888888-8888-8888-8888-888888888888', 'cuti', 'pending', '2026-10-14 15:00+07', NULL),
  ('5e310500-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'c0000000-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000002', 'c1000000-0000-0000-0000-000000000001',
   'c2000000-0000-0000-0000-000000000002', '2026-10-12', '09:10', '10:30', '77777777-7777-7777-7777-777777777777',
   '88888888-8888-8888-8888-888888888888', 'sakit', 'accepted', '2026-10-12 06:15+07', '2026-10-12 06:40+07'),
  ('5e310500-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '12121212-1212-1212-1212-121212121212',
   'c0000000-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000002', 'c1000000-0000-0000-0000-000000000001',
   'c2000000-0000-0000-0000-000000000002', '2026-09-21', '09:10', '10:30', '77777777-7777-7777-7777-777777777777',
   '55555555-5555-5555-5555-555555555555', 'izin', 'rejected', '2026-09-18 16:00+07', '2026-09-18 17:30+07')
ON CONFLICT DO NOTHING;

-- 8. Dedupe notifikasi alpa ke orang tua (Budi alpa 5 Oktober)
INSERT INTO alpa_notifications (student_id, session_date, school_id, created_at)
VALUES ('44444444-4444-4444-4444-444444444444', '2026-10-05', '11111111-1111-1111-1111-111111111111', '2026-10-05 09:00+07')
ON CONFLICT DO NOTHING;

COMMIT;
