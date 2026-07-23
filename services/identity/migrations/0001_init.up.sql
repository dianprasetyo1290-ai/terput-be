-- identity-service schema
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ==========================================================================
-- Global tables (define school_id; per CONVENTIONS.md these do NOT scope to
-- themselves).
-- ==========================================================================

CREATE TABLE foundations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    address       TEXT,
    year_founded  INT,
    year_joined   INT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);

CREATE TABLE schools (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    foundation_id             UUID REFERENCES foundations(id) ON DELETE SET NULL,
    name                      TEXT NOT NULL,
    code                      TEXT UNIQUE,
    level                     TEXT, -- e.g. SD/SMP/SMA (kept as free text; school_levels not modeled as a separate table, see README Assumptions)
    address                   TEXT,
    subdistrict               TEXT, -- kelurahan/desa
    district                  TEXT, -- kecamatan
    city                      TEXT,
    province                  TEXT,
    registration_number       TEXT,
    year_founded              TEXT,
    phone_number              TEXT,
    email                     TEXT,
    website                   TEXT,
    logo                      TEXT,
    is_active                 BOOLEAN NOT NULL DEFAULT true,
    current_academic_year_id  UUID, -- FK added below, after academic_years exists
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at                TIMESTAMPTZ
);

-- identity owns academic_years (id/label/current-flag only); semester/term
-- and all scheduling data live in academic-service, which resolves IDs via
-- GET/POST /academic-years over HTTP.
CREATE TABLE academic_years (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    label       TEXT NOT NULL, -- e.g. "2026/2027"
    is_current  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, label)
);

ALTER TABLE schools
    ADD CONSTRAINT fk_schools_current_academic_year
    FOREIGN KEY (current_academic_year_id) REFERENCES academic_years(id) ON DELETE SET NULL;

-- users are identity-global (an account is one row regardless of how many
-- schools/roles it holds); per-school role/tenancy scoping happens via
-- user_school_roles below, which does carry school_id.
CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL,
    email             TEXT NOT NULL UNIQUE,
    phone             TEXT,
    password_hash     TEXT NOT NULL,
    default_password  TEXT,    -- plaintext of the currently-active default password; cleared once the user changes it
    password_changed  BOOLEAN NOT NULL DEFAULT false,
    photo             TEXT,
    birth_date        DATE,
    birth_place       TEXT,
    address           TEXT,
    is_superadmin     BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ
);

CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;

-- ==========================================================================
-- School-scoped tables
-- ==========================================================================

-- The 7 system roles (4 dashboard + 3 jabatan-only) that drive JWT
-- active_role/roles claims. A user can hold multiple rows (multi-role /
-- switch-role support). school_id is NULL only for role='superadmin'.
CREATE TABLE user_school_roles (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    school_id         UUID REFERENCES schools(id) ON DELETE CASCADE,
    role              TEXT NOT NULL CHECK (role IN ('superadmin','admin','guru','siswa','walikelas','kepala_sekolah','kepala_yayasan')),
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')), -- guru aktif/nonaktif lives here
    academic_year_id  UUID REFERENCES academic_years(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, school_id, role)
);

CREATE INDEX idx_usr_school ON user_school_roles(school_id);
CREATE INDEX idx_usr_user ON user_school_roles(user_id);
CREATE INDEX idx_usr_role ON user_school_roles(role);

-- Data Jabatan / Peran (same concept, one implementation): admin-defined
-- position/role labels per school (e.g. "Kepala Yayasan", "Walikelas 5A").
CREATE TABLE positions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (school_id, name)
);

CREATE INDEX idx_positions_school ON positions(school_id);

-- Assigns a guru to hold a position; feeds "Organisasi Sekolah" (Jabatan | Nama).
CREATE TABLE position_assignments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    position_id  UUID NOT NULL REFERENCES positions(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    school_id    UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (position_id, user_id)
);

CREATE INDEX idx_position_assignments_school ON position_assignments(school_id);

-- Informasi (announcements). school_id NULL = broadcast across all schools
-- (superadmin); otherwise scoped to the publishing admin's school.
CREATE TABLE announcements (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     UUID REFERENCES schools(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    message       TEXT NOT NULL,
    target_roles  TEXT[] NOT NULL DEFAULT '{}', -- empty array = visible to all roles
    created_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_announcements_school ON announcements(school_id);

-- Rows parsed from the Sekoci import template's "Data Kelas" / "Data mata
-- pelajaran" sheets that belong to academic-service. Staged here (not
-- dispatched) until academic-service exists; see README Assumptions +
-- internal/handler/import.go TODO for the intended HTTP hand-off contract.
CREATE TABLE import_staged_rows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('kelas','mata_pelajaran')),
    payload     JSONB NOT NULL,
    dispatched  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_import_staged_rows_school ON import_staged_rows(school_id);
