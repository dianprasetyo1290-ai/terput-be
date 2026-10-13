CREATE TABLE dapodik_field_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    entity TEXT NOT NULL CHECK (entity IN ('student', 'rombel', 'ptk')),
    dapodik_column TEXT NOT NULL,
    sekoci_field TEXT NOT NULL,
    is_match_key BOOLEAN NOT NULL DEFAULT false,
    match_priority INT NULL,
    normalize TEXT NOT NULL DEFAULT 'none' CHECK (normalize IN ('none', 'trim_upper', 'digits', 'date')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_dapodik_mapping UNIQUE (school_id, entity, dapodik_column)
);
CREATE INDEX idx_dapodik_mappings_school ON dapodik_field_mappings(school_id, entity);

CREATE TABLE dapodik_imports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    entity TEXT NOT NULL CHECK (entity IN ('student', 'rombel', 'ptk')),
    file_name TEXT NOT NULL,
    file_checksum TEXT NOT NULL,
    row_count INT NOT NULL DEFAULT 0,
    imported_by UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'parsed' CHECK (status IN ('parsed', 'compared', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_dapodik_imports_school ON dapodik_imports(school_id, entity, created_at DESC);

CREATE TABLE dapodik_import_rows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    import_id UUID NOT NULL REFERENCES dapodik_imports(id) ON DELETE CASCADE,
    row_no INT NOT NULL,
    data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_dapodik_import_rows_import ON dapodik_import_rows(import_id, row_no);

CREATE TABLE dapodik_diffs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    import_id UUID NOT NULL REFERENCES dapodik_imports(id) ON DELETE CASCADE,
    entity TEXT NOT NULL CHECK (entity IN ('student', 'rombel', 'ptk')),
    sekoci_ref TEXT NULL,
    import_row_id UUID NULL REFERENCES dapodik_import_rows(id) ON DELETE SET NULL,
    kind TEXT NOT NULL CHECK (kind IN ('field_mismatch', 'missing_in_dapodik', 'missing_in_sekoci')),
    field TEXT NULL,
    sekoci_value TEXT NULL,
    dapodik_value TEXT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'decided')),
    decision TEXT NULL CHECK (decision IN ('update_dapodik', 'update_sekoci', 'ignore')),
    decision_reason TEXT NULL,
    decided_by UUID NULL,
    decided_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_dapodik_diffs_import ON dapodik_diffs(import_id, status);

CREATE TABLE dapodik_sync_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    entity TEXT NOT NULL CHECK (entity IN ('student', 'rombel', 'ptk')),
    action TEXT NOT NULL CHECK (action IN ('import', 'compare', 'decide', 'export')),
    actor_user_id UUID NOT NULL,
    import_id UUID NULL REFERENCES dapodik_imports(id) ON DELETE SET NULL,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_dapodik_sync_logs_school ON dapodik_sync_logs(school_id, created_at DESC);
