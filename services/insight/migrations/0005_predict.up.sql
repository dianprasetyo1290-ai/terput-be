CREATE TABLE IF NOT EXISTS insight_settings (
    school_id UUID PRIMARY KEY,
    predictive_enabled BOOLEAN NOT NULL DEFAULT false,
    enabled_by UUID,
    enabled_at TIMESTAMPTZ,
    review_note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS risk_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    model TEXT NOT NULL CHECK (model IN ('arrears_risk', 'absence_risk', 'ppdb_forecast', 'cashflow_forecast')),
    version INT NOT NULL,
    params JSONB NOT NULL,
    effective_from DATE NOT NULL,
    created_by UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'retired')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(school_id, model, version)
);

CREATE TABLE IF NOT EXISTS prediction_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    model TEXT NOT NULL,
    rule_id UUID NOT NULL REFERENCES risk_rules(id),
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status TEXT NOT NULL CHECK (status IN ('ok', 'failed', 'skipped')),
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS predictions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    run_id UUID NOT NULL REFERENCES prediction_runs(id) ON DELETE CASCADE,
    model TEXT NOT NULL,
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('student', 'wave', 'month')),
    subject_id TEXT NOT NULL,
    score INT,
    level TEXT CHECK (level IN ('low', 'medium', 'high')),
    projected_value BIGINT,
    factors JSONB NOT NULL,
    review_label TEXT NOT NULL DEFAULT 'perlu_tinjauan',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS prediction_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    prediction_id UUID NOT NULL REFERENCES predictions(id) ON DELETE CASCADE,
    reviewed_by UUID NOT NULL,
    note TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS prediction_accuracy (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    model TEXT NOT NULL,
    rule_id UUID NOT NULL REFERENCES risk_rules(id),
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    measure TEXT NOT NULL CHECK (measure IN ('precision_bp', 'recall_bp', 'mape_bp')),
    value_bp INT NOT NULL,
    sample_size INT NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
