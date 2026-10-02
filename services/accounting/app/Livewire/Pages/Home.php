<?php

namespace App\Livewire\Pages;

use App\Models\ChartOfAccount;
use App\Models\Division;
use App\Models\Parameter;
use App\Models\Period;
use App\Models\ThisHead;
use Carbon\Carbon;
use Illuminate\Support\Facades\DB;
use Livewire\Component;

class Home extends Component
{
    public $trendYearFilter;
    protected $queryString = ['trendYearFilter'];

    public function render()
    {
        $parameter = Parameter::first();
        $currentYear = (int) ($parameter?->year ?? now()->year);
        $activePeriod = (int) ($parameter?->period ?? 0);

        $availableTrendYears = DB::table('this_head')
            ->selectRaw('DISTINCT EXTRACT(YEAR FROM trans_date)::int AS year')
            ->whereNotNull('trans_date')
            ->orderBy('year')
            ->pluck('year')
            ->map(fn($y) => (int) $y)
            ->values()
            ->toArray();

        if (!in_array($currentYear, $availableTrendYears, true)) {
            $availableTrendYears[] = $currentYear;
            sort($availableTrendYears);
        }

        if (!$this->trendYearFilter) {
            $trendYear = $currentYear;
            $this->trendYearFilter = $trendYear;
        } else {
            $trendYear = (int) $this->trendYearFilter;
        }

        if (!in_array($trendYear, $availableTrendYears, true)) {
            $availableTrendYears[] = $trendYear;
            sort($availableTrendYears);
        }

        $trendYearHasData = ThisHead::whereYear('trans_date', $trendYear)->exists();
        $isTrendYearFallback = false;

        $totalYearTransactions = (int) ThisHead::whereYear('trans_date', $trendYear)->count();
        $totalTransactionsActive = (int) ThisHead::count();

        $periodName = null;
        if ($parameter) {
            $period = Period::where('period_name', 'like', '%' . $parameter->year . '%')->first();
            if ($period) {
                $periodName = $period->{'p' . $parameter->period};
            }
        }

        $monthlyTrendRaw = ThisHead::query()
            ->selectRaw('EXTRACT(MONTH FROM trans_date)::int AS month_no')
            ->selectRaw('COUNT(*) AS tx_count')
            ->selectRaw('COALESCE(SUM(debet), 0) AS total_debet')
            ->selectRaw('COALESCE(SUM(credit), 0) AS total_credit')
            ->selectRaw('COALESCE(SUM(credit) - SUM(debet), 0) AS net_movement')
            ->whereYear('trans_date', $trendYear)
            ->groupBy('month_no')
            ->orderBy('month_no')
            ->get();

        $monthlyNominalTrendRaw = DB::table('this_detail as d')
            ->join('mst_account as a', 'd.acc_code', '=', 'a.acc_code')
            ->selectRaw('EXTRACT(MONTH FROM d.trans_date)::int AS month_no')
            ->selectRaw('COALESCE(SUM(d.credit) - SUM(d.debet), 0) AS nominal_net_movement')
            ->whereYear('d.trans_date', $trendYear)
            ->whereIn('a.acc_group', [4, 5, 6, 7, 8])
            ->groupBy('month_no')
            ->orderBy('month_no')
            ->get();

        $monthlyTrend = [];
        $maxTx = 1;
        for ($month = 1; $month <= 12; $month++) {
            $monthRecord = $monthlyTrendRaw->firstWhere('month_no', $month);
            $nominalRecord = $monthlyNominalTrendRaw->firstWhere('month_no', $month);
            $item = [
                'month_no' => $month,
                'month_label' => Carbon::create(2000, $month, 1)->format('M'),
                'tx_count' => (int) ($monthRecord?->tx_count ?? 0),
                'total_debet' => (float) ($monthRecord?->total_debet ?? 0),
                'total_credit' => (float) ($monthRecord?->total_credit ?? 0),
                'net_movement' => (float) ($monthRecord?->net_movement ?? 0),
                'nominal_net_movement' => (float) ($nominalRecord?->nominal_net_movement ?? 0),
            ];
            $monthlyTrend[] = $item;
            if ($item['tx_count'] > $maxTx) {
                $maxTx = $item['tx_count'];
            }
        }

        $topTransactionTypes = ThisHead::query()
            ->selectRaw('trans_code, COUNT(*) AS tx_count')
            ->whereYear('trans_date', $trendYear)
            ->groupBy('trans_code')
            ->orderByDesc('tx_count')
            ->orderBy('trans_code')
            ->limit(5)
            ->get();

        $activeMonth = $activePeriod <= 12 && $activePeriod > 0 ? $activePeriod : max(1, min(12, $activePeriod - 1));
        $trendCurrentMonth = $monthlyTrend[$activeMonth - 1] ?? null;
        $trendPrevMonth = $activeMonth > 1 ? $monthlyTrend[$activeMonth - 2] : null;

        $yearlyDebit = (float) ThisHead::query()
            ->whereYear('trans_date', $trendYear)
            ->sum('debet');

        $yearlyCredit = (float) ThisHead::query()
            ->whereYear('trans_date', $trendYear)
            ->sum('credit');

        $yearlyNet = $yearlyCredit - $yearlyDebit;

        $yearlyNominalNet = (float) DB::table('this_detail as d')
            ->join('mst_account as a', 'd.acc_code', '=', 'a.acc_code')
            ->whereYear('d.trans_date', $trendYear)
            ->whereIn('a.acc_group', [4, 5, 6, 7, 8])
            ->selectRaw('COALESCE(SUM(d.credit) - SUM(d.debet), 0) as nominal_net')
            ->value('nominal_net');

        $periodClosedCount = DB::table('this_head')
            ->whereRaw('EXTRACT(YEAR FROM trans_date) = ?', [$trendYear])
            ->distinct('period')
            ->count('period');

        $diversity = DB::table('trans_detail')
            ->selectRaw("COUNT(DISTINCT acc_code) AS unique_accounts, COUNT(DISTINCT div_code) AS unique_divisions")
            ->whereRaw('EXTRACT(YEAR FROM trans_date) = ?', [$trendYear])
            ->first();

        return view('livewire.pages.home', [
            'parameter' => $parameter,
            'periodName' => $periodName,
            'totalAccounts' => ChartOfAccount::count(),
            'totalTransactions' => $totalTransactionsActive,
            'totalDivisions' => Division::count(),
            'userName' => auth()->user()->user_name ?? 'User',
            'companyName' => $parameter?->company ?? 'Company',
            'currentYear' => $currentYear,
            'trendYear' => $trendYear,
            'trendYearHasData' => $trendYearHasData,
            'trendYearLabel' => $isTrendYearFallback ? "Data available fallback: {$trendYear}" : null,
            'isTrendYearFallback' => $isTrendYearFallback,
            'trendYearOptions' => $availableTrendYears,
            'trendYearFilter' => $trendYear,
            'monthlyTrend' => $monthlyTrend,
            'trendMaxTx' => $maxTx,
            'topTransactionTypes' => $topTransactionTypes,
            'totalYearTransactions' => $totalYearTransactions,
            'activeMonthLabel' => $trendCurrentMonth['month_label'] ?? '-',
            'activeMonthTx' => $trendCurrentMonth['tx_count'] ?? 0,
            'activeMonthNet' => $trendCurrentMonth['net_movement'] ?? 0,
            'activeMonthNominalNet' => $trendCurrentMonth['nominal_net_movement'] ?? 0,
            'prevMonthTx' => $trendPrevMonth['tx_count'] ?? 0,
            'yearlyDebit' => $yearlyDebit,
            'yearlyCredit' => $yearlyCredit,
            'yearlyNet' => $yearlyNet,
            'yearlyNominalNet' => $yearlyNominalNet,
            'periodClosedCount' => $periodClosedCount,
            'uniqueAccountsUsed' => (int) ($diversity->unique_accounts ?? 0),
            'uniqueDivisionsUsed' => (int) ($diversity->unique_divisions ?? 0),
        ]);
    }
}
