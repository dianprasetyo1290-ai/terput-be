-- Dev-only seed: a single superadmin account so the service is usable
-- immediately after `migrate up` with no manual bootstrap step.
-- Login: superadmin@sekoci.local / Sekoci@2026  (see README "Assumptions").
INSERT INTO users (name, email, password_hash, default_password, password_changed, is_superadmin)
VALUES (
    'Superadmin Sekoci',
    'superadmin@sekoci.local',
    '$2a$10$v0ZRjh5GZosVoZTYOxIXK.zkOXlvE1z89BJWODs5vMoYEasU5LB1S', -- bcrypt('Sekoci@2026')
    'Sekoci@2026',
    false,
    true
)
ON CONFLICT (email) DO NOTHING;

-- NOTE: not using ON CONFLICT here because the (user_id, school_id, role)
-- unique constraint doesn't dedupe NULL school_id (Postgres treats NULLs as
-- distinct); guard with NOT EXISTS instead so this stays idempotent.
INSERT INTO user_school_roles (user_id, school_id, role)
SELECT id, NULL, 'superadmin' FROM users u
WHERE u.email = 'superadmin@sekoci.local'
  AND NOT EXISTS (
      SELECT 1 FROM user_school_roles r
      WHERE r.user_id = u.id AND r.school_id IS NULL AND r.role = 'superadmin'
  );
