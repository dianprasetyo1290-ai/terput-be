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
        Schema::create('mst_account', function (Blueprint $table) {
            $table->string('acc_code', 15)->unique();
            $table->string('acc_name', 50)->nullable();
            $table->unsignedBigInteger('acc_group')->nullable();
            $table->unsignedBigInteger('sub_acc_group')->nullable();
            $table->string('acc_currency', 5)->nullable();
            $table->timestamps();
            $table->index('acc_code');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('mst_account');
    }
};
