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
        Schema::create('this_head', function (Blueprint $table) {
            $table->string('period', 2)->nullable();
            $table->string('trans_code', 5)->nullable();
            $table->string('trans_no', 15)->nullable();
            $table->date('trans_date')->nullable();
            $table->date('trans_update')->nullable();
            $table->string('trans_user', 5)->nullable();
            $table->float('debet')->nullable();
            $table->float('credit')->nullable();
            $table->timestamps();
            $table->foreign('trans_code')->references('trans_code')->on('mst_transaction')->onUpdate('cascade')->onDelete('cascade');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('this_head');
    }
};
