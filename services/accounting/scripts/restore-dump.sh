#!/usr/bin/env bash
# Load backup_exco.dump (legacy "exco" data, 41 tables) into ONE tenant schema.
#
# Why this is not just `pg_restore`:
#   * the dump is custom-format written by pg_dump 18 (archive v1.16); a PG16
#     pg_restore refuses it with "unsupported version (1.16)", and prod is
#     PG 16.15. So the archive is converted to plain SQL by a PG18 *client*
#     (docker postgres:18) and then fed to the PG16 server with psql. The data
#     itself came from a 16.1 server, so nothing in it needs PG17+.
#   * the dump's objects are all qualified `public.<name>`, and here `public`
#     is shared infrastructure. Rewriting that prefix in the generated SQL is
#     the least fragile redirection available: pg_dump >= 10 sets an empty
#     search_path and qualifies every single reference, so a textual rewrite is
#     exhaustive -- but it is done with awk, not sed, so that COPY data blocks
#     (where a literal "public." could legitimately appear inside a text
#     column) are left untouched. Function bodies ARE rewritten, which is what
#     we want: gen_mst_saldo() hardcodes public.mst_saldo_news.
#
# Everything is loaded in ONE transaction: a failure half-way leaves the target
# schema exactly as it was, so re-running after an error is safe.
#
# NEVER point this at prod without a fresh backup of the target database.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

DUMP="$HERE/../backup_exco.dump"
SCHEMA="tenant_legacy_exco"   # not claimed by any school yet -- see DESIGN.md ADDENDUM 1
DB="skci_accounting"
HOST="127.0.0.1"
PORT="5432"
USER="sekoci"
FORCE=0
KEEP_SQL=0
CLIENT_IMAGE="${PGCLIENT_IMAGE:-postgres:18}"
NET="${PGCLIENT_NETWORK:-host}"   # docker network for the client container

usage() {
  cat <<USAGE
usage: $0 [--schema NAME] [--db NAME] [--host H] [--port P] [--user U]
          [--dump PATH] [--force] [--keep-sql]

  --force      target schema may already contain tables: DROP SCHEMA ... CASCADE
               first (inside the same transaction). Without it, a non-empty
               target schema is a hard error -- data is never overwritten silently.
  --keep-sql   keep the generated .sql for inspection.

  Password comes from \$PGPASSWORD.
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --schema) SCHEMA="$2"; shift 2 ;;
    --db) DB="$2"; shift 2 ;;
    --host) HOST="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --user) USER="$2"; shift 2 ;;
    --dump) DUMP="$2"; shift 2 ;;
    --force) FORCE=1; shift ;;
    --keep-sql) KEEP_SQL=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "argumen tak dikenal: $1" >&2; usage; exit 2 ;;
  esac
done

# Same rule as App\Support\Tenancy::schema() -- the name goes into SQL unescaped.
if ! [[ "$SCHEMA" =~ ^[a-z0-9_]{1,63}$ ]]; then
  echo "nama schema tidak valid: $SCHEMA" >&2; exit 2
fi
if [ "$SCHEMA" = "public" ]; then
  echo "menolak restore ke public: itu schema infrastruktur bersama" >&2; exit 2
fi
[ -f "$DUMP" ] || { echo "dump tidak ditemukan: $DUMP" >&2; exit 2; }

WORK="$(mktemp -d)"
trap '[ "$KEEP_SQL" = 1 ] || rm -rf "$WORK"' EXIT

psql_() {
  docker run --rm -i --network "$NET" \
    -e PGPASSWORD="${PGPASSWORD:-}" \
    -v "$WORK:/work" \
    "$CLIENT_IMAGE" psql -h "$HOST" -p "$PORT" -U "$USER" -d "$DB" "$@"
}

echo "==> cek schema tujuan ($SCHEMA di $DB@$HOST:$PORT)"
EXISTING="$(psql_ -tAc "select count(*) from information_schema.tables where table_schema='$SCHEMA' and table_type='BASE TABLE'")"
if [ "$EXISTING" -gt 0 ]; then
  if [ "$FORCE" != 1 ]; then
    echo "TOLAK: schema $SCHEMA sudah punya $EXISTING tabel. Pakai --force kalau memang mau menggantinya." >&2
    exit 1
  fi
  echo "    --force: $EXISTING tabel yang ada akan di-DROP CASCADE"
fi

echo "==> konversi dump custom-format (v1.16) -> SQL polos pakai $CLIENT_IMAGE"
cp "$DUMP" "$WORK/in.dump"
docker run --rm -v "$WORK:/work" "$CLIENT_IMAGE" \
  pg_restore --no-owner --no-privileges --no-comments -f /work/plain.sql /work/in.dump

echo "==> arahkan objek public.* -> $SCHEMA.*"
awk -v schema="$SCHEMA" '
  # COPY ... FROM stdin; .. \.  = raw data, must not be rewritten.
  !in_copy && /^COPY .* FROM stdin;$/ { in_copy = 1; gsub(/public\./, schema "."); print; next }
  in_copy { if ($0 == "\\.") in_copy = 0; print; next }
  # the dump recreates `public` itself; we create the tenant schema ourselves.
  /^(CREATE|DROP) SCHEMA (IF (NOT )?EXISTS )?public;?$/ { next }
  /^(COMMENT ON|ALTER) SCHEMA public/ { next }
  # pg_dump 18 emits GUCs that PG16 does not know; they are cosmetic.
  /^SET transaction_timeout/ { next }
  # extensions stay where they are (they are server-wide, not tenant data).
  /EXTENSION/ { print; next }
  { gsub(/public\./, schema "."); print }
' "$WORK/plain.sql" > "$WORK/tenant.sql"

{
  echo "\\set ON_ERROR_STOP on"
  [ "$FORCE" = 1 ] && echo "DROP SCHEMA IF EXISTS \"$SCHEMA\" CASCADE;"
  echo "CREATE SCHEMA IF NOT EXISTS \"$SCHEMA\";"
  cat "$WORK/tenant.sql"
  # These belong to public only (mirrors App\Support\Tenancy::PUBLIC_TABLES):
  # a tenant-local `sessions` would break login, because the session is read
  # before the tenant -- and therefore the search_path -- is known.
  for t in sessions cache cache_locks jobs job_batches failed_jobs \
           telescope_entries telescope_entries_tags telescope_monitoring; do
    echo "DROP TABLE IF EXISTS \"$SCHEMA\".\"$t\" CASCADE;"
  done
} > "$WORK/load.sql"

echo "==> load (single transaction)"
psql_ -v ON_ERROR_STOP=1 --single-transaction -q -o /dev/null -f /work/load.sql

echo "==> hasil"
echo "tabel: $(psql_ -tAc "select count(*) from information_schema.tables where table_schema='$SCHEMA' and table_type='BASE TABLE'")"
for t in $(psql_ -tAc "select tablename from pg_tables where schemaname='$SCHEMA'
             order by pg_total_relation_size(quote_ident(schemaname)||'.'||quote_ident(tablename)) desc limit 5"); do
  echo "  $t = $(psql_ -tAc "select count(*) from \"$SCHEMA\".\"$t\"") baris"
done
[ "$KEEP_SQL" = 1 ] && echo "SQL ada di $WORK"
echo "selesai: $SCHEMA"
