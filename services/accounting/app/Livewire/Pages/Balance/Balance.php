<?php

namespace App\Livewire\Pages\Balance;

use App\Models\Balance as ModelsBalance;
use App\Models\ChartOfAccount;
use App\Models\Division;
use App\Models\Parameter;
use App\Models\View\VwBalance;
use Illuminate\Support\Facades\Crypt;
use Illuminate\Support\Facades\DB;
use Livewire\Component;
use Livewire\WithPagination;

class Balance extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';
    public $search, $order = "ASC", $limit = 10;

    public $form = false, $idNya, $lihat;
    public $saldo;
    public $acc_code;
    public $div_code;
    public $tahun;
    public $act = [];
    public $actk = [];
    public $lst = [];
    public $lstk = [];
    public $snd = [];
    public $sndk = [];
    public $perPage = 10;

    public function mount()
    {
        $this->tahun =  Parameter::first()->year;
    }
    public function sort($order)
    {
        $this->resetPage();
        $this->order = $order;
    }

    public function updatedSearch()
    {
        $this->resetPage();
    }

    public function limits($limit)
    {
        $this->resetPage();
        $this->limit = $limit;
    }

    public function add($id = "")
    {
        $this->js(<<<'JS'
            $(document).ready(function() {
                $('.select-search').select2();
                $('.select-search2').select2();
                        
                $('.select-search').on('change', function (e) {
                    var data = $('.select-search').select2("val");
                    $wire.set('acc_code', data);
                });
                $('.select-search2').on('change', function (e) {
                    var data = $('.select-search2').select2("val");
                    $wire.set('div_code', data);
                });
            });
        JS);
        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $this->saldo = ModelsBalance::find($this->idNya);
            $this->saldo = $this->saldo->toArray();
            $this->resetValidation();
        } else {
            $this->form = true;
        }
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->lihat = false;
    }

    public function view($acc_code, $div_code)
    {
        $this->form = true;
        $this->lihat = true;
        $this->resetValidation();
        $this->acc_code = $acc_code;
        $this->div_code = $div_code;

        // $this->idNya = Crypt::decrypt($id);
        // $this->saldo = ModelsBalance::where($this->idNya);
        // $this->saldo = $this->saldo->toArray();
        // $this->resetValidation();
    }
    public function save()
    {
        $valid = [
            'saldo.acc_code' => 'required',
            'saldo.div_code' => 'required',
        ];

        foreach (range(1, 13) as $month) {
            $monthKey = sprintf('%02d', $month);
            $valid['saldo.act_' . $monthKey] = 'nullable|numeric';
            $valid['saldo.lst_' . $monthKey] = 'nullable|numeric';
            $valid['saldo.fut_' . $monthKey] = 'nullable|numeric';
        }

        $this->validate($valid);

        if ($this->idNya) {
            $saldo = ModelsBalance::find($this->idNya);
            $saldo->update($this->saldo);
        } else {
            ModelsBalance::create($this->saldo);
        }

        $this->reset();

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
    public function render()
    {
        $data = VwBalance::query();
        // ->paginate(10);

        if ($this->search) {
            $data->where('acc_code', 'ilike', '%' . $this->search . '%')
                ->orWhere('acc_name', 'ilike', '%' . $this->search . '%')
                ->orWhere('div_code', 'ilike', '%' . $this->search . '%')
                ->orWhere('div_name', 'ilike', '%' . $this->search . '%');
        }

        // $data = $data->get();
        // dd($data[0]);
        // if ($this->order) {
        //     $data->orderBy('id', $this->order);
        // }
        $data = $data->paginate($this->limit);
        $account = ChartOfAccount::get();
        $division = Division::get();
        return view('livewire.pages.balance.balance', [
            'data' => $data,
            'account' => $account,
            'division' => $division,
        ]);
    }
}
