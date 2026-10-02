<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Report Final" subjudul="Manage your report final"
            :breadcrumb="['Report Generator', 'Report Final Management']" />
    </x-slot>
    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Report Final Data</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>
        <div class="card-body">
            @if (session()->has('message'))
                <div class="alert alert-success alert-dismissible">
                    <button type="button" class="close" data-dismiss="alert"><span>&times;</span></button>
                    {{ session('message') }}
                </div>
            @endif

            @if ($form)
                <form action="#" wire:submit.prevent="store">
                    @csrf
                    <fieldset class="mb-3">
                        <legend class="text-uppercase font-size-sm font-weight-bold">
                            {{ $updateMode ? 'Edit ' : 'Input ' }}
                            {{ __('Report Final') }}</legend>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Name') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('name') is-invalid @enderror"
                                    wire:model="name">
                                @error('name')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('name') }}
                                    </div>
                                @enderror
                            </div>
                        </div>
                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Print Title') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('print_title') is-invalid @enderror"
                                    wire:model="print_title">
                                @error('print_title')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('print_title') }}
                                    </div>
                                @enderror
                            </div>
                        </div>
                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Sub Title') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('sub_title') is-invalid @enderror"
                                    wire:model="sub_title">
                                @error('sub_title')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('sub_title') }}
                                    </div>
                                @enderror
                            </div>
                        </div>
                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Column Template') }}</label>
                            <div class="col-lg-10">
                                <select class="form-control @error('report_col_head_id') is-invalid @enderror"
                                    wire:model="report_col_head_id">
                                    <option value="">{{ __('-- Select Column Template --') }}</option>
                                    @foreach ($colHeads as $colHead)
                                        <option value="{{ $colHead->id }}">{{ $colHead->name }} ({{ $colHead->col_type }})</option>
                                    @endforeach
                                </select>
                                @error('report_col_head_id')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('report_col_head_id') }}
                                    </div>
                                @enderror
                            </div>
                        </div>
                    </fieldset>

                    <div class="card-footer bg-white d-flex justify-content-between align-items-center py-2">
                        <ul class="list-inline mb-0">
                            <li class="list-inline-item">
                                <button type="button" class="btn btn-danger" wire:click="clear">Cancel</button>
                            </li>
                        </ul>

                        <ul class="list-inline mb-0">
                            <li class="list-inline-item">
                                <button type="submit" class="btn btn-primary text-right">Save changes</button>
                            </li>
                        </ul>
                    </div>
                </form>
            @else
                <button type="button" class="btn btn-primary" wire:click="create">Add Report Final<i
                        class="icon-plus-circle2 ml-2"></i></button>
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

                            <div class="dropdown-menu">
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

                            <div class="dropdown-menu">
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('10')">10 items</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('30')">30 items</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('50')">50 items</a>
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
                            <th>Name</th>
                            <th>Print Title</th>
                            <th>Column Template</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        @foreach ($data as $row)
                            <tr role="row" class="odd">
                                <td class="sorting_1">{{ $loop->iteration }}</td>
                                <td>{{ $row->name }}</td>
                                <td>{{ $row->print_title }}</td>
                                <td>
                                    @if ($row->reportColHead)
                                        <span class="badge badge-info">{{ $row->reportColHead->name }}</span>
                                        <small class="text-muted">({{ $row->reportColHead->col_type }})</small>
                                    @else
                                        <span class="text-muted">-</span>
                                    @endif
                                </td>
                                <td class="text-center">
                                    <div class="list-icons">
                                        <div class="dropdown">
                                            <a href="#" class="list-icons-item" data-toggle="dropdown">
                                                <i class="icon-menu9"></i>
                                            </a>

                                            <div class="dropdown-menu dropdown-menu-right">
                                                <a href="#" class="dropdown-item"
                                                    wire:click.prevent='edit("{{ $row->id }}")'><i
                                                        class="icon-pencil"></i> Edit</a>
                                                <a href="{{ route('generator.viewfinal_design_detail', $row->id) }}"
                                                    class="dropdown-item">
                                                    <i class="icon-eye"></i> Lihat</a>
                                                <a href="#" class="dropdown-item"
                                                    wire:click.prevent='confirmDelete("{{ $row->id }}")'><i
                                                        class="icon-trash"></i> Delete</a>
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
