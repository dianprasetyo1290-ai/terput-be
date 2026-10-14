DROP TABLE IF EXISTS student_status_history;
ALTER TABLE user_school_roles DROP COLUMN IF EXISTS student_status;

ALTER TABLE academic_years DROP CONSTRAINT IF EXISTS academic_years_dates_check;
ALTER TABLE academic_years
    DROP COLUMN IF EXISTS lock_reason,
    DROP COLUMN IF EXISTS locked_by,
    DROP COLUMN IF EXISTS locked_at,
    DROP COLUMN IF EXISTS end_date,
    DROP COLUMN IF EXISTS start_date;

DROP INDEX IF EXISTS idx_users_phone;
DROP INDEX IF EXISTS idx_users_nik;
DROP INDEX IF EXISTS idx_users_nisn;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_nik_format_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_nisn_format_check;
ALTER TABLE users DROP COLUMN IF EXISTS nik;

ALTER TABLE schools DROP CONSTRAINT IF EXISTS schools_settlement_account_number_check;
ALTER TABLE schools
    DROP COLUMN IF EXISTS settlement_account_name,
    DROP COLUMN IF EXISTS settlement_account_number,
    DROP COLUMN IF EXISTS settlement_bank_name,
    DROP COLUMN IF EXISTS accounting_entity_code,
    DROP COLUMN IF EXISTS principal_user_id,
    DROP COLUMN IF EXISTS principal_name;
