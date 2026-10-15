#!/usr/bin/env bash
# Merge the duplicated demo records of the local stack: the @teratai.local
# accounts of scripts/seed/identity.sql vs the @teratai.sch.id accounts of
# seed-all-roles-teratai.sql, and the classes/subjects that the legacy seed
# (02_academic.sql) and the feature seed (30_academic_features.sql) both
# create. For each pair the id with the most references is kept; every
# reference in every database is repointed at it (scripts/dedupe/remap.sql),
# then the duplicate row is deleted. The kept account takes the demo e-mail
# and name, so the demo logins keep working. Idempotent.
#
#   ./scripts/dedupe-demo-data.sh
set -euo pipefail
cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.local.yml --env-file .env.local)
psql_db() { "${COMPOSE[@]}" exec -T postgres psql -U "${POSTGRES_USER:-teratai}" -d "$1" -v ON_ERROR_STOP=1 -q "${@:2}"; }

# dup|keep — users first, then classes, then subjects.
PAIRS=(
  "2aea557d-e1c3-4752-af17-fc264b157efc|22222222-2222-2222-2222-222222222222"  # Admin
  "8bf3324b-90cf-4a24-a28a-18612ddf0a65|33333333-3333-3333-3333-333333333333"  # Wali Santoso
  "fd7c9220-c89a-4787-8390-3a0ad007adf7|44444444-4444-4444-4444-444444444444"  # Budi Santoso
  "c5e28909-7b3b-4255-8d90-3355462a227d|55555555-5555-5555-5555-555555555555"  # Guru
  "8ada9ae9-ed62-4999-a5cb-359b2917b93a|66666666-6666-6666-6666-666666666666"  # Wali Kelas
  "65f065a8-ada2-46b1-96a6-d5edc9300675|77777777-7777-7777-7777-777777777777"  # Kepala Sekolah
  "add75135-947c-414c-ace6-79280178785f|88888888-8888-8888-8888-888888888888"  # Ketua Yayasan
  "f539b570-bae5-4062-882a-f73d3a4026d3|99999999-9999-9999-9999-999999999999"  # Pengawas
  "445d22e6-e0b0-42b5-9393-cdb2bd194c7f|c1000000-0000-0000-0000-000000000001"  # Kelas 7A -> Kelas VII A
  "ad5de00f-bd37-4bbe-bb76-00bfb4ded38a|5e300200-0000-0000-0000-000000000001"  # Kelas 7B -> Kelas VII B
  "962a77f0-6a0c-49f5-a40c-f788759eb2f3|5e300200-0000-0000-0000-000000000002"  # Kelas 8A -> Kelas VIII A
  "23de2a80-2805-466c-9da1-e7a594fa29a0|c2000000-0000-0000-0000-000000000001"  # Matematika
  "c025e7e2-ea05-4d9f-ba65-457c6e732054|c2000000-0000-0000-0000-000000000002"  # Bahasa Indonesia
)

echo "==> identity: drop the extra roles of the old admin, free the demo e-mails"
psql_db teratai_identity <<'SQL'
-- The old local admin held every operational role at once; each of those
-- roles has its own demo account, so the merged Admin keeps only "admin".
DELETE FROM user_school_roles
WHERE user_id = '22222222-2222-2222-2222-222222222222' AND role <> 'admin';
-- Duplicate superadmin role rows (school_id NULL escapes the unique key).
DELETE FROM user_school_roles a USING user_school_roles b
WHERE a.user_id = b.user_id AND a.role = b.role AND a.school_id IS NULL AND b.school_id IS NULL AND a.ctid > b.ctid;
SQL

for db in $("${COMPOSE[@]}" exec -T postgres psql -U "${POSTGRES_USER:-teratai}" -d postgres -Atc \
    "SELECT datname FROM pg_database WHERE datname LIKE 'teratai_%' ORDER BY 1"); do
  {
    cat scripts/dedupe/remap.sql
    for p in "${PAIRS[@]}"; do
      echo "SELECT '${p%%|*}' AS dup, pg_temp.remap('${p%%|*}', '${p##*|}') AS moved;"
    done
  } | psql_db "$db" -At | awk -v db="$db" -F'|' '$2 > 0 { printf "    %-22s %s  %s refs\n", db, $1, $2 }'
done

echo "==> identity: delete the merged accounts, give the kept ones the demo e-mail"
psql_db teratai_identity <<'SQL'
DELETE FROM users WHERE id IN (
  '2aea557d-e1c3-4752-af17-fc264b157efc', '8bf3324b-90cf-4a24-a28a-18612ddf0a65', 'fd7c9220-c89a-4787-8390-3a0ad007adf7',
  'c5e28909-7b3b-4255-8d90-3355462a227d', '8ada9ae9-ed62-4999-a5cb-359b2917b93a', '65f065a8-ada2-46b1-96a6-d5edc9300675',
  'add75135-947c-414c-ace6-79280178785f', 'f539b570-bae5-4062-882a-f73d3a4026d3');
UPDATE users u SET email = v.email, name = v.name, updated_at = now()
FROM (VALUES
  ('22222222-2222-2222-2222-222222222222'::uuid, 'admin@teratai.sch.id',     'Admin Teratai'),
  ('33333333-3333-3333-3333-333333333333'::uuid, 'orangtua@teratai.sch.id',  'Wali Santoso'),
  ('44444444-4444-4444-4444-444444444444'::uuid, 'siswa@teratai.sch.id',     'Budi Santoso'),
  ('55555555-5555-5555-5555-555555555555'::uuid, 'guru@teratai.sch.id',      'Guru Teratai'),
  ('66666666-6666-6666-6666-666666666666'::uuid, 'walikelas@teratai.sch.id', 'Wali Kelas Teratai'),
  ('77777777-7777-7777-7777-777777777777'::uuid, 'kepsek@teratai.sch.id',    'Kepala Sekolah Teratai'),
  ('88888888-8888-8888-8888-888888888888'::uuid, 'yayasan@teratai.sch.id',   'Ketua Yayasan Teratai'),
  ('99999999-9999-9999-9999-999999999999'::uuid, 'pengawas@teratai.sch.id',  'Pengawas Teratai')
) AS v(id, email, name)
WHERE u.id = v.id;
SQL

echo "==> academic: delete the merged classes and subjects"
psql_db teratai_academic <<'SQL'
DELETE FROM classrooms WHERE id IN ('445d22e6-e0b0-42b5-9393-cdb2bd194c7f', 'ad5de00f-bd37-4bbe-bb76-00bfb4ded38a', '962a77f0-6a0c-49f5-a40c-f788759eb2f3');
DELETE FROM subjects WHERE id IN ('23de2a80-2805-466c-9da1-e7a594fa29a0', 'c025e7e2-ea05-4d9f-ba65-457c6e732054');
-- Same naming as the other classes.
UPDATE classrooms SET name = 'Kelas IX A' WHERE id = '9ee5def5-e06d-443f-9a98-85846de9695d' AND name = 'Kelas 9A';
SQL
echo "done"
