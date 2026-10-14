<?php

namespace App\Livewire\Pages\Report\TrialBalance;

use App\Livewire\Traits\WithPeriodSelection;
use App\Support\Ledger;
use App\Support\Statements;
use Livewire\Component;

/** Neraca saldo (PRD menu 36, QA BUG-063). */
class TrialBalance extends Component
{
    use WithPeriodSelection;

    public $showZero = false;

    public function mount(): void
    {
        $this->initPeriod();
    }

    public function render()
    {
        [$year, $period, $division] = $this->selection();
        $a = Ledger::active();

        return view('livewire.pages.report.trial-balance.trial-balance', array_merge(
            Statements::trialBalance($year, $period, $division, (bool) $this->showZero),
            $this->selectionOptions(),
            [
                'label' => Ledger::periodLabel($year, $period),
                'provisional' => $year > $a['year'] || ($year === $a['year'] && $period >= $a['period']),
                'generatedAt' => now(),
            ]
        ));
    }
}
