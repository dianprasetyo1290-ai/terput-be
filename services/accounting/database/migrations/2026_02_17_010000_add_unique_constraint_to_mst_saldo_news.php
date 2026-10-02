<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        // Remove duplicate records before adding unique constraint
        // Keep the record with the highest id for each combination
        DB::statement("
            DELETE FROM mst_saldo_news
            WHERE id NOT IN (
                SELECT MAX(id)
                FROM mst_saldo_news
                GROUP BY acc_code, div_code, periode, tahun
            )
        ");

        Schema::table('mst_saldo_news', function (Blueprint $table) {
            $table->unique(
                ['acc_code', 'div_code', 'periode', 'tahun'],
                'mst_saldo_news_unique'
            );
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::table('mst_saldo_news', function (Blueprint $table) {
            $table->dropUnique('mst_saldo_news_unique');
        });
    }
};
