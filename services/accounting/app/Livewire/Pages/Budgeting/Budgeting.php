<?php

namespace App\Livewire\Pages\Budgeting;

use App\Models\Budgeting as ModelsBudgeting;
use App\Models\ChartOfAccount;
use App\Models\Division;
use Illuminate\Support\Facades\Crypt;
use Livewire\Component;
use Livewire\WithPagination;

class Budgeting extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';
    public $search, $order = "ASC", $limit = 10;

    public $form, $idNya, $lihat;
    public $budget;

    public function mount()
    {
        $this->budget = [
            'acc_code' => null,
            'div_code' => null,
        ];

        foreach (range(1, 13) as $month) {
            $monthKey = sprintf('%02d', $month);
            $this->budget['act_' . $monthKey] = null;
            $this->budget['lst_' . $monthKey] = null;
            $this->budget['fut_' . $monthKey] = null;
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
            $this->idNya = Crypt::decrypt($id);
            $this->budget = ModelsBudgeting::find($this->idNya);
            $this->budget = $this->budget->toArray();
            $this->resetValidation();
        } else {
            $this->form = true;
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
        $this->budget = ModelsBudgeting::find($this->idNya);
        $this->budget = $this->budget->toArray();
        $this->resetValidation();
    }
    public function save()
    {
        $valid = [
            'budget.acc_code' => 'required',
            'budget.div_code' => 'required',
        ];

        foreach (range(1, 13) as $month) {
            $monthKey = sprintf('%02d', $month);
            $valid['budget.act_' . $monthKey] = 'nullable|numeric';
            $valid['budget.lst_' . $monthKey] = 'nullable|numeric';
            $valid['budget.fut_' . $monthKey] = 'nullable|numeric';
        }

        $this->validate($valid);

        if ($this->idNya) {
            $budget = ModelsBudgeting::find($this->idNya);
            $budget->update($this->budget);
        } else {
            ModelsBudgeting::create($this->budget);
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
        $data = ModelsBudgeting::query();
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
        return view('livewire.pages.budgeting.budgeting', [
            'data' => $data,
            'account' => $account,
            'division' => $division,
        ]);
    }
}
