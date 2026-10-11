#!/usr/bin/env bash
# Seed the local Docker stack with Teratai demo data, then point the admission
# service at the seeded school so the public PPDB pages resolve their tenant.
#
#   ./scripts/local-seed.sh
#
# Requires the local stack to be up:
#   docker compose -f docker-compose.yml -f docker-compose.local.yml \
#     --env-file .env.local up -d --build
set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.local.yml --env-file .env.local)
SCHOOL_ID="11111111-1111-1111-1111-111111111111"

echo "==> waiting for postgres"
for _ in $(seq 1 60); do
  if "${COMPOSE[@]}" exec -T postgres pg_isready -U teratai >/dev/null 2>&1; then break; fi
  sleep 2
done

echo "==> seeding identity"
"${COMPOSE[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -U teratai -d teratai_identity < scripts/seed/identity.sql

echo "==> seeding admission"
"${COMPOSE[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -U teratai -d teratai_admission < scripts/seed/admission.sql

echo "==> pinning ADMISSION_SCHOOL_ID=$SCHOOL_ID in .env.local"
if grep -q '^ADMISSION_SCHOOL_ID=' .env.local; then
  # macOS/BSD sed
  sed -i '' "s|^ADMISSION_SCHOOL_ID=.*|ADMISSION_SCHOOL_ID=$SCHOOL_ID|" .env.local
else
  echo "ADMISSION_SCHOOL_ID=$SCHOOL_ID" >> .env.local
fi

echo "==> recreating admission so it picks up the tenant"
"${COMPOSE[@]}" up -d admission

cat <<EOF

Seeded. Login accounts (password: Sekoci@2026):
  superadmin : superadmin@sekoci.local
  admin PPDB : admin@teratai.local
  wali/siswa : wali@teratai.local  (linked to Budi Santoso)
  siswa      : siswa@teratai.local

Public PPDB:  http://localhost/ppdb
Admin PPDB :  http://localhost/ppdb/admin
EOF
