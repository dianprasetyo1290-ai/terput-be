-- 05_studentlife.sql
-- Seed UKS medicines, counseling categories, discipline types, extracurriculars, PKL partners

DO $$
DECLARE
  v_school_id UUID := '11111111-1111-1111-1111-111111111111';
  v_siswa_id  UUID := '44444444-4444-4444-4444-444444444444';
  v_guru_id   UUID := '77777777-7777-7777-7777-777777777777';
  v_ecc_robot UUID;
  v_ecc_futsal UUID;
  v_partner_id UUID;
  v_major_id  UUID;
BEGIN
  -- 1. UKS Medicines
  INSERT INTO uks_medicines (school_id, name, unit, active)
  VALUES (v_school_id, 'Paracetamol 500mg', 'tablet', true)
  ON CONFLICT DO NOTHING;

  INSERT INTO uks_medicines (school_id, name, unit, active)
  VALUES (v_school_id, 'Minyak Kayu Putih 60ml', 'botol', true)
  ON CONFLICT DO NOTHING;

  INSERT INTO uks_medicines (school_id, name, unit, active)
  VALUES (v_school_id, 'Betadine Antiseptik 30ml', 'botol', true)
  ON CONFLICT DO NOTHING;

  INSERT INTO uks_medicines (school_id, name, unit, active)
  VALUES (v_school_id, 'Kasa Steril & Plester Cepat', 'kotak', true)
  ON CONFLICT DO NOTHING;

  -- 2. Counseling Categories
  INSERT INTO counseling_categories (school_id, name, active)
  VALUES (v_school_id, 'Akademik & Motivasi Belajar', true)
  ON CONFLICT DO NOTHING;

  INSERT INTO counseling_categories (school_id, name, active)
  VALUES (v_school_id, 'Pribadi & Emosional', true)
  ON CONFLICT DO NOTHING;

  INSERT INTO counseling_categories (school_id, name, active)
  VALUES (v_school_id, 'Karier & Perencanaan Studi', true)
  ON CONFLICT DO NOTHING;

  -- 3. Discipline Types (Violations & Merits)
  INSERT INTO discipline_types (school_id, code, name, kind, points, active)
  VALUES (v_school_id, 'DISC-LATE', 'Terlambat Masuk Sekolah (>15 menit)', 'violation', 5, true)
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;

  INSERT INTO discipline_types (school_id, code, name, kind, points, active)
  VALUES (v_school_id, 'DISC-UNIFORM', 'Seragam / Atribut Tidak Lengkap', 'violation', 3, true)
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;

  INSERT INTO discipline_types (school_id, code, name, kind, points, active)
  VALUES (v_school_id, 'DISC-MERIT-ROBOT', 'Juara Lomba Akademik / Sains', 'merit', 20, true)
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;

  -- 4. Extracurriculars
  INSERT INTO act_extracurriculars (school_id, name, description, quota, fee_idr, full_policy, active)
  VALUES (
    v_school_id, 'Klub Robotik & Coding', 'Pelatihan pemrograman Arduino, Python & Robotik',
    30, 50000, 'waitlist', true
  )
  ON CONFLICT (school_id, name) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_ecc_robot FROM act_extracurriculars WHERE school_id = v_school_id AND name = 'Klub Robotik & Coding' LIMIT 1;

  INSERT INTO act_extracurriculars (school_id, name, description, quota, fee_idr, full_policy, active)
  VALUES (
    v_school_id, 'Futsal Teratai Putih', 'Latihan fisik, taktik, dan kompetisi futsal',
    40, 25000, 'reject', true
  )
  ON CONFLICT (school_id, name) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_ecc_futsal FROM act_extracurriculars WHERE school_id = v_school_id AND name = 'Futsal Teratai Putih' LIMIT 1;

  INSERT INTO act_extracurriculars (school_id, name, description, quota, fee_idr, full_policy, active)
  VALUES (
    v_school_id, 'Pramuka Inti', 'Kepanduan, kemandirian, dan kepemimpinan',
    100, 0, 'waitlist', true
  )
  ON CONFLICT (school_id, name) WHERE deleted_at IS NULL DO NOTHING;

  -- 5. Achievements
  IF v_ecc_robot IS NOT NULL THEN
    INSERT INTO act_achievements (
      school_id, student_user_id, ecc_id, activity_name, organizer, level, rank_label,
      achieved_on, submitted_by, submitted_via, status
    ) VALUES (
      v_school_id, v_siswa_id, v_ecc_robot, 'Lomba Robotik Pelajar Kota Bekasi',
      'Dinas Pendidikan Kota Bekasi', 'kabupaten_kota', 'Juara 1',
      '2026-08-17', v_guru_id, 'pembina_ekskul', 'verified'
    )
    ON CONFLICT DO NOTHING;
  END IF;

  -- 6. PKL Majors & Partners
  INSERT INTO majors (school_id, code, name, active)
  VALUES (v_school_id, 'RPL', 'Rekayasa Perangkat Lunak', true)
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_major_id FROM majors WHERE school_id = v_school_id AND code = 'RPL' LIMIT 1;

  INSERT INTO majors (school_id, code, name, active)
  VALUES (v_school_id, 'TKJ', 'Teknik Komputer dan Jaringan', true)
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;

  INSERT INTO pkl_partners (school_id, name, field, address, contact_name, contact_phone, contact_email, active)
  VALUES (
    v_school_id, 'PT Telkom Indonesia (Witel Bekasi)', 'Telekomunikasi & Jaringan',
    'Jl. Hasibuan No. 1, Bekasi', 'Bpk. Hendra Kurniawan', '081298765432', 'pkl@telkom-bekasi.co.id', true
  )
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_partner_id FROM pkl_partners WHERE school_id = v_school_id AND name = 'PT Telkom Indonesia (Witel Bekasi)' LIMIT 1;

  INSERT INTO pkl_partners (school_id, name, field, address, contact_name, contact_phone, contact_email, active)
  VALUES (
    v_school_id, 'PT Astra Graphia Information Technology', 'Software & IT Infrastructure',
    'Jl. Kramat Raya No. 43, Jakarta', 'Ibu Maya Lestari', '081345678901', 'hrd@agit.astra.co.id', true
  )
  ON CONFLICT DO NOTHING;

END $$;
