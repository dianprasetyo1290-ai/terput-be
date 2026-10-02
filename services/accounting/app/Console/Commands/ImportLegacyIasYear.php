<?php

namespace App\Console\Commands;

use Carbon\Carbon;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Hash;

class ImportLegacyIasYear extends Command
{
    protected $signature = 'legacy:import-ias-year
        {file : Legacy IAS.<year> Access/MDB file}
        {--year= : Target year. Defaults to suffix from file name}
        {--batch=1000 : Insert batch size}
        {--skip-budget : Do not import Mst_Budget from this legacy year}
        {--fresh-year : Delete local data for this year before importing}';

    protected $description = 'Import one legacy IAS year into local Laravel tables, including masters, transactions, budgets, and mst_saldo_news.';

    public function handle(): int
    {
        $file = (string) $this->argument('file');
        if (!is_readable($file)) {
            $this->error("File is not readable: {$file}");
            return self::FAILURE;
        }

        $year = (int) ($this->option('year') ?: $this->guessYear($file));
        if ($year <= 0) {
            $this->error('Unable to determine target year. Pass --year=YYYY.');
            return self::FAILURE;
        }

        $batchSize = max(1, (int) $this->option('batch'));

        DB::transaction(function () use ($file, $year, $batchSize) {
            if ($this->option('fresh-year')) {
                $this->deleteYearData($year);
            }

            $this->importParameter($file, $year);
            $this->importPeriods($file, $batchSize);
            $this->importCurrencies($file, $batchSize);
            $this->importDivisions($file, $batchSize);
            $this->importAccounts($file, $batchSize);
            $this->importTransactions($file, $batchSize);
            if (!$this->option('skip-budget')) {
                $this->importBudgets($file, $batchSize);
            }
            $this->importSaldo($file, $year, $batchSize);
        });

        $this->info("Imported legacy IAS data for {$year}.");

        return self::SUCCESS;
    }

    private function deleteYearData(int $year): void
    {
        DB::table('trans_detail')->whereYear('trans_date', $year)->delete();
        DB::table('trans_head')->whereYear('trans_date', $year)->delete();
        DB::table('this_detail')->whereYear('trans_date', $year)->delete();
        DB::table('this_head')->whereYear('trans_date', $year)->delete();
        DB::table('mst_saldo_news')->where('tahun', $year)->delete();
    }

    private function importParameter(string $file, int $year): void
    {
        $row = null;
        foreach ($this->readRows($file, 'Parameter') as $legacyRow) {
            $row = $legacyRow;
            break;
        }

        if (!$row) {
            return;
        }

        DB::table('parameter')->truncate();
        DB::table('parameter')->insert([
            'company' => $this->string($row, 'Company') ?: 'SEKOCI',
            'serial_no' => $this->string($row, 'Serial No') ?: 'LOCAL001',
            'address' => $this->string($row, 'Address'),
            'contact' => $this->string($row, 'Contact'),
            'tax_number' => $this->string($row, 'Tax Number'),
            'period' => max(1, (int) preg_replace('/\D/', '', $this->string($row, 'Period') ?: '13')),
            'year' => $year,
            'mask_account' => $this->string($row, 'Mask Account') ?: 'AAAAAAAAAAAAAAA',
            'base_currency' => $this->string($row, 'Base Currency') ?: 'Rp',
            'control_date' => $this->bool($this->value($row, 'Control Date')),
            'created_at' => now(),
            'updated_at' => now(),
        ]);
    }

    private function importPeriods(string $file, int $batchSize): void
    {
        DB::table('mst_period')->truncate();

        $rows = [];
        foreach ($this->readRows($file, 'Mst_Period') as $row) {
            $periodCode = $this->string($row, 'Period_Code');
            if ($periodCode === '') {
                continue;
            }

            $payload = [
                'period_code' => $periodCode,
                'period_name' => $this->string($row, 'Period_Name'),
                'created_at' => now(),
                'updated_at' => now(),
            ];

            for ($period = 1; $period <= 13; $period++) {
                $suffix = sprintf('%02d', $period);
                $payload['p' . $suffix] = $this->string($row, 'P' . $suffix);
                $payload['p' . $suffix . '_from'] = $this->date($row, 'P' . $suffix . '_From');
                $payload['p' . $suffix . '_to'] = $this->date($row, 'P' . $suffix . '_To');
            }

            $rows[] = $payload;
            if (count($rows) >= $batchSize) {
                DB::table('mst_period')->insert($rows);
                $rows = [];
            }
        }

        if ($rows !== []) {
            DB::table('mst_period')->insert($rows);
        }
    }

    private function importCurrencies(string $file, int $batchSize): void
    {
        $rows = [];
        foreach ($this->readRows($file, 'Mst_Currency') as $row) {
            $code = $this->string($row, 'Curr_Code');
            if ($code === '') {
                continue;
            }

            $payload = [
                'curr_code' => $code,
                'curr_name' => $this->string($row, 'Curr_Name'),
                'rate_system' => $this->string($row, 'Rate_System'),
                'created_at' => now(),
                'updated_at' => now(),
            ];

            foreach (['act', 'lst'] as $prefix) {
                for ($period = 1; $period <= 13; $period++) {
                    $column = sprintf('%s_%02d', $prefix, $period);
                    $payload[$column] = (string) $this->number($row, ucfirst($prefix) . '_' . sprintf('%02d', $period));
                }
            }

            $rows[] = $payload;
            if (count($rows) >= $batchSize) {
                $this->upsert('mst_currency', $rows, ['curr_code']);
                $rows = [];
            }
        }

        if ($rows !== []) {
            $this->upsert('mst_currency', $rows, ['curr_code']);
        }
    }

    private function importDivisions(string $file, int $batchSize): void
    {
        $rows = [];
        foreach ($this->readRows($file, 'Mst_Division') as $row) {
            $code = $this->string($row, 'Div_Code');
            if ($code === '') {
                continue;
            }

            $rows[] = [
                'div_code' => $code,
                'div_name' => $this->string($row, 'Div_Name'),
                'created_at' => now(),
                'updated_at' => now(),
            ];

            if (count($rows) >= $batchSize) {
                $this->upsert('mst_division', $rows, ['div_code']);
                $rows = [];
            }
        }

        if ($rows !== []) {
            $this->upsert('mst_division', $rows, ['div_code']);
        }
    }

    private function importAccounts(string $file, int $batchSize): void
    {
        $rows = [];
        foreach ($this->readRows($file, 'Mst_Account') as $row) {
            $code = $this->string($row, 'Acc_Code');
            if ($code === '') {
                continue;
            }

            $rows[] = [
                'acc_code' => $code,
                'acc_name' => $this->string($row, 'Acc_Name'),
                'acc_group' => (int) $this->number($row, 'Acc_Group'),
                'sub_acc_group' => null,
                'acc_currency' => $this->string($row, 'Acc_Currency') ?: null,
                'created_at' => now(),
                'updated_at' => now(),
            ];

            if (count($rows) >= $batchSize) {
                $this->upsert('mst_account', $rows, ['acc_code']);
                $rows = [];
            }
        }

        if ($rows !== []) {
            $this->upsert('mst_account', $rows, ['acc_code']);
        }
    }

    private function importTransactions(string $file, int $batchSize): void
    {
        $this->importTransactionTypes($file, $batchSize);
        $this->importHeads($file, 'Trans_Head', 'trans_head', false, $batchSize);
        $this->importHeads($file, 'This_Head', 'this_head', true, $batchSize);
        $this->importDetails($file, 'Trans_Detail', 'trans_detail', false, $batchSize);
        $this->importDetails($file, 'This_Detail', 'this_detail', true, $batchSize);
    }

    private function importTransactionTypes(string $file, int $batchSize): void
    {
        $rows = [];
        foreach ($this->readRows($file, 'Mst_Transaction') as $row) {
            $code = $this->string($row, 'Trans_Code');
            if ($code === '') {
                continue;
            }

            $rows[] = [
                'trans_code' => $code,
                'trans_name' => $this->string($row, 'Trans_Name'),
                'trans_status' => $this->string($row, 'Trans_Status'),
                'trans_account' => $this->string($row, 'Trans_Account') ?: null,
                'created_at' => now(),
                'updated_at' => now(),
            ];

            if (count($rows) >= $batchSize) {
                $this->upsert('mst_transaction', $rows, ['trans_code']);
                $rows = [];
            }
        }

        if ($rows !== []) {
            $this->upsert('mst_transaction', $rows, ['trans_code']);
        }
    }

    private function importHeads(string $file, string $legacyTable, string $targetTable, bool $withPeriod, int $batchSize): void
    {
        $rows = [];
        foreach ($this->readRows($file, $legacyTable) as $row) {
            $transNo = $this->string($row, 'Trans_No');
            if ($transNo === '') {
                continue;
            }

            $userId = $this->string($row, 'Trans_User') ?: '00001';
            $this->ensureUser($userId);

            $payload = [
                'trans_code' => $this->string($row, 'Trans_Code') ?: null,
                'trans_no' => $transNo,
                'trans_date' => $this->date($row, 'Trans_Date'),
                'trans_update' => $this->date($row, 'Trans_Update'),
                'trans_user' => $userId,
                'debet' => $this->number($row, 'Debet'),
                'credit' => $this->number($row, 'Credit'),
                'created_at' => now(),
                'updated_at' => now(),
            ];

            if ($withPeriod) {
                $payload = ['period' => $this->string($row, 'Period')] + $payload;
            }

            $rows[] = $payload;
            if (count($rows) >= $batchSize) {
                DB::table($targetTable)->insertOrIgnore($rows);
                $rows = [];
            }
        }

        if ($rows !== []) {
            DB::table($targetTable)->insertOrIgnore($rows);
        }
    }

    private function importDetails(string $file, string $legacyTable, string $targetTable, bool $withPeriod, int $batchSize): void
    {
        $rows = [];
        foreach ($this->readRows($file, $legacyTable) as $row) {
            $transNo = $this->string($row, 'Trans_No');
            if ($transNo === '') {
                continue;
            }

            $payload = [
                'trans_code' => $this->string($row, 'Trans_Code') ?: null,
                'trans_no' => $transNo,
                'trans_count' => (int) $this->number($row, 'Trans_Count'),
                'acc_code' => $this->string($row, 'Acc_Code') ?: null,
                'div_code' => $this->string($row, 'Div_Code') ?: null,
                'description' => $this->string($row, 'Description'),
                'debet' => $this->number($row, 'Debet'),
                'credit' => $this->number($row, 'Credit'),
                'curr_code' => $this->string($row, 'Curr_Code') ?: null,
                'curr_rate' => $this->number($row, 'Curr_Rate'),
                'curr_debet' => $this->number($row, 'Curr_Debet'),
                'curr_credit' => $this->number($row, 'Curr_Credit'),
                'trans_date' => $this->dateTime($row, 'Trans_Date'),
                'curr_system' => $this->string($row, 'Curr_System') ?: null,
                'curr_yes' => $this->bool($this->value($row, 'Curr_Yes')),
                'created_at' => now(),
                'updated_at' => now(),
            ];

            if ($withPeriod) {
                $payload = ['period' => $this->string($row, 'Period')] + $payload;
            } else {
                $payload['trans_system'] = $this->string($row, 'Trans_System') ?: null;
                $payload['trans_yes'] = $this->bool($this->value($row, 'Trans_Yes'));
            }

            $rows[] = $payload;
            if (count($rows) >= $batchSize) {
                DB::table($targetTable)->insertOrIgnore($rows);
                $rows = [];
            }
        }

        if ($rows !== []) {
            DB::table($targetTable)->insertOrIgnore($rows);
        }
    }

    private function importBudgets(string $file, int $batchSize): void
    {
        $rows = [];
        foreach ($this->readRows($file, 'Mst_Budget') as $row) {
            $accCode = $this->string($row, 'Acc_Code');
            $divCode = $this->string($row, 'Div_Code');
            if ($accCode === '' || $divCode === '') {
                continue;
            }

            $payload = [
                'acc_code' => $accCode,
                'div_code' => $divCode,
                'created_at' => now(),
                'updated_at' => now(),
            ];

            foreach (['Act', 'Lst', 'Fut'] as $prefix) {
                for ($period = 1; $period <= 13; $period++) {
                    $payload[strtolower($prefix) . '_' . sprintf('%02d', $period)] = $this->number($row, $prefix . '_' . sprintf('%02d', $period));
                }
            }

            $rows[] = $payload;
            if (count($rows) >= $batchSize) {
                DB::table('mst_budget')->insert($rows);
                $rows = [];
            }
        }

        if ($rows !== []) {
            DB::table('mst_budget')->insert($rows);
        }
    }

    private function importSaldo(string $file, int $year, int $batchSize): void
    {
        $rows = [];
        foreach ($this->readRows($file, 'Mst_Saldo') as $row) {
            $accCode = $this->string($row, 'Acc_Code');
            $divCode = $this->string($row, 'Div_Code');
            if ($accCode === '' || $divCode === '') {
                continue;
            }

            $rows[] = $this->saldoRow($accCode, $divCode, 0, $year, $this->number($row, 'CurD'), $this->number($row, 'CurK'));

            for ($period = 1; $period <= 13; $period++) {
                $suffix = sprintf('%02d', $period);
                $rows[] = $this->saldoRow(
                    $accCode,
                    $divCode,
                    $period,
                    $year,
                    $this->number($row, 'Act_' . $suffix),
                    $this->number($row, 'Actk_' . $suffix)
                );
            }

            if (count($rows) >= $batchSize) {
                $this->upsert('mst_saldo_news', $rows, ['acc_code', 'div_code', 'periode', 'tahun']);
                $rows = [];
            }
        }

        if ($rows !== []) {
            $this->upsert('mst_saldo_news', $rows, ['acc_code', 'div_code', 'periode', 'tahun']);
        }
    }

    private function saldoRow(string $accCode, string $divCode, int $period, int $year, float $debet, float $credit): array
    {
        return [
            'acc_code' => $accCode,
            'div_code' => $divCode,
            'periode' => $period,
            'tahun' => $year,
            'debet' => $debet,
            'credit' => $credit,
            'created_at' => now(),
            'updated_at' => now(),
        ];
    }

    private function ensureUser(string $userId): void
    {
        if (DB::table('mst_user')->where('user_id', $userId)->exists()) {
            return;
        }

        DB::table('mst_user')->insert([
            'user_id' => $userId,
            'user_name' => "Legacy {$userId}",
            'user_access' => null,
            'email' => null,
            'password' => Hash::make('password'),
            'actived' => true,
            'created_at' => now(),
            'updated_at' => now(),
        ]);
    }

    private function upsert(string $table, array $rows, array $uniqueBy): void
    {
        if ($rows === []) {
            return;
        }

        $columns = array_keys($rows[0]);
        $updates = array_values(array_diff($columns, $uniqueBy, ['created_at']));
        DB::table($table)->upsert($rows, $uniqueBy, $updates);
    }

    private function readRows(string $file, string $table): \Generator
    {
        $command = 'mdb-export ' . escapeshellarg($file) . ' ' . escapeshellarg($table);
        $handle = popen($command, 'r');
        if ($handle === false) {
            throw new \RuntimeException("Unable to run mdb-export for {$table}");
        }

        $headers = fgetcsv($handle);
        if ($headers === false) {
            pclose($handle);
            return;
        }

        while (($values = fgetcsv($handle)) !== false) {
            if (count($values) < count($headers)) {
                $values = array_pad($values, count($headers), null);
            }

            yield array_combine($headers, array_slice($values, 0, count($headers)));
        }

        pclose($handle);
    }

    private function guessYear(string $file): int
    {
        preg_match('/(?:IAS|Ias)\.(\d{4})$/', $file, $matches);
        return (int) ($matches[1] ?? 0);
    }

    private function value(array $row, string $key): mixed
    {
        return $row[$key] ?? null;
    }

    private function string(array $row, string $key): string
    {
        return trim((string) ($this->value($row, $key) ?? ''));
    }

    private function number(array $row, string $key): float
    {
        $value = $this->value($row, $key);
        if ($value === null || $value === '') {
            return 0.0;
        }

        return (float) str_replace(',', '', (string) $value);
    }

    private function bool(mixed $value): bool
    {
        return in_array((string) $value, ['1', 'true', 'TRUE', 't', 'T', 'yes', 'YES'], true);
    }

    private function date(array $row, string $key): ?string
    {
        $date = $this->dateTime($row, $key);
        return $date ? substr($date, 0, 10) : null;
    }

    private function dateTime(array $row, string $key): ?string
    {
        $value = $this->string($row, $key);
        if ($value === '') {
            return null;
        }

        foreach (['m/d/y H:i:s', 'm/d/Y H:i:s', 'Y-m-d H:i:s'] as $format) {
            try {
                return Carbon::createFromFormat($format, $value)->format('Y-m-d H:i:s');
            } catch (\Throwable) {
                // Try next format.
            }
        }

        return Carbon::parse($value)->format('Y-m-d H:i:s');
    }
}
