<?php

namespace App\Livewire\Pages\Report\GeneralLedger;

use App\Exports\LedgerExport;
use App\Exports\SummaryLedgerExport;
use App\Models\ChartOfAccount;
use App\Models\Division;
use App\Models\MstSaldoNew;
use App\Models\Parameter;
use App\Models\Period;
use Illuminate\Support\Facades\DB;
use Livewire\Component;
use Maatwebsite\Excel\Facades\Excel;

ini_set('max_execution_time', 0);
ini_set('memory_limit', '-1');

class ReportGeneralLedger extends Component
{
    public $acc;

    public $acc_code;

    public $acc_code2;

    public $period;

    public $periodeFrom;

    public $periodeTo;

    public $tahun;

    public $year;

    public $codedivfrom;

    public $codedivto;

    public $listDivision;

    public $report;

    public $this_period = false;

    public $reportData = [];

    public $reportMode = null;

    public $showReport = false;

    public function updated($property, $value)
    {
        // Reset report when any filter changes
        if (in_array($property, ['year', 'acc_code', 'acc_code2', 'periodeFrom', 'periodeTo', 'codedivfrom', 'codedivto', 'report', 'this_period'])) {
            $this->showReport = false;
            $this->reportData = [];
            $this->reportMode = null;
        }
    }

    public function updatedThisPeriod()
    {
        $isDisabled = $this->this_period ? 'true' : 'false';
        $this->js(<<<JS
            $(document).ready(function() {
                var vale = $isDisabled;
                $('.periodeFrom').prop('disabled', vale);
                $('.periodeTo').prop('disabled', vale);
                if (vale) {
                    $('.periodeFrom').val('').trigger('change');
                    $('.periodeTo').val('').trigger('change');
                }
            });
        JS);
    }

    public function viewReport()
    {
        $this->validate([
            'acc_code' => 'required',
            'year' => 'required',
            'report' => 'required',
        ]);

        if (! $this->this_period && (empty($this->periodeFrom) || empty($this->periodeTo))) {
            $this->addError('periodeFrom', 'Periode harus diisi jika bukan Report For This Period.');

            return;
        }

        if ($this->report === 'generalledger') {
            $this->reportMode = 'detail';
            $this->loadDetailReport();
        } elseif ($this->report === 'summarygeneralledger') {
            $this->reportMode = 'summary';
            $this->loadSummaryReport();
        }

        $this->showReport = true;
    }

    private function loadDetailReport()
    {
        $acc_code2 = $this->acc_code2 ?: $this->acc_code;
        $account = DB::table('mst_account')
            ->whereRaw('acc_code BETWEEN ? AND ?', [$this->acc_code, $acc_code2])
            ->get();

        $tahun = $this->year;
        $periodfrom = $this->periodeFrom;
        $periodto = $this->periodeTo;
        $codedivfrom = $this->codedivfrom;
        $codedivto = $this->codedivto;
        $this_period = $this->this_period;

        if ($this_period) {
            $param = Parameter::first();
            if ($param && empty($periodto)) {
                $periodto = $param->period;
            }
        }

        $data = [];
        foreach ($account as $val) {
            $bb = getBeginningBalance($tahun, $val->acc_code, $periodfrom, $codedivfrom, $codedivto);
            $details = getLedgerDetails($tahun, $val->acc_code, $periodfrom, $periodto, $codedivfrom, $codedivto, $this_period);

            $totalDebet = 0;
            $totalCredit = 0;
            $rows = [];

            foreach ($details as $detail) {
                $totalDebet += $detail->debet;
                $totalCredit += $detail->credit;
                $rows[] = [
                    'period' => $detail->period ?? null,
                    'trans_date' => $detail->trans_date,
                    'trans_code' => $detail->trans_code,
                    'trans_no' => $detail->trans_no,
                    'div_code' => $detail->div_code,
                    'description' => $detail->description,
                    'debet' => (float) $detail->debet,
                    'credit' => (float) $detail->credit,
                ];
            }

            $data[] = [
                'acc_code' => $val->acc_code,
                'acc_name' => $val->acc_name,
                'bb_debet' => $bb->total_debet,
                'bb_credit' => $bb->total_credit,
                'details' => $rows,
                'total_debet' => $totalDebet,
                'total_credit' => $totalCredit,
                'ending_debet' => $bb->total_debet + $totalDebet,
                'ending_credit' => $bb->total_credit + $totalCredit,
            ];
        }

        $this->reportData = $data;
    }

    private function loadSummaryReport()
    {
        $acc_code2 = $this->acc_code2 ?: $this->acc_code;
        $tahun = (int) $this->year;
        $periodfrom = $this->periodeFrom;
        $periodto = $this->periodeTo;

        $detailTable = $this->this_period ? 'trans_detail' : 'this_detail';
        $headTable = $this->this_period ? 'trans_head' : 'this_head';

        $paramPeriod = optional(Parameter::first())->period;
        $periodCutoff = $periodfrom ?: ($paramPeriod ?: 1);

        [$dateFrom, $dateTo] = getDateRangeFromPeriods($periodfrom, $periodto);

        // Build beginning/ending balance subqueries with division filter
        [$begSql, $begBindings] = $this->buildSaldoSubquery('sb', $tahun, $periodCutoff);
        [$endSql, $endBindings] = $this->buildSaldoSubquery('se', $tahun, $periodCutoff);

        $query = DB::table('mst_account as a')
            ->leftJoin("{$detailTable} as d", 'd.acc_code', '=', 'a.acc_code')
            ->leftJoin("{$headTable} as h", function ($join) use ($detailTable) {
                $join->on('d.trans_code', '=', 'h.trans_code')
                    ->on('d.trans_no', '=', 'h.trans_no');
                if ($detailTable === 'this_detail') {
                    $join->on('d.period', '=', 'h.period')
                        ->whereRaw('d.trans_date::date = h.trans_date::date');
                }
            })
            ->whereBetween('a.acc_code', [$this->acc_code, $acc_code2]);

        // Filter divisi on detail rows
        if (! empty($this->codedivfrom) && ! empty($this->codedivto)) {
            $query->whereBetween('d.div_code', [$this->codedivfrom, $this->codedivto]);
        } elseif (! empty($this->codedivfrom)) {
            $query->where('d.div_code', $this->codedivfrom);
        }

        // Filter periode/tanggal
        if ($detailTable === 'this_detail') {
            if (! empty($periodfrom) && ! empty($periodto)) {
                $query->whereBetween(DB::raw('CAST(d.period AS INTEGER)'), [(int) $periodfrom, (int) $periodto]);
            }
            $query->whereYear('d.trans_date', $tahun);
        } else {
            if ($dateFrom && $dateTo) {
                $query->whereBetween('h.trans_date', [$dateFrom->toDateTimeString(), $dateTo->toDateTimeString()]);
            } elseif (! empty($tahun)) {
                $query->whereYear('h.trans_date', $tahun);
            }
        }

        $results = $query
            ->select(['a.acc_code', 'a.acc_name'])
            ->selectRaw("({$begSql}) AS beg_balance", $begBindings)
            ->selectRaw('COALESCE(SUM(d.debet),0) AS debet')
            ->selectRaw('COALESCE(SUM(d.credit),0) AS credit')
            ->selectRaw("(({$endSql}) + COALESCE(SUM(d.debet),0) - COALESCE(SUM(d.credit),0)) AS end_balance", $endBindings)
            ->groupBy('a.acc_code', 'a.acc_name')
            ->orderBy('a.acc_code')
            ->get();

        $this->reportData = $results->map(fn ($r) => [
            'acc_code' => $r->acc_code,
            'acc_name' => $r->acc_name,
            'beg_balance' => (float) ($r->beg_balance ?? 0),
            'debet' => (float) ($r->debet ?? 0),
            'credit' => (float) ($r->credit ?? 0),
            'end_balance' => (float) ($r->end_balance ?? 0),
        ])->toArray();
    }

    private function buildSaldoSubquery(string $alias, int $tahun, $periodCutoff): array
    {
        $sql = "SELECT COALESCE(SUM({$alias}.debet - {$alias}.credit),0) FROM mst_saldo_news {$alias} WHERE {$alias}.acc_code = a.acc_code AND {$alias}.tahun = ? AND {$alias}.periode < ?";
        $bindings = [$tahun, (int) $periodCutoff];

        if (! empty($this->codedivfrom) && ! empty($this->codedivto)) {
            $sql .= " AND {$alias}.div_code BETWEEN ? AND ?";
            $bindings[] = $this->codedivfrom;
            $bindings[] = $this->codedivto;
        } elseif (! empty($this->codedivfrom)) {
            $sql .= " AND {$alias}.div_code = ?";
            $bindings[] = $this->codedivfrom;
        }

        return [$sql, $bindings];
    }

    public function mount()
    {
        $this->tahun = MstSaldoNew::selectRaw('tahun')
            ->groupBy(DB::raw('tahun'))
            ->orderBy('tahun', 'DESC')
            ->get();
        $this->acc = ChartOfAccount::orderBy('acc_code', 'ASC')->get();

        $period = Period::where('period_code', 1)->first() ?? Period::first();
        $this->period = $period?->periods ?? [];

        $this->listDivision = Division::all();

        $this->js(<<<'JS'
            $(document).ready(function() {
                $('.year').select2();
                $('.year').on('change', function (e) {
                    var data = $('.year').select2("val");
                    $wire.set('year', data);
                });

                $('.periodeFrom').select2();
                $('.periodeFrom').on('change', function (e) {
                    var data = $('.periodeFrom').select2("val");
                    $wire.set('periodeFrom', data);
                });

                $('.periodeTo').select2();
                $('.periodeTo').on('change', function (e) {
                    var data = $('.periodeTo').select2("val");
                    $wire.set('periodeTo', data);
                });

                $('.acc_code').select2();
                $('.acc_code').on('change', function (e) {
                    var data = $('.acc_code').select2("val");
                    $wire.set('acc_code', data);
                });

                $('.acc_code2').select2();
                $('.acc_code2').on('change', function (e) {
                    var data = $('.acc_code2').select2("val");
                    $wire.set('acc_code2', data);
                });

                $('.codedivfrom').select2();
                $('.codedivfrom').on('change', function (e) {
                    var data = $('.codedivfrom').select2("val");
                    $wire.set('codedivfrom', data);
                });

                $('.codedivto').select2();
                $('.codedivto').on('change', function (e) {
                    var data = $('.codedivto').select2("val");
                    $wire.set('codedivto', data);
                });

                $('.report').select2();
                $('.report').on('change', function (e) {
                    var data = $('.report').select2("val");
                    $wire.set('report', data);
                });
            });
        JS);
    }

    public function batal()
    {
        return redirect()->route('reportgeneralledger');
    }

    public function sendReport()
    {
        $acc_code = $this->acc_code;
        $acc_code2 = $this->acc_code2;
        $periodeFrom = $this->periodeFrom;
        $periodeTo = $this->periodeTo;
        $codedivfrom = $this->codedivfrom;
        $codedivto = $this->codedivto;
        $year = $this->year;
        $this_period = $this->this_period;

        $this->validate([
            'acc_code' => 'required',
            'year' => 'required',
            'report' => 'required',
        ]);

        if ($this->report == 'generalledger') {
            return Excel::download(new LedgerExport($acc_code, $acc_code2, $periodeFrom, $periodeTo, $codedivfrom, $codedivto, $year, $this_period), 'ledger_details-'.time().'.xlsx');
        }

        if ($this->report == 'summarygeneralledger') {
            return Excel::download(
                new SummaryLedgerExport($acc_code, $acc_code2, $periodeFrom, $periodeTo, $codedivfrom, $codedivto, $year, $this_period),
                'SummaryLedger-'.time().'.xlsx'
            );
        }
    }

    public function render()
    {
        return view('livewire.pages.report.general-ledger.report-general-ledger');
    }
}
