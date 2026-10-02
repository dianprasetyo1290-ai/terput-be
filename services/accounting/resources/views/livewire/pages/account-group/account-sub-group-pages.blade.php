<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Account Sub Group" subjudul="Manage your account sub groups"
            :breadcrumb="['Master', 'Account Sub Group Management']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Account Sub Group Data</h5>
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
                <form action="#" wire:submit.prevent="save">
                    @csrf
                    <fieldset class="mb-3">
                        <legend class="text-uppercase font-size-sm font-weight-bold">{{ $idNya ? 'Edit ' : 'Input ' }}
                            {{ __('Account Sub Group') }}</legend>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Account Group') }}</label>
                            <div class="col-lg-10">
                                <div wire:ignore>

                                    <select
                                        class="select-search form-control @error('sub_group.account_group_table_id') is-invalid @enderror"
                                        wire:model="sub_group.account_group_table_id">
                                        <option value="">Select Account Group</option>
                                        @foreach ($accountGroupOptions as $id => $name)
                                            <option value="{{ $id }}">{{ $name }}</option>
                                        @endforeach
                                    </select>
                                </div>

                                @error('sub_group.account_group_table_id')
                                    <span class="form-text text-danger">
                                        {{ $message }}
                                    </span>
                                @enderror
                            </div>

                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Sub Group Name') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('sub_group.name') is-invalid @enderror"
                                    wire:model="sub_group.name">
                                @error('sub_group.name')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('sub_group.name') }}
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
                                    <button type="button" class="btn btn-light" disabled>canceling...<i
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
                                    <button type="button" class="btn btn-light" disabled>saving...<i
                                            class="icon-spinner2 spinner"></i></button>
                                </div>
                            </li>
                        </ul>
                    </div>
                </form>
            @else
                @can('createsub_account_group_table')
                    <div wire:loading.remove wire:target='add'>
                        <button type="button" class="btn btn-primary" wire:click="add">Add Account Sub Group<i
                                class="icon-plus-circle2 ml-2"></i></button>
                    </div>
                    <div wire:loading wire:target="add">
                        <button type="button" class="btn btn-light" disabled>Open...<i
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
                                {{ $limit }} items
                            </a>

                            <div class="dropdown-menu" x-placement="bottom-start"
                                style="position: absolute; will-change: transform; top: 0px; left: 0px; transform: translate3d(163px, 36px, 0px);">
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('30')">30 items</a>
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
                            <th>Sub Group Name</th>
                            <th>Account Group</th>
                            @canany(['editsub_account_group_table', 'deletesub_account_group_table'])
                                <th class="text-center">Actions</th>
                            @endcanany
                        </tr>
                    </thead>
                    <tbody>
                        @foreach ($data as $row)
                            <tr role="row" class="odd">
                                <td class="sorting_1">{{ $loop->iteration }}</td>
                                <td>{{ $row->name }}</td>
                                <td>{{ $row->accountGroup->name }}</td>
                                <td class="text-center">
                                    <div class="list-icons">
                                        <div class="dropdown">
                                            <a href="#" class="list-icons-item" data-toggle="dropdown"><i
                                                    class="icon-menu9"></i></a>

                                            <div class="dropdown-menu dropdown-menu-right">
                                                @can('editsub_account_group_table')
                                                    <a href="#" class="dropdown-item"
                                                        wire:click.prevent="edit('{{ Crypt::encrypt($row->id) }}')"><i
                                                            class="icon-pencil"></i> Edit</a>
                                                @endcan
                                                @can('editsub_account_group_table')
                                                    <a href="#" class="dropdown-item"
                                                        wire:click.prevent="confirmDelete('{{ Crypt::encrypt($row->id) }}')"><i
                                                            class="icon-trash"></i> Delete</a>
                                                @endcan
                                            </div>
                                        </div>
                                    </div>
                                </td>
                            </tr>
                        @endforeach
                    </tbody>
                </table>

                {{ $data->links() }}
            </div>
        </div>
    </div>
</div>

@push('scripts')
    <script src="https://cdn.jsdelivr.net/npm/sweetalert2@11"></script>
    <script>
        window.addEventListener('swal:confirm', event => {
            Swal.fire({
                title: 'Are you sure?',
                text: event.detail.message,
                icon: 'warning',
                showCancelButton: true,
                confirmButtonColor: '#3085d6',
                cancelButtonColor: '#d33',
                confirmButtonText: 'Yes, delete it!',
                cancelButtonText: 'Cancel'
            }).then((result) => {
                if (result.isConfirmed) {
                    Livewire.emit('delete', event.detail.id);
                }
            });
        });

        window.addEventListener('swal:success', event => {
            Swal.fire({
                icon: 'success',
                title: event.detail.message,
                showConfirmButton: false,
                timer: 1500
            });
        });
    </script>
@endpush
