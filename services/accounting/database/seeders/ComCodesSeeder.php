<?php

namespace Database\Seeders;

use App\Models\AccountGroupTable;
use App\Models\ComCodes;
use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;

class ComCodesSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        ComCodes::truncate();

        $data = [
            ['code_cd' => '1', 'code_nm' => 'Asset', 'code_group' => 'ACCOUNT_GROUP', 'code_value' => ''],
            ['code_cd' => '2', 'code_nm' => 'Liability', 'code_group' => 'ACCOUNT_GROUP', 'code_value' => ''],
            ['code_cd' => '3', 'code_nm' => 'Equity', 'code_group' => 'ACCOUNT_GROUP', 'code_value' => ''],
            ['code_cd' => '4', 'code_nm' => 'Income', 'code_group' => 'ACCOUNT_GROUP', 'code_value' => ''],
            ['code_cd' => '5', 'code_nm' => 'Cost of Sales', 'code_group' => 'ACCOUNT_GROUP', 'code_value' => ''],
            ['code_cd' => '6', 'code_nm' => 'Expense', 'code_group' => 'ACCOUNT_GROUP', 'code_value' => ''],
            ['code_cd' => '7', 'code_nm' => 'Other Income', 'code_group' => 'ACCOUNT_GROUP', 'code_value' => ''],
            ['code_cd' => '8', 'code_nm' => 'Other Expense', 'code_group' => 'ACCOUNT_GROUP', 'code_value' => ''],
            ['code_cd' => 'B', 'code_nm' => '1 Base = n Foreign', 'code_group' => 'RATE_SYSTEM', 'code_value' => ''],
            ['code_cd' => 'F', 'code_nm' => '1 Foreign = n Base', 'code_group' => 'RATE_SYSTEM', 'code_value' => ''],
            ['code_cd' => 'D', 'code_nm' => 'Debet Transaction', 'code_group' => 'TRANSACTION', 'code_value' => ''],
            ['code_cd' => 'C', 'code_nm' => 'Credit Transaction', 'code_group' => 'TRANSACTION', 'code_value' => ''],
            ['code_cd' => 'M', 'code_nm' => 'Memorial Transaction', 'code_group' => 'TRANSACTION', 'code_value' => ''],
        ];

        // Insert data into ComCodes table
        ComCodes::insert($data);

        // // Create AccountGroupTable entries for ACCOUNT_GROUP codes
        // foreach ($data as $value) {
        //     if ($value['code_group'] == 'ACCOUNT_GROUP') {
        //         AccountGroupTable::create([
        //             'id' => $value['code_cd'],
        //             'name' => $value['code_nm']
        //         ]);
        //     }
        // }
        // DB::statement("SELECT setval('account_group_tables_id_seq', (SELECT MAX(id) FROM account_group_tables))");
    }
}
