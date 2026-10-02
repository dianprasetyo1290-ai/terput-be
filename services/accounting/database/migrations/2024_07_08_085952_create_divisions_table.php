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
        Schema::create('mst_division', function (Blueprint $table) {
            $table->string('div_code', 5)->unique();
            $table->string('div_name', 50)->nullable();
            $table->timestamps();
            $table->index('div_code');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('mst_division');
    }
};
