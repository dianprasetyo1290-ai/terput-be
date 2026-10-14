<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

/**
 * QA BUG-146 / BR-ACC-06: mst_account.is_control marks a control account (e.g.
 * 1201 Piutang Siswa) whose sub-ledger lives in SEKOCI billing. Only automatic
 * postings may touch it; a manual journal line is rejected.
 *
 * QA BUG-060/178: acc_audit_log, a small append-only trail for accounting
 * master/workflow events (COA deactivation, journal approval/reversal,
 * imports, period close).
 *
 * Raw IF NOT EXISTS SQL like the other tenant migrations: lands in whatever
 * schema DB_SEARCH_PATH points at and is safe to re-run.
 */
return new class extends Migration
{
    public function up(): void
    {
        DB::statement('ALTER TABLE mst_account ADD COLUMN IF NOT EXISTS is_control boolean NOT NULL DEFAULT false');

        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_audit_log (
                id         bigserial PRIMARY KEY,
                event      varchar(40) NOT NULL,
                subject    varchar(100),
                user_id    varchar(255),
                detail     jsonb NOT NULL DEFAULT '{}',
                created_at timestamp NOT NULL DEFAULT now()
            )
        SQL);
        DB::statement('CREATE INDEX IF NOT EXISTS acc_audit_log_event_idx ON acc_audit_log (event, created_at)');
    }

    public function down(): void
    {
        DB::statement('DROP TABLE IF EXISTS acc_audit_log');
        DB::statement('ALTER TABLE mst_account DROP COLUMN IF EXISTS is_control');
    }
};
