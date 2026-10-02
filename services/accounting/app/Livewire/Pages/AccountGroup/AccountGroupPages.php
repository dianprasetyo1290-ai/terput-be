<?php

namespace App\Livewire\Pages\AccountGroup;

use Livewire\Component;
use Livewire\WithPagination;
use App\Models\AccountGroupTable; // Make sure this is the correct model namespace
use Illuminate\Support\Facades\Crypt;

class AccountGroupPages extends Component
{
    use WithPagination;

    public $form = false;
    public $account_group = [];
    public $search = '';
    public $order = 'ASC';
    public $limit = 30;
    public $idNya;

    protected $rules = [
        'account_group.name' => 'required|string|max:255',
    ];

    public function render()
    {
        $data = AccountGroupTable::where('name', 'like', '%' . $this->search . '%')
            ->orderBy('created_at', $this->order)
            ->paginate($this->limit);

        return view('livewire.pages.account-group.account-group-pages', [
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
        $user = AccountGroupTable::find($this->idNya);
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
        if ($id) {
            $this->idNya = Crypt::decrypt($id);
            $this->account_group = AccountGroupTable::find($this->idNya)->toArray();
        } else {
            $this->idNya = null;
            $this->account_group = ['name' => '']; // Initialize with default values
        }
        $this->form = true;
    }

    public function save()
    {
        // Validasi dengan aturan unik, kecuali jika ada idNya
        $this->validate([
            'account_group.name' => [
                'required',
                'string',
                'max:255',
            ],
        ]);

        if ($this->idNya) {
            $accountGroup = AccountGroupTable::find($this->idNya);
            $accountGroup->update([
                'name' => $this->account_group['name'],
            ]);
        } else {
            AccountGroupTable::create([
                'name' => $this->account_group['name'],
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
        $this->account_group = ['name' => '']; // Reset to default
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
