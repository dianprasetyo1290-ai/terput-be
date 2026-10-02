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
        Schema::create('parameter', function (Blueprint $table) {
            $table->string('company', 50)->nullable();
            $table->string('serial_no', 12)->unique();
            $table->string('address')->nullable();
            $table->string('contact', 50)->nullable();
            $table->string('tax_number', 18)->nullable();
            $table->bigInteger('period')->nullable();
            $table->bigInteger('year')->nullable();
            $table->string('mask_account', 20)->nullable();
            $table->string('base_currency', 5)->nullable();
            $table->boolean('control_date')->nullable();
            $table->timestamps();
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('parameter');
    }
};
