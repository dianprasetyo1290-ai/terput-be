<?php

namespace App\Console\Commands;

use Illuminate\Console\Command;
use Illuminate\Support\Facades\DB;

class ImportLegacyOpeningBalances extends Command
{
    protected $signature = 'legacy:import-opening-balances
        {file : CSV export of legacy IAS Mst_Saldo}
        {--year=2025 : Target year for period 0 balances}
        {--batch=1000 : Upsert batch size}';

    protected $description = 'Import legacy Mst_Saldo CurD/CurK as mst_saldo_news period 0 opening balances.';

    public function handle(): int
    {
        $file = (string) $this->argument('file');
        $year = (int) $this->option('year');
        $batchSize = max(1, (int) $this->option('batch'));

        if (!is_readable($file)) {
            $this->error("File is not readable: {$file}");
            return self::FAILURE;
        }

        $handle = fopen($file, 'r');
        if ($handle === false) {
            $this->error("Unable to open file: {$file}");
            return self::FAILURE;
        }

        $headers = fgetcsv($handle);
        if ($headers === false) {
            fclose($handle);
            $this->error('CSV header row is missing.');
            return self::FAILURE;
        }

        $headerMap = array_flip($headers);
        foreach (['Acc_Code', 'Div_Code', 'CurD', 'CurK'] as $requiredColumn) {
            if (!array_key_exists($requiredColumn, $headerMap)) {
                fclose($handle);
                $this->error("Required column is missing: {$requiredColumn}");
                return self::FAILURE;
            }
        }

        $now = now();
        $rows = [];
        $count = 0;

        while (($legacyRow = fgetcsv($handle)) !== false) {
            $accCode = trim((string) ($legacyRow[$headerMap['Acc_Code']] ?? ''));
            $divCode = trim((string) ($legacyRow[$headerMap['Div_Code']] ?? ''));
            if ($accCode === '' || $divCode === '') {
                continue;
            }

            $rows[] = [
                'acc_code' => $accCode,
                'div_code' => $divCode,
                'periode' => 0,
                'tahun' => $year,
                'debet' => (float) ($legacyRow[$headerMap['CurD']] ?? 0),
                'credit' => (float) ($legacyRow[$headerMap['CurK']] ?? 0),
                'created_at' => $now,
                'updated_at' => $now,
            ];

            if (count($rows) >= $batchSize) {
                $count += $this->upsertRows($rows);
                $rows = [];
            }
        }

        fclose($handle);

        if ($rows !== []) {
            $count += $this->upsertRows($rows);
        }

        $this->info("Imported {$count} opening balance rows for {$year}.");

        return self::SUCCESS;
    }

    private function upsertRows(array $rows): int
    {
        DB::table('mst_saldo_news')->upsert(
            $rows,
            ['acc_code', 'div_code', 'periode', 'tahun'],
            ['debet', 'credit', 'updated_at']
        );

        return count($rows);
    }
}
