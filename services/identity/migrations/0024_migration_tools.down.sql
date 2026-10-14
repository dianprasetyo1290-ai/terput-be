DROP TRIGGER IF EXISTS trg_archive_records_read_only ON archive_records;
DROP FUNCTION IF EXISTS archive_records_read_only();
DROP TABLE IF EXISTS archive_records;
DROP TABLE IF EXISTS source_id_map;
-- Rows that only exist for the new kinds/entities cannot be kept in the old shape.
DELETE FROM import_batch_items WHERE entity_type <> 'user';
ALTER TABLE import_batch_items DROP CONSTRAINT import_batch_items_pkey;
ALTER TABLE import_batch_items ADD PRIMARY KEY (batch_id, row_number);
ALTER TABLE import_batch_items DROP COLUMN created_account, DROP COLUMN entity_id, DROP COLUMN entity_type;
DELETE FROM import_batches WHERE kind <> 'users';
DROP INDEX IF EXISTS idx_import_batches_run;
ALTER TABLE import_batches DROP CONSTRAINT IF EXISTS import_batches_kind_check,
    DROP COLUMN IF EXISTS trial_refs, DROP COLUMN category_counts, DROP COLUMN trial, DROP COLUMN run_id;
DROP TABLE IF EXISTS migration_control_totals;
DROP TABLE IF EXISTS migration_runs;
