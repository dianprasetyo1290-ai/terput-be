<?php

namespace Tests\Feature;

use App\Livewire\Pages\HistoryJournalTransaction\HJT;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\DB;
use Livewire\Livewire;
use Tests\TestCase;

class HistoryJournalTransactionTest extends TestCase
{
    use RefreshDatabase;

    public function test_history_journal_filters_period_by_active_parameter_year(): void
    {
        $this->seedReferenceData();
        $this->seedHistoryJournal('JV/ADJ2/12/25', 2025);
        $this->seedHistoryJournal('JV/ADJ2/12/26', 2026);

        Livewire::test(HJT::class)
            ->set('bulan', 13)
            ->set('search', 'JV/ADJ2')
            ->assertSee('JV/ADJ2/12/26')
            ->assertDontSee('JV/ADJ2/12/25');
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
            'trans_code' => 'JV',
            'trans_name' => 'Journal Voucher',
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

    private function seedHistoryJournal(string $transNo, int $year): void
    {
        DB::table('this_head')->insert([
            'period' => '13',
            'trans_code' => 'JV',
            'trans_no' => $transNo,
            'trans_date' => "{$year}-12-31",
            'trans_update' => "{$year}-12-31",
            'trans_user' => '0001',
            'debet' => 100,
            'credit' => 100,
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('this_detail')->insert([
            'period' => '13',
            'trans_code' => 'JV',
            'trans_no' => $transNo,
            'trans_count' => 1,
            'acc_code' => '1000',
            'div_code' => '0000',
            'description' => 'Adjustment',
            'debet' => 100,
            'credit' => 100,
            'curr_code' => 'Rp',
            'curr_rate' => 1,
            'curr_debet' => 100,
            'curr_credit' => 100,
            'trans_date' => "{$year}-12-31",
            'curr_system' => 'N',
            'curr_yes' => false,
            'created_at' => now(),
            'updated_at' => now(),
        ]);
    }
}
