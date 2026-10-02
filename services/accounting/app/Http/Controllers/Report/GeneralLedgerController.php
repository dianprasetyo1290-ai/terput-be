<?php

namespace App\Http\Controllers\Report;

use App\Http\Controllers\Controller;
use App\Models\Parameter;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\DB;

class GeneralLedgerController extends Controller
{
    public function report(Request $request)
    {
        $acc_code = $request->acc_code;
        $acc_code2 = $request->acc_code2;
        $periodfrom = $request->periodfrom;
        $periodto = $request->periodto;
        $codedivfrom = $request->codedivfrom;
        $codedivto = $request->codedivto;
        $tahun = $request->tahun;

        // Normalisasi boolean this_period (true = pakai trans_*, false = pakai this_*)
        $this_period = filter_var($request->input('this_period', false), FILTER_VALIDATE_BOOLEAN);

        if ($this_period) {
            // Jika “periode berjalan”, pakai period aktif dari parameter untuk batas period this_*
            $param = Parameter::first();
            if ($param && empty($periodto)) {
                $periodto = $param->period; // mis. '202508' → sesuaikan format di DB Anda
            }
        }

        // Ambil daftar akun (range)
        $account = DB::table('mst_account')
            ->whereRaw('acc_code BETWEEN ? AND ?', [$acc_code, $acc_code2])
            ->get();

        return view('report.generalledger', compact(
            'account',
            'tahun',
            'periodfrom',
            'periodto',
            'codedivfrom',
            'codedivto',
            'this_period'
        ));
    }

    public function reportsummary(Request $request)
    {
        $acc_code = $request->acc_code;
        $acc_code2 = $request->acc_code2;
        $periodfrom = $request->periodfrom;     // "yyyymm"
        $periodto = $request->periodto;       // "yyyymm"
        $tahun = (int) $request->tahun;
        $this_period = filter_var($request->this_period, FILTER_VALIDATE_BOOLEAN);

        // Tentukan tabel
        $detailTable = $this_period ? 'trans_detail' : 'this_detail';
        $headTable = $this_period ? 'trans_head' : 'this_head';

        // Cutoff untuk beginning balance (periode sebelumnya)
        // mst_saldo_news.periode = integer 1-13, bukan yyyymm
        $paramPeriod = optional(Parameter::first())->period;         // bisa null
        $periodCutoff = $periodfrom ?: ($paramPeriod ?: 1);

        // Jika trans_* (pakai tanggal), siapkan rentang tanggal dari yyyymm
        [$dateFrom, $dateTo] = getDateRangeFromPeriods($periodfrom, $periodto);

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
            ->whereBetween('a.acc_code', [$acc_code, $acc_code2]);

        // Filter periode/tanggal
        if ($detailTable === 'this_detail') {
            if (! empty($periodfrom) && ! empty($periodto)) {
                // CAST period ke integer agar pasti numerik di Postgres
                $query->whereBetween(DB::raw('CAST(d.period AS INTEGER)'), [(int) $periodfrom, (int) $periodto]);
            }
            $query->whereYear('d.trans_date', $tahun);
        } else {
            // trans_* → filter berdasarkan tanggal head
            if ($dateFrom && $dateTo) {
                $query->whereBetween('h.trans_date', [$dateFrom->toDateTimeString(), $dateTo->toDateTimeString()]);
            } elseif (! empty($tahun)) {
                $query->whereYear('h.trans_date', $tahun);
            }
        }

        // SELECT kolom ringkasan
        $results = $query
            ->select([
                'a.acc_code',
                'a.acc_name',
                // Beginning balance (subquery aman dengan binding)
                DB::raw('(SELECT COALESCE(SUM(s.debet - s.credit),0)
                      FROM mst_saldo_news s
                      WHERE s.acc_code = a.acc_code
                        AND s.tahun    = ?
                        AND s.periode  < ?) as beg_balance'),
                DB::raw('COALESCE(SUM(d.debet),0)  as debet'),
                DB::raw('COALESCE(SUM(d.credit),0) as credit'),
                // End balance = beg + debet - credit (ulang ekspresi beg agar portable)
                DB::raw('( (SELECT COALESCE(SUM(s2.debet - s2.credit),0)
                        FROM mst_saldo_news s2
                        WHERE s2.acc_code = a.acc_code
                          AND s2.tahun    = ?
                          AND s2.periode  < ?)
                       + COALESCE(SUM(d.debet),0)
                       - COALESCE(SUM(d.credit),0)
                     ) as end_balance'),
            ])
            ->addBinding([$tahun, $periodCutoff, $tahun, $periodCutoff], 'select') // binding untuk kedua subquery
            ->groupBy('a.acc_code', 'a.acc_name')
            ->orderBy('a.acc_code')
            ->get();

        return view('report.summarygeneralledger', compact('results'));
    }
}
