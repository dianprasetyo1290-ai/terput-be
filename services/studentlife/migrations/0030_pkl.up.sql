-- 0030_pkl.up.sql: Master, penempatan, absensi, jurnal, penilaian & sertifikat PKL (T22)

CREATE TABLE IF NOT EXISTS majors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_majors_school_code ON majors (school_id, code) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS classroom_majors (
    classroom_id UUID PRIMARY KEY,
    school_id UUID NOT NULL,
    major_id UUID NOT NULL REFERENCES majors(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS pkl_partners (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    field TEXT NULL,
    address TEXT NULL,
    contact_name TEXT NULL,
    contact_phone TEXT NULL,
    contact_email TEXT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS pkl_partner_mous (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    partner_id UUID NOT NULL REFERENCES pkl_partners(id) ON DELETE CASCADE,
    mou_number TEXT NOT NULL,
    valid_from DATE NOT NULL,
    valid_to DATE NOT NULL,
    object_key TEXT NULL,
    file_name TEXT NULL,
    mime TEXT NULL,
    uploaded_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (valid_to >= valid_from)
);

CREATE TABLE IF NOT EXISTS pkl_periods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    academic_year_id UUID NULL,
    name TEXT NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'planning' CHECK (status IN ('planning', 'ongoing', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_on >= starts_on)
);

CREATE TABLE IF NOT EXISTS pkl_quotas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    partner_id UUID NOT NULL REFERENCES pkl_partners(id) ON DELETE CASCADE,
    period_id UUID NOT NULL REFERENCES pkl_periods(id) ON DELETE CASCADE,
    major_id UUID NOT NULL REFERENCES majors(id) ON DELETE CASCADE,
    quota INT NOT NULL CHECK (quota >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (partner_id, period_id, major_id)
);

CREATE TABLE IF NOT EXISTS industry_mentors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    partner_id UUID NOT NULL REFERENCES pkl_partners(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    full_name TEXT NOT NULL,
    email TEXT NOT NULL,
    phone TEXT NULL,
    job_title TEXT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deactivated_at TIMESTAMPTZ NULL,
    UNIQUE (school_id, partner_id, user_id)
);

CREATE TABLE IF NOT EXISTS pkl_settings (
    school_id UUID PRIMARY KEY,
    certificate_prefix TEXT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Penempatan & Absensi (140)
CREATE TABLE IF NOT EXISTS pkl_placements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    period_id UUID NOT NULL REFERENCES pkl_periods(id) ON DELETE RESTRICT,
    partner_id UUID NOT NULL REFERENCES pkl_partners(id) ON DELETE RESTRICT,
    major_id UUID NOT NULL REFERENCES majors(id) ON DELETE RESTRICT,
    student_id UUID NOT NULL,
    school_mentor_user_id UUID NOT NULL,
    industry_mentor_id UUID NULL REFERENCES industry_mentors(id) ON DELETE SET NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'active', 'completed', 'cancelled')),
    cancel_reason TEXT NULL,
    created_by UUID NULL,
    idempotency_key TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_placements_period_student ON pkl_placements(period_id, student_id) WHERE status <> 'cancelled';

CREATE TABLE IF NOT EXISTS pkl_attendance (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    placement_id UUID NOT NULL REFERENCES pkl_placements(id) ON DELETE CASCADE,
    work_date DATE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('hadir', 'izin', 'sakit')),
    check_in_at TIMESTAMPTZ NULL,
    check_out_at TIMESTAMPTZ NULL,
    note TEXT NULL,
    attachment_key TEXT NULL,
    confirmation TEXT NOT NULL DEFAULT 'pending' CHECK (confirmation IN ('pending', 'confirmed', 'rejected')),
    confirmed_by UUID NULL,
    confirmed_at TIMESTAMPTZ NULL,
    reject_reason TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (placement_id, work_date)
);

-- Jurnal, Laporan, Skema Nilai, Hasil & Sertifikat (141)
CREATE TABLE IF NOT EXISTS pkl_journals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    placement_id UUID NOT NULL REFERENCES pkl_placements(id) ON DELETE CASCADE,
    entry_date DATE NOT NULL,
    activity TEXT NOT NULL,
    photos JSONB NOT NULL DEFAULT '[]'::jsonb,
    mentor_note TEXT NULL,
    mentor_note_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (placement_id, entry_date)
);

CREATE TABLE IF NOT EXISTS pkl_final_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    placement_id UUID NOT NULL REFERENCES pkl_placements(id) ON DELETE CASCADE,
    version INT NOT NULL,
    object_key TEXT NOT NULL,
    file_name TEXT NULL,
    mime TEXT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (placement_id, version)
);

CREATE TABLE IF NOT EXISTS pkl_schemes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    period_id UUID NOT NULL REFERENCES pkl_periods(id) ON DELETE CASCADE UNIQUE,
    industry_weight_bp INT NOT NULL,
    school_weight_bp INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (industry_weight_bp >= 0 AND school_weight_bp >= 0 AND industry_weight_bp + school_weight_bp = 10000)
);

CREATE TABLE IF NOT EXISTS pkl_components (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    scheme_id UUID NOT NULL REFERENCES pkl_schemes(id) ON DELETE CASCADE,
    assessor TEXT NOT NULL CHECK (assessor IN ('industry', 'school')),
    name TEXT NOT NULL,
    weight_bp INT NOT NULL CHECK (weight_bp > 0),
    sort INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS pkl_scores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    placement_id UUID NOT NULL REFERENCES pkl_placements(id) ON DELETE CASCADE,
    component_id UUID NOT NULL REFERENCES pkl_components(id) ON DELETE CASCADE,
    score_x100 INT NOT NULL CHECK (score_x100 BETWEEN 0 AND 10000),
    scored_by UUID NOT NULL,
    scored_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (placement_id, component_id)
);

CREATE TABLE IF NOT EXISTS pkl_results (
    placement_id UUID PRIMARY KEY REFERENCES pkl_placements(id) ON DELETE CASCADE,
    school_id UUID NOT NULL,
    industry_x100 INT NULL,
    school_x100 INT NULL,
    final_x100 INT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finalized_by UUID NULL,
    finalized_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS pkl_certificates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    placement_id UUID NOT NULL REFERENCES pkl_placements(id) ON DELETE CASCADE UNIQUE,
    certificate_no TEXT NOT NULL,
    object_key TEXT NOT NULL,
    issued_by UUID NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, certificate_no)
);
