<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction Type" subjudul="Manage your Transaction Type" :breadcrumb="['Master', 'Transaction Type']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Transaction Type Data</h5>
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
                            {{ __('Transaction Type') }}</legend>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Transaction Code') }}</label>
                            <div class="col-lg-10">
                                <input type="text"
                                    class="form-control @error('transaction.trans_code') is-invalid  @enderror"
                                    wire:model="transaction.trans_code">
                                @error('transaction.trans_code')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('transaction.trans_code') }}
                                    </div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Transaction Name') }}</label>
                            <div class="col-lg-10">
                                <input type="text"
                                    class="form-control @error('transaction.trans_name') is-invalid  @enderror"
                                    wire:model="transaction.trans_name">
                                @error('transaction.trans_name')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('transaction.trans_name') }}
                                    </div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Transaction Status') }}</label>
                            <div class="col-lg-10">
                                <div wire:ignore>
                                    <select
                                        class="form-control select-search form-control-lg @error('transaction.trans_status') is-invalid  @enderror"
                                        wire:model.live="transaction.trans_status">
                                        <option value="">{{ __('Pilih') }}</option>
                                        @foreach ($transaction_code as $trans)
                                            <option value="{{ $trans->code_cd }}">{{ $trans->code_nm }}
                                            </option>
                                        @endforeach
                                    </select>
                                </div>
                                @error('transaction.trans_status')
                                    <span class="form-text text-danger">
                                        {{ $errors->first('transaction.trans_status') }}
                                    </span>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row" wire:ignore>
                            <label class="col-form-label col-lg-2">{{ __('Master Account') }}</label>
                            <div class="col-lg-10">
                                <select
                                    class="form-control select-search2 form-control-lg @error('transaction.trans_account') is-invalid  @enderror"
                                    wire:model.live="transaction.trans_account">
                                    <option value="">{{ __('Pilih') }}</option>
                                    @foreach ($m_account as $acc)
                                        <option value="{{ $acc->acc_code }}">{{ $acc->acc_name }}
                                        </option>
                                    @endforeach
                                </select>
                                @error('transaction.trans_account')
                                    <div class="invalid-feedback">
                                        {{ $errors->first('transaction.trans_account') }}
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
                @can('createtransaction_type')
                    <div wire:loading.remove wire:target='add'>
                        <button type="button" class="btn btn-primary" wire:click="add">
                            Add Transaction <i class="icon-plus-circle2 ml-2"></i>
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
                            <th>Transaction Code</th>
                            <th>Transaction Name</th>
                            <th>Transaction Status</th>
                            <th>Transaction Account</th>
                            @canany(['edittransaction_type', 'deletetransaction_type'])
                                <th>Actions</th>
                            @endcanany
                        </tr>
                    </thead>
                    <tbody>
                        @foreach ($data as $row)
                            <tr role="row" class="odd">
                                <td class="sorting_1">{{ $loop->iteration }}</td>
                                <td>{{ $row->trans_code }}</td>
                                <td>{{ $row->trans_name }}</td>
                                <td>{{ $row->trans_status }}</td>
                                <td>{{ $row->trans_account }}</td>
                                @canany(['edittransaction_type', 'deletetransaction_type'])
                                    <td class="text-center">
                                        <div class="list-icons">
                                            <div class="dropdown">
                                                <a href="#" class="list-icons-item" data-toggle="dropdown">
                                                    <i class="icon-menu9"></i>
                                                </a>

                                                <div class="dropdown-menu dropdown-menu-right">
                                                    @can('edittransaction_type')
                                                        <a href="#" class="dropdown-item"
                                                            wire:click.prevent='add("{{ Crypt::encrypt($row->trans_code) }}")'><i
                                                                class="icon-pencil"></i> Edit</a>
                                                    @endcan
                                                    @can('deletetransaction_type')
                                                        <a href="#" class="dropdown-item"
                                                            wire:click='confirmDelete("{{ Crypt::encrypt($row->trans_code) }}")'><i
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
