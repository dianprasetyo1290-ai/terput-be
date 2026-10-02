<?php

namespace App\Console\Commands;

use App\Livewire\Report\ReportPrint;
use App\Livewire\Report\ReportRowPrint;
use App\Models\ReportFinal;
use Illuminate\Console\Command;
use ReflectionClass;

class SmokeTestLegacyReports extends Command
{
    protected $signature = 'legacy:smoke-reports {year} {--period=12}';

    protected $description = 'Process all financial reports for a year/period and fail on rendering calculation errors or blank outputs.';

    public function handle(): int
    {
        $year = (int) $this->argument('year');
        $period = (int) $this->option('period');

        $reports = ReportFinal::with(['reportData.reportRowHead.reportRowDetail', 'reportColHead.reportColDetails'])
            ->orderBy('id')
            ->get();

        if ($reports->isEmpty()) {
            $this->error('No report finals found.');
            return self::FAILURE;
        }

        foreach ($reports as $report) {
            $print = new ReportPrint();
            $print->report = $report;
            $print->periode = $period;
            $print->tahun = $year;

            $isBalanceSheet = $this->callPrivate($print, 'detectBalanceSheetReport', [$report]);
            $columns = $this->callPrivate($print, 'loadColumnDefinitions');

            $rowCount = 0;
            $numericRows = 0;
            $absTotal = 0.0;

            foreach ($report->reportData as $data) {
                $head = $data->reportRowHead;
                if (!$head) {
                    continue;
                }

                $rowPrint = new ReportRowPrint();
                $rowPrint->mount($period, $year, $head->id, null, $columns, $isBalanceSheet);

                $patterns = $head->reportRowDetail
                    ->filter(fn ($item) => !empty($item->kode))
                    ->pluck('kode')
                    ->unique()
                    ->toArray();

                $this->callPrivate($rowPrint, 'loadRawBalances', [$patterns]);
                if (collect($columns)->contains(fn ($col) => ($col['content'] ?? '') === 'Budget')) {
                    $this->callPrivate($rowPrint, 'loadBudgetData', [$patterns]);
                }

                $processedRows = $this->callPrivate($rowPrint, 'processReportRows', [$head->reportRowDetail]);
                $rowCount += count($processedRows);

                foreach ($processedRows as $row) {
                    foreach ($row->columns as $value) {
                        if (is_numeric($value) && abs((float) $value) > 0.0001) {
                            $numericRows++;
                            $absTotal += abs((float) $value);
                            break;
                        }
                    }
                }
            }

            $this->line(sprintf(
                '%d|%s|cols=%d|rows=%d|numericRows=%d|absTotal=%.2f',
                $report->id,
                $report->name,
                count($columns),
                $rowCount,
                $numericRows,
                $absTotal
            ));

            if ($rowCount === 0 || $numericRows === 0) {
                $this->error("Report {$report->id} produced no usable rows.");
                return self::FAILURE;
            }
        }

        return self::SUCCESS;
    }

    private function callPrivate(object $object, string $method, array $args = []): mixed
    {
        $reflection = new ReflectionClass($object);
        return $reflection->getMethod($method)->invokeArgs($object, $args);
    }
}
