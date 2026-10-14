-- QA-IDN: master-data rules from the UAT of 2026-10-14.
--   BUG-068/012  schools: head of unit, accounting entity, settlement account
--   BUG-074/016  users: NIK column, NISN/NIK format (NOT VALID: existing bad
--                rows stay readable; new writes are checked)
--   BUG-070/015  academic_years: start/end dates + period lock
--   BUG-017/075  student status calon/aktif/lulus/keluar + history

-- ---------- schools ----------
ALTER TABLE schools
    ADD COLUMN principal_name            TEXT,
    ADD COLUMN principal_user_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN accounting_entity_code    TEXT,
    ADD COLUMN settlement_bank_name      TEXT,
    ADD COLUMN settlement_account_number TEXT,
    ADD COLUMN settlement_account_name   TEXT;
ALTER TABLE schools ADD CONSTRAINT schools_settlement_account_number_check
    CHECK (settlement_account_number IS NULL OR settlement_account_number ~ '^[0-9]{5,20}$');

-- ---------- users ----------
ALTER TABLE users ADD COLUMN nik TEXT;
ALTER TABLE users ADD CONSTRAINT users_nisn_format_check CHECK (nisn IS NULL OR nisn ~ '^[0-9]{10}$') NOT VALID;
ALTER TABLE users ADD CONSTRAINT users_nik_format_check  CHECK (nik  IS NULL OR nik  ~ '^[0-9]{16}$') NOT VALID;
-- Lookup indexes for duplicate detection (not unique: duplicates are a
-- warning the admin may override, and existing data already has some).
CREATE INDEX idx_users_nisn ON users(nisn) WHERE nisn IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_users_nik  ON users(nik)  WHERE nik  IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_users_phone ON users(phone) WHERE phone IS NOT NULL AND deleted_at IS NULL;

-- ---------- academic_years ----------
ALTER TABLE academic_years
    ADD COLUMN start_date  DATE,
    ADD COLUMN end_date    DATE,
    ADD COLUMN locked_at   TIMESTAMPTZ,
    ADD COLUMN locked_by   UUID,
    ADD COLUMN lock_reason TEXT;
-- Backfill from "YYYY/YYYY+1" labels: Indonesian school year 1 July - 30 June.
UPDATE academic_years
   SET start_date = make_date(substring(label from '^(\d{4})')::int, 7, 1),
       end_date   = make_date(substring(label from '^\d{4}\s*/\s*(\d{4})')::int, 6, 30)
 WHERE label ~ '^\d{4}\s*/\s*\d{4}'
   AND substring(label from '^\d{4}\s*/\s*(\d{4})')::int = substring(label from '^(\d{4})')::int + 1;
ALTER TABLE academic_years ADD CONSTRAINT academic_years_dates_check
    CHECK (start_date IS NULL OR end_date IS NULL OR end_date > start_date);

-- ---------- student status ----------
ALTER TABLE user_school_roles ADD COLUMN student_status TEXT
    CHECK (student_status IN ('calon','aktif','lulus','keluar'));
UPDATE user_school_roles SET student_status = CASE WHEN status = 'active' THEN 'aktif' ELSE 'keluar' END
 WHERE role = 'siswa';

CREATE TABLE student_status_history (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    school_id   UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    from_status TEXT,
    to_status   TEXT NOT NULL,
    reason      TEXT,
    correction  BOOLEAN NOT NULL DEFAULT false,
    changed_by  UUID,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_student_status_history_user ON student_status_history(user_id, changed_at DESC);
