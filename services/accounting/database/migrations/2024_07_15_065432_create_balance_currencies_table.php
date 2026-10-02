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
        Schema::create('mst_saldocurr', function (Blueprint $table) {
            $table->id();
            $table->string('acc_code', 15)->nullable();
            $table->string('div_code', 5)->nullable();
            $table->string('curr_code', 5)->nullable();
            $table->float('curd', 8, 2)->nullable();
            $table->float('curk', 8, 2)->nullable();
            $table->float('act_00', 8, 2)->nullable();
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
            $table->float('actk_00', 8, 2)->nullable();
            $table->float('actk_01', 8, 2)->nullable();
            $table->float('actk_02', 8, 2)->nullable();
            $table->float('actk_03', 8, 2)->nullable();
            $table->float('actk_04', 8, 2)->nullable();
            $table->float('actk_05', 8, 2)->nullable();
            $table->float('actk_06', 8, 2)->nullable();
            $table->float('actk_07', 8, 2)->nullable();
            $table->float('actk_08', 8, 2)->nullable();
            $table->float('actk_09', 8, 2)->nullable();
            $table->float('actk_10', 8, 2)->nullable();
            $table->float('actk_11', 8, 2)->nullable();
            $table->float('actk_12', 8, 2)->nullable();
            $table->float('actk_13', 8, 2)->nullable();
            $table->float('lst_00', 8, 2)->nullable();
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
            $table->float('lstk_00', 8, 2)->nullable();
            $table->float('lstk_01', 8, 2)->nullable();
            $table->float('lstk_02', 8, 2)->nullable();
            $table->float('lstk_03', 8, 2)->nullable();
            $table->float('lstk_04', 8, 2)->nullable();
            $table->float('lstk_05', 8, 2)->nullable();
            $table->float('lstk_06', 8, 2)->nullable();
            $table->float('lstk_07', 8, 2)->nullable();
            $table->float('lstk_08', 8, 2)->nullable();
            $table->float('lstk_09', 8, 2)->nullable();
            $table->float('lstk_10', 8, 2)->nullable();
            $table->float('lstk_11', 8, 2)->nullable();
            $table->float('lstk_12', 8, 2)->nullable();
            $table->float('lstk_13', 8, 2)->nullable();
            $table->float('snd_00', 8, 2)->nullable();
            $table->float('snd_01', 8, 2)->nullable();
            $table->float('snd_02', 8, 2)->nullable();
            $table->float('snd_03', 8, 2)->nullable();
            $table->float('snd_04', 8, 2)->nullable();
            $table->float('snd_05', 8, 2)->nullable();
            $table->float('snd_06', 8, 2)->nullable();
            $table->float('snd_07', 8, 2)->nullable();
            $table->float('snd_08', 8, 2)->nullable();
            $table->float('snd_09', 8, 2)->nullable();
            $table->float('snd_10', 8, 2)->nullable();
            $table->float('snd_11', 8, 2)->nullable();
            $table->float('snd_12', 8, 2)->nullable();
            $table->float('snd_13', 8, 2)->nullable();
            $table->float('sndk_00', 8, 2)->nullable();
            $table->float('sndk_01', 8, 2)->nullable();
            $table->float('sndk_02', 8, 2)->nullable();
            $table->float('sndk_03', 8, 2)->nullable();
            $table->float('sndk_04', 8, 2)->nullable();
            $table->float('sndk_05', 8, 2)->nullable();
            $table->float('sndk_06', 8, 2)->nullable();
            $table->float('sndk_07', 8, 2)->nullable();
            $table->float('sndk_08', 8, 2)->nullable();
            $table->float('sndk_09', 8, 2)->nullable();
            $table->float('sndk_10', 8, 2)->nullable();
            $table->float('sndk_11', 8, 2)->nullable();
            $table->float('sndk_12', 8, 2)->nullable();
            $table->float('sndk_13', 8, 2)->nullable();
            $table->timestamps();

            $table->foreign('acc_code')->references('acc_code')->on('mst_account')->onUpdate('cascade')->onDelete('cascade');
            $table->foreign('div_code')->references('div_code')->on('mst_division')->onUpdate('cascade')->onDelete('cascade');
            $table->foreign('curr_code')->references('curr_code')->on('mst_currency')->onUpdate('cascade')->onDelete('cascade');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('mst_saldocurr');
    }
};
