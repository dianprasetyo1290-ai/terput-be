<?php

namespace App\Livewire\Pages\EndOfMonth;

use App\Models\JournalTransaction;
use App\Models\Parameter;
use App\Models\MstSaldoNew;
use App\Support\AuditLog;
use App\Support\Ledger;
use Livewire\Attributes\Locked;
use Illuminate\Support\Facades\DB;
use Livewire\Component;

class EOM extends Component
{
    public $period;
    public $year;
    public $checks = [];
    public $allPassed = false;

    /** Period the page was opened for; transfer() refuses any other. */
    #[Locked]
    public $expectedPeriod;

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

        // 4. QA BUG-147: every journal of the period must be posted (or
        // reversed) -- a draft or a journal waiting for approval would be
        // archived into history and counted in the closed balances.
        $open = DB::table('trans_head')->whereIn('status', ['draft', 'pending'])
            ->orderBy('trans_date')->get(['trans_no', 'status']);

        // 5. QA BUG-147: failed automatic postings (billing/payment) dated in
        // this period must be fixed and re-sent before it is closed.
        $failed = $this->failedPostings();

        // 6. Informational: an empty month can be closed too.
        $transactionCount = DB::table('trans_head')->count();

        $this->checks = [
            ['label' => 'Journal Integrity', 'desc' => 'Semua jurnal memiliki detail transaksi', 'passed' => $orphanCount === 0, 'issue' => $orphanCount],
            ['label' => 'Balance Integrity', 'desc' => 'Total header sesuai dengan detail', 'passed' => $mismatchCount === 0, 'issue' => $mismatchCount],
            ['label' => 'Unbalance Journal', 'desc' => 'Semua jurnal balance (debit = credit)', 'passed' => $unbalancedCount === 0, 'issue' => $unbalancedCount],
            ['label' => 'Jurnal Draft / Menunggu Approval', 'desc' => 'Semua jurnal periode ini sudah diposting',
                'passed' => $open->isEmpty(), 'issue' => $open->count(),
                'items' => $open->map(fn ($j) => "{$j->trans_no} ({$j->status})")->take(20)->all()],
            ['label' => 'Posting Otomatis Gagal', 'desc' => 'Tidak ada posting otomatis gagal pada periode ini',
                'passed' => $failed->isEmpty(), 'issue' => $failed->count(),
                'items' => $failed->map(fn ($f) => "{$f->kind} {$f->reference} ({$f->posting_date})")->take(20)->all()],
            ['label' => 'Data Transaksi', 'desc' => "$transactionCount jurnal akan dipindahkan ke history", 'passed' => true, 'issue' => 0, 'count' => $transactionCount, 'info' => true],
        ];

        $this->allPassed = collect($this->checks)->every(fn($c) => $c['passed']);
    }

    /** Failed auto postings (acc_auto_postings) dated inside the open period. */
    private function failedPostings()
    {
        $q = DB::table('acc_auto_postings')->where('status', 'failed');
        $range = Ledger::range((int) $this->year, (int) $this->period);
        if ($range) {
            $q->whereBetween('posting_date', $range);
        }

        return $q->orderBy('posting_date')->get(['kind', 'reference', 'posting_date']);
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
        $this->expectedPeriod = (int) $this->period;
        $this->runChecks();
    }

    private function alert(string $text, string $icon): void
    {
        $text = json_encode($text);
        $this->js("Swal.fire({text: $text, icon: '$icon', confirmButtonColor: '#3085d6', confirmButtonText: 'OK'});");
    }

    public function loadPeriod()
    {
        $param = $this->resolveParameterContext();
        $this->period = $param['period'];
        $this->year = $param['year'];
    }

    public function transfer()
    {
        $this->authorize('createend_of_month');

        // Guard double-run (QA BUG-147): the period this page was opened for
        // must still be the active one. A second click, a second tab or a
        // stale page would otherwise close the NEXT period too.
        $expected = (int) $this->expectedPeriod;
        $this->loadPeriod();
        if ($expected !== (int) $this->period || (int) $this->period >= 13) {
            $this->runChecks();
            $this->alert("Periode $expected sudah ditutup atau periode aktif berubah. Muat ulang halaman.", 'warning');

            return;
        }

        // Never trust the button state: re-run every check server-side.
        $this->runChecks();
        if (! $this->allPassed) {
            $this->alert('Validasi belum lengkap. Perbaiki masalah yang ditemukan sebelum melanjutkan.', 'error');

            return;
        }

        $closed = DB::transaction(function () use ($expected) {
            // Serialise concurrent runs on the parameter row.
            $param = DB::table('parameter')->lockForUpdate()->first();
            if (! $param || (int) $param->period !== $expected) {
                return false;
            }

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
            AuditLog::record('period.close', $this->year.'-'.str_pad((string) $expected, 2, '0', STR_PAD_LEFT), ['journals' => $data->count()]);

            return true;
        });
        if (! $closed) {
            $this->alert("Periode $expected sudah ditutup oleh proses lain.", 'warning');

            return;
        }
        $this->expectedPeriod = (int) $this->period;
        $this->runChecks();

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
