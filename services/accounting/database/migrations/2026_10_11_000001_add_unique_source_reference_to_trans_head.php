<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

/**
 * BR-INT-01 / AC-98-02: Ensure idempotent journal posting from payroll and payment gateway.
 * Multiple posts with the same source and non-null reference must not produce duplicate journals.
 */
return new class extends Migration
{
    public function up(): void
    {
        DB::statement('CREATE UNIQUE INDEX IF NOT EXISTS idx_trans_head_source_reference ON trans_head (source, reference) WHERE reference IS NOT NULL');
    }

    public function down(): void
    {
        DB::statement('DROP INDEX IF EXISTS idx_trans_head_source_reference');
    }
};
