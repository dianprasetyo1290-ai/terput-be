<?php

use App\Models\ChartOfAccount;
use App\Models\Division;
use App\Models\MstSaldoNew;
use App\Models\SettingWeb;

function getMonthName($monthNumber)
{
    $months = [
        1 => 'Jan',
        2 => 'Feb',
        3 => 'Mar',
        4 => 'Apr',
        5 => 'Mei',
        6 => 'Jun',
        7 => 'Jul',
        8 => 'Agu',
        9 => 'Sep',
        10 => 'Okt',
        11 => 'Nov',
        12 => 'Des'
    ];

    return $months[$monthNumber] ?? 'Adj';
}

if (!function_exists('title_web')) {
    function title_web()
    {
        $title = str_replace('-', ' ', strtoupper(request()->path()));
        $title = str_replace('/', ' - ', strtoupper($title));
        return $title;
    }
}

if (!function_exists('get_setting_web')) {
    function get_setting_web()
    {
        return SettingWeb::find(1) ?? new SettingWeb();
    }
}

if (!function_exists('cek_min')) {
    function cek_min($nilai)
    {
        if ($nilai >= 0) {
            return number_format($nilai, 2, ',', '.');
        } else {
            // Menghilangkan tanda minus dengan abs() jika nilai kurang dari 0
            return "( " . number_format(abs($nilai), 2, ',', '.') . " )";
        }
    }
}

if (!function_exists('gen_mst_saldo')) {
    function gen_mst_saldo($year)
    {
        $coa = ChartOfAccount::all();
        foreach ($coa as $datum) {
            $division = Division::all();
            foreach ($division as $div) {
                for ($a = 1; $a <= 12; $a++) {
                    // cek jika sudah ada
                    $cek = MstSaldoNew::where('tahun', $year)
                        ->where('div_code', $div->div_code)
                        ->where('periode', $a)
                        ->first();
                    if (!$cek) {
                        MstSaldoNew::create([
                            'acc_code' => $datum->acc_code,
                            'div_code' => $div->div_code,
                            'periode' => $a,
                            'tahun' => $year,
                            'debet' => 0,
                            'credit' => 0

                        ]);
                    }
                }
            }
        }
    }
}

function formatTanggalIndonesia($tanggal)
{
    // Pastikan tanggal tidak kosong atau NULL
    if (!$tanggal) {
        return null;
    }

    // Ubah format tanggal dari YYYY-MM-DD ke DD-MM-YYYY
    return date("d-m-Y", strtotime($tanggal));
}

if (!function_exists('image_url')) {
    function image_url($path, $fallback = null)
    {
        // Return fallback if path is empty, null, or invalid
        if (empty($path) || trim($path) === '' || strpos($path, '..') !== false) {
            return $fallback ?: asset('limitless4.0/assets/images/placeholders/placeholder.jpg');
        }

        return route('helper.show-picture', ['path' => $path]);
    }
}
