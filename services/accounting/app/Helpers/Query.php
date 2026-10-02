<?php

use App\Models\MstSaldoNew;
use App\Models\TransDetail;
// use DB;
function getTotalValue($account, $periode, $tahun, $normalBalance, $divisi = null)
{
    $arrAccount = explode(",", $account);
    $a = MstSaldoNew::select(DB::Raw("
    sum(debet) as total_debet, sum(credit) as total_credit
    "))
        ->where(function ($query) use ($arrAccount) {
            foreach ($arrAccount as $value) {
                $query->orWhere('acc_code', 'ilike', $value);
            }
        })
        ->where('periode', $periode)->where('tahun', $tahun);
    if ($divisi) {
        $a->where('div_code', $divisi);
    }
    $a = $a->first();
    if ($normalBalance == 'd') {
        $hasil =  $a->total_debet - $a->total_credit;
    } else {
        $hasil = $a->total_credit - $a->total_debet;
    }

    return round($hasil, 2);
}
function getTotalLastMonth($account, $periode, $tahun, $normalBalance, $divisi = null)
{

    $arrAccount = explode(",", $account);
    $periode = $periode - 1;
    $a = MstSaldoNew::select(DB::Raw("
    sum(debet) as total_debet, sum(credit) as total_credit
    "))
        ->where(function ($query) use ($arrAccount) {
            foreach ($arrAccount as $value) {
                $query->orWhere('acc_code', 'ilike', $value);
            }
        })
        ->where('periode', $periode)->where('tahun', $tahun);
    if ($divisi) {
        $a->where('div_code', $divisi);
    }
    $a = $a->first();
    if ($normalBalance == 'd') {
        $hasil =  $a->total_debet - $a->total_credit;
    } else {
        $hasil = $a->total_credit - $a->total_debet;
    }

    return round($hasil, 2);
}
function getTotalLastYear($account, $tahun, $normalBalance, $divisi = null)
{

    $arrAccount = explode(",", $account);
    $periode = 13;
    $tahun = $tahun - 1;
    $a = MstSaldoNew::select(DB::Raw("
    sum(debet) as total_debet, sum(credit) as total_credit
    "))
        ->where(function ($query) use ($arrAccount) {
            foreach ($arrAccount as $value) {
                $query->orWhere('acc_code', 'ilike', $value);
            }
        })
        ->where('periode', $periode)->where('tahun', $tahun);
    if ($divisi) {
        $a->where('div_code', $divisi);
    }
    $a = $a->first();

    if ($normalBalance == 'd') {
        $hasil =  $a->total_debet - $a->total_credit;
    } else {
        $hasil = $a->total_credit - $a->total_debet;
    }

    return round($hasil, 2);
}
