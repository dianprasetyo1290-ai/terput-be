<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Budgeting" subjudul="Manage your Budget" :breadcrumb="['Master', 'Budgeting']" />
    </x-slot>
    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Budgeting</h5>
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
                        <legend class="text-uppercase font-size-sm font-weight-bold">{{ $idNya ? 'Edit ' : 'Input ' }}
                            {{ __('Currency') }}</legend>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Chart Of Account') }}</label>
                            <div class="col-lg-10">
                                <div class="form-group">
                                    <select class="form-control" wire:model.live='budget.acc_code'>
                                        <option value="" selected>
                                            Pilih </option>
                                        @foreach ($account as $accounts)
                                            <option value="{{ $accounts->acc_code }}">{{ $accounts->acc_name }}</option>
                                        @endforeach
                                    </select>
                                </div>
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Division') }}</label>
                            <div class="col-lg-10">
                                <div class="form-group">
                                    <select class="form-control" wire:model.live='budget.div_code'>
                                        <option value="" selected>
                                            Pilih </option>
                                        @foreach ($division as $divisions)
                                            <option value="{{ $divisions->div_code }}">{{ $divisions->div_name }}
                                            </option>
                                        @endforeach
                                    </select>
                                </div>
                            </div>
                        </div>

                        <div class="row">
                            <div class="col-md-12">
                                <div class="card">
                                    <div class="card-body">
                                        <ul class="nav nav-tabs nav-tabs-highlight">
                                            <li class="nav-item col-md-4 text-center actives"><a
                                                    href="#highlighted-tab1" class="nav-link active"
                                                    data-toggle="tab">Current
                                                    Period</a></li>
                                            <li class="nav-item col-md-4 text-center"><a href="#highlighted-tab2"
                                                    class="nav-link" data-toggle="tab">Last Period</a></li>
                                            <li class="col-md-4 nav-item text-center">
                                                <a href="#highlighted-tab3" class="nav-link" data-toggle="tab">Last
                                                    Two Period</a>
                                        </ul>

                                        <div class="tab-content">
                                            <div class="tab-pane fade show active" id="highlighted-tab1">
                                                <table class="table table-bordered">
                                                    <tr class="text-center">
                                                        <td>Period</td>
                                                        <td>
                                                            Debet
                                                        </td>
                                                    </tr>
                                                    @foreach (range(1, 13) as $month)
                                                        <tr class="text-center">
                                                            <td>{{ sprintf('%02d', $month) }}</td>
                                                            <td>
                                                                <input @if ($lihat) readonly @endif
                                                                    type="number" placeholder="0" class="form-control"
                                                                    wire:model="budget.act_{{ sprintf('%02d', $month) }}">
                                                            </td>
                                                        </tr>
                                                    @endforeach
                                                </table>
                                            </div>

                                            <div class="tab-pane fade" id="highlighted-tab2">
                                                <table class="table table-bordered">

                                                    <tr class="text-center">
                                                        <td>Period</td>
                                                        <td>
                                                            Debet
                                                        </td>
                                                    </tr>
                                                    @foreach (range(1, 13) as $month)
                                                        <tr class="text-center">
                                                            <td>{{ sprintf('%02d', $month) }}</td>
                                                            <td>
                                                                <input @if ($lihat) readonly @endif
                                                                    type="number" placeholder="0" class="form-control"
                                                                    wire:model="budget.lst_{{ sprintf('%02d', $month) }}">
                                                            </td>
                                                        </tr>
                                                    @endforeach
                                                </table>
                                            </div>

                                            <div class="tab-pane fade" id="highlighted-tab3">
                                                <table class="table table-bordered">

                                                    <tr class="text-center">
                                                        <td>Period</td>
                                                        <td>
                                                            Debet
                                                        </td>
                                                    </tr>
                                                    @foreach (range(1, 13) as $month)
                                                        <tr class="text-center">
                                                            <td>{{ sprintf('%02d', $month) }}</td>
                                                            <td>
                                                                <input @if ($lihat) readonly @endif
                                                                    type="number" placeholder="0" class="form-control"
                                                                    wire:model="budget.fut_{{ sprintf('%02d', $month) }}">
                                                            </td>
                                                        </tr>
                                                    @endforeach
                                                </table>
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </fieldset>

                    <div class="card-footer bg-white d-flex justify-content-between align-items-center py-2">
                        <ul class="list-inline mb-0">
                            <li class="list-inline-item">
                                <button type="button" class="btn btn-danger" wire:click="clear">Cancel</button>
                            </li>
                        </ul>

                        @if (!$lihat)
                            <ul class="list-inline mb-0">
                                <li class="list-inline-item">
                                    <button type="submit" class="btn btn-primary text-right">Save changes</button>
                                </li>
                            </ul>
                        @endif
                    </div>
                </form>
            @else
                <button type="button" class="btn btn-primary" wire:click="add">
                    Add Budget <i class="icon-plus-circle2 ml-2"></i>
                </button>
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
                                <th>Account</th>
                                <th>Division</th>
                                <th>Detail</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            @foreach ($data as $row)
                                <tr role="row" class="odd">
                                    <td class="sorting_1">{{ $loop->iteration }}</td>
                                    <td>{{ $row->chartofaccount->acc_code . ' => ' . $row->chartofaccount->acc_name }}
                                    </td>
                                    <td>{{ $row->division->div_code . ' => ' . $row->division->div_name }}</td>
                                    <td><a href="#"
                                            wire:click.prevent='view("{{ Crypt::encrypt($row->id) }}")'>View
                                            Data</a>
                                    </td>
                                    <td class="text-center">
                                        <div class="list-icons">
                                            <div class="dropdown">
                                                <a href="#" class="list-icons-item" data-toggle="dropdown">
                                                    <i class="icon-menu9"></i>
                                                </a>

                                                <div class="dropdown-menu dropdown-menu-right">
                                                    <a href="#" class="dropdown-item"
                                                        wire:click.prevent='add("{{ Crypt::encrypt($row->id) }}")'><i
                                                            class="icon-pencil"></i> Edit</a>
                                                    <a href="#" class="dropdown-item"
                                                        wire:click='confirmDelete("{{ Crypt::encrypt($row->id) }}")'><i
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
            @endif
        </div>
    </div>
</div>
