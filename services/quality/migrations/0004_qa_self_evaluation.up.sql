CREATE TABLE qa_self_evaluations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    instrument_id UUID NOT NULL REFERENCES qa_instruments(id) ON DELETE RESTRICT,
    academic_year_id UUID NOT NULL,
    period_label TEXT NOT NULL,
    period_from DATE NOT NULL,
    period_to DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_finalization', 'finalized', 'pending_reopen')),
    approval_id UUID NULL,
    reopen_approval_id UUID NULL,
    finalized_at TIMESTAMPTZ NULL,
    finalized_by UUID NULL,
    version INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_self_eval_period UNIQUE (school_id, instrument_id, academic_year_id, period_label)
);
CREATE INDEX idx_qa_self_eval_school ON qa_self_evaluations(school_id, status);

CREATE TABLE qa_self_eval_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    self_evaluation_id UUID NOT NULL REFERENCES qa_self_evaluations(id) ON DELETE CASCADE,
    indicator_id UUID NOT NULL REFERENCES qa_indicators(id) ON DELETE RESTRICT,
    target_value BIGINT NULL,
    actual_value BIGINT NULL,
    value_source TEXT NOT NULL DEFAULT 'manual' CHECK (value_source IN ('manual', 'metric')),
    metric_as_of TIMESTAMPTZ NULL,
    metric_fetched_at TIMESTAMPTZ NULL,
    score INT NULL,
    below_target BOOLEAN NOT NULL DEFAULT false,
    note TEXT NULL,
    assessed_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_self_eval_item UNIQUE (self_evaluation_id, indicator_id)
);
CREATE INDEX idx_qa_self_eval_items_eval ON qa_self_eval_items(self_evaluation_id);

CREATE TABLE qa_action_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    self_eval_item_id UUID NOT NULL REFERENCES qa_self_eval_items(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    owner_user_id UUID NOT NULL,
    due_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'done', 'cancelled')),
    done_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_qa_action_plans_item ON qa_action_plans(self_eval_item_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_qa_action_plans_owner ON qa_action_plans(school_id, owner_user_id, status) WHERE deleted_at IS NULL;

CREATE TABLE qa_self_eval_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    self_evaluation_id UUID NOT NULL REFERENCES qa_self_evaluations(id) ON DELETE CASCADE,
    version INT NOT NULL,
    snapshot JSONB NOT NULL,
    finalized_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finalized_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_self_eval_version UNIQUE (self_evaluation_id, version)
);
CREATE INDEX idx_qa_self_eval_versions_eval ON qa_self_eval_versions(self_evaluation_id, version);
