<?php

namespace Tests\Feature;

use App\Livewire\Pages\Import\Import;
use App\Livewire\Pages\JournalDetail\JournalDetail;
use App\Services\AutoJournalService;
use App\Support\ReceivableReconciliation;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Http;
use Illuminate\Validation\ValidationException;
use Livewire\Livewire;
use Tests\Concerns\ActsAsAccountingRoles;
use Tests\Concerns\SeedsBooks;
use Tests\TestCase;

/** QA BUG-148 (drilldown to billing source) and BUG-059 (receivable control check + sign-off). */
class QaAccSourcesTest extends TestCase
{
    use ActsAsAccountingRoles, RefreshDatabase, SeedsBooks;

    private const SCHOOL = '5006c011-b21e-42ad-9fb5-c07de8d99f74';

    protected function setUp(): void
    {
        parent::setUp();
        $this->seedBooks();
        $this->installRoles();
        $this->withoutMiddleware(\App\Http\Middleware\TenantSchema::class);
        config(['app.sekoci_internal_key' => 'k-123', 'app.billing_internal_url' => 'http://billing:8082']);
        session(['sekoci_school_id' => self::SCHOOL]);
    }

    public function test_source_refs_are_cleaned(): void
    {
        $this->assertSame(['payment' => ['p1', '7']], AutoJournalService::cleanSourceRefs(['payment' => ['p1', 7, 'p1', ''], 'Bad Type' => ['x'], 'invoice' => 'nolist']));
        $this->assertSame([], AutoJournalService::cleanSourceRefs('x'));
    }

    public function test_source_refs_are_stored_with_the_auto_posting(): void
    {
        $this->userAs('SYS_USER', 'Accounting');
        config(['app.identity_service_url' => 'http://identity:8081']);
        Http::fake(['identity:8081/*' => Http::response(['data' => [['feature_key' => 'accounting', 'enabled' => true]]])]);
        $this->withHeaders(['X-Internal-Key' => 'k-123'])->postJson('/api/internal/journals', [
            'school_id' => self::SCHOOL, 'source' => 'billing', 'reference' => 'billing:payments:2026-10-06',
            'trans_date' => '2026-10-06', 'description' => 'Pembayaran',
            'lines' => [['acc_code' => '1101', 'debit_idr' => 500, 'credit_idr' => 0], ['acc_code' => '4101', 'debit_idr' => 0, 'credit_idr' => 500]],
            'source_refs' => ['payment' => ['pay-9']],
        ])->assertStatus(201);
        $ids = json_decode((string) DB::table('acc_auto_postings')->where('reference', 'billing:payments:2026-10-06')->value('source_ids'), true);
        $this->assertSame(['payment' => ['pay-9']], $ids);
    }

    private function autoJournal(): void
    {
        $this->userAs('U1', 'Accounting');
        $this->book('BIL26-0001', '2026-10-05', [['1101', 300, 0], ['1201', 0, 300]], 'posted', false,
            ['trans_code' => 'BIL', 'source' => 'billing', 'reference' => 'billing:payments:2026-10-05']);
        DB::table('acc_auto_postings')->insert(['posting_date' => '2026-10-05', 'kind' => 'billing', 'status' => 'posted',
            'trans_no' => 'BIL26-0001', 'amount' => 300, 'reference' => 'billing:payments:2026-10-05',
            'source_ids' => json_encode(['payment' => ['pay-1', 'pay-2']])]);
    }

    public function test_drilldown_lists_billing_sources(): void
    {
        $this->autoJournal();
        Http::fake(['billing:8082/internal/journal-sources*' => Http::response(['data' => ['reference' => 'billing:payments:2026-10-05', 'items' => [
            ['type' => 'payment', 'id' => 'pay-1', 'number' => 'PAY-001', 'student_name' => 'Ani', 'nis' => '1001', 'unit' => 'SD', 'amount_idr' => 100, 'date' => '2026-10-05', 'status' => 'paid'],
            ['type' => 'payment', 'id' => 'pay-2', 'number' => 'PAY-002', 'student_name' => 'Budi', 'nis' => '1002', 'unit' => 'SD', 'amount_idr' => 200, 'date' => '2026-10-05', 'status' => 'paid'],
        ]], 'message' => '', 'errors' => null])]);

        $this->actingAs($this->userAs('AUD', 'Auditor'));
        Livewire::test(JournalDetail::class, ['transNo' => 'BIL26-0001'])
            ->assertSee('pay-1, pay-2')
            ->call('loadSources')
            ->assertSee('PAY-002')->assertSee('Budi')->assertSet('sourcesError', null);
        Http::assertSent(fn ($r) => $r->hasHeader('X-Internal-Key', 'k-123') && str_contains($r->url(), 'school_id='.self::SCHOOL));
    }

    public function test_drilldown_without_billing_config_says_so(): void
    {
        $this->autoJournal();
        config(['app.billing_internal_url' => '']);
        $this->actingAs($this->userAs('AUD', 'Auditor'));
        Livewire::test(JournalDetail::class, ['transNo' => 'BIL26-0001'])->call('loadSources')
            ->assertSee('belum dikonfigurasi')->assertSee('pay-1');
    }

    public function test_drilldown_survives_billing_errors(): void
    {
        $this->autoJournal();
        Http::fake(['billing:8082/*' => Http::response(['message' => 'boom'], 500)]);
        $this->actingAs($this->userAs('AUD', 'Auditor'));
        Livewire::test(JournalDetail::class, ['transNo' => 'BIL26-0001'])->call('loadSources')
            ->assertSee('Billing menolak permintaan');
    }

    private function controlOpening(float $amount): void
    {
        DB::table('mst_account')->where('acc_code', '1201')->update(['is_control' => true]);
        foreach ([['1201', $amount, 0], ['3101', 0, $amount]] as [$acc, $d, $c]) {
            DB::table('mst_saldo_news')->insert(['acc_code' => $acc, 'div_code' => 'SD', 'periode' => 0, 'tahun' => 2026, 'debet' => $d, 'credit' => $c]);
        }
    }

    public function test_reconciliation_matches_and_signs_off(): void
    {
        $this->controlOpening(150000000);
        Http::fake(['billing:8082/internal/receivables/summary*' => Http::response(['data' => ['outstanding_idr' => 150000000, 'students' => 420, 'as_of' => '2026-10-14']])]);
        $c = ReceivableReconciliation::check();
        $this->assertSame(150000000.0, $c['opening']);
        $this->assertSame(0.0, $c['difference']);

        $this->actingAs($this->userAs('ACC', 'Accounting'));
        Livewire::test(Import::class)->call('reconcile')->assertSee('420 siswa')
            ->call('signOff')->assertHasNoErrors()->assertSet('signOffMessage', fn ($m) => (bool) $m);
        $this->assertTrue(DB::table('acc_audit_log')->where('event', 'migration.signoff')->where('user_id', 'ACC')->exists());
    }

    public function test_difference_needs_explanation(): void
    {
        $this->controlOpening(150000000);
        Http::fake(['billing:8082/*' => Http::response(['data' => ['outstanding_idr' => 149000000]])]);
        $c = ReceivableReconciliation::check();
        $this->assertSame(1000000.0, $c['difference']);
        try {
            ReceivableReconciliation::signOff($c, '', 'U1');
            $this->fail('signed off a difference without a note');
        } catch (ValidationException) {
        }
        $this->userAs('U1', 'Accounting');
        ReceivableReconciliation::signOff($c, 'Selisih 1 siswa pindah, tagihan dihapus di billing', 'U1');
        $this->assertNotNull(ReceivableReconciliation::lastSignOff());
    }

    public function test_reconciliation_without_billing_config(): void
    {
        $this->controlOpening(100);
        config(['app.billing_internal_url' => '']);
        $c = ReceivableReconciliation::check();
        $this->assertNull($c['billing']);
        $this->assertStringContainsString('belum dikonfigurasi', $c['error']);
        $this->expectException(ValidationException::class);
        ReceivableReconciliation::signOff($c, 'apa saja panjang', 'U1');
    }

    public function test_auditor_cannot_sign_off(): void
    {
        $this->actingAs($this->userAs('AUD', 'Auditor'));
        Livewire::test(Import::class)->call('signOff')->assertForbidden();
    }
}
