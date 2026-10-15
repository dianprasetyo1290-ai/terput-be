<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Permission" subjudul="Manage Permission" :breadcrumb="['Master File', 'User Management', 'Permission']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Role Permission User</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>
        <div class="card-body">
            @can('createpermission')
                <div wire:loading.remove wire:target='add'>
                    <button type="button" class="btn btn-primary" wire:click="add">Tambah Permission<i
                            class="icon-user-plus ml-2"></i></button>
                </div>
                <div wire:loading wire:target="add">
                    <button type="button" class="btn bg-slate-600" disabled>Saving...<i
                            class="icon-spinner2 spinner"></i></button>
                </div>
            @endcan
            <div class="card-header">
                <div class="col-12 row">
                    <div class="search-set">
                        <div class="search-input">
                            <div class="dataTables_filter"><label> <input type="search"
                                        class="form-control form-control-sm" placeholder="Search"
                                        wire:model.live='search'></label></div>
                        </div>
                    </div>
                    <label class="col-form-label col-md-1">Sort By</label>
                    <div class="col-md-2">
                        <div class="btn-group justify-content-center">
                            <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                                aria-expanded="false">
                                @if ($order == 'ASC')
                                    Latest added
                                @else
                                    Oldest added
                                @endif
                            </a>

                            <div class="dropdown-menu" x-placement="bottom-start"
                                style="position: absolute; will-change: transform; top: 0px; left: 0px; transform: translate3d(163px, 36px, 0px);">
                                <a class="dropdown-item" href="#" wire:click.prevent="sort('ASC')">Latest
                                    added</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="sort('DESC')">Oldest
                                    added</a>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="datatable-scroll">
                <table class="table datatable-basic">
                    <thead>
                        <tr role="row">
                            <th>Permissions</th>
                            <th>Create</th>
                            <th>Edit</th>
                            <th>Delete</th>
                            <th>View</th>
                        </tr>
                    </thead>
                    <tbody>
                        @foreach ($modules as $module)
                            <tr>
                                <td>{{ $module->display_name }}</td>
                                @foreach (['create', 'edit', 'delete', 'view'] as $action)
                                    <td>
                                        <input type="checkbox" wire:ignore
                                            name="permissions[{{ $module->name }}][{{ $action }}]"
                                            wire:model.fill='permission_user.{{ $module->name }}.{{ $action }}'
                                            {{ in_array($action . $module->name, $permissions) ? 'checked' : '' }}>
                                    </td>
                                @endforeach
                            </tr>
                        @endforeach
                    </tbody>
                </table>
                {{-- {{ $modules->links() }} --}}
                <div class="row mt-3">
                    <div class="col-lg-12">
                        <div class="d-flex justify-content-between">

                            <a class="btn btn-secondary btn-small" href="{{ route('role') }}">Back</a>
                            @can('editpermission')
                                <div wire:loading.remove wire:target='simpan'>
                                    <button class="btn btn-primary btn-small" wire:click="simpan">Save Changes</button>
                                </div>
                                <div wire:loading wire:target="simpan">
                                    <button type="button" class="btn bg-slate-600" disabled>Saving...<i
                                            class="icon-spinner2 spinner"></i></button>
                                </div>
                            @endcan
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
    <div class="modal fade" id="add-department" data-bs-keyboard="false" data-bs-backdrop="static" wire:ignore.self>
        <div class="modal-dialog modal-dialog-centered custom-modal-two">
            <div class="modal-content">
                <div class="page-wrapper-new p-0">
                    <div class="content">
                        <div class="modal-header border-0 custom-modal-header">
                            <div class="page-title">
                                @if ($idPermission == '')
                                    <h4>Add Permission</h4>
                                @else
                                    <h4>Edit Permission</h4>
                                @endif
                            </div>
                            <button type="button" class="close" wire:click='cancel'>
                                <span aria-hidden="true">&times;</span>
                            </button>
                        </div>
                        <div class="modal-body custom-modal-body">
                            <form wire:submit='save'>
                                <div class="row">
                                    <div class="col-lg-12">
                                        <div class="mb-3">
                                            <label class="form-label">Name Permission</label>
                                            <input type="text" class="form-control" wire:model='name' required>
                                            @error('name')
                                                <span class="form-text text-danger">{{ $message }}</span>
                                            @enderror
                                        </div>
                                    </div>
                                </div>
                                <div class="modal-footer-btn">
                                    <button type="button" class="btn btn-cancel me-2"
                                        wire:click='cancel'>Cancel</button>
                                    <button type="submit" class="btn btn-submit">Save Changes</button>
                                </div>
                            </form>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <div wire:ignore.self class="modal fade" id="add-modal" tabindex="-1" data-backdrop="static"
        data-keyboard="false" role="dialog" aria-labelledby="modelTitleId" aria-hidden="true">
        <div class="modal-dialog" role="document">
            <div class="modal-content">
                <div class="modal-header">
                    <h5 class="modal-title">Informasi Detail</h5>
                    <button type="button" class="close" data-dismiss="modal" aria-label="Close"
                        wire:click="closeViewModalFileKet">
                        <span aria-hidden="true">&times;</span>
                    </button>
                </div>
                <div class="modal-body">
                    <form wire:submit='save'>
                        <div class="row">
                            <div class="col-lg-12">
                                <div class="mb-3">
                                    <label class="form-label">Name Permission</label>
                                    <input type="text" class="form-control" wire:model='name' required>
                                    @error('name')
                                        <span class="form-text text-danger">{{ $message }}</span>
                                    @enderror
                                </div>
                            </div>
                        </div>
                        <div class="modal-footer-btn">
                            <div class="row mt-3">
                                <div class="col-lg-12">
                                    <div class="d-flex justify-content-between">
                                        <button type="button" class="btn btn-cancel me-2"
                                            wire:click='cancel'>Cancel</button>
                                        <button type="submit" class="btn btn-primary btn-small">Simpan
                                        </button>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </form>
                </div>
            </div>
        </div>
    </div>
</div>
