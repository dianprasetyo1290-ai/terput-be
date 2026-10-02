<?php

namespace Tests\Feature;

use App\Livewire\Pages\Report\GeneralLedger\ReportGeneralLedger;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\DB;
use Livewire\Livewire;
use Tests\TestCase;

class GeneralLedgerReportTest extends TestCase
{
    use RefreshDatabase;

    public function test_history_summary_report_does_not_duplicate_reused_transaction_numbers_across_years(): void
    {
        $this->seedReferenceData();
        $this->seedHistoryHead('PV/001', 2025);
        $this->seedHistoryHead('PV/001', 2026);
        $this->seedHistoryDetail('PV/001', 2026, 100, 0);

        Livewire::test(ReportGeneralLedger::class)
            ->set('acc_code', '1000')
            ->set('acc_code2', '1000')
            ->set('year', 2026)
            ->set('periodeFrom', '1')
            ->set('periodeTo', '1')
            ->set('report', 'summarygeneralledger')
            ->set('this_period', false)
            ->call('viewReport')
            ->assertSet('reportData.0.debet', 100.0)
            ->assertSet('reportData.0.credit', 0.0);
    }

    public function test_history_detail_report_does_not_duplicate_reused_transaction_numbers_across_years(): void
    {
        $this->seedReferenceData();
        $this->seedHistoryHead('PV/001', 2025);
        $this->seedHistoryHead('PV/001', 2026);
        $this->seedHistoryDetail('PV/001', 2026, 100, 0);

        $details = getLedgerDetails(2026, '1000', '01', '01', null, null, false);

        $this->assertCount(1, $details);
        $this->assertSame(100.0, (float) $details->first()->debet);
        $this->assertSame('2026-01-02', $details->first()->trans_date);
    }

    private function seedReferenceData(): void
    {
        DB::table('parameter')->insert([
            'serial_no' => 'EXCO',
            'company' => 'EXCO',
            'period' => 6,
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
            'trans_code' => 'PV',
            'trans_name' => 'Payment Voucher',
            'trans_status' => '1',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_account')->insert([
            'acc_code' => '1000',
            'acc_name' => 'Cash',
            'acc_group' => 1,
            'acc_currency' => 'Rp',
            'created_at' => now(),
            'updated_at' => now(),
        ]);
    }

    private function seedHistoryHead(string $transNo, int $year): void
    {
        DB::table('this_head')->insert([
            'period' => '01',
            'trans_code' => 'PV',
            'trans_no' => $transNo,
            'trans_date' => "{$year}-01-02",
            'trans_update' => "{$year}-01-02",
            'trans_user' => '0001',
            'debet' => 100,
            'credit' => 0,
            'created_at' => now(),
            'updated_at' => now(),
        ]);
    }

    private function seedHistoryDetail(string $transNo, int $year, float $debet, float $credit): void
    {
        DB::table('this_detail')->insert([
            'period' => '01',
            'trans_code' => 'PV',
            'trans_no' => $transNo,
            'trans_count' => 1,
            'acc_code' => '1000',
            'div_code' => '0000',
            'description' => 'Legacy recurring transaction number',
            'debet' => $debet,
            'credit' => $credit,
            'curr_code' => 'Rp',
            'curr_rate' => 1,
            'curr_debet' => $debet,
            'curr_credit' => $credit,
            'trans_date' => "{$year}-01-02",
            'curr_system' => 'N',
            'curr_yes' => false,
            'created_at' => now(),
            'updated_at' => now(),
        ]);
    }
}
