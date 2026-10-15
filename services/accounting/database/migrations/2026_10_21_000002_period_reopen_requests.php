<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

/**
 * QA BUG-178: reopening a closed period goes through a request (with a
 * mandatory reason) that a different user holding editperiod_reopen
 * (FinanceManager / SuperAdmin) approves or rejects. At most one request is
 * pending at a time. Every step is also written to acc_audit_log.
 */
return new class extends Migration
{
    public function up(): void
    {
        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_period_reopen_requests (
                id             bigserial PRIMARY KEY,
                year           int NOT NULL,
                period         smallint NOT NULL,
                reason         text NOT NULL,
                status         varchar(10) NOT NULL DEFAULT 'pending',
                requested_by   varchar(255) NOT NULL,
                requested_at   timestamp NOT NULL DEFAULT now(),
                decided_by     varchar(255),
                decided_at     timestamp,
                decision_note  text,
                moved_journals int
            )
        SQL);
        DB::statement("CREATE UNIQUE INDEX IF NOT EXISTS acc_period_reopen_one_pending ON acc_period_reopen_requests ((true)) WHERE status = 'pending'");
    }

    public function down(): void
    {
        DB::statement('DROP TABLE IF EXISTS acc_period_reopen_requests');
    }
};
