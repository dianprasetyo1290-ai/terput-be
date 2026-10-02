<?php

namespace Tests\Unit;

use App\Livewire\Report\ReportPrint;
use App\Livewire\Report\ReportRowPrint;
use App\Models\ReportFinal;
use App\Models\ReportRowDetail;
use Illuminate\Support\Collection;
use PHPUnit\Framework\TestCase;
use ReflectionClass;

class ReportPrintCalculationTest extends TestCase
{
    public function test_income_statement_report_four_is_not_treated_as_balance_sheet(): void
    {
        $component = new ReportPrint();
        $report = new ReportFinal();
        $report->id = 4;
        $report->name = 'Income Statement - Monthly';
        $report->print_title = 'INCOME STATEMENT COMPARATIVE';

        $this->assertFalse($this->callPrivate($component, 'detectBalanceSheetReport', [$report]));
    }

    public function test_financial_report_balance_sheet_names_use_balance_sheet_accumulation(): void
    {
        $component = new ReportPrint();
        $report = new ReportFinal();
        $report->id = 1;
        $report->name = 'Balance Sheet - Monthly';
        $report->print_title = 'BALANCE SHEET COMPARATIVE';

        $this->assertTrue($this->callPrivate($component, 'detectBalanceSheetReport', [$report]));
    }

    public function test_balance_sheet_ytd_column_is_labeled_as_previous_year_december_closing(): void
    {
        $component = new ReportPrint();
        $component->tahun = 2026;

        $columns = $this->callPrivate($component, 'normalizeBalanceSheetColumns', [[
            ['sort_order' => 1, 'col_title' => 'This Month', 'content' => 'Current', 'period' => 'Curr'],
            ['sort_order' => 2, 'col_title' => 'Last Month', 'content' => 'Current', 'period' => 'Curr - 01'],
            ['sort_order' => 5, 'col_title' => 'Year To Date', 'content' => 'Current', 'period' => 'YTD'],
        ]]);

        $this->assertSame('31 Desember 2025', $columns[2]['col_title']);
        $this->assertSame('Previous Year Closing', $columns[2]['period']);
    }

    public function test_balance_sheet_beginning_column_is_labeled_as_previous_year_december_closing(): void
    {
        $component = new ReportPrint();
        $component->tahun = 2026;

        $columns = $this->callPrivate($component, 'normalizeBalanceSheetColumns', [[
            ['sort_order' => 40, 'col_title' => 'Beginning', 'content' => 'Current', 'period' => 'Beg'],
        ]]);

        $this->assertSame('31 Desember 2025', $columns[0]['col_title']);
        $this->assertSame('Previous Year Closing', $columns[0]['period']);
    }

    public function test_balance_sheet_previous_year_closing_uses_current_year_opening_balance(): void
    {
        $component = new ReportRowPrint();
        $component->tahun = 2026;

        $ranges = $this->callPrivate($component, 'resolvePeriodRanges', ['Previous Year Closing', true]);

        $this->assertSame([['periods' => [0], 'year' => 2026]], $ranges);
    }

    public function test_balance_sheet_previous_year_closing_ignores_partial_period_thirteen_adjustment(): void
    {
        $component = new ReportRowPrint();
        $component->tahun = 2026;
        $component->isBalanceSheetReport = true;
        $this->setPrivate($component, 'rawBalances', [
            '2025_11010000' => [13 => ['debet' => 25.0, 'credit' => 0.0]],
            '2026_11010000' => [0 => ['debet' => 500.0, 'credit' => 100.0]],
        ]);

        $value = $this->callPrivate($component, 'resolveColumnValue', [
            '1101%',
            ['sort_order' => 5, 'content' => 'Current', 'period' => 'Previous Year Closing'],
            'D',
        ]);

        $this->assertSame(400.0, $value);
    }

    public function test_income_statement_current_period_uses_only_selected_month(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 12;
        $component->tahun = 2025;

        $resolved = $this->callPrivate($component, 'resolvePeriods', ['Curr', false]);

        $this->assertSame([12], $resolved['periods']);
        $this->assertSame(2025, $resolved['year']);
    }

    public function test_income_statement_january_last_month_does_not_roll_back_to_previous_year(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 1;
        $component->tahun = 2025;

        $resolved = $this->callPrivate($component, 'resolvePeriods', ['Curr - 01', false]);

        $this->assertSame([], $resolved['periods']);
        $this->assertSame(2025, $resolved['year']);
    }

    public function test_credit_normal_balance_is_case_insensitive(): void
    {
        $component = new ReportRowPrint();
        $component->tahun = 2025;
        $this->setPrivate($component, 'rawBalances', [
            '2025_40005001' => [
                12 => ['debet' => 100.0, 'credit' => 450.0],
            ],
        ]);

        $value = $this->callPrivate($component, 'sumBalances', ['40005001%', [12], 2025, 'c']);

        $this->assertSame(350.0, $value);
    }

    public function test_total_row_without_formula_sums_matching_legacy_children(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 12;
        $component->tahun = 2025;
        $component->columnDefinitions = [
            ['sort_order' => 1, 'content' => 'Current', 'period' => 'Curr'],
        ];
        $this->setPrivate($component, 'rawBalances', [
            '2025_4000500100' => [12 => ['debet' => 0.0, 'credit' => 100.0]],
            '2025_4000500200' => [12 => ['debet' => 0.0, 'credit' => 25.0]],
        ]);

        $rows = new Collection([
            $this->reportRow('0100100', 'd', 'Brokerage Fees - Local', 'C', '40005001%'),
            $this->reportRow('0100110', 'd', 'Brokerage Fees - Int', 'C', '40005002%'),
            $this->reportRow('01001x', 't', 'Total Brokerage Fees', 'C'),
        ]);

        $processedRows = $this->callPrivate($component, 'processReportRows', [$rows]);

        $this->assertSame(125.0, $processedRows[2]->columns[1]);
    }

    public function test_parent_total_without_formula_uses_subtotals_without_double_counting_details(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 12;
        $component->tahun = 2025;
        $component->columnDefinitions = [
            ['sort_order' => 1, 'content' => 'Current', 'period' => 'Curr'],
        ];
        $this->setPrivate($component, 'rawBalances', [
            '2025_4000500100' => [12 => ['debet' => 0.0, 'credit' => 100.0]],
            '2025_5000601000' => [12 => ['debet' => 40.0, 'credit' => 0.0]],
        ]);

        $rows = new Collection([
            $this->reportRow('0100100', 'd', 'Brokerage Fees - Local', 'C', '40005001%'),
            $this->reportRow('01001x', 't', 'Total Brokerage Fees', 'C'),
            $this->reportRow('0101100', 'd', 'Personnel Expenses', 'C', '50006010%'),
            $this->reportRow('0101x', 't', 'Total Expenses', 'C'),
            $this->reportRow('010x', 't', 'Gross Profit (Loss) - FX', 'C'),
        ]);

        $processedRows = $this->callPrivate($component, 'processReportRows', [$rows]);

        $this->assertSame(100.0, $processedRows[1]->columns[1]);
        $this->assertSame(-40.0, $processedRows[3]->columns[1]);
        $this->assertSame(60.0, $processedRows[4]->columns[1]);
    }

    public function test_total_row_with_blank_legacy_formula_uses_auto_total(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 12;
        $component->tahun = 2025;
        $component->columnDefinitions = [
            ['sort_order' => 1, 'content' => 'Current', 'period' => 'Curr'],
        ];
        $this->setPrivate($component, 'rawBalances', [
            '2025_11010000' => [12 => ['debet' => 100.0, 'credit' => 0.0]],
            '2025_12010000' => [12 => ['debet' => 25.0, 'credit' => 0.0]],
        ]);

        $rows = new Collection([
            $this->reportRow('011010', 'd', 'Cash', 'D', '1101%'),
            $this->reportRow('011x', 't', 'Total Current Assets', 'D'),
            $this->reportRow('013001', 'd', 'Fixed Assets - Cost', 'D', '1201%'),
            $this->reportRow('0130x', 't', 'Net Fixed Assets', 'D'),
            $this->reportRow('01x', 'gt', 'TOTAL ASSETS', 'D', null, ' '),
        ]);

        $processedRows = $this->callPrivate($component, 'processReportRows', [$rows]);

        $this->assertSame(125.0, $processedRows[4]->columns[1]);
    }

    public function test_row_formula_handles_negative_referenced_values(): void
    {
        $component = new ReportRowPrint();

        $value = $this->callPrivate($component, 'evaluateRowFormula', [
            '{01x}-{021x}-{024001}-{024002}',
            [
                '01x' => 100.0,
                '021x' => 30.0,
                '024001' => -10.0,
                '024002' => -5.0,
            ],
        ]);

        $this->assertSame(85.0, $value);
    }

    public function test_balance_sheet_retained_earning_formula_balances_assets_and_liabilities(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 12;
        $component->tahun = 2025;
        $component->isBalanceSheetReport = true;
        $component->columnDefinitions = [
            ['sort_order' => 1, 'content' => 'Current', 'period' => 'Curr'],
        ];
        $this->setPrivate($component, 'rawBalances', [
            '2025_11010000' => [12 => ['debet' => 100.0, 'credit' => 0.0]],
            '2025_2101200000' => [12 => ['debet' => 0.0, 'credit' => 30.0]],
            '2025_31013000' => [12 => ['debet' => 0.0, 'credit' => 10.0]],
            '2025_38000000' => [12 => ['debet' => 0.0, 'credit' => 5.0]],
        ]);

        $rows = new Collection([
            $this->reportRow('011010', 'd', 'Cash', 'D', '1101%'),
            $this->reportRow('011x', 't', 'Total Current Assets', 'D'),
            $this->reportRow('01x', 'gt', 'TOTAL ASSETS', 'D'),
            $this->reportRow('021010', 'd', 'Account Payable', 'C', '2101200%'),
            $this->reportRow('021x', 't', 'Total Current Liabilities', 'C'),
            $this->reportRow('024001', 'd', 'Paid In Capital', 'C', '31013%'),
            $this->reportRow('024002', 'd', 'Surplus Prior Years', 'C', '38%'),
            $this->reportRow('024003', 'd', 'Current Year Retained Earning', 'C', null, '{01x}-{021x}-{024001}-{024002}'),
            $this->reportRow('0240x', 't', 'Total Stockholders And Equity', 'C'),
            $this->reportRow('02x', 'gt', "TOTAL LIABILITIES & STOCKHOLDERS' EQUITY", 'C'),
        ]);

        $processedRows = $this->callPrivate($component, 'processReportRows', [$rows]);

        $this->assertSame(100.0, $processedRows[2]->columns[1]);
        $this->assertSame(55.0, $processedRows[7]->columns[1]);
        $this->assertSame(100.0, $processedRows[9]->columns[1]);
    }

    public function test_balance_sheet_moves_opening_retained_earnings_to_surplus_prior_years(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 8;
        $component->tahun = 2025;
        $component->isBalanceSheetReport = true;
        $component->columnDefinitions = [
            ['sort_order' => 1, 'content' => 'Current', 'period' => 'Curr'],
            ['sort_order' => 2, 'content' => 'Current', 'period' => 'Beg'],
        ];
        $this->setPrivate($component, 'rawBalances', [
            '2025_11010000' => [
                0 => ['debet' => 100.0, 'credit' => 0.0],
                8 => ['debet' => 20.0, 'credit' => 0.0],
            ],
            '2025_2101200000' => [
                0 => ['debet' => 0.0, 'credit' => 30.0],
            ],
            '2025_31013000' => [
                0 => ['debet' => 0.0, 'credit' => 10.0],
            ],
            '2025_38000000' => [
                0 => ['debet' => 0.0, 'credit' => 5.0],
            ],
        ]);

        $rows = new Collection([
            $this->reportRow('011010', 'd', 'Cash', 'D', '1101%'),
            $this->reportRow('01x', 'gt', 'TOTAL ASSETS', 'D'),
            $this->reportRow('021010', 'd', 'Account Payable', 'C', '2101200%'),
            $this->reportRow('021x', 't', 'Total Current Liabilities', 'C'),
            $this->reportRow('024001', 'd', 'Paid In Capital', 'C', '31013%'),
            $this->reportRow('024002', 'd', 'Surplus Prior Years', 'C', '38%'),
            $this->reportRow('024003', 'd', 'Current Year Retained Earning', 'C', null, '{01x}-{021x}-{024001}-{024002}'),
            $this->reportRow('02x', 'gt', "TOTAL LIABILITIES & STOCKHOLDERS' EQUITY", 'C'),
        ]);

        $processedRows = $this->callPrivate($component, 'processReportRows', [$rows]);

        $this->assertSame(60.0, $processedRows[5]->columns[2]);
        $this->assertSame(0.0, $processedRows[6]->columns[2]);
        $this->assertSame(20.0, $processedRows[6]->columns[1]);
        $this->assertSame(120.0, $processedRows[7]->columns[1]);
    }

    public function test_balance_sheet_current_and_beginning_include_current_year_period_zero_balance(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 9;
        $component->tahun = 2025;
        $component->isBalanceSheetReport = true;
        $component->columnDefinitions = [
            ['sort_order' => 1, 'content' => 'Current', 'period' => 'Curr'],
            ['sort_order' => 2, 'content' => 'Current', 'period' => 'Beg'],
        ];
        $this->setPrivate($component, 'rawBalances', [
            '2025_11010000' => [
                0 => ['debet' => 70.0, 'credit' => 0.0],
                1 => ['debet' => 10.0, 'credit' => 0.0],
                9 => ['debet' => 15.0, 'credit' => 0.0],
            ],
        ]);

        $rows = new Collection([
            $this->reportRow('011010', 'd', 'Cash', 'D', '1101%'),
        ]);

        $processedRows = $this->callPrivate($component, 'processReportRows', [$rows]);

        $this->assertSame(95.0, $processedRows[0]->columns[1]);
        $this->assertSame(70.0, $processedRows[0]->columns[2]);
    }

    public function test_non_balance_sheet_account_starting_with_one_uses_current_period_only(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 9;
        $component->tahun = 2025;
        $component->isBalanceSheetReport = false;
        $component->columnDefinitions = [
            ['sort_order' => 1, 'content' => 'Current', 'period' => 'Curr'],
        ];
        $this->setPrivate($component, 'rawBalances', [
            '2025_11030500' => [
                0 => ['debet' => 1000.0, 'credit' => 0.0],
                8 => ['debet' => 20.0, 'credit' => 0.0],
                9 => ['debet' => 30.0, 'credit' => 0.0],
            ],
        ]);

        $rows = new Collection([
            $this->reportRow('00005', 'd', 'Brokerage Receivables', 'C', '110305%'),
        ]);

        $processedRows = $this->callPrivate($component, 'processReportRows', [$rows]);

        $this->assertSame(-30.0, $processedRows[0]->columns[1]);
    }

    public function test_cash_flow_beginning_and_ending_balance_rows_use_balance_sheet_ranges(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 9;
        $component->tahun = 2025;
        $component->isBalanceSheetReport = false;
        $component->columnDefinitions = [
            ['sort_order' => 1, 'content' => 'Current', 'period' => 'Curr'],
            ['sort_order' => 2, 'content' => 'Current', 'period' => 'YTD'],
        ];
        $this->setPrivate($component, 'rawBalances', [
            '2025_11010000' => [
                0 => ['debet' => 100.0, 'credit' => 0.0],
                8 => ['debet' => 20.0, 'credit' => 0.0],
                9 => ['debet' => 30.0, 'credit' => 0.0],
            ],
        ]);

        $rows = new Collection([
            $this->reportRow('06', 'd', 'BEGINING BALANCE', 'D', '1101%'),
            $this->reportRow('07', 'd', 'ENDING BALANCE', 'D', '1101%'),
        ]);

        $processedRows = $this->callPrivate($component, 'processReportRows', [$rows]);

        $this->assertSame(120.0, $processedRows[0]->columns[1]);
        $this->assertSame(100.0, $processedRows[0]->columns[2]);
        $this->assertSame(150.0, $processedRows[1]->columns[1]);
        $this->assertSame(150.0, $processedRows[1]->columns[2]);
    }

    public function test_leading_zero_row_identifiers_do_not_collide_with_short_identifiers(): void
    {
        $component = new ReportRowPrint();
        $component->periode = 9;
        $component->tahun = 2025;
        $component->columnDefinitions = [
            ['sort_order' => 1, 'content' => 'Current', 'period' => 'Curr'],
        ];
        $this->setPrivate($component, 'rawBalances', [
            '2025_11030500' => [9 => ['debet' => 30.0, 'credit' => 0.0]],
        ]);

        $rows = new Collection([
            $this->reportRow('00005', 'd', 'Brokerage Receivables', 'C', '110305%'),
            $this->reportRow('05', 't', 'NET CASH FLOW', 'D', null, '{00005} + 100'),
        ]);

        $processedRows = $this->callPrivate($component, 'processReportRows', [$rows]);

        $this->assertSame(-30.0, $processedRows[0]->columns[1]);
        $this->assertSame(70.0, $processedRows[1]->columns[1]);
        $this->assertSame('NET CASH FLOW', $processedRows[1]->name);
    }

    private function callPrivate(object $object, string $method, array $args = []): mixed
    {
        $reflection = new ReflectionClass($object);
        $method = $reflection->getMethod($method);

        return $method->invokeArgs($object, $args);
    }

    private function setPrivate(object $object, string $property, mixed $value): void
    {
        $reflection = new ReflectionClass($object);
        $property = $reflection->getProperty($property);
        $property->setValue($object, $value);
    }

    private function reportRow(
        string $identifier,
        string $group,
        string $name,
        string $normalBalance = 'D',
        ?string $kode = null,
        ?string $formula = null,
    ): ReportRowDetail {
        $row = new ReportRowDetail();
        $row->id = crc32($identifier);
        $row->identifier = $identifier;
        $row->group = $group;
        $row->name = $name;
        $row->level = 0;
        $row->normal_balance = $normalBalance;
        $row->kode = $kode;
        $row->formula = $formula;

        return $row;
    }
}
