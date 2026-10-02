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
        Schema::create('trans_head', function (Blueprint $table) {
            $table->string('trans_code', 5);
            $table->string('trans_no', 15);
            $table->date('trans_date');
            $table->date('trans_update');
            $table->string('trans_user', 5);
            $table->float('debet')->nullable();
            $table->float('credit')->nullable();
            $table->timestamps();
            $table->foreign('trans_code')->references('trans_code')->on('mst_transaction')->onUpdate('cascade')->onDelete('cascade');
            $table->foreign('trans_user')->references('user_id')->on('mst_user')->onUpdate('cascade')->onDelete('cascade');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('trans_head');
    }
};
