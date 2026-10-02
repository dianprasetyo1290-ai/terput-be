<?php

namespace App\Livewire\Pages\ChartOfAccount;

use App\Models\AccountGroupTable;
use App\Models\AccountSubGroupTable;
use App\Models\ChartOfAccount as ModelsChartOfAccount;
use App\Models\Currency;
use Illuminate\Support\Facades\Crypt;
use Livewire\Component;
use Livewire\WithPagination;

class ChartOfAccount extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';

    public $listSubGroup = [];
    public $form, $idNya;
    public $coa = [
        'acc_code' => null,
        'acc_name' => null,
        'acc_group' => null,
        'sub_acc_group' => null,
        'acc_currency' => null,
    ];
    public $search, $order = "ASC", $limit = 10;

    public function updatedCoaAccGroup($value = '')
    {
        // Ambil data sub-group berdasarkan grup yang dipilih
        $this->listSubGroup = AccountSubGroupTable::where('account_group_table_id', $value)->get();
        // $this->dispatch('subGroupUpdated');
        $data = json_encode($this->listSubGroup);

        $this->js(<<<JS
                $('#OutletId').select2({ allowClear: true });
                var selectElement = document.getElementById('OutletId');
                // Remove all options
                selectElement.innerHTML = '';

                // Add new options
                var newOptions = $data;

                var newOption2 = document.createElement('option');
                newOption2.value = '';
                newOption2.text = 'Pilih';
                selectElement.add(newOption2);
                newOptions.forEach(function(option) {
                    var newOption = document.createElement('option');
                    newOption.value = option.id;
                    newOption.text = option.name;
                    selectElement.add(newOption);
                });
        JS);
    }

    public function updatedSearch()
    {
        $this->resetPage();
    }

    public function add($id = "")
    {
        $this->js(<<<'JS'
            $(document).ready(function() {
                $('.select-search').select2();
                $('.select-search2').select2();
                $('.select-search3').select2();
                        
                $('.select-search').on('change', function (e) {
                    var data = $('.select-search').select2("val");
                    console.log(data);
                    $wire.set('coa.acc_group', data);
                });
                $('.select-search2').on('change', function (e) {
                    var data = $('.select-search2').select2("val");
                    console.log(data);
                    $wire.set('coa.sub_acc_group', data);
                });
                $('.select-search3').on('change', function (e) {
                    var data = $('.select-search3').select2("val");
                    console.log(data);
                    $wire.set('coa.acc_currency', data);
                });
            });
        JS);

        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $this->coa = ModelsChartOfAccount::find($this->idNya);
            $this->listSubGroup = AccountSubGroupTable::where('account_group_table_id', $this->coa['acc_group'])->get();
            $this->coa = $this->coa->toArray();
            $this->resetValidation();
        } else {
            $this->form = true;
        }
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->coa = [
            'acc_code' => null,
            'acc_name' => null,
            'acc_group' => null,
            'sub_acc_group' => null,
            'acc_currency' => null,
        ];
    }

    public function save()
    {
        $valid = [
            'coa.acc_code' => 'required|unique:mst_account,acc_code|max:15',
            'coa.acc_name' => 'required|max:50',
            'coa.acc_group' => 'required',
        ];
        if ($this->idNya) {
            ModelsChartOfAccount::find($this->idNya)->delete();
            $this->validate($valid);
            ModelsChartOfAccount::create($this->coa);
        } else {
            $this->validate($valid);
            ModelsChartOfAccount::create($this->coa);
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
        $user = ModelsChartOfAccount::find($this->idNya);
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

    public function limits($limit)
    {
        $this->resetPage();
        $this->limit = $limit;
    }

    public function render()
    {
        $data = ModelsChartOfAccount::with('group', 'sub_group');
        if ($this->search) {
            $data->where('acc_code', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('acc_name', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('acc_group', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('acc_currency', 'ilike', '%' .  $this->search . '%');
        }
        if ($this->order) {
            $data->orderBy('acc_code', $this->order);
        }
        $data = $data->paginate($this->limit);
        $listGroup = AccountGroupTable::all();

        $currency = Currency::orderBy('curr_code', 'ASC')->get();
        return view('livewire.pages.chart-of-account.chart-of-account', [
            'data' => $data,
            'listGroup' => $listGroup,
            'listSubGroup' => $this->listSubGroup,
            'currency' => $currency
        ]);
    }
}
