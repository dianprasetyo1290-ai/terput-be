<?php

namespace App\Console\Commands;

use App\Models\Balance;
use App\Models\ChartOfAccount;
use App\Models\JournalTransaction;
use App\Models\MstSaldoNew;
use App\Models\Parameter;
use App\Models\TransDetail;
use Carbon\Carbon;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Hash;

class TestAccountingCycle extends Command
{
    protected $signature = 'test:accounting-cycle {--fresh : Truncate tables and start fresh} {--force : Skip confirmation prompt}';
    protected $description = 'Run end-to-end accounting cycle test: 12 months + EOY + new year verification';

    private int $passed = 0;
    private int $failed = 0;
    private int $testYear = 2025;
    private int $transCounter = 0;

    // Test account codes (15 chars)
    const ACC_CASH    = '110000000000000';
    const ACC_AR      = '120000000000000';
    const ACC_AP      = '210000000000000';
    const ACC_EQUITY  = '310000000000000';
    const ACC_REVENUE = '410000000000000';
    const ACC_COGS    = '510000000000000';
    const ACC_RENT    = '610000000000000';
    const ACC_SALARY  = '620000000000000';

    const DIV_CODE   = '0000';
    const TRANS_CODE = 'JV';
    const CURR_CODE  = 'Rp';
    const USER_ID    = 'TST';

    public function handle(): int
    {
        $this->info('');
        $this->info('╔══════════════════════════════════════════════════════╗');
        $this->info('║   End-to-End Accounting Cycle Test                   ║');
        $this->info('╚══════════════════════════════════════════════════════╝');
        $this->newLine();

        if (!$this->option('fresh')) {
            $this->error('This command requires the --fresh flag to run.');
            $this->info('Usage: php artisan test:accounting-cycle --fresh');
            return 1;
        }

        $dbName = config('database.connections.' . config('database.default') . '.database');
        if (!$this->option('force') && !$this->confirm("This will TRUNCATE transaction/balance/master tables in [{$dbName}]. Continue?")) {
            return 0;
        }

        try {
            // === SETUP ===
            $this->section('SETUP');
            $this->cleanTables();
            $this->seedMasterData();
            $this->generateSaldoTemplate();
            $this->validateSeedData();

            // === 12 MONTHLY PERIODS ===
            for ($period = 1; $period <= 12; $period++) {
                $this->section("PERIOD {$period}");
                $this->createPeriodTransactions($period);
                $this->verifyPreEOMSaldo($period);
                $this->runEOM();
                $this->verifySaldoAfterEOM($period);
                $this->verifyBSValues($period);
                $this->verifyISValues($period);
            }

            // === QUARTERLY VERIFICATION ===
            $this->section('QUARTERLY VERIFICATION');
            $this->verifyQuarterlyReports();

            // === EOY ===
            $this->section('END OF YEAR');
            $this->runEOY();
            $this->verifyEOY();

            // === YEAR 2 ===
            $this->section('YEAR 2 (2026)');
            $this->testNewYear();

        } catch (\Throwable $e) {
            $this->error("FATAL: {$e->getMessage()}");
            $this->error($e->getFile() . ':' . $e->getLine());
            $this->error($e->getTraceAsString());
            return 1;
        }

        // === SUMMARY ===
        $this->printSummary();

        return $this->failed > 0 ? 1 : 0;
    }

    // ─── SETUP METHODS ────────────────────────────────────────────

    private function cleanTables(): void
    {
        $this->info('Cleaning tables...');

        // Disable FK checks for PostgreSQL
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
        // Clean test user only
        DB::table('mst_user')->where('user_id', self::USER_ID)->delete();

        // Re-enable FK checks
        DB::statement('SET session_replication_role = DEFAULT');

        $this->info('  Tables cleaned.');
    }

    private function seedMasterData(): void
    {
        $this->info('Seeding master data...');

        // User
        DB::table('mst_user')->insert([
            'user_id' => self::USER_ID,
            'user_name' => 'Test User',
            'email' => 'test-cycle@example.com',
            'password' => Hash::make('password'),
            'actived' => true,
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        // Division (no FK dependencies)
        DB::table('mst_division')->insert([
            'div_code' => self::DIV_CODE,
            'div_name' => 'Head Office',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        // Currency (needed before mst_account due to FK acc_currency → curr_code)
        DB::table('mst_currency')->insert([
            'curr_code' => self::CURR_CODE,
            'curr_name' => 'Rupiah',
            'rate_system' => 'B',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        // Transaction Type (needed before trans_head/trans_detail)
        DB::table('mst_transaction')->insert([
            'trans_code' => self::TRANS_CODE,
            'trans_name' => 'Journal Voucher',
            'trans_status' => 'A',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        // Account Groups
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

        // Chart of Accounts (after currency for FK)
        $accounts = [
            ['acc_code' => self::ACC_CASH,    'acc_name' => 'Cash',               'acc_group' => 1],
            ['acc_code' => self::ACC_AR,      'acc_name' => 'Accounts Receivable', 'acc_group' => 1],
            ['acc_code' => self::ACC_AP,      'acc_name' => 'Accounts Payable',    'acc_group' => 2],
            ['acc_code' => self::ACC_EQUITY,  'acc_name' => 'Owner Equity',        'acc_group' => 3],
            ['acc_code' => self::ACC_REVENUE, 'acc_name' => 'Service Revenue',     'acc_group' => 4],
            ['acc_code' => self::ACC_COGS,    'acc_name' => 'Cost of Goods Sold',  'acc_group' => 5],
            ['acc_code' => self::ACC_RENT,    'acc_name' => 'Rent Expense',        'acc_group' => 6],
            ['acc_code' => self::ACC_SALARY,  'acc_name' => 'Salary Expense',      'acc_group' => 6],
        ];
        foreach ($accounts as $a) {
            DB::table('mst_account')->insert(array_merge($a, [
                'acc_currency' => self::CURR_CODE,
                'created_at' => now(),
                'updated_at' => now(),
            ]));
        }

        // Parameter
        DB::table('parameter')->insert([
            'serial_no' => 'TEST001',
            'company' => 'Test Company',
            'period' => 1,
            'year' => $this->testYear,
            'base_currency' => self::CURR_CODE,
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        // Periods (3 rows: current, prior, 2-year-back)
        for ($pc = 1; $pc <= 3; $pc++) {
            $year = $this->testYear - ($pc - 1);
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

        $this->info('  Master data seeded (8 accounts, 1 division, 1 currency, 1 trans type).');
    }

    private function generateSaldoTemplate(): void
    {
        $this->info('Generating mst_saldo_news template...');
        $accounts = DB::table('mst_account')->pluck('acc_code');
        $divisions = DB::table('mst_division')->pluck('div_code');

        $rows = [];
        $now = now()->toDateTimeString();
        foreach ($accounts as $acc) {
            foreach ($divisions as $div) {
                // Periods 0-13 (0=beginning, 1-12=months, 13=adjustment)
                for ($p = 0; $p <= 13; $p++) {
                    $rows[] = [
                        'acc_code' => $acc,
                        'div_code' => $div,
                        'periode' => $p,
                        'tahun' => $this->testYear,
                        'debet' => 0,
                        'credit' => 0,
                        'created_at' => $now,
                        'updated_at' => $now,
                    ];
                }
            }
        }
        // Batch insert
        foreach (array_chunk($rows, 100) as $chunk) {
            DB::table('mst_saldo_news')->insert($chunk);
        }

        $count = count($rows);
        $this->info("  Generated {$count} saldo template rows (periods 0-13).");
        $this->info("  Parameter: Year={$this->testYear}, Period=1");
    }

    private function validateSeedData(): void
    {
        $this->info('Validating seed data before transactions...');

        $checks = [
            ['table' => 'mst_user',            'expected' => 1, 'label' => 'Users'],
            ['table' => 'account_group_tables', 'expected' => 8, 'label' => 'Account Groups'],
            ['table' => 'mst_account',          'expected' => 8, 'label' => 'Chart of Accounts'],
            ['table' => 'mst_division',         'expected' => 1, 'label' => 'Divisions'],
            ['table' => 'mst_currency',         'expected' => 1, 'label' => 'Currencies'],
            ['table' => 'mst_transaction',      'expected' => 1, 'label' => 'Transaction Types'],
            ['table' => 'parameter',            'expected' => 1, 'label' => 'Parameter'],
            ['table' => 'mst_period',           'expected' => 3, 'label' => 'Periods'],
        ];

        $allOk = true;
        foreach ($checks as $check) {
            $count = DB::table($check['table'])->count();
            if ($count >= $check['expected']) {
                $this->line("  <fg=green>✓</> {$check['label']}: {$count} rows");
            } else {
                $this->line("  <fg=red>✗</> {$check['label']}: expected >={$check['expected']}, got {$count}");
                $allOk = false;
            }
        }

        // Verify saldo template
        $saldoCount = DB::table('mst_saldo_news')->count();
        $expectedSaldo = 8 * 1 * 14; // 8 accounts x 1 division x 14 periods (0-13)
        if ($saldoCount >= $expectedSaldo) {
            $this->line("  <fg=green>✓</> Saldo template: {$saldoCount} rows");
        } else {
            $this->line("  <fg=red>✗</> Saldo template: expected >={$expectedSaldo}, got {$saldoCount}");
            $allOk = false;
        }

        // Verify Parameter values
        $param = Parameter::first();
        if ($param) {
            $paramOk = (int) $param->period === 1 && (int) $param->year === $this->testYear;
            if ($paramOk) {
                $this->line("  <fg=green>✓</> Parameter: Year={$param->year}, Period={$param->period}");
            } else {
                $this->line("  <fg=red>✗</> Parameter: Year={$param->year}, Period={$param->period} (expected Year={$this->testYear}, Period=01)");
                $allOk = false;
            }
        }

        // Verify all test accounts exist
        $testAccounts = [self::ACC_CASH, self::ACC_AR, self::ACC_AP, self::ACC_EQUITY,
                         self::ACC_REVENUE, self::ACC_COGS, self::ACC_RENT, self::ACC_SALARY];
        foreach ($testAccounts as $acc) {
            $exists = DB::table('mst_account')->where('acc_code', $acc)->exists();
            if (!$exists) {
                $this->line("  <fg=red>✗</> Account {$acc} missing!");
                $allOk = false;
            }
        }

        // Verify trans_head and trans_detail are empty
        $transCount = DB::table('trans_head')->count() + DB::table('trans_detail')->count();
        $histCount = DB::table('this_head')->count() + DB::table('this_detail')->count();
        if ($transCount === 0 && $histCount === 0) {
            $this->line("  <fg=green>✓</> Transaction tables: empty (clean start)");
        } else {
            $this->line("  <fg=red>✗</> Transaction tables not empty: trans={$transCount}, hist={$histCount}");
            $allOk = false;
        }

        if (!$allOk) {
            throw new \RuntimeException('Seed data validation failed. Cannot proceed with testing.');
        }

        $this->info('  All seed data validated successfully.');
    }

    // ─── TRANSACTION CREATION ─────────────────────────────────────

    private function getRevenueAmount(int $period): float
    {
        return 10000 + ($period - 1) * 3000;
    }

    private function getRentAmount(): float
    {
        return 2000;
    }

    private function getSalaryAmount(int $period): float
    {
        return 3000 + ($period - 1) * 500;
    }

    private function createPeriodTransactions(int $period): void
    {
        $revenue = $this->getRevenueAmount($period);
        $rent = $this->getRentAmount();
        $salary = $this->getSalaryAmount($period);

        $this->info("Creating 3 transactions (Rev:{$revenue}, Rent:{$rent}, Sal:{$salary})...");

        $date = Carbon::create($this->testYear, $period, 15)->format('Y-m-d');

        // JV 1: Cash (D) / Revenue (C)
        $this->createJournal($date, 'Revenue receipt', [
            ['acc' => self::ACC_CASH,    'debet' => $revenue, 'credit' => 0,        'desc' => 'Cash receipt'],
            ['acc' => self::ACC_REVENUE, 'debet' => 0,        'credit' => $revenue, 'desc' => 'Service revenue'],
        ]);

        // JV 2: Rent (D) / Cash (C)
        $this->createJournal($date, 'Rent payment', [
            ['acc' => self::ACC_RENT, 'debet' => $rent, 'credit' => 0,     'desc' => 'Rent expense'],
            ['acc' => self::ACC_CASH, 'debet' => 0,     'credit' => $rent, 'desc' => 'Cash payment'],
        ]);

        // JV 3: Salary (D) / Cash (C)
        $this->createJournal($date, 'Salary payment', [
            ['acc' => self::ACC_SALARY, 'debet' => $salary, 'credit' => 0,       'desc' => 'Salary expense'],
            ['acc' => self::ACC_CASH,   'debet' => 0,       'credit' => $salary, 'desc' => 'Cash payment'],
        ]);
    }

    private function createJournal(string $date, string $memo, array $lines): void
    {
        $this->transCounter++;
        $transNo = 'JV-' . str_pad($this->transCounter, 3, '0', STR_PAD_LEFT);

        // Create header (totals will be auto-updated by TransDetail::created event)
        JournalTransaction::create([
            'trans_code' => self::TRANS_CODE,
            'trans_no' => $transNo,
            'trans_date' => $date,
            'trans_update' => $date,
            'trans_user' => self::USER_ID,
            'debet' => 0,
            'credit' => 0,
        ]);

        // Create detail lines (each triggers created event → updates mst_saldo_news)
        foreach ($lines as $i => $line) {
            TransDetail::create([
                'trans_code' => self::TRANS_CODE,
                'trans_no' => $transNo,
                'trans_count' => $i + 1,
                'acc_code' => $line['acc'],
                'div_code' => self::DIV_CODE,
                'description' => $line['desc'],
                'debet' => $line['debet'],
                'credit' => $line['credit'],
                'curr_code' => self::CURR_CODE,
                'curr_rate' => 1,
                'trans_date' => $date,
            ]);
        }
    }

    // ─── EOM PROCESS (mirrors EOM.php transfer logic) ─────────────

    private function runEOM(): void
    {
        $parameter = Parameter::first();
        $period = $parameter->period; // padded string e.g. '01'
        $year = (int) $parameter->year;
        $periodInt = (int) $period;

        DB::transaction(function () use ($period, $year, $periodInt) {
            // 1. Copy trans_head → this_head
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

            // 2. Copy trans_detail → this_detail
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

            // 3. Delete current transactions (raw — no Eloquent events)
            DB::table('trans_head')->delete();
            DB::table('trans_detail')->delete();

            // 4. Reset & re-set saldo from this_detail (THE FIX)
            $this->updateSaldoFromTransactions($periodInt, $year, $period);

            // 5. Increment period
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
        $this->info("  EOM complete. Parameter now: Year={$param->year}, Period={$param->period}");
    }

    private function updateSaldoFromTransactions(int $periodInt, int $year, string $periodPadded): void
    {
        // Reset saldo for this period (prevents double-counting)
        MstSaldoNew::where('periode', $periodInt)
            ->where('tahun', $year)
            ->update(['debet' => 0, 'credit' => 0]);

        // Aggregate from this_detail (source of truth)
        // Filter by year via trans_date to prevent cross-year contamination
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

    // ─── EOY PROCESS (mirrors EOY.php generate logic) ─────────────

    private function runEOY(): void
    {
        $this->info('Running End of Year process...');

        $parameter = Parameter::first();
        $currentYear = (int) $parameter->year;
        $nextYear = $currentYear + 1;

        // Verify we're at period 13
        $currentPeriod = (int) $parameter->period;
        if ($currentPeriod !== 13) {
            $this->error("  Expected period 13, got {$currentPeriod}");
            $this->failed++;
            return;
        }

        DB::transaction(function () use ($parameter, $currentYear, $nextYear) {
            // Step 1: Transfer adjustments (period 13) — none in our test
            $transCount = DB::table('trans_head')->count();
            if ($transCount > 0) {
                $this->transferAdjustments($parameter);
            }

            // Step 2: Create Retained Earnings account
            $reAccCode = $this->createRetainedEarningsAccount($currentYear);

            // Step 3: Generate next year saldo
            $this->genNextYearSaldo($currentYear, $nextYear, $reAccCode);

            // Step 4: Update parameter
            $parameter->period = 1;
            $parameter->year = $nextYear;
            $parameter->save();

            // Step 5: Update period table
            $this->updatePeriodTable($nextYear);
        });

        $param = Parameter::first();
        $this->info("  EOY complete. Parameter: Year={$param->year}, Period={$param->period}");
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

        $periodInt = (int) $period;
        $year = (int) $parameter->year;
        $this->updateSaldoFromTransactions($periodInt, $year, $period);
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

        $this->info("  RE account created: {$newAccCode}");
        return $newAccCode;
    }

    private function genNextYearSaldo(int $currentYear, int $nextYear, string $reAccCode): void
    {
        // Step 1: Insert template for next year (periods 0-13 for all accounts x divisions)
        $insertQuery = "
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
        ";
        DB::statement($insertQuery, ['nextYear' => $nextYear, 'nextYear2' => $nextYear]);

        // Step 2: Carry forward REAL accounts (groups 1,2,3) to period 0
        $updateRealQuery = "
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
        ";
        DB::statement($updateRealQuery, ['currentYear' => $currentYear, 'nextYear' => $nextYear]);

        // Step 3: Calculate net income from nominal accounts
        $netIncome = (float) DB::table('mst_saldo_news')
            ->join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->whereIn('mst_account.acc_group', [4, 5, 6, 7, 8])
            ->where('mst_saldo_news.tahun', $currentYear)
            ->selectRaw('COALESCE(SUM(mst_saldo_news.credit) - SUM(mst_saldo_news.debet), 0) as net')
            ->value('net');

        $this->info("  Net income: " . number_format($netIncome, 2));

        // Step 4: Post net income to RE account period 0
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

    // ─── YEAR 2 TEST ──────────────────────────────────────────────

    private function testNewYear(): void
    {
        $this->testYear = 2026;
        $param = Parameter::first();
        $this->assertValue('Parameter Year', 2026, (int) $param->year);
        $this->assertValue('Parameter Period', 1, (int) $param->period);

        // Create transactions in new year period 1
        $revAmount = 50000;
        $expAmount = 10000;
        $this->info("Creating Year 2 transactions (Rev:{$revAmount}, Exp:{$expAmount})...");

        $date = '2026-01-15';

        $this->createJournal($date, 'Y2 Revenue', [
            ['acc' => self::ACC_CASH,    'debet' => $revAmount, 'credit' => 0,          'desc' => 'Cash receipt'],
            ['acc' => self::ACC_REVENUE, 'debet' => 0,          'credit' => $revAmount, 'desc' => 'Service revenue'],
        ]);

        $this->createJournal($date, 'Y2 Rent', [
            ['acc' => self::ACC_RENT, 'debet' => $expAmount, 'credit' => 0,          'desc' => 'Rent expense'],
            ['acc' => self::ACC_CASH, 'debet' => 0,          'credit' => $expAmount, 'desc' => 'Cash payment'],
        ]);

        // Run EOM
        $this->runEOM();

        // Verify saldo
        $this->info('Verifying Year 2 balances...');

        // Cash P1 2026: D=50000 C=10000
        $cashSaldo = $this->getSaldo(self::ACC_CASH, 1, 2026);
        $this->assertValue('Cash P1 2026 D', 50000, $cashSaldo->debet);
        $this->assertValue('Cash P1 2026 C', 10000, $cashSaldo->credit);

        // Revenue P1 2026: D=0 C=50000 (fresh, no carryover from 2025)
        $revSaldo = $this->getSaldo(self::ACC_REVENUE, 1, 2026);
        $this->assertValue('Revenue P1 2026 D', 0, $revSaldo->debet);
        $this->assertValue('Revenue P1 2026 C', 50000, $revSaldo->credit);

        // BS verification: Cash balance = beginning + period movement
        // Beginning (P0): Net cash from 2025 = 225000 (carried forward as D=318000, C=93000)
        $cashP0 = $this->getSaldo(self::ACC_CASH, 0, 2026);
        $beginBalance = $cashP0->debet - $cashP0->credit; // Should be 225000

        // Cumulative (P0+P1): 225000 + (50000-10000) = 265000
        $cumulativeCash = $beginBalance + ($cashSaldo->debet - $cashSaldo->credit);
        $this->assertValue('BS Cash cumulative 2026 P1', 265000, $cumulativeCash);

        // IS verification: Revenue is fresh for new year
        $this->assertValue('IS Revenue P1 2026', 50000, $revSaldo->credit - $revSaldo->debet);
    }

    // ─── VERIFICATION METHODS ─────────────────────────────────────

    private function verifyPreEOMSaldo(int $period): void
    {
        // Before EOM, TransDetail::created events should have updated saldo
        $revenue = $this->getRevenueAmount($period);
        $rent = $this->getRentAmount();
        $salary = $this->getSalaryAmount($period);

        $cashSaldo = $this->getSaldo(self::ACC_CASH, $period, $this->testYear);
        // Cash debet should = revenue, credit should = rent + salary
        $this->assertValue("Pre-EOM Cash P{$period} D", $revenue, $cashSaldo->debet);
        $this->assertValue("Pre-EOM Cash P{$period} C", $rent + $salary, $cashSaldo->credit);
    }

    private function verifySaldoAfterEOM(int $period): void
    {
        $revenue = $this->getRevenueAmount($period);
        $rent = $this->getRentAmount();
        $salary = $this->getSalaryAmount($period);

        $this->info('Verifying mst_saldo_news...');

        // Cash
        $cashSaldo = $this->getSaldo(self::ACC_CASH, $period, $this->testYear);
        $this->assertValue("Cash P{$period} D", $revenue, $cashSaldo->debet);
        $this->assertValue("Cash P{$period} C", $rent + $salary, $cashSaldo->credit);

        // Revenue
        $revSaldo = $this->getSaldo(self::ACC_REVENUE, $period, $this->testYear);
        $this->assertValue("Revenue P{$period} D", 0, $revSaldo->debet);
        $this->assertValue("Revenue P{$period} C", $revenue, $revSaldo->credit);

        // Rent
        $rentSaldo = $this->getSaldo(self::ACC_RENT, $period, $this->testYear);
        $this->assertValue("Rent P{$period} D", $rent, $rentSaldo->debet);
        $this->assertValue("Rent P{$period} C", 0, $rentSaldo->credit);

        // Salary
        $salSaldo = $this->getSaldo(self::ACC_SALARY, $period, $this->testYear);
        $this->assertValue("Salary P{$period} D", $salary, $salSaldo->debet);
        $this->assertValue("Salary P{$period} C", 0, $salSaldo->credit);
    }

    private function verifyBSValues(int $period): void
    {
        $this->info('Verifying BS values...');

        // BS Cash = cumulative sum(D) - sum(C) from period 0 to current
        $cumCash = $this->getCumulativeBalance(self::ACC_CASH, $period, $this->testYear);
        $expectedCash = $this->expectedCumulativeCash($period);
        $this->assertValue("BS Cash P{$period}", $expectedCash, $cumCash);

        // BS Cash Last Month (period - 1)
        if ($period > 1) {
            $lastCash = $this->getCumulativeBalance(self::ACC_CASH, $period - 1, $this->testYear);
            $expectedLastCash = $this->expectedCumulativeCash($period - 1);
            $this->assertValue("BS Cash LastMonth P{$period}", $expectedLastCash, $lastCash);
        } else {
            // Period 1: last month = beginning = 0
            $this->assertValue("BS Cash LastMonth P1", 0, 0);
        }

        // BS Cash Beginning (period 0) = 0 for first year
        $begCash = $this->getCumulativeBalance(self::ACC_CASH, 0, $this->testYear);
        $this->assertValue("BS Cash Beginning", 0, $begCash);
    }

    private function verifyISValues(int $period): void
    {
        $this->info('Verifying IS values...');

        // IS Revenue This Month = single period credit - debet
        $revSaldo = $this->getSaldo(self::ACC_REVENUE, $period, $this->testYear);
        $revThisMonth = $revSaldo->credit - $revSaldo->debet;
        $expectedRev = $this->getRevenueAmount($period);
        $this->assertValue("IS Revenue P{$period}", $expectedRev, $revThisMonth);

        // IS Revenue YTD = sum of all periods 1 to current
        $revYTD = $this->getYTDBalance(self::ACC_REVENUE, $period, $this->testYear, 'C');
        $expectedYTD = 0;
        for ($p = 1; $p <= $period; $p++) {
            $expectedYTD += $this->getRevenueAmount($p);
        }
        $this->assertValue("IS Revenue YTD P{$period}", $expectedYTD, $revYTD);

        // IS Rent This Month
        $rentSaldo = $this->getSaldo(self::ACC_RENT, $period, $this->testYear);
        $rentThisMonth = $rentSaldo->debet - $rentSaldo->credit;
        $this->assertValue("IS Rent P{$period}", $this->getRentAmount(), $rentThisMonth);

        // IS Net Income YTD
        $totalRevYTD = $revYTD;
        $totalExpYTD = 0;
        for ($p = 1; $p <= $period; $p++) {
            $totalExpYTD += $this->getRentAmount() + $this->getSalaryAmount($p);
        }
        $expectedNetIncome = $totalRevYTD - $totalExpYTD;
        $actualNetIncome = $this->calculateNetIncome($period, $this->testYear);
        $this->assertValue("IS Net Income YTD P{$period}", $expectedNetIncome, $actualNetIncome);
    }

    private function verifyQuarterlyReports(): void
    {
        // For each quarter, verify cumulative BS at quarter-end months and IS totals
        $quarters = [
            ['q' => 1, 'months' => [1, 2, 3]],
            ['q' => 2, 'months' => [4, 5, 6]],
            ['q' => 3, 'months' => [7, 8, 9]],
            ['q' => 4, 'months' => [10, 11, 12]],
        ];

        foreach ($quarters as $qData) {
            $q = $qData['q'];
            $months = $qData['months'];
            $qEnd = end($months);

            $this->info("Q{$q} (periods {$months[0]}-{$qEnd}):");

            // BS: Cash at each month within quarter
            foreach ($months as $m) {
                $cumCash = $this->getCumulativeBalance(self::ACC_CASH, $m, $this->testYear);
                $expectedCash = $this->expectedCumulativeCash($m);
                $this->assertValue("  BS Cash P{$m}", $expectedCash, $cumCash);
            }

            // IS: Revenue per individual month within quarter
            $qRevTotal = 0;
            foreach ($months as $m) {
                $revSaldo = $this->getSaldo(self::ACC_REVENUE, $m, $this->testYear);
                $rev = $revSaldo->credit - $revSaldo->debet;
                $expectedRev = $this->getRevenueAmount($m);
                $this->assertValue("  IS Revenue P{$m}", $expectedRev, $rev);
                $qRevTotal += $rev;
            }

            // IS: Revenue quarter total
            $expectedQRev = 0;
            foreach ($months as $m) {
                $expectedQRev += $this->getRevenueAmount($m);
            }
            $this->assertValue("  IS Revenue Q{$q} total", $expectedQRev, $qRevTotal);

            // IS: Revenue YTD at quarter end
            $revYTD = $this->getYTDBalance(self::ACC_REVENUE, $qEnd, $this->testYear, 'C');
            $expectedYTD = 0;
            for ($p = 1; $p <= $qEnd; $p++) {
                $expectedYTD += $this->getRevenueAmount($p);
            }
            $this->assertValue("  IS Revenue YTD at P{$qEnd}", $expectedYTD, $revYTD);
        }
    }

    private function verifyEOY(): void
    {
        $this->info('Verifying EOY results...');

        $currentYear = 2025;
        $nextYear = 2026;

        // 1. RE account exists
        $reAcc = ChartOfAccount::where('acc_code', 'like', '38888888%')
            ->where('acc_code', '!=', '388888880000000')
            ->first();
        $this->assertValue('RE account exists', true, $reAcc !== null);

        if (!$reAcc) {
            $this->error('  Cannot continue EOY verification without RE account.');
            return;
        }

        // 2. Net Income = 225,000
        // Revenue total: sum(10000 + (n-1)*3000) for n=1..12 = 318,000
        // Rent total: 2000 * 12 = 24,000
        // Salary total: sum(3000 + (n-1)*500) for n=1..12 = 69,000
        // Net Income = 318000 - 24000 - 69000 = 225,000
        $expectedNetIncome = 225000;

        // 3. Cash P0 next year: carry forward of all periods 0-12 from 2025
        $cashP0 = $this->getSaldo(self::ACC_CASH, 0, $nextYear);
        // Total cash debet across all 2025 periods = total revenue = 318000
        // Total cash credit across all 2025 periods = total rent + salary = 93000
        $this->assertValue('Cash P0 2026 D', 318000, $cashP0->debet);
        $this->assertValue('Cash P0 2026 C', 93000, $cashP0->credit);
        $cashNet = $cashP0->debet - $cashP0->credit;
        $this->assertValue('Cash P0 2026 net', $expectedNetIncome, $cashNet);

        // 4. Revenue P0 next year: should be 0 (nominal accounts reset)
        $revP0 = $this->getSaldo(self::ACC_REVENUE, 0, $nextYear);
        $this->assertValue('Revenue P0 2026 D', 0, $revP0->debet);
        $this->assertValue('Revenue P0 2026 C', 0, $revP0->credit);

        // 5. Rent P0 next year: should be 0
        $rentP0 = $this->getSaldo(self::ACC_RENT, 0, $nextYear);
        $this->assertValue('Rent P0 2026 D', 0, $rentP0->debet);
        $this->assertValue('Rent P0 2026 C', 0, $rentP0->credit);

        // 6. RE P0 next year: credit = net income
        $reP0 = $this->getSaldo($reAcc->acc_code, 0, $nextYear);
        $this->assertValue('RE P0 2026 C', $expectedNetIncome, $reP0->credit);
        $this->assertValue('RE P0 2026 D', 0, $reP0->debet);

        // 7. Accounting equation: Assets = Liabilities + Equity at P0
        $totalAssets = $this->getP0NetBalance([1], $nextYear, 'D');
        $totalLiabilities = $this->getP0NetBalance([2], $nextYear, 'C');
        $totalEquity = $this->getP0NetBalance([3], $nextYear, 'C');
        $this->assertValue('Acct Equation: A=L+E',
            round($totalAssets, 2),
            round($totalLiabilities + $totalEquity, 2)
        );
    }

    // ─── BALANCE QUERY HELPERS ────────────────────────────────────

    private function getSaldo(string $accCode, int $period, int $year): object
    {
        $saldo = MstSaldoNew::where('acc_code', $accCode)
            ->where('div_code', self::DIV_CODE)
            ->where('periode', $period)
            ->where('tahun', $year)
            ->first();

        return $saldo ?? (object) ['debet' => 0, 'credit' => 0];
    }

    /**
     * Cumulative balance for BS accounts: sum(D) - sum(C) from period 0 to $period.
     */
    private function getCumulativeBalance(string $accCode, int $period, int $year): float
    {
        $result = MstSaldoNew::where('acc_code', $accCode)
            ->where('div_code', self::DIV_CODE)
            ->where('tahun', $year)
            ->whereBetween('periode', [0, $period])
            ->selectRaw('SUM(debet) as total_d, SUM(credit) as total_c')
            ->first();

        return ($result->total_d ?? 0) - ($result->total_c ?? 0);
    }

    /**
     * YTD balance for IS accounts: sum over periods 1 to $period.
     */
    private function getYTDBalance(string $accCode, int $period, int $year, string $normalBalance): float
    {
        $result = MstSaldoNew::where('acc_code', $accCode)
            ->where('div_code', self::DIV_CODE)
            ->where('tahun', $year)
            ->whereBetween('periode', [1, $period])
            ->selectRaw('SUM(debet) as total_d, SUM(credit) as total_c')
            ->first();

        $d = $result->total_d ?? 0;
        $c = $result->total_c ?? 0;

        return $normalBalance === 'C' ? ($c - $d) : ($d - $c);
    }

    /**
     * Net income = sum(credit - debet) for nominal accounts (groups 4-8) up to period.
     */
    private function calculateNetIncome(int $period, int $year): float
    {
        return (float) MstSaldoNew::join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->whereIn('mst_account.acc_group', [4, 5, 6, 7, 8])
            ->where('mst_saldo_news.tahun', $year)
            ->where('mst_saldo_news.div_code', self::DIV_CODE)
            ->whereBetween('mst_saldo_news.periode', [1, $period])
            ->selectRaw('COALESCE(SUM(mst_saldo_news.credit) - SUM(mst_saldo_news.debet), 0) as net')
            ->value('net');
    }

    /**
     * Get net balance for period 0 by account groups.
     */
    private function getP0NetBalance(array $groups, int $year, string $normalBalance): float
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

    // ─── EXPECTED VALUE CALCULATORS ───────────────────────────────

    /**
     * Expected cumulative cash balance at end of period n (first year, beginning=0).
     */
    private function expectedCumulativeCash(int $period): float
    {
        $total = 0;
        for ($p = 1; $p <= $period; $p++) {
            $total += $this->getRevenueAmount($p) - $this->getRentAmount() - $this->getSalaryAmount($p);
        }
        return $total;
    }

    // ─── ASSERTION & OUTPUT HELPERS ───────────────────────────────

    private function assertValue(string $label, mixed $expected, mixed $actual): void
    {
        // Handle float comparison with tolerance
        if (is_float($expected) || is_float($actual)) {
            $match = abs((float) $expected - (float) $actual) < 0.01;
        } else {
            $match = $expected === $actual;
        }

        if ($match) {
            $this->passed++;
            $displayVal = is_bool($expected) ? ($expected ? 'true' : 'false') : number_format((float) $expected, 2, ',', '.');
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
            $this->info('<fg=green;options=bold>ALL CHECKS PASSED!</>');
        } else {
            $this->newLine();
            $this->error("FAILED: {$this->failed} check(s) did not pass.");
        }
    }
}
