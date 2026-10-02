<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;
use Illuminate\Support\Facades\DB;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        // Tambah kolom sort_order
        Schema::table('report_row_details', function (Blueprint $table) {
            $table->integer('sort_order')->default(0)->after('report_row_head_id');
        });

        // Populate sort_order untuk data yang sudah ada
        // Diurutkan berdasarkan ID per report_row_head_id
        $reportHeads = DB::table('report_row_details')
            ->select('report_row_head_id')
            ->distinct()
            ->get();

        foreach ($reportHeads as $head) {
            $details = DB::table('report_row_details')
                ->where('report_row_head_id', $head->report_row_head_id)
                ->orderBy('id', 'asc')
                ->get();

            $order = 1;
            foreach ($details as $detail) {
                DB::table('report_row_details')
                    ->where('id', $detail->id)
                    ->update(['sort_order' => $order]);
                $order++;
            }
        }
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::table('report_row_details', function (Blueprint $table) {
            $table->dropColumn('sort_order');
        });
    }
};
