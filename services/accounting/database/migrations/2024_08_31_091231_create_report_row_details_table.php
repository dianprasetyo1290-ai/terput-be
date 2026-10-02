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
        Schema::create('report_row_details', function (Blueprint $table) {
            $table->id();
            $table->unsignedBigInteger('report_row_head_id');
            $table->string('name');
            $table->string('group')->comment('h deader, d detail, t total, g grant total');
            $table->integer('level')->comment('0,1,2');
            $table->string('normal_balance')->nullable()->comment('d,c');
            $table->string('operator')->nullable()->comment('i increase, d decrase');
            $table->string('kode')->nullable();
            $table->timestamps();
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('report_row_details');
    }
};
