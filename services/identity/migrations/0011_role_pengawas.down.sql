DELETE FROM user_school_roles WHERE role = 'pengawas';
ALTER TABLE user_school_roles DROP CONSTRAINT IF EXISTS user_school_roles_role_check;
ALTER TABLE user_school_roles ADD CONSTRAINT user_school_roles_role_check
    CHECK (role IN ('superadmin','admin','guru','siswa','walikelas','kepala_sekolah','kepala_yayasan'));
