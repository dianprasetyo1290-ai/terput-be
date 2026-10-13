<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

/**
 * T17: Ekstensi akuntansi untuk service ops:
 *  - acc_vendors: sync vendor dari T15 (FR-106-005)
 *  - trans_detail, this_detail: kolom vendor_id untuk tracking hutang per vendor
 *  - fa_assets, fa_asset_events, fa_depreciation: buku aset tetap untuk T16 & T17
 *
 * Menggunakan raw SQL IF NOT EXISTS tanpa schema qualifier agar tenant-safe.
 */
return new class extends Migration
{
    public function up(): void
    {
        // acc_vendors: sinkronisasi vendor pengadaan
        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS acc_vendors (
                vendor_id uuid PRIMARY KEY,
                code varchar(100) NOT NULL,
                name varchar(255) NOT NULL,
                npwp_masked varchar(50) NULL,
                status varchar(20) NOT NULL DEFAULT 'active',
                updated_at timestamp NOT NULL DEFAULT now()
            )
        SQL);

        // vendor_id pada trans_detail dan this_detail
        foreach (['trans_detail', 'this_detail'] as $t) {
            DB::statement("ALTER TABLE $t ADD COLUMN IF NOT EXISTS vendor_id uuid NULL");
        }

        // fa_assets: daftar aset tetap untuk depresiasi dan nilai buku
        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS fa_assets (
                asset_ref uuid PRIMARY KEY,
                code varchar(100) NOT NULL,
                name varchar(255) NOT NULL,
                acquired_on date NOT NULL,
                cost numeric(20,2) NOT NULL,
                residual numeric(20,2) NOT NULL DEFAULT 0,
                opening_accum numeric(20,2) NOT NULL DEFAULT 0,
                useful_life_months int NOT NULL DEFAULT 48,
                dep_start varchar(20) NOT NULL DEFAULT 'acquisition_month',
                asset_acc_code varchar(20) NOT NULL,
                accum_dep_acc_code varchar(20) NOT NULL,
                dep_expense_acc_code varchar(20) NOT NULL,
                status varchar(15) NOT NULL DEFAULT 'active',
                closed_on date NULL,
                created_at timestamp NOT NULL DEFAULT now(),
                updated_at timestamp NOT NULL DEFAULT now()
            )
        SQL);

        // fa_asset_events: outbox jurnal dan riwayat mutasi/disposisi
        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS fa_asset_events (
                reference varchar(100) PRIMARY KEY,
                asset_ref uuid NOT NULL,
                kind varchar(15) NOT NULL,
                trans_no varchar(15) NULL,
                payload jsonb NOT NULL,
                created_at timestamp NOT NULL DEFAULT now()
            )
        SQL);

        // fa_depreciation: riwayat penyusutan bulanan per aset
        DB::statement(<<<'SQL'
            CREATE TABLE IF NOT EXISTS fa_depreciation (
                asset_ref uuid NOT NULL,
                period char(7) NOT NULL,
                amount numeric(20,2) NOT NULL,
                trans_no varchar(15) NULL,
                PRIMARY KEY (asset_ref, period)
            )
        SQL);
    }

    public function down(): void
    {
        DB::statement('DROP TABLE IF EXISTS fa_depreciation');
        DB::statement('DROP TABLE IF EXISTS fa_asset_events');
        DB::statement('DROP TABLE IF EXISTS fa_assets');
        DB::statement('ALTER TABLE trans_detail DROP COLUMN IF EXISTS vendor_id');
        DB::statement('ALTER TABLE this_detail DROP COLUMN IF EXISTS vendor_id');
        DB::statement('DROP TABLE IF EXISTS acc_vendors');
    }
};
