<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Role" subjudul="Manage Role" :breadcrumb="['Master File', 'User Management', 'Role']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Role Data</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>
        <div class="card-body">
            @if ($form)
                <form action="#" wire:submit="save">
                    @csrf
                    <fieldset class="mb-3">
                        <legend class="text-uppercase font-size-sm font-weight-bold">{{ $idNya ? 'Edit ' : 'Tambah' }}
                            {{ __('User Account') }}</legend>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Name') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('formRole.name') is-invalid  @enderror"
                                    wire:model="formRole.name">
                                @error('formRole.name')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('formRole.name') }}
                                    </div>
                                @enderror
                            </div>
                        </div>
                    </fieldset>

                    <div class="card-footer bg-white d-flex justify-content-between align-items-center py-2">
                        <ul class="list-inline mb-0">
                            <li class="list-inline-item">
                                <div wire:loading.remove wire:target='clear'>
                                    <button type="button" class="btn btn-danger" wire:click="clear">Cancel</button>
                                </div>
                                <div wire:loading wire:target="clear">
                                    <button type="button" class="btn bg-slate-600" disabled>Canceling...<i
                                            class="icon-spinner2 spinner"></i></button>
                                </div>
                            </li>
                        </ul>

                        <ul class="list-inline mb-0">
                            <li class="list-inline-item">
                                <div wire:loading.remove wire:target='save'>
                                    <button type="submit" class="btn btn-primary text-right">Save changes</button>
                                </div>
                                <div wire:loading wire:target="save">
                                    <button type="button" class="btn bg-slate-600" disabled>Saving...<i
                                            class="icon-spinner2 spinner"></i></button>
                                </div>
                            </li>
                        </ul>
                    </div>
                </form>
            @else
                @can('createrole')
                    <div wire:loading.remove wire:target='add'>
                        <button type="button" class="btn btn-primary" wire:click="add">Add Role<i
                                class="icon-user-plus ml-2"></i></button>
                    </div>
                    <div wire:loading wire:target="add">
                        <button type="button" class="btn bg-slate-600" disabled>Open...<i
                                class="icon-spinner2 spinner"></i></button>
                    </div>
                @endcan
            @endif
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
                    <label class="col-form-label col-md-1">Show</label>
                    <div class="col-md-2">
                        <div class="btn-group justify-content-center">
                            <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                                aria-expanded="false">
                                {{ $limit }}
                                items
                            </a>

                            <div class="dropdown-menu" x-placement="bottom-start"
                                style="position: absolute; will-change: transform; top: 0px; left: 0px; transform: translate3d(163px, 36px, 0px);">
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('30')">30
                                    items</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('50')">50
                                    items</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('100')">100
                                    items</a>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="datatable-scroll">
                <table class="table datatable-basic">
                    <thead>
                        <tr role="row">
                            <th>#</th>
                            <th>Role Name</th>
                            <th>Created On</th>
                            @canany(['editrole', 'deleterole', 'viewpermission'])
                                <th class="text-center">Actions</th>
                            @endcan
                        </tr>
                    </thead>
                    <tbody>
                        @foreach ($data as $row)
                            <tr role="row" class="odd {{ $idNya == $row->id ? 'table-active' : '' }}">
                                <td class="sorting_1">{{ $loop->iteration }}</td>
                                <td>{{ $row->name }}</td>
                                <td>{{ $row->created_at }}</td>
                                @canany(['editrole', 'deleterole', 'viewpermission'])
                                    <td class="text-center">
                                        <div class="list-icons d-flex justify-content-center align-items-center">
                                            <div class="dropdown">
                                                <a href="#" class="list-icons-item" data-toggle="dropdown">
                                                    <i class="icon-menu9"></i>
                                                </a>
                                                <div class="dropdown-menu dropdown-menu-right">
                                                    @can('viewpermission')
                                                        <a href="{{ route('permissions', ['foo' => Crypt::encrypt($row->id)]) }}"
                                                            class="dropdown-item">
                                                            <i class="icon-shield-check"></i> Permission
                                                        </a>
                                                    @endcan
                                                    @can('editrole')
                                                        <a href="#" class="dropdown-item"
                                                            wire:click.prevent='add("{{ Crypt::encrypt($row->id) }}")'>
                                                            <i class="icon-pencil"></i> Edit
                                                        </a>
                                                    @endcan
                                                    @can('deleterole')
                                                        <a href="#" class="dropdown-item"
                                                            wire:click='confirmDelete("{{ Crypt::encrypt($row->id) }}")'>
                                                            <i class="icon-trash"></i> Delete
                                                        </a>
                                                    @endcan
                                                </div>
                                            </div>
                                        </div>
                                    </td>
                                @endcan
                            </tr>
                        @endforeach
                    </tbody>
                </table>
                {{ $data->links() }}
            </div>
        </div>
    </div>
</div>
