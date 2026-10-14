-- QA-IDN BUG-020/077: every committed user import is a batch with its rows,
-- so it can be listed, traced to the source file/ID and rolled back while
-- the accounts are still untouched.
CREATE TABLE import_batches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       UUID REFERENCES schools(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL DEFAULT 'users',
    file_name       TEXT,
    file_format     TEXT NOT NULL CHECK (file_format IN ('xlsx','csv')),
    total_rows      INT NOT NULL DEFAULT 0,
    created_rows    INT NOT NULL DEFAULT 0,
    skipped_rows    INT NOT NULL DEFAULT 0,
    created_by      UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    rolled_back_at  TIMESTAMPTZ,
    rolled_back_by  UUID,
    rollback_reason TEXT
);
CREATE INDEX idx_import_batches_school ON import_batches(school_id, created_at DESC);

CREATE TABLE import_batch_items (
    batch_id    UUID NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
    row_number  INT NOT NULL,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL,
    email       TEXT NOT NULL, -- original address (freed on rollback)
    source_ref  TEXT,          -- "ID Sumber" column: id in the legacy system
    PRIMARY KEY (batch_id, row_number)
);
CREATE INDEX idx_import_batch_items_user ON import_batch_items(user_id);
CREATE INDEX idx_import_batch_items_source ON import_batch_items(source_ref) WHERE source_ref IS NOT NULL;
