<?php

use App\Support\AccountingRoles;
use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

/**
 * QA BUG-144 / FR-34-006: journal templates and scheduled recurring journals.
 *  - acc_journal_templates / _lines: a reusable, balanced set of lines;
 *  - acc_recurring_journals: "create a draft from template T on day D of
 *    every month" (next_run is the next date to book);
 *  - acc_recurring_runs: one row per (schedule, run date) -- the idempotency
 *    key of the daily job, and the visible log of what it did.
 * A recurring journal is only ever created as a DRAFT and follows the normal
 * maker-checker workflow (submit -> approve).
 *
 * Also installs the journal_template / period_reopen permissions (QA
 * BUG-144/178) on already-provisioned schools, the same way
 * 2026_10_14_000001 did.
 */
return new class extends Migration
{
    public function up(): void
    {
        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_journal_templates (
                id          bigserial PRIMARY KEY,
                name        varchar(100) NOT NULL,
                trans_code  varchar(5) NOT NULL,
                description varchar(250),
                is_active   boolean NOT NULL DEFAULT true,
                created_by  varchar(255),
                created_at  timestamp NOT NULL DEFAULT now(),
                updated_at  timestamp NOT NULL DEFAULT now()
            )
        SQL);
        DB::statement('CREATE UNIQUE INDEX IF NOT EXISTS acc_journal_templates_name_uq ON acc_journal_templates (lower(name))');

        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_journal_template_lines (
                id          bigserial PRIMARY KEY,
                template_id bigint NOT NULL REFERENCES acc_journal_templates(id) ON DELETE CASCADE,
                line_no     int NOT NULL,
                acc_code    varchar(15) NOT NULL,
                div_code    varchar(5) NOT NULL,
                description varchar(250) NOT NULL,
                debit       numeric(20,2) NOT NULL DEFAULT 0,
                credit      numeric(20,2) NOT NULL DEFAULT 0
            )
        SQL);
        DB::statement('CREATE INDEX IF NOT EXISTS acc_journal_template_lines_tpl_idx ON acc_journal_template_lines (template_id, line_no)');

        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_recurring_journals (
                id            bigserial PRIMARY KEY,
                template_id   bigint NOT NULL REFERENCES acc_journal_templates(id) ON DELETE RESTRICT,
                day_of_month  smallint NOT NULL CHECK (day_of_month BETWEEN 1 AND 28),
                start_date    date NOT NULL,
                end_date      date,
                next_run      date NOT NULL,
                is_active     boolean NOT NULL DEFAULT true,
                created_by    varchar(255) NOT NULL,
                last_run_at   timestamp,
                last_trans_no varchar(15),
                last_error    text,
                created_at    timestamp NOT NULL DEFAULT now(),
                updated_at    timestamp NOT NULL DEFAULT now()
            )
        SQL);
        DB::statement('CREATE INDEX IF NOT EXISTS acc_recurring_journals_due_idx ON acc_recurring_journals (is_active, next_run)');

        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_recurring_runs (
                recurring_id bigint NOT NULL REFERENCES acc_recurring_journals(id) ON DELETE CASCADE,
                run_date     date NOT NULL,
                status       varchar(10) NOT NULL,
                trans_no     varchar(15),
                message      text,
                created_at   timestamp NOT NULL DEFAULT now(),
                updated_at   timestamp NOT NULL DEFAULT now(),
                PRIMARY KEY (recurring_id, run_date)
            )
        SQL);

        if (Schema::hasTable('permissions') && DB::table('permissions')->exists()) {
            AccountingRoles::install(additive: true);
        }
    }

    public function down(): void
    {
        DB::statement('DROP TABLE IF EXISTS acc_recurring_runs');
        DB::statement('DROP TABLE IF EXISTS acc_recurring_journals');
        DB::statement('DROP TABLE IF EXISTS acc_journal_template_lines');
        DB::statement('DROP TABLE IF EXISTS acc_journal_templates');
    }
};
