-- Rollback allowed roles to pre-T1 list.
-- Fail if any rows still use the new roles to avoid silent data corruption.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM user_school_roles
        WHERE role IN ('hr_admin', 'hr_manager', 'payroll_officer', 'recruiter', 'training_admin', 'auditor', 'finance_manager', 'pegawai')
    ) THEN
        RAISE EXCEPTION 'Cannot rollback migration 0014: rows with new roles still exist in user_school_roles';
    END IF;
END $$;

ALTER TABLE user_school_roles DROP CONSTRAINT IF EXISTS user_school_roles_role_check;
ALTER TABLE user_school_roles ADD CONSTRAINT user_school_roles_role_check
    CHECK (role IN ('superadmin','admin','guru','siswa','orang_tua','walikelas','kepala_sekolah','kepala_yayasan','pengawas'));
