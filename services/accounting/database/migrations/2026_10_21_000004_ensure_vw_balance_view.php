<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

/**
 * The Saldo (Balance) page reads vw_balance, which only ViewSeeder created.
 * A schema that was migrated but not seeded (or seeded before the view
 * existed) answered /balance with a 500 (QA round 2). Create it with the
 * schema; ViewSeeder's CREATE OR REPLACE stays compatible.
 */
return new class extends Migration
{
    public function up(): void
    {
        if (! Schema::hasTable('mst_saldo_news') || ! Schema::hasTable('mst_account') || ! Schema::hasTable('mst_division')) {
            return;
        }
        DB::statement('CREATE OR REPLACE VIEW vw_balance AS
            SELECT DISTINCT msn.acc_code, msn.div_code, ma.acc_name, md.div_name
            FROM mst_saldo_news msn
            JOIN mst_account ma ON msn.acc_code = ma.acc_code
            JOIN mst_division md ON msn.div_code = md.div_code');
    }

    public function down(): void
    {
        // Dropped so rolling back further (which drops its tables) works;
        // migrating up again, or ViewSeeder, recreates it.
        DB::statement('DROP VIEW IF EXISTS vw_balance');
    }
};
