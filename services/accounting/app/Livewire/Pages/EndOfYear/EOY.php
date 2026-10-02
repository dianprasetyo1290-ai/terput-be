<?php

namespace App\Livewire\Pages\EndOfYear;

use App\Models\ChartOfAccount;
use App\Models\JournalTransaction;
use App\Models\MstSaldoNew;
use App\Models\Parameter;
use App\Models\Period;
use Carbon\Carbon;
use Illuminate\Support\Facades\DB;
use Livewire\Component;
use Throwable;

class EOY extends Component
{
    public $answer = '';

    public $nextYear = '';

    public $periode;

    public $disabled = false;

    public $checks = [];

    public $allPassed = false;

    public $netIncome = 0;

    public $realAccountCount = 0;

    public $nominalAccountCount = 0;

    public $justRunMessage = '';

    public $failureMessage = '';

    public bool $showConfirmModal = false;

    public function mount()
    {
        $this->periode = Parameter::first();
        if ((int) $this->periode->period === 13) {
            $this->runChecks();
            $this->loadPreview();
        }
    }

    public function render()
    {
        return view('livewire.pages.end-of-year.e-o-y');
    }

    public function runChecks()
    {
        $parameter = Parameter::first();
        $currentYear = (int) $parameter->year;

        // 1. Period must be 13
        $isPeriod13 = (int) $parameter->period === 13;

        // 2. All periods 1-12 of the CURRENT YEAR must be closed (exist in
        //    this_head). this_head menyimpan riwayat semua tahun, jadi WAJIB
        //    difilter by trans_date year — tanpa filter, periode dari tahun
        //    lain bisa membuat check ini lolos padahal tahun berjalan belum
        //    lengkap di-close.
        $closedPeriods = DB::table('this_head')
            ->whereRaw('EXTRACT(YEAR FROM trans_date) = ?', [$currentYear])
            ->select('period')
            ->distinct()
            ->pluck('period')
            ->map(fn ($p) => (int) $p)
            ->toArray();
        $allPeriodsClosedCount = count(array_intersect(range(1, 12), $closedPeriods));

        // 3. Check unbalanced journals in trans_head (period 13 adjustments)
        $transCount = DB::table('trans_head')->count();
        $unbalancedCount = 0;
        if ($transCount > 0) {
            $unbalancedCount = JournalTransaction::withNotEqualDebetAndCredit()->count();
        }

        // 4. Check EOY hasn't been run for next year already
        $nextYear = $currentYear + 1;
        $alreadyGenerated = MstSaldoNew::where('tahun', $nextYear)->exists();

        $this->checks = [
            [
                'label' => 'Periode Aktif',
                'desc' => 'Periode harus di 13 (Adjustment)',
                'passed' => $isPeriod13,
                'issue' => ! $isPeriod13 ? 1 : 0,
            ],
            [
                'label' => 'Periode 1-12 Closed',
                'desc' => "Semua 12 periode sudah di-close ({$allPeriodsClosedCount}/12)",
                'passed' => $allPeriodsClosedCount === 12,
                'issue' => 12 - $allPeriodsClosedCount,
            ],
            [
                'label' => 'Jurnal Balance',
                'desc' => 'Semua jurnal adjustment balance (debit = credit)',
                'passed' => $unbalancedCount === 0,
                'issue' => $unbalancedCount,
            ],
            [
                'label' => 'Belum Dijalankan',
                'desc' => "Saldo tahun {$nextYear} belum ada",
                'passed' => ! $alreadyGenerated,
                'issue' => $alreadyGenerated ? 1 : 0,
            ],
        ];

        $this->allPassed = collect($this->checks)->every(fn ($c) => $c['passed']);
    }

    public function loadPreview()
    {
        $currentYear = (int) Parameter::first()->year;

        $this->realAccountCount = ChartOfAccount::whereIn('acc_group', [1, 2, 3])->count();
        $this->nominalAccountCount = ChartOfAccount::whereIn('acc_group', [4, 5, 6, 7, 8])->count();

        // Net income = SUM(credit - debet) from all nominal accounts
        $this->netIncome = (float) DB::table('mst_saldo_news')
            ->join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->whereIn('mst_account.acc_group', [4, 5, 6, 7, 8])
            ->where('mst_saldo_news.tahun', $currentYear)
            ->selectRaw('COALESCE(SUM(mst_saldo_news.credit) - SUM(mst_saldo_news.debet), 0) as net')
            ->value('net');
    }

    public function confirmGenerate(): void
    {
        $this->failureMessage = '';
        $this->justRunMessage = '';
        $this->runChecks();
        $this->loadPreview();

        if (! $this->allPassed) {
            $this->showConfirmModal = false;
            $this->failEoy('End of Year belum bisa dijalankan. Validasi belum lengkap. Perbaiki masalah yang ditemukan sebelum melanjutkan.');

            return;
        }

        $this->showConfirmModal = true;
    }

    public function cancelGenerate(): void
    {
        $this->showConfirmModal = false;
    }

    public function generate()
    {
        $this->showConfirmModal = false;
        $this->failureMessage = '';
        $this->justRunMessage = '';
        $this->runChecks();
        $this->loadPreview();

        if (! $this->allPassed) {
            $this->failEoy('End of Year belum bisa dijalankan. Validasi belum lengkap. Perbaiki masalah yang ditemukan sebelum melanjutkan.');

            return;
        }

        $parameter = Parameter::first();
        $currentYear = (int) $parameter->year;
        $nextYear = $currentYear + 1;

        // Guard double-run: checks bisa basi setelah request pertama selesai.
        if (MstSaldoNew::where('tahun', $nextYear)->exists()) {
            $this->failEoy("End of Year sudah pernah dijalankan untuk tahun {$nextYear}.");

            return;
        }

        @set_time_limit(0);
        ini_set('max_execution_time', '0');

        try {
            DB::transaction(function () use ($parameter, $currentYear, $nextYear) {
                DB::statement("SET LOCAL statement_timeout = '15min'");

                // Step 1: Transfer adjustment entries (period 13) if any exist
                $this->transferAdjustments($parameter);

                // Step 2: Create Retained Earnings account for the closing year
                $reAccCode = $this->createRetainedEarningsAccount($currentYear);

                // Step 3: Generate next year saldo (only REAL accounts + RE closing)
                $this->genMstSaldo($currentYear, $nextYear, $reAccCode);

                // Step 4: Update parameter (year+1, period=1)
                $parameter->period = 1;
                $parameter->year = $nextYear;
                $parameter->save();

                // Step 5: Update Period table (rolling window)
                $this->updatePeriodTable($nextYear);
            });
        } catch (Throwable $exception) {
            report($exception);
            $this->periode = Parameter::first();
            $this->runChecks();
            $this->loadPreview();
            $this->failEoy('End of Year gagal. Semua perubahan di dalam transaksi sudah di-rollback. Detail: '.$exception->getMessage());

            return;
        }

        $this->disabled = true;
        $this->nextYear = $nextYear;
        $this->justRunMessage = "End of Year berhasil! Tahun buku {$nextYear} telah dibuat.";
        $this->periode = Parameter::first();
        $this->runChecks();

        $this->appendAnswer("End of Year berhasil! Tahun buku {$nextYear} telah dibuat.\n");
    }

    private function transferAdjustments($parameter)
    {
        $transCount = DB::table('trans_head')->count();
        if ($transCount === 0) {
            $this->appendAnswer("Tidak ada jurnal adjustment periode 13.\n");

            return;
        }

        $period = str_pad((string) $parameter->period, 2, '0', STR_PAD_LEFT);

        DB::statement(<<<'SQL'
            INSERT INTO this_head (
                period, trans_code, trans_no, trans_date, trans_update, trans_user,
                debet, credit, created_at, updated_at
            )
            SELECT
                :period, trans_code, trans_no, trans_date, trans_update, trans_user,
                debet, credit, created_at, updated_at
            FROM trans_head
            ON CONFLICT (trans_no, trans_date) DO NOTHING
        SQL, ['period' => $period]);

        DB::statement(<<<'SQL'
            INSERT INTO this_detail (
                period, trans_code, trans_no, trans_count, acc_code, div_code, description,
                debet, credit, curr_code, curr_rate, curr_debet, curr_credit, trans_date,
                curr_system, curr_yes, created_at, updated_at
            )
            SELECT
                :period, trans_code, trans_no, trans_count, acc_code, div_code, description,
                debet, credit, curr_code, curr_rate, curr_debet, curr_credit, trans_date,
                curr_system, curr_yes, created_at, updated_at
            FROM trans_detail
            ON CONFLICT (trans_no, trans_code, trans_count, acc_code, div_code, trans_date) DO NOTHING
        SQL, ['period' => $period]);

        DB::table('trans_head')->delete();
        DB::table('trans_detail')->delete();

        // Update saldo for period 13 from transferred transactions
        $this->updateSaldoForPeriod((int) $parameter->period, (int) $parameter->year);

        $this->appendAnswer("Transfer adjustment periode 13 selesai ({$transCount} transaksi).\n");
    }

    private function createRetainedEarningsAccount($year): string
    {
        // Check if RE account for this year already exists
        $existing = ChartOfAccount::where('acc_name', 'ilike', "%Retained Earning%{$year}%")->first();
        if ($existing) {
            $this->appendAnswer("Akun RE '{$existing->acc_code} - {$existing->acc_name}' sudah ada.\n");

            return $existing->acc_code;
        }

        // Find last index from pattern 388888880000XXX
        $lastRE = ChartOfAccount::where('acc_code', 'like', '38888888%')
            ->where('acc_code', '!=', '388888880000000')
            ->orderBy('acc_code', 'desc')
            ->first();

        $lastIndex = $lastRE ? (int) substr($lastRE->acc_code, -3) : 0;
        $newIndex = str_pad($lastIndex + 1, 3, '0', STR_PAD_LEFT);
        $newAccCode = '388888880000'.$newIndex;

        ChartOfAccount::create([
            'acc_code' => $newAccCode,
            'acc_name' => "Retained Earning of {$year}",
            'acc_group' => 3,
            'acc_currency' => 'Rp',
        ]);

        $this->appendAnswer("Akun RE '{$newAccCode} - Retained Earning of {$year}' dibuat.\n");

        return $newAccCode;
    }

    private function genMstSaldo($currentYear, $nextYear, $reAccCode)
    {
        ini_set('memory_limit', '-1');
        ini_set('max_execution_time', '0');

        $this->appendAnswer("Mulai generate saldo tahun {$nextYear}...\n");

        // Step 1: INSERT all acc x div x period combinations for next year.
        $insertQuery = '
            INSERT INTO mst_saldo_news (acc_code, div_code, periode, tahun, debet, credit, created_at, updated_at)
            SELECT coa.acc_code, divs.div_code, gs.periode, :nextYear, 0, 0, NOW(), NOW()
            FROM mst_account coa
            CROSS JOIN mst_division divs
            CROSS JOIN generate_series(0, 13) AS gs(periode)
            ON CONFLICT (acc_code, div_code, periode, tahun) DO NOTHING
        ';
        DB::statement($insertQuery, ['nextYear' => $nextYear]);

        $this->appendAnswer("Template saldo tahun {$nextYear} dibuat.\n");

        // Step 2: UPDATE period 0 — ONLY REAL accounts (group 1,2,3)
        $updateRealQuery = '
            UPDATE mst_saldo_news m
            SET debet = s.total_debet, credit = s.total_credit
            FROM (
                SELECT sn.acc_code, sn.div_code,
                       SUM(sn.debet) AS total_debet, SUM(sn.credit) AS total_credit
                FROM mst_saldo_news sn
                JOIN mst_account a ON sn.acc_code = a.acc_code
                WHERE sn.tahun = :currentYear AND a.acc_group IN (1, 2, 3)
                GROUP BY sn.acc_code, sn.div_code
            ) s
            WHERE m.acc_code = s.acc_code AND m.div_code = s.div_code
              AND m.tahun = :nextYear AND m.periode = 0
        ';
        DB::statement($updateRealQuery, ['currentYear' => $currentYear, 'nextYear' => $nextYear]);

        $this->appendAnswer("Saldo awal akun REAL (Asset, Liability, Equity) di-carry forward.\n");

        // Step 3: Calculate net income from NOMINAL accounts
        $netIncome = (float) DB::table('mst_saldo_news')
            ->join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->whereIn('mst_account.acc_group', [4, 5, 6, 7, 8])
            ->where('mst_saldo_news.tahun', $currentYear)
            ->selectRaw('COALESCE(SUM(mst_saldo_news.credit) - SUM(mst_saldo_news.debet), 0) as net')
            ->value('net');

        $this->appendAnswer("Net income tahun {$currentYear}: Rp ".number_format($netIncome, 2, ',', '.')."\n");

        // Step 4: Post net income to Retained Earnings (division 0000, period 0)
        MstSaldoNew::where('acc_code', $reAccCode)
            ->where('div_code', '0000')
            ->where('periode', 0)
            ->where('tahun', $nextYear)
            ->update([
                'debet' => DB::raw($netIncome < 0 ? 'debet + '.abs($netIncome) : 'debet'),
                'credit' => DB::raw($netIncome >= 0 ? 'credit + '.$netIncome : 'credit'),
                'updated_at' => now(),
            ]);

        // NOMINAL accounts period 0 stay at 0 (already 0 from INSERT step 1)

        $this->appendAnswer("Closing entry ke Retained Earnings selesai. Akun nominal di-nolkan.\n");
        $this->appendAnswer("Proses generate saldo tahun {$nextYear} selesai!\n");
    }

    private function appendAnswer(string $content): void
    {
        $this->answer .= $content;

        if ($this->getId()) {
            $this->stream(to: 'answer', content: $content, replace: false);
        }
    }

    private function failEoy(string $message): void
    {
        $this->failureMessage = $message;
        $this->disabled = false;
        $this->appendAnswer($message."\n");
    }

    private function updateSaldoForPeriod($period, $year)
    {
        $periodPadded = str_pad($period, 2, '0', STR_PAD_LEFT);

        // Reset saldo periode ini untuk mencegah double-counting
        MstSaldoNew::where('periode', $period)
            ->where('tahun', $year)
            ->update(['debet' => 0, 'credit' => 0]);

        DB::statement(<<<'SQL'
            INSERT INTO mst_saldo_news (acc_code, div_code, periode, tahun, debet, credit, created_at, updated_at)
            SELECT
                acc_code,
                div_code,
                :period,
                :year,
                SUM(COALESCE(debet, 0)) AS total_debet,
                SUM(COALESCE(credit, 0)) AS total_credit,
                NOW(),
                NOW()
            FROM this_detail
            WHERE period = :periodPadded
              AND EXTRACT(YEAR FROM trans_date) = :yearForDate
            GROUP BY acc_code, div_code
            ON CONFLICT (acc_code, div_code, periode, tahun)
            DO UPDATE SET
                debet = EXCLUDED.debet,
                credit = EXCLUDED.credit,
                updated_at = NOW()
        SQL, [
            'period' => $period,
            'year' => $year,
            'periodPadded' => $periodPadded,
            'yearForDate' => $year,
        ]);
    }

    private function updatePeriodTable($nextYear)
    {
        $periods = Period::whereIn('period_code', [1, 2, 3])->get();

        foreach ($periods as $period) {
            $year = match ((int) $period->period_code) {
                1 => $nextYear,
                2 => $nextYear - 1,
                3 => $nextYear - 2,
            };

            $period->period_name = $year;

            for ($i = 1; $i <= 13; $i++) {
                $monthCode = str_pad($i, 2, '0', STR_PAD_LEFT);

                if ($i == 13) {
                    $period->{"p{$monthCode}"} = "Adj {$year}";
                    $period->{"p{$monthCode}_from"} = Carbon::create($year, 12, 1)->format('Y-m-d');
                    $period->{"p{$monthCode}_to"} = Carbon::create($year, 12, 1)->endOfMonth()->format('Y-m-d');
                } else {
                    $period->{"p{$monthCode}"} = Carbon::create($year, $i, 1)->format('M Y');
                    $period->{"p{$monthCode}_from"} = Carbon::create($year, $i, 1)->format('Y-m-d');
                    $period->{"p{$monthCode}_to"} = Carbon::create($year, $i, 1)->endOfMonth()->format('Y-m-d');
                }
            }

            $period->save();
        }
    }
}
