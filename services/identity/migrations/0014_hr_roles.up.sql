-- Broaden allowed roles to include HR, Payroll, ATS, Training, Finance, and Auditor roles (T1 / PRD Bagian 2)
ALTER TABLE user_school_roles DROP CONSTRAINT IF EXISTS user_school_roles_role_check;
ALTER TABLE user_school_roles ADD CONSTRAINT user_school_roles_role_check
    CHECK (role IN (
        'superadmin', 'admin', 'guru', 'siswa', 'orang_tua', 'walikelas', 'kepala_sekolah', 'kepala_yayasan', 'pengawas',
        'hr_admin', 'hr_manager', 'payroll_officer', 'recruiter', 'training_admin', 'auditor', 'finance_manager', 'pegawai'
    ));
