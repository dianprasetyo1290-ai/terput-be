CREATE TABLE regulatory_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    recipient TEXT NOT NULL,
    frequency TEXT NOT NULL CHECK (frequency IN ('monthly', 'semester', 'annual', 'adhoc')),
    columns JSONB NOT NULL DEFAULT '[]'::jsonb,
    export_format TEXT NOT NULL DEFAULT 'xlsx' CHECK (export_format IN ('xlsx', 'csv')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_regulatory_template_code UNIQUE (school_id, code)
);
CREATE INDEX idx_regulatory_templates_school ON regulatory_templates(school_id) WHERE deleted_at IS NULL;

CREATE TABLE regulatory_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    template_id UUID NOT NULL REFERENCES regulatory_templates(id) ON DELETE RESTRICT,
    period_from DATE NOT NULL,
    period_to DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_approval', 'approved', 'rejected', 'submitted')),
    approval_id UUID NULL,
    template_snapshot JSONB NULL,
    data_snapshot JSONB NULL,
    snapshot_at TIMESTAMPTZ NULL,
    submitted_at TIMESTAMPTZ NULL,
    submitted_by UUID NULL,
    submission_note TEXT NULL,
    export_object_key TEXT NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_regulatory_report_idempotency UNIQUE (school_id, idempotency_key)
);
CREATE UNIQUE INDEX uq_regulatory_report_period ON regulatory_reports(template_id, period_from, period_to) WHERE status <> 'rejected';
CREATE INDEX idx_regulatory_reports_school ON regulatory_reports(school_id, status);
