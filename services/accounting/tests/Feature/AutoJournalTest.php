<?php

namespace Tests\Feature;

use App\Models\User;
use App\Support\Tenancy;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Http;
use Tests\TestCase;

class AutoJournalTest extends TestCase
{
    use RefreshDatabase;

    private const SCHOOL_ID = '5006c011-b21e-42ad-9fb5-c07de8d99f74';
    private const INTERNAL_KEY = 'test-internal-key-12345';

    protected function setUp(): void
    {
        parent::setUp();
        config([
            'app.sekoci_internal_key' => self::INTERNAL_KEY,
            'app.identity_service_url' => 'http://identity:8081',
        ]);
    }

    private function fakeAccountingFeature(bool $enabled = true, int $status = 200): void
    {
        Http::fake([
            'identity:8081/*' => Http::response([
                'data' => [['feature_key' => 'accounting', 'enabled' => $enabled]],
                'message' => '',
                'errors' => null,
            ], $status),
        ]);
    }

    private function seedReferenceData(): void
    {
        DB::table('mst_currency')->insertOrIgnore([
            'curr_code' => 'Rp', 'curr_name' => 'Rupiah', 'rate_system' => 'N', 'created_at' => now(), 'updated_at' => now(),
        ]);
        DB::table('mst_user')->insertOrIgnore([
            'user_id' => 'SYS_USER',
            'user_name' => 'System User',
            'email' => 'sys@accounting.test',
            'password' => bcrypt('secret'),
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_account')->insertOrIgnore([
            ['acc_code' => '6101', 'acc_name' => 'Gaji Pokok', 'acc_group' => 6, 'acc_currency' => 'Rp', 'is_active' => true, 'created_at' => now(), 'updated_at' => now()],
            ['acc_code' => '2101', 'acc_name' => 'Utang Gaji', 'acc_group' => 2, 'acc_currency' => 'Rp', 'is_active' => true, 'created_at' => now(), 'updated_at' => now()],
            ['acc_code' => '9999', 'acc_name' => 'Akun Nonaktif', 'acc_group' => 9, 'acc_currency' => 'Rp', 'is_active' => false, 'created_at' => now(), 'updated_at' => now()],
        ]);
    }

    public function test_rejects_missing_or_invalid_internal_key(): void
    {
        $response = $this->postJson('/api/internal/journals', [
            'school_id' => self::SCHOOL_ID,
        ]);
        $response->assertStatus(401);

        $responseWrong = $this->withHeaders(['X-Internal-Key' => 'wrong-key'])
            ->postJson('/api/internal/journals', [
                'school_id' => self::SCHOOL_ID,
            ]);
        $responseWrong->assertStatus(401);
    }

    public function test_rejects_when_accounting_module_disabled(): void
    {
        $this->fakeAccountingFeature(false);

        $payload = [
            'school_id' => self::SCHOOL_ID,
            'source' => 'payroll',
            'reference' => 'payroll:test-1:accrual',
            'trans_date' => '2026-10-31',
            'description' => 'Payroll Test',
            'lines' => [
                ['acc_code' => '6101', 'debit_idr' => 1000000, 'credit_idr' => 0],
                ['acc_code' => '2101', 'debit_idr' => 0, 'credit_idr' => 1000000],
            ],
        ];

        $response = $this->withHeaders(['X-Internal-Key' => self::INTERNAL_KEY])
            ->postJson('/api/internal/journals', $payload);

        $response->assertStatus(409);
        $this->assertStringContainsString('modul akuntansi tidak aktif', $response->json('message'));
    }

    public function test_rejects_unbalanced_journal(): void
    {
        $this->fakeAccountingFeature(true);
        $this->seedReferenceData();

        $payload = [
            'school_id' => self::SCHOOL_ID,
            'source' => 'payroll',
            'reference' => 'payroll:test-unbalanced:accrual',
            'trans_date' => '2026-10-31',
            'description' => 'Payroll Unbalanced',
            'lines' => [
                ['acc_code' => '6101', 'debit_idr' => 1000000, 'credit_idr' => 0],
                ['acc_code' => '2101', 'debit_idr' => 0, 'credit_idr' => 500000],
            ],
        ];

        $response = $this->withHeaders(['X-Internal-Key' => self::INTERNAL_KEY])
            ->postJson('/api/internal/journals', $payload);

        $response->assertStatus(422);
        $this->assertStringContainsString('tidak seimbang', $response->json('message'));
    }

    public function test_rejects_invalid_or_inactive_account(): void
    {
        $this->fakeAccountingFeature(true);
        $this->seedReferenceData();

        $payload = [
            'school_id' => self::SCHOOL_ID,
            'source' => 'payroll',
            'reference' => 'payroll:test-inactive:accrual',
            'trans_date' => '2026-10-31',
            'description' => 'Payroll Inactive',
            'lines' => [
                ['acc_code' => '9999', 'debit_idr' => 1000000, 'credit_idr' => 0],
                ['acc_code' => '2101', 'debit_idr' => 0, 'credit_idr' => 1000000],
            ],
        ];

        $response = $this->withHeaders(['X-Internal-Key' => self::INTERNAL_KEY])
            ->postJson('/api/internal/journals', $payload);

        $response->assertStatus(422);
        $this->assertContains('9999', $response->json('invalid_accounts'));
    }

    public function test_posts_balanced_journal_and_is_idempotent(): void
    {
        $this->fakeAccountingFeature(true);
        $this->seedReferenceData();

        $payload = [
            'school_id' => self::SCHOOL_ID,
            'source' => 'payroll',
            'reference' => 'payroll:test-idempotent:accrual',
            'trans_date' => '2026-10-31',
            'description' => 'Payroll Sukses',
            'lines' => [
                ['acc_code' => '6101', 'debit_idr' => 150000000, 'credit_idr' => 0],
                ['acc_code' => '2101', 'debit_idr' => 0, 'credit_idr' => 150000000],
            ],
        ];

        // 1st request -> 201 Created
        $response1 = $this->withHeaders(['X-Internal-Key' => self::INTERNAL_KEY])
            ->postJson('/api/internal/journals', $payload);

        $response1->assertStatus(201);
        $transNo1 = $response1->json('trans_no');
        $this->assertNotEmpty($transNo1);
        $this->assertStringStartsWith('PRL26-', $transNo1);

        // Check DB trans_head and trans_detail
        $this->assertDatabaseHas('trans_head', [
            'trans_no' => $transNo1,
            'source' => 'payroll',
            'reference' => 'payroll:test-idempotent:accrual',
            'status' => 'posted',
        ]);
        $this->assertDatabaseHas('trans_detail', [
            'trans_no' => $transNo1,
            'acc_code' => '6101',
            'debet' => 150000000,
        ]);
        $this->assertDatabaseHas('trans_detail', [
            'trans_no' => $transNo1,
            'acc_code' => '2101',
            'credit' => 150000000,
        ]);

        // 2nd request with same reference -> 200 OK with same trans_no (idempotent)
        $response2 = $this->withHeaders(['X-Internal-Key' => self::INTERNAL_KEY])
            ->postJson('/api/internal/journals', $payload);

        $response2->assertStatus(200);
        $transNo2 = $response2->json('trans_no');
        $this->assertSame($transNo1, $transNo2);
    }
}
