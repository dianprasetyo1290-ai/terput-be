<?php

namespace App\Support;

use App\Models\Modul;
use Spatie\Permission\Models\Permission;
use Spatie\Permission\Models\Role;
use Spatie\Permission\PermissionRegistrar;

/**
 * Accounting RBAC (PRD Bagian 1 §7.2 + menu 32-38), one place for the seeder,
 * the tenant migration and the tests.
 *
 * Permissions are "<action><module>" (create/edit/delete/view), as Exco always
 * had them. Workflow actions reuse that grid:
 *   - editjournal_transaction   submit a draft for approval (maker)
 *   - editjournal_validation    approve / reject a pending journal, book a
 *                               reversal of a posted one (checker; never the
 *                               journal's own maker, see Journals::transition)
 *   - createend_of_month        run End of Month
 *   - createimport_data         import COA / opening balances
 *
 * Roles (Sekoci active_role -> role, see SsoController::ROLE_MAP):
 *   SuperAdmin      superadmin          everything (Gate::before)
 *   Admin           admin               everything in its school's books
 *   Accounting      akuntan/accounting  everything but user/role/permission admin
 *   FinanceManager  finance_manager     read the books + approve/reverse journals
 *   Auditor         auditor             read-only
 *   Executive       kepala_sekolah,     dashboard + financial reports/consolidation
 *                   kepala_yayasan      only (no COA, journals or full ledger)
 */
final class AccountingRoles
{
    public const ACTIONS = ['create', 'edit', 'delete', 'view'];

    /** Modules added after the original Exco seed. */
    public const NEW_MODULES = [
        'journal_validation' => 'Journal Validation',
        'trial_balance' => 'Trial Balance',
        'consolidation' => 'Consolidation',
        'auto_posting' => 'Auto Posting',
        'import_data' => 'Import Data',
        'journal_template' => 'Journal Template & Recurring',
        'period_reopen' => 'Period Reopen',
    ];

    /**
     * Approving a period reopen (QA BUG-178) is reserved for FinanceManager
     * (and SuperAdmin via Gate::before): Admin/Accounting may only request it,
     * so whoever closes the books can never reopen them alone.
     */
    public const REOPEN_APPROVE = 'editperiod_reopen';

    /** Pages a read-only reviewer (Finance Manager, Auditor) may open. */
    public const READ_MODULES = [
        'journal_transaction', 'history_journal', 'history_journal_detail', 'journal_validation',
        'general_ledger', 'trial_balance', 'financial_report', 'consolidation', 'chart_of_account',
        'balance', 'auto_posting', 'end_of_month', 'end_of_year', 'journal_template', 'period_reopen',
    ];

    /** Pages kepala sekolah / kepala yayasan may open (besides the dashboard). */
    public const EXECUTIVE_MODULES = ['financial_report', 'consolidation'];

    /** Role name => permission names. */
    public static function spec(): array
    {
        $view = fn (array $modules) => array_map(fn ($m) => "view$m", $modules);
        $all = array_values(array_diff(Permission::pluck('name')->all(), [self::REOPEN_APPROVE]));

        return [
            'Admin' => $all,
            'Accounting' => array_values(array_filter($all, fn ($p) => ! preg_match('/user|role|permission|setting_website/', $p))),
            'FinanceManager' => array_merge($view(self::READ_MODULES), ['editjournal_validation', 'createperiod_reopen', self::REOPEN_APPROVE]),
            'Auditor' => $view(self::READ_MODULES),
            'Executive' => $view(self::EXECUTIVE_MODULES),
            // Legacy name kepala sekolah/yayasan were mapped to before; kept so a
            // live session holding it gets the same narrow rights.
            'Finance' => $view(self::EXECUTIVE_MODULES),
        ];
    }

    /** Create the new modules and their permissions (idempotent). */
    public static function ensurePermissions(): void
    {
        foreach (self::NEW_MODULES as $name => $display) {
            Modul::firstOrCreate(['name' => $name], ['display_name' => $display]);
        }
        foreach (Modul::pluck('name') as $module) {
            foreach (self::ACTIONS as $action) {
                Permission::findOrCreate($action.$module, 'web');
            }
        }
        app(PermissionRegistrar::class)->forgetCachedPermissions();
    }

    /**
     * Apply the spec. $additive keeps whatever an admin granted Admin and
     * Accounting by hand (they only gain the new permissions); the narrow
     * roles are always synced exactly, since widening them is the bug.
     */
    public static function install(bool $additive = false): void
    {
        self::ensurePermissions();
        foreach (self::spec() as $role => $permissions) {
            $r = Role::findOrCreate($role, 'web');
            $permissions = array_values(array_intersect($permissions, Permission::pluck('name')->all()));
            if ($additive && in_array($role, ['Admin', 'Accounting'], true)) {
                $r->givePermissionTo($permissions);
            } else {
                $r->syncPermissions($permissions);
            }
        }
        Role::findOrCreate('SuperAdmin', 'web');
        app(PermissionRegistrar::class)->forgetCachedPermissions();
    }
}
