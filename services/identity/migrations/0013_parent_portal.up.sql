-- Parent portal identity primitives. All relationships and external references
-- are school scoped; secrets are stored only as password/claim hashes.
ALTER TABLE user_school_roles DROP CONSTRAINT IF EXISTS user_school_roles_role_check;
ALTER TABLE user_school_roles ADD CONSTRAINT user_school_roles_role_check
    CHECK (role IN ('superadmin','admin','guru','siswa','orang_tua','walikelas','kepala_sekolah','kepala_yayasan','pengawas'));

-- Remove legacy recoverable password copies. The column remains for backwards
-- compatible reads, but identity no longer writes it.
UPDATE users SET default_password = NULL WHERE default_password IS NOT NULL;

CREATE TABLE guardian_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    guardian_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    student_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    relationship TEXT NOT NULL DEFAULT 'wali',
    is_primary BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('pending','active','inactive')),
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (guardian_user_id <> student_user_id),
    UNIQUE (school_id, guardian_user_id, student_user_id)
);
CREATE INDEX idx_guardian_links_guardian_tenant ON guardian_links(school_id, guardian_user_id, status);
CREATE INDEX idx_guardian_links_student_tenant ON guardian_links(school_id, student_user_id, status);

CREATE TABLE guardian_claim_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    student_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    used_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    CHECK (expires_at > created_at)
);
CREATE INDEX idx_guardian_claim_active ON guardian_claim_codes(school_id, student_user_id, expires_at)
    WHERE used_at IS NULL AND revoked_at IS NULL;

CREATE TABLE guardian_claim_rate_limits (
    school_id UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    guardian_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    window_started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    PRIMARY KEY (school_id, guardian_user_id)
);

CREATE TABLE identity_external_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('guardian','student')),
    external_ref TEXT NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, source, entity_type, external_ref),
    UNIQUE (school_id, source, entity_type, user_id)
);
CREATE INDEX idx_identity_external_refs_user ON identity_external_refs(school_id, user_id);

CREATE TABLE admission_conversion_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    source TEXT NOT NULL DEFAULT 'admission',
    conversion_ref TEXT NOT NULL,
    guardian_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    student_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    guardian_link_id UUID REFERENCES guardian_links(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, source, conversion_ref)
);
CREATE INDEX idx_admission_conversion_mapping_tenant ON admission_conversion_mappings(school_id, source, conversion_ref);
