<?php

use App\Support\AccountingRoles;
use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

/**
 * QA BUG-061/062: accounting RBAC per PRD §7.2 for schools that are already
 * provisioned (the baseline seeders only run on an empty schema).
 *  - adds the journal_validation / trial_balance / consolidation / auto_posting
 *    / import_data modules and their permissions;
 *  - creates FinanceManager, Auditor and Executive;
 *  - narrows the legacy Finance role (kepala sekolah/yayasan) to reports only.
 * Admin and Accounting only GAIN the new permissions; hand-made grants stay.
 *
 * A schema without any permission yet (public, or a brand-new tenant that is
 * about to be seeded) is skipped: RoleAndPermissionSeeder installs the same
 * spec there.
 */
return new class extends Migration
{
    public function up(): void
    {
        if (! Schema::hasTable('permissions') || ! DB::table('permissions')->exists()) {
            return;
        }
        AccountingRoles::install(additive: true);
    }

    public function down(): void
    {
        // Roles/permissions are data an admin may have built on; leave them.
    }
};
