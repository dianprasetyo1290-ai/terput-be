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
        Schema::table('mst_account', function (Blueprint $table) {
            $table->foreign('acc_currency')->references('curr_code')->on('mst_currency')->onUpdate('cascade')->onDelete('cascade');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::table('mst_account', function (Blueprint $table) {
            $table->dropForeign(['acc_currency']);
        });
    }
};
