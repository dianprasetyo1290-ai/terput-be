-- Restore every role row the SoD clean-up removed (those not yet restored),
-- then drop the 0022 objects. Fails clearly while `akuntan` rows exist so no
-- role data is lost silently.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM user_school_roles WHERE role = 'akuntan') THEN
        RAISE EXCEPTION 'Cannot rollback migration 0022: rows with role akuntan still exist in user_school_roles';
    END IF;
END $$;

INSERT INTO user_school_roles (id, user_id, school_id, role, status, student_status, academic_year_id, deactivated_at, created_at)
SELECT l.user_school_role_id, l.user_id, l.school_id, l.role, l.status, l.student_status, l.academic_year_id, l.deactivated_at, l.original_created_at
FROM role_sod_cleanup_log l
JOIN users u ON u.id = l.user_id
WHERE l.restored_at IS NULL
  AND (l.school_id IS NULL OR EXISTS (SELECT 1 FROM schools s WHERE s.id = l.school_id))
ON CONFLICT DO NOTHING;

-- Break-glass rows still active are removed together with their grant.
DELETE FROM user_school_roles usr
USING break_glass_grants g
WHERE g.ended_at IS NULL AND g.role_row_created
  AND usr.user_id = g.user_id AND usr.school_id = g.school_id AND usr.role = g.role;

ALTER TABLE users DROP COLUMN IF EXISTS anonymized_at;

DROP TABLE IF EXISTS reference_values;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS data_subject_requests;
DROP TABLE IF EXISTS break_glass_grants;
DROP FUNCTION IF EXISTS sod_resolve_violations(TEXT);
DROP FUNCTION IF EXISTS sod_keep_rank(TEXT);
DROP TABLE IF EXISTS role_sod_cleanup_log;
DROP TABLE IF EXISTS role_conflict_exceptions;

DELETE FROM role_conflicts WHERE role_a = 'akuntan' OR role_b = 'akuntan';

ALTER TABLE user_school_roles DROP CONSTRAINT IF EXISTS user_school_roles_role_check;
ALTER TABLE user_school_roles ADD CONSTRAINT user_school_roles_role_check
    CHECK (role IN (
        'superadmin', 'admin', 'guru', 'siswa', 'orang_tua', 'walikelas', 'kepala_sekolah', 'kepala_yayasan', 'pengawas',
        'hr_admin', 'hr_manager', 'payroll_officer', 'recruiter', 'training_admin', 'auditor', 'finance_manager', 'pegawai',
        'inventory_officer', 'procurement_officer', 'asset_officer', 'pustakawan', 'facility_officer', 'teknisi',
        'transport_officer', 'pendamping_transport', 'pengemudi', 'canteen_admin', 'canteen_operator', 'petugas_uks',
        'konselor', 'kesiswaan', 'event_coordinator', 'pembina_ekskul', 'servicedesk_agent', 'servicedesk_manager',
        'koordinator_pkl', 'pembimbing_pkl', 'pembimbing_industri', 'qa_coordinator', 'operator_dapodik', 'humas', 'bi_analyst'
    ));
