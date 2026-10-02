<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Report Row Details" subjudul="Manage your report row details"
            :breadcrumb="['Master', 'Report Row Detail Management']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Report Row Detail Data</h5>
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
                <form action="#" wire:submit.prevent="store">
                    @csrf
                    <fieldset class="mb-3">
                        <legend class="text-uppercase font-size-sm font-weight-bold">
                            {{ $updateMode ? 'Edit ' : 'Input ' }} {{ __('Report Row Detail') }}
                        </legend>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Identifier') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('identifier') is-invalid @enderror"
                                    wire:model="identifier">
                                @error('identifier')
                                    <div class="invalid-feedback">{{ $errors->first('identifier') }}</div>
                                @enderror
                            </div>
                        </div>
                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Name') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('name') is-invalid @enderror"
                                    wire:model="name">
                                @error('name')
                                    <div class="invalid-feedback">{{ $errors->first('name') }}</div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Group') }}</label>
                            <div class="col-lg-10">
                                <select class="form-control @error('group') is-invalid @enderror" wire:model="group">
                                    <option value="">Pilih Group</option>
                                    @foreach ($groupList as $key => $item)
                                        <option value="{{ $key }}">{{ $item }}</option>
                                    @endforeach
                                </select>
                                @error('group')
                                    <div class="invalid-feedback">{{ $errors->first('group') }}</div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Level') }}</label>
                            <div class="col-lg-10">
                                <select class="form-control @error('level') is-invalid @enderror" wire:model="level">
                                    <option value="">Pilih Indent</option>
                                    @foreach ($levelList as $key => $item)
                                        <option value="{{ $key }}">{{ $item }}</option>
                                    @endforeach
                                </select>
                                @error('level')
                                    <div class="invalid-feedback">{{ $errors->first('level') }}</div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Normal Balance') }}</label>
                            <div class="col-lg-10">
                                <select class="form-control @error('normal_balance') is-invalid @enderror"
                                    wire:model="normal_balance">
                                    <option value="">Pilih Balance Normal</option>
                                    @foreach ($normal_balanceList as $key => $item)
                                        <option value="{{ $key }}">{{ $item }}</option>
                                    @endforeach
                                </select>
                                @error('normal_balance')
                                    <div class="invalid-feedback">{{ $errors->first('normal_balance') }}</div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Operator') }}</label>
                            <div class="col-lg-10">
                                <select class="form-control @error('operator') is-invalid @enderror"
                                    wire:model="operator">
                                    <option value="">Pilih Operator</option>
                                    @foreach ($operatorList as $key => $item)
                                        <option value="{{ $key }}">{{ $item }}</option>
                                    @endforeach
                                </select>
                                @error('operator')
                                    <div class="invalid-feedback">{{ $errors->first('operator') }}</div>
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Kode') }}</label>
                            <div class="col-lg-10">
                                <input type="text" class="form-control @error('kode') is-invalid @enderror"
                                    wire:model="kode">
                                @error('kode')
                                    <div class="invalid-feedback">{{ $errors->first('kode') }}</div>
                                @enderror
                            </div>
                        </div>
                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('formula') }}</label>
                            <div class="col-lg-10">
                                <textarea class="form-control @error('formula') is-invalid @enderror" wire:model="formula">
                                </textarea>
                                @error('formula')
                                    <div class="invalid-feedback">{{ $errors->first('formula') }}</div>
                                @enderror
                            </div>
                        </div>
                    </fieldset>

                    <div class="py-2 bg-white card-footer d-flex justify-content-between align-items-center">
                        <ul class="mb-0 list-inline">
                            <li class="list-inline-item">
                                <button type="button" class="btn btn-danger" wire:click="clear">Cancel</button>
                            </li>
                        </ul>

                        <ul class="mb-0 list-inline">
                            <li class="list-inline-item">
                                <button type="submit" class="text-right btn btn-primary">Save changes</button>
                            </li>
                        </ul>
                    </div>
                </form>
            @else
                <button type="button" class="btn btn-primary" wire:click="create">Add Report Row Detail<i
                        class="ml-2 icon-plus-circle2"></i></button>
                <a class="btn btn-warning"
                    href="{{ route('generator.report-preview', ['idHead' => $report_row_head_id]) }}"
                    target="_blank">Preview
                    Report<i class="ml-2 icon-plus-circle2"></i></a>
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
                            <th>Order</th>
                            <th>Identifier</th>
                            <th>Name</th>
                            <th>Group</th>
                            <th>Level</th>
                            <th>Normal Balance</th>
                            <th>Operator</th>
                            <th>Kode</th>
                            <th>Formula</th>
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
                                                wire:click="moveUp({{ $row->id }})"
                                                title="Move Up">
                                            <i class="icon-arrow-up8"></i>
                                        </button>
                                        <button type="button" class="btn btn-light"
                                                wire:click="moveDown({{ $row->id }})"
                                                title="Move Down">
                                            <i class="icon-arrow-down8"></i>
                                        </button>
                                    </div>
                                </td>
                                <td>{{ $row->identifier }}</td>
                                <td>{{ $row->name }}</td>
                                <td>{{ $row->group }}</td>
                                <td>{{ $row->level }}</td>
                                <td>{{ $row->normal_balance }}</td>
                                <td>{{ $row->operator }}</td>
                                <td>{{ $row->kode }}</td>
                                <td>{{ $row->formula }}</td>
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
