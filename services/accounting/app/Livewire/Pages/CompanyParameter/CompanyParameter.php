<?php

namespace App\Livewire\Pages\CompanyParameter;

use App\Models\Parameter;
use App\Models\Period;
use Carbon\Carbon;
use Livewire\Component;

class CompanyParameter extends Component
{
    public $edit = false, $idNya;

    public $cp = [
        'company' => '',
        'serial_no' => '',
        'address' => '',
        'contact' => '',
        'tax_number' => '',
        'period' => '',
        'year' => '',
        'mask_account' => '',
        'base_currency' => '',
        'control_date' => '',
    ];

    public function getData()
    {
        $data = Parameter::first();
        if ($data) {
            $this->cp['company'] = $data->company;
            $this->cp['serial_no'] = $data->serial_no;
            $this->cp['address'] = $data->address;
            $this->cp['contact'] = $data->contact;
            $this->cp['tax_number'] = $data->tax_number;
            $this->cp['period'] = $data->period;
            $this->cp['year'] = $data->year;
            $this->cp['mask_account'] = $data->mask_account;
            $this->cp['base_currency'] = $data->base_currency;
            $this->cp['control_date'] = $data->control_date;
        }
        $this->idNya = $this->cp['serial_no'];
        $this->centangCheck($data->control_date ?? 0);
        $this->disableEnableCheck();
    }

    public function btnBatal()
    {
        $this->redirect('/companyparameter');
    }

    public function centangCheck($data)
    {
        if ($data == 1) {
            $this->js(
                <<<'JS'
                document.getElementById("check").checked = true;
                $("#check").prop( "checked", true );
                JS
            );
        } else {
            $this->js(
                <<<'JS'
                document.getElementById("check").checked = false;
                $("#check").prop( "checked", false );
                JS
            );
        }
    }

    public function disableEnableCheck()
    {
        if ($this->edit) {
            $this->js(<<<'JS'
                document.getElementById("check").disabled = false;
        JS);
        } else {
            $this->js(<<<'JS'
                document.getElementById("check").disabled = true;
            JS);
        }
    }

    public function mount()
    {
        $this->getData();
    }

    public function save()
    {
        $valid = [
            'cp.company' => 'required|max:50',
            'cp.serial_no' => 'required|max:12|unique:parameter,serial_no,' . $this->idNya . ',serial_no',
            'cp.address' => 'required',
            'cp.contact' => 'required|max:50',
            'cp.tax_number' => 'required|max:18',
            'cp.period' => 'required|numeric|between:1,13',
            'cp.year' => 'required|numeric|between:2000,2050',
            'cp.mask_account' => 'required|max:20',
            'cp.base_currency' => 'required|max:5',
            'cp.control_date' => 'required',
        ];

        $data = Parameter::first();

        if ($data) {
            $this->validate($valid);
            $data->update($this->cp);
        } else {
            $this->validate($valid);
            Parameter::create($this->cp);
        }

        $newYear = (int) $data->year;

        // Ambil tiga data pertama berdasarkan period_code
        $periods = Period::whereIn('period_code', [1, 2, 3])->get();

        foreach ($periods as $period) {
            // Tentukan tahun berdasarkan period_code
            if ($period->period_code == 1) {
                $year = $newYear;
            } elseif ($period->period_code == 2) {
                $year = $newYear - 1;
            } elseif ($period->period_code == 3) {
                $year = $newYear - 2;
            }

            // Update period_name
            $period->period_name = $year;

            // Loop untuk update tahun di setiap bulan
            for ($i = 1; $i <= 13; $i++) { // dan bulan yang ke 13 namanya Adj
                if ($i == 13) {
                    // Bulan ke-13 sebagai "Adj"
                    $monthName = "Adj " . $year;
                    $monthCode = "13"; // Format kode untuk bulan ke-13
                    $fromDate = Carbon::create($year, $i - 1, 1)->format('Y-m-d');
                    $toDate = Carbon::create($year, $i - 1, 1)->endOfMonth()->format('Y-m-d');
                } else {
                    $month = str_pad($i, 2, '0', STR_PAD_LEFT); // Format angka bulan (01-12)
                    $monthName = Carbon::create($year, $i, 1)->format('M Y');
                    $monthCode = $month;
                    $fromDate = Carbon::create($year, $i, 1)->format('Y-m-d');
                    $toDate = Carbon::create($year, $i, 1)->endOfMonth()->format('Y-m-d');
                }

                // Simpan format "Nama Bulan Tahun" (contoh: Jan 2025, Adj 2025)
                $period->{"p$monthCode"} = $monthName;

                // Update kolom tanggal mulai (`pXX_from`) dan tanggal akhir (`pXX_to`)
                $period->{"p{$monthCode}_from"} = $fromDate;
                $period->{"p{$monthCode}_to"} = $toDate;
            }

            // Simpan perubahan ke database
            $period->save();
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
        $this->btnedit();
    }

    public function btnedit()
    {
        $this->edit = !$this->edit;
        $this->centangCheck($this->cp['control_date'] ?? 0);
        $this->disableEnableCheck();
        $this->getData();
    }

    public function render()
    {
        $this->getData();
        return view('livewire.pages.company-parameter.company-parameter');
    }
}
