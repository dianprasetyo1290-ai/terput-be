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
        Schema::create('mst_transaction', function (Blueprint $table) {
            $table->string('trans_code', '5')->unique();
            $table->string('trans_name', '50')->nullable();
            $table->string('trans_status', '1')->nullable();
            $table->string('trans_account', '15')->nullable();
            $table->timestamps();
            $table->index('trans_code');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('mst_transaction');
    }
};
