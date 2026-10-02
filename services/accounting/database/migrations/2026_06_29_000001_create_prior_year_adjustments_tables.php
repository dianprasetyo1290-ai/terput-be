<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::create('prior_year_adjustment_sessions', function (Blueprint $table) {
            $table->id();
            $table->integer('target_year');
            $table->date('adjustment_date');
            $table->string('trans_code', 5)->default('PYA');
            $table->string('trans_no', 15)->nullable()->unique();
            $table->string('status', 20)->default('draft');
            $table->string('description', 250)->nullable();
            $table->timestamp('finalized_at')->nullable();
            $table->string('finalized_by', 20)->nullable();
            $table->timestamps();

            $table->index(['target_year', 'status']);
        });

        Schema::create('prior_year_adjustment_lines', function (Blueprint $table) {
            $table->id();
            $table->foreignId('session_id')
                ->constrained('prior_year_adjustment_sessions')
                ->cascadeOnDelete();
            $table->string('acc_code', 15);
            $table->string('div_code', 5);
            $table->string('description', 250)->nullable();
            $table->decimal('debet', 20, 2)->default(0);
            $table->decimal('credit', 20, 2)->default(0);
            $table->string('curr_code', 5)->nullable();
            $table->decimal('curr_rate', 20, 6)->default(0);
            $table->decimal('curr_debet', 20, 2)->default(0);
            $table->decimal('curr_credit', 20, 2)->default(0);
            $table->timestamps();

            $table->index(['session_id', 'acc_code', 'div_code']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('prior_year_adjustment_lines');
        Schema::dropIfExists('prior_year_adjustment_sessions');
    }
};
