<?php

namespace Database\Seeders;

use App\Models\Modul;
use Illuminate\Database\Seeder;
use Spatie\Permission\Models\Permission;

class ModulesSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        Modul::truncate();
        $modules = [
            ['name' => 'journal_transaction', 'display_name' => 'Journal Transaction'],
            ['name' => 'end_of_month', 'display_name' => 'End of Month'],
            ['name' => 'history_journal', 'display_name' => 'History Journal'],
            ['name' => 'history_journal_detail', 'display_name' => 'History Journal Detail'],
            ['name' => 'end_of_year', 'display_name' => 'End of Year'],
            ['name' => 'prior_year_adjustment', 'display_name' => 'Prior Year Adjustment'],
            ['name' => 'general_ledger', 'display_name' => 'General Ledger'],
            ['name' => 'financial_report', 'display_name' => 'Financial Report'],
            ['name' => 'report_row_design', 'display_name' => 'Report Row Design'],
            ['name' => 'final_design', 'display_name' => 'Final Design'],
            ['name' => 'balance', 'display_name' => 'Balance'],
            ['name' => 'chart_of_account', 'display_name' => 'Chart of Account'],
            ['name' => 'company_parameter', 'display_name' => 'Company Parameter'],
            ['name' => 'currency_table', 'display_name' => 'Currency Table'],
            ['name' => 'division', 'display_name' => 'Division'],
            ['name' => 'periode', 'display_name' => 'Periode'],
            ['name' => 'transaction_type', 'display_name' => 'Transaction Type'],
            ['name' => 'users', 'display_name' => 'Users'],
            ['name' => 'role', 'display_name' => 'Role'],
            ['name' => 'permission', 'display_name' => 'Permission'],
            ['name' => 'account_group_table', 'display_name' => 'Account Group Table'],
            ['name' => 'sub_account_group_table', 'display_name' => 'Sub Account Group Table'],

            // ['name' => 'balance_currency', 'display_name' => 'Balance Currency'],
            // ['name' => 'budgeting', 'display_name' => 'Budgeting'],
            // ['name' => 'setting_website', 'display_name' => 'Setting Website'],
            // ['name' => 'journal_validation', 'display_name' => 'Journal Validation'],
            // ['name' => 'history_journal_validation', 'display_name' => 'History Journal Validation'],
            // ['name' => 'report_column', 'display_name' => 'Report Column'],
        ];

        foreach ($modules as $module) {
            Modul::create([
                'name' => $module['name'],
                'display_name' => $module['display_name'],
            ]);
        }
        $modules = Modul::get();
        $actions = ['create', 'edit', 'delete', 'view'];
        Permission::truncate();
        foreach ($modules as $module) {
            foreach ($actions as $action) {
                Permission::updateOrCreate(
                    ['name' => "$action$module->name", 'guard_name' => 'web'],
                    ['name' => "$action$module->name", 'guard_name' => 'web'] // Fields to update if exists
                );
            }
        }
    }
}
