ALTER TABLE users DROP COLUMN extra;

ALTER TABLE import_staged_rows DROP CONSTRAINT import_staged_rows_kind_check;
ALTER TABLE import_staged_rows ADD CONSTRAINT import_staged_rows_kind_check
    CHECK (kind IN ('kelas','mata_pelajaran'));
