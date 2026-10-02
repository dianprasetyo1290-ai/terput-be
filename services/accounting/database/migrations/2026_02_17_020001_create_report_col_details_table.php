<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::create('report_col_details', function (Blueprint $table) {
            $table->id();
            $table->foreignId('report_col_head_id')->constrained('report_col_heads')->onDelete('cascade');
            $table->integer('sort_order')->default(0);
            $table->string('col_title', 100);
            $table->string('content', 20)->default('Current'); // Current, Budget, Formula, Base%
            $table->string('period', 50)->nullable(); // Curr, Curr - 01, Beg, YTD, 01, 01,02,03, etc.
            $table->string('division', 10)->default('*');
            $table->string('col_format', 30)->default('#,##0.00');
            $table->integer('period_key')->nullable(); // For dynamic templates: month (1-12) or quarter max period (3,6,9,12)
            $table->timestamps();
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('report_col_details');
    }
};
