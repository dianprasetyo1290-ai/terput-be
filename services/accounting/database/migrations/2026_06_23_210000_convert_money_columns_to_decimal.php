<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        $moneyColumns = [
            'trans_head' => ['debet', 'credit'],
            'this_head' => ['debet', 'credit'],
            'trans_detail' => ['debet', 'credit', 'curr_debet', 'curr_credit'],
            'this_detail' => ['debet', 'credit', 'curr_debet', 'curr_credit'],
            'mst_saldo_news' => ['debet', 'credit'],
        ];

        foreach ($moneyColumns as $table => $columns) {
            foreach ($columns as $column) {
                DB::statement("ALTER TABLE {$table} ALTER COLUMN {$column} TYPE NUMERIC(18, 2) USING ROUND({$column}::numeric, 2)");
            }
        }

        foreach (['trans_detail', 'this_detail'] as $table) {
            DB::statement("ALTER TABLE {$table} ALTER COLUMN curr_rate TYPE NUMERIC(20, 8) USING ROUND(curr_rate::numeric, 8)");
        }

        $budgetColumns = [];
        foreach (['act', 'lst', 'fut'] as $prefix) {
            for ($period = 1; $period <= 13; $period++) {
                $budgetColumns[] = sprintf('%s_%02d', $prefix, $period);
            }
        }

        foreach ($budgetColumns as $column) {
            DB::statement("ALTER TABLE mst_budget ALTER COLUMN {$column} TYPE NUMERIC(18, 2) USING ROUND({$column}::numeric, 2)");
        }
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        $moneyColumns = [
            'trans_head' => ['debet', 'credit'],
            'this_head' => ['debet', 'credit'],
            'trans_detail' => ['debet', 'credit', 'curr_debet', 'curr_credit'],
            'this_detail' => ['debet', 'credit', 'curr_debet', 'curr_credit'],
            'mst_saldo_news' => ['debet', 'credit'],
        ];

        foreach ($moneyColumns as $table => $columns) {
            foreach ($columns as $column) {
                DB::statement("ALTER TABLE {$table} ALTER COLUMN {$column} TYPE DOUBLE PRECISION USING {$column}::double precision");
            }
        }

        foreach (['trans_detail', 'this_detail'] as $table) {
            DB::statement("ALTER TABLE {$table} ALTER COLUMN curr_rate TYPE DOUBLE PRECISION USING curr_rate::double precision");
        }

        $budgetColumns = [];
        foreach (['act', 'lst', 'fut'] as $prefix) {
            for ($period = 1; $period <= 13; $period++) {
                $budgetColumns[] = sprintf('%s_%02d', $prefix, $period);
            }
        }

        foreach ($budgetColumns as $column) {
            DB::statement("ALTER TABLE mst_budget ALTER COLUMN {$column} TYPE REAL USING {$column}::real");
        }
    }
};
