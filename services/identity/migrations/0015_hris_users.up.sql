-- HRIS (T2): identity keeps two facts about HR-managed accounts.
--   managed_by_hr  set by POST /internal/employees/create-or-link; Data Guru /
--                  Data Pengguna then refuse name edits while the school's
--                  'hris' feature is on (AC-83-02).
--   deactivate_at  set by POST /internal/users/{id}/deactivate; Login rejects
--                  once now() >= deactivate_at (AC-83-03).
-- The role CHECK (hr_admin, hr_manager, pegawai, ...) is widened by 0014 (T1).
ALTER TABLE users ADD COLUMN managed_by_hr BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN deactivate_at TIMESTAMPTZ NULL;
