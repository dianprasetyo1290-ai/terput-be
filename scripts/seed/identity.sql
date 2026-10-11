-- Local seed for the identity database (Teratai fork).
-- Deterministic ids so the admission seed and the E2E specs can reference them.
-- Passwords reuse the same bcrypt hash as the built-in superadmin seed, i.e.
-- the password is "Sekoci@2026" (bcrypt('Sekoci@2026')). Local only.

BEGIN;

INSERT INTO schools (id, name, code, level, city, is_active)
VALUES ('11111111-1111-1111-1111-111111111111', 'Sekolah Teratai', 'TRT', 'SMP', 'Jakarta', true)
ON CONFLICT DO NOTHING;

INSERT INTO academic_years (id, school_id, label, is_current)
VALUES ('12121212-1212-1212-1212-121212121212', '11111111-1111-1111-1111-111111111111', '2026/2027', true)
ON CONFLICT DO NOTHING;

UPDATE schools SET current_academic_year_id = '12121212-1212-1212-1212-121212121212'
WHERE id = '11111111-1111-1111-1111-111111111111';

-- Users: admin, wali (orang_tua), siswa + one account per remaining role so the
-- role-landing behaviour can be verified locally (kepsek/walikelas also hold
-- guru, exactly like the demo server, which is what used to make them land on
-- the guru UI). Shared bcrypt hash of "Sekoci@2026".
INSERT INTO users (id, name, email, password_hash, password_changed, nisn, is_superadmin)
VALUES
  ('22222222-2222-2222-2222-222222222222', 'Admin Teratai', 'admin@teratai.local',
   '$2a$10$v0ZRjh5GZosVoZTYOxIXK.zkOXlvE1z89BJWODs5vMoYEasU5LB1S', false, NULL, false),
  ('33333333-3333-3333-3333-333333333333', 'Wali Santoso', 'wali@teratai.local',
   '$2a$10$v0ZRjh5GZosVoZTYOxIXK.zkOXlvE1z89BJWODs5vMoYEasU5LB1S', false, NULL, false),
  ('44444444-4444-4444-4444-444444444444', 'Budi Santoso', 'siswa@teratai.local',
   '$2a$10$v0ZRjh5GZosVoZTYOxIXK.zkOXlvE1z89BJWODs5vMoYEasU5LB1S', false, '1234567890', false),
  ('55555555-5555-5555-5555-555555555555', 'Guru Matematika', 'guru@teratai.local',
   '$2a$10$v0ZRjh5GZosVoZTYOxIXK.zkOXlvE1z89BJWODs5vMoYEasU5LB1S', false, NULL, false),
  ('66666666-6666-6666-6666-666666666666', 'Wali Kelas VII A', 'walikelas@teratai.local',
   '$2a$10$v0ZRjh5GZosVoZTYOxIXK.zkOXlvE1z89BJWODs5vMoYEasU5LB1S', false, NULL, false),
  ('77777777-7777-7777-7777-777777777777', 'Kepala Sekolah', 'kepsek@teratai.local',
   '$2a$10$v0ZRjh5GZosVoZTYOxIXK.zkOXlvE1z89BJWODs5vMoYEasU5LB1S', false, NULL, false),
  ('88888888-8888-8888-8888-888888888888', 'Kepala Yayasan', 'yayasan@teratai.local',
   '$2a$10$v0ZRjh5GZosVoZTYOxIXK.zkOXlvE1z89BJWODs5vMoYEasU5LB1S', false, NULL, false),
  ('99999999-9999-9999-9999-999999999999', 'Pengawas Ujian', 'pengawas@teratai.local',
   '$2a$10$v0ZRjh5GZosVoZTYOxIXK.zkOXlvE1z89BJWODs5vMoYEasU5LB1S', false, NULL, false)
ON CONFLICT DO NOTHING;

-- guru is inserted BEFORE the jabatan row on purpose: the account mirrors the
-- demo server (guru@ holds guru + walikelas/kepala_sekolah), which is exactly
-- the shape that used to make Kepala Sekolah/Wali Kelas land on the guru UI.
INSERT INTO user_school_roles (user_id, school_id, role, status)
VALUES
  ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'admin', 'active'),
  ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', 'orang_tua', 'active'),
  ('44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', 'siswa', 'active'),
  ('55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', 'guru', 'active'),
  ('66666666-6666-6666-6666-666666666666', '11111111-1111-1111-1111-111111111111', 'guru', 'active'),
  ('66666666-6666-6666-6666-666666666666', '11111111-1111-1111-1111-111111111111', 'walikelas', 'active'),
  ('77777777-7777-7777-7777-777777777777', '11111111-1111-1111-1111-111111111111', 'guru', 'active'),
  ('77777777-7777-7777-7777-777777777777', '11111111-1111-1111-1111-111111111111', 'kepala_sekolah', 'active'),
  ('88888888-8888-8888-8888-888888888888', '11111111-1111-1111-1111-111111111111', 'kepala_yayasan', 'active'),
  ('99999999-9999-9999-9999-999999999999', '11111111-1111-1111-1111-111111111111', 'pengawas', 'active')
ON CONFLICT DO NOTHING;

-- Wali <-> siswa link.
INSERT INTO guardian_links (school_id, guardian_user_id, student_user_id, relationship, is_primary, status, verified_at)
VALUES ('11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333',
        '44444444-4444-4444-4444-444444444444', 'ayah', true, 'active', now())
ON CONFLICT DO NOTHING;

COMMIT;
