<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

/**
 * QA BUG-141 / FR-33-002, AC-33-01: a school COA template that is copied, and
 * then adjusted, per unit.
 *  - mst_account.unit_scope: NULL = shared by every unit (the existing COA);
 *    a div_code = the account belongs to that unit only, and a journal line
 *    of another unit may not post to it;
 *  - acc_coa_templates / _lines: named snapshots of a COA.
 */
return new class extends Migration
{
    public function up(): void
    {
        DB::statement('ALTER TABLE mst_account ADD COLUMN IF NOT EXISTS unit_scope varchar(5)');
        DB::statement('CREATE INDEX IF NOT EXISTS mst_account_unit_scope_idx ON mst_account (unit_scope)');

        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_coa_templates (
                id          bigserial PRIMARY KEY,
                name        varchar(100) NOT NULL,
                description varchar(250),
                created_by  varchar(255),
                created_at  timestamp NOT NULL DEFAULT now(),
                updated_at  timestamp NOT NULL DEFAULT now()
            )
        SQL);
        DB::statement('CREATE UNIQUE INDEX IF NOT EXISTS acc_coa_templates_name_uq ON acc_coa_templates (lower(name))');

        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_coa_template_lines (
                id             bigserial PRIMARY KEY,
                template_id    bigint NOT NULL REFERENCES acc_coa_templates(id) ON DELETE CASCADE,
                acc_code       varchar(15) NOT NULL,
                acc_name       varchar(50) NOT NULL,
                acc_group      bigint NOT NULL,
                sub_acc_group  bigint,
                parent_code    varchar(15),
                normal_balance char(1),
                is_cash        boolean NOT NULL DEFAULT false,
                is_interunit   boolean NOT NULL DEFAULT false,
                is_control     boolean NOT NULL DEFAULT false,
                UNIQUE (template_id, acc_code)
            )
        SQL);
    }

    public function down(): void
    {
        DB::statement('DROP TABLE IF EXISTS acc_coa_template_lines');
        DB::statement('DROP TABLE IF EXISTS acc_coa_templates');
        DB::statement('DROP INDEX IF EXISTS mst_account_unit_scope_idx');
        DB::statement('ALTER TABLE mst_account DROP COLUMN IF EXISTS unit_scope');
    }
};
