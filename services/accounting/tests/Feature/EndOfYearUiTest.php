<?php

namespace Tests\Feature;

use App\Livewire\Pages\EndOfYear\EOY;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\DB;
use Livewire\Livewire;
use Tests\TestCase;

class EndOfYearUiTest extends TestCase
{
    use RefreshDatabase;

    public function test_end_of_year_action_uses_custom_confirmation_modal(): void
    {
        $view = file_get_contents(resource_path('views/livewire/pages/end-of-year/e-o-y.blade.php'));
        $component = file_get_contents(app_path('Livewire/Pages/EndOfYear/EOY.php'));

        $this->assertStringContainsString('wire:click="confirmGenerate"', $view);
        $this->assertStringContainsString('wire:click="generate"', $view);
        $this->assertStringContainsString('Finalize End of Year', $view);
        $this->assertStringContainsString('$showConfirmModal', $view);
        $this->assertStringContainsString('wire:loading wire:target="generate"', $view);
        $this->assertStringContainsString('wire:loading.attr="disabled"', $view);
        $this->assertStringContainsString('$failureMessage', $view);
        $this->assertStringNotContainsString('wire:confirm=', $view);
        $this->assertStringNotContainsString('Swal.fire', $view);
        $this->assertStringNotContainsString('Swal.fire', $component);
    }

    public function test_end_of_year_confirmation_modal_opens_when_validation_passes(): void
    {
        $this->seedEndOfYearReferenceData();
        $this->seedCurrentYearBalances();

        Livewire::test(EOY::class)
            ->call('confirmGenerate')
            ->assertSet('showConfirmModal', true)
            ->assertSee('Finalize End of Year');
    }

    public function test_end_of_year_uses_bulk_transfer_queries_for_adjustments(): void
    {
        $component = file_get_contents(app_path('Livewire/Pages/EndOfYear/EOY.php'));

        $this->assertStringContainsString('INSERT INTO this_head', $component);
        $this->assertStringContainsString('INSERT INTO this_detail', $component);
        $this->assertStringContainsString('ON CONFLICT', $component);
        $this->assertStringNotContainsString('$data = DB::table(\'trans_head\')->get();', $component);
        $this->assertStringNotContainsString('$data2 = DB::table(\'trans_detail\')->get();', $component);
    }

    public function test_end_of_year_failure_is_reported_and_state_is_reloaded(): void
    {
        $this->seedEndOfYearReferenceData();

        DB::table('mst_saldo_news')->insert([
            'acc_code' => '1000',
            'div_code' => '0000',
            'periode' => 0,
            'tahun' => 2026,
            'debet' => 1,
            'credit' => 0,
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        Livewire::test(EOY::class)
            ->call('generate')
            ->assertSet('failureMessage', 'End of Year belum bisa dijalankan. Validasi belum lengkap. Perbaiki masalah yang ditemukan sebelum melanjutkan.')
            ->assertSet('disabled', false);
    }

    public function test_end_of_year_generates_next_year_opening_balance(): void
    {
        $this->seedEndOfYearReferenceData();
        $this->seedCurrentYearBalances();
        $this->seedPendingAdjustment();

        Livewire::test(EOY::class)
            ->call('generate')
            ->assertSet('disabled', true)
            ->assertSet('failureMessage', '');

        $parameter = DB::table('parameter')->first();
        $this->assertSame(2026, (int) $parameter->year);
        $this->assertSame(1, (int) $parameter->period);

        $cashOpening = DB::table('mst_saldo_news')
            ->where('tahun', 2026)
            ->where('periode', 0)
            ->where('acc_code', '1000')
            ->where('div_code', '0000')
            ->first();

        $this->assertNotNull($cashOpening);
        $this->assertSame(600.0, (float) $cashOpening->debet);
        $this->assertSame(0.0, (float) $cashOpening->credit);

        $retainedEarnings = DB::table('mst_saldo_news')
            ->join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->where('mst_saldo_news.tahun', 2026)
            ->where('mst_saldo_news.periode', 0)
            ->where('mst_account.acc_name', 'ilike', '%Retained Earning%2025%')
            ->select('mst_saldo_news.*')
            ->first();

        $this->assertNotNull($retainedEarnings);
        $this->assertSame(0.0, (float) $retainedEarnings->debet);
        $this->assertSame(300.0, (float) $retainedEarnings->credit);

        $this->assertDatabaseCount('trans_head', 0);
        $this->assertDatabaseCount('trans_detail', 0);
        $this->assertDatabaseHas('this_head', [
            'period' => '13',
            'trans_no' => 'JV-EOY-1',
        ]);
        $this->assertDatabaseHas('this_detail', [
            'period' => '13',
            'trans_no' => 'JV-EOY-1',
            'trans_code' => 'JV',
            'acc_code' => '1000',
            'div_code' => '0000',
        ]);
    }

    private function seedEndOfYearReferenceData(): void
    {
        DB::table('parameter')->insert([
            'serial_no' => 'EXCO',
            'company' => 'EXCO',
            'period' => 13,
            'year' => 2025,
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

        DB::table('mst_user')->insert([
            'user_id' => 'admin',
            'user_name' => 'Admin',
            'email' => 'admin@example.test',
            'password' => bcrypt('password'),
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
            [
                'acc_code' => '1000',
                'acc_name' => 'Cash',
                'acc_group' => 1,
                'acc_currency' => 'Rp',
                'created_at' => now(),
                'updated_at' => now(),
            ],
            [
                'acc_code' => '4000',
                'acc_name' => 'Revenue',
                'acc_group' => 4,
                'acc_currency' => 'Rp',
                'created_at' => now(),
                'updated_at' => now(),
            ],
        ]);

        foreach (range(1, 12) as $period) {
            DB::table('this_head')->insert([
                'period' => str_pad((string) $period, 2, '0', STR_PAD_LEFT),
                'trans_code' => null,
                'trans_no' => 'CLOSE'.$period,
                'trans_date' => "2025-{$period}-01",
                'trans_update' => "2025-{$period}-01",
                'trans_user' => null,
                'debet' => 0,
                'credit' => 0,
                'created_at' => now(),
                'updated_at' => now(),
            ]);
        }
    }

    private function seedCurrentYearBalances(): void
    {
        DB::table('mst_saldo_news')->insert([
            [
                'acc_code' => '1000',
                'div_code' => '0000',
                'periode' => 0,
                'tahun' => 2025,
                'debet' => 500,
                'credit' => 0,
                'created_at' => now(),
                'updated_at' => now(),
            ],
            [
                'acc_code' => '4000',
                'div_code' => '0000',
                'periode' => 12,
                'tahun' => 2025,
                'debet' => 0,
                'credit' => 200,
                'created_at' => now(),
                'updated_at' => now(),
            ],
        ]);
    }

    private function seedPendingAdjustment(): void
    {
        DB::table('trans_head')->insert([
            'trans_code' => 'JV',
            'trans_no' => 'JV-EOY-1',
            'trans_date' => '2025-12-31',
            'trans_update' => '2025-12-31',
            'trans_user' => 'admin',
            'debet' => 100,
            'credit' => 100,
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('trans_detail')->insert([
            [
                'trans_code' => 'JV',
                'trans_no' => 'JV-EOY-1',
                'trans_count' => 1,
                'acc_code' => '1000',
                'div_code' => '0000',
                'description' => 'Adjustment debit',
                'debet' => 100,
                'credit' => 0,
                'curr_code' => 'Rp',
                'curr_rate' => 1,
                'curr_debet' => 100,
                'curr_credit' => 0,
                'trans_date' => '2025-12-31',
                'curr_system' => null,
                'curr_yes' => false,
                'trans_system' => null,
                'trans_yes' => false,
                'created_at' => now(),
                'updated_at' => now(),
            ],
            [
                'trans_code' => 'JV',
                'trans_no' => 'JV-EOY-1',
                'trans_count' => 2,
                'acc_code' => '4000',
                'div_code' => '0000',
                'description' => 'Adjustment credit',
                'debet' => 0,
                'credit' => 100,
                'curr_code' => 'Rp',
                'curr_rate' => 1,
                'curr_debet' => 0,
                'curr_credit' => 100,
                'trans_date' => '2025-12-31',
                'curr_system' => null,
                'curr_yes' => false,
                'trans_system' => null,
                'trans_yes' => false,
                'created_at' => now(),
                'updated_at' => now(),
            ],
        ]);
    }
}
