<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

return new class extends Migration
{
    public function up(): void
    {
        // Idempotent: aman dijalankan ulang. DROP pakai IF EXISTS, ADD pakai
        // DO block dengan pengecekan pg_constraint biar tidak error kalau
        // sudah ada / sudah di-drop di lingkungan apapun.

        // this_head: ganti UNIQUE(trans_no) -> UNIQUE(trans_no, trans_date)
        // Recurring JV (mis. JV/1/25 dipakai tiap bulan) butuh trans_date
        // sebagai diskriminator antar periode.
        DB::statement('ALTER TABLE this_head DROP CONSTRAINT IF EXISTS this_head_trans_no_unique');
        DB::statement(<<<'SQL'
            DO $$ BEGIN
                IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='this_head_trans_no_date_unique') THEN
                    ALTER TABLE this_head
                        ADD CONSTRAINT this_head_trans_no_date_unique
                        UNIQUE (trans_no, trans_date);
                END IF;
            END $$;
        SQL);

        // this_detail: ganti UNIQUE(trans_no, trans_count, acc_code, div_code, trans_date)
        // -> tambah trans_code. Data legacy bisa punya baris dengan trans_code beda
        // pada trans_count yang sama, jadi jangan drop detail line yang valid.
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
                        UNIQUE (trans_no, trans_count, acc_code, div_code);
                END IF;
            END $$;
        SQL);

        DB::statement('ALTER TABLE this_head DROP CONSTRAINT IF EXISTS this_head_trans_no_date_unique');
        DB::statement(<<<'SQL'
            DO $$ BEGIN
                IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='this_head_trans_no_unique') THEN
                    ALTER TABLE this_head
                        ADD CONSTRAINT this_head_trans_no_unique
                        UNIQUE (trans_no);
                END IF;
            END $$;
        SQL);
    }
};
