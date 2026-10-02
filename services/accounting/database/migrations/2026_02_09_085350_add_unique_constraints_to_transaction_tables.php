<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        // Add unique constraint to trans_head.trans_no
        Schema::table('trans_head', function (Blueprint $table) {
            $table->unique('trans_no', 'trans_head_trans_no_unique');
        });

        // Add unique constraint to this_head.trans_no
        Schema::table('this_head', function (Blueprint $table) {
            $table->unique('trans_no', 'this_head_trans_no_unique');
        });

        // Add unique constraint to this_detail for combination of fields
        Schema::table('this_detail', function (Blueprint $table) {
            $table->unique(
                ['trans_no', 'trans_count', 'acc_code', 'div_code'],
                'this_detail_unique_transaction'
            );
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::table('trans_head', function (Blueprint $table) {
            $table->dropUnique('trans_head_trans_no_unique');
        });

        Schema::table('this_head', function (Blueprint $table) {
            $table->dropUnique('this_head_trans_no_unique');
        });

        Schema::table('this_detail', function (Blueprint $table) {
            $table->dropUnique('this_detail_unique_transaction');
        });
    }
};
