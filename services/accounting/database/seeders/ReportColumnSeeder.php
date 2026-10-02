<?php

namespace Database\Seeders;

use App\Models\ReportColHead;
use App\Models\ReportColDetail;
use App\Models\ReportFinal;
use Illuminate\Database\Seeder;

class ReportColumnSeeder extends Seeder
{
    public function run(): void
    {
        // Clear existing data
        ReportColDetail::query()->delete();
        ReportColHead::query()->delete();

        // 1. BS Monthly (CN000001) - 3 fixed columns
        $bsMonthly = ReportColHead::create([
            'name' => 'Standard Neraca Columns',
            'col_type' => 'fixed',
        ]);
        $this->createDetails($bsMonthly->id, [
            ['sort_order' => 10, 'col_title' => 'This Month', 'content' => 'Current', 'period' => 'Curr'],
            ['sort_order' => 30, 'col_title' => 'Last Month', 'content' => 'Current', 'period' => 'Curr - 01'],
            ['sort_order' => 40, 'col_title' => 'Beginning', 'content' => 'Current', 'period' => 'Beg'],
        ]);

        // 2. IS Monthly / Cash Flow Monthly (CRL00001) - 3 fixed columns
        $isMonthly = ReportColHead::create([
            'name' => 'Standard Income Columns',
            'col_type' => 'fixed',
        ]);
        $this->createDetails($isMonthly->id, [
            ['sort_order' => 1, 'col_title' => 'This Month', 'content' => 'Current', 'period' => 'Curr'],
            ['sort_order' => 2, 'col_title' => 'Last Month', 'content' => 'Current', 'period' => 'Curr - 01'],
            ['sort_order' => 5, 'col_title' => 'Year To Date', 'content' => 'Current', 'period' => 'YTD'],
        ]);

        // 3. YTD (BS & IS) - 12 month columns, dynamically sliced
        $ytd = ReportColHead::create([
            'name' => 'Year To Date Per Bulan',
            'col_type' => 'ytd',
        ]);
        $months = ['January', 'February', 'March', 'April', 'May', 'June',
                    'July', 'August', 'September', 'October', 'November', 'December'];
        foreach ($months as $i => $month) {
            $period = str_pad($i + 1, 2, '0', STR_PAD_LEFT);
            ReportColDetail::create([
                'report_col_head_id' => $ytd->id,
                'sort_order' => $i + 1,
                'col_title' => $month,
                'content' => 'Current',
                'period' => $period,
                'period_key' => $i + 1,
            ]);
        }

        // 4. BS Quarterly - 16 columns (12 individual months + 4 quarter totals), dynamically sliced
        $bsQtr = ReportColHead::create([
            'name' => 'Neraca Triwulan',
            'col_type' => 'bs_quarterly',
        ]);
        $this->createDetails($bsQtr->id, [
            // Q1 individual months + total
            ['sort_order' => 1, 'col_title' => 'January', 'content' => 'Current', 'period' => '01', 'period_key' => 3],
            ['sort_order' => 2, 'col_title' => 'February', 'content' => 'Current', 'period' => '02', 'period_key' => 3],
            ['sort_order' => 3, 'col_title' => 'March', 'content' => 'Current', 'period' => '03', 'period_key' => 3],
            ['sort_order' => 4, 'col_title' => 'Quarter 1', 'content' => 'Current', 'period' => '01,02,03', 'period_key' => 3],
            // Q2 individual months + total
            ['sort_order' => 5, 'col_title' => 'April', 'content' => 'Current', 'period' => '04', 'period_key' => 6],
            ['sort_order' => 6, 'col_title' => 'May', 'content' => 'Current', 'period' => '05', 'period_key' => 6],
            ['sort_order' => 7, 'col_title' => 'June', 'content' => 'Current', 'period' => '06', 'period_key' => 6],
            ['sort_order' => 8, 'col_title' => 'Quarter 2', 'content' => 'Current', 'period' => '04,05,06', 'period_key' => 6],
            // Q3 individual months + total
            ['sort_order' => 9, 'col_title' => 'July', 'content' => 'Current', 'period' => '07', 'period_key' => 9],
            ['sort_order' => 10, 'col_title' => 'August', 'content' => 'Current', 'period' => '08', 'period_key' => 9],
            ['sort_order' => 11, 'col_title' => 'September', 'content' => 'Current', 'period' => '09', 'period_key' => 9],
            ['sort_order' => 12, 'col_title' => 'Quarter 3', 'content' => 'Current', 'period' => '07,08,09', 'period_key' => 9],
            // Q4 individual months + total
            ['sort_order' => 13, 'col_title' => 'October', 'content' => 'Current', 'period' => '10', 'period_key' => 12],
            ['sort_order' => 14, 'col_title' => 'November', 'content' => 'Current', 'period' => '11', 'period_key' => 12],
            ['sort_order' => 15, 'col_title' => 'December', 'content' => 'Current', 'period' => '12', 'period_key' => 12],
            ['sort_order' => 16, 'col_title' => 'Quarter 4', 'content' => 'Current', 'period' => '10,11,12', 'period_key' => 12],
        ]);

        // 5. IS Quarterly - 17 columns (12 individual months + 4 quarter totals + YTD), dynamically sliced
        $isQtr = ReportColHead::create([
            'name' => 'Income Triwulan',
            'col_type' => 'is_quarterly',
        ]);
        $this->createDetails($isQtr->id, [
            // Q1 individual months + total
            ['sort_order' => 1, 'col_title' => 'January', 'content' => 'Current', 'period' => '01', 'period_key' => 3],
            ['sort_order' => 2, 'col_title' => 'February', 'content' => 'Current', 'period' => '02', 'period_key' => 3],
            ['sort_order' => 3, 'col_title' => 'March', 'content' => 'Current', 'period' => '03', 'period_key' => 3],
            ['sort_order' => 4, 'col_title' => 'Quarter 1', 'content' => 'Current', 'period' => '01,02,03', 'period_key' => 3],
            // Q2 individual months + total
            ['sort_order' => 5, 'col_title' => 'April', 'content' => 'Current', 'period' => '04', 'period_key' => 6],
            ['sort_order' => 6, 'col_title' => 'May', 'content' => 'Current', 'period' => '05', 'period_key' => 6],
            ['sort_order' => 7, 'col_title' => 'June', 'content' => 'Current', 'period' => '06', 'period_key' => 6],
            ['sort_order' => 8, 'col_title' => 'Quarter 2', 'content' => 'Current', 'period' => '04,05,06', 'period_key' => 6],
            // Q3 individual months + total
            ['sort_order' => 9, 'col_title' => 'July', 'content' => 'Current', 'period' => '07', 'period_key' => 9],
            ['sort_order' => 10, 'col_title' => 'August', 'content' => 'Current', 'period' => '08', 'period_key' => 9],
            ['sort_order' => 11, 'col_title' => 'September', 'content' => 'Current', 'period' => '09', 'period_key' => 9],
            ['sort_order' => 12, 'col_title' => 'Quarter 3', 'content' => 'Current', 'period' => '07,08,09', 'period_key' => 9],
            // Q4 individual months + total
            ['sort_order' => 13, 'col_title' => 'October', 'content' => 'Current', 'period' => '10', 'period_key' => 12],
            ['sort_order' => 14, 'col_title' => 'November', 'content' => 'Current', 'period' => '11', 'period_key' => 12],
            ['sort_order' => 15, 'col_title' => 'December', 'content' => 'Current', 'period' => '12', 'period_key' => 12],
            ['sort_order' => 16, 'col_title' => 'Quarter 4', 'content' => 'Current', 'period' => '10,11,12', 'period_key' => 12],
            // YTD
            ['sort_order' => 17, 'col_title' => 'Year To Date', 'content' => 'Current', 'period' => 'YTD', 'period_key' => null],
        ]);

        // 6. IS Var-Budget (CRL00004) - 6 fixed columns
        $varBudget = ReportColHead::create([
            'name' => 'Income Var-Budget',
            'col_type' => 'fixed',
        ]);
        $this->createDetails($varBudget->id, [
            ['sort_order' => 1, 'col_title' => 'Actual This Month', 'content' => 'Current', 'period' => 'Curr'],
            ['sort_order' => 2, 'col_title' => 'Budget This Month', 'content' => 'Budget', 'period' => 'Curr'],
            ['sort_order' => 5, 'col_title' => 'Variance', 'content' => 'Formula', 'period' => '{0001} - {0002}'],
            ['sort_order' => 10, 'col_title' => 'YTD Actual', 'content' => 'Current', 'period' => 'YTD'],
            ['sort_order' => 11, 'col_title' => 'YTD Budget', 'content' => 'Budget', 'period' => 'YTD'],
            ['sort_order' => 15, 'col_title' => 'Variance', 'content' => 'Formula', 'period' => '{0010} - {0011}'],
        ]);

        // 7. IS % Revenue (CRL00005) - 6 fixed columns
        $pctRevenue = ReportColHead::create([
            'name' => 'Income (%) Total Revenue',
            'col_type' => 'fixed',
        ]);
        $this->createDetails($pctRevenue->id, [
            ['sort_order' => 1, 'col_title' => 'Current Month', 'content' => 'Current', 'period' => 'Curr'],
            ['sort_order' => 2, 'col_title' => '(%) Total Revenue', 'content' => 'Base%', 'period' => '{0001}'],
            ['sort_order' => 10, 'col_title' => 'Last Month', 'content' => 'Current', 'period' => 'Curr - 01'],
            ['sort_order' => 12, 'col_title' => '(%) Total Revenue', 'content' => 'Base%', 'period' => '{0010}'],
            ['sort_order' => 20, 'col_title' => 'Year To Date', 'content' => 'Current', 'period' => 'YTD'],
            ['sort_order' => 22, 'col_title' => '(%) Total Revenue', 'content' => 'Base%', 'period' => '{0020}'],
        ]);

        // Link report_finals in FINRPT order.
        $mapping = [
            1 => $bsMonthly->id,    // Balance Sheet - Monthly
            2 => $ytd->id,          // Balance Sheet - Year To Date
            3 => $bsQtr->id,        // Balance Sheet - Quarterly
            4 => $isMonthly->id,    // Income Statement - Monthly
            5 => $varBudget->id,    // Income Statement - Act >< Bdt
            6 => $pctRevenue->id,   // Income (%) On Total Revenue
            7 => $isQtr->id,        // Income Statement - Quarterly
            8 => $ytd->id,          // Income Statement - Year To Date
            9 => $isMonthly->id,    // Cash Flow Statement - Monthly
        ];

        foreach ($mapping as $finalId => $colHeadId) {
            ReportFinal::where('id', $finalId)->update(['report_col_head_id' => $colHeadId]);
        }
    }

    private function createDetails(int $headId, array $details): void
    {
        foreach ($details as $detail) {
            ReportColDetail::create(array_merge($detail, [
                'report_col_head_id' => $headId,
                'division' => $detail['division'] ?? '*',
                'col_format' => $detail['col_format'] ?? '#,##0.00',
            ]));
        }
    }
}
