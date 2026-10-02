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

        // These three were created with ZERO permissions, so anyone mapped to
        // them could log in and open nothing but the dashboard. Each school's
        // books live in their own schema, so "everything" here still only ever
        // means that one school's data.
        //   Admin      = the school's admin: full access to its own books.
        //   Accounting = same, minus user/role/permission administration.
        //   Finance    = read-only (kepala sekolah / kepala yayasan).
        Role::firstOrCreate(['name' => 'Admin'])
            ->syncPermissions(Permission::all());

        Role::firstOrCreate(['name' => 'Accounting'])
            ->syncPermissions(
                Permission::where('name', 'not like', '%user%')
                    ->where('name', 'not like', '%role%')
                    ->where('name', 'not like', '%permission%')
                    ->get()
            );

        Role::firstOrCreate(['name' => 'Finance'])
            ->syncPermissions(Permission::where('name', 'like', 'view%')->get());
    }
}
