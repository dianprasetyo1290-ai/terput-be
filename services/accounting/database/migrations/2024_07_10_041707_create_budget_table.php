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
        Schema::create('mst_budget', function (Blueprint $table) {
            $table->id();
            $table->string('acc_code', 15)->nullable();
            $table->string('div_code', 5)->nullable();
            $table->float('act_01', 8, 2)->nullable();
            $table->float('act_02', 8, 2)->nullable();
            $table->float('act_03', 8, 2)->nullable();
            $table->float('act_04', 8, 2)->nullable();
            $table->float('act_05', 8, 2)->nullable();
            $table->float('act_06', 8, 2)->nullable();
            $table->float('act_07', 8, 2)->nullable();
            $table->float('act_08', 8, 2)->nullable();
            $table->float('act_09', 8, 2)->nullable();
            $table->float('act_10', 8, 2)->nullable();
            $table->float('act_11', 8, 2)->nullable();
            $table->float('act_12', 8, 2)->nullable();
            $table->float('act_13', 8, 2)->nullable();
            $table->float('lst_01', 8, 2)->nullable();
            $table->float('lst_02', 8, 2)->nullable();
            $table->float('lst_03', 8, 2)->nullable();
            $table->float('lst_04', 8, 2)->nullable();
            $table->float('lst_05', 8, 2)->nullable();
            $table->float('lst_06', 8, 2)->nullable();
            $table->float('lst_07', 8, 2)->nullable();
            $table->float('lst_08', 8, 2)->nullable();
            $table->float('lst_09', 8, 2)->nullable();
            $table->float('lst_10', 8, 2)->nullable();
            $table->float('lst_11', 8, 2)->nullable();
            $table->float('lst_12', 8, 2)->nullable();
            $table->float('lst_13', 8, 2)->nullable();
            $table->float('fut_01', 8, 2)->nullable();
            $table->float('fut_02', 8, 2)->nullable();
            $table->float('fut_03', 8, 2)->nullable();
            $table->float('fut_04', 8, 2)->nullable();
            $table->float('fut_05', 8, 2)->nullable();
            $table->float('fut_06', 8, 2)->nullable();
            $table->float('fut_07', 8, 2)->nullable();
            $table->float('fut_08', 8, 2)->nullable();
            $table->float('fut_09', 8, 2)->nullable();
            $table->float('fut_10', 8, 2)->nullable();
            $table->float('fut_11', 8, 2)->nullable();
            $table->float('fut_12', 8, 2)->nullable();
            $table->float('fut_13', 8, 2)->nullable();
            $table->timestamps();

            $table->foreign('acc_code')->references('acc_code')->on('mst_account')->onUpdate('cascade')->onDelete('cascade');
            $table->foreign('div_code')->references('div_code')->on('mst_division')->onUpdate('cascade')->onDelete('cascade');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('mst_budget');
    }
};
