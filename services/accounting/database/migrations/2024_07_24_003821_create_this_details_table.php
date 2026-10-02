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
        Schema::create('this_detail', function (Blueprint $table) {
            $table->id();
            $table->string('period', 2);
            $table->string('trans_code', 5)->nullable();
            $table->string('trans_no', 15)->nullable();
            $table->smallInteger('trans_count')->nullable();
            $table->string('acc_code', 15)->nullable();
            $table->string('div_code', 5)->nullable();
            $table->string('description', 250)->nullable();
            $table->float('debet')->nullable();
            $table->float('credit')->nullable();
            $table->string('curr_code', 5)->nullable();
            $table->float('curr_rate')->nullable();
            $table->float('curr_debet')->nullable();
            $table->float('curr_credit')->nullable();
            $table->timestamp('trans_date')->nullable();
            $table->string('curr_system', 1)->nullable();
            $table->boolean('curr_yes')->nullable();
            $table->timestamps();
            $table->foreign('trans_code')->references('trans_code')->on('mst_transaction')->onUpdate('cascade')->onDelete('cascade');
            $table->foreign('curr_code')->references('curr_code')->on('mst_currency')->onUpdate('cascade')->onDelete('cascade');
            $table->foreign('div_code')->references('div_code')->on('mst_division')->onUpdate('cascade')->onDelete('cascade');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('this_detail');
    }
};
