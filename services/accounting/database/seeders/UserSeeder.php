<?php

namespace Database\Seeders;

use App\Models\User;
use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Hash;
use Spatie\Permission\Models\Role;
use Spatie\Permission\Models\Permission;

class UserSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        $superadmin = User::updateOrCreate(
            [
                'email' => 'superadmin@app.com',
            ],
            [
                'user_name' => 'SuperAdmin',
                'user_id' => '0001',
                'password' => Hash::make('password'),
                'email_verified_at' => now(),
            ]
        );
        
        // Check if the role exists before assigning it
        if (Role::where('name', 'SuperAdmin')->exists()) {
            $superadmin->assignRole('SuperAdmin');
            
            // Berikan semua permission kepada superadmin
            $permissions = Permission::all();
            $superadmin->syncPermissions($permissions);
        }

        $admin = User::updateOrCreate(
            [
                'email' => 'admin@app.com',
            ],
            [
                'user_name' => 'Admin',
                'user_id' => '0002',
                'password' => Hash::make('password'),
                'email_verified_at' => now(),
            ]
        );
        
        if (Role::where('name', 'SuperAdmin')->exists()) {
            $admin->syncRoles(['SuperAdmin']);
            $admin->syncPermissions(Permission::all());
        }

        $testAdmin = User::updateOrCreate(
            [
                'email' => 'test@admin.com',
            ],
            [
                'user_name' => 'Test Admin',
                // Explicit: without it the model auto-assigns the next id,
                // which was '0003' -- the one accounting@app.com below claims,
                // so seeding a fresh schema died on a duplicate mst_user PK.
                'user_id' => '0005',
                'password' => Hash::make('password'),
                'email_verified_at' => now(),
                'actived' => true,
            ]
        );

        if (Role::where('name', 'SuperAdmin')->exists()) {
            $testAdmin->syncRoles(['SuperAdmin']);
            $testAdmin->syncPermissions(Permission::all());
        }

        User::updateOrCreate(
            [
                'email' => 'accounting@app.com',
            ],
            [
                'user_name' => 'Accounting',
                'user_id' => '0003',
                'password' => Hash::make('password'),
                'email_verified_at' => now(),
            ]
        );
        
        if (Role::where('name', 'Accounting')->exists()) {
            User::where('email', 'accounting@app.com')->first()->assignRole('Accounting');
        }

        User::updateOrCreate(
            [
                'email' => 'finance@app.com',
            ],
            [
                'user_name' => 'Finance',
                'user_id' => '0004',
                'password' => Hash::make('password'),
                'email_verified_at' => now(),
            ]
        );
        
        if (Role::where('name', 'Finance')->exists()) {
            User::where('email', 'finance@app.com')->first()->assignRole('Finance');
        }

        // // Ambil data dari database lama
        // $oldData = DB::connection('dblama')->table('mst_user')->get();
        // // Masukkan data ke database baru
        // foreach ($oldData as $data) {
        //     User::insert(array_merge(
        //         (array) $data, // Data yang diambil dari database lama
        //     ));
        // }
    }
}
