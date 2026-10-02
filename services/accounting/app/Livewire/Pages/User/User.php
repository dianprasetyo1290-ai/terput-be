<?php

namespace App\Livewire\Pages\User;

use App\Models\User as ModelsUser;
use Illuminate\Support\Facades\Crypt;
use Livewire\Component;
use Livewire\WithFileUploads;
use Livewire\WithPagination;
use Spatie\Permission\Models\Role;

class User extends Component
{
    use WithPagination, WithFileUploads;
    protected $paginationTheme = 'bootstrap';

    public $form, $idNya, $role;
    public $user = [
        'user_name' => null,
        'email' => null,
        'password' => null,
    ];
    public $search, $order = "ASC", $limit = 10;


    public function add($id = "")
    {
        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $this->user = ModelsUser::where('user_id', $this->idNya)->first();
            $this->role = $this->user->getRoleNames()->first();
            $this->user = $this->user->toArray();
            $this->resetValidation();
        } else {
            $this->form = true;
        }
    }
    function generateNewUserId()
    {
        $lastUser = ModelsUser::orderBy('user_id', 'desc')->first();
        if ($lastUser) {
            $lastId = intval($lastUser->user_id);
            $newId = $lastId + 1;
        } else {
            $newId = 1;
        }
        return str_pad($newId, 4, '0', STR_PAD_LEFT);
    }
    public function save()
    {
        $valid = [
            'user.user_name' => 'required',
            'role' => 'required'
        ];
        if ($this->idNya) {
            $valid['user.email'] = 'required|email|unique:mst_user,email,' . $this->idNya . ',user_id';
            $this->validate($valid);

            $user = ModelsUser::where('user_id', $this->idNya)->firstOrFail();
            $user->update($this->user);
            $user->syncRoles($this->role);
        } else {
            $valid['user.email'] = 'required|email|unique:mst_user,email';
            $valid['user.password'] = 'required';
            $this->validate($valid);

            $userData = $this->user;
            $userData['user_id'] = $this->generateNewUserId();
            $user = ModelsUser::create($userData);
            $user->syncRoles($this->role);
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
        $user = ModelsUser::find($this->idNya);
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
        $this->order = $order;
    }

    public function limits($limit)
    {
        $this->limit = $limit;
    }

    public function suspend($id)
    {

        $c = ModelsUser::find(Crypt::decrypt($id));
        $this->idNya = $c->user_id;
        if ($c->actived == 0) {
            $this->js(<<<'JS'
                    Swal.fire({
                        title: 'Apakah Anda yakin?',
                            text: "Apakah Anda yakin akan mengaktifkan User ini ?",
                            type: 'warning',
                            showCancelButton: true,
                            confirmButtonColor: '#3085d6',
                            cancelButtonColor: '#d33',
                            confirmButtonText: 'Aktived!',
                            cancelButtonText: 'Batal'
                      }).then((result) => {
                        if (result.isConfirmed) {
                            $wire.confirm()
                        }
                      })
                    JS);
        } else {
            $this->js(<<<'JS'
                    Swal.fire({
                        title: 'Apakah Anda yakin?',
                            text: "Apakah Anda yakin akan menangguhkan User ini ?",
                            type: 'warning',
                            showCancelButton: true,
                            confirmButtonColor: '#3085d6',
                            cancelButtonColor: '#d33',
                            confirmButtonText: 'Suspend!',
                            cancelButtonText: 'Batal'
                      }).then((result) => {
                        if (result.isConfirmed) {
                            $wire.confirm()
                        }
                      })
                    JS);
        }
    }
    public function confirm()
    {
        $c = ModelsUser::find($this->idNya);
        if ($c->actived == 0) {
            $c->actived = 1;
            $c->save();
        } else {
            $c->actived = 0;
            $c->save();
        }
        $this->clear();
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->role = null;
        $this->user = [
            'user_name' => null,
            'email' => null,
            'password' => null,
        ];
    }
    public function render()
    {
        $data = ModelsUser::query();
        if ($this->search) {
            $data->where('user_name', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('email', 'ilike', '%' .  $this->search . '%');
        }
        if ($this->order) {
            $data->orderBy('user_id', $this->order);
        }
        $data = $data->paginate($this->limit);
        $listRole = Role::pluck('name', 'name');
        return view('livewire.pages.user.user', [
            'data' => $data,
            'listRole' => $listRole
        ]);
    }
}
