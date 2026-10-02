<?php

use App\Models\Parameter;
use Carbon\Carbon;
use Illuminate\Support\Facades\DB;

function getBeginningBalance($tahun, $acc_code, $period = null, $divFrom = null, $divTo = null)
{
    // Normalisasi period: ambil dari parameter jika kosong
    // mst_saldo_news.periode = integer 1-13 (bukan yyyymm)
    if (empty($period)) {
        $param = Parameter::first();
        $period = ! empty($param?->period) ? (int) $param->period : 1;
    }

    $q = DB::table('mst_saldo_news')
        ->where('acc_code', $acc_code)
        ->where('tahun', $tahun)
        ->where('periode', '<', (int) $period);

    // Filter divisi jika ada
    if (! empty($divFrom) && ! empty($divTo)) {
        $q->whereBetween('div_code', [$divFrom, $divTo]);
    } elseif (! empty($divFrom)) {
        $q->where('div_code', $divFrom);
    }

    $row = $q->selectRaw('COALESCE(SUM(debet),0) AS total_debet, COALESCE(SUM(credit),0) AS total_credit')
        ->first();

    // Pastikan selalu ada properti numerik
    return (object) [
        'total_debet' => (float) ($row->total_debet ?? 0),
        'total_credit' => (float) ($row->total_credit ?? 0),
    ];
}

function getLedgerDetails($tahun, $acc_code, $periodfrom, $periodto, $codedivfrom, $codedivto, $this_period)
{
    $detailTable = $this_period ? 'trans_detail' : 'this_detail';
    $headTable = $this_period ? 'trans_head' : 'this_head';

    $q = DB::table($detailTable)
        ->join($headTable, function ($join) use ($detailTable, $headTable) {
            $join->on("{$detailTable}.trans_code", '=', "{$headTable}.trans_code")
                ->on("{$detailTable}.trans_no", '=', "{$headTable}.trans_no");
            if ($detailTable === 'this_detail') {
                $join->on("{$detailTable}.period", '=', "{$headTable}.period")
                    ->whereRaw("{$detailTable}.trans_date::date = {$headTable}.trans_date::date");
            }
        })
        ->leftJoin('mst_division', "{$detailTable}.div_code", '=', 'mst_division.div_code')
        ->select(
            "{$headTable}.trans_date",
            "{$headTable}.trans_update",
            "{$headTable}.trans_no",
            "{$headTable}.trans_user",
            "{$detailTable}.trans_code",
            "{$detailTable}.div_code",
            "{$detailTable}.acc_code",
            "{$detailTable}.description",
            "{$detailTable}.debet",
            "{$detailTable}.credit",
            "{$detailTable}.curr_code",
            "{$detailTable}.curr_rate",
            "{$detailTable}.curr_debet",
            "{$detailTable}.curr_credit",
            'mst_division.div_name'
        );

    if ($detailTable === 'this_detail') {
        $q->addSelect("{$detailTable}.period");
    }

    // Filter akun
    if (! empty($acc_code)) {
        $q->where("{$detailTable}.acc_code", $acc_code);
    }

    // Filter divisi (opsional)
    if (! empty($codedivfrom) && ! empty($codedivto)) {
        $q->whereBetween("{$detailTable}.div_code", [$codedivfrom, $codedivto]);
    } elseif (! empty($codedivfrom)) {
        $q->where("{$detailTable}.div_code", $codedivfrom);
    }

    if ($detailTable === 'this_detail') {
        // Mode this_* → filter berdasarkan PERIOD
        if (! empty($periodfrom) && ! empty($periodto)) {
            $q->whereBetween("{$detailTable}.period", [$periodfrom, $periodto]);
        }
        if (! empty($tahun)) {
            $q->whereYear("{$detailTable}.trans_date", $tahun);
        }
    } else {
        // Mode trans_* → filter berdasarkan TANGGAL HEAD
        // NEW: jika periodfrom/periodto tersedia (yyyymm), konversi ke rentang tanggal
        [$dateFrom, $dateTo] = getDateRangeFromPeriods($periodfrom, $periodto);

        if ($dateFrom && $dateTo) {
            // Utamakan rentang tanggal lengkap
            $q->whereBetween("{$headTable}.trans_date", [$dateFrom->toDateTimeString(), $dateTo->toDateTimeString()]);
        } elseif (! empty($tahun)) {
            // Fallback: hanya tahun
            $q->whereYear("{$headTable}.trans_date", $tahun);
        }
    }

    return $q->orderBy("{$headTable}.trans_date", 'asc')->get();
}

function getDateRangeFromPeriods(?string $periodFrom, ?string $periodTo, string $tz = 'Asia/Jakarta'): array
{
    if (empty($periodFrom) || empty($periodTo)) {
        return [null, null];
    }
    if (! preg_match('/^\d{6}$/', $periodFrom) || ! preg_match('/^\d{6}$/', $periodTo)) {
        return [null, null];
    }

    $y1 = (int) substr($periodFrom, 0, 4);
    $m1 = (int) substr($periodFrom, 4, 2);
    $y2 = (int) substr($periodTo, 0, 4);
    $m2 = (int) substr($periodTo, 4, 2);

    $start = Carbon::create($y1, $m1, 1, 0, 0, 0, $tz)->startOfDay();
    $end = Carbon::create($y2, $m2, 1, 0, 0, 0, $tz)->endOfMonth()->endOfDay();
    if ($start->gt($end)) {
        [$start, $end] = [$end->copy()->startOfMonth()->startOfDay(), $start->copy()->endOfMonth()->endOfDay()];
    }

    return [$start, $end];
}
