CREATE TABLE qa_instruments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    issuer TEXT NOT NULL,
    version_label TEXT NOT NULL,
    score_min INT NOT NULL,
    score_max INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'retired')),
    effective_from DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT chk_score_range CHECK (score_max > score_min),
    CONSTRAINT uq_instrument_code_version UNIQUE (school_id, code, version_label)
);
CREATE INDEX idx_qa_instruments_school ON qa_instruments(school_id) WHERE deleted_at IS NULL;

CREATE TABLE qa_standards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    instrument_id UUID NOT NULL REFERENCES qa_instruments(id) ON DELETE CASCADE,
    sort INT NOT NULL DEFAULT 0,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    weight_bp INT NOT NULL DEFAULT 0 CHECK (weight_bp >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_qa_standards_inst ON qa_standards(instrument_id, sort);

CREATE TABLE qa_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    standard_id UUID NOT NULL REFERENCES qa_standards(id) ON DELETE CASCADE,
    sort INT NOT NULL DEFAULT 0,
    code TEXT NOT NULL,
    statement TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_qa_items_std ON qa_items(standard_id, sort);

CREATE TABLE qa_indicators (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    item_id UUID NOT NULL REFERENCES qa_items(id) ON DELETE CASCADE,
    sort INT NOT NULL DEFAULT 0,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    weight_bp INT NOT NULL DEFAULT 0 CHECK (weight_bp >= 0),
    unit TEXT NOT NULL CHECK (unit IN ('percent_bp', 'count', 'ratio_milli', 'idr', 'days', 'score')),
    direction TEXT NOT NULL CHECK (direction IN ('higher_better', 'lower_better')),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('manual', 'metric')),
    metric_key TEXT NULL,
    rubric JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT chk_metric_key CHECK (
        (source_kind = 'metric' AND metric_key IS NOT NULL) OR
        (source_kind <> 'metric' AND metric_key IS NULL)
    )
);
CREATE INDEX idx_qa_indicators_item ON qa_indicators(item_id, sort);

CREATE TABLE qa_targets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    indicator_id UUID NOT NULL REFERENCES qa_indicators(id) ON DELETE CASCADE,
    academic_year_id UUID NOT NULL,
    target_value BIGINT NOT NULL,
    owner_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_qa_target UNIQUE (school_id, indicator_id, academic_year_id)
);
CREATE INDEX idx_qa_targets_indicator ON qa_targets(indicator_id, academic_year_id);
