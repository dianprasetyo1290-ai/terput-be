<?php

namespace App\Livewire\Pages\RoleAndPermission;

use Illuminate\Support\Facades\Crypt;
use Livewire\Component;
use Livewire\WithPagination;
use Spatie\Permission\Models\Role as ModelsRole;

class Role extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';
    public $order = "ASC", $limit = 10, $search, $idNya;

    public $formRole = [
        'name' => '',
        'guard_name' => 'web'

    ];

    public $form;
    public function limits($limit)
    {
        $this->limit = $limit;
    }

    public function sort($order)
    {
        $this->order = $order;
    }
    public function add($id = "")
    {
        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $p = ModelsRole::find($this->idNya);
            $this->formRole = $p->toArray();
            $this->resetValidation();
        } else {
            $this->form = true;
        }
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
                denyButtonText: 'clear'
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
        $user = ModelsRole::find($this->idNya);
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
    public function clear()
    {
        $this->form = false;
        $this->formRole = [
            'name' => '',
            'guard_name' => 'web'
        ];
        $this->idNya = '';
    }

    public function save()
    {
        if ($this->idNya) {
            $role = ModelsRole::find($this->idNya);
            $role->update($this->formRole);
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
                title: "Data saved successfully"
            });
        JS);
        } else {
            ModelsRole::create($this->formRole);
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
                title: "Data saved successfully"
            });
        JS);
        }
    }
    public function render()
    {
        $data = ModelsRole::query();
        if ($this->search) {
            $data->where('name', 'ilike', '%' . $this->search . '%');
        }
        $data = $data->orderBy('id', $this->order)->paginate($this->limit);
        return view('livewire.pages.role-and-permission.role', ['data' => $data]);
    }
}
