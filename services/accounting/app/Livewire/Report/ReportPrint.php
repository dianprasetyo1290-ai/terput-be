<?php

namespace App\Livewire\Report;

use App\Models\ReportFinal;
use App\Models\ReportColHead;
use Livewire\Attributes\Layout;
use Livewire\Component;

class ReportPrint extends Component
{
    public $report;
    public $periode;
    public $tahun;
    public $divisi;
    public $columnDefinitions = [];
    public $isBalanceSheetReport = false;
    public $periodeLabel;

    protected $queryString = ['periode', 'tahun', 'divisi'];

    public function mount($id = null)
    {
        if ($id) {
            $this->report = ReportFinal::with(['reportData.reportRowHead.reportRowDetail', 'reportColHead.reportColDetails'])->find($id);
        }
        if (! $this->report) {
            // No (or an unknown) report: back to the report list, not a 500.
            abort(new \Illuminate\Http\RedirectResponse(route('report.financial')));
        }

        if (!isset($this->periode)) {
            $this->periode = (int) date('n');
        } else {
            $this->periode = (int) $this->periode;
        }

        if (!isset($this->tahun)) {
            $this->tahun = (int) date('Y');
        } else {
            $this->tahun = (int) $this->tahun;
        }

        if (isset($this->divisi) && $this->divisi === '') {
            $this->divisi = null;
        }

        // Determine if this is a Balance Sheet report
        if ($this->report) {
            $this->isBalanceSheetReport = $this->detectBalanceSheetReport($this->report);

            // Load column definitions based on the linked column template
            $this->columnDefinitions = $this->loadColumnDefinitions();

            // Build friendly period label
            $this->periodeLabel = $this->buildPeriodeLabel();
        }
    }

    /**
     * Detect if this report should use Balance Sheet period accumulation.
     * Income Statement reports must keep "Curr" as the selected month only.
     */
    private function detectBalanceSheetReport(ReportFinal $report): bool
    {
        $reportName = strtoupper(trim((string) $report->name));
        $printTitle = strtoupper(trim((string) $report->print_title));

        return str_contains($reportName, 'BALANCE SHEET')
            || str_contains($printTitle, 'BALANCE SHEET');
    }

    /**
     * Load column definitions for the current report and period.
     * Returns array of column definition arrays with keys: sort_order, col_title, content, period, etc.
     */
    private function loadColumnDefinitions(): array
    {
        if (!$this->report || !$this->report->reportColHead) {
            // Fallback: return default 3-column Monthly layout
            return $this->getDefaultColumns();
        }

        $colHead = $this->report->reportColHead;
        $columns = $colHead->getColumnsForPeriod($this->periode);

        $definitions = $columns->map(function ($col) {
            return [
                'sort_order' => $col->sort_order,
                'col_title' => $col->col_title,
                'content' => $col->content,
                'period' => $col->period,
                'division' => $col->division,
                'col_format' => $col->col_format,
                'period_key' => $col->period_key,
            ];
        })->toArray();

        return $this->isBalanceSheetReport
            ? $this->normalizeBalanceSheetColumns($definitions)
            : $definitions;
    }

    private function normalizeBalanceSheetColumns(array $columns): array
    {
        return array_map(function (array $column) {
            if (($column['content'] ?? '') === 'Current' && in_array(($column['period'] ?? ''), ['YTD', 'Beg'], true)) {
                $column['col_title'] = '31 Desember ' . ((int) $this->tahun - 1);
                $column['period'] = 'Previous Year Closing';
            }

            return $column;
        }, $columns);
    }

    private function buildPeriodeLabel(): string
    {
        $colType = $this->report?->reportColHead?->col_type;
        $quarterLabels = [
            3  => 'Q1 (Jan - Mar)',
            6  => 'Q2 (Apr - Jun)',
            9  => 'Q3 (Jul - Sep)',
            12 => 'Q4 (Oct - Dec)',
        ];

        if (in_array($colType, ['bs_quarterly', 'is_quarterly']) && isset($quarterLabels[$this->periode])) {
            return $quarterLabels[$this->periode];
        }

        return str_pad($this->periode, 2, '0', STR_PAD_LEFT) . ' [' . getMonthName($this->periode) . ']';
    }

    private function getDefaultColumns(): array
    {
        $columns = [
            ['sort_order' => 1, 'col_title' => 'This Month', 'content' => 'Current', 'period' => 'Curr', 'division' => '*', 'col_format' => '#,##0.00', 'period_key' => null],
            ['sort_order' => 2, 'col_title' => 'Last Month', 'content' => 'Current', 'period' => 'Curr - 01', 'division' => '*', 'col_format' => '#,##0.00', 'period_key' => null],
            ['sort_order' => 5, 'col_title' => 'Year To Date', 'content' => 'Current', 'period' => 'YTD', 'division' => '*', 'col_format' => '#,##0.00', 'period_key' => null],
        ];

        return $this->isBalanceSheetReport
            ? $this->normalizeBalanceSheetColumns($columns)
            : $columns;
    }

    #[Layout('layouts.rpt')]
    public function render()
    {
        return view('livewire.report.report-print');
    }
}
