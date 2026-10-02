<?php

namespace App\Livewire\Pages\EndOfMonth;

use App\Models\JournalTransaction;
use App\Models\Parameter;
use App\Models\MstSaldoNew;
use Illuminate\Support\Facades\DB;
use Livewire\Component;

class EOM extends Component
{
    public $period;
    public $year;
    public $checks = [];
    public $allPassed = false;

    private function resolveParameterContext(): array
    {
        $parameter = Parameter::first();

        return [
            'period' => optional($parameter)->period ?? '01',
            'year' => (int) (optional($parameter)->year ?? now()->year),
        ];
    }

    public function runChecks()
    {
        // 1. Journal Integrity - orphan headers without details
        $orphanCount = DB::table('trans_head')
            ->leftJoin('trans_detail', 'trans_head.trans_no', '=', 'trans_detail.trans_no')
            ->whereNull('trans_detail.id')
            ->count();

        // 2. Balance Integrity - header totals match detail sums
        $mismatchCount = DB::table('trans_head')
            ->join('trans_detail', 'trans_head.trans_no', '=', 'trans_detail.trans_no')
            ->select('trans_head.trans_no')
            ->groupBy('trans_head.trans_no', 'trans_head.debet', 'trans_head.credit')
            ->havingRaw('ABS(trans_head.debet - SUM(trans_detail.debet)) > 0.01
                      OR ABS(trans_head.credit - SUM(trans_detail.credit)) > 0.01')
            ->get()->count();

        // 3. Unbalance Journal - use existing scope
        $unbalancedCount = JournalTransaction::withNotEqualDebetAndCredit()->count();

        // 4. Has transactions to transfer
        $transactionCount = DB::table('trans_head')->count();

        $this->checks = [
            ['label' => 'Journal Integrity', 'desc' => 'Semua jurnal memiliki detail transaksi', 'passed' => $orphanCount === 0, 'issue' => $orphanCount],
            ['label' => 'Balance Integrity', 'desc' => 'Total header sesuai dengan detail', 'passed' => $mismatchCount === 0, 'issue' => $mismatchCount],
            ['label' => 'Unbalance Journal', 'desc' => 'Semua jurnal balance (debit = credit)', 'passed' => $unbalancedCount === 0, 'issue' => $unbalancedCount],
            ['label' => 'Data Transaksi', 'desc' => 'Terdapat data transaksi untuk ditransfer', 'passed' => $transactionCount > 0, 'issue' => $transactionCount === 0 ? 1 : 0, 'count' => $transactionCount],
        ];

        $this->allPassed = collect($this->checks)->every(fn($c) => $c['passed']);
    }

    public function confirmTransfer()
    {
        if (!$this->allPassed) {
            $this->js(<<<'JS'
                Swal.fire({
                    text: "Validasi belum lengkap. Perbaiki masalah yang ditemukan sebelum melanjutkan.",
                    icon: 'error',
                    confirmButtonColor: '#3085d6',
                    confirmButtonText: 'OK'
                });
            JS);
            return;
        }

        $this->js(<<<'JS'
        Swal.fire({
                text: "Do you want to Transfer this data? This process is irreversible.",
                icon: 'question',
                showDenyButton: true,
                confirmButtonColor: '#3085d6',
                confirmButtonText: 'Process!',
                denyButtonText: 'Cancel'
          }).then((result) => {
            if (result.isConfirmed) {
                $wire.transfer()
            } else if (result.isDenied) {
                // $wire.clear()
            }
          })
        JS);
    }

    public function mount()
    {
        $this->loadPeriod();
        $this->runChecks();
    }

    public function loadPeriod()
    {
        $param = $this->resolveParameterContext();
        $this->period = $param['period'];
        $this->year = $param['year'];
    }

    public function transfer()
    {
        $this->loadPeriod();

        // Guard double-run: kalau trans_head sudah kosong, periode ini sudah
        // ditutup. Tanpa guard ini, re-run akan tetap incrementPeriod() dan
        // melompati periode.
        if (DB::table('trans_head')->count() === 0) {
            $this->js(<<<'JS'
                Swal.fire({
                    text: "Tidak ada data untuk ditransfer. Periode ini kemungkinan sudah ditutup.",
                    icon: 'warning',
                    confirmButtonColor: '#3085d6',
                    confirmButtonText: 'OK'
                });
            JS);
            return;
        }

        DB::transaction(function () {
            $data = DB::table('trans_head')->get();
            $data2 = DB::table('trans_detail')->get();

            foreach ($data as $row) {
                $rowArray = (array) $row;
                $rowArray['period'] = $this->period;

                // Dedup harus include trans_date: trans_no sering reused antar bulan
                // untuk jurnal recurring (mis. JV/1/25 dipakai tiap bulan).
                $exists = DB::table('this_head')
                    ->where('trans_no', $row->trans_no)
                    ->where('trans_date', $row->trans_date)
                    ->exists();

                if (!$exists) {
                    DB::table('this_head')->insert($rowArray);
                }
            }

            foreach ($data2 as $row2) {
                $rowArray2 = (array) $row2;
                unset($rowArray2['id']);
                unset($rowArray2['trans_system']);
                unset($rowArray2['trans_yes']);
                $rowArray2['period'] = $this->period;

                // Dedup harus include trans_date: jurnal recurring pakai (trans_no,
                // trans_count, acc_code, div_code) yang sama tiap bulan, hanya
                // trans_date yang berbeda.
                $existsDetail = DB::table('this_detail')
                    ->where('trans_no', $row2->trans_no)
                    ->where('trans_count', $row2->trans_count)
                    ->where('acc_code', $row2->acc_code)
                    ->where('div_code', $row2->div_code)
                    ->where('trans_date', $row2->trans_date)
                    ->exists();

                if (!$existsDetail) {
                    DB::table('this_detail')->insert($rowArray2);
                }
            }

            DB::table('trans_head')->delete();
            DB::table('trans_detail')->delete();

            // Update mst_saldo_news dengan agregasi dari this_detail untuk periode ini
            $this->updateSaldoFromTransactions();

            $this->incrementPeriod();
        });

        $this->js(<<<'JS'
            const Toast = Swal.mixin({
                toast: true,
                position: "top-end",
                showConfirmButton: false,
                timer: 2000,
                timerProgressBar: true,
                didOpen: (toast) => {
                    toast.onmouseenter = Swal.stopTimer;
                    toast.onmouseleave = Swal.resumeTimer;
                }
            });
            Toast.fire({
                icon: "success",
                title: "Processing End Of Month successfully"
            });
        JS);
    }

    /**
     * Update mst_saldo_news dengan agregasi transaksi dari this_detail
     * untuk periode dan tahun yang sedang ditutup.
     *
     * Reset dulu saldo periode ini karena TransDetail::created events
     * sudah mengupdate saldo saat transaksi dibuat, tetapi raw delete
     * di transfer() tidak trigger Eloquent deleted events untuk menguranginya.
     */
    public function updateSaldoFromTransactions()
    {
        $currentPeriode = (int) $this->period;
        $currentYear = (int) $this->year;

        // Reset saldo periode ini untuk mencegah double-counting
        MstSaldoNew::where('periode', $currentPeriode)
            ->where('tahun', $currentYear)
            ->update(['debet' => 0, 'credit' => 0]);

        // Agregasi dari this_detail (sumber kebenaran setelah transfer)
        // Filter by year via trans_date untuk mencegah kontaminasi lintas tahun
        $aggregatedTransactions = DB::table('this_detail')
            ->select('acc_code', 'div_code')
            ->selectRaw('SUM(COALESCE(debet, 0)) as total_debet')
            ->selectRaw('SUM(COALESCE(credit, 0)) as total_credit')
            ->where('period', $this->period)
            ->whereRaw('EXTRACT(YEAR FROM trans_date) = ?', [$currentYear])
            ->groupBy('acc_code', 'div_code')
            ->get();

        foreach ($aggregatedTransactions as $trans) {
            $existingSaldo = MstSaldoNew::where('acc_code', $trans->acc_code)
                ->where('div_code', $trans->div_code)
                ->where('periode', $currentPeriode)
                ->where('tahun', $currentYear)
                ->first();

            if ($existingSaldo) {
                $existingSaldo->debet = $trans->total_debet;
                $existingSaldo->credit = $trans->total_credit;
                $existingSaldo->save();
            } else {
                MstSaldoNew::create([
                    'acc_code' => $trans->acc_code,
                    'div_code' => $trans->div_code,
                    'periode' => $currentPeriode,
                    'tahun' => $currentYear,
                    'debet' => $trans->total_debet,
                    'credit' => $trans->total_credit,
                ]);
            }
        }
    }

    public function incrementPeriod()
    {
        $parameter = Parameter::first();
        if (!$parameter) {
            $this->js(<<<'JS'
            Swal.fire({
                text: "Parameter perusahaan belum tersedia. Buka menu Company Parameter lalu isi tahun dan periode aktif.",
                icon: 'warning',
                confirmButtonColor: '#3085d6',
                confirmButtonText: 'OK'
            });
            JS);
            return;
        }

        $currentPeriod = (int) $parameter->period;
        $newPeriod = $currentPeriod + 1;

        // End of Month hanya bisa maju maksimal sampai periode 13 (Adjustment).
        // Periode 13 menjadi state yang dituntut untuk menjalankan End of Year.
        if ($newPeriod > 13) {
            $parameter->period = 13;
        } else {
            $parameter->period = $newPeriod;
        }

        // Simpan perubahan
        $parameter->save();

        // Perbarui nilai properti period dan year di komponen Livewire
        $this->period = $parameter->period;
        $this->year = $parameter->year;
    }

    public function render()
    {
        return view('livewire.pages.end-of-month.e-o-m');
    }
}
