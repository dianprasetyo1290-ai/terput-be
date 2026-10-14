<?php

namespace App\Livewire\Pages\Report\Statements;

use App\Livewire\Traits\WithPeriodSelection;
use App\Support\Statements as Builder;
use Livewire\Component;

/**
 * Laporan keuangan dari buku besar (PRD menu 37, QA BUG-064): aktivitas,
 * posisi keuangan, arus kas, perubahan aset neto, with comparison against the
 * previous period, per unit or all units.
 */
class Statements extends Component
{
    use WithPeriodSelection;

    public $tab = 'activity';

    public function mount(): void
    {
        $this->initPeriod();
    }

    public function setTab(string $tab): void
    {
        $this->tab = in_array($tab, ['activity', 'position', 'cash_flow', 'net_assets'], true) ? $tab : 'activity';
    }

    public function render()
    {
        [$year, $period, $division] = $this->selection();

        return view('livewire.pages.report.statements.statements', array_merge(
            ['s' => Builder::build($year, $period, $division)],
            $this->selectionOptions()
        ));
    }
}
