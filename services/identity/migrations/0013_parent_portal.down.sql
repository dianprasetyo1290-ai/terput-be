DROP TABLE IF EXISTS admission_conversion_mappings;
DROP TABLE IF EXISTS identity_external_refs;
DROP TABLE IF EXISTS guardian_claim_rate_limits;
DROP TABLE IF EXISTS guardian_claim_codes;
DROP TABLE IF EXISTS guardian_links;

DELETE FROM user_school_roles WHERE role = 'orang_tua';
ALTER TABLE user_school_roles DROP CONSTRAINT IF EXISTS user_school_roles_role_check;
ALTER TABLE user_school_roles ADD CONSTRAINT user_school_roles_role_check
    CHECK (role IN ('superadmin','admin','guru','siswa','walikelas','kepala_sekolah','kepala_yayasan','pengawas'));
