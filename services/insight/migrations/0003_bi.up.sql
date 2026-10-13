CREATE TABLE IF NOT EXISTS dashboards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    owner_user_id UUID NOT NULL,
    title TEXT NOT NULL,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN ('unit', 'foundation')),
    shared_roles TEXT[] NOT NULL DEFAULT '{}',
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dashboard_widgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    sort INT NOT NULL DEFAULT 0,
    title TEXT NOT NULL,
    metric_key TEXT NOT NULL REFERENCES metric_definitions(key),
    chart TEXT NOT NULL CHECK (chart IN ('stat', 'bar', 'line', 'table')),
    group_by TEXT,
    period JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS saved_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    owner_user_id UUID NOT NULL,
    title TEXT NOT NULL,
    columns JSONB NOT NULL,
    filters JSONB NOT NULL,
    shared_roles TEXT[] NOT NULL DEFAULT '{}',
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scheduled_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    saved_report_id UUID NOT NULL REFERENCES saved_reports(id) ON DELETE CASCADE,
    frequency TEXT NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    weekday INT,
    month_day INT,
    send_hour INT NOT NULL CHECK (send_hour BETWEEN 0 AND 23),
    recipient_user_ids UUID[] NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    next_run_at TIMESTAMPTZ NOT NULL,
    last_run_at TIMESTAMPTZ,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scheduled_report_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    schedule_id UUID NOT NULL REFERENCES scheduled_reports(id) ON DELETE CASCADE,
    recipient_user_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('sent', 'skipped', 'failed')),
    reason TEXT,
    deliveries JSONB,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(school_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS analytics_scope_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    user_id UUID NOT NULL,
    granted_school_id UUID NOT NULL,
    granted_by UUID NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_scope_grants_active
    ON analytics_scope_grants(user_id, granted_school_id)
    WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS bi_export_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    metric_keys TEXT[] NOT NULL,
    school_ids UUID[] NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
