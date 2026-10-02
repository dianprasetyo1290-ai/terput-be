<?php

namespace App\Livewire\Component;

use App\Models\Balance;
use App\Models\MstSaldoNew;
use App\Models\Parameter;
use Livewire\Component;
use Livewire\Attributes\Reactive;

class FormBalance extends Component
{
    #[Reactive]
    public $acc_code;
    #[Reactive]
    public  $div_code;
    #[Reactive]
    public $tahun;
    public $asu;

    // public function  updated($property)
    // {
    //     if ($property === 'acc_code') {
    //         $this->ambilData();
    //     }
    //     if ($property === 'div_code') {
    //         $this->ambilData();
    //         // dd("asu");
    //     }
    // }
    public function abc()
    {
        $periods = range(0, 13);
        foreach ($periods as $period) {
            // Skip if acc_code or div_code is null
            if ($this->acc_code === null || $this->div_code === null) {
                continue;
            }

            Balance::create([
                'acc_code' => $this->acc_code,
                'div_code' => $this->div_code,
                'tahun' => $this->tahun,
                'periode' => $period,  // Insert one period value at a time
                'credit' => $item['credit'] ?? 0,
                'debet' => $item['debet'] ?? 0,
            ]);
        }
        $this->render();
    }

    public function simpan()
    {

        foreach ($this->asu as $item) {
            $a = Balance::where('acc_code', $item['acc_code'])
                ->where('div_code', $item['div_code'])
                ->where('tahun', $item['tahun'])
                ->where('periode', $item['periode'])->first();

            if ($a) {
                $a->credit = $item['credit'];
                $a->debet = $item['debet'];
                $a->save();
            }
        }
    }

    public function render()
    {

        $b = Balance::where('acc_code', $this->acc_code)->where('div_code', $this->div_code)->where('tahun', $this->tahun)
            ->orderBy('periode', 'asc')->get()
            ->toArray();
        if ($b) {
            $this->asu = $b;
        } else {
            $this->abc();
        }
        // dd($data);
        return view('livewire.component.form-balance');
    }
}
