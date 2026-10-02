<?php

namespace App\Livewire\Report;

use App\Models\ReportRowHead;
use App\Models\ChartOfAccount;
use App\Models\MstBudget;
use App\Models\ReportColDetail;
use Livewire\Component;
use Livewire\Attributes\Layout;
use App\Models\MstSaldoNew as MstSaldoNews;
use Illuminate\Support\Facades\Log;

class ReportRowPrint extends Component
{
    protected $queryString = ['idHead'];

    public $idHead = 1;
    public $periode = 1;
    public $tahun = 2024;
    public $divisi = null;

    // Column definitions passed from parent (ReportPrint)
    public $columnDefinitions = [];
    // Whether this report section uses BS-type accounts
    public $isBalanceSheetReport = false;

    // Raw balance data: [acc_code][periode] => ['debet' => float, 'credit' => float]
    private $rawBalances = [];
    // Budget data cache: [acc_code] => [1 => val, 2 => val, ...]
    private $budgetCache = [];
    // Base row value for Base% calculations (Total Revenue identifier)
    private $baseRowIdentifier = '0441'; // TOTAL BROKERAGE FEES in IS
    private ?float $openingRetainedEarningsAdjustment = null;

    public function mount($periode = null, $tahun = null, $idHead = null, $divisi = null, $columnDefinitions = [], $isBalanceSheetReport = false)
    {
        if ($periode !== null) $this->periode = (int) $periode;
        if ($tahun !== null) $this->tahun = (int) $tahun;
        if ($idHead !== null) $this->idHead = (int) $idHead;

        if ($divisi !== null && $divisi !== '') {
            $this->divisi = $divisi;
        } else {
            $this->divisi = null;
        }

        $this->columnDefinitions = $columnDefinitions;
        $this->isBalanceSheetReport = $isBalanceSheetReport;
    }

    #[Layout('layouts.rpt')]
    public function render()
    {
        $reportHead = ReportRowHead::with(['reportRowDetail'])->find($this->idHead);

        if (!$reportHead) {
            session()->flash('error', 'Report header not found for ID: ' . $this->idHead);
            return view('livewire.report.report-row-print', [
                'dataRow' => null,
                'processedRows' => [],
                'columnDefinitions' => $this->columnDefinitions,
            ]);
        }

        // Collect all unique account code patterns
        $accountPatterns = $reportHead->reportRowDetail
            ->filter(fn($item) => !empty($item->kode))
            ->pluck('kode')
            ->unique()
            ->toArray();

        // Load ALL raw balance data (all periods for the year)
        $this->loadRawBalances($accountPatterns);

        // Load budget data if needed
        if ($this->needsBudgetData()) {
            $this->loadBudgetData($accountPatterns);
        }

        // Process rows with dynamic columns
        $processedRows = $this->processReportRows($reportHead->reportRowDetail);

        return view('livewire.report.report-row-print', [
            'dataRow' => $reportHead,
            'processedRows' => $processedRows,
            'columnDefinitions' => $this->columnDefinitions,
        ]);
    }

    /**
     * Load ALL saldo data for relevant accounts for the entire year.
     * Stores as: $this->rawBalances[acc_code][periode] = ['debet' => float, 'credit' => float]
     * Also loads previous year data when needed (for Curr-01 when periode=1).
     */
    private function loadRawBalances(array $accountPatterns): void
    {
        $this->rawBalances = [];

        if (empty($accountPatterns)) return;

        // Load all periods (0-13) for current year
        $this->loadBalancesForYear($this->tahun, $accountPatterns);

        // Period 1 needs last year's period 12 for "Curr - 01".
        if ($this->periode == 1 || $this->needsPreviousYearClosing()) {
            $this->loadBalancesForYear($this->tahun - 1, $accountPatterns);
        }
    }

    private function loadBalancesForYear(int $year, array $accountPatterns): void
    {
        $query = MstSaldoNews::selectRaw('acc_code, periode, SUM(debet) as total_debet, SUM(credit) as total_credit')
            ->where('tahun', $year)
            ->whereBetween('periode', [0, 13]);

        if ($this->divisi !== null) {
            $query->where('div_code', $this->divisi);
        }

        $query->where(function ($q) use ($accountPatterns) {
            foreach ($accountPatterns as $pattern) {
                // Handle comma-separated patterns like "1101%,1102%"
                $parts = explode(',', $pattern);
                foreach ($parts as $part) {
                    $q->orWhere('acc_code', 'LIKE', trim($part));
                }
            }
        });

        $balances = $query->groupBy('acc_code', 'periode')->get();

        foreach ($balances as $b) {
            $key = $year . '_' . $b->acc_code;
            if (!isset($this->rawBalances[$key])) {
                $this->rawBalances[$key] = [];
            }
            $this->rawBalances[$key][$b->periode] = [
                'debet' => (float) $b->total_debet,
                'credit' => (float) $b->total_credit,
            ];
        }
    }

    /**
     * Check if any column needs budget data.
     */
    private function needsBudgetData(): bool
    {
        foreach ($this->columnDefinitions as $col) {
            if (($col['content'] ?? '') === 'Budget') return true;
        }
        return false;
    }

    private function needsPreviousYearClosing(): bool
    {
        foreach ($this->columnDefinitions as $col) {
            if (($col['period'] ?? '') === 'Previous Year Closing') return true;
        }

        return false;
    }

    /**
     * Load budget data for all account patterns.
     */
    private function loadBudgetData(array $accountPatterns): void
    {
        $this->budgetCache = [];

        if (empty($accountPatterns)) return;

        $query = MstBudget::select('*');

        if ($this->divisi !== null) {
            $query->where('div_code', $this->divisi);
        }

        $query->where(function ($q) use ($accountPatterns) {
            foreach ($accountPatterns as $pattern) {
                $parts = explode(',', $pattern);
                foreach ($parts as $part) {
                    $q->orWhere('acc_code', 'LIKE', trim($part));
                }
            }
        });

        foreach ($query->get() as $budget) {
            $key = $budget->acc_code;
            if (!isset($this->budgetCache[$key])) {
                $this->budgetCache[$key] = [];
            }
            for ($p = 1; $p <= 13; $p++) {
                $col = 'act_' . str_pad($p, 2, '0', STR_PAD_LEFT);
                $existing = $this->budgetCache[$key][$p] ?? 0;
                $this->budgetCache[$key][$p] = $existing + (float) ($budget->$col ?? 0);
            }
        }
    }

    /**
     * Resolve which periods to sum for a given period expression and account type.
     *
     * @param string $periodExpr Period expression (Curr, Curr - 01, Beg, YTD, 01, 01,02,03, etc.)
     * @param bool $isBS Whether account is Balance Sheet (cumulative)
     * @return array ['periods' => [int], 'year' => int]
     */
    private function resolvePeriods(string $periodExpr, bool $isBS): array
    {
        $periodExpr = trim($periodExpr);
        $year = $this->tahun;

        if ($periodExpr === 'Curr') {
            if ($isBS) {
                return ['periods' => range(0, $this->periode), 'year' => $year];
            }
            return ['periods' => [$this->periode], 'year' => $year];
        }

        if ($periodExpr === 'Curr - 01') {
            $lastPeriod = $this->periode - 1;
            $lastYear = $this->tahun;
            if ($lastPeriod < 1) {
                if (!$isBS) {
                    return ['periods' => [], 'year' => $year];
                }
                $lastPeriod = 12;
                $lastYear--;
            }
            if ($isBS) {
                return ['periods' => range(0, $lastPeriod), 'year' => $lastYear];
            }
            return ['periods' => [$lastPeriod], 'year' => $lastYear];
        }

        if ($periodExpr === 'Beg') {
            return ['periods' => [0], 'year' => $year];
        }

        if ($periodExpr === 'YTD') {
            if ($isBS) {
                return ['periods' => range(0, $this->periode), 'year' => $year];
            }
            return ['periods' => range(1, $this->periode), 'year' => $year];
        }

        // Specific periods: "01", "04", "01,02,03"
        $specificPeriods = array_map(fn($p) => (int) trim($p), explode(',', $periodExpr));
        if ($isBS) {
            // For BS with specific periods: cumulative from 0 to max(specified)
            $maxPeriod = max($specificPeriods);
            return ['periods' => range(0, $maxPeriod), 'year' => $year];
        }
        return ['periods' => $specificPeriods, 'year' => $year];
    }

    /**
     * Resolve one or more year/period ranges for a report column.
     * Balance Sheet columns include previous year closing as opening balance.
     *
     * @return array<int, array{periods: array<int>, year: int}>
     */
    private function resolvePeriodRanges(string $periodExpr, bool $isBS): array
    {
        $periodExpr = trim($periodExpr);

        if (!$isBS) {
            return [$this->resolvePeriods($periodExpr, false)];
        }

        $openingBalance = ['periods' => [0], 'year' => $this->tahun];

        if ($periodExpr === 'Beg') {
            return [$openingBalance];
        }

        if ($periodExpr === 'Curr') {
            return [
                $openingBalance,
                ['periods' => range(1, $this->periode), 'year' => $this->tahun],
            ];
        }

        if ($periodExpr === 'Curr - 01') {
            $lastPeriod = $this->periode - 1;
            if ($lastPeriod < 1) {
                return [$openingBalance];
            }

            return [
                $openingBalance,
                ['periods' => range(1, $lastPeriod), 'year' => $this->tahun],
            ];
        }

        if ($periodExpr === 'YTD') {
            return [
                $openingBalance,
                ['periods' => range(1, $this->periode), 'year' => $this->tahun],
            ];
        }

        if ($periodExpr === 'Previous Year Closing') {
            return [$openingBalance];
        }

        if (preg_match('/^[\d,\s]+$/', $periodExpr)) {
            $specificPeriods = array_map(fn($p) => (int) trim($p), explode(',', $periodExpr));
            $maxPeriod = max($specificPeriods);

            return [
                $openingBalance,
                ['periods' => range(1, $maxPeriod), 'year' => $this->tahun],
            ];
        }

        return [$openingBalance];
    }

    /**
     * Cash Flow exported rows use Row_Content B/E in the legacy app. The current
     * DB does not store that flag, so infer it from the stable row labels.
     *
     * @return array<int, array{periods: array<int>, year: int}>
     */
    private function resolveBalanceRowRanges(string $periodExpr, bool $endingBalance): array
    {
        $periodExpr = trim($periodExpr);
        $endPeriod = match ($periodExpr) {
            'Beg' => 0,
            'Curr - 01' => max(0, $this->periode - ($endingBalance ? 1 : 2)),
            'YTD' => $endingBalance ? $this->periode : 0,
            default => $endingBalance ? $this->periode : max(0, $this->periode - 1),
        };

        if (preg_match('/^[\d,\s]+$/', $periodExpr)) {
            $specificPeriods = array_map(fn($p) => (int) trim($p), explode(',', $periodExpr));
            $maxPeriod = max($specificPeriods);
            $endPeriod = $endingBalance ? $maxPeriod : max(0, $maxPeriod - 1);
        }

        $ranges = [['periods' => [0], 'year' => $this->tahun]];
        if ($endPeriod >= 1) {
            $ranges[] = ['periods' => range(1, $endPeriod), 'year' => $this->tahun];
        }

        return $ranges;
    }

    /**
     * Sum balance values for an account pattern across specified periods.
     */
    private function sumBalances(string $accPattern, array $periods, int $year, string $normalBalance): float
    {
        $total = 0.0;
        $normalBalance = strtoupper($normalBalance);

        // Handle comma-separated patterns
        $patterns = explode(',', $accPattern);
        foreach ($patterns as $pat) {
            $cleanPat = rtrim(trim($pat), '%');

            foreach ($this->rawBalances as $key => $periodData) {
                if (!str_starts_with($key, $year . '_' . $cleanPat)) continue;

                foreach ($periods as $p) {
                    if (!isset($periodData[$p])) continue;
                    $d = $periodData[$p]['debet'] ?? 0;
                    $c = $periodData[$p]['credit'] ?? 0;

                    if ($normalBalance === 'C') {
                        $total += ($c - $d);
                    } else {
                        $total += ($d - $c);
                    }
                }
            }
        }

        return $total;
    }

    private function sumBalanceRanges(string $accPattern, array $ranges, string $normalBalance): float
    {
        $total = 0.0;
        foreach ($ranges as $range) {
            $total += $this->sumBalances(
                $accPattern,
                $range['periods'] ?? [],
                (int) ($range['year'] ?? $this->tahun),
                $normalBalance
            );
        }

        return $total;
    }

    private function sumPreviousYearClosingBalance(string $accPattern, string $normalBalance): float
    {
        return $this->sumBalanceRanges(
            $accPattern,
            [['periods' => [0], 'year' => $this->tahun]],
            $normalBalance
        );
    }

    /**
     * Sum budget values for an account pattern for specified periods.
     */
    private function sumBudget(string $accPattern, string $periodExpr, string $normalBalance): float
    {
        $periodExpr = trim($periodExpr);

        // Determine which period columns to sum
        $periods = [];
        if ($periodExpr === 'Curr') {
            $periods = [$this->periode];
        } elseif ($periodExpr === 'YTD') {
            $periods = range(1, $this->periode);
        } elseif (preg_match('/^[\d,\s]+$/', $periodExpr)) {
            $periods = array_map(fn($p) => (int) trim($p), explode(',', $periodExpr));
        } else {
            $periods = [$this->periode];
        }

        $total = 0.0;
        $patterns = explode(',', $accPattern);
        foreach ($patterns as $pat) {
            $cleanPat = rtrim(trim($pat), '%');
            foreach ($this->budgetCache as $accCode => $budgetData) {
                if (!str_starts_with($accCode, $cleanPat)) continue;
                foreach ($periods as $p) {
                    $total += ($budgetData[$p] ?? 0);
                }
            }
        }

        // Apply normal balance sign (budget values are usually stored as positive)
        return $total;
    }

    /**
     * Determine if an account code pattern represents a Balance Sheet account.
     */
    private function isBalanceSheetAccount(string $accCodePattern): bool
    {
        return $this->isBalanceSheetReport;
    }

    /**
     * Resolve a column's value for an account-based row.
     */
    private function resolveColumnValue(string $accPattern, array $colDef, string $normalBalance, ?string $rowName = null): float
    {
        $content = $colDef['content'] ?? 'Current';
        $period = $colDef['period'] ?? 'Curr';
        $isBS = $this->isBalanceSheetAccount($accPattern);

        if ($content === 'Current') {
            if ($period === 'Previous Year Closing') {
                return $this->sumPreviousYearClosingBalance($accPattern, $normalBalance);
            }

            $upperRowName = strtoupper(trim((string) $rowName));
            if (!$this->isBalanceSheetReport && str_contains($upperRowName, 'BEGIN')) {
                return $this->sumBalanceRanges(
                    $accPattern,
                    $this->resolveBalanceRowRanges($period, false),
                    $normalBalance
                );
            }

            if (!$this->isBalanceSheetReport && str_contains($upperRowName, 'ENDING')) {
                return $this->sumBalanceRanges(
                    $accPattern,
                    $this->resolveBalanceRowRanges($period, true),
                    $normalBalance
                );
            }

            $ranges = $this->resolvePeriodRanges($period, $isBS);
            return $this->sumBalanceRanges($accPattern, $ranges, $normalBalance);
        }

        if ($content === 'Budget') {
            return $this->sumBudget($accPattern, $period, $normalBalance);
        }

        return 0.0;
    }

    /**
     * Evaluate a column formula that references other columns by sort_order.
     * Formula format: {0001} - {0002} where 0001 is zero-padded sort_order.
     */
    private function evaluateColumnFormula(string $formula, array $columnValues): float
    {
        $evaluated = preg_replace_callback(
            '/\{(\d+)\}/',
            function ($matches) use ($columnValues) {
                $sortOrder = (int) $matches[1];
                return '(' . (string) ($columnValues[$sortOrder] ?? 0.0) . ')';
            },
            $formula
        );

        $evaluated = str_replace(' ', '', $evaluated);

        try {
            $result = eval('return ' . $evaluated . ';');
            return (float) $result;
        } catch (\Throwable $e) {
            Log::error("Column formula error: {$e->getMessage()}. Formula: {$formula}");
            return 0.0;
        }
    }

    /**
     * Evaluate a row formula that references other rows by identifier.
     * Formula format: {011x} + {0130x}
     */
    private function evaluateRowFormula(string $formula, array $dataMap): float
    {
        $evaluated = preg_replace_callback(
            '/\{([0-9a-zA-Z_x]+)\}/',
            function ($matches) use ($dataMap) {
                return '(' . (string) ($dataMap[$matches[1]] ?? 0.0) . ')';
            },
            $formula
        );

        $evaluated = str_replace(' ', '', $evaluated);

        try {
            $result = eval('return ' . $evaluated . ';');
            return (float) $result;
        } catch (\Throwable $e) {
            Log::error("Row formula error: {$e->getMessage()}. Formula: {$formula}");
            return 0.0;
        }
    }

    /**
     * Legacy Access reports auto-total rows ending in x/z even when Formula is empty.
     * The identifier prefix defines the subtotal scope, and later subtotals replace
     * their covered detail rows so parent totals do not double count children.
     */
    private function calculateLegacyAutoTotal(?string $identifier, array $dataMap): float
    {
        $prefix = $this->legacyTotalPrefix($identifier);
        if ($prefix === null) {
            return 0.0;
        }

        $selected = [];
        foreach ($dataMap as $candidateIdentifier => $value) {
            $candidateIdentifier = (string) $candidateIdentifier;
            if ($candidateIdentifier === $identifier) {
                continue;
            }

            if (!str_starts_with($candidateIdentifier, $prefix)) {
                continue;
            }

            $coveragePrefix = $this->legacyTotalPrefix($candidateIdentifier);
            if ($coveragePrefix !== null) {
                foreach (array_keys($selected) as $selectedIdentifier) {
                    if (str_starts_with((string) $selectedIdentifier, $coveragePrefix)) {
                        unset($selected[$selectedIdentifier]);
                    }
                }
            }

            $selected[$candidateIdentifier] = (float) $value;
        }

        return array_sum($selected);
    }

    private function legacyTotalPrefix(?string $identifier): ?string
    {
        $identifier = trim((string) $identifier);
        if ($identifier === '') {
            return null;
        }

        $lastChar = strtolower(substr($identifier, -1));
        if (!in_array($lastChar, ['x', 'z'], true)) {
            return null;
        }

        $prefix = substr($identifier, 0, -1);
        return $prefix !== '' ? $prefix : null;
    }

    /**
     * Process all report rows, calculating values for each dynamic column.
     */
    private function processReportRows($reportDetails): array
    {
        $processedRows = [];
        $columns = $this->columnDefinitions;

        // Per-column calculated value maps for row formula evaluation
        // $calculatedValues[sort_order][identifier] = value
        $calculatedValues = [];
        foreach ($columns as $col) {
            $calculatedValues[$col['sort_order']] = [];
        }

        // Base row values for Base% calculation (per column)
        $baseRowValues = [];

        foreach ($reportDetails as $item) {
            $rowData = (object) [
                'id' => $item->id,
                'name' => $item->name,
                'group' => $item->group,
                'level' => $item->level,
                'identifier' => $item->identifier,
                'columns' => [], // Dynamic column values keyed by sort_order
            ];

            if ($item->group === 'h') {
                // Header row — null values for all columns
                foreach ($columns as $col) {
                    $rowData->columns[$col['sort_order']] = null;
                }
            } elseif (in_array($item->group, ['d', 't', 'gt'])) {
                // First pass: calculate Current/Budget values
                $currentRowColumnValues = [];

                foreach ($columns as $col) {
                    $sortOrder = $col['sort_order'];
                    $content = $col['content'] ?? 'Current';
                    $value = 0.0;

                    if ($content === 'Formula' || $content === 'Base%') {
                        // Will be resolved in second pass
                        $currentRowColumnValues[$sortOrder] = null;
                        continue;
                    }

                    $rowFormula = trim((string) ($item->formula ?? ''));
                    if ($rowFormula !== '') {
                        // Row has formula — evaluate using previously calculated row values
                        $value = $this->evaluateRowFormula($rowFormula, $calculatedValues[$sortOrder] ?? []);
                    } elseif (!empty($item->kode)) {
                        // Account-based row
                        $value = $this->resolveColumnValue($item->kode, $col, $item->normal_balance ?? 'D', $item->name ?? null);
                    } elseif (in_array($item->group, ['t', 'gt'], true)) {
                        // Legacy Access auto-subtotal for total rows with empty Formula/Account.
                        $value = $this->calculateLegacyAutoTotal($item->identifier, $calculatedValues[$sortOrder] ?? []);
                    }

                    if ($this->isBalanceSheetReport && trim((string) $item->identifier) === '024002') {
                        $value += $this->calculateOpeningRetainedEarningsAdjustment();
                    }

                    $currentRowColumnValues[$sortOrder] = $value;
                }

                // Second pass: resolve Formula and Base% columns
                foreach ($columns as $col) {
                    $sortOrder = $col['sort_order'];
                    $content = $col['content'] ?? 'Current';

                    if ($content === 'Formula') {
                        // Column formula references other columns of the same row
                        $colFormula = $col['period'] ?? '';
                        $value = $this->evaluateColumnFormula($colFormula, $currentRowColumnValues);
                        $currentRowColumnValues[$sortOrder] = $value;
                    } elseif ($content === 'Base%') {
                        // Percentage relative to base row's value in referenced column
                        $refColSortOrder = 0;
                        if (preg_match('/\{(\d+)\}/', $col['period'] ?? '', $m)) {
                            $refColSortOrder = (int) $m[1];
                        }
                        $currentValue = $currentRowColumnValues[$refColSortOrder] ?? 0.0;
                        $baseValue = $baseRowValues[$refColSortOrder] ?? 0.0;

                        if (abs($baseValue) > 0.001) {
                            $currentRowColumnValues[$sortOrder] = ($currentValue / $baseValue) * 100;
                        } else {
                            $currentRowColumnValues[$sortOrder] = 0.0;
                        }
                    }
                }

                // Store calculated values
                foreach ($currentRowColumnValues as $sortOrder => $value) {
                    $rowData->columns[$sortOrder] = $value;

                    // Track in calculated values map for formula references
                    if (!empty($item->identifier)) {
                        $calculatedValues[$sortOrder][$item->identifier] = $value ?? 0.0;
                    }
                }

                // Track base row values for Base% (Total Brokerage Fees = 0441)
                if ($item->identifier === $this->baseRowIdentifier) {
                    foreach ($currentRowColumnValues as $sortOrder => $value) {
                        $baseRowValues[$sortOrder] = $value ?? 0.0;
                    }
                }
            }

            $processedRows[] = $rowData;
        }

        return $processedRows;
    }

    private function calculateOpeningRetainedEarningsAdjustment(): float
    {
        if ($this->openingRetainedEarningsAdjustment !== null) {
            return $this->openingRetainedEarningsAdjustment;
        }

        $openingRange = [['periods' => [0], 'year' => $this->tahun]];

        $assets = $this->sumBalanceRanges(
            '1101%,1102%,1103%,1113%,1104%,1105%,1107%,1201%,1202%,13011%',
            $openingRange,
            'D'
        );
        $liabilities = $this->sumBalanceRanges(
            '2101200%,2203%,2101201%,2205%,2206%',
            $openingRange,
            'C'
        );
        $paidInCapital = $this->sumBalanceRanges('31013%', $openingRange, 'C');
        $surplusPriorYears = $this->sumBalanceRanges('38%', $openingRange, 'C');

        $this->openingRetainedEarningsAdjustment = $assets - $liabilities - $paidInCapital - $surplusPriorYears;

        return $this->openingRetainedEarningsAdjustment;
    }
}
