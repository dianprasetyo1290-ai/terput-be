<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Chart of Account" subjudul="Manage your Chart of Account" :breadcrumb="['Master', 'Chart of Account']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Chart of Account Data</h5>
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
                            {{ __('Chart of Account') }}</legend>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Account Code') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('coa.acc_code') is-invalid  @enderror"
                                    wire:model="coa.acc_code">
                                @error('coa.acc_code')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('coa.acc_code') }}
                                    </div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Account Name') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('coa.acc_name') is-invalid  @enderror"
                                    wire:model="coa.acc_name">
                                @error('coa.acc_name')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('coa.acc_name') }}
                                    </div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Account Group') }}</label>
                            <div class="col-lg-10">
                                <div class="form-group">
                                    <div wire:ignore>
                                        <select
                                            class="form-control form-control-lg select-search @error('coa.acc_group') is-invalid  @enderror"
                                            wire:model.live="coa.acc_group">
                                            <option value="">{{ __('Pilih') }}</option>
                                            @foreach ($listGroup as $group)
                                                <option value="{{ $group->id }}">{{ $group->name }}
                                                </option>
                                            @endforeach
                                        </select>
                                    </div>
                                    @error('coa.acc_group')
                                        <span class="form-text text-danger">
                                            {{ $errors->first('coa.acc_group') }}
                                        </span>
                                    @enderror
                                </div>
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Sub Account Group') }}</label>
                            <div class="col-lg-10">
                                <div class="form-group">

                                    <div wire:ignore>
                                        <select id="OutletId"
                                            class="form-control form-control-lg select-search2 @error('coa.sub_acc_group') is-invalid  @enderror"
                                            wire:model.live="coa.sub_acc_group">
                                            <option value="">{{ __('Pilih') }}</option>
                                            @foreach ($listSubGroup as $subGroup)
                                                <option value="{{ $subGroup->id }}">{{ $subGroup->name }}
                                                </option>
                                            @endforeach
                                        </select>
                                    </div>
                                    @error('coa.sub_acc_group')
                                        <span class="form-text text-danger">
                                            {{ $message }}
                                        </span>
                                    @enderror
                                </div>

                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Account Currency') }}</label>
                            <div class="col-lg-10" wire:ignore>
                                <select
                                    class="form-control select-search3 form-control-lg @error('coa.acc_currency') is-invalid  @enderror"
                                    wire:model.live="coa.acc_currency">
                                    <option value="">{{ __('Pilih') }}</option>
                                    @foreach ($currency as $group)
                                        <option value="{{ $group->curr_code }}">{{ $group->curr_code }}
                                        </option>
                                    @endforeach
                                </select>
                                @error('coa.acc_currency')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('coa.acc_currency') }}
                                    </div>
                                @enderror
                            </div>
                        </div>

                    </fieldset>

                    <div class="py-2 bg-white card-footer d-flex justify-content-between align-items-center">
                        <ul class="mb-0 list-inline">
                            <li class="list-inline-item">
                                <div wire:loading.remove wire:target='clear'>
                                    <button type="button" class="btn btn-danger" wire:click="clear">Cancel</button>
                                </div>
                                <div wire:loading wire:target="clear">
                                    <button type="button" class="btn bg-slate-600" disabled>canceling...<i
                                            class="icon-spinner2 spinner"></i></button>
                                </div>
                            </li>
                        </ul>

                        <ul class="mb-0 list-inline">
                            <li class="list-inline-item">
                                <div wire:loading.remove wire:target='save'>
                                    <button type="submit" class="text-right btn btn-primary">Save changes</button>
                                </div>

                                <div wire:loading wire:target="save">
                                    <button type="button" class="btn bg-slate-600" disabled>saving...<i
                                            class="icon-spinner2 spinner"></i></button>
                                </div>
                            </li>
                        </ul>
                    </div>
                </form>
            @else
                @can('createchart_of_account')
                    <div wire:loading.remove wire:target='add'>
                        <button type="button" class="btn btn-primary" wire:click="add">
                            Add Chart of Account <i class="ml-2 icon-plus-circle2"></i>
                        </button>
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
                            <th>Account Code</th>
                            <th>Account Name</th>
                            <th>Account Group</th>
                            <th>Sub Account Group</th>
                            <th>Currency</th>
                            @canany(['editchart_of_account', 'deletechart_of_account'])
                                <th class="text-center">Actions</th>
                            @endcanany
                        </tr>
                    </thead>
                    <tbody>
                        @foreach ($data as $row)
                            <tr role="row" class="odd">
                                <td class="sorting_1">{{ $loop->iteration }}</td>
                                <td>{{ $row->acc_code }}</td>
                                <td>{{ $row->acc_name }}</td>
                                <td>{{ $row->group->name ?? '-' }}</td>
                                <td>{{ $row->sub_group->name ?? '-' }}</td>
                                <td>{{ $row->acc_currency }}</td>
                                @canany(['editchart_of_account', 'deletechart_of_account'])
                                    <td class="text-center">
                                        <div class="list-icons">
                                            <div class="dropdown">
                                                <a href="#" class="list-icons-item" data-toggle="dropdown">
                                                    <i class="icon-menu9"></i>
                                                </a>

                                                <div class="dropdown-menu dropdown-menu-right">
                                                    @can('editchart_of_account')
                                                        <a href="#" class="dropdown-item"
                                                            wire:click.prevent='add("{{ Crypt::encrypt($row->acc_code) }}")'><i
                                                                class="icon-pencil"></i> Edit</a>
                                                    @endcan
                                                    @can('deletechart_of_account')
                                                        <a href="#" class="dropdown-item"
                                                            wire:click='confirmDelete("{{ Crypt::encrypt($row->acc_code) }}")'><i
                                                                class="icon-trash"></i> Delete</a>
                                                    @endcan
                                                </div>
                                            </div>
                                        </div>
                                    </td>
                                @endcanany
                            </tr>
                        @endforeach
                    </tbody>
                </table>
                {{ $data->links() }}
            </div>
        </div>
    </div>
</div>
