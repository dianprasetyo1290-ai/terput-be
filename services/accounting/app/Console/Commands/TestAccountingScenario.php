<?php

namespace App\Console\Commands;

use App\Models\ChartOfAccount;
use App\Models\JournalTransaction;
use App\Models\MstSaldoNew;
use App\Models\Parameter;
use App\Models\TransDetail;
use Carbon\Carbon;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Hash;

/**
 * Comprehensive 2-year accounting scenario test.
 *
 * Unlike test:accounting-cycle (3 fixed journals/month, no adjustments),
 * this simulates a realistic business cycle:
 *   - 18 accounts covering all 8 account groups
 *   - 10 DAILY transactions per month (sales, purchases, collections,
 *     payments, interest, bank charges)
 *   - one-time setup in Year 1 period 1 (capital, equipment, inventory,
 *     prepaid insurance)
 *   - period-13 ADJUSTMENT entries (depreciation, prepaid amortization,
 *     accrued salary)
 *   - monthly EOM close + EOY year-end close
 *   - repeated for 2 full years, ending with the year-3 opening balances
 *
 * Every checkpoint has an explicit expected-value "control". All amounts
 * are deterministic so expected balances are computed, not guessed.
 */
class TestAccountingScenario extends Command
{
    protected $signature = 'test:accounting-scenario {--fresh : Truncate tables and start fresh} {--force : Skip confirmation prompt}';
    protected $description = 'Run a 2-year daily-transaction accounting scenario (EOM + period-13 adjustments + EOY) with value controls';

    private int $passed = 0;
    private int $failed = 0;
    private int $transCounter = 0;

    private int $y1 = 2025;
    private int $y2 = 2026;
    private int $y3 = 2027; // only its opening balances are produced

    // ─── Account codes (15 chars), grouped by account group ──────────
    const ACC_CASH        = '110000000000000'; // 1 Asset
    const ACC_BANK        = '111000000000000'; // 1 Asset
    const ACC_AR          = '120000000000000'; // 1 Asset
    const ACC_INVENTORY   = '130000000000000'; // 1 Asset
    const ACC_PREPAID     = '140000000000000'; // 1 Asset
    const ACC_EQUIPMENT   = '150000000000000'; // 1 Asset
    const ACC_ACCUM_DEPR  = '160000000000000'; // 1 Asset (contra)
    const ACC_AP          = '210000000000000'; // 2 Liability
    const ACC_ACCRUED_SAL = '220000000000000'; // 2 Liability
    const ACC_CAPITAL     = '310000000000000'; // 3 Equity
    const ACC_REVENUE     = '410000000000000'; // 4 Income
    const ACC_COGS        = '510000000000000'; // 5 COGS
    const ACC_SALARY      = '610000000000000'; // 6 Expense
    const ACC_RENT        = '620000000000000'; // 6 Expense
    const ACC_INSURANCE   = '630000000000000'; // 6 Expense
    const ACC_DEPR_EXP    = '640000000000000'; // 6 Expense
    const ACC_INTEREST    = '710000000000000'; // 7 Other Income
    const ACC_BANK_CHG    = '810000000000000'; // 8 Other Expense

    // ─── Monthly recurring amounts (periods 1-12, both years) ────────
    const CREDIT_SALE = 5000000;  // AR (D) / Revenue (C)
    const CASH_SALE   = 3000000;  // Cash (D) / Revenue (C)
    const COGS_AMT    = 4000000;  // COGS (D) / Inventory (C)
    const PURCHASE    = 6000000;  // Inventory (D) / AP (C)
    const COLLECT_AR  = 4500000;  // Bank (D) / AR (C)
    const SALARY      = 2000000;  // Salary Exp (D) / Cash (C)
    const RENT        = 1000000;  // Rent Exp (D) / Cash (C)
    const PAY_AP      = 5000000;  // AP (D) / Bank (C)
    const INTEREST    = 100000;   // Bank (D) / Interest Income (C)
    const BANK_CHG    = 50000;    // Bank Charges (D) / Bank (C)

    // ─── Year 1 period 1 one-time setup ──────────────────────────────
    const CAPITAL        = 100000000; // Cash (D) / Capital (C)
    const EQUIPMENT      = 50000000;  // Equipment (D) / Cash (C)
    const PREPAID_INS    = 1200000;   // Prepaid Insurance (D) / Cash (C)
    const INIT_INVENTORY = 30000000;  // Inventory (D) / Cash (C)

    // ─── Period-13 adjustment amounts (both years) ───────────────────
    const DEPRECIATION   = 5000000; // Depr Exp (D) / Accum Depr (C)
    const INS_AMORT      = 1200000; // Insurance Exp (D) / Prepaid Insurance (C)
    const ACCRUED_SALARY = 500000;  // Salary Exp (D) / Accrued Salary (C)

    const DIV_CODE   = '0000';
    const TRANS_CODE = 'JV';
    const CURR_CODE  = 'Rp';
    const USER_ID    = 'TSC';

    /** acc_code => acc_group */
    private function accountMap(): array
    {
        return [
            self::ACC_CASH        => ['Cash',                       1],
            self::ACC_BANK        => ['Bank',                       1],
            self::ACC_AR          => ['Accounts Receivable',        1],
            self::ACC_INVENTORY   => ['Inventory',                  1],
            self::ACC_PREPAID     => ['Prepaid Insurance',          1],
            self::ACC_EQUIPMENT   => ['Equipment',                  1],
            self::ACC_ACCUM_DEPR  => ['Accumulated Depreciation',   1],
            self::ACC_AP          => ['Accounts Payable',           2],
            self::ACC_ACCRUED_SAL => ['Accrued Salary',             2],
            self::ACC_CAPITAL     => ['Owner Capital',              3],
            self::ACC_REVENUE     => ['Sales Revenue',              4],
            self::ACC_COGS        => ['Cost of Goods Sold',         5],
            self::ACC_SALARY      => ['Salary Expense',             6],
            self::ACC_RENT        => ['Rent Expense',               6],
            self::ACC_INSURANCE   => ['Insurance Expense',          6],
            self::ACC_DEPR_EXP    => ['Depreciation Expense',       6],
            self::ACC_INTEREST    => ['Interest Income',            7],
            self::ACC_BANK_CHG    => ['Bank Charges',               8],
        ];
    }

    private function accGroup(string $acc): int
    {
        return $this->accountMap()[$acc][1] ?? 0;
    }

    private function realAccounts(): array
    {
        return array_keys(array_filter($this->accountMap(), fn($v) => in_array($v[1], [1, 2, 3])));
    }

    private function nominalAccounts(): array
    {
        return array_keys(array_filter($this->accountMap(), fn($v) => in_array($v[1], [4, 5, 6, 7, 8])));
    }

    public function handle(): int
    {
        $this->info('');
        $this->info('╔══════════════════════════════════════════════════════╗');
        $this->info('║   2-Year Accounting Scenario Test (daily + adjust)   ║');
        $this->info('╚══════════════════════════════════════════════════════╝');
        $this->newLine();

        if (!$this->option('fresh')) {
            $this->error('This command requires the --fresh flag to run.');
            $this->info('Usage: APP_ENV=testing php artisan test:accounting-scenario --fresh --force');
            return 1;
        }

        $dbName = config('database.connections.' . config('database.default') . '.database');
        if (!$this->option('force') && !$this->confirm("This will TRUNCATE transaction/balance/master tables in [{$dbName}]. Continue?")) {
            return 0;
        }

        try {
            $this->section('SETUP');
            $this->cleanTables();
            $this->seedMasterData();
            $this->generateSaldoTemplate($this->y1);

            // ===== YEAR 1 =====
            $this->runFullYear($this->y1);

            // ===== YEAR 2 =====
            $this->runFullYear($this->y2);

            // ===== FINAL: year-3 opening balances =====
            $this->section('YEAR 3 OPENING BALANCES (post Year-2 EOY)');
            $this->verifyOpeningBalances($this->y3);
            $this->verifyAccountingEquation($this->y3);
        } catch (\Throwable $e) {
            $this->error("FATAL: {$e->getMessage()}");
            $this->error($e->getFile() . ':' . $e->getLine());
            $this->error($e->getTraceAsString());
            return 1;
        }

        $this->printSummary();

        return $this->failed > 0 ? 1 : 0;
    }

    // ─── ORCHESTRATION ───────────────────────────────────────────────

    private function runFullYear(int $year): void
    {
        $this->section("YEAR {$year}");

        // Year 2+: verify the opening balances carried by the prior EOY first.
        if ($year !== $this->y1) {
            $this->verifyOpeningBalances($year);
            $this->verifyAccountingEquation($year);
        }

        for ($period = 1; $period <= 12; $period++) {
            $this->section("YEAR {$year} — PERIOD {$period}");

            if ($period === 1) {
                $this->createInitialSetup($year);
            }
            $this->createDailyTransactions($period, $year);

            $this->verifyPreEomPeriodSaldo($period, $year);
            $this->runEOM();
            $this->verifyPeriodSaldo($period, $year);
        }

        // Period 13 — adjustment entries.
        $this->section("YEAR {$year} — PERIOD 13 (ADJUSTMENTS)");
        $param = Parameter::first();
        $this->assertValue("Y{$year} at period 13", 13, (int) $param->period);
        $this->createPeriod13Adjustments($year);
        $this->verifyPreEomPeriodSaldo(13, $year);

        // EOY transfers the period-13 adjustments, then closes the year.
        $this->section("YEAR {$year} — END OF YEAR");
        $this->runEOY();
        $this->verifyPeriodSaldo(13, $year);
        $this->verifyYearEnd($year);
    }

    // ─── SETUP ───────────────────────────────────────────────────────

    private function cleanTables(): void
    {
        $this->info('Cleaning tables...');
        DB::statement('SET session_replication_role = replica');
        DB::table('trans_detail')->delete();
        DB::table('trans_head')->delete();
        DB::table('this_detail')->delete();
        DB::table('this_head')->delete();
        DB::table('mst_saldo_news')->delete();
        DB::table('mst_account')->delete();
        DB::table('account_group_tables')->delete();
        DB::table('mst_division')->delete();
        DB::table('mst_currency')->delete();
        DB::table('mst_transaction')->delete();
        DB::table('parameter')->delete();
        DB::table('mst_period')->delete();
        DB::table('mst_user')->where('user_id', self::USER_ID)->delete();
        DB::statement('SET session_replication_role = DEFAULT');
        $this->info('  Tables cleaned.');
    }

    private function seedMasterData(): void
    {
        $this->info('Seeding master data...');

        DB::table('mst_user')->insert([
            'user_id' => self::USER_ID,
            'user_name' => 'Scenario Test User',
            'email' => 'test-scenario@example.com',
            'password' => Hash::make('password'),
            'actived' => true,
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_division')->insert([
            'div_code' => self::DIV_CODE,
            'div_name' => 'Head Office',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_currency')->insert([
            'curr_code' => self::CURR_CODE,
            'curr_name' => 'Rupiah',
            'rate_system' => 'B',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_transaction')->insert([
            'trans_code' => self::TRANS_CODE,
            'trans_name' => 'Journal Voucher',
            'trans_status' => 'A',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        $groups = [
            ['id' => 1, 'name' => 'Asset'],
            ['id' => 2, 'name' => 'Liability'],
            ['id' => 3, 'name' => 'Equity'],
            ['id' => 4, 'name' => 'Income'],
            ['id' => 5, 'name' => 'COGS'],
            ['id' => 6, 'name' => 'Expense'],
            ['id' => 7, 'name' => 'Other Income'],
            ['id' => 8, 'name' => 'Other Expense'],
        ];
        foreach ($groups as $g) {
            DB::table('account_group_tables')->insert(array_merge($g, [
                'created_at' => now(), 'updated_at' => now(),
            ]));
        }

        foreach ($this->accountMap() as $code => [$name, $group]) {
            DB::table('mst_account')->insert([
                'acc_code' => $code,
                'acc_name' => $name,
                'acc_group' => $group,
                'acc_currency' => self::CURR_CODE,
                'created_at' => now(),
                'updated_at' => now(),
            ]);
        }

        DB::table('parameter')->insert([
            'serial_no' => 'SCN001',
            'company' => 'Scenario Test Company',
            'period' => 1,
            'year' => $this->y1,
            'base_currency' => self::CURR_CODE,
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        for ($pc = 1; $pc <= 3; $pc++) {
            $year = $this->y1 - ($pc - 1);
            $data = [
                'period_code' => $pc,
                'period_name' => (string) $year,
                'created_at' => now(),
                'updated_at' => now(),
            ];
            for ($m = 1; $m <= 13; $m++) {
                $idx = str_pad($m, 2, '0', STR_PAD_LEFT);
                if ($m == 13) {
                    $data["p{$idx}"] = "Adj {$year}";
                    $data["p{$idx}_from"] = Carbon::create($year, 12, 1)->format('Y-m-d');
                    $data["p{$idx}_to"] = Carbon::create($year, 12, 1)->endOfMonth()->format('Y-m-d');
                } else {
                    $data["p{$idx}"] = Carbon::create($year, $m, 1)->format('M Y');
                    $data["p{$idx}_from"] = Carbon::create($year, $m, 1)->format('Y-m-d');
                    $data["p{$idx}_to"] = Carbon::create($year, $m, 1)->endOfMonth()->format('Y-m-d');
                }
            }
            DB::table('mst_period')->insert($data);
        }

        $count = count($this->accountMap());
        $this->info("  Master data seeded ({$count} accounts across 8 groups, 1 division).");
    }

    private function generateSaldoTemplate(int $year): void
    {
        $accounts = DB::table('mst_account')->pluck('acc_code');
        $divisions = DB::table('mst_division')->pluck('div_code');

        $rows = [];
        $now = now()->toDateTimeString();
        foreach ($accounts as $acc) {
            foreach ($divisions as $div) {
                for ($p = 0; $p <= 13; $p++) {
                    $rows[] = [
                        'acc_code' => $acc,
                        'div_code' => $div,
                        'periode' => $p,
                        'tahun' => $year,
                        'debet' => 0,
                        'credit' => 0,
                        'created_at' => $now,
                        'updated_at' => $now,
                    ];
                }
            }
        }
        foreach (array_chunk($rows, 100) as $chunk) {
            DB::table('mst_saldo_news')->insert($chunk);
        }
        $this->info('  mst_saldo_news template generated for ' . $year . ' (' . count($rows) . ' rows).');
    }

    // ─── TRANSACTION CREATION ────────────────────────────────────────

    private function createInitialSetup(int $year): void
    {
        if ($year === $this->y1) {
            $this->info('  Initial setup (capital, equipment, inventory, prepaid insurance)...');
            $date = "{$year}-01-01";
            $this->createJournal($date, 'Owner capital injection', [
                ['acc' => self::ACC_CASH,    'debet' => self::CAPITAL, 'credit' => 0],
                ['acc' => self::ACC_CAPITAL, 'debet' => 0,             'credit' => self::CAPITAL],
            ]);
            $this->createJournal($date, 'Purchase equipment', [
                ['acc' => self::ACC_EQUIPMENT, 'debet' => self::EQUIPMENT, 'credit' => 0],
                ['acc' => self::ACC_CASH,      'debet' => 0,               'credit' => self::EQUIPMENT],
            ]);
            $this->createJournal($date, 'Prepay annual insurance', [
                ['acc' => self::ACC_PREPAID, 'debet' => self::PREPAID_INS, 'credit' => 0],
                ['acc' => self::ACC_CASH,    'debet' => 0,                 'credit' => self::PREPAID_INS],
            ]);
            $this->createJournal($date, 'Initial inventory', [
                ['acc' => self::ACC_INVENTORY, 'debet' => self::INIT_INVENTORY, 'credit' => 0],
                ['acc' => self::ACC_CASH,      'debet' => 0,                    'credit' => self::INIT_INVENTORY],
            ]);
        } else {
            // Year 2+: re-buy annual prepaid insurance only.
            $this->info('  Initial setup (re-buy annual prepaid insurance)...');
            $date = "{$year}-01-01";
            $this->createJournal($date, 'Prepay annual insurance', [
                ['acc' => self::ACC_PREPAID, 'debet' => self::PREPAID_INS, 'credit' => 0],
                ['acc' => self::ACC_CASH,    'debet' => 0,                 'credit' => self::PREPAID_INS],
            ]);
        }
    }

    private function createDailyTransactions(int $period, int $year): void
    {
        $this->info("  Creating 10 daily transactions for {$year}-" . str_pad($period, 2, '0', STR_PAD_LEFT) . '...');
        $d = fn(int $day) => Carbon::create($year, $period, $day)->format('Y-m-d');

        // Day 3 — credit sale
        $this->createJournal($d(3), 'Credit sale', [
            ['acc' => self::ACC_AR,      'debet' => self::CREDIT_SALE, 'credit' => 0],
            ['acc' => self::ACC_REVENUE, 'debet' => 0,                 'credit' => self::CREDIT_SALE],
        ]);
        // Day 5 — cash sale
        $this->createJournal($d(5), 'Cash sale', [
            ['acc' => self::ACC_CASH,    'debet' => self::CASH_SALE, 'credit' => 0],
            ['acc' => self::ACC_REVENUE, 'debet' => 0,               'credit' => self::CASH_SALE],
        ]);
        // Day 7 — cost of goods sold
        $this->createJournal($d(7), 'Cost of goods sold', [
            ['acc' => self::ACC_COGS,      'debet' => self::COGS_AMT, 'credit' => 0],
            ['acc' => self::ACC_INVENTORY, 'debet' => 0,              'credit' => self::COGS_AMT],
        ]);
        // Day 10 — inventory purchase on credit
        $this->createJournal($d(10), 'Inventory purchase on credit', [
            ['acc' => self::ACC_INVENTORY, 'debet' => self::PURCHASE, 'credit' => 0],
            ['acc' => self::ACC_AP,        'debet' => 0,              'credit' => self::PURCHASE],
        ]);
        // Day 15 — collect receivable into bank
        $this->createJournal($d(15), 'Collect receivable', [
            ['acc' => self::ACC_BANK, 'debet' => self::COLLECT_AR, 'credit' => 0],
            ['acc' => self::ACC_AR,   'debet' => 0,                'credit' => self::COLLECT_AR],
        ]);
        // Day 20 — pay salary
        $this->createJournal($d(20), 'Pay salary', [
            ['acc' => self::ACC_SALARY, 'debet' => self::SALARY, 'credit' => 0],
            ['acc' => self::ACC_CASH,   'debet' => 0,            'credit' => self::SALARY],
        ]);
        // Day 22 — pay rent
        $this->createJournal($d(22), 'Pay rent', [
            ['acc' => self::ACC_RENT, 'debet' => self::RENT, 'credit' => 0],
            ['acc' => self::ACC_CASH, 'debet' => 0,          'credit' => self::RENT],
        ]);
        // Day 25 — pay supplier
        $this->createJournal($d(25), 'Pay supplier', [
            ['acc' => self::ACC_AP,   'debet' => self::PAY_AP, 'credit' => 0],
            ['acc' => self::ACC_BANK, 'debet' => 0,            'credit' => self::PAY_AP],
        ]);
        // Day 28 — bank interest received
        $this->createJournal($d(28), 'Bank interest received', [
            ['acc' => self::ACC_BANK,     'debet' => self::INTEREST, 'credit' => 0],
            ['acc' => self::ACC_INTEREST, 'debet' => 0,              'credit' => self::INTEREST],
        ]);
        // Day 28 — bank charge
        $this->createJournal($d(28), 'Bank charge', [
            ['acc' => self::ACC_BANK_CHG, 'debet' => self::BANK_CHG, 'credit' => 0],
            ['acc' => self::ACC_BANK,     'debet' => 0,              'credit' => self::BANK_CHG],
        ]);
    }

    private function createPeriod13Adjustments(int $year): void
    {
        $this->info('  Creating period-13 adjustment entries...');
        $date = "{$year}-12-31";

        $this->createJournal($date, 'Adj: depreciation', [
            ['acc' => self::ACC_DEPR_EXP,   'debet' => self::DEPRECIATION, 'credit' => 0],
            ['acc' => self::ACC_ACCUM_DEPR, 'debet' => 0,                  'credit' => self::DEPRECIATION],
        ]);
        $this->createJournal($date, 'Adj: prepaid insurance amortization', [
            ['acc' => self::ACC_INSURANCE, 'debet' => self::INS_AMORT, 'credit' => 0],
            ['acc' => self::ACC_PREPAID,   'debet' => 0,               'credit' => self::INS_AMORT],
        ]);
        $this->createJournal($date, 'Adj: accrued salary', [
            ['acc' => self::ACC_SALARY,      'debet' => self::ACCRUED_SALARY, 'credit' => 0],
            ['acc' => self::ACC_ACCRUED_SAL, 'debet' => 0,                    'credit' => self::ACCRUED_SALARY],
        ]);
    }

    private function createJournal(string $date, string $memo, array $lines): void
    {
        $this->transCounter++;
        $transNo = 'SC-' . str_pad($this->transCounter, 4, '0', STR_PAD_LEFT);

        JournalTransaction::create([
            'trans_code' => self::TRANS_CODE,
            'trans_no' => $transNo,
            'trans_date' => $date,
            'trans_update' => $date,
            'trans_user' => self::USER_ID,
            'debet' => 0,
            'credit' => 0,
        ]);

        foreach ($lines as $i => $line) {
            TransDetail::create([
                'trans_code' => self::TRANS_CODE,
                'trans_no' => $transNo,
                'trans_count' => $i + 1,
                'acc_code' => $line['acc'],
                'div_code' => self::DIV_CODE,
                'description' => $memo,
                'debet' => $line['debet'],
                'credit' => $line['credit'],
                'curr_code' => self::CURR_CODE,
                'curr_rate' => 1,
                'trans_date' => $date,
            ]);
        }
    }

    // ─── EOM (mirrors EOM.php transfer logic) ────────────────────────

    private function runEOM(): void
    {
        $parameter = Parameter::first();
        $period = $parameter->period; // padded string e.g. '01'
        $year = (int) $parameter->year;
        $periodInt = (int) $period;

        DB::transaction(function () use ($period, $year, $periodInt) {
            $heads = DB::table('trans_head')->get();
            foreach ($heads as $row) {
                $rowArray = (array) $row;
                $rowArray['period'] = $period;
                $exists = DB::table('this_head')
                    ->where('trans_no', $row->trans_no)
                    ->where('trans_date', $row->trans_date)
                    ->exists();
                if (!$exists) {
                    DB::table('this_head')->insert($rowArray);
                }
            }

            $details = DB::table('trans_detail')->get();
            foreach ($details as $row) {
                $rowArray = (array) $row;
                unset($rowArray['id'], $rowArray['trans_system'], $rowArray['trans_yes']);
                $rowArray['period'] = $period;
                $exists = DB::table('this_detail')
                    ->where('trans_no', $row->trans_no)
                    ->where('trans_count', $row->trans_count)
                    ->where('acc_code', $row->acc_code)
                    ->where('div_code', $row->div_code)
                    ->where('trans_date', $row->trans_date)
                    ->exists();
                if (!$exists) {
                    DB::table('this_detail')->insert($rowArray);
                }
            }

            DB::table('trans_head')->delete();
            DB::table('trans_detail')->delete();

            $this->updateSaldoFromTransactions($periodInt, $year, $period);

            $param = Parameter::first();
            $newPeriod = (int) $param->period + 1;
            if ($newPeriod > 13) {
                $param->period = 1;
                $param->year = (int) $param->year + 1;
            } else {
                $param->period = $newPeriod;
            }
            $param->save();
        });

        $param = Parameter::first();
        $this->info("  EOM done — parameter now Year={$param->year}, Period={$param->period}");
    }

    private function updateSaldoFromTransactions(int $periodInt, int $year, string $periodPadded): void
    {
        MstSaldoNew::where('periode', $periodInt)
            ->where('tahun', $year)
            ->update(['debet' => 0, 'credit' => 0]);

        $aggregated = DB::table('this_detail')
            ->select('acc_code', 'div_code')
            ->selectRaw('SUM(COALESCE(debet, 0)) as total_debet')
            ->selectRaw('SUM(COALESCE(credit, 0)) as total_credit')
            ->where('period', $periodPadded)
            ->whereRaw('EXTRACT(YEAR FROM trans_date) = ?', [$year])
            ->groupBy('acc_code', 'div_code')
            ->get();

        foreach ($aggregated as $trans) {
            $existing = MstSaldoNew::where('acc_code', $trans->acc_code)
                ->where('div_code', $trans->div_code)
                ->where('periode', $periodInt)
                ->where('tahun', $year)
                ->first();

            if ($existing) {
                $existing->debet = $trans->total_debet;
                $existing->credit = $trans->total_credit;
                $existing->save();
            } else {
                MstSaldoNew::create([
                    'acc_code' => $trans->acc_code,
                    'div_code' => $trans->div_code,
                    'periode' => $periodInt,
                    'tahun' => $year,
                    'debet' => $trans->total_debet,
                    'credit' => $trans->total_credit,
                ]);
            }
        }
    }

    // ─── EOY (mirrors EOY.php generate logic) ────────────────────────

    private function runEOY(): void
    {
        $this->info('  Running End of Year...');
        $parameter = Parameter::first();
        $currentYear = (int) $parameter->year;
        $nextYear = $currentYear + 1;

        if ((int) $parameter->period !== 13) {
            $this->error("  Expected period 13, got {$parameter->period}");
            $this->failed++;
            return;
        }

        DB::transaction(function () use ($parameter, $currentYear, $nextYear) {
            if (DB::table('trans_head')->count() > 0) {
                $this->transferAdjustments($parameter);
            }
            $reAccCode = $this->createRetainedEarningsAccount($currentYear);
            $this->genNextYearSaldo($currentYear, $nextYear, $reAccCode);
            $parameter->period = 1;
            $parameter->year = $nextYear;
            $parameter->save();
            $this->updatePeriodTable($nextYear);
        });

        $param = Parameter::first();
        $this->info("  EOY done — parameter now Year={$param->year}, Period={$param->period}");
    }

    private function transferAdjustments($parameter): void
    {
        $period = $parameter->period;

        $heads = DB::table('trans_head')->get();
        foreach ($heads as $row) {
            $rowArray = (array) $row;
            $rowArray['period'] = $period;
            $exists = DB::table('this_head')
                ->where('trans_no', $row->trans_no)
                ->where('trans_date', $row->trans_date)
                ->exists();
            if (!$exists) {
                DB::table('this_head')->insert($rowArray);
            }
        }

        $details = DB::table('trans_detail')->get();
        foreach ($details as $row) {
            $rowArray = (array) $row;
            unset($rowArray['id'], $rowArray['trans_system'], $rowArray['trans_yes']);
            $rowArray['period'] = $period;
            $exists = DB::table('this_detail')
                ->where('trans_no', $row->trans_no)
                ->where('trans_count', $row->trans_count)
                ->where('acc_code', $row->acc_code)
                ->where('div_code', $row->div_code)
                ->where('trans_date', $row->trans_date)
                ->exists();
            if (!$exists) {
                DB::table('this_detail')->insert($rowArray);
            }
        }

        DB::table('trans_head')->delete();
        DB::table('trans_detail')->delete();

        $this->updateSaldoFromTransactions((int) $period, (int) $parameter->year, $period);
    }

    private function createRetainedEarningsAccount(int $year): string
    {
        $existing = ChartOfAccount::where('acc_name', 'ilike', "%Retained Earning%{$year}%")->first();
        if ($existing) {
            return $existing->acc_code;
        }

        $lastRE = ChartOfAccount::where('acc_code', 'like', '38888888%')
            ->where('acc_code', '!=', '388888880000000')
            ->orderBy('acc_code', 'desc')
            ->first();

        $lastIndex = $lastRE ? (int) substr($lastRE->acc_code, -3) : 0;
        $newIndex = str_pad($lastIndex + 1, 3, '0', STR_PAD_LEFT);
        $newAccCode = '388888880000' . $newIndex;

        ChartOfAccount::create([
            'acc_code' => $newAccCode,
            'acc_name' => "Retained Earning of {$year}",
            'acc_group' => 3,
            'acc_currency' => self::CURR_CODE,
        ]);

        $this->info("  RE account created: {$newAccCode} (Retained Earning of {$year})");
        return $newAccCode;
    }

    private function genNextYearSaldo(int $currentYear, int $nextYear, string $reAccCode): void
    {
        DB::statement("
            INSERT INTO mst_saldo_news (acc_code, div_code, periode, tahun, debet, credit, created_at, updated_at)
            SELECT coa.acc_code, divs.div_code, gs.periode, :nextYear, 0, 0, NOW(), NOW()
            FROM mst_account coa
            CROSS JOIN mst_division divs
            CROSS JOIN generate_series(0, 13) AS gs(periode)
            LEFT JOIN mst_saldo_news ex
                ON ex.acc_code = coa.acc_code
                AND ex.div_code = divs.div_code
                AND ex.periode = gs.periode
                AND ex.tahun = :nextYear2
            WHERE ex.id IS NULL
        ", ['nextYear' => $nextYear, 'nextYear2' => $nextYear]);

        DB::statement("
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
        ", ['currentYear' => $currentYear, 'nextYear' => $nextYear]);

        $netIncome = (float) DB::table('mst_saldo_news')
            ->join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->whereIn('mst_account.acc_group', [4, 5, 6, 7, 8])
            ->where('mst_saldo_news.tahun', $currentYear)
            ->selectRaw('COALESCE(SUM(mst_saldo_news.credit) - SUM(mst_saldo_news.debet), 0) as net')
            ->value('net');

        $this->info('  Net income ' . $currentYear . ': ' . number_format($netIncome, 2));

        $reSaldo = MstSaldoNew::where('acc_code', $reAccCode)
            ->where('div_code', self::DIV_CODE)
            ->where('periode', 0)
            ->where('tahun', $nextYear)
            ->first();

        if ($reSaldo) {
            if ($netIncome >= 0) {
                $reSaldo->credit += $netIncome;
            } else {
                $reSaldo->debet += abs($netIncome);
            }
            $reSaldo->save();
        } else {
            $this->error('  RE period-0 saldo row missing — net income not posted!');
            $this->failed++;
        }
    }

    private function updatePeriodTable(int $nextYear): void
    {
        $periods = DB::table('mst_period')->whereIn('period_code', [1, 2, 3])->get();
        foreach ($periods as $period) {
            $year = match ((int) $period->period_code) {
                1 => $nextYear,
                2 => $nextYear - 1,
                3 => $nextYear - 2,
            };
            $data = ['period_name' => (string) $year, 'updated_at' => now()];
            for ($i = 1; $i <= 13; $i++) {
                $idx = str_pad($i, 2, '0', STR_PAD_LEFT);
                if ($i == 13) {
                    $data["p{$idx}"] = "Adj {$year}";
                    $data["p{$idx}_from"] = Carbon::create($year, 12, 1)->format('Y-m-d');
                    $data["p{$idx}_to"] = Carbon::create($year, 12, 1)->endOfMonth()->format('Y-m-d');
                } else {
                    $data["p{$idx}"] = Carbon::create($year, $i, 1)->format('M Y');
                    $data["p{$idx}_from"] = Carbon::create($year, $i, 1)->format('Y-m-d');
                    $data["p{$idx}_to"] = Carbon::create($year, $i, 1)->endOfMonth()->format('Y-m-d');
                }
            }
            DB::table('mst_period')->where('id', $period->id)->update($data);
        }
    }

    // ─── EXPECTED-VALUE MODEL (the "controls") ───────────────────────

    /**
     * Expected [debet, credit] movement posted to mst_saldo_news for an
     * account in a given period/year. period 0 = opening balance.
     */
    private function expectedPeriodSaldo(string $acc, int $period, int $year): array
    {
        if ($period === 0) {
            return $this->expectedOpening($acc, $year);
        }

        $d = 0.0;
        $c = 0.0;

        // Monthly recurring (periods 1-12)
        if ($period >= 1 && $period <= 12) {
            switch ($acc) {
                case self::ACC_CASH:      $d += self::CASH_SALE;  $c += self::SALARY + self::RENT; break;
                case self::ACC_BANK:      $d += self::COLLECT_AR + self::INTEREST; $c += self::PAY_AP + self::BANK_CHG; break;
                case self::ACC_AR:        $d += self::CREDIT_SALE; $c += self::COLLECT_AR; break;
                case self::ACC_INVENTORY: $d += self::PURCHASE;   $c += self::COGS_AMT; break;
                case self::ACC_AP:        $d += self::PAY_AP;     $c += self::PURCHASE; break;
                case self::ACC_REVENUE:   $c += self::CREDIT_SALE + self::CASH_SALE; break;
                case self::ACC_COGS:      $d += self::COGS_AMT; break;
                case self::ACC_SALARY:    $d += self::SALARY; break;
                case self::ACC_RENT:      $d += self::RENT; break;
                case self::ACC_INTEREST:  $c += self::INTEREST; break;
                case self::ACC_BANK_CHG:  $d += self::BANK_CHG; break;
            }
        }

        // Period 1 one-time setup
        if ($period === 1) {
            if ($year === $this->y1) {
                switch ($acc) {
                    case self::ACC_CASH:      $d += self::CAPITAL; $c += self::EQUIPMENT + self::PREPAID_INS + self::INIT_INVENTORY; break;
                    case self::ACC_EQUIPMENT: $d += self::EQUIPMENT; break;
                    case self::ACC_PREPAID:   $d += self::PREPAID_INS; break;
                    case self::ACC_INVENTORY: $d += self::INIT_INVENTORY; break;
                    case self::ACC_CAPITAL:   $c += self::CAPITAL; break;
                }
            } else {
                switch ($acc) {
                    case self::ACC_CASH:    $c += self::PREPAID_INS; break;
                    case self::ACC_PREPAID: $d += self::PREPAID_INS; break;
                }
            }
        }

        // Period 13 adjustments
        if ($period === 13) {
            switch ($acc) {
                case self::ACC_DEPR_EXP:    $d += self::DEPRECIATION; break;
                case self::ACC_ACCUM_DEPR:  $c += self::DEPRECIATION; break;
                case self::ACC_INSURANCE:   $d += self::INS_AMORT; break;
                case self::ACC_PREPAID:     $c += self::INS_AMORT; break;
                case self::ACC_SALARY:      $d += self::ACCRUED_SALARY; break;
                case self::ACC_ACCRUED_SAL: $c += self::ACCRUED_SALARY; break;
            }
        }

        return [$d, $c];
    }

    /**
     * Expected period-0 (opening) [debet, credit] for an account in a year.
     * Year 1 starts fresh (all zero). Year N>1 carries forward the sum of
     * all prior-year periods for REAL accounts; nominal accounts reset.
     */
    private function expectedOpening(string $acc, int $year): array
    {
        if ($year === $this->y1) {
            return [0.0, 0.0];
        }

        // Nominal accounts always reset at year start.
        if (in_array($this->accGroup($acc), [4, 5, 6, 7, 8])) {
            return [0.0, 0.0];
        }

        // Real account: sum of all prior-year periods 0-13.
        $priorYear = $year - 1;
        $d = 0.0;
        $c = 0.0;
        for ($p = 0; $p <= 13; $p++) {
            [$pd, $pc] = $this->expectedPeriodSaldo($acc, $p, $priorYear);
            $d += $pd;
            $c += $pc;
        }
        return [$d, $c];
    }

    /** Net income for a year = SUM(credit - debet) over nominal accounts, periods 1-13. */
    private function expectedNetIncome(int $year): float
    {
        $net = 0.0;
        foreach ($this->nominalAccounts() as $acc) {
            for ($p = 1; $p <= 13; $p++) {
                [$d, $c] = $this->expectedPeriodSaldo($acc, $p, $year);
                $net += $c - $d;
            }
        }
        return $net;
    }

    // ─── VERIFICATION (the "controls") ───────────────────────────────

    /** Before EOM: TransDetail::created events should have built the period saldo. */
    private function verifyPreEomPeriodSaldo(int $period, int $year): void
    {
        $this->info("  Verifying pre-EOM saldo (events) for period {$period}...");
        foreach (array_keys($this->accountMap()) as $acc) {
            [$expD, $expC] = $this->expectedPeriodSaldo($acc, $period, $year);
            $saldo = $this->getSaldo($acc, $period, $year);
            $this->assertValue("  pre-EOM {$this->shortName($acc)} P{$period} D", $expD, (float) $saldo->debet);
            $this->assertValue("  pre-EOM {$this->shortName($acc)} P{$period} C", $expC, (float) $saldo->credit);
        }
    }

    /** After EOM/EOY: mst_saldo_news for the period must equal the expected movement. */
    private function verifyPeriodSaldo(int $period, int $year): void
    {
        $this->info("  Verifying mst_saldo_news for period {$period}...");
        foreach (array_keys($this->accountMap()) as $acc) {
            [$expD, $expC] = $this->expectedPeriodSaldo($acc, $period, $year);
            $saldo = $this->getSaldo($acc, $period, $year);
            $this->assertValue("  {$this->shortName($acc)} P{$period} D", $expD, (float) $saldo->debet);
            $this->assertValue("  {$this->shortName($acc)} P{$period} C", $expC, (float) $saldo->credit);
        }
    }

    /** Opening balances (period 0) at the start of a year. */
    private function verifyOpeningBalances(int $year): void
    {
        $this->info("  Verifying period-0 opening balances for {$year}...");
        foreach (array_keys($this->accountMap()) as $acc) {
            [$expD, $expC] = $this->expectedOpening($acc, $year);
            $saldo = $this->getSaldo($acc, 0, $year);
            $this->assertValue("  open {$this->shortName($acc)} {$year} D", $expD, (float) $saldo->debet);
            $this->assertValue("  open {$this->shortName($acc)} {$year} C", $expC, (float) $saldo->credit);
        }

        // Retained earnings accounts: each closed year posts net income to its
        // own RE account's period-0 row, and that carries forward unchanged.
        $reAccounts = ChartOfAccount::where('acc_code', 'like', '38888888%')
            ->where('acc_code', '!=', '388888880000000')
            ->get();
        foreach ($reAccounts as $re) {
            if (preg_match('/(\d{4})/', $re->acc_name, $m)) {
                $reYear = (int) $m[1];
                if ($reYear < $year) {
                    $saldo = $this->getSaldo($re->acc_code, 0, $year);
                    $this->assertValue("  open RE {$reYear} in {$year} C", $this->expectedNetIncome($reYear), (float) $saldo->credit);
                }
            }
        }
    }

    /** Year-end checks: net income, carry-forward, nominal reset, RE posting. */
    private function verifyYearEnd(int $year): void
    {
        $this->info("  Verifying year-end {$year} results...");
        $nextYear = $year + 1;

        // Net income computed from saldo equals the deterministic expectation.
        $expectedNet = $this->expectedNetIncome($year);
        $actualNet = (float) MstSaldoNew::join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->whereIn('mst_account.acc_group', [4, 5, 6, 7, 8])
            ->where('mst_saldo_news.tahun', $year)
            ->selectRaw('COALESCE(SUM(mst_saldo_news.credit) - SUM(mst_saldo_news.debet), 0) as net')
            ->value('net');
        $this->assertValue("  Net income {$year}", $expectedNet, $actualNet);

        // RE account for this year exists and holds net income at next-year P0.
        $reAcc = ChartOfAccount::where('acc_name', 'ilike', "%Retained Earning%{$year}%")->first();
        $this->assertValue("  RE account for {$year} exists", true, $reAcc !== null);
        if ($reAcc) {
            $reSaldo = $this->getSaldo($reAcc->acc_code, 0, $nextYear);
            $this->assertValue("  RE {$year} posted at P0 {$nextYear} C", $expectedNet, (float) $reSaldo->credit);
            $this->assertValue("  RE {$year} posted at P0 {$nextYear} D", 0.0, (float) $reSaldo->debet);
        }

        // Nominal accounts reset to zero in next year's period 0.
        foreach ($this->nominalAccounts() as $acc) {
            $saldo = $this->getSaldo($acc, 0, $nextYear);
            $this->assertValue("  nominal {$this->shortName($acc)} reset P0 {$nextYear} D", 0.0, (float) $saldo->debet);
            $this->assertValue("  nominal {$this->shortName($acc)} reset P0 {$nextYear} C", 0.0, (float) $saldo->credit);
        }

        // Parameter advanced to next year, period 1.
        $param = Parameter::first();
        $this->assertValue("  parameter year after EOY", $nextYear, (int) $param->year);
        $this->assertValue("  parameter period after EOY", 1, (int) $param->period);
    }

    /** Assets = Liabilities + Equity at period 0 of a year (incl. RE accounts). */
    private function verifyAccountingEquation(int $year): void
    {
        $assets = $this->p0GroupNet([1], $year, 'D');
        $liabilities = $this->p0GroupNet([2], $year, 'C');
        $equity = $this->p0GroupNet([3], $year, 'C'); // includes RE accounts

        $this->assertValue(
            "  Accounting equation A = L + E ({$year} P0)",
            round($assets, 2),
            round($liabilities + $equity, 2)
        );
    }

    // ─── QUERY HELPERS ───────────────────────────────────────────────

    private function getSaldo(string $accCode, int $period, int $year): object
    {
        $saldo = MstSaldoNew::where('acc_code', $accCode)
            ->where('div_code', self::DIV_CODE)
            ->where('periode', $period)
            ->where('tahun', $year)
            ->first();

        return $saldo ?? (object) ['debet' => 0, 'credit' => 0];
    }

    private function p0GroupNet(array $groups, int $year, string $normalBalance): float
    {
        $result = MstSaldoNew::join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->whereIn('mst_account.acc_group', $groups)
            ->where('mst_saldo_news.tahun', $year)
            ->where('mst_saldo_news.periode', 0)
            ->selectRaw('COALESCE(SUM(mst_saldo_news.debet), 0) as total_d, COALESCE(SUM(mst_saldo_news.credit), 0) as total_c')
            ->first();

        $d = (float) $result->total_d;
        $c = (float) $result->total_c;

        return $normalBalance === 'C' ? ($c - $d) : ($d - $c);
    }

    private function shortName(string $acc): string
    {
        $name = $this->accountMap()[$acc][0] ?? $acc;
        // Compact label for readable output.
        $abbr = [
            'Cash' => 'Cash', 'Bank' => 'Bank', 'Accounts Receivable' => 'AR',
            'Inventory' => 'Inv', 'Prepaid Insurance' => 'Prepaid', 'Equipment' => 'Equip',
            'Accumulated Depreciation' => 'AccDepr', 'Accounts Payable' => 'AP',
            'Accrued Salary' => 'AccrSal', 'Owner Capital' => 'Capital',
            'Sales Revenue' => 'Revenue', 'Cost of Goods Sold' => 'COGS',
            'Salary Expense' => 'Salary', 'Rent Expense' => 'Rent',
            'Insurance Expense' => 'Insurance', 'Depreciation Expense' => 'DeprExp',
            'Interest Income' => 'Interest', 'Bank Charges' => 'BankChg',
        ];
        return $abbr[$name] ?? $name;
    }

    // ─── ASSERTION & OUTPUT ──────────────────────────────────────────

    private function assertValue(string $label, mixed $expected, mixed $actual): void
    {
        if (is_float($expected) || is_float($actual)) {
            $match = abs((float) $expected - (float) $actual) < 0.01;
        } else {
            $match = $expected === $actual;
        }

        if ($match) {
            $this->passed++;
            $displayVal = is_bool($expected)
                ? ($expected ? 'true' : 'false')
                : number_format((float) $expected, 2, ',', '.');
            $this->line("  <fg=green>✓</> {$label}: {$displayVal}");
        } else {
            $this->failed++;
            $displayExpected = is_bool($expected) ? ($expected ? 'true' : 'false') : $expected;
            $displayActual = is_bool($actual) ? ($actual ? 'true' : 'false') : $actual;
            $this->line("  <fg=red>✗</> {$label}: expected={$displayExpected} actual={$displayActual}");
        }
    }

    private function section(string $title): void
    {
        $this->newLine();
        $this->info("━━━ [{$title}] " . str_repeat('━', max(0, 50 - strlen($title))));
    }

    private function printSummary(): void
    {
        $total = $this->passed + $this->failed;
        $this->newLine();
        $this->info('╔══════════════════════════════════════════════════════╗');
        $this->info('║   SUMMARY                                            ║');
        $this->info('╠══════════════════════════════════════════════════════╣');
        $this->info("║   Total checks: {$total}");
        $this->info("║   <fg=green>Passed: {$this->passed}</>");
        if ($this->failed > 0) {
            $this->info("║   <fg=red>Failed: {$this->failed}</>");
        } else {
            $this->info("║   Failed: 0");
        }
        $this->info('╚══════════════════════════════════════════════════════╝');

        if ($this->failed === 0) {
            $this->newLine();
            $this->info('<fg=green;options=bold>ALL CONTROLS PASSED!</>');
        } else {
            $this->newLine();
            $this->error("FAILED: {$this->failed} control(s) did not pass.");
        }
    }
}
