<?php

namespace Database\Seeders;

use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;

class ViewSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        DB::statement("CREATE OR replace VIEW vw_balance AS
        SELECT DISTINCT msn.acc_code, 
                        msn.div_code, 
                        ma.acc_name, 
                        md.div_name
        FROM mst_saldo_news msn
        JOIN mst_account ma ON msn.acc_code = ma.acc_code
        JOIN mst_division md ON msn.div_code = md.div_code;");
    }
}
