DROP TABLE IF EXISTS role_conflicts;
DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS mfa_challenges;
DROP TABLE IF EXISTS login_events;
DROP TABLE IF EXISTS user_sessions;
ALTER TABLE users
    DROP COLUMN IF EXISTS mfa_enabled_at,
    DROP COLUMN IF EXISTS mfa_secret_enc,
    DROP COLUMN IF EXISTS tokens_valid_after,
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS failed_login_count;
DROP TABLE IF EXISTS security_policy;
