<?php

namespace Tests\Concerns;

use App\Models\User;
use App\Support\AccountingRoles;
use Database\Seeders\ModulesSeeder;

/** Seeds the real module/permission grid and hands out users per role. */
trait ActsAsAccountingRoles
{
    protected function installRoles(): void
    {
        $this->seed(ModulesSeeder::class);
        AccountingRoles::install();
    }

    protected function userAs(string $id, string $role): User
    {
        $user = User::query()->forceCreate([
            'user_id' => $id, 'user_name' => "User $id", 'email' => strtolower($id).'@uji.test', 'password' => bcrypt('secret'),
        ]);
        $user->assignRole($role);

        return $user;
    }
}
