<?php

namespace App\Livewire\Pages\Report\Consolidation;

use App\Livewire\Traits\WithPeriodSelection;
use App\Support\Ledger;
use App\Support\Statements;
use Livewire\Component;

/** Konsolidasi antar unit dengan eliminasi akun antar-unit (PRD menu 38, QA BUG-065). */
class Consolidation extends Component
{
    use WithPeriodSelection;

    public function mount(): void
    {
        $this->initPeriod();
    }

    public function render()
    {
        [$year, $period] = $this->selection();
        $a = Ledger::active();

        return view('livewire.pages.report.consolidation.consolidation', array_merge(
            ['c' => Statements::consolidation($year, $period)],
            $this->selectionOptions(),
            [
                'label' => Ledger::periodLabel($year, $period),
                'provisional' => $year > $a['year'] || ($year === $a['year'] && $period >= $a['period']),
                'generatedAt' => now(),
            ]
        ));
    }
}
