DELETE FROM user_school_roles WHERE role = 'superadmin' AND user_id IN (SELECT id FROM users WHERE email = 'superadmin@sekoci.local');
DELETE FROM users WHERE email = 'superadmin@sekoci.local';
