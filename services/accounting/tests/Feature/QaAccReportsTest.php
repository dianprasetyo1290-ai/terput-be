<?php

namespace Tests\Feature;

use App\Http\Middleware\TenantSchema;
use App\Livewire\Pages\EndOfMonth\EOM;
use App\Livewire\Pages\Import\Import;
use App\Support\Importer;
use App\Support\Statements;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Http\UploadedFile;
use Illuminate\Support\Facades\DB;
use Livewire\Livewire;
use Tests\Concerns\ActsAsAccountingRoles;
use Tests\Concerns\SeedsBooks;
use Tests\TestCase;

/** QA BUG-059/063/064/065/140/147/148: EOM checks, ledger reports, import, drilldown, dashboard. */
class QaAccReportsTest extends TestCase
{
    use ActsAsAccountingRoles, RefreshDatabase, SeedsBooks;

    private array $users = [];

    protected function setUp(): void
    {
        parent::setUp();
        $this->seedBooks(2026, 10);
        $this->installRoles();
        foreach (['U1' => 'Accounting', 'ACC' => 'Accounting', 'AUD' => 'Auditor', 'EXE' => 'Executive'] as $id => $role) {
            $this->users[$id] = $this->userAs($id, $role);
        }
        $this->withoutMiddleware(TenantSchema::class);
    }

    private function opening(string $acc, float $d, float $c, string $div = 'SD', int $year = 2026): void
    {
        DB::table('mst_saldo_news')->insert(['acc_code' => $acc, 'div_code' => $div, 'periode' => 0, 'tahun' => $year,
            'debet' => $d, 'credit' => $c, 'created_at' => now(), 'updated_at' => now()]);
    }

    /** Opening 1.000 kas vs aset neto; October: SPP 300 received (via piutang), salary 100, a draft. */
    private function scenario(): void
    {
        $this->opening('1101', 1000, 0);
        $this->opening('3101', 0, 1000);
        $this->book('P1', '2026-10-02', [['1201', 300, 0], ['4101', 0, 300]]);
        $this->book('P2', '2026-10-03', [['1102', 300, 0], ['1201', 0, 300]]);
        $this->book('P3', '2026-10-04', [['6101', 100, 0, 'SMP'], ['1101', 0, 100, 'SMP']]);
        $this->book('P4', '2026-10-05', [['1102', 50, 0], ['1101', 0, 50]]); // kas -> bank transfer
        $this->book('D1', '2026-10-06', [['6101', 999, 0], ['1101', 0, 999]], 'draft');
    }

    // ---- BUG-147 ------------------------------------------------------------

    public function test_eom_blocks_on_drafts_and_failed_postings_and_rechecks_server_side(): void
    {
        $this->scenario();
        DB::table('acc_auto_postings')->insert(['posting_date' => '2026-10-07', 'kind' => 'billing', 'status' => 'failed',
            'reference' => 'billing:bill:x', 'message' => 'akun belum dipetakan']);

        $c = Livewire::actingAs($this->users['ACC'])->test(EOM::class)->assertSet('allPassed', false)
            ->assertSee('D1 (draft)')->assertSee('billing:bill:x');
        $c->call('transfer');
        $this->assertSame(10, (int) DB::table('parameter')->value('period'));
        $this->assertSame(5, DB::table('trans_head')->count());
    }

    public function test_eom_closes_once_and_refuses_a_stale_second_run(): void
    {
        $this->opening('1101', 1000, 0);
        $this->opening('3101', 0, 1000);
        $this->book('P1', '2026-10-02', [['1101', 300, 0], ['4101', 0, 300]]);

        $c = Livewire::actingAs($this->users['ACC'])->test(EOM::class)->assertSet('allPassed', true);
        $stale = Livewire::actingAs($this->users['ACC'])->test(EOM::class);
        $c->call('transfer');
        $this->assertSame(11, (int) DB::table('parameter')->value('period'));
        $this->assertDatabaseHas('acc_audit_log', ['event' => 'period.close', 'subject' => '2026-10']);

        $stale->call('transfer');
        $this->assertSame(11, (int) DB::table('parameter')->value('period'));
    }

    public function test_eom_with_no_transactions_can_still_close(): void
    {
        Livewire::actingAs($this->users['ACC'])->test(EOM::class)->assertSet('allPassed', true)->call('transfer');
        $this->assertSame(11, (int) DB::table('parameter')->value('period'));
    }

    public function test_eom_requires_permission(): void
    {
        Livewire::actingAs($this->users['AUD'])->test(EOM::class)->call('transfer')->assertForbidden();
    }

    // ---- BUG-063 ------------------------------------------------------------

    public function test_trial_balance_counts_posted_only_and_balances(): void
    {
        $this->scenario();
        $tb = Statements::trialBalance(2026, 10);

        $this->assertTrue($tb['totals']['balanced']);
        $this->assertEquals(1000, $tb['totals']['open_debit']);
        $this->assertEquals(750, $tb['totals']['debit']);         // drafts excluded
        $kas = $tb['rows']->firstWhere('acc_code', '1101');
        $this->assertEquals(850, $kas->end_debit);

        $smp = Statements::trialBalance(2026, 10, 'SMP');
        $this->assertEquals(100, $smp['rows']->firstWhere('acc_code', '6101')->end_debit);

        $this->actingAs($this->users['AUD'])->get('/report/trial-balance')->assertOk()->assertSee('Seimbang')->assertSee('SEMENTARA');
    }

    // ---- BUG-064 ------------------------------------------------------------

    public function test_financial_statements_tie_out(): void
    {
        $this->scenario();
        $s = Statements::build(2026, 10);

        $this->assertTrue($s['provisional']);
        $this->assertEquals(300, $s['activity']['total_revenue']['period']);
        $this->assertEquals(100, $s['activity']['total_expense']['period']);
        $this->assertEquals(200, $s['activity']['surplus']['period']);

        $this->assertEquals(1200, $s['position']['total_assets']['amount']);
        $this->assertEquals(1200, $s['position']['total_net_assets']['amount']);
        $this->assertEquals(0, $s['position']['difference']);
        $this->assertEquals(1000, $s['position']['total_assets']['prev']);

        $cf = $s['cash_flow'];
        $this->assertEquals(1000, $cf['opening']);
        $this->assertEquals(300, $cf['total_receipts']);  // collected receivable, not the bill
        $this->assertEquals(-100, $cf['total_payments']);
        $this->assertEquals(1200, $cf['ending']);
        $this->assertEquals(0, $cf['difference']);        // kas->bank transfer cancels out
        $this->assertSame('1201', $cf['receipts']->first()->acc_code);

        $this->assertEquals(1000, $s['net_assets']['opening']);
        $this->assertEquals(200, $s['net_assets']['surplus']);
        $this->assertEquals(1200, $s['net_assets']['ending']);
        $this->assertEquals(0, $s['net_assets']['difference']);
    }

    public function test_statements_page_for_executive(): void
    {
        $this->scenario();
        $this->actingAs($this->users['EXE'])->get('/report/statements')->assertOk()
            ->assertSee('Laporan Aktivitas')->assertSee('Pendapatan SPP');
    }

    // ---- BUG-065 ------------------------------------------------------------

    public function test_consolidation_eliminates_interunit_accounts(): void
    {
        DB::table('mst_account')->insert([
            ['acc_code' => '1301', 'acc_name' => 'Piutang antar unit', 'acc_group' => 1, 'acc_currency' => 'Rp', 'is_interunit' => true, 'normal_balance' => 'D', 'is_active' => true, 'created_at' => now(), 'updated_at' => now()],
            ['acc_code' => '2301', 'acc_name' => 'Utang antar unit', 'acc_group' => 2, 'acc_currency' => 'Rp', 'is_interunit' => true, 'normal_balance' => 'K', 'is_active' => true, 'created_at' => now(), 'updated_at' => now()],
        ]);
        // SD lends SMP 200 cash.
        $this->book('IU1', '2026-10-02', [['1301', 200, 0, 'SD'], ['1101', 0, 200, 'SD'], ['1101', 200, 0, 'SMP'], ['2301', 0, 200, 'SMP']]);

        $c = Statements::consolidation(2026, 10);
        $this->assertEquals(0, $c['interunit_residual']);
        $r = $c['rows']->firstWhere('acc_code', '1301');
        $this->assertEquals(200, $r->units['SD']);
        $this->assertEquals(-200, $r->elimination);
        $this->assertEquals(0, $r->consolidated);
        $this->assertEquals(0, $c['summary']['assets']);   // cash just moved between units

        // A one-sided inter-unit entry is flagged.
        $this->book('IU2', '2026-10-03', [['1301', 50, 0, 'SD'], ['4101', 0, 50, 'SD']]);
        $this->assertEquals(50, Statements::consolidation(2026, 10)['interunit_residual']);
        $this->actingAs($this->users['EXE'])->get('/report/consolidation')->assertOk()->assertSee('tidak saling menghapus');
    }

    // ---- BUG-059 ------------------------------------------------------------

    public function test_coa_import_validates_all_rows_then_commits(): void
    {
        $csv = "acc_code;acc_name;acc_group;parent_code;normal_balance;is_cash\n1100;Kas dan Bank;1;;;0\n1103;Kas Kecil;1;1100;D;1\n4102;Uang Gedung;4;;;\n";
        [$rows] = Importer::parse($csv, ['acc_code', 'acc_name', 'acc_group']);

        $dry = Importer::coa($rows, true);
        $this->assertSame([], $dry['errors']);
        $this->assertDatabaseMissing('mst_account', ['acc_code' => '1103']);

        $res = Importer::coa($rows, false);
        $this->assertTrue($res['committed']);
        $this->assertDatabaseHas('mst_account', ['acc_code' => '1103', 'parent_code' => '1100', 'is_cash' => true]);
        $this->assertDatabaseHas('mst_account', ['acc_code' => '4102', 'normal_balance' => 'K']);

        [$bad] = Importer::parse("acc_code,acc_name,acc_group,parent_code\n1104,Bank X,1,4102\n1105,,9,\n", ['acc_code', 'acc_name', 'acc_group']);
        $res = Importer::coa($bad, false);
        $this->assertCount(3, $res['errors']);
        $this->assertDatabaseMissing('mst_account', ['acc_code' => '1104']);
    }

    public function test_opening_balance_import_must_balance_and_replaces_the_year(): void
    {
        $this->opening('1102', 777, 0);
        [$rows] = Importer::parse("acc_code,div_code,debit,credit\n1101,SD,\"1.500.000,00\",0\n1201,SMP,500000,0\n3101,SD,0,2000000\n", Importer::OPENING_COLUMNS);
        $res = Importer::opening($rows, 2026, false);
        $this->assertSame([], $res['errors']);
        $this->assertEquals(1500000, DB::table('mst_saldo_news')->where(['acc_code' => '1101', 'div_code' => 'SD', 'periode' => 0, 'tahun' => 2026])->value('debet'));
        $this->assertEquals(0, DB::table('mst_saldo_news')->where(['acc_code' => '1102', 'periode' => 0])->value('debet'));
        $this->assertTrue(Statements::trialBalance(2026, 10)['totals']['balanced']);

        [$bad] = Importer::parse("acc_code,div_code,debit,credit\n1101,SD,100,0\n3101,XX,0,90\n9999,SD,-5,0\n", Importer::OPENING_COLUMNS);
        $res = Importer::opening($bad, 2026, false);
        $this->assertNotEmpty($res['errors']);
        $this->assertFalse($res['committed']);
        $this->assertEquals(1500000, DB::table('mst_saldo_news')->where(['acc_code' => '1101', 'div_code' => 'SD', 'periode' => 0])->value('debet'));
    }

    public function test_import_page_requires_permission_and_confirmation(): void
    {
        $this->opening('1101', 10, 0);
        $file = UploadedFile::fake()->createWithContent('saldo.csv', "acc_code,div_code,debit,credit\n1101,SD,100,0\n3101,SD,0,100\n");

        Livewire::actingAs($this->users['AUD'])->test(Import::class)->set('kind', 'opening')->set('file', $file)->call('import')->assertForbidden();

        $c = Livewire::actingAs($this->users['ACC'])->test(Import::class)->set('kind', 'opening')->set('file', $file)->call('import')
            ->assertSee('Centang konfirmasi');
        $this->assertEquals(10, DB::table('mst_saldo_news')->where(['acc_code' => '1101', 'periode' => 0])->value('debet'));
        $c->set('confirmReplace', true)->call('import')->assertSee('Import selesai');
        $this->assertEquals(100, DB::table('mst_saldo_news')->where(['acc_code' => '1101', 'periode' => 0])->value('debet'));
    }

    // ---- BUG-148 / BUG-140 ----------------------------------------------------

    public function test_journal_drilldown_open_and_closed(): void
    {
        $this->book('P1', '2026-10-02', [['1101', 300, 0], ['4101', 0, 300]], 'posted', head: ['source' => 'billing', 'reference' => 'billing:payment:2026-10-02:SD']);
        $this->book('H1', '2026-09-02', [['1101', 50, 0], ['4101', 0, 50]], 'posted', true);

        $this->actingAs($this->users['AUD'])->get('/journal/P1')->assertOk()->assertSee('billing:payment:2026-10-02:SD')->assertSee('Pendapatan SPP');
        $this->get('/journal/H1?date=2026-09-02')->assertOk()->assertSee('periode sudah ditutup');
        $this->get('/journal/NOPE')->assertNotFound();
    }

    public function test_general_ledger_detail_links_to_journal_and_skips_drafts(): void
    {
        $this->book('P1', '2026-10-02', [['1101', 300, 0], ['4101', 0, 300]]);
        $this->book('D1', '2026-10-03', [['1101', 999, 0], ['4101', 0, 999]], 'draft');

        $rows = getLedgerDetails(2026, '1101', null, null, null, null, true);
        $this->assertSame(['P1'], $rows->pluck('trans_no')->all());
        $view = file_get_contents(resource_path('views/livewire/pages/report/general-ledger/report-general-ledger.blade.php'));
        $this->assertStringContainsString("url('journal/'", $view);
    }

    public function test_dashboard_shows_kpis_status_and_failed_postings(): void
    {
        $this->scenario();
        DB::table('trans_head')->where('trans_no', 'D1')->update(['status' => 'pending']);
        DB::table('acc_auto_postings')->insert(['posting_date' => '2026-10-07', 'kind' => 'billing', 'status' => 'failed',
            'reference' => 'billing:bill:x', 'message' => 'akun belum dipetakan']);

        $this->actingAs($this->users['EXE'])->get('/home')->assertOk()
            ->assertSee('Ringkasan Keuangan')->assertSee('Rp 300<', false)
            ->assertSee('Menunggu approval: 1')->assertSee('1 gagal')->assertSee('akun belum dipetakan');
    }

    public function test_auto_posting_page(): void
    {
        DB::table('acc_auto_postings')->insert(['posting_date' => '2026-10-07', 'kind' => 'billing', 'status' => 'failed',
            'reference' => 'billing:bill:x', 'message' => 'akun belum dipetakan']);
        $this->actingAs($this->users['AUD'])->get('/autoposting')->assertOk()->assertSee('billing:bill:x');
    }
}
