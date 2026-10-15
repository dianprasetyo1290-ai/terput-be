DO $$
DECLARE
  v_school_id uuid := '11111111-1111-1111-1111-111111111111';
  v_pw_hash text := '$2a$10$aX2hW3yHCaaQJ1h87c.vMO4P12Y/94ldm6k.txlnG5b5RMJjvXzq2'; -- bcrypt of the demo password (shared out of band, env DEMO_PASSWORD)
  v_user_id uuid;
BEGIN
  -- 1. superadmin
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Superadmin Sekoci', 'superadmin@sekoci.local', v_pw_hash, true, true)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash, is_superadmin = true
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, NULL, 'superadmin', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 2. admin
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Admin Teratai', 'admin@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'admin', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 3. kepala_sekolah
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Kepala Sekolah Teratai', 'kepsek@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'kepala_sekolah', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 4. kepala_yayasan
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Ketua Yayasan Teratai', 'yayasan@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'kepala_yayasan', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 5. guru
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Guru Teratai', 'guru@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'guru', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 6. walikelas
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Wali Kelas Teratai', 'walikelas@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'walikelas', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 7. pengawas
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Pengawas Teratai', 'pengawas@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'pengawas', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 8. siswa
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Budi Santoso (Siswa)', 'siswa@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'siswa', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 9. orang_tua
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Wali Santoso (Orang Tua)', 'orangtua@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'orang_tua', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 10. hr_admin
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('HR Administrator Teratai', 'hradmin@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'hr_admin', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 11. hr_manager
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('HR Manager Teratai', 'hrmanager@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'hr_manager', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 12. payroll_officer
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Petugas Payroll Teratai', 'payroll@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'payroll_officer', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 13. recruiter
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Recruiter Teratai', 'recruiter@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'recruiter', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 14. training_admin
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Admin Pelatihan Teratai', 'training@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'training_admin', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 15. auditor
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Auditor Internal Teratai', 'auditor@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'auditor', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 16. finance_manager
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Finance Manager Teratai', 'financemanager@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'finance_manager', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

  -- 17. pegawai
  INSERT INTO users (name, email, password_hash, password_changed, is_superadmin)
  VALUES ('Staf Pegawai Teratai', 'pegawai@teratai.sch.id', v_pw_hash, true, false)
  ON CONFLICT (email) DO UPDATE SET password_hash = v_pw_hash
  RETURNING id INTO v_user_id;
  INSERT INTO user_school_roles (user_id, school_id, role, status)
  VALUES (v_user_id, v_school_id, 'pegawai', 'active')
  ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

END $$;
