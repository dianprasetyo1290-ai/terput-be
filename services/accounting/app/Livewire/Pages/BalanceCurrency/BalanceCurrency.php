<?php

namespace App\Livewire\Pages\BalanceCurrency;

use App\Models\BalanceCurrency as ModelsBalanceCurrency;
use App\Models\ChartOfAccount;
use App\Models\Currency;
use App\Models\Division;
use Illuminate\Support\Facades\Crypt;
use Livewire\Component;
use Livewire\WithPagination;

class BalanceCurrency extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';
    public $search, $order = "ASC", $limit = 10;

    public $form, $idNya, $lihat;
    public $saldo;

    public function mount()
    {
        $this->saldo = [
            'acc_code' => null,
            'div_code' => null,
            'curr_code' => null,
            'act_00' => null,
            'actk_00' => null,
            'lst_00' => null,
            'lstk_00' => null,
            'snd_00' => null,
            'sndk_00' => null
        ];

        foreach (range(1, 13) as $month) {
            $monthKey = sprintf('%02d', $month);
            $this->saldo['act_' . $monthKey] = null;
            $this->saldo['actk_' . $monthKey] = null;
            $this->saldo['lst_' . $monthKey] = null;
            $this->saldo['lstk_' . $monthKey] = null;
            $this->saldo['snd_' . $monthKey] = null;
            $this->saldo['sndk_' . $monthKey] = null;
        }
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
          setTimeout(function() {
             $('.select-search').select2();
            }, 500);
          JS);
        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $this->saldo = ModelsBalanceCurrency::find($this->idNya);
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
        $this->mount();
    }

    public function view($id)
    {
        $this->form = true;
        $this->lihat = true;
        $this->idNya = Crypt::decrypt($id);
        $this->saldo = ModelsBalanceCurrency::find($this->idNya);
        $this->saldo = $this->saldo->toArray();
        $this->resetValidation();
    }
    public function save()
    {
        $valid = [
            'saldo.acc_code' => 'required',
            'saldo.div_code' => 'required',
            'saldo.curr_code' => 'required',
        ];

        foreach (range(1, 13) as $month) {
            $monthKey = sprintf('%02d', $month);
            $valid['saldo.act_' . $monthKey] = 'nullable|numeric';
            $valid['saldo.lst_' . $monthKey] = 'nullable|numeric';
            $valid['saldo.fut_' . $monthKey] = 'nullable|numeric';
        }

        $this->validate($valid);

        if ($this->idNya) {
            $saldo = ModelsBalanceCurrency::find($this->idNya);
            $saldo->update($this->saldo);
        } else {
            ModelsBalanceCurrency::create($this->saldo);
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
        $data = ModelsBalanceCurrency::query();
        if ($this->search) {
            $data->where('curr_code', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('curr_name', 'ilike', '%' .  $this->search . '%');
        }
        if ($this->order) {
            $data->orderBy('id', $this->order);
        }
        $data = $data->paginate($this->limit);
        $account = ChartOfAccount::get();
        $division = Division::get();
        $currency = Currency::get();
        return view('livewire.pages.balance-currency.balance-currency', [
            'data' => $data,
            'account' => $account,
            'division' => $division,
            'currency' => $currency
        ]);
    }
}
