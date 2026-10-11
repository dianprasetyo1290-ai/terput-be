-- Revert 0016: drop deactivation timestamps

ALTER TABLE guardian_links DROP COLUMN IF EXISTS deactivated_at;
ALTER TABLE user_school_roles DROP COLUMN IF EXISTS deactivated_at;
