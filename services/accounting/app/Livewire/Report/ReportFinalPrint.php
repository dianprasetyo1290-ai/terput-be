<?php

namespace App\Livewire\Report;

use App\Models\ReportRowHead;
use Livewire\Component;
use Livewire\Attributes\Layout;

class ReportFinalPrint extends Component
{
    protected $queryString = ['idHead'];

    public $idHead = 1;
    public $periode = 5;
    public $tahun = 2024;
    // public $nilai;

    public function mount($periode = null, $tahun = NULL, $idHead = NULL)
    {
        if ($periode) {
            $this->periode = $periode;
        }
        if ($tahun) {
            $this->tahun = $tahun;
        }
        if ($idHead) {
            $this->idHead = $idHead;
        }
    }
    #[Layout('layouts.rpt')]
    public function render()
    {
        $dataRow = ReportRowHead::with(['reportRowDetail'])->find($this->idHead);
        return view('livewire.report.report-final-print', ['dataRow' => $dataRow]);
    }
}
