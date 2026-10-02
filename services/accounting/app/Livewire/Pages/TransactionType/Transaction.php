<?php

namespace App\Livewire\Pages\TransactionType;

use App\Models\ChartOfAccount;
use App\Models\ComCodes;
use Livewire\Component;
use Livewire\WithPagination;
use App\Models\TransactionType;
use Illuminate\Support\Facades\Crypt;

class Transaction extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';

    public $form, $idNya;
    public $transaction = [
        'trans_code' => null,
        'trans_name' => null,
        'trans_status' => null,
        'trans_account' => null,
    ];
    public $search, $order = "ASC", $limit = 10;

    public function add($id = "")
    {
        $this->js(<<<'JS'
            $(document).ready(function() {
                $('.select-search').select2();
                $('.select-search2').select2();
                        
                $('.select-search').on('change', function (e) {
                    var data = $('.select-search').select2("val");
                    console.log(data);
                    $wire.set('transaction.trans_status', data);
                });
                $('.select-search2').on('change', function (e) {
                    var data = $('.select-search2').select2("val");
                    console.log(data);
                    $wire.set('transaction.trans_account', data);
                });
            });
        JS);

        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $this->transaction = TransactionType::find($this->idNya);
            $this->transaction = $this->transaction->toArray();
            $this->resetValidation();
        } else {
            $this->form = true;
        }
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->transaction = [
            'trans_code' => null,
            'trans_name' => null,
            'trans_status' => null,
            'trans_account' => null,
        ];
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
        $user = TransactionType::find($this->idNya);
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

    public function save()
    {
        $valid = [
            'transaction.trans_code' => 'required|unique:mst_transaction,trans_code,' . $this->idNya . ',trans_code|max:5',
            'transaction.trans_name' => 'required',
            'transaction.trans_status' => 'required',
        ];
        if ($this->idNya) {
            $this->validate($valid);
            $transaction = TransactionType::find($this->idNya);
            $transaction->update($this->transaction);
        } else {
            $this->validate($valid);
            TransactionType::create($this->transaction);
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
        $data = TransactionType::query();
        if ($this->search) {
            $data->where('trans_code', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('trans_name', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('trans_status', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('trans_account', 'ilike', '%' .  $this->search . '%');
        }
        if ($this->order) {
            $data->orderBy('trans_code', $this->order);
        }
        $data = $data->paginate($this->limit);
        $transaction_code = ComCodes::where('code_group', 'TRANSACTION')->get();
        $m_account = ChartOfAccount::orderBy('acc_name', 'ASC')->get();
        return view('livewire.pages.transaction-type.transaction', [
            'data' => $data,
            'transaction_code' => $transaction_code,
            'm_account' => $m_account
        ]);
    }
}
