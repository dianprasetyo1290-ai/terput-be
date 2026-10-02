<?php

namespace Database\Seeders;

// use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;

class DatabaseSeeder extends Seeder
{
    /**
     * Seed the application's database.
     */
    public function run(): void
    {
        $this->call(ModulesSeeder::class);
        $this->call(RoleAndPermissionSeeder::class);
        $this->call(UserSeeder::class);
        $this->call(SettingWebSeeder::class);
        $this->call(ComCodesSeeder::class);
        $this->call(PeriodSeeder::class);
        $this->call(ParameterSeeder::class);
        $this->call(ViewSeeder::class);
        $this->call(RowDetailsSeeder::class);
        $this->call(ReportColumnSeeder::class);
        // $this->call(CurrencySeeder::class);
        // $this->call(DivisionSeeder::class);
        // $this->call(COASeeder::class);
        // $this->call(TPSeeder::class);
        // $this->call(SaldoSeeder::class);
        // $this->call(AccountGroupSeeder::class);
        // $this->call(TransHeadSeeder::class);
        // $this->call(TransDetailSeeder::class);
        // $this->call(ThisHeadSeeder::class);
        // $this->call(ThisDetailSeeder::class);
    }
}
