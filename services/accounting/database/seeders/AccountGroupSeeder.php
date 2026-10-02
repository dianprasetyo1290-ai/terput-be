<?php

namespace Database\Seeders;

use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use App\Models\AccountGroupTable;

class AccountGroupSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        $data = [
            ['id' => '1', 'name' => 'Asset'],
            ['id' => '2', 'name' => 'Liability'],
            ['id' => '3', 'name' => 'Equity'],
            ['id' => '4', 'name' => 'Income'],
            ['id' => '5', 'name' => 'Cost of Sales'],
            ['id' => '6', 'name' => 'Expense'],
            ['id' => '7', 'name' => 'Other Income'],
            ['id' => '8', 'name' => 'Other Expense'],
        ];

        AccountGroupTable::insert($data);
    }
}
