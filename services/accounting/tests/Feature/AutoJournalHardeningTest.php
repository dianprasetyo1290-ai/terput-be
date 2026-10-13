<?php

namespace Tests\Feature;

use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Http;
use Tests\Concerns\SeedsBooks;
use Tests\TestCase;

/**
 * P3B: AutoJournalService hardening -- BR-ACC-07 closed period, idempotency
 * across trans_head + this_head (after End of Month), numbering that does not
 * collide with history, and the acc_auto_postings log behind the dashboard's
 * failed-posting count (FR-32-002).
 */
class AutoJournalHardeningTest extends TestCase
{
    use RefreshDatabase, SeedsBooks;

    private const SCHOOL_ID = '5006c011-b21e-42ad-9fb5-c07de8d99f74';
    private const KEY = 'test-internal-key-12345';

    protected function setUp(): void
    {
        parent::setUp();
        config(['app.sekoci_internal_key' => self::KEY, 'app.identity_service_url' => 'http://identity:8081']);
        Http::fake(['identity:8081/*' => Http::response(['data' => [['feature_key' => 'accounting', 'enabled' => true]]], 200)]);
        $this->seedBooks(2026, 10);
    }

    private function send(array $over = [])
    {
        return $this->withHeaders(['X-Internal-Key' => self::KEY])->postJson('/api/internal/journals', array_merge([
            'school_id' => self::SCHOOL_ID,
            'source' => 'billing',
            'reference' => 'billing:payment:2026-10-01:SD',
            'trans_date' => '2026-10-01',
            'description' => 'Penerimaan SPP 1 Okt SD',
            'lines' => [
                ['acc_code' => '1102', 'div_code' => 'SD', 'debit_idr' => 3000000, 'credit_idr' => 0],
                ['acc_code' => '1201', 'div_code' => 'SD', 'debit_idr' => 0, 'credit_idr' => 3000000],
            ],
        ], $over));
    }

    public function test_rejects_journal_dated_in_closed_period_and_logs_failure(): void
    {
        $r = $this->send(['reference' => 'billing:bill:2026-09-15:SD', 'trans_date' => '2026-09-15']);

        $r->assertStatus(422)->assertJsonPath('code', 'period_closed');
        $this->assertStringContainsString('sudah ditutup', $r->json('message'));
        $this->assertDatabaseCount('trans_head', 0);
        $this->assertDatabaseHas('acc_auto_postings', [
            'kind' => 'billing', 'reference' => 'billing:bill:2026-09-15:SD', 'status' => 'failed',
        ]);
    }

    public function test_rejects_journal_dated_in_period_not_yet_open(): void
    {
        $this->send(['trans_date' => '2026-11-02'])->assertStatus(422)->assertJsonPath('code', 'period_closed');
    }

    public function test_posts_in_open_period_with_division_and_logs_success(): void
    {
        $r = $this->send();

        $r->assertStatus(201);
        $no = $r->json('trans_no');
        $this->assertStringStartsWith('BIL26-', $no);
        $this->assertDatabaseHas('trans_head', ['trans_no' => $no, 'status' => 'posted', 'source' => 'billing']);
        $this->assertDatabaseHas('trans_detail', ['trans_no' => $no, 'acc_code' => '1102', 'div_code' => 'SD']);
        $this->assertDatabaseHas('acc_auto_postings', ['reference' => 'billing:payment:2026-10-01:SD', 'status' => 'posted', 'trans_no' => $no]);
    }

    public function test_retry_after_failure_flips_log_to_posted(): void
    {
        $this->send(['lines' => [
            ['acc_code' => '1102', 'debit_idr' => 100, 'credit_idr' => 0],
            ['acc_code' => '9999', 'debit_idr' => 0, 'credit_idr' => 100],
        ]])->assertStatus(422);
        $this->assertSame(1, DB::table('acc_auto_postings')->where('status', 'failed')->count());

        $this->send()->assertStatus(201);
        $this->assertSame(0, DB::table('acc_auto_postings')->where('status', 'failed')->count());
        $this->assertSame(1, DB::table('acc_auto_postings')->count());
    }

    public function test_idempotent_after_end_of_month_moved_journal_to_history(): void
    {
        // Booked in September, closed by End of Month (row now lives in this_head).
        $this->book('BIL26-000007', '2026-09-30', [['1102', 500, 0], ['1201', 0, 500]], 'posted', true,
            ['trans_code' => 'BIL', 'source' => 'billing', 'reference' => 'billing:payment:2026-09-30:SD']);

        // A late retry of the same reference -- even dated in the closed period -- returns the original number.
        $r = $this->send(['reference' => 'billing:payment:2026-09-30:SD', 'trans_date' => '2026-09-30']);

        $r->assertStatus(200)->assertJsonPath('trans_no', 'BIL26-000007');
        $this->assertDatabaseCount('trans_head', 0);
    }

    public function test_numbering_continues_after_history_numbers(): void
    {
        $this->book('BIL26-000007', '2026-09-30', [['1102', 500, 0], ['1201', 0, 500]], 'posted', true,
            ['trans_code' => 'BIL', 'source' => 'billing', 'reference' => 'old-ref']);

        $this->send()->assertStatus(201)->assertJsonPath('trans_no', 'BIL26-000008');
    }

    public function test_rejects_negative_amounts(): void
    {
        $this->send(['lines' => [
            ['acc_code' => '1102', 'debit_idr' => -100, 'credit_idr' => 0],
            ['acc_code' => '1201', 'debit_idr' => 0, 'credit_idr' => -100],
        ]])->assertStatus(422);
    }
}
