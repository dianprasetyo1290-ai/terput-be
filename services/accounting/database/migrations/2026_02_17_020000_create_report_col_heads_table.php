<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::create('report_col_heads', function (Blueprint $table) {
            $table->id();
            $table->string('name', 100);
            $table->string('col_type', 30)->default('fixed'); // fixed, ytd, bs_quarterly, is_quarterly
            $table->timestamps();
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('report_col_heads');
    }
};
