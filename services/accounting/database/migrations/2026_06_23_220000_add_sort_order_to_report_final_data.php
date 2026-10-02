<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::table('report_final_data', function (Blueprint $table) {
            $table->integer('sort_order')->default(0)->after('report_row_head_id');
        });

        $groups = DB::table('report_final_data')
            ->select('report_final_id')
            ->distinct()
            ->pluck('report_final_id');

        foreach ($groups as $reportFinalId) {
            $rows = DB::table('report_final_data')
                ->where('report_final_id', $reportFinalId)
                ->orderBy('id')
                ->get(['id']);

            foreach ($rows as $index => $row) {
                DB::table('report_final_data')
                    ->where('id', $row->id)
                    ->update(['sort_order' => $index + 1]);
            }
        }
    }

    public function down(): void
    {
        Schema::table('report_final_data', function (Blueprint $table) {
            $table->dropColumn('sort_order');
        });
    }
};
