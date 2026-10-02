<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

return new class extends Migration
{
    public function up(): void
    {
        DB::statement('ALTER TABLE this_detail DROP CONSTRAINT IF EXISTS this_detail_unique_transaction');
        DB::statement(<<<'SQL'
            DO $$ BEGIN
                IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='this_detail_unique_transaction') THEN
                    ALTER TABLE this_detail
                        ADD CONSTRAINT this_detail_unique_transaction
                        UNIQUE (trans_no, trans_code, trans_count, acc_code, div_code, trans_date);
                END IF;
            END $$;
        SQL);
    }

    public function down(): void
    {
        DB::statement('ALTER TABLE this_detail DROP CONSTRAINT IF EXISTS this_detail_unique_transaction');
        DB::statement(<<<'SQL'
            DO $$ BEGIN
                IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='this_detail_unique_transaction') THEN
                    ALTER TABLE this_detail
                        ADD CONSTRAINT this_detail_unique_transaction
                        UNIQUE (trans_no, trans_count, acc_code, div_code, trans_date);
                END IF;
            END $$;
        SQL);
    }
};
