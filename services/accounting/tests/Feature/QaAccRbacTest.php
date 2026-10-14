<?php

namespace Tests\Feature;

use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\Concerns\ActsAsAccountingRoles;
use Tests\Concerns\SeedsBooks;
use Tests\TestCase;

/** QA BUG-061: accounting RBAC per PRD role (Accounting / FinanceManager / Auditor / Executive). */
class QaAccRbacTest extends TestCase
{
    use ActsAsAccountingRoles, RefreshDatabase, SeedsBooks;

    protected function setUp(): void
    {
        parent::setUp();
        $this->seedBooks();
        $this->installRoles();
        // Tables live in the test default schema; tenancy has its own tests.
        $this->withoutMiddleware(\App\Http\Middleware\TenantSchema::class);
    }

    public function test_role_permission_matrix(): void
    {
        $acc = $this->userAs('ACC', 'Accounting');
        $fm = $this->userAs('FM', 'FinanceManager');
        $aud = $this->userAs('AUD', 'Auditor');
        $exe = $this->userAs('EXE', 'Executive');

        $this->assertTrue($acc->can('createjournal_transaction'));
        $this->assertTrue($acc->can('editjournal_validation'));
        $this->assertTrue($acc->can('createend_of_month'));
        $this->assertTrue($acc->can('createimport_data'));
        $this->assertFalse($acc->can('editrole'));

        $this->assertTrue($fm->can('viewjournal_transaction'));
        $this->assertTrue($fm->can('editjournal_validation'));
        $this->assertFalse($fm->can('createjournal_transaction'));
        $this->assertFalse($fm->can('deletechart_of_account'));

        $this->assertTrue($aud->can('viewgeneral_ledger'));
        $this->assertTrue($aud->can('viewtrial_balance'));
        foreach (['create', 'edit', 'delete'] as $a) {
            foreach (['journal_transaction', 'journal_validation', 'chart_of_account', 'end_of_month', 'balance'] as $m) {
                $this->assertFalse($aud->can($a.$m), "auditor must not $a$m");
            }
        }

        $this->assertTrue($exe->can('viewfinancial_report'));
        $this->assertTrue($exe->can('viewconsolidation'));
        $this->assertFalse($exe->can('viewjournal_transaction'));
        $this->assertFalse($exe->can('viewchart_of_account'));
        $this->assertFalse($exe->can('viewgeneral_ledger'));
    }

    public function test_routes_follow_the_matrix(): void
    {
        $this->actingAs($this->userAs('EXE', 'Executive'))->get('/chartofaccount')->assertForbidden();
        $this->get('/journaltransaction')->assertForbidden();
        $this->get('/reportgeneralledger')->assertForbidden();
        $this->get('/summarygeneralledger')->assertForbidden();
    }

    public function test_auditor_opens_the_approval_queue_read_only(): void
    {
        $this->actingAs($this->userAs('AUD', 'Auditor'))->get('/journalvalidation')->assertOk()
            ->assertDontSee('wire:click="approveJournal', false);
    }

    public function test_journal_validation_page_opens_for_accounting(): void
    {
        $this->actingAs($this->userAs('ACC', 'Accounting'))->get('/journalvalidation')
            ->assertOk()->assertSee('Jurnal Menunggu Approval');
    }

    public function test_account_lookup_api_needs_login_and_permission(): void
    {
        $this->get('/api/search-accounts?q=11')->assertRedirect();
        $this->actingAs($this->userAs('EXE', 'Executive'))->get('/api/search-accounts?q=11')->assertForbidden();
    }

    public function test_account_lookup_api_for_accounting(): void
    {
        $this->actingAs($this->userAs('ACC', 'Accounting'))->get('/api/search-accounts?q=11')->assertOk();
    }
}
