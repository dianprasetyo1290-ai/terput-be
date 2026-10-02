<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::table('report_finals', function (Blueprint $table) {
            $table->foreignId('report_col_head_id')->nullable()->constrained('report_col_heads')->nullOnDelete();
        });
    }

    public function down(): void
    {
        Schema::table('report_finals', function (Blueprint $table) {
            $table->dropConstrainedForeignId('report_col_head_id');
        });
    }
};
