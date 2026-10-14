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
        // Opened bare (no ?foo= from the Peran list) → back to Peran, not a 500.
        if (!$this->foo) {
            $this->redirectRoute('role');

            return;
        }
        try {
            $this->role_id = Crypt::decrypt($this->foo);
        } catch (\Illuminate\Contracts\Encryption\DecryptException) {
            $this->redirectRoute('role');
        }
    }

    /**
     * Who may change which permission (QA round 2): approving a period
     * reopen (editperiod_reopen) is the checker half of maker-checker, so
     * only a SuperAdmin may grant or revoke it; the SuperAdmin role itself
     * (all permissions via Gate::before) is only edited by a SuperAdmin.
     */
    private function editableRole(): ?Role
    {
        $this->authorize('editpermission');
        $role = Role::find($this->role_id);
        if (! $role) {
            return null;
        }
        if ($role->name === 'SuperAdmin' && ! auth()->user()?->hasRole('SuperAdmin')) {
            throw new \Illuminate\Auth\Access\AuthorizationException('Hanya SuperAdmin yang dapat mengubah izin peran SuperAdmin.');
        }

        return $role;
    }

    private static function superAdminOnly(string $permission): bool
    {
        return $permission === \App\Support\AccountingRoles::REOPEN_APPROVE;
    }

    private function mayToggle(string $permission): bool
    {
        return ! self::superAdminOnly($permission) || (bool) auth()->user()?->hasRole('SuperAdmin');
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
        $role = $this->editableRole();
        if (! $role) {
            return;
        }
        $permission_user = $role->getPermissionNames()->toArray();
        $permission = $action . '' . $module;
        if (! $this->mayToggle($permission)) {
            $this->js("Swal.fire({icon: 'error', title: 'Izin ini hanya dapat diubah oleh SuperAdmin.'})");

            return;
        }

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
        $role = $this->editableRole();
        if (! $role) {
            return;
        }
        $wanted = [];
        foreach ($this->permission_user as $module => $actions) {
            foreach ((array) $actions as $action => $value) {
                if ($value) {
                    $wanted[] = $action . $module;
                }
            }
        }
        // A SuperAdmin-only permission keeps its current state unless a
        // SuperAdmin is saving.
        $current = $role->getPermissionNames()->all();
        $wanted = array_values(array_filter($wanted, fn ($p) => $this->mayToggle($p)));
        foreach ($current as $p) {
            if (! $this->mayToggle($p)) {
                $wanted[] = $p;
            }
        }
        $wanted = array_values(array_unique(array_intersect($wanted, Permission::pluck('name')->all())));
        $role->syncPermissions($wanted);
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
    /** Close the add-permission modal (Cancel / x buttons). */
    public function cancel()
    {
        $this->name = null;
        $this->resetValidation();
        $this->js("$('#add-modal').modal('hide'); $('#add-department').modal('hide');");
    }

    public function closeViewModalFileKet()
    {
        $this->cancel();
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
        $role = Role::find($this->role_id);
        if (! $role) {
            return '<div></div>'; // redirecting back to Peran
        }
        $permissions = $role->getPermissionNames()->toArray();
        return view('livewire.pages.role-and-permission.permissions', ['modules' => $modules, 'permissions' => $permissions, 'role' => $role]);
    }
}
