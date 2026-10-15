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
                                    wire:model="coa.acc_code" @if ($idNya) readonly @endif>
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

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Akun Induk') }}</label>
                            <div class="col-lg-10">
                                <select class="form-control @error('coa.parent_code') is-invalid @enderror" wire:model="coa.parent_code">
                                    <option value="">{{ __('— Tanpa induk (akun tingkat atas) —') }}</option>
                                    @foreach ($parents as $p)
                                        <option value="{{ $p->acc_code }}">{{ $p->acc_code }} — {{ $p->acc_name }}</option>
                                    @endforeach
                                </select>
                                <small class="form-text text-muted">Induk harus satu kelompok. Akun yang menjadi induk tidak dapat menerima posting jurnal.</small>
                                @error('coa.parent_code')
                                    @foreach ($errors->get('coa.parent_code') as $m)<div class="text-danger small">{{ $m }}</div>@endforeach
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Saldo Normal') }}</label>
                            <div class="col-lg-10">
                                <select class="form-control @error('coa.normal_balance') is-invalid @enderror" wire:model="coa.normal_balance">
                                    <option value="">{{ __('Pilih') }}</option>
                                    <option value="D">Debit</option>
                                    <option value="K">Kredit</option>
                                </select>
                                @error('coa.normal_balance')
                                    @foreach ($errors->get('coa.normal_balance') as $m)<div class="text-danger small">{{ $m }}</div>@endforeach
                                @enderror
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Unit') }}</label>
                            <div class="col-lg-10">
                                <select class="form-control @error('coa.unit_scope') is-invalid @enderror" wire:model="coa.unit_scope">
                                    <option value="">{{ __('— Semua unit (akun bersama) —') }}</option>
                                    @foreach ($divisions as $d)
                                        <option value="{{ $d->div_code }}">{{ $d->div_code }} — {{ $d->div_name }}</option>
                                    @endforeach
                                </select>
                                <small class="form-text text-muted">Akun khusus unit hanya dapat dipakai baris jurnal unit tersebut. Terkunci setelah akun memiliki transaksi.</small>
                                @foreach ($errors->get('coa.unit_scope') as $m)<div class="text-danger small">{{ $m }}</div>@endforeach
                            </div>
                        </div>

                        <div class="form-group row">
                            <label class="col-form-label col-lg-2">{{ __('Status & Tanda') }}</label>
                            <div class="col-lg-10">
                                <label class="mr-3"><input type="checkbox" wire:model="coa.is_active"> Aktif</label>
                                <label class="mr-3"><input type="checkbox" wire:model="coa.is_cash"> Kas/Bank</label>
                                <label class="mr-3"><input type="checkbox" wire:model="coa.is_interunit"> Antar-unit (dieliminasi saat konsolidasi)</label>
                                <label class="mr-3"><input type="checkbox" wire:model="coa.is_control"> Akun kontrol (hanya posting otomatis)</label>
                                @foreach (['coa.is_active', 'coa.is_control', 'coa.acc_group'] as $f)
                                    @foreach ($errors->get($f) as $m)<div class="text-danger small">{{ $m }}</div>@endforeach
                                @endforeach
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
            @if (!empty($blockers))
                <div class="alert alert-danger mt-3">
                    <strong>Akun {{ $blockedCode }} tidak dapat dinonaktifkan:</strong>
                    <ul class="mb-0">
                        @foreach ($blockers as $b)
                            <li>{{ $b }}</li>
                        @endforeach
                    </ul>
                </div>
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
                    <label class="col-form-label col-md-1">Status</label>
                    <div class="col-md-2">
                        <select class="form-control form-control-sm" wire:model.live="status">
                            <option value="all">Semua</option>
                            <option value="active">Aktif</option>
                            <option value="inactive">Nonaktif</option>
                        </select>
                    </div>
                    <label class="col-form-label col-md-1">Unit</label>
                    <div class="col-md-2">
                        <select class="form-control form-control-sm" wire:model.live="unit">
                            <option value="">Semua</option>
                            <option value="shared">Akun bersama</option>
                            @foreach ($divisions as $d)
                                <option value="{{ $d->div_code }}">{{ $d->div_code }}</option>
                            @endforeach
                        </select>
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
                            <th>Induk</th>
                            <th>Saldo Normal</th>
                            <th>Status</th>
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
                                <td>{{ $row->parent_code ?? '-' }}</td>
                                <td>{{ ['D' => 'Debit', 'K' => 'Kredit'][$row->normal_balance] ?? '-' }}</td>
                                <td>
                                    <span class="badge {{ $row->is_active ? 'badge-success' : 'badge-secondary' }}">{{ $row->is_active ? 'Aktif' : 'Nonaktif' }}</span>
                                    @if (isset($parentCodes[$row->acc_code]))<span class="badge badge-info">Induk</span>@endif
                                    @if ($row->is_control)<span class="badge badge-warning">Kontrol</span>@endif
                                    @if ($row->is_interunit)<span class="badge badge-light">Antar-unit</span>@endif
                                    @if ($row->unit_scope)<span class="badge badge-primary">Unit {{ $row->unit_scope }}</span>@endif
                                </td>
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
                                                        @if ($row->is_active)
                                                            <a href="#" class="dropdown-item"
                                                                wire:click.prevent='deactivate("{{ Crypt::encrypt($row->acc_code) }}")'
                                                                wire:confirm="Nonaktifkan akun {{ $row->acc_code }}? Akun nonaktif tidak dapat menerima jurnal baru."><i
                                                                    class="icon-blocked"></i> Nonaktifkan</a>
                                                        @endif
                                                    @endcan
                                                    @can('editchart_of_account')
                                                        @if (! $row->is_active)
                                                            <a href="#" class="dropdown-item"
                                                                wire:click.prevent='activate("{{ Crypt::encrypt($row->acc_code) }}")'><i
                                                                    class="icon-checkmark"></i> Aktifkan</a>
                                                        @endif
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

    <div class="card">
        <div class="card-header"><h5 class="card-title">Template COA per Unit</h5></div>
        <div class="card-body">
            <p class="small text-muted mb-2">Simpan akun bersama yang aktif sebagai template, lalu salin ke unit (SD/SMP/…).
                Salinan berkode <code>&lt;kode&gt;.&lt;UNIT&gt;</code>, khusus unit tersebut, dan dapat disesuaikan di atas. Menyalin ulang hanya menambah akun yang belum ada.</p>
            @if ($tplMessage)
                <div class="alert alert-success">{{ $tplMessage }}</div>
            @endif
            @can('createchart_of_account')
                <div class="row">
                    <div class="col-md-6">
                        <label class="font-weight-bold">Simpan COA bersama sebagai template</label>
                        <input type="text" class="form-control mb-1 @error('coaTpl.name') is-invalid @enderror" placeholder="Nama template" wire:model="coaTpl.name">
                        @foreach ($errors->get('coaTpl.name') as $m)<div class="text-danger small">{{ $m }}</div>@endforeach
                        <input type="text" class="form-control mb-2" placeholder="Keterangan (opsional)" wire:model="coaTpl.description">
                        <button type="button" class="btn btn-primary" wire:click="saveTemplate" wire:loading.attr="disabled">Simpan Template</button>
                    </div>
                    <div class="col-md-6">
                        <label class="font-weight-bold">Salin template ke unit</label>
                        <select class="form-control mb-1 @error('coaCopy.template_id') is-invalid @enderror" wire:model="coaCopy.template_id">
                            <option value="">Pilih template</option>
                            @foreach ($coaTemplates as $t)
                                <option value="{{ $t->id }}">{{ $t->name }} ({{ $t->accounts }} akun)</option>
                            @endforeach
                        </select>
                        @foreach ($errors->get('coaCopy.template_id') as $m)<div class="text-danger small">{{ $m }}</div>@endforeach
                        <select class="form-control mb-2 @error('coaCopy.div_code') is-invalid @enderror" wire:model="coaCopy.div_code">
                            <option value="">Pilih unit</option>
                            @foreach ($divisions as $d)
                                <option value="{{ $d->div_code }}">{{ $d->div_code }} — {{ $d->div_name }}</option>
                            @endforeach
                        </select>
                        @foreach ($errors->get('coaCopy.div_code') as $m)<div class="text-danger small">{{ $m }}</div>@endforeach
                        <button type="button" class="btn btn-success" wire:click="copyTemplate" wire:loading.attr="disabled"
                            wire:confirm="Salin template ke unit terpilih? Akun baru akan dibuat.">Salin ke Unit</button>
                    </div>
                </div>
                <hr>
            @endcan
            <table class="table table-sm">
                <thead class="thead-light"><tr><th>Template</th><th>Keterangan</th><th class="text-right">Akun</th><th>Dibuat</th>@can('deletechart_of_account')<th></th>@endcan</tr></thead>
                <tbody>
                    @forelse ($coaTemplates as $t)
                        <tr>
                            <td>{{ $t->name }}</td>
                            <td class="small">{{ $t->description }}</td>
                            <td class="text-right">{{ $t->accounts }}</td>
                            <td class="small">{{ $t->created_by }} · {{ $t->created_at }}</td>
                            @can('deletechart_of_account')
                                <td class="text-right"><button class="btn btn-sm btn-light" wire:click="deleteTemplate({{ $t->id }})"
                                    wire:confirm="Hapus template {{ $t->name }}?">Hapus</button></td>
                            @endcan
                        </tr>
                    @empty
                        <tr><td colspan="5" class="text-center text-muted">Belum ada template COA.</td></tr>
                    @endforelse
                </tbody>
            </table>
        </div>
    </div>
</div>
