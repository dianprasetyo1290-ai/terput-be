-- Repoint every reference to a duplicate id at the id that is kept, in the
-- database this runs in. Used by scripts/dedupe-demo-data.sh, run once per
-- database. Every uuid, uuid[] and text column of the public schema is
-- scanned (except single-column primary keys: the duplicate row itself is
-- deleted by the caller afterwards). A row that would collide with a row the
-- kept id already has (a unique key) is a duplicate of that row and is deleted.
CREATE OR REPLACE FUNCTION pg_temp.remap(dup uuid, keep uuid) RETURNS integer
LANGUAGE plpgsql AS $$
DECLARE
  c record;
  r record;
  moved integer := 0;
  n integer;
BEGIN
  FOR c IN
    SELECT col.table_name AS t, col.column_name AS k, col.data_type AS ty
    FROM information_schema.columns col
    JOIN information_schema.tables tb ON tb.table_schema = col.table_schema AND tb.table_name = col.table_name
    WHERE col.table_schema = 'public' AND tb.table_type = 'BASE TABLE'
      AND (col.data_type IN ('uuid', 'text', 'character varying') OR col.udt_name = '_uuid')
      AND NOT EXISTS (
        SELECT 1 FROM pg_index i
        JOIN pg_class cl ON cl.oid = i.indrelid
        JOIN pg_namespace ns ON ns.oid = cl.relnamespace AND ns.nspname = 'public'
        JOIN pg_attribute a ON a.attrelid = cl.oid AND a.attnum = i.indkey[0]
        WHERE i.indisprimary AND i.indnatts = 1 AND cl.relname = col.table_name AND a.attname = col.column_name)
  LOOP
    IF c.ty = 'ARRAY' THEN
      EXECUTE format('UPDATE %I SET %I = array_replace(%I, $1, $2) WHERE $1 = ANY(%I)', c.t, c.k, c.k, c.k) USING dup, keep;
      GET DIAGNOSTICS n = ROW_COUNT;
      moved := moved + n;
      CONTINUE;
    END IF;
    BEGIN
      IF c.ty = 'uuid' THEN
        EXECUTE format('UPDATE %I SET %I = $2 WHERE %I = $1', c.t, c.k, c.k) USING dup, keep;
      ELSE
        EXECUTE format('UPDATE %I SET %I = $2::text WHERE %I = $1::text', c.t, c.k, c.k) USING dup, keep;
      END IF;
      GET DIAGNOSTICS n = ROW_COUNT;
      moved := moved + n;
    EXCEPTION WHEN unique_violation OR exclusion_violation THEN
      -- Row by row: the colliding ones duplicate a row of the kept id.
      FOR r IN EXECUTE format('SELECT ctid FROM %I WHERE %I::text = $1::text', c.t, c.k) USING dup LOOP
        BEGIN
          EXECUTE format('UPDATE %I SET %I = %L WHERE ctid = $1', c.t, c.k, keep::text) USING r.ctid;
          moved := moved + 1;
        EXCEPTION WHEN unique_violation OR exclusion_violation THEN
          EXECUTE format('DELETE FROM %I WHERE ctid = $1', c.t) USING r.ctid;
        END;
      END LOOP;
    WHEN OTHERS THEN
      RAISE NOTICE 'skip %.%: %', c.t, c.k, SQLERRM;
    END;
  END LOOP;
  RETURN moved;
END $$;
