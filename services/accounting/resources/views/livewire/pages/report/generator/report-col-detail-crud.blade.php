<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Column Details" subjudul="Manage columns for template: {{ $colHead->name }}"
            :breadcrumb="['Report Generator', 'Column Template', 'Column Details']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">
                Column Details &mdash;
                <span class="text-primary">{{ $colHead->name }}</span>
                <small class="text-muted">({{ $colHead->col_type }})</small>
            </h5>
            <div class="header-elements">
                <a href="{{ route('generator.col') }}" class="btn btn-light btn-sm">
                    <i class="icon-arrow-left8 mr-1"></i> Back to Templates
                </a>
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
                            {{ $updateMode ? 'Edit ' : 'Input ' }} {{ __('Column Detail') }}
                        </legend>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Column Title') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('col_title') is-invalid @enderror"
                                    wire:model="col_title" placeholder="e.g., This Month, Q1, January">
                                @error('col_title')
                                    <div class="invalid-feedback">{{ $errors->first('col_title') }}</div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Content Type') }}</label>
                            <div class="col-lg-10">
                                <select class="form-control @error('content') is-invalid @enderror" wire:model="content">
                                    <option value="">{{ __('-- Select Content Type --') }}</option>
                                    @foreach ($contentList as $key => $label)
                                        <option value="{{ $key }}">{{ $label }}</option>
                                    @endforeach
                                </select>
                                @error('content')
                                    <div class="invalid-feedback">{{ $errors->first('content') }}</div>
                                @enderror
                                <small class="form-text text-muted">
                                    <strong>Current</strong>: Actual balance from mst_saldo |
                                    <strong>Budget</strong>: Budget data from mst_budget |
                                    <strong>Formula</strong>: Calculation between columns (e.g., {0001} - {0002}) |
                                    <strong>Base%</strong>: Percentage relative to a base row
                                </small>
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Period Expression') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('period') is-invalid @enderror"
                                    wire:model="period" placeholder="e.g., Curr, Curr - 01, Beg, YTD, 01,02,03">
                                @error('period')
                                    <div class="invalid-feedback">{{ $errors->first('period') }}</div>
                                @enderror
                                <small class="form-text text-muted">
                                    <strong>Curr</strong>: Current period |
                                    <strong>Curr - 01</strong>: Previous period |
                                    <strong>Beg</strong>: Beginning balance |
                                    <strong>YTD</strong>: Year to date |
                                    <strong>01,02,03</strong>: Specific periods |
                                    For <strong>Formula</strong>: use column references like {0001} - {0002} |
                                    For <strong>Base%</strong>: use base column reference like {0001}
                                </small>
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Period Key') }}</label>
                            <div class="col-lg-10">
                                <input type="number" class="form-control @error('period_key') is-invalid @enderror"
                                    wire:model="period_key" placeholder="e.g., 1-12 for months, 3/6/9/12 for quarters">
                                @error('period_key')
                                    <div class="invalid-feedback">{{ $errors->first('period_key') }}</div>
                                @enderror
                                <small class="form-text text-muted">
                                    Used for dynamic column slicing in YTD/Quarterly templates. Columns with period_key > current period will be hidden.
                                    Leave empty for fixed templates or formula/base% columns.
                                </small>
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Division') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('division') is-invalid @enderror"
                                    wire:model="division" placeholder="* for all divisions">
                                @error('division')
                                    <div class="invalid-feedback">{{ $errors->first('division') }}</div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Format') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('col_format') is-invalid @enderror"
                                    wire:model="col_format" placeholder="#,##0.00">
                                @error('col_format')
                                    <div class="invalid-feedback">{{ $errors->first('col_format') }}</div>
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
                <button type="button" class="btn btn-primary" wire:click="create">Add Column<i
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
                    <label class="col-form-label col-md-1">Show</label>
                    <div class="col-md-2">
                        <div class="btn-group justify-content-center">
                            <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                                aria-expanded="false">
                                {{ $limit }} items
                            </a>
                            <div class="dropdown-menu">
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('30')">30 items</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('50')">50 items</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('100')">100 items</a>
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
                            <th>Order</th>
                            <th>Sort</th>
                            <th>Column Title</th>
                            <th>Content</th>
                            <th>Period</th>
                            <th>Period Key</th>
                            <th>Division</th>
                            <th>Format</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        @foreach ($data as $row)
                            <tr role="row" class="odd">
                                <td class="sorting_1">{{ $loop->iteration }}</td>
                                <td>
                                    <div class="btn-group btn-group-sm">
                                        <button type="button" class="btn btn-light"
                                            wire:click="moveUp({{ $row->id }})" title="Move Up">
                                            <i class="icon-arrow-up8"></i>
                                        </button>
                                        <button type="button" class="btn btn-light"
                                            wire:click="moveDown({{ $row->id }})" title="Move Down">
                                            <i class="icon-arrow-down8"></i>
                                        </button>
                                    </div>
                                </td>
                                <td>{{ $row->sort_order }}</td>
                                <td><strong>{{ $row->col_title }}</strong></td>
                                <td>
                                    @switch($row->content)
                                        @case('Current')
                                            <span class="badge badge-primary">Current</span>
                                            @break
                                        @case('Budget')
                                            <span class="badge badge-success">Budget</span>
                                            @break
                                        @case('Formula')
                                            <span class="badge badge-warning">Formula</span>
                                            @break
                                        @case('Base%')
                                            <span class="badge badge-info">Base%</span>
                                            @break
                                        @default
                                            <span class="badge badge-secondary">{{ $row->content }}</span>
                                    @endswitch
                                </td>
                                <td><code>{{ $row->period }}</code></td>
                                <td>{{ $row->period_key }}</td>
                                <td>{{ $row->division }}</td>
                                <td><small>{{ $row->col_format }}</small></td>
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
