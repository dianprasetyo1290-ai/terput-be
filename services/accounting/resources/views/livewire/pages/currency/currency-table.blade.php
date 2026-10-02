<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Currency" subjudul="Manage your Currency" :breadcrumb="['Master', 'Currency']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Currency Data</h5>
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
                            <label class="col-form-label col-lg-2">{{ __('Currency Code') }}</label>
                            <div class="col-lg-10">
                                <input type="text"
                                    class="form-control @error('currency.curr_code') is-invalid  @enderror"
                                    wire:model="currency.curr_code" @if ($lihat) readonly @endif>
                                @error('currency.curr_code')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('currency.curr_code') }}
                                    </div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Currency Name') }}</label>
                            <div class="col-lg-10">
                                <input type="text"
                                    class="form-control @error('currency.curr_name') is-invalid  @enderror"
                                    wire:model="currency.curr_name" @if ($lihat) readonly @endif>
                                @error('currency.curr_name')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('currency.curr_name') }}
                                    </div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Rate System') }}</label>
                            <div class="col-lg-10 d-flex align-items-center">
                                @foreach ($radio as $q => $row)
                                    <div class="custom-control custom-radio mr-2">
                                        <input type="radio"
                                            class="custom-control-input @error('currency.rate_system') is-invalid @enderror"
                                            name="custom-stacked-radio" id="cobs{{ $q }}"
                                            wire:model="currency.rate_system" value="{{ $row->code_cd }}"
                                            @if ($row->code_nm == $currency['rate_system']) checked @endif
                                            @if ($lihat) disabled @endif>
                                        <label class="custom-control-label" for="cobs{{ $q }}">
                                            {{ $row->code_nm }} </label>
                                    </div>
                                @endforeach
                                @error('currency.rate_system')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('currency.rate_system') }}
                                    </div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <table class="table table-bordered">
                                <tr class="text-center">
                                    <td></td>
                                    <td>Curent Period</td>
                                    <td>Last Period</td>
                                </tr>
                                <tr class="text-center">
                                    <td>01</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_01">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_01">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>02</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_02">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_02">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>03</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_03">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_03">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>04</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_04">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_04">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>05</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_05">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_05">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>06</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_06">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_06">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>07</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_07">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_07">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>08</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_08">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_08">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>09</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_09">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_09">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>10</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_10">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_10">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>11</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_11">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_11">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>12</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_12">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_12">
                                    </td>
                                </tr>
                                <tr class="text-center">
                                    <td>13</td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.act_13">
                                    </td>
                                    <td>
                                        <input @if ($lihat) readonly @endif type="text"
                                            class="form-control" wire:model="currency.lst_13">
                                    </td>
                                </tr>
                            </table>
                        </div>
                    </fieldset>

                    <div class="card-footer bg-white d-flex justify-content-between align-items-center py-2">
                        <ul class="list-inline mb-0">
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

                        @if (!$lihat)
                            <ul class="list-inline mb-0">
                                <li class="list-inline-item">
                                    <div wire:loading.remove wire:target='save'>
                                        <button type="submit" class="btn btn-primary text-right">Save
                                            changes</button>
                                    </div>
                                    <div wire:loading wire:target="save">
                                        <button type="button" class="btn bg-slate-600" disabled>saving...<i
                                                class="icon-spinner2 spinner"></i></button>
                                    </div>
                                </li>
                            </ul>
                        @endif
                    </div>
                </form>
            @else
                @can('createcurrency_table')
                    <div wire:loading.remove wire:target='add'>
                        <button type="button" class="btn btn-primary" wire:click="add">
                            Add Curenncy <i class="icon-plus-circle2 ml-2"></i>
                        </button>
                    </div>
                    <div wire:loading wire:target="add">
                        <button type="button" class="btn bg-slate-600" disabled>Open...<i
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
                                <th>Currency Code</th>
                                <th>Currency Name</th>
                                <th>Rate System</th>
                                <th>Detail</th>
                                @canany(['editcurrency_table', 'deletecurrency_table'])
                                    <th class="text-center">Actions</th>
                                @endcanany
                            </tr>
                        </thead>
                        <tbody>
                            @foreach ($data as $row)
                                <tr role="row" class="odd">
                                    <td class="sorting_1">{{ $loop->iteration }}</td>
                                    <td>{{ $row->curr_code }}</td>
                                    <td>{{ $row->curr_name }}</td>
                                    <td>
                                        @if ($row->rate_system === 'B')
                                            1 Base = n Foreign
                                        @else
                                            1 Foreign = n Base
                                        @endif
                                    </td>

                                    <td>
                                        <a href="#"
                                            wire:click.prevent='view("{{ Crypt::encrypt($row->curr_code) }}")'>View
                                            Data</a>
                                    </td>
                                    @canany(['editcurrency_table', 'deletecurrency_table'])
                                        <td class="text-center">
                                            <div class="list-icons">
                                                <div class="dropdown">
                                                    <a href="#" class="list-icons-item" data-toggle="dropdown">
                                                        <i class="icon-menu9"></i>
                                                    </a>

                                                    <div class="dropdown-menu dropdown-menu-right">
                                                        @can('editcurrency_table')
                                                            <a href="#" class="dropdown-item"
                                                                wire:click.prevent='add("{{ Crypt::encrypt($row->curr_code) }}")'><i
                                                                    class="icon-pencil"></i> Edit</a>
                                                        @endcan
                                                        @can('deletecurrency_table')
                                                            <a href="#" class="dropdown-item"
                                                                wire:click='confirmDelete("{{ Crypt::encrypt($row->curr_code) }}")'><i
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
            @endif
        </div>
    </div>
</div>
