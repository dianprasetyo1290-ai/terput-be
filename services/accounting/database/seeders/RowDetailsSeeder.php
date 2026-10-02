<?php

namespace Database\Seeders;

use App\Models\ReportFinal;
use App\Models\ReportFinalData;
use App\Models\ReportRowDetail;
use App\Models\ReportRowHead;
use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;
use Throwable;

class RowDetailsSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        DB::table('report_final_data')->truncate();
        DB::table('report_finals')->truncate();
        DB::table('report_row_details')->truncate();
        DB::table('report_row_heads')->truncate();

        $finRptFile = env('LEGACY_FINRPT_FILE', base_path('storage/exco_mdb_special/EXCO Database/FINRPT.2025'));
        if (is_readable($finRptFile)) {
            $this->runFromMdbFile($finRptFile);
            return;
        }

        // Report layouts are per-school, and the legacy EXCO report database
        // (dblama_rpt, a host that is dead) is the only source here. Without
        // this guard provisioning a brand-new school aborts on the very last
        // seeder with a connection error. Skipping leaves the school with an
        // empty Report Generator, which it fills itself -- importing another
        // company's report layout would be wrong anyway. Set
        // LEGACY_FINRPT_FILE to import from a FINRPT file instead.
        try {
            DB::connection('dblama_rpt')->getPdo();
        } catch (Throwable $e) {
            $this->command?->warn('RowDetailsSeeder dilewati: sumber laporan lama (dblama_rpt) tidak terjangkau.');

            return;
        }

        $oldData = DB::connection('dblama_rpt')->table('row_master')
            ->leftJoin('row_title', 'row_title.row_master', '=', 'row_master.row_master')
            ->select('row_master.*', 'row_title.row_title')
            ->get();

        $count = 0; // Inisialisasi counter
        $batchSize = 1000; // Jumlah data per batch
        $sortCounters = [];

        // Masukkan data ke database baru
        foreach ($oldData as $data) {
            $data_report_row_heads = ReportRowHead::firstOrCreate(
                ['name' => $data->row_title]
            );

            $rowMasterKey = (string) $data->row_master;
            $sortCounters[$rowMasterKey] = ($sortCounters[$rowMasterKey] ?? 0) + 1;

            $dataArray = [
                'report_row_head_id' => $data_report_row_heads->id,
                'sort_order' => $sortCounters[$rowMasterKey],
                'name' => $data->description,
                'group' => strtolower($data->group_code == 'G' ? 'gt' : $data->group_code),
                'formula' => $data->formula ?? null,
                'identifier' => $data->row_code,
                'level' => $data->level ?? 0,
                'normal_balance' => $data->normal_bal,
                'operator' => $this->cekOperator($data->operator),
                'kode' => str_replace("*", "%", $data->account),
            ];

            // Insert data ke database baru
            ReportRowDetail::insert($dataArray);

            // Update counter
            $count++;

            // Log setiap 1000 baris
            if ($count % $batchSize === 0) {
                echo "Telah memasukkan $count baris data.\n";
            }
        }

        // Log sisa data jika kurang dari 1000
        if ($count % $batchSize !== 0) {
            echo "Telah memasukkan $count baris data.\n";
        }

        // Keep final report mapping inside this import session so same title in this upload stays grouped,
        // but repeated imports can still create new workspaces for the same title.
        $finalByTitleMap = [];

        $results = DB::connection('dblama_rpt')->table('final')->leftJoin('row_title', 'row_title.row_master', '=', 'final.row_master')
            ->select('final.*', 'row_title.row_title') // Select columns as needed
            ->get();

        // Masukkan data ke database baru
        $finalSortCounters = [];
        foreach ($results as $data) {
            $data_report_row_heads = ReportRowHead::firstOrCreate(
                ['name' => $data->row_title]
            );

            $finalTitleKey = trim(($data->report_title ?? '')) . '|' . trim(($data->print_title ?? '')) . '|' . trim(($data->sub_title ?? ''));
            if (!array_key_exists($finalTitleKey, $finalByTitleMap)) {
                $finalByTitleMap[$finalTitleKey] = ReportFinal::create([
                    'name' => $data->report_title,
                    'print_title' => $data->print_title,
                    'sub_title' => $data->sub_title,
                ]);
            }

            $data_report_final = $finalByTitleMap[$finalTitleKey];

            $alreadyExists = ReportFinalData::where([
                'report_final_id' => $data_report_final->id,
                'report_row_head_id' => $data_report_row_heads->id,
            ])->exists();

            if ($alreadyExists) {
                continue;
            }

            $dataArray = [
                'report_row_head_id' => $data_report_row_heads->id,
                'report_final_id' => $data_report_final->id,
                'sort_order' => ($finalSortCounters[$data_report_final->id] ?? 0) + 1,
            ];
            $finalSortCounters[$data_report_final->id] = $dataArray['sort_order'];

            // Insert data ke database baru
            ReportFinalData::insert($dataArray);

            // Update counter
            $count++;

            // Log setiap 1000 baris
            if ($count % $batchSize === 0) {
                echo "Telah memasukkan $count baris data.\n";
            }
        }
    }

    public function cekOperator($op)
    {
        if ($op == '+') {
            return 'i';
        } else {
            return 'd';
        }
    }

    private function runFromMdbFile(string $file): void
    {
        $titles = [];
        foreach ($this->readMdbRows($file, 'Row_Title') as $row) {
            $titles[trim((string) ($row['Row_Master'] ?? ''))] = trim((string) ($row['Row_Title'] ?? ''));
        }

        $rowHeadByMaster = [];
        $sortCounters = [];
        $count = 0;

        foreach ($this->readMdbRows($file, 'Row_Master') as $data) {
            $rowMaster = trim((string) ($data['Row_Master'] ?? ''));
            $rowTitle = $titles[$rowMaster] ?? $rowMaster;

            if (!isset($rowHeadByMaster[$rowMaster])) {
                $rowHeadByMaster[$rowMaster] = ReportRowHead::firstOrCreate(['name' => $rowTitle]);
            }

            $sortCounters[$rowMaster] = ($sortCounters[$rowMaster] ?? 0) + 1;

            ReportRowDetail::insert([
                'report_row_head_id' => $rowHeadByMaster[$rowMaster]->id,
                'sort_order' => $sortCounters[$rowMaster],
                'name' => trim((string) ($data['Description'] ?? '')),
                'group' => strtolower(($data['Group_Code'] ?? '') === 'G' ? 'gt' : (string) ($data['Group_Code'] ?? '')),
                'formula' => $data['Formula'] ?: null,
                'identifier' => trim((string) ($data['Row_Code'] ?? '')),
                'level' => (int) ($data['Level'] ?: 0),
                'normal_balance' => $data['Normal_Bal'] ?: null,
                'operator' => $this->cekOperator($data['Operator'] ?? null),
                'kode' => str_replace('*', '%', (string) ($data['Account'] ?? '')) ?: null,
            ]);

            $count++;
        }

        echo "Telah memasukkan {$count} baris data dari {$file}.\n";

        $finalByTitleMap = [];
        $finalSortCounters = [];
        foreach ($this->readMdbRows($file, 'Final') as $data) {
            $rowMaster = trim((string) ($data['Row_Master'] ?? ''));
            $rowTitle = $titles[$rowMaster] ?? $rowMaster;
            $rowHead = $rowHeadByMaster[$rowMaster] ?? ReportRowHead::firstOrCreate(['name' => $rowTitle]);

            $finalTitleKey = trim((string) ($data['Report_Title'] ?? '')) . '|' . trim((string) ($data['Print_Title'] ?? '')) . '|' . trim((string) ($data['Sub_Title'] ?? ''));
            if (!array_key_exists($finalTitleKey, $finalByTitleMap)) {
                $finalByTitleMap[$finalTitleKey] = ReportFinal::create([
                    'name' => trim((string) ($data['Report_Title'] ?? '')),
                    'print_title' => trim((string) ($data['Print_Title'] ?? '')),
                    'sub_title' => trim((string) ($data['Sub_Title'] ?? '')),
                ]);
            }

            $reportFinal = $finalByTitleMap[$finalTitleKey];
            $sortOrder = ($finalSortCounters[$reportFinal->id] ?? 0) + 1;
            $finalSortCounters[$reportFinal->id] = $sortOrder;

            ReportFinalData::firstOrCreate([
                'report_final_id' => $reportFinal->id,
                'report_row_head_id' => $rowHead->id,
            ], [
                'sort_order' => $sortOrder,
            ]);
        }
    }

    private function readMdbRows(string $file, string $table): \Generator
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
}
