<?php

namespace Tests\Feature;

use App\Livewire\Pages\ChartOfAccount\ChartOfAccount;
use App\Livewire\Pages\JournalTransaction\JT;
use App\Livewire\Pages\JournalValidation\JV;
use App\Support\Accounts;
use App\Support\Journals;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Crypt;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;
use Livewire\Livewire;
use Tests\Concerns\ActsAsAccountingRoles;
use Tests\Concerns\SeedsBooks;
use Tests\TestCase;

/** QA BUG-060/062/142/143/145/146: COA rules, line validation and the maker-checker workflow. */
class QaAccJournalCoaTest extends TestCase
{
    use ActsAsAccountingRoles, RefreshDatabase, SeedsBooks;

    protected function setUp(): void
    {
        parent::setUp();
        $this->seedBooks(2026, 10);
        $this->installRoles();
        $this->users = [
            'U1' => $this->userAs('U1', 'Accounting'),
            'ACC' => $this->userAs('ACC', 'Accounting'),
            'FM' => $this->userAs('FM', 'FinanceManager'),
            'AUD' => $this->userAs('AUD', 'Auditor'),
        ];
    }

    private array $users = [];

    private function u(string $id): \App\Models\User
    {
        return $this->users[$id];
    }

    private function child(string $code, string $parent, int $group = 1): void
    {
        DB::table('mst_account')->insert(['acc_code' => $code, 'acc_name' => "Akun $code", 'acc_group' => $group, 'acc_currency' => 'Rp',
            'parent_code' => $parent, 'is_active' => true, 'normal_balance' => 'D', 'created_at' => now(), 'updated_at' => now()]);
    }

    // ---- BUG-060: no hard delete --------------------------------------

    public function test_deactivate_is_refused_with_reasons_when_account_has_balance(): void
    {
        $this->book('J1', '2026-10-02', [['1101', 500, 0], ['4101', 0, 500]]);
        $acc = $this->u('ACC');

        Livewire::actingAs($acc)->test(ChartOfAccount::class)
            ->call('deactivate', Crypt::encrypt('1101'))
            ->assertSet('blockedCode', '1101')
            ->assertSee('Saldo akun belum nol');

        $this->assertTrue((bool) DB::table('mst_account')->where('acc_code', '1101')->value('is_active'));
    }

    public function test_unused_account_is_deactivated_not_deleted_and_audited(): void
    {
        $acc = $this->u('ACC');

        Livewire::actingAs($acc)->test(ChartOfAccount::class)->call('deactivate', Crypt::encrypt('2101'))->assertSet('blockers', []);

        $this->assertDatabaseHas('mst_account', ['acc_code' => '2101', 'is_active' => false]);
        $this->assertDatabaseHas('acc_audit_log', ['event' => 'coa.deactivate', 'subject' => '2101', 'user_id' => 'ACC']);
        $this->assertFalse(method_exists(ChartOfAccount::class, 'delete'));
    }

    public function test_auditor_cannot_deactivate(): void
    {
        Livewire::actingAs($this->u('AUD'))->test(ChartOfAccount::class)
            ->call('deactivate', Crypt::encrypt('2101'))->assertForbidden();
    }

    // ---- BUG-142: parent / normal balance / active ----------------------

    public function test_coa_form_saves_parent_normal_balance_and_status(): void
    {
        $acc = $this->u('ACC');
        DB::table('mst_account')->insert(['acc_code' => '1100', 'acc_name' => 'Kas & Bank', 'acc_group' => 1, 'acc_currency' => 'Rp',
            'is_active' => true, 'normal_balance' => 'D', 'created_at' => now(), 'updated_at' => now()]);

        Livewire::actingAs($acc)->test(ChartOfAccount::class)
            ->call('add')
            ->set('coa.acc_code', '1103')->set('coa.acc_name', 'Kas Kecil')
            ->set('coa.acc_group', 1)->set('coa.parent_code', '1100')
            ->set('coa.normal_balance', 'D')->set('coa.is_cash', true)
            ->call('save')->assertHasNoErrors();

        $this->assertDatabaseHas('mst_account', ['acc_code' => '1103', 'parent_code' => '1100', 'normal_balance' => 'D', 'is_active' => true, 'is_cash' => true]);
    }

    public function test_parent_must_be_same_group_and_without_postings(): void
    {
        $this->book('J1', '2026-10-02', [['1101', 500, 0], ['4101', 0, 500]]);

        $this->assertNotEmpty(Accounts::parentProblems('1103', '4101', 1));         // other group
        $this->assertNotEmpty(Accounts::parentProblems('1103', '1101', 1));         // has postings
        $this->assertNotEmpty(Accounts::parentProblems('1101', '1101', 1));         // itself
        $this->assertSame([], Accounts::parentProblems('1103', '1102', 1));
    }

    public function test_normal_balance_is_locked_once_used(): void
    {
        $this->book('J1', '2026-10-02', [['1101', 500, 0], ['4101', 0, 500]]);

        Livewire::actingAs($this->u('ACC'))->test(ChartOfAccount::class)
            ->call('add', Crypt::encrypt('1101'))
            ->set('coa.normal_balance', 'K')
            ->call('save')->assertHasErrors('coa.normal_balance');
    }

    // ---- BUG-143 / 145 / 146: what a journal line may post to -------------

    public function test_parent_inactive_and_control_accounts_are_not_postable(): void
    {
        $this->child('1101A', '1101');
        DB::table('mst_account')->where('acc_code', '1201')->update(['is_control' => true]);
        DB::table('mst_account')->where('acc_code', '2101')->update(['is_active' => false]);

        $this->assertStringContainsString('akun induk', implode(' ', Accounts::postingProblems('1101')));
        $this->assertStringContainsString('akun kontrol', implode(' ', Accounts::postingProblems('1201')));
        $this->assertSame([], Accounts::postingProblems('1201', manual: false));
        $this->assertStringContainsString('tidak aktif', implode(' ', Accounts::postingProblems('2101')));
        $this->assertSame([], Accounts::postingProblems('1101A'));
    }

    public function test_journal_line_rejects_negative_amount_and_empty_description(): void
    {
        $this->book('D1', '2026-10-02', [], 'draft', head: ['trans_user' => 'ACC']);
        $acc = $this->u('ACC');

        $c = Livewire::actingAs($acc)->test(JT::class)->call('detail', Crypt::encrypt('D1'))
            ->set('jtdetail.acc_code', '1101')->set('jtdetail.div_code', 'SD')
            ->set('jtdetail.description', '')->set('jtdetail.debetnew', -500)->set('jtdetail.creditnew', 0)
            ->call('saveDetail')
            ->assertHasErrors(['jtdetail.description', 'jtdetail.debetnew']);
        $this->assertSame(0, DB::table('trans_detail')->where('trans_no', 'D1')->count());

        $c->set('jtdetail.acc_code', '1201')->set('jtdetail.description', 'Koreksi piutang')->set('jtdetail.debetnew', 500)
            ->call('saveDetail');
        DB::table('mst_account')->where('acc_code', '1201')->update(['is_control' => true]);
        $c->call('saveDetail')->assertHasErrors('jtdetail.acc_code');
    }

    public function test_auto_posting_to_parent_account_is_refused(): void
    {
        $this->child('1102A', '1102');
        config(['app.sekoci_internal_key' => 'k']);
        \Illuminate\Support\Facades\Http::fake(['*' => \Illuminate\Support\Facades\Http::response(['data' => [['feature_key' => 'accounting', 'enabled' => true]]])]);

        $this->withHeaders(['X-Internal-Key' => 'k'])->postJson('/api/internal/journals', [
            'school_id' => '5006c011-b21e-42ad-9fb5-c07de8d99f74', 'source' => 'billing', 'reference' => 'billing:payment:x',
            'trans_date' => '2026-10-01', 'description' => 'Bayar',
            'lines' => [
                ['acc_code' => '1102', 'div_code' => 'SD', 'debit_idr' => 100, 'credit_idr' => 0],
                ['acc_code' => '1201', 'div_code' => 'SD', 'debit_idr' => 0, 'credit_idr' => 100],
            ],
        ])->assertStatus(422);
        $this->assertDatabaseHas('acc_auto_postings', ['reference' => 'billing:payment:x', 'status' => 'failed']);
    }

    // ---- BUG-062: maker-checker workflow ----------------------------------

    public function test_maker_submits_checker_approves(): void
    {
        $this->book('D1', '2026-10-02', [['1101', 500, 0], ['4101', 0, 500]], 'draft', head: ['trans_user' => 'ACC']);
        $maker = $this->u('ACC');
        $checker = $this->u('FM');

        Livewire::actingAs($maker)->test(JT::class)->call('submitJournal', 'D1')->assertSet('workflowError', null);
        $this->assertSame('pending', DB::table('trans_head')->where('trans_no', 'D1')->value('status'));

        // The maker cannot approve their own journal even with the permission.
        Livewire::actingAs($maker)->test(JV::class)->call('approveJournal', 'D1')
            ->assertSet('workflowError', fn ($e) => str_contains((string) $e, 'maker-checker'));
        $this->assertSame('pending', DB::table('trans_head')->where('trans_no', 'D1')->value('status'));

        Livewire::actingAs($checker)->test(JV::class)->assertSee('D1')->call('approveJournal', 'D1')->assertSet('workflowError', null);
        $this->assertSame('posted', DB::table('trans_head')->where('trans_no', 'D1')->value('status'));
        $this->assertDatabaseHas('acc_audit_log', ['event' => 'journal.approve', 'subject' => 'D1', 'user_id' => 'FM']);
    }

    public function test_finance_manager_cannot_submit_and_auditor_cannot_approve(): void
    {
        $this->book('D1', '2026-10-02', [['1101', 500, 0], ['4101', 0, 500]], 'pending', head: ['trans_user' => 'ACC']);

        Livewire::actingAs($this->u('FM'))->test(JT::class)->call('submitJournal', 'D1')->assertForbidden();
        Livewire::actingAs($this->u('AUD'))->test(JV::class)->call('approveJournal', 'D1')->assertForbidden();
    }

    public function test_reject_returns_to_draft(): void
    {
        $this->book('D1', '2026-10-02', [['1101', 500, 0], ['4101', 0, 500]], 'pending', head: ['trans_user' => 'ACC', 'status_by' => 'ACC']);

        Livewire::actingAs($this->u('FM'))->test(JV::class)->call('rejectJournal', 'D1')->assertSet('workflowError', null);
        $this->assertSame('draft', DB::table('trans_head')->where('trans_no', 'D1')->value('status'));
    }

    public function test_approval_rechecks_lines(): void
    {
        $this->book('D1', '2026-10-02', [['1201', 500, 0], ['4101', 0, 500]], 'pending', head: ['trans_user' => 'ACC']);
        DB::table('mst_account')->where('acc_code', '1201')->update(['is_control' => true]);

        $this->expectException(ValidationException::class);
        Journals::transition('D1', 'approve', 'FM');
    }

    public function test_reversal_from_ui_books_mirror_and_blocks_double_reversal(): void
    {
        $this->book('P1', '2026-10-02', [['1101', 500, 0], ['4101', 0, 500]], 'posted', head: ['trans_user' => 'ACC']);
        $fm = $this->u('FM');

        $c = Livewire::actingAs($fm)->test(JT::class)->call('reverseJournal', 'P1')->assertSet('workflowError', null);
        $rv = $c->get('lastReversal');
        $this->assertNotNull($rv);
        $this->assertSame('reversed', DB::table('trans_head')->where('trans_no', 'P1')->value('status'));
        $this->assertEquals(500, DB::table('trans_detail')->where('trans_no', $rv)->where('acc_code', '4101')->value('debet'));

        $c->call('reverseJournal', $rv)->assertSet('workflowError', fn ($e) => str_contains((string) $e, 'jurnal pembalik'));
    }
}
