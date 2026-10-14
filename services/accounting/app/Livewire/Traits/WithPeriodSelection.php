<?php

namespace App\Livewire\Traits;

use App\Support\Ledger;
use Illuminate\Support\Facades\DB;

/** Year / period / unit picker shared by the ledger-based reports. */
trait WithPeriodSelection
{
    public $year;
    public $period;
    /** '' = all units */
    public $division = '';

    protected function initPeriod(): void
    {
        $a = Ledger::active();
        $this->year ??= $a['year'];
        $this->period ??= min(12, max(1, $a['period']));
    }

    /** Normalised [year, period, division|null]. */
    protected function selection(): array
    {
        $year = max(2000, min(2100, (int) $this->year));
        $period = max(1, min(13, (int) $this->period));
        $division = $this->division !== '' && $this->division !== null ? (string) $this->division : null;

        return [$year, $period, $division];
    }

    protected function selectionOptions(): array
    {
        $years = DB::table('mst_saldo_news')->distinct()->orderBy('tahun')->pluck('tahun')->map(fn ($y) => (int) $y)
            ->push(Ledger::active()['year'])->unique()->sort()->values();

        return [
            'years' => $years,
            'periods' => collect(range(1, 13))->mapWithKeys(fn ($p) => [$p => $p === 13 ? 'Penyesuaian (13)' : Ledger::periodLabel((int) $this->year ?: now()->year, $p)]),
            'divisions' => DB::table('mst_division')->orderBy('div_code')->get(['div_code', 'div_name']),
        ];
    }
}
