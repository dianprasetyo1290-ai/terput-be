<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

/**
 * acc_auto_postings (FR-32-002 "posting otomatis gagal") is written by
 * AutoJournalService, one row per (source, reference) the internal journal API
 * receives: billing posts one summary journal per day per unit per component
 * (BR-ACC-06), so (posting_date, kind) is not unique. Nothing wrote the table
 * before this change, so dropping that constraint loses nothing.
 */
return new class extends Migration
{
    public function up(): void
    {
        DB::statement('ALTER TABLE acc_auto_postings ADD COLUMN IF NOT EXISTS reference varchar(100)');
        DB::statement('ALTER TABLE acc_auto_postings DROP CONSTRAINT IF EXISTS acc_auto_postings_posting_date_kind_key');
        DB::statement('CREATE UNIQUE INDEX IF NOT EXISTS acc_auto_postings_kind_reference_uq ON acc_auto_postings (kind, reference) WHERE reference IS NOT NULL');
        DB::statement('CREATE INDEX IF NOT EXISTS acc_auto_postings_status_idx ON acc_auto_postings (status)');
    }

    public function down(): void
    {
        DB::statement('DROP INDEX IF EXISTS acc_auto_postings_status_idx');
        DB::statement('DROP INDEX IF EXISTS acc_auto_postings_kind_reference_uq');
        DB::statement('ALTER TABLE acc_auto_postings DROP COLUMN IF EXISTS reference');
    }
};
