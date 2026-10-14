<?php

namespace Database\Seeders;

use App\Models\Modul;
use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Spatie\Permission\Models\Permission;
use Spatie\Permission\Models\Role;

class RoleAndPermissionSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        app()[\Spatie\Permission\PermissionRegistrar::class]->forgetCachedPermissions();

        // create permissions


        Role::truncate();
        // Create the SuperAdmin role and assign all permissions to it
        $xyz = Role::firstOrCreate(['name' => 'SuperAdmin']);
        $xyz->givePermissionTo(Permission::all());

        // Admin, Accounting, FinanceManager, Auditor, Executive (+ legacy
        // Finance): one spec shared with the tenant migration. Each school's
        // books live in their own schema, so "everything" still only ever
        // means that one school's data.
        \App\Support\AccountingRoles::install();
    }
}
