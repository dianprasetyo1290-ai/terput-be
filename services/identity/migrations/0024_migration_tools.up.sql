-- QA-IDN putaran 2: alat migrasi data dari sistem lama.
--   BUG-019 control totals + sign-off       -> migration_runs, migration_control_totals
--   BUG-078 migrasi percobaan (trial)       -> migration_runs.mode, import_batches.trial
--   BUG-021 impor orang tua & kelas         -> import_batches.kind, import_batch_items.entity_*
--   BUG-077 peta ID sumber -> ID baru       -> source_id_map
--   BUG-079 arsip historis read-only        -> archive_records

CREATE TABLE migration_runs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id      UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    title          TEXT NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 150),
    mode           TEXT NOT NULL CHECK (mode IN ('trial', 'production')),
    status         TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'signed_off', 'discarded')),
    trial_run_id   UUID REFERENCES migration_runs(id) ON DELETE RESTRICT,
    notes          TEXT,
    created_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    signed_off_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    signed_off_at  TIMESTAMPTZ,
    sign_off_note  TEXT,
    discarded_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    discarded_at   TIMESTAMPTZ,
    discard_reason TEXT,
    -- A production run is only opened from a signed-off trial run.
    CHECK (mode = 'trial' OR trial_run_id IS NOT NULL),
    CHECK (status <> 'signed_off' OR (signed_off_by IS NOT NULL AND signed_off_at IS NOT NULL))
);
CREATE INDEX idx_migration_runs_school ON migration_runs(school_id, created_at DESC);

CREATE TABLE migration_control_totals (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID NOT NULL REFERENCES migration_runs(id) ON DELETE CASCADE,
    category      TEXT NOT NULL CHECK (category ~ '^[a-z0-9_]{2,40}$'),
    metric        TEXT NOT NULL DEFAULT 'count' CHECK (metric IN ('count', 'sum_amount')),
    label         TEXT NOT NULL,
    source_value  NUMERIC(18,2),
    target_value  NUMERIC(18,2),
    target_source TEXT NOT NULL DEFAULT 'manual' CHECK (target_source IN ('auto', 'manual')),
    diff          NUMERIC(18,2) GENERATED ALWAYS AS (COALESCE(target_value, 0) - COALESCE(source_value, 0)) STORED,
    explanation   TEXT,
    updated_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (run_id, category, metric)
);

ALTER TABLE import_batches
    ADD COLUMN run_id          UUID REFERENCES migration_runs(id) ON DELETE SET NULL,
    ADD COLUMN trial           BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN category_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Trial batches store nothing; the source ids of students they would
    -- have created let a following trial import of parents/classes resolve.
    ADD COLUMN trial_refs      TEXT[] NOT NULL DEFAULT '{}',
    ADD CONSTRAINT import_batches_kind_check CHECK (kind IN ('users', 'orang_tua', 'kelas'));
CREATE INDEX idx_import_batches_run ON import_batches(run_id) WHERE run_id IS NOT NULL;

ALTER TABLE import_batch_items
    ADD COLUMN entity_type TEXT NOT NULL DEFAULT 'user'
        CHECK (entity_type IN ('user', 'guardian_link', 'enrollment')),
    ADD COLUMN entity_id   TEXT,
    ADD COLUMN created_account BOOLEAN NOT NULL DEFAULT true;
-- One spreadsheet row can produce an account and a link (orang_tua).
ALTER TABLE import_batch_items DROP CONSTRAINT import_batch_items_pkey;
ALTER TABLE import_batch_items ADD PRIMARY KEY (batch_id, row_number, entity_type);

CREATE TABLE source_id_map (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK (entity_type ~ '^[a-z_]{2,30}$'),
    source_ref  TEXT NOT NULL CHECK (length(btrim(source_ref)) BETWEEN 1 AND 100),
    target_id   TEXT NOT NULL,
    batch_id    UUID REFERENCES import_batches(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, entity_type, source_ref)
);
CREATE INDEX idx_source_id_map_target ON source_id_map(target_id);

-- Backfill from user imports made before this migration (BUG-077).
INSERT INTO source_id_map (school_id, entity_type, source_ref, target_id, batch_id, created_at)
SELECT DISTINCT ON (b.school_id, i.role, i.source_ref)
       b.school_id, i.role, i.source_ref, i.user_id::text, b.id, b.created_at
FROM import_batch_items i
JOIN import_batches b ON b.id = i.batch_id
WHERE i.source_ref IS NOT NULL AND b.school_id IS NOT NULL AND b.rolled_back_at IS NULL
  AND i.role ~ '^[a-z_]{2,30}$'
ORDER BY b.school_id, i.role, i.source_ref, b.created_at DESC
ON CONFLICT DO NOTHING;

CREATE TABLE archive_records (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    run_id      UUID REFERENCES migration_runs(id) ON DELETE SET NULL,
    category    TEXT NOT NULL CHECK (category ~ '^[a-z0-9_]{2,40}$'),
    source_ref  TEXT,
    title       TEXT NOT NULL,
    payload     JSONB NOT NULL,
    search      TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', title || ' ' || payload::text)) STORED,
    file_name   TEXT,
    imported_by UUID REFERENCES users(id) ON DELETE SET NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_archive_school_category ON archive_records(school_id, category);
CREATE INDEX idx_archive_search ON archive_records USING GIN (search);
CREATE INDEX idx_archive_payload ON archive_records USING GIN (payload jsonb_path_ops);

-- Arsip bersifat read-only: baris tidak boleh diubah setelah diimpor.
CREATE FUNCTION archive_records_read_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'archive_records is read-only' USING ERRCODE = '42501';
END $$;
CREATE TRIGGER trg_archive_records_read_only BEFORE UPDATE ON archive_records
    FOR EACH ROW EXECUTE FUNCTION archive_records_read_only();
