-- 0020_ats.up.sql: Applicant Tracking System (Menu 88-92)

CREATE TABLE IF NOT EXISTS requisitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    position_id UUID NOT NULL,
    department_id UUID NULL,
    quantity INT NOT NULL CHECK(quantity > 0),
    reason TEXT NOT NULL CHECK(reason IN ('replacement', 'addition')),
    replaces_employee_id UUID NULL,
    qualifications TEXT NOT NULL DEFAULT '',
    budget_idr BIGINT NULL,
    exceeds_headcount BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'pending_approval', 'approved', 'rejected', 'filled', 'cancelled')),
    filled_count INT NOT NULL DEFAULT 0,
    approval_id UUID NULL,
    requested_by UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_requisition_school_idempotency UNIQUE(school_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS job_postings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    requisition_id UUID NOT NULL REFERENCES requisitions(id),
    slug TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    qualifications TEXT NOT NULL DEFAULT '',
    employment_type TEXT NOT NULL DEFAULT 'full_time',
    closes_on DATE NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'open', 'closed')),
    closed_reason TEXT NULL,
    published_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_postings_school_slug UNIQUE(school_id, slug)
);

CREATE TABLE IF NOT EXISTS pipeline_stages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    sort INT NOT NULL DEFAULT 0,
    kind TEXT NOT NULL DEFAULT 'normal' CHECK(kind IN ('normal', 'offer', 'hired', 'rejected')),
    notify_template_key TEXT NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    full_name TEXT NOT NULL,
    email TEXT NOT NULL,
    email_norm TEXT NOT NULL,
    phone_enc BYTEA NULL,
    consent_processing_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    consent_talent_pool BOOLEAN NOT NULL DEFAULT false,
    anonymized_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_candidate_school_email UNIQUE(school_id, email_norm)
);

CREATE TABLE IF NOT EXISTS applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    posting_id UUID NOT NULL REFERENCES job_postings(id),
    candidate_id UUID NOT NULL REFERENCES candidates(id),
    stage_id UUID NOT NULL REFERENCES pipeline_stages(id),
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'rejected', 'withdrawn', 'hired')),
    cv_object_key TEXT NOT NULL DEFAULT '',
    answers JSONB NOT NULL DEFAULT '{}'::jsonb,
    source TEXT NOT NULL DEFAULT 'careers_portal',
    access_token_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_application_posting_candidate UNIQUE(posting_id, candidate_id)
);

CREATE TABLE IF NOT EXISTS application_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    application_id UUID NOT NULL REFERENCES applications(id),
    from_stage UUID NULL,
    to_stage UUID NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    actor_user_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS application_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    application_id UUID NOT NULL REFERENCES applications(id),
    kind TEXT NOT NULL,
    object_key TEXT NOT NULL,
    file_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scorecard_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    position_id UUID NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scorecard_criteria (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    template_id UUID NOT NULL REFERENCES scorecard_templates(id),
    name TEXT NOT NULL,
    weight_bp INT NOT NULL CHECK(weight_bp > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS interviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    application_id UUID NOT NULL REFERENCES applications(id),
    kind TEXT NOT NULL DEFAULT 'interview' CHECK(kind IN ('interview', 'microteaching', 'test')),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    location_or_link TEXT NOT NULL DEFAULT '',
    template_id UUID NULL REFERENCES scorecard_templates(id),
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK(status IN ('scheduled', 'done', 'cancelled', 'rescheduled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS interview_panel (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    interview_id UUID NOT NULL REFERENCES interviews(id),
    user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_panel_interview_user UNIQUE(interview_id, user_id)
);

CREATE TABLE IF NOT EXISTS interview_scores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    interview_id UUID NOT NULL REFERENCES interviews(id),
    panel_user_id UUID NOT NULL,
    criterion_id UUID NOT NULL,
    score INT NOT NULL CHECK(score BETWEEN 0 AND 100),
    recommendation TEXT NULL,
    submitted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_panel_score_criterion UNIQUE(interview_id, panel_user_id, criterion_id)
);

CREATE TABLE IF NOT EXISTS offers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    application_id UUID NOT NULL REFERENCES applications(id),
    position_id UUID NOT NULL,
    start_date DATE NOT NULL,
    employment_type TEXT NOT NULL DEFAULT 'full_time',
    base_salary_enc BYTEA NOT NULL,
    out_of_range BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'pending_approval', 'approved', 'sent', 'accepted', 'declined', 'rejected', 'expired')),
    approval_id UUID NULL,
    document_object_key TEXT NOT NULL DEFAULT '',
    responded_at TIMESTAMPTZ NULL,
    response_note TEXT NULL,
    employee_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ats_settings (
    school_id UUID PRIMARY KEY,
    retention_months INT NOT NULL DEFAULT 6,
    career_page_enabled BOOLEAN NOT NULL DEFAULT false,
    career_intro TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
