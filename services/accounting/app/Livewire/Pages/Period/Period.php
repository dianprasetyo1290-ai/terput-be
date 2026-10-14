<?php

namespace App\Livewire\Pages\Period;

use App\Models\Parameter;
use App\Models\Period as ModelsPeriod;
use Carbon\Carbon;
use Illuminate\Support\Facades\Crypt;
use Livewire\Component;
use Livewire\WithPagination;

class Period extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';
    public $search, $order = "ASC", $limit = 10;

    public $form, $id1Nya, $id2Nya, $id3Nya, $lihat;
    public $period;

    public $years;
    public $Curent;
    public $Last;
    public $LastTwo;
    public $activeTab = 'tab1'; // Default tab

    private function getActiveYear(): int
    {
        return (int) optional(Parameter::first())->year ?: now()->year;
    }

    public function setActiveTab($tab)
    {
        $this->activeTab = $tab;
    }

    private function initializeYearsRange()
    {
        $currentYear = $this->getActiveYear();
        $range = 2;
        $years = [];

        for ($i = $currentYear - $range; $i <= $currentYear + $range; $years[] = $i++) {
            // Years array is being populated
        }

        $this->years = $years;
    }

    public function updatedCurentPeriodName($value)
    {
        if ($value) {
            $this->Curent['period_code'] = 1;
            $this->Last['period_code'] = 2;
            $this->Last['period_name'] = (int) $value - 1;
            $this->LastTwo['period_name'] = (int) $value - 2;
            $this->LastTwo['period_code'] = 3;
        } else {
            $this->Last['period_code'] = null;
            $this->Last['period_name'] = null;
            $this->LastTwo['period_name'] = null;
            $this->LastTwo['period_code'] = null;
        }
    }

    public function updatedCurentp01($value)
    {
        if ($value) {
            $this->Last['p01'] = $value;
            $this->LastTwo['p01'] = $value;
        } else {
            $this->Last['p01'] = null;
            $this->LastTwo['p01'] = null;
        }
    }

    public function updatedCurentp01From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p01_from'] = $lastDate;
            $this->LastTwo['p01_from'] = $lastTwoDate;
        } else {
            $this->Last['p01_from'] = null;
            $this->LastTwo['p01_from'] = null;
        }
    }

    public function updatedCurentp01To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p01_to'] = $lastDate;
            $this->LastTwo['p01_to'] = $lastTwoDate;
        } else {
            $this->Last['p01_to'] = null;
            $this->LastTwo['p01_to'] = null;
        }
    }

    public function updatedCurentp02($value)
    {
        if ($value) {
            $this->Last['p02'] = $value;
            $this->LastTwo['p02'] = $value;
        } else {
            $this->Last['p02'] = null;
            $this->LastTwo['p02'] = null;
        }
    }

    public function updatedCurentp02From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p02_from'] = $lastDate;
            $this->LastTwo['p02_from'] = $lastTwoDate;
        } else {
            $this->Last['p02_from'] = null;
            $this->LastTwo['p02_from'] = null;
        }
    }

    public function updatedCurentp02To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p02_to'] = $lastDate;
            $this->LastTwo['p02_to'] = $lastTwoDate;
        } else {
            $this->Last['p02_to'] = null;
            $this->LastTwo['p02_to'] = null;
        }
    }

    public function updatedCurentp03($value)
    {
        if ($value) {
            $this->Last['p03'] = $value;
            $this->LastTwo['p03'] = $value;
        } else {
            $this->Last['p03'] = null;
            $this->LastTwo['p03'] = null;
        }
    }

    public function updatedCurentp03From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p03_from'] = $lastDate;
            $this->LastTwo['p03_from'] = $lastTwoDate;
        } else {
            $this->Last['p03_from'] = null;
            $this->LastTwo['p03_from'] = null;
        }
    }

    public function updatedCurentp03To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p03_to'] = $lastDate;
            $this->LastTwo['p03_to'] = $lastTwoDate;
        } else {
            $this->Last['p03_to'] = null;
            $this->LastTwo['p03_to'] = null;
        }
    }

    public function updatedCurentp04($value)
    {
        if ($value) {
            $this->Last['p04'] = $value;
            $this->LastTwo['p04'] = $value;
        } else {
            $this->Last['p04'] = null;
            $this->LastTwo['p04'] = null;
        }
    }

    public function updatedCurentp04From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p04_from'] = $lastDate;
            $this->LastTwo['p04_from'] = $lastTwoDate;
        } else {
            $this->Last['p04_from'] = null;
            $this->LastTwo['p04_from'] = null;
        }
    }

    public function updatedCurentp04To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p04_to'] = $lastDate;
            $this->LastTwo['p04_to'] = $lastTwoDate;
        } else {
            $this->Last['p04_to'] = null;
            $this->LastTwo['p04_to'] = null;
        }
    }

    public function updatedCurentp05($value)
    {
        if ($value) {
            $this->Last['p05'] = $value;
            $this->LastTwo['p05'] = $value;
        } else {
            $this->Last['p05'] = null;
            $this->LastTwo['p05'] = null;
        }
    }

    public function updatedCurentp05From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p05_from'] = $lastDate;
            $this->LastTwo['p05_from'] = $lastTwoDate;
        } else {
            $this->Last['p05_from'] = null;
            $this->LastTwo['p05_from'] = null;
        }
    }

    public function updatedCurentp05To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p05_to'] = $lastDate;
            $this->LastTwo['p05_to'] = $lastTwoDate;
        } else {
            $this->Last['p05_to'] = null;
            $this->LastTwo['p05_to'] = null;
        }
    }

    public function updatedCurentp06($value)
    {
        if ($value) {
            $this->Last['p06'] = $value;
            $this->LastTwo['p06'] = $value;
        } else {
            $this->Last['p06'] = null;
            $this->LastTwo['p06'] = null;
        }
    }

    public function updatedCurentp06From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p06_from'] = $lastDate;
            $this->LastTwo['p06_from'] = $lastTwoDate;
        } else {
            $this->Last['p06_from'] = null;
            $this->LastTwo['p06_from'] = null;
        }
    }

    public function updatedCurentp06To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p06_to'] = $lastDate;
            $this->LastTwo['p06_to'] = $lastTwoDate;
        } else {
            $this->Last['p06_to'] = null;
            $this->LastTwo['p06_to'] = null;
        }
    }

    public function updatedCurentp07($value)
    {
        if ($value) {
            $this->Last['p07'] = $value;
            $this->LastTwo['p07'] = $value;
        } else {
            $this->Last['p07'] = null;
            $this->LastTwo['p07'] = null;
        }
    }

    public function updatedCurentp07From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p07_from'] = $lastDate;
            $this->LastTwo['p07_from'] = $lastTwoDate;
        } else {
            $this->Last['p07_from'] = null;
            $this->LastTwo['p07_from'] = null;
        }
    }

    public function updatedCurentp07To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p07_to'] = $lastDate;
            $this->LastTwo['p07_to'] = $lastTwoDate;
        } else {
            $this->Last['p07_to'] = null;
            $this->LastTwo['p07_to'] = null;
        }
    }

    public function updatedCurentp08($value)
    {
        if ($value) {
            $this->Last['p08'] = $value;
            $this->LastTwo['p08'] = $value;
        } else {
            $this->Last['p08'] = null;
            $this->LastTwo['p08'] = null;
        }
    }

    public function updatedCurentp08From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p08_from'] = $lastDate;
            $this->LastTwo['p08_from'] = $lastTwoDate;
        } else {
            $this->Last['p08_from'] = null;
            $this->LastTwo['p08_from'] = null;
        }
    }

    public function updatedCurentp08To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p08_to'] = $lastDate;
            $this->LastTwo['p08_to'] = $lastTwoDate;
        } else {
            $this->Last['p08_to'] = null;
            $this->LastTwo['p08_to'] = null;
        }
    }

    public function updatedCurentp09($value)
    {
        if ($value) {
            $this->Last['p09'] = $value;
            $this->LastTwo['p09'] = $value;
        } else {
            $this->Last['p09'] = null;
            $this->LastTwo['p09'] = null;
        }
    }

    public function updatedCurentp09From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p09_from'] = $lastDate;
            $this->LastTwo['p09_from'] = $lastTwoDate;
        } else {
            $this->Last['p09_from'] = null;
            $this->LastTwo['p09_from'] = null;
        }
    }

    public function updatedCurentp09To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p09_to'] = $lastDate;
            $this->LastTwo['p09_to'] = $lastTwoDate;
        } else {
            $this->Last['p09_to'] = null;
            $this->LastTwo['p09_to'] = null;
        }
    }

    public function updatedCurentp10($value)
    {
        if ($value) {
            $this->Last['p10'] = $value;
            $this->LastTwo['p10'] = $value;
        } else {
            $this->Last['p10'] = null;
            $this->LastTwo['p10'] = null;
        }
    }

    public function updatedCurentp10From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p10_from'] = $lastDate;
            $this->LastTwo['p10_from'] = $lastTwoDate;
        } else {
            $this->Last['p10_from'] = null;
            $this->LastTwo['p10_from'] = null;
        }
    }

    public function updatedCurentp10To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p10_to'] = $lastDate;
            $this->LastTwo['p10_to'] = $lastTwoDate;
        } else {
            $this->Last['p10_to'] = null;
            $this->LastTwo['p10_to'] = null;
        }
    }

    public function updatedCurentp11($value)
    {
        if ($value) {
            $this->Last['p11'] = $value;
            $this->LastTwo['p11'] = $value;
        } else {
            $this->Last['p11'] = null;
            $this->LastTwo['p11'] = null;
        }
    }

    public function updatedCurentp11From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p11_from'] = $lastDate;
            $this->LastTwo['p11_from'] = $lastTwoDate;
        } else {
            $this->Last['p11_from'] = null;
            $this->LastTwo['p11_from'] = null;
        }
    }

    public function updatedCurentp11To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p11_to'] = $lastDate;
            $this->LastTwo['p11_to'] = $lastTwoDate;
        } else {
            $this->Last['p11_to'] = null;
            $this->LastTwo['p11_to'] = null;
        }
    }

    public function updatedCurentp12($value)
    {
        if ($value) {
            $this->Last['p12'] = $value;
            $this->LastTwo['p12'] = $value;
        } else {
            $this->Last['p12'] = null;
            $this->LastTwo['p12'] = null;
        }
    }

    public function updatedCurentp12From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p12_from'] = $lastDate;
            $this->LastTwo['p12_from'] = $lastTwoDate;
        } else {
            $this->Last['p12_from'] = null;
            $this->LastTwo['p12_from'] = null;
        }
    }

    public function updatedCurentp12To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p12_to'] = $lastDate;
            $this->LastTwo['p12_to'] = $lastTwoDate;
        } else {
            $this->Last['p12_to'] = null;
            $this->LastTwo['p12_to'] = null;
        }
    }

    public function updatedCurentp13($value)
    {
        if ($value) {
            $this->Last['p13'] = $value;
            $this->LastTwo['p13'] = $value;
        } else {
            $this->Last['p13'] = null;
            $this->LastTwo['p13'] = null;
        }
    }

    public function updatedCurentp13From($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p13_from'] = $lastDate;
            $this->LastTwo['p13_from'] = $lastTwoDate;
        } else {
            $this->Last['p13_from'] = null;
            $this->LastTwo['p13_from'] = null;
        }
    }

    public function updatedCurentp13To($value)
    {
        if ($value) {
            $date = Carbon::createFromFormat('Y-m-d', $value);
            $lastDate = $date->subYear()->format('Y-m-d');
            $date->addYear();
            $lastTwoDate = $date->subYears(2)->format('Y-m-d');

            $this->Last['p13_to'] = $lastDate;
            $this->LastTwo['p13_to'] = $lastTwoDate;
        } else {
            $this->Last['p13_to'] = null;
            $this->LastTwo['p13_to'] = null;
        }
    }

    public function add($id = "")
    {
        $this->js(<<<'JS'
          setTimeout(function() {
             $('.select-search').select2();
            }, 500);
          JS);

        if ($id) {
            $this->form = true;
            $this->resetValidation();
        } else {
            $this->form = true;
        }
    }

    public function save()
    {
        $valid = [
            'Curent.period_name' => 'required',
        ];

        foreach (range(1, 13) as $month) {
            $monthKey = sprintf('%02d', $month);
            $valid['Curent.p' . $monthKey] = 'required';
            $valid['Curent.p' . $monthKey . '_from'] = 'required';
            $valid['Curent.p' . $monthKey . '_to'] = 'required';
        }

        $valid['Last.period_name'] = 'required';

        foreach (range(1, 13) as $month) {
            $monthKey = sprintf('%02d', $month);
            $valid['Last.p' . $monthKey] = 'required';
            $valid['Last.p' . $monthKey . '_from'] = 'required';
            $valid['Last.p' . $monthKey . '_to'] = 'required';
        }

        $valid['LastTwo.period_name'] = 'required';

        foreach (range(1, 13) as $month) {
            $monthKey = sprintf('%02d', $month);
            $valid['LastTwo.p' . $monthKey] = 'required';
            $valid['LastTwo.p' . $monthKey . '_from'] = 'required';
            $valid['LastTwo.p' . $monthKey . '_to'] = 'required';
        }

        $this->validate($valid);

        if ($this->id1Nya and $this->id2Nya and $this->id3Nya) {
            $Curent = ModelsPeriod::find($this->id1Nya);
            $Curent->update($this->Curent);
            $Last = ModelsPeriod::find($this->id2Nya);
            $Last->update($this->Last);
            $LastTwo = ModelsPeriod::find($this->id3Nya);
            $LastTwo->update($this->LastTwo);
        } else if ($this->id1Nya and $this->id2Nya) {
            $Curent = ModelsPeriod::find($this->id1Nya);
            $Curent->update($this->Curent);
            $Last = ModelsPeriod::find($this->id2Nya);
            $Last->update($this->Last);
            $LastTwo = ModelsPeriod::create($this->LastTwo);
        } else if ($this->id1Nya) {
            $Curent = ModelsPeriod::find($this->id1Nya);
            $Curent->update($this->Curent);
            $LastTwo = ModelsPeriod::create($this->Last);
            $LastTwo = ModelsPeriod::create($this->LastTwo);
        } else {
            // $this->Curent['id'] = 1;
            // $this->Last['id'] = 2;
            // $this->LastTwo['id'] = 3;
            ModelsPeriod::create($this->Curent);
            ModelsPeriod::create($this->Last);
            ModelsPeriod::create($this->LastTwo);
        }

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
                title: "Data saved successfully"
            });
        JS);
    }
    public function clear()
    {
        return redirect()->to('periode');
    }
    public function mount()
    {
        $currentYear = $this->getActiveYear();
        $cek1 = ModelsPeriod::where('period_name', 'ilike', '%' . $currentYear . '%')->first();
        $cek2 = ModelsPeriod::where('period_name', 'ilike', '%' . ($currentYear - 1) . '%')->first();
        $cek3 = ModelsPeriod::where('period_name', 'ilike', '%' . ($currentYear - 2) . '%')->first();

        if ($cek1 and $cek2 and $cek3) {
            $this->id1Nya = $cek1->id;
            $this->id2Nya = $cek2->id;
            $this->id3Nya = $cek3->id;
            $this->Curent = $cek1->toArray();
            $this->Last = $cek2->toArray();
            $this->LastTwo = $cek3->toArray();
        } else if ($cek1 and $cek2) {
            $this->id1Nya = $cek1->id;
            $this->id2Nya = $cek2->id;
            $this->Curent = $cek1->toArray();
            $this->Last = $cek2->toArray();
            $this->LastTwo = [
                'period_name' => (int) $cek1->period_name - 2,
            ];
        } else if ($cek1) {
            $this->id1Nya = $cek1->id;
            $this->id2Nya = optional($cek2)->id;
            $this->Curent = $cek1->toArray();
            $this->Last = [
                'period_name' => (int) $cek1->period_name - 1,
            ];
            $this->LastTwo = [
                'period_name' => (int) $cek1->period_name - 2,
            ];
        } else {
            $this->Curent = [
                'period_code' => 1,
                'period_name' => $currentYear,
            ];

            foreach (range(1, 13) as $month) {
                $monthKey = sprintf('%02d', $month);
                $this->Curent['p' . $monthKey] = null;
                $this->Curent['p' . $monthKey . '_from'] = null;
                $this->Curent['p' . $monthKey . '_to'] = null;
            }
            $this->Last = [
                'period_code' => null,
                'period_name' => $currentYear - 1,
            ];

            foreach (range(1, 13) as $month) {
                $monthKey = sprintf('%02d', $month);
                $this->Last['p' . $monthKey] = null;
                $this->Last['p' . $monthKey . '_from'] = null;
                $this->Last['p' . $monthKey . '_to'] = null;
            }
            $this->LastTwo = [
                'period_code' => null,
                'period_name' => $currentYear - 2,
            ];

            foreach (range(1, 13) as $month) {
                $monthKey = sprintf('%02d', $month);
                $this->LastTwo['p' . $monthKey] = null;
                $this->LastTwo['p' . $monthKey . '_from'] = null;
                $this->LastTwo['p' . $monthKey . '_to'] = null;
            }
        }
    }

    public function render()
    {
        $data = ModelsPeriod::query();
        if ($this->search) {
            $data->where('curr_code', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('curr_name', 'ilike', '%' .  $this->search . '%');
        }
        if ($this->order) {
            $data->orderBy('id', $this->order);
        }
        $data = $data->paginate($this->limit);
        $this->initializeYearsRange();
        return view('livewire.pages.period.period', [
            'data' => $data,
        ]);
    }
}
