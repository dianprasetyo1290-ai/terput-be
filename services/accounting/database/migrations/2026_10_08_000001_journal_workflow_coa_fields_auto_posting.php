<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

/**
 * PRD TP-SMS v2.1 menu 32-38 (Grup Accounting):
 *  - trans_head/this_head: journal workflow draft -> pending -> posted -> reversed
 *    (FR-34-005, BR-ACC-01/02) plus source/reference columns for auto journals
 *    and GL drilldown (FR-34-001..003, FR-35-002). Both tables get the SAME
 *    columns because End of Month copies trans_head rows verbatim into this_head.
 *  - mst_account: parent, normal balance, active flag, inter-unit and kas/bank flags
 *    (FR-33-001, BR-ACC-04 elimination).
 *  - acc_posting_maps / acc_auto_postings: billing -> journal account mapping
 *    and the daily auto-posting log (BR-ACC-05/06, FR-32-002 "posting gagal").
 *
 * Raw "IF NOT EXISTS" SQL, unqualified, so it lands in whatever tenant schema
 * DB_SEARCH_PATH points at and is safe to re-run.
 */
return new class extends Migration
{
    public function up(): void
    {
        foreach (['trans_head', 'this_head'] as $t) {
            // this_head only ever receives posted journals (End of Month requires
            // every open journal to be posted; prior-year adjustments insert
            // finalized rows), so its default is 'posted'.
            $default = $t === 'this_head' ? 'posted' : 'draft';
            DB::statement("ALTER TABLE $t ADD COLUMN IF NOT EXISTS status varchar(10) NOT NULL DEFAULT '$default'");
            DB::statement("ALTER TABLE $t ADD COLUMN IF NOT EXISTS source varchar(20) NOT NULL DEFAULT 'manual'");
            DB::statement("ALTER TABLE $t ADD COLUMN IF NOT EXISTS reference varchar(100)");
            DB::statement("ALTER TABLE $t ADD COLUMN IF NOT EXISTS reversal_of varchar(15)");
            DB::statement("ALTER TABLE $t ADD COLUMN IF NOT EXISTS status_by varchar(255)");
            DB::statement("ALTER TABLE $t ADD COLUMN IF NOT EXISTS status_at timestamp");
        }
        // Everything that existed before the workflow was already live under
        // Exco's rules (counted in balances, transferred at End of Month), so
        // it is "posted", not a draft waiting for approval. Only runs once:
        // the migrations table guards re-runs, and new rows default to draft.
        DB::statement("UPDATE trans_head SET status = 'posted' WHERE status = 'draft'");

        DB::statement('ALTER TABLE mst_account ADD COLUMN IF NOT EXISTS parent_code varchar(15)');
        DB::statement('ALTER TABLE mst_account ADD COLUMN IF NOT EXISTS normal_balance char(1)');
        DB::statement('ALTER TABLE mst_account ADD COLUMN IF NOT EXISTS is_active boolean NOT NULL DEFAULT true');
        DB::statement('ALTER TABLE mst_account ADD COLUMN IF NOT EXISTS is_interunit boolean NOT NULL DEFAULT false');
        // Kas & bank flag drives the dashboard "Kas & Bank" figure and the cash
        // flow statement. Backfilled once from the name; editable in COA after.
        DB::statement('ALTER TABLE mst_account ADD COLUMN IF NOT EXISTS is_cash boolean NOT NULL DEFAULT false');
        DB::statement("UPDATE mst_account SET is_cash = true WHERE acc_group = 1 AND acc_name ~* '^(kas|bank|cash)'");
        // Groups 1 Asset, 5 Cost of Sales, 6 Expense, 8 Other Expense are debit-normal.
        DB::statement("UPDATE mst_account SET normal_balance = CASE WHEN acc_group IN (1,5,6,8) THEN 'D' ELSE 'K' END WHERE normal_balance IS NULL");

        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_posting_maps (
                map_key    varchar(60) PRIMARY KEY,
                acc_code   varchar(15) NOT NULL,
                updated_by varchar(255),
                updated_at timestamp NOT NULL DEFAULT now()
            )
        SQL);

        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_auto_postings (
                id           bigserial PRIMARY KEY,
                posting_date date NOT NULL,
                kind         varchar(20) NOT NULL,
                status       varchar(10) NOT NULL,
                trans_no     varchar(15),
                amount       numeric(20,2) NOT NULL DEFAULT 0,
                source_ids   jsonb NOT NULL DEFAULT '[]',
                message      text,
                created_at   timestamp NOT NULL DEFAULT now(),
                updated_at   timestamp NOT NULL DEFAULT now(),
                UNIQUE (posting_date, kind)
            )
        SQL);
    }

    public function down(): void
    {
        DB::statement('DROP TABLE IF EXISTS acc_auto_postings');
        DB::statement('DROP TABLE IF EXISTS acc_posting_maps');
        foreach (['parent_code', 'normal_balance', 'is_active', 'is_interunit', 'is_cash'] as $c) {
            DB::statement("ALTER TABLE mst_account DROP COLUMN IF EXISTS $c");
        }
        foreach (['trans_head', 'this_head'] as $t) {
            foreach (['status', 'source', 'reference', 'reversal_of', 'status_by', 'status_at'] as $c) {
                DB::statement("ALTER TABLE $t DROP COLUMN IF EXISTS $c");
            }
        }
    }
};
