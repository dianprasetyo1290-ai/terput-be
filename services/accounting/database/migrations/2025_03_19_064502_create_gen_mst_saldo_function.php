<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        // Unqualified on purpose (schema-per-tenant, ADDENDUM 1): these used
        // to say public.*, which made every school's saldo generation read
        // and write the SHARED schema instead of its own books. plpgsql
        // resolves unqualified names against the caller's search_path, which
        // TenantSchema has already pointed at the school's schema.
        $sql = <<<SQL
        CREATE OR REPLACE FUNCTION gen_mst_saldo(_tahun integer)
        RETURNS void AS $$
        BEGIN
          INSERT INTO mst_saldo_news (acc_code, div_code, periode, tahun, debet, credit)
          SELECT a.acc_code, d.div_code, gs.period, _tahun, 0, 0
          FROM mst_account a
          CROSS JOIN mst_division d
          CROSS JOIN generate_series(0, 13) AS gs(period)
          LEFT JOIN mst_saldo_news s
            ON s.acc_code = a.acc_code
            AND s.div_code = d.div_code
            AND s.periode = gs.period
            AND s.tahun = _tahun
          WHERE s.id IS NULL;
        END;
        $$ LANGUAGE plpgsql;
        SQL;

        DB::statement($sql);
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        DB::statement('DROP FUNCTION IF EXISTS gen_mst_saldo(integer)');
    }
};
