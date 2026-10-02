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
        Schema::create('mst_saldo_news', function (Blueprint $table) {
            $table->id();
            $table->string('acc_code');
            $table->string('div_code');
            $table->smallInteger('periode')->comment('1-12');
            $table->integer('tahun')->comment('2024');
            $table->float('debet')->comment('jumlah nya');
            $table->float('credit')->comment('jumlah nya');
            $table->timestamps();
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('mst_saldo_news');
    }
};
