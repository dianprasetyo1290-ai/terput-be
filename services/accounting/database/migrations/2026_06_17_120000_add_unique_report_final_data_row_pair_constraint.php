<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        DB::statement('
            DELETE FROM report_final_data a
            USING report_final_data b
            WHERE a.id > b.id
              AND a.report_final_id = b.report_final_id
              AND a.report_row_head_id = b.report_row_head_id
        ');

        Schema::table('report_final_data', function (Blueprint $table) {
            $table->unique(
                ['report_final_id', 'report_row_head_id'],
                'report_final_data_report_final_id_report_row_head_id_unique'
            );
        });
    }

    public function down(): void
    {
        Schema::table('report_final_data', function (Blueprint $table) {
            $table->dropUnique('report_final_data_report_final_id_report_row_head_id_unique');
        });
    }
};
