<?php

namespace App\Livewire\Pages\RoleAndPermission;

use App\Models\Modul;
use Illuminate\Support\Facades\Crypt;
use Livewire\Component;
use Livewire\WithPagination;
use Spatie\Permission\Models\Permission;
use Spatie\Permission\Models\Role;

class Permissions extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';
    public $role_id, $foo, $idPermission, $name;
    public $permission_user = [];
    public $selectedRole, $searchRole, $search, $order = "ASC", $limit = 10, $idNya;
    protected $queryString = ['foo' => ['except' => '', 'as' => 'foo']];
    public function mount()
    {
        $this->role_id = Crypt::decrypt($this->foo);
    }
    public function limits($limit)
    {
        $this->limit = $limit;
    }

    public function sort($order)
    {
        $this->order = $order;
    }
    public function abc($module, $action)
    {
        $role = Role::find($this->role_id);
        $permission_user = $role->getPermissionNames()->toArray();
        $permission = $action . '' . $module;

        // $role->syncPermissions($permission);
        if (in_array($permission, $permission_user)) {
            $role->revokePermissionTo($permission);
        } else {
            $role->givePermissionTo($permission);
        }
        $this->js(
            <<<'js'
            Swal.fire({
                position: "top-end",
                icon: "success",
                title: "Permission Update",
                showConfirmButton: false,
                timer: 1000
              });
            js
        );
    }
    public function add($id = '')
    {
        if ($id) {
        } else {
            $this->js(
                <<<'js'
                $('#add-modal').modal('toggle')
                js
            );
        }
    }
    public function simpan()
    {
        $role = Role::find($this->role_id);
        $role->syncPermissions([]);
        foreach ($this->permission_user as $module => $actions) {
            foreach ($actions as $action => $value) {

                if ($value) {
                    $permission =  $action . $module;
                    $role->givePermissionTo($permission);
                }
            }
        }
        $this->js(
            <<<'js'
            Swal.fire({
                position: "top-end",
                icon: "success",
                title: "Permission Update",
                showConfirmButton: false,
                timer: 1000
              });
            js
        );
    }
    public function save()
    {
        $this->validate([
            'name' => 'required|max:255',
        ]);
        $value = $this->name;
        $display_name = ucwords(strtolower($value));
        $name = strtolower($value);
        $name = str_replace(' ', '_', $name);
        Modul::create(['name' => $name, 'display_name' => $display_name]);
        $actions = ['create', 'edit', 'delete', 'view'];

        foreach ($actions as $action) {
            Permission::create(['name' => "$action$name", 'guard_name' => "web"]);
        }
        $this->js(
            <<<'js'
            Swal.fire({
                position: "top-end",
                icon: "success",
                title: "Permission Added",
                showConfirmButton: false,
                timer: 1000
              });
            js
        );
        $this->name = null;
        $this->js(
            <<<'js'
            $('#add-modal').modal('hide');
            js
        );
    }
    public function render()
    {
        $m = Modul::query();
        if ($this->search) {
            $m->where('name', 'ilike', '%' . $this->search . '%');
        }
        $modules = $m->orderBy('id', $this->order)->get();
        $role = Role::find($this->role_id); // Assuming role with ID 1 exists
        $permissions = $role->getPermissionNames()->toArray(); // Convert permissions to array
        return view('livewire.pages.role-and-permission.permissions', ['modules' => $modules, 'permissions' => $permissions, 'role' => $role]);
    }
}
