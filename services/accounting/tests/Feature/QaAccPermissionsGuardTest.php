<?php

namespace Tests\Feature;

use App\Livewire\Pages\HistoryJournalTransaction\HJT;
use App\Livewire\Pages\RoleAndPermission\Permissions;
use App\Livewire\Pages\TransactionType\Transaction;
use App\Support\AccountingRoles;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Crypt;
use Livewire\Livewire;
use Spatie\Permission\Models\Role;
use Tests\Concerns\ActsAsAccountingRoles;
use Tests\Concerns\SeedsBooks;
use Tests\TestCase;

/**
 * QA round 2: the permission page can not be used to hand out the reopen
 * approval (maker-checker) or rewrite the SuperAdmin role, and every
 * Livewire action passes the central guard.
 */
class QaAccPermissionsGuardTest extends TestCase
{
    use ActsAsAccountingRoles, RefreshDatabase, SeedsBooks;

    protected function setUp(): void
    {
        parent::setUp();
        $this->seedBooks();
        $this->installRoles();
        $this->withoutMiddleware(\App\Http\Middleware\TenantSchema::class);
        Role::findOrCreate('SuperAdmin', 'web');
    }

    private function page(string $role)
    {
        return Livewire::withQueryParams(['foo' => Crypt::encrypt(Role::findByName($role)->id)])->test(Permissions::class);
    }

    public function test_admin_can_not_grant_reopen_approval(): void
    {
        $this->actingAs($this->userAs('A1', 'Admin'));
        $this->page('Accounting')->call('abc', 'period_reopen', 'edit');

        $this->assertFalse(Role::findByName('Accounting')->hasPermissionTo(AccountingRoles::REOPEN_APPROVE));
    }

    public function test_admin_save_keeps_reopen_approval_as_it_was(): void
    {
        $this->actingAs($this->userAs('A1', 'Admin'));
        // Admin unticks everything on Finance Manager and adds one permission.
        $this->page('FinanceManager')
            ->set('permission_user', ['general_ledger' => ['view' => true]])
            ->call('simpan');

        $fm = Role::findByName('FinanceManager');
        $this->assertTrue($fm->hasPermissionTo(AccountingRoles::REOPEN_APPROVE), 'SuperAdmin-only permission kept');
        $this->assertTrue($fm->hasPermissionTo('viewgeneral_ledger'));
        $this->assertFalse($fm->hasPermissionTo('editjournal_validation'));
    }

    public function test_superadmin_can_grant_reopen_approval(): void
    {
        $this->actingAs($this->userAs('S1', 'SuperAdmin'));
        $this->page('Accounting')->call('abc', 'period_reopen', 'edit');

        $this->assertTrue(Role::findByName('Accounting')->hasPermissionTo(AccountingRoles::REOPEN_APPROVE));
    }

    public function test_admin_can_not_edit_superadmin_role(): void
    {
        $this->actingAs($this->userAs('A1', 'Admin'));
        $this->page('SuperAdmin')->call('abc', 'periode', 'view')->assertForbidden();

        $this->assertFalse(Role::findByName('SuperAdmin')->hasPermissionTo('viewperiode'));
    }

    public function test_bare_permissions_url_goes_back_to_roles(): void
    {
        $this->actingAs($this->userAs('A1', 'Admin'));
        Livewire::test(Permissions::class)->assertRedirect(route('role'));
    }

    public function test_guard_blocks_write_without_permission(): void
    {
        $this->actingAs($this->userAs('X1', 'Auditor'));
        Livewire::test(Transaction::class)->call('save')->assertForbidden();
    }

    public function test_history_journal_is_read_only(): void
    {
        $this->actingAs($this->userAs('S1', 'SuperAdmin'));
        Livewire::test(HJT::class)->call('add')->assertForbidden();
    }
}
