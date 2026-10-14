-- QA-IDN (UAT 2026-10-14): session & account security.
-- BUG-022/024 token revocation, BUG-025 lockout + idle timeout, BUG-083 login
-- history & active sessions, BUG-047 MFA TOTP, BUG-085 self-service reset
-- link, BUG-023 forbidden role combinations (SoD).

-- Platform security policy (NFR-SEC-01/04). Accounts are global (one user can
-- hold roles at several units), so the policy is one row for the platform,
-- editable by superadmin. Seeded with the PRD numbers: lock after [5] failed
-- logins, session ends after [30] idle minutes.
CREATE TABLE security_policy (
    id                        SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    max_failed_logins         INT NOT NULL DEFAULT 5 CHECK (max_failed_logins BETWEEN 1 AND 50),
    lockout_minutes           INT NOT NULL DEFAULT 15 CHECK (lockout_minutes BETWEEN 1 AND 1440),
    idle_timeout_minutes      INT NOT NULL DEFAULT 30 CHECK (idle_timeout_minutes BETWEEN 5 AND 1440),
    session_max_hours         INT NOT NULL DEFAULT 24 CHECK (session_max_hours BETWEEN 1 AND 720),
    reset_token_ttl_minutes   INT NOT NULL DEFAULT 30 CHECK (reset_token_ttl_minutes BETWEEN 5 AND 1440),
    mfa_roles                 TEXT[] NOT NULL DEFAULT '{}',
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by                UUID REFERENCES users(id) ON DELETE SET NULL
);
INSERT INTO security_policy (id, mfa_roles)
VALUES (1, ARRAY['superadmin','finance_manager','payroll_officer','hr_manager']);

ALTER TABLE users
    ADD COLUMN failed_login_count  INT NOT NULL DEFAULT 0,
    ADD COLUMN locked_until        TIMESTAMPTZ,
    -- Every token issued before this instant is rejected (deactivate, delete,
    -- password change, "keluar dari semua sesi"). Covers tokens minted before
    -- sessions existed, which carry no sid claim.
    ADD COLUMN tokens_valid_after  TIMESTAMPTZ,
    -- TOTP secret, AES-GCM encrypted (never stored or logged in clear).
    ADD COLUMN mfa_secret_enc      TEXT,
    ADD COLUMN mfa_enabled_at      TIMESTAMPTZ;

-- One row per issued access token (JWT claim sid).
CREATE TABLE user_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    revoked_reason  TEXT,
    ip              TEXT,
    user_agent      TEXT,
    mfa_verified    BOOLEAN NOT NULL DEFAULT false,
    CHECK (expires_at > created_at)
);
CREATE INDEX idx_user_sessions_user_active ON user_sessions(user_id) WHERE revoked_at IS NULL;

-- Login history (success and failure). user_id is NULL when the email is
-- unknown; the typed email is kept so brute force on unknown accounts shows up.
CREATE TABLE login_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    success     BOOLEAN NOT NULL,
    reason      TEXT NOT NULL,
    ip          TEXT,
    user_agent  TEXT,
    session_id  UUID REFERENCES user_sessions(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_login_events_user ON login_events(user_id, created_at DESC);

-- Short-lived second step of a login that needs MFA. Only the SHA-256 of the
-- opaque challenge token is stored.
CREATE TABLE mfa_challenges (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    purpose      TEXT NOT NULL CHECK (purpose IN ('verify','enroll')),
    pending_secret_enc TEXT,
    attempts     INT NOT NULL DEFAULT 0,
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_mfa_challenges_user ON mfa_challenges(user_id);

-- Self-service password reset (single use, expiring). Only the SHA-256 of the
-- emailed token is stored.
CREATE TABLE password_reset_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    requested_ip TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);
CREATE INDEX idx_password_reset_tokens_user ON password_reset_tokens(user_id);

-- Forbidden role combinations on one account (AC-RBAC-08). Stored with
-- role_a < role_b so a pair has exactly one row; superadmin maintains it.
CREATE TABLE role_conflicts (
    role_a      TEXT NOT NULL,
    role_b      TEXT NOT NULL,
    reason      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (role_a, role_b),
    CHECK (role_a < role_b)
);
INSERT INTO role_conflicts (role_a, role_b, reason)
SELECT LEAST(a, b), GREATEST(a, b), reason FROM (VALUES
    ('auditor', 'admin', 'Auditor tidak boleh mengelola data yang diauditnya'),
    ('auditor', 'superadmin', 'Auditor tidak boleh mengelola data yang diauditnya'),
    ('auditor', 'finance_manager', 'Auditor tidak boleh menyetujui transaksi keuangan'),
    ('auditor', 'payroll_officer', 'Auditor tidak boleh memproses penggajian'),
    ('auditor', 'hr_admin', 'Auditor tidak boleh mengelola data kepegawaian'),
    ('auditor', 'hr_manager', 'Auditor tidak boleh menyetujui data kepegawaian'),
    ('payroll_officer', 'finance_manager', 'Pembuat penggajian dan penyetuju pembayaran harus terpisah'),
    ('payroll_officer', 'hr_manager', 'Pembuat penggajian dan penyetuju penggajian harus terpisah'),
    ('procurement_officer', 'inventory_officer', 'Pemesan barang dan penerima barang harus terpisah'),
    ('siswa', 'admin', 'Akun siswa tidak boleh memegang peran pengelola'),
    ('siswa', 'superadmin', 'Akun siswa tidak boleh memegang peran pengelola'),
    ('siswa', 'guru', 'Akun siswa tidak boleh memegang peran guru'),
    ('siswa', 'orang_tua', 'Akun siswa tidak boleh sekaligus akun orang tua'),
    ('siswa', 'finance_manager', 'Akun siswa tidak boleh memegang peran keuangan'),
    ('siswa', 'payroll_officer', 'Akun siswa tidak boleh memegang peran keuangan')
) AS t(a, b, reason);
