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
        Schema::create('mst_period', function (Blueprint $table) {
            $table->id();
            $table->string('period_code', 4)->nullable();
            $table->string('period_name', 15)->nullable();
            $table->string('p01', 20)->nullable();
            $table->date('p01_from')->nullable();
            $table->date('p01_to')->nullable();
            $table->string('p02', 20)->nullable();
            $table->date('p02_from')->nullable();
            $table->date('p02_to')->nullable();
            $table->string('p03', 20)->nullable();
            $table->date('p03_from')->nullable();
            $table->date('p03_to')->nullable();
            $table->string('p04', 20)->nullable();
            $table->date('p04_from')->nullable();
            $table->date('p04_to')->nullable();
            $table->string('p05', 20)->nullable();
            $table->date('p05_from')->nullable();
            $table->date('p05_to')->nullable();
            $table->string('p06', 20)->nullable();
            $table->date('p06_from')->nullable();
            $table->date('p06_to')->nullable();
            $table->string('p07', 20)->nullable();
            $table->date('p07_from')->nullable();
            $table->date('p07_to')->nullable();
            $table->string('p08', 20)->nullable();
            $table->date('p08_from')->nullable();
            $table->date('p08_to')->nullable();
            $table->string('p09', 20)->nullable();
            $table->date('p09_from')->nullable();
            $table->date('p09_to')->nullable();
            $table->string('p10', 20)->nullable();
            $table->date('p10_from')->nullable();
            $table->date('p10_to')->nullable();
            $table->string('p11', 20)->nullable();
            $table->date('p11_from')->nullable();
            $table->date('p11_to')->nullable();
            $table->string('p12', 20)->nullable();
            $table->date('p12_from')->nullable();
            $table->date('p12_to')->nullable();
            $table->string('p13', 20)->nullable();
            $table->date('p13_from')->nullable();
            $table->date('p13_to')->nullable();
            $table->timestamps();
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('mst_period');
    }
};
