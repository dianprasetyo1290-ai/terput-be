-- Delete the rows the demo generators created (their ids are v5-shaped:
-- xxxxxxxx-xxxx-5xxx-...) from the given tables of the current database, so
-- a generator can rebuild them. Tables without an id column lose the rows
-- that point at a generated row. Repeats until nothing more can be deleted,
-- so foreign-key order does not matter. Usage (psql):
--   \set tables '{canteen_sales,canteen_sale_items}'   (or NULL for all)
CREATE OR REPLACE FUNCTION pg_temp.purge_generated(only_tables text[]) RETURNS bigint
LANGUAGE plpgsql AS $$
DECLARE
  t record;
  pass int;
  n bigint;
  total bigint := 0;
  progressed boolean;
  cond text;
BEGIN
  FOR pass IN 1..12 LOOP
    progressed := false;
    FOR t IN SELECT tb.table_name AS name FROM information_schema.tables tb
             WHERE tb.table_schema = 'public' AND tb.table_type = 'BASE TABLE' AND tb.table_name <> 'schema_migrations'
               AND (only_tables IS NULL OR tb.table_name = ANY(only_tables)) LOOP
      IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = t.name AND column_name = 'id' AND data_type = 'uuid') THEN
        cond := 'id::text ~ ''^[0-9a-f]{8}-[0-9a-f]{4}-5''';
      ELSE
        SELECT string_agg(format('%I::text ~ ''^[0-9a-f]{8}-[0-9a-f]{4}-5''', column_name), ' OR ') INTO cond
        FROM information_schema.columns WHERE table_schema = 'public' AND table_name = t.name AND data_type = 'uuid' AND column_name <> 'school_id';
      END IF;
      CONTINUE WHEN cond IS NULL;
      BEGIN
        EXECUTE format('DELETE FROM %I WHERE %s', t.name, cond);
        GET DIAGNOSTICS n = ROW_COUNT;
        IF n > 0 THEN total := total + n; progressed := true; END IF;
      EXCEPTION WHEN foreign_key_violation THEN
        progressed := true; -- a child table goes first; try again next pass
      END;
    END LOOP;
    EXIT WHEN NOT progressed;
  END LOOP;
  RETURN total;
END $$;
