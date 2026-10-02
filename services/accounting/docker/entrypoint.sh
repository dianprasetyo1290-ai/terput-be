#!/bin/sh
set -e

# Fix permissions
chown -R www-data:www-data /var/www/storage /var/www/bootstrap/cache 2>/dev/null || true
chmod -R 775 /var/www/storage /var/www/bootstrap/cache 2>/dev/null || true

# Only the SHARED infrastructure tables live in `public` (sessions, cache,
# jobs, telescope) -- so only those migration files run here. Business tables
# live in one schema per school and are created by
#   php artisan accounting:provision <school_id>
# run deliberately, once per school. Provisioning is NOT automatic on first
# SSO: it seeds baseline data, which is not something to do inside a web
# request. /sso answers 403 "Akuntansi belum disiapkan" until the schema
# exists. This entrypoint cannot know which schools exist, so it must never
# migrate a tenant schema blindly.
#
# Side effect, on purpose: the first file also creates (empty) mst_user and
# password_reset_tokens in public. Harmless -- `public` is LAST in the runtime
# search_path, so a tenant's own mst_user always wins.
#
# Do NOT change this to a plain `php artisan migrate`: with search_path=public
# that would create all ~40 business tables in public, where no tenant can see
# them and where they would shadow nothing but confuse everything.
echo "[Sekoci Accounting] Migrating shared (public) infrastructure tables..."
php artisan migrate --force \
  --path=database/migrations/0001_01_01_000000_create_users_table.php \
  --path=database/migrations/0001_01_01_000001_create_cache_table.php \
  --path=database/migrations/0001_01_01_000002_create_jobs_table.php \
  --path=database/migrations/2024_07_03_151447_create_telescope_entries_table.php \
  || echo "[Sekoci Accounting] WARNING: public migration failed (DB unreachable?) -- sessions/cache may not work"

# DELIBERATELY NO `php artisan db:seed` HERE.
# Six of the baseline seeders TRUNCATE their tables (ParameterSeeder,
# PeriodSeeder, ModulesSeeder, ComCodesSeeder, RoleAndPermissionSeeder,
# RowDetailsSeeder). This script runs on every container start, so seeding here
# meant: every restart / redeploy wiped live accounting data. Baseline seeding
# now happens exactly once per school, inside accounting:provision, and only
# while that school's schema is still empty.

echo "[Sekoci Accounting] Starting services via supervisord..."
exec supervisord -c /etc/supervisor/supervisord.conf
