<?php

namespace App\Livewire\Pages\Currency;

use App\Models\ComCodes;
use App\Models\Currency;
use Illuminate\Support\Facades\Crypt;
use Livewire\Component;
use Livewire\WithPagination;

class CurrencyTable extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';

    public $form, $idNya, $lihat;
    public $currency = [
        'curr_code' => null,
        'curr_name' => null,
        'rate_system' => null,
        'act_01' => null,
        'act_02' => null,
        'act_03' => null,
        'act_04' => null,
        'act_05' => null,
        'act_06' => null,
        'act_07' => null,
        'act_08' => null,
        'act_09' => null,
        'act_10' => null,
        'act_11' => null,
        'act_12' => null,
        'act_13' => null,
        'lst_01' => null,
        'lst_02' => null,
        'lst_03' => null,
        'lst_04' => null,
        'lst_05' => null,
        'lst_06' => null,
        'lst_07' => null,
        'lst_08' => null,
        'lst_09' => null,
        'lst_10' => null,
        'lst_11' => null,
        'lst_12' => null,
        'lst_13' => null
    ];

    public $search, $order = "ASC", $limit = 10;

    public function add($id = "")
    {
        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $this->currency = Currency::find($this->idNya);
            $this->currency = $this->currency->toArray();
            $this->resetValidation();
        } else {
            $this->form = true;
        }
    }

    public function view($id)
    {
        $this->form = true;
        $this->lihat = true;
        $this->idNya = Crypt::decrypt($id);
        $this->currency = Currency::find($this->idNya);
        $this->currency = $this->currency->toArray();
        $this->resetValidation();
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->lihat = false;
        $this->currency = [
            'curr_code' => null,
            'curr_name' => null,
            'rate_system' => null,
            'act_01' => null,
            'act_02' => null,
            'act_03' => null,
            'act_04' => null,
            'act_05' => null,
            'act_06' => null,
            'act_07' => null,
            'act_08' => null,
            'act_09' => null,
            'act_10' => null,
            'act_11' => null,
            'act_12' => null,
            'act_13' => null,
            'lst_01' => null,
            'lst_02' => null,
            'lst_03' => null,
            'lst_04' => null,
            'lst_05' => null,
            'lst_06' => null,
            'lst_07' => null,
            'lst_08' => null,
            'lst_09' => null,
            'lst_10' => null,
            'lst_11' => null,
            'lst_12' => null,
            'lst_13' => null
        ];
    }

    public function save()
    {
        $valid = [
            'currency.curr_name' => 'required|max:50',
            'currency.rate_system' => 'required|string|max:1'
        ];
        if ($this->idNya) {
            $this->validate($valid);
            $currency = Currency::find($this->idNya);
            $currency->update($this->currency);
        } else {
            $valid = [
                'currency.curr_code' => 'required|unique:mst_currency,curr_code|max:5',
            ];
            $this->validate($valid);
            Currency::create($this->currency);
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

    public function confirmDelete($id)
    {
        $this->idNya = Crypt::decrypt($id);
        $this->js(<<<'JS'
        Swal.fire({
                text: "Do you want to delete this data? This process is irreversible.",
                icon: 'warning',
                showDenyButton: true,
                confirmButtonColor: '#3085d6',
                confirmButtonText: 'Delete!',
                denyButtonText: 'Cancel'
          }).then((result) => {
            if (result.isConfirmed) {
                $wire.delete()
            } else if (result.isDenied) {
                $wire.clear()
            }
          })
        JS);
    }

    public function delete()
    {
        $user = Currency::find($this->idNya);
        $user->delete();
        $this->clear();
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
            title: "Data deleted successfully"
            });
        JS);
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

    public function render()
    {
        $data = Currency::query();
        if ($this->search) {
            $data->where('curr_code', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('curr_name', 'ilike', '%' .  $this->search . '%');
        }
        if ($this->order) {
            $data->orderBy('curr_code', $this->order);
        }
        $data = $data->paginate($this->limit);
        $radio = ComCodes::where('code_group', 'RATE_SYSTEM')->get();
        return view('livewire.pages.currency.currency-table', [
            'data' => $data,
            'radio' => $radio
        ]);
    }
}
