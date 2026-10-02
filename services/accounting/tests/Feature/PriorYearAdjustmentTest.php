<?php

namespace Tests\Feature;

use App\Models\Parameter;
use App\Services\PriorYearAdjustmentService;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\DB;
use Tests\TestCase;

class PriorYearAdjustmentTest extends TestCase
{
    use RefreshDatabase;

    public function test_draft_does_not_change_history_or_balance_until_finalized(): void
    {
        $this->seedAccountingReferenceData();

        $service = app(PriorYearAdjustmentService::class);
        $session = $service->createSession([
            'target_year' => 2025,
            'adjustment_date' => '2025-12-31',
            'description' => 'Late audit adjustment',
        ]);

        $service->addLine($session, [
            'acc_code' => '1000',
            'div_code' => '0000',
            'description' => 'Recognize missing asset',
            'debet' => 100,
            'credit' => 0,
            'curr_code' => 'Rp',
        ]);
        $service->addLine($session, [
            'acc_code' => '4000',
            'div_code' => '0000',
            'description' => 'Recognize missing revenue',
            'debet' => 0,
            'credit' => 100,
            'curr_code' => 'Rp',
        ]);

        $this->assertDatabaseMissing('this_head', ['trans_date' => '2025-12-31']);
        $this->assertSaldo(2025, 13, '1000', '0000', 0, 0);
        $this->assertSaldo(2026, 0, '1000', '0000', 500, 0);
    }

    public function test_finalize_posts_period_13_and_cascades_opening_balance_without_changing_active_period(): void
    {
        $this->seedAccountingReferenceData();

        $service = app(PriorYearAdjustmentService::class);
        $session = $service->createSession([
            'target_year' => 2025,
            'adjustment_date' => '2025-12-31',
            'description' => 'Late audit adjustment',
        ]);

        $service->addLine($session, [
            'acc_code' => '1000',
            'div_code' => '0000',
            'description' => 'Recognize missing asset',
            'debet' => 100,
            'credit' => 0,
            'curr_code' => 'Rp',
        ]);
        $service->addLine($session, [
            'acc_code' => '4000',
            'div_code' => '0000',
            'description' => 'Recognize missing revenue',
            'debet' => 0,
            'credit' => 100,
            'curr_code' => 'Rp',
        ]);

        $service->finalize($session);

        $head = DB::table('this_head')->where('trans_date', '2025-12-31')->first();
        $this->assertNotNull($head);
        $this->assertSame('13', $head->period);
        $this->assertStringStartsWith('PYA25-', $head->trans_no);

        $this->assertSaldo(2025, 13, '1000', '0000', 100, 0);
        $this->assertSaldo(2025, 13, '4000', '0000', 0, 100);
        $this->assertSaldo(2026, 0, '1000', '0000', 600, 0);

        $retainedEarnings = DB::table('mst_account')
            ->where('acc_name', 'ilike', '%Retained Earning%2025%')
            ->first();

        $this->assertNotNull($retainedEarnings);
        $this->assertSaldo(2026, 0, $retainedEarnings->acc_code, '0000', 0, 100);

        $parameter = Parameter::first();
        $this->assertSame(2026, (int) $parameter->year);
        $this->assertSame(5, (int) $parameter->period);
    }

    public function test_add_line_uses_transaction_type_pairing_and_indonesian_amount_separator(): void
    {
        $this->seedAccountingReferenceData();

        DB::table('mst_transaction')->insert([
            'trans_code' => 'JV',
            'trans_name' => 'Journal Voucher',
            'trans_status' => '1',
            'trans_account' => '3000',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        $service = app(PriorYearAdjustmentService::class);
        $session = $service->createSession([
            'target_year' => 2025,
            'adjustment_date' => '2025-12-31',
            'trans_code' => 'JV',
            'description' => 'Late audit adjustment',
        ]);

        $service->addLine($session, [
            'acc_code' => '1000',
            'div_code' => '0000',
            'description' => 'Recognize missing asset',
            'debet' => '1.250.000,50',
            'credit' => '0',
            'curr_code' => 'Rp',
        ]);

        $lines = $session->fresh('lines')->lines;

        $this->assertCount(2, $lines);
        $this->assertSame('1000', $lines[0]->acc_code);
        $this->assertSame(1250000.50, (float) $lines[0]->debet);
        $this->assertSame(0.0, (float) $lines[0]->credit);
        $this->assertSame('3000', $lines[1]->acc_code);
        $this->assertSame(0.0, (float) $lines[1]->debet);
        $this->assertSame(1250000.50, (float) $lines[1]->credit);
    }

    private function seedAccountingReferenceData(): void
    {
        DB::table('parameter')->insert([
            'serial_no' => 'EXCO',
            'company' => 'EXCO',
            'period' => 5,
            'year' => 2026,
            'base_currency' => 'Rp',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_currency')->insert([
            'curr_code' => 'Rp',
            'curr_name' => 'Rupiah',
            'rate_system' => 'N',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_division')->insert([
            'div_code' => '0000',
            'div_name' => 'Head Office',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_transaction')->insert([
            'trans_code' => 'PYA',
            'trans_name' => 'Prior Year Adjustment',
            'trans_status' => '1',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_account')->insert([
            ['acc_code' => '1000', 'acc_name' => 'Cash', 'acc_group' => 1, 'acc_currency' => 'Rp', 'created_at' => now(), 'updated_at' => now()],
            ['acc_code' => '3000', 'acc_name' => 'Capital', 'acc_group' => 3, 'acc_currency' => 'Rp', 'created_at' => now(), 'updated_at' => now()],
            ['acc_code' => '4000', 'acc_name' => 'Revenue', 'acc_group' => 4, 'acc_currency' => 'Rp', 'created_at' => now(), 'updated_at' => now()],
        ]);

        foreach ([2025, 2026] as $year) {
            foreach (['1000', '3000', '4000'] as $account) {
                foreach (range(0, 13) as $period) {
                    DB::table('mst_saldo_news')->insert([
                        'acc_code' => $account,
                        'div_code' => '0000',
                        'periode' => $period,
                        'tahun' => $year,
                        'debet' => $account === '1000' && $period === 0 ? 500 : 0,
                        'credit' => $account === '3000' && $period === 0 ? 500 : 0,
                        'created_at' => now(),
                        'updated_at' => now(),
                    ]);
                }
            }
        }
    }

    private function assertSaldo(int $year, int $period, string $account, string $division, float $debet, float $credit): void
    {
        $saldo = DB::table('mst_saldo_news')
            ->where('tahun', $year)
            ->where('periode', $period)
            ->where('acc_code', $account)
            ->where('div_code', $division)
            ->first();

        $this->assertNotNull($saldo, "Missing saldo {$account}/{$division} {$year} P{$period}");
        $this->assertSame($debet, (float) $saldo->debet);
        $this->assertSame($credit, (float) $saldo->credit);
    }
}
