<?php

namespace App\Livewire\Component;

use App\Support\AuditLog;
use App\Support\Ledger;
use Illuminate\Support\Facades\DB;
use Livewire\Attributes\Reactive;
use Livewire\Component;

/**
 * Balance of one account/unit/year per period (mst_saldo_news).
 *
 * QA round 2: viewing no longer writes (it used to insert 14 zero rows on
 * render, also for read-only roles). Period balances 1-13 are produced by
 * End of Month from posted journals and are shown read-only; only the opening
 * balance (period 0) of the ACTIVE year can be edited here (editbalance), with
 * an audit entry -- the same row the opening-balance import writes.
 */
class FormBalance extends Component
{
    #[Reactive]
    public $acc_code;
    #[Reactive]
    public $div_code;
    #[Reactive]
    public $tahun;

    public $asu = [];
    public $message = null;
    public $error = null;

    public function simpan()
    {
        $this->authorize('editbalance');
        $this->message = null;
        $this->error = null;
        if (! $this->acc_code || ! $this->div_code || ! $this->tahun) {
            $this->error = 'Pilih akun dan unit terlebih dahulu.';

            return;
        }
        if ((int) $this->tahun !== Ledger::active()['year']) {
            $this->error = 'Saldo awal hanya dapat diubah untuk tahun buku aktif.';

            return;
        }
        $row = collect($this->asu)->firstWhere('periode', 0);
        $d = $this->num($row['debet'] ?? 0);
        $c = $this->num($row['credit'] ?? 0);
        if ($d === null || $c === null || $d < 0 || $c < 0) {
            $this->error = 'Saldo awal harus berupa angka tidak negatif.';

            return;
        }
        $key = ['acc_code' => $this->acc_code, 'div_code' => $this->div_code, 'tahun' => (int) $this->tahun, 'periode' => 0];
        $before = DB::table('mst_saldo_news')->where($key)->first(['debet', 'credit']);
        DB::table('mst_saldo_news')->updateOrInsert($key, ['debet' => $d, 'credit' => $c, 'updated_at' => now()]);
        AuditLog::record('balance.opening', $this->acc_code.'/'.$this->div_code.'/'.$this->tahun,
            ['before' => $before ? (array) $before : null, 'after' => ['debet' => $d, 'credit' => $c]]);
        $this->message = 'Saldo awal disimpan.';
    }

    private function num(mixed $v): ?float
    {
        if (is_string($v)) {
            $v = str_replace([' ', ','], ['', '.'], trim($v));
        }
        if ($v === '' || $v === null) {
            return 0.0;
        }

        return is_numeric($v) ? round((float) $v, 2) : null;
    }

    public function render()
    {
        $rows = ($this->acc_code && $this->div_code && $this->tahun)
            ? DB::table('mst_saldo_news')->where('acc_code', $this->acc_code)->where('div_code', $this->div_code)
                ->where('tahun', (int) $this->tahun)->get(['periode', 'debet', 'credit'])->keyBy('periode')
            : collect();
        $this->asu = array_map(fn ($p) => [
            'periode' => $p,
            'debet' => (float) ($rows[$p]->debet ?? 0),
            'credit' => (float) ($rows[$p]->credit ?? 0),
        ], range(0, 13));

        return view('livewire.component.form-balance', [
            'editable' => (int) $this->tahun === Ledger::active()['year'],
        ]);
    }
}
