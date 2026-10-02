<?php

namespace App\Livewire\Pages\AccountGroup;

use Livewire\Component;
use Livewire\WithPagination;
use App\Models\AccountSubGroupTable;
use App\Models\AccountGroupTable;
use Illuminate\Support\Facades\Crypt;

class AccountSubGroupPages extends Component
{
    use WithPagination;

    public $form = false;
    public $sub_group = [];
    public $search = '';
    public $order = 'ASC';
    public $limit = 30;
    public $idNya;
    public $accountGroupOptions = [];

    protected $rules = [
        'sub_group.name' => 'required|string|max:255',
        'sub_group.account_group_table_id' => 'required|exists:account_group_tables,id',
    ];

    public function mount()
    {
        $this->accountGroupOptions = AccountGroupTable::pluck('name', 'id')->toArray();
    }

    public function render()
    {
        $data = AccountSubGroupTable::where('name', 'like', '%' . $this->search . '%')
            ->orderBy('created_at', $this->order)
            ->paginate($this->limit);

        return view('livewire.pages.account-group.account-sub-group-pages', [
            'data' => $data,
        ]);
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
        $user = AccountSubGroupTable::find($this->idNya);
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

    public function add($id = null)
    {
        $this->js(<<<'JS'
            $(document).ready(function() {
                $('.select-search').select2();
                        
                $('.select-search').on('change', function (e) {
                    var data = $('.select-search').select2("val");
                    console.log(data);
                    $wire.set('sub_group.account_group_table_id', data);
                });
               
            });
        JS);
        if ($id) {
            $this->idNya = Crypt::decrypt($id);
            $this->sub_group = AccountSubGroupTable::find($this->idNya)->toArray();
        } else {
            $this->idNya = null;
            $this->sub_group = ['name' => '', 'account_group_table_id' => '']; // Initialize with default values
        }
        $this->form = true;
    }

    public function save()
    {
        $this->validate();

        if ($this->idNya) {
            $subGroup = AccountSubGroupTable::find($this->idNya);
            $subGroup->update([
                'name' => $this->sub_group['name'],
                'account_group_table_id' => $this->sub_group['account_group_table_id'],
            ]);
        } else {
            AccountSubGroupTable::create([
                'name' => $this->sub_group['name'],
                'account_group_table_id' => $this->sub_group['account_group_table_id'],
            ]);
        }

        $this->resetForm();

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
        $this->resetForm();
    }

    private function resetForm()
    {
        $this->form = false;
        $this->sub_group = ['name' => '', 'account_group_table_id' => '']; // Reset to default
        $this->idNya = null;
    }

    public function edit($id)
    {
        $this->add($id);
    }

    public function sort($order)
    {
        $this->order = $order;
    }

    public function limits($limit)
    {
        $this->limit = $limit;
    }
}
