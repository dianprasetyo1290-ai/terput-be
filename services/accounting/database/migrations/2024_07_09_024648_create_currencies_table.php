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
        Schema::create('mst_currency', function (Blueprint $table) {
            $table->string('curr_code', 5)->unique();
            $table->string('curr_name', 50)->nullable();
            $table->string('rate_system', 1)->nullable();
            $table->string('act_01')->nullable();
            $table->string('act_02')->nullable();
            $table->string('act_03')->nullable();
            $table->string('act_04')->nullable();
            $table->string('act_05')->nullable();
            $table->string('act_06')->nullable();
            $table->string('act_07')->nullable();
            $table->string('act_08')->nullable();
            $table->string('act_09')->nullable();
            $table->string('act_10')->nullable();
            $table->string('act_11')->nullable();
            $table->string('act_12')->nullable();
            $table->string('act_13')->nullable();
            $table->string('lst_01')->nullable();
            $table->string('lst_02')->nullable();
            $table->string('lst_03')->nullable();
            $table->string('lst_04')->nullable();
            $table->string('lst_05')->nullable();
            $table->string('lst_06')->nullable();
            $table->string('lst_07')->nullable();
            $table->string('lst_08')->nullable();
            $table->string('lst_09')->nullable();
            $table->string('lst_10')->nullable();
            $table->string('lst_11')->nullable();
            $table->string('lst_12')->nullable();
            $table->string('lst_13')->nullable();
            $table->timestamps();
            $table->index('curr_code');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('mst_currency');
    }
};
