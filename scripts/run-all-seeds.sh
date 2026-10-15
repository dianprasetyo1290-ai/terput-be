#!/usr/bin/env bash
# Demo seed data for Teratai on shared-postgres. Safe on a live server:
#   - every file only adds rows (fixed UUIDs, ON CONFLICT DO NOTHING);
#   - each file runs in one transaction with ON_ERROR_STOP;
#   - public.seed_ledger in each database records the files already applied,
#     so a file is never run twice (some legacy files are not idempotent).
#
#   scripts/run-all-seeds.sh                 run every file not yet applied
#   BASELINE_LEGACY=1 scripts/run-all-seeds.sh
#       first run on a server that already got the legacy files (01–17) by
#       hand: mark them applied without running them, then run the rest.
#   PG_CONTAINER / ACCOUNTING_CONTAINER override the container names.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
PG="${PG_CONTAINER:-shared-postgres}"
ACC="${ACCOUNTING_CONTAINER:-teratai-accounting-1}"
BASELINE_LEGACY="${BASELINE_LEGACY:-0}"

psql_db() { docker exec -i "$PG" psql -U postgres -d "$1" -v ON_ERROR_STOP=1 -q "${@:2}"; }

seed() { # seed <database> <file> [legacy]
  local db="$1" file="$2" legacy="${3:-}"
  psql_db "$db" -c "CREATE TABLE IF NOT EXISTS public.seed_ledger (file text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())" >/dev/null
  if [ "$(psql_db "$db" -Atc "SELECT 1 FROM public.seed_ledger WHERE file = '$file'")" = "1" ]; then
    echo "    skip $db $file (already applied)"
    return
  fi
  if [ -n "$legacy" ] && [ "$BASELINE_LEGACY" = "1" ]; then
    echo "    baseline $db $file (marked applied, not run)"
  else
    echo "--> $db $file"
    psql_db "$db" < "$DIR/seed/$file"
  fi
  psql_db "$db" -c "INSERT INTO public.seed_ledger (file) VALUES ('$file') ON CONFLICT DO NOTHING" >/dev/null
}

echo "=== Seeding Teratai databases on $PG ==="

# Legacy files (applied by hand on the server before the ledger existed).
seed teratai_identity     01_identity_features.sql   legacy
seed teratai_academic     02_academic.sql            legacy
seed teratai_billing      03_billing.sql             legacy
seed teratai_ops          04_ops.sql                 legacy
seed teratai_studentlife  05_studentlife.sql         legacy
seed teratai_quality      06_quality.sql             legacy
seed teratai_insight      07_insight.sql             legacy
seed teratai_academic     08_academic_schedules.sql  legacy
seed teratai_attendance   09_attendance.sql          legacy
seed teratai_exam         10_exam.sql                legacy
seed teratai_evaluation   11_evaluation.sql          legacy
seed teratai_report       12_report.sql              legacy
seed teratai_hr           14_hr.sql                  legacy
seed teratai_notification 15_notification.sql        legacy
seed teratai_core         16_core.sql                legacy
seed teratai_insight      17_insight_extra.sql       legacy

# Feature seeds (additive, idempotent). identity first: core/admission/
# quality rows reference identity users and core approval ids.
seed teratai_identity     20_identity_features.sql
seed teratai_core         21_core_features.sql
seed teratai_notification 22_notification_features.sql
seed teratai_admission    23_admission_features.sql
seed teratai_insight      24_insight_features.sql
seed teratai_quality      25_quality_features.sql
seed teratai_academic     30_academic_features.sql
seed teratai_attendance   31_attendance_features.sql
seed teratai_exam         32_exam_features.sql
seed teratai_evaluation   33_evaluation_features.sql
seed teratai_report       34_report_features.sql
seed teratai_studentlife  35_studentlife_features.sql
seed teratai_hr           50_hr_features.sql
seed teratai_ops          60_ops_features.sql

echo "=== ALL SEED DATA SUCCESSFULLY APPLIED ==="
