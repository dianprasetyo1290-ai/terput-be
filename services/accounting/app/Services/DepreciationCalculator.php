<?php

namespace App\Services;

use Carbon\Carbon;

class DepreciationCalculator
{
    /**
     * Hitung beban penyusutan bulanan (metode garis lurus integer rupiah).
     * Sisa pembagian ditempatkan pada bulan terakhir agar total akumulasi tepat sama dengan nilai yang disusutkan.
     */
    public static function monthly(int $cost, int $residual, int $lifeMonths, int $monthIndex): int
    {
        if ($lifeMonths <= 0 || $monthIndex < 0 || $monthIndex >= $lifeMonths) {
            return 0;
        }

        $depreciable = max(0, $cost - $residual);
        if ($depreciable === 0) {
            return 0;
        }

        $baseMonthly = intdiv($depreciable, $lifeMonths);
        $remainder = $depreciable % $lifeMonths;

        // Bulan terakhir menyerap sisa pembagian integer
        if ($monthIndex === $lifeMonths - 1) {
            return $baseMonthly + $remainder;
        }

        return $baseMonthly;
    }

    /**
     * Tentukan periode awal penyusutan (YYYY-MM).
     */
    public static function firstPeriod(string $acquiredOn, string $depStart = 'acquisition_month'): string
    {
        $d = Carbon::parse($acquiredOn);
        if ($depStart === 'next_month') {
            return $d->addMonth()->format('Y-m');
        }
        return $d->format('Y-m');
    }

    /**
     * Bentuk baris jurnal pelepasan aset tetap (selalu seimbang Dr == Cr).
     */
    public static function disposalLines(
        int $cost,
        int $accum,
        int $proceeds,
        string $assetAcc,
        string $accumAcc,
        string $gainAcc,
        string $lossAcc,
        ?string $cashAcc = null
    ): array {
        $lines = [];

        // 1. Balik akumulasi penyusutan (Debit)
        if ($accum > 0) {
            $lines[] = [
                'acc_code' => $accumAcc,
                'debit_idr' => $accum,
                'credit_idr' => 0,
                'description' => 'Balik Akumulasi Penyusutan Aset',
            ];
        }

        // 2. Penerimaan kas hasil penjualan jika ada (Debit)
        if ($proceeds > 0 && ! empty($cashAcc)) {
            $lines[] = [
                'acc_code' => $cashAcc,
                'debit_idr' => $proceeds,
                'credit_idr' => 0,
                'description' => 'Penerimaan Hasil Pelepasan Aset',
            ];
        }

        // 3. Hapus harga perolehan aset (Kredit)
        $lines[] = [
            'acc_code' => $assetAcc,
            'debit_idr' => 0,
            'credit_idr' => $cost,
            'description' => 'Penghentian Pengakuan Aset Tetap',
        ];

        // 4. Hitung Laba / Rugi Pelepasan
        // Net = (Akumulasi + Hasil) - Harga Perolehan
        $net = ($accum + $proceeds) - $cost;
        if ($net > 0) {
            // Laba Pelepasan Aset (Kredit)
            $lines[] = [
                'acc_code' => $gainAcc,
                'debit_idr' => 0,
                'credit_idr' => $net,
                'description' => 'Keuntungan Pelepasan Aset Tetap',
            ];
        } elseif ($net < 0) {
            // Rugi Pelepasan Aset (Debit)
            $lines[] = [
                'acc_code' => $lossAcc,
                'debit_idr' => abs($net),
                'credit_idr' => 0,
                'description' => 'Kerugian Pelepasan Aset Tetap',
            ];
        }

        return $lines;
    }
}
