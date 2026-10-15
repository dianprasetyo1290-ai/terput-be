<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="Manage your History Journal Transaction"
            :breadcrumb="['Transaction', 'History Journal Transaction']" />
    </x-slot>

    <div class="rounded-lg shadow-lg card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title"><i class="mr-2 icon-history"></i>Data History Jurnal Transaksi</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>

        <div class="card-body">
            <div class="alert alert-info">Jurnal histori berasal dari periode yang sudah ditutup dan hanya dapat dilihat.
                Koreksi dilakukan melalui <a href="{{ url('period-reopen') }}">Buka Kembali Periode</a> (periode terakhir, dengan persetujuan)
                atau <a href="{{ url('prior-year-adjustment') }}">Penyesuaian Tahun Lalu</a>.</div>
            @if ($formDetail)
                {{-- ============================================ --}}
                {{-- DETAIL FORM --}}
                {{-- ============================================ --}}
                <form action="#" wire:submit.prevent="saveDetail">
                    {{-- Info Header Transaksi --}}
                    <div class="p-4 mb-5 border border-blue-100 bg-blue-50 rounded-xl">
                        <div class="row">
                            <div class="col-md-3">
                                <span class="block mb-1 text-xs font-medium tracking-wide text-blue-400 uppercase">Kode Transaksi</span>
                                <span class="text-sm font-semibold text-blue-800">{{ $jtdetail['trans_code'] }}</span>
                            </div>
                            <div class="col-md-3">
                                <span class="block mb-1 text-xs font-medium tracking-wide text-blue-400 uppercase">Nama Transaksi</span>
                                <span class="text-sm font-semibold text-blue-800">{{ $jtdetail['trans_type']['trans_name'] ?? '-' }}</span>
                            </div>
                            <div class="col-md-3">
                                <span class="block mb-1 text-xs font-medium tracking-wide text-blue-400 uppercase">No. Transaksi</span>
                                <span class="text-sm font-semibold text-blue-800">{{ $jtdetail['trans_no'] }}</span>
                            </div>
                            <div class="col-md-3">
                                <span class="block mb-1 text-xs font-medium tracking-wide text-blue-400 uppercase">Tanggal Transaksi</span>
                                <span class="text-sm font-semibold text-blue-800">
                                    {{ !empty($jtdetail['trans_date']) ? \Carbon\Carbon::parse($jtdetail['trans_date'])->format('d M Y') : '-' }}
                                </span>
                            </div>
                        </div>
                    </div>

                    @if ($writable)
                    {{-- Form Detail Entri --}}
                    <div class="p-6 mb-5 bg-white border border-gray-200 rounded-xl">
                        <h6 class="flex items-center mb-5 text-sm font-semibold tracking-wide text-blue-700 uppercase">
                            <i class="mr-2 icon-pencil7"></i> Detail Entri
                        </h6>

                        {{-- Row 1: No Entri + Kode Akun + Divisi --}}
                        <div class="form-row">
                            <div class="form-group col-md-2">
                                <label class="mb-1 text-sm font-medium text-gray-700">No. Entri</label>
                                <input type="text"
                                    class="text-center form-control bg-light @error('count') is-invalid @enderror"
                                    wire:model="count" readonly>
                                @error('count')
                                    <div class="invalid-feedback">{{ $errors->first('count') }}</div>
                                @enderror
                            </div>
                            <div class="form-group col-md-5">
                                <label class="mb-1 text-sm font-medium text-gray-700">Kode Akun <span class="text-red-500">*</span></label>
                                <div wire:ignore>
                                    <select
                                        class="form-control select-search2 @error('jtdetail.acc_code') is-invalid @enderror"
                                        data-fouc wire:model.live="jtdetail.acc_code">
                                        <option value="">{{ __('Pilih Akun') }}</option>
                                        @foreach ($acc as $ac)
                                            <option value="{{ $ac->acc_code }}">
                                                {{ $ac->acc_code }} [{{ $ac->acc_name }}]
                                            </option>
                                        @endforeach
                                    </select>
                                </div>
                                @error('jtdetail.acc_code')
                                    <span class="mt-1 text-xs text-red-500">{{ $message }}</span>
                                @enderror
                            </div>
                            <div class="form-group col-md-5">
                                <label class="mb-1 text-sm font-medium text-gray-700">Divisi <span class="text-red-500">*</span></label>
                                <div wire:ignore>
                                    <select
                                        class="form-control select-search3 @error('jtdetail.div_code') is-invalid @enderror"
                                        data-fouc wire:model.live="jtdetail.div_code">
                                        <option value="">{{ __('Pilih Divisi') }}</option>
                                        @foreach ($div as $dv)
                                            <option value="{{ $dv->div_code }}">
                                                {{ $dv->div_code }} [{{ $dv->div_name }}]
                                            </option>
                                        @endforeach
                                    </select>
                                </div>
                                @error('jtdetail.div_code')
                                    <span class="mt-1 text-xs text-red-500">{{ $message }}</span>
                                @enderror
                            </div>
                        </div>

                        {{-- Row 2: Deskripsi --}}
                        <div class="form-row">
                            <div class="form-group col-md-12">
                                <label class="mb-1 text-sm font-medium text-gray-700">Deskripsi</label>
                                <textarea class="form-control @error('jtdetail.description') is-invalid @enderror"
                                    wire:model.live="jtdetail.description" rows="2" placeholder="Masukkan deskripsi entri..."></textarea>
                                @error('jtdetail.description')
                                    <div class="invalid-feedback">{{ $errors->first('jtdetail.description') }}</div>
                                @enderror
                            </div>
                        </div>

                        {{-- Row 3: Debit + Kredit (input + preview) --}}
                        <div class="form-row">
                            <div class="form-group col-md-3">
                                <label class="mb-1 text-sm font-medium text-gray-700">Debit</label>
                                <input type="text"
                                    class="form-control text-right @error('jtdetail.debetnew') is-invalid @enderror"
                                    wire:model.live="jtdetail.debetnew" placeholder="0">
                                @error('jtdetail.debetnew')
                                    <span class="mt-1 text-xs text-red-500">{{ $errors->first('jtdetail.debetnew') }}</span>
                                @enderror
                            </div>
                            <div class="form-group col-md-3">
                                <label class="mb-1 text-xs font-medium text-gray-400">Format Debit</label>
                                <input type="text" class="form-control bg-light text-right font-weight-bold text-success"
                                    value="{{ number_format((float) ($jtdetail['debetnew'] ?? 0), 2, ',', '.') }}" readonly>
                            </div>
                            <div class="form-group col-md-3">
                                <label class="mb-1 text-sm font-medium text-gray-700">Kredit</label>
                                <input type="text"
                                    class="form-control text-right @error('jtdetail.creditnew') is-invalid @enderror"
                                    wire:model.live="jtdetail.creditnew" placeholder="0">
                                @error('jtdetail.creditnew')
                                    <span class="mt-1 text-xs text-red-500">{{ $errors->first('jtdetail.creditnew') }}</span>
                                @enderror
                            </div>
                            <div class="form-group col-md-3">
                                <label class="mb-1 text-xs font-medium text-gray-400">Format Kredit</label>
                                <input type="text" class="form-control bg-light text-right font-weight-bold text-danger"
                                    value="{{ number_format((float) ($jtdetail['creditnew'] ?? 0), 2, ',', '.') }}" readonly>
                            </div>
                        </div>

                        <div class="my-4 border-t border-gray-100"></div>

                        {{-- Row 4: Sisa Saldo — cards with colored borders --}}
                        @php
                            $newBalanceDifferenceDebet =
                                ($jtdetail['tcredit'] ?? 0) - ($jtdetail['tdebet'] ?? 0) - (int) ($jtdetail['debetnew'] ?? 0);
                            $newBalanceDifferenceCredit =
                                ($jtdetail['tdebet'] ?? 0) - ($jtdetail['tcredit'] ?? 0) - (int) ($jtdetail['creditnew'] ?? 0);
                            $finalBalance =
                                ($jtdetail['tdebet'] ?? 0) + ($jtdetail['debetnew'] ?? 0) -
                                (($jtdetail['tcredit'] ?? 0) + ($jtdetail['creditnew'] ?? 0));
                        @endphp
                        <div class="form-row">
                            <div class="form-group col-md-4">
                                <div class="p-3 border-l-4 {{ $newBalanceDifferenceDebet == 0 ? 'border-green-400 bg-green-50' : 'border-amber-400 bg-amber-50' }} rounded-lg">
                                    <label class="mb-1 text-xs font-medium text-gray-500">Sisa Saldo Debit</label>
                                    <div class="text-lg font-bold {{ $newBalanceDifferenceDebet == 0 ? 'text-green-700' : 'text-amber-700' }} text-right">
                                        {{ number_format((float) $newBalanceDifferenceDebet, 2, ',', '.') }}
                                    </div>
                                </div>
                            </div>
                            <div class="form-group col-md-4">
                                <div class="p-3 border-l-4 {{ $newBalanceDifferenceCredit == 0 ? 'border-green-400 bg-green-50' : 'border-amber-400 bg-amber-50' }} rounded-lg">
                                    <label class="mb-1 text-xs font-medium text-gray-500">Sisa Saldo Kredit</label>
                                    <div class="text-lg font-bold {{ $newBalanceDifferenceCredit == 0 ? 'text-green-700' : 'text-amber-700' }} text-right">
                                        {{ number_format((float) $newBalanceDifferenceCredit, 2, ',', '.') }}
                                    </div>
                                </div>
                            </div>
                            <div class="form-group col-md-4">
                                <div class="p-3 border-l-4 {{ $finalBalance == 0 ? 'border-green-400 bg-green-50' : 'border-red-400 bg-red-50' }} rounded-lg">
                                    <label class="mb-1 text-xs font-medium text-gray-500">Status Saldo</label>
                                    <div class="d-flex align-items-center justify-content-between">
                                        @if (($jtdetail['tdebet'] ?? 0) + ($jtdetail['debetnew'] ?? 0) == ($jtdetail['tcredit'] ?? 0) + ($jtdetail['creditnew'] ?? 0))
                                            <span class="badge badge-success"><i class="mr-1 icon-check"></i> SEIMBANG</span>
                                        @else
                                            <span class="badge badge-warning"><i class="mr-1 icon-alert"></i> BELUM SEIMBANG</span>
                                        @endif
                                        <span class="text-sm font-bold {{ $finalBalance == 0 ? 'text-green-700' : 'text-red-600' }}">
                                            Selisih: {{ number_format(abs($finalBalance), 2, ',', '.') }}
                                        </span>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>

                    @endif

                    {{-- Action Buttons --}}
                    <div class="mb-5 d-flex justify-content-between align-items-center">
                        <button type="button" class="btn btn-light border" wire:click="batal">
                            <i class="mr-1 icon-arrow-left16"></i>Batal
                        </button>
                        @if ($writable)
                            <button type="submit" class="btn btn-primary" wire:loading.attr="disabled">
                                <i class="mr-1 icon-floppy-disk"></i>Simpan Detail
                                <span wire:loading wire:target="saveDetail" class="spinner-border spinner-border-sm ml-1"></span>
                            </button>
                        @endif
                    </div>

                    {{-- Tabel Daftar Detail --}}
                    <h6 class="flex items-center mb-3 text-sm font-semibold text-gray-700">
                        <i class="mr-2 icon-list-unordered"></i> Daftar Detail Transaksi
                    </h6>
                    <div class="datatable-scroll">
                        <table class="table table-hover">
                            <thead>
                                <tr role="row">
                                    <th class="text-center" style="width: 40px">#</th>
                                    <th style="width: 60px">Entri</th>
                                    <th>Akun</th>
                                    <th>Divisi</th>
                                    <th>Deskripsi</th>
                                    <th class="text-right">Debit</th>
                                    <th class="text-right">Kredit</th>
                                    <th class="text-center">Mata Uang</th>
                                    @if ($writable)
                                        <th class="text-center" style="width: 60px">Aksi</th>
                                    @endif
                                </tr>
                            </thead>
                            <tbody>
                                @forelse ($jtdetail['trans_detail'] as $row)
                                    <tr role="row">
                                        <td class="text-center text-gray-400">{{ $loop->iteration }}</td>
                                        <td class="text-center">
                                            <span class="inline-flex items-center justify-center rounded-full" style="width: 1.75rem; height: 1.75rem; font-size: 0.75rem; font-weight: 600; color: #1e40af; background-color: #dbeafe;">{{ $row['trans_count'] }}</span>
                                        </td>
                                        <td>
                                            <span class="text-sm font-medium text-gray-800">{{ $row['acc_code'] }}</span>
                                            <span class="text-xs text-gray-400">[{{ $acc->firstWhere('acc_code', $row['acc_code'])?->acc_name ?? '-' }}]</span>
                                        </td>
                                        <td>
                                            <span class="text-sm font-medium text-gray-800">{{ $row['div_code'] }}</span>
                                            <span class="text-xs text-gray-400">[{{ $div->firstWhere('div_code', $row['div_code'])?->div_name ?? '-' }}]</span>
                                        </td>
                                        <td class="text-sm text-gray-600">{{ $row['description'] }}</td>
                                        <td class="text-right font-semibold text-emerald-600">{{ number_format($row['debet'] ?? 0, 2, ',', '.') }}</td>
                                        <td class="text-right font-semibold text-red-500">{{ number_format($row['credit'] ?? 0, 2, ',', '.') }}</td>
                                        <td class="text-center">
                                            <span class="px-2 py-1 rounded-md" style="font-size: 0.75rem; font-weight: 500; color: #475569; background-color: #f1f5f9; border: 1px solid #e2e8f0;">{{ $row['curr_code'] }}</span>
                                        </td>
                                        @if ($writable)
                                            <td class="text-center">
                                                <div class="list-icons">
                                                    <div class="dropdown">
                                                        <a href="#" class="list-icons-item" data-toggle="dropdown">
                                                            <i class="icon-menu9"></i>
                                                        </a>
                                                        <div class="dropdown-menu dropdown-menu-right">
                                                            @if ($writable)
                                                                <a href="#" class="dropdown-item"
                                                                    wire:click.prevent='detail("{{ $row['trans_no'] }}","{{ $row['id'] }}")'>
                                                                    <i class="icon-pencil"></i> Edit
                                                                </a>
                                                            @endif
                                                            @if ($writable)
                                                                <a href="#" class="dropdown-item text-danger"
                                                                    wire:click.prevent='confirmDeleteDetail("{{ $row['id'] }}")'>
                                                                    <i class="icon-trash"></i> Hapus
                                                                </a>
                                                            @endif
                                                        </div>
                                                    </div>
                                                </div>
                                            </td>
                                        @endif
                                    </tr>
                                @empty
                                    <tr>
                                        <td colspan="9" class="py-8 text-center">
                                            <div class="flex flex-col items-center justify-center">
                                                <i class="mb-3 icon-file-empty2" style="font-size: 2.5rem; color: #cbd5e1"></i>
                                                <p class="text-sm text-gray-400">Belum ada detail transaksi.</p>
                                            </div>
                                        </td>
                                    </tr>
                                @endforelse
                            </tbody>
                            <tfoot>
                                <tr>
                                    <th colspan="5" class="text-right font-weight-bold">Total:</th>
                                    <th class="text-right font-semibold text-emerald-600">
                                        {{ number_format(array_sum(array_column($jtdetail['trans_detail'], 'debet')), 2, ',', '.') }}
                                    </th>
                                    <th class="text-right font-semibold text-red-500">
                                        {{ number_format(array_sum(array_column($jtdetail['trans_detail'], 'credit')), 2, ',', '.') }}
                                    </th>
                                    <th></th>
                                    <th></th>
                                </tr>
                                <tr>
                                    <th colspan="5" class="text-right">Status:</th>
                                    <th colspan="4" class="text-left">
                                        @php
                                            $totalDebetDetail = array_sum(array_column($jtdetail['trans_detail'], 'debet'));
                                            $totalCreditDetail = array_sum(array_column($jtdetail['trans_detail'], 'credit'));
                                        @endphp
                                        @if ($totalDebetDetail == $totalCreditDetail && $totalDebetDetail > 0)
                                            <span class="badge badge-success"><i class="mr-1 icon-check"></i> SEIMBANG</span>
                                        @elseif ($totalDebetDetail != $totalCreditDetail && ($totalDebetDetail > 0 || $totalCreditDetail > 0))
                                            <span class="badge badge-warning"><i class="mr-1 icon-alert"></i> BELUM SEIMBANG (Selisih:
                                                {{ number_format(abs($totalDebetDetail - $totalCreditDetail), 2, ',', '.') }})</span>
                                        @else
                                            <span class="badge badge-secondary">Tidak Ada Entri</span>
                                        @endif
                                    </th>
                                </tr>
                            </tfoot>
                        </table>
                    </div>
                </form>

            @elseif ($form)
                {{-- ============================================ --}}
                {{-- HEADER FORM --}}
                {{-- ============================================ --}}
                <form action="#" wire:submit.prevent="save">
                    <div class="p-6 mb-6 bg-white border border-gray-200 rounded-xl">
                        <h6 class="flex items-center mb-5 text-sm font-semibold tracking-wide text-blue-700 uppercase">
                            <i class="mr-2 icon-pencil7"></i> {{ $idNya ? 'Edit ' : 'Input ' }} History Jurnal Transaksi
                        </h6>

                        {{-- Row 1: Tipe Transaksi + Nomor Transaksi --}}
                        <div class="form-row">
                            <div class="form-group col-md-6">
                                <label class="mb-1 text-sm font-medium text-gray-700">{{ __('Tipe Transaksi') }} <span class="text-red-500">*</span></label>
                                <div wire:ignore>
                                    <select
                                        class="form-control select-search @error('jt.trans_code') is-invalid @enderror"
                                        data-fouc wire:model.live="jt.trans_code"
                                        @if ($idNya) disabled @endif>
                                        <option value="">{{ __('Pilih Tipe Transaksi') }}</option>
                                        @foreach ($transaction_code as $trans)
                                            <option value="{{ $trans->trans_code }}">{{ $trans->trans_name }}</option>
                                        @endforeach
                                    </select>
                                </div>
                                @error('jt.trans_code')
                                    <span class="mt-1 text-xs text-red-500">{{ $message }}</span>
                                @enderror
                            </div>
                            <div class="form-group col-md-6">
                                <label class="mb-1 text-sm font-medium text-gray-700">{{ __('Nomor Transaksi') }} <span class="text-red-500">*</span></label>
                                <input type="text" class="form-control @error('jt.trans_no') is-invalid @enderror"
                                    wire:model.live="jt.trans_no" placeholder="Masukkan nomor transaksi...">
                                @error('jt.trans_no')
                                    <span class="mt-1 text-xs text-red-500">{{ $message }}</span>
                                @enderror
                            </div>
                        </div>

                        {{-- Row 2: Tanggal Transaksi + Pengguna --}}
                        <div class="form-row">
                            <div class="form-group col-md-6">
                                <label class="mb-1 text-sm font-medium text-gray-700">{{ __('Tanggal Transaksi') }} <span class="text-red-500">*</span></label>
                                <div class="input-group" wire:ignore>
                                    <span class="input-group-prepend">
                                        <span class="input-group-text"><i class="icon-calendar22"></i></span>
                                    </span>
                                    <input type="text"
                                        class="form-control daterange-single @error('jt.trans_date') is-invalid @enderror"
                                        wire:model.live="jt.trans_date"
                                        value="{{ !empty($jt['trans_date']) ? $jt['trans_date'] : \Carbon\Carbon::now()->format('m-d-Y') }}">
                                </div>
                                @error('jt.trans_date')
                                    <span class="mt-1 text-xs text-red-500">{{ $message }}</span>
                                @enderror
                            </div>
                            <div class="form-group col-md-6">
                                <label class="mb-1 text-sm font-medium text-gray-700">{{ __('Pengguna') }}</label>
                                <div class="input-group">
                                    <span class="input-group-prepend">
                                        <span class="input-group-text"><i class="icon-user"></i></span>
                                    </span>
                                    <input type="text" class="form-control bg-light" value="{{ Auth::user()->user_name }}" readonly>
                                </div>
                            </div>
                        </div>

                        <div class="my-4 border-t border-gray-100"></div>

                        {{-- Row 3: Total Debit + Total Kredit + Status --}}
                        <div class="form-row">
                            <div class="form-group col-md-4">
                                <label class="mb-1 text-sm font-medium text-gray-700">{{ __('Total Debit') }}</label>
                                <input type="text" class="form-control bg-light font-weight-bold text-success text-right"
                                    value="{{ number_format($jt['debet'] ?? 0, 2, ',', '.') }}" readonly>
                            </div>
                            <div class="form-group col-md-4">
                                <label class="mb-1 text-sm font-medium text-gray-700">{{ __('Total Kredit') }}</label>
                                <input type="text" class="form-control bg-light font-weight-bold text-danger text-right"
                                    value="{{ number_format($jt['credit'] ?? 0, 2, ',', '.') }}" readonly>
                            </div>
                            <div class="form-group col-md-4">
                                <label class="mb-1 text-sm font-medium text-gray-700">{{ __('Status Saldo') }}</label>
                                @php
                                    $isHeadBalanced = ($jt['debet'] ?? 0) == ($jt['credit'] ?? 0) && ($jt['debet'] ?? 0) > 0;
                                    $hasHeadEntries = ($jt['debet'] ?? 0) > 0 || ($jt['credit'] ?? 0) > 0;
                                @endphp
                                <div class="form-control bg-light d-flex align-items-center" style="pointer-events: none;">
                                    @if ($isHeadBalanced)
                                        <span class="badge badge-success"><i class="mr-1 icon-check"></i> SEIMBANG</span>
                                    @elseif ($hasHeadEntries)
                                        <span class="badge badge-warning"><i class="mr-1 icon-alert"></i> Selisih:
                                            {{ number_format(abs(($jt['debet'] ?? 0) - ($jt['credit'] ?? 0)), 2, ',', '.') }}</span>
                                    @else
                                        <span class="badge badge-secondary">Tidak Ada Entri</span>
                                    @endif
                                </div>
                            </div>
                        </div>

                        {{-- Row 4: Timestamp info --}}
                        <div class="form-row">
                            <div class="form-group col-md-12 mb-0">
                                <span class="text-xs text-gray-400">
                                    <i class="mr-1 icon-history"></i>Terakhir diperbarui: {{ \Carbon\Carbon::now()->format('d M Y, H:i:s') }}
                                </span>
                            </div>
                        </div>
                    </div>

                    <div class="d-flex justify-content-between align-items-center">
                        <button type="button" class="btn btn-light border" wire:click="clear">
                            <i class="mr-1 icon-arrow-left16"></i>Batal
                        </button>
                        <button type="submit" class="btn btn-primary" wire:loading.attr="disabled">
                            <i class="mr-1 icon-floppy-disk"></i>Simpan
                            <span wire:loading wire:target="save" class="spinner-border spinner-border-sm ml-1"></span>
                        </button>
                    </div>
                </form>

            @else
                {{-- ============================================ --}}
                {{-- LIST VIEW --}}
                {{-- ============================================ --}}
                @if ($writable)
                    <div class="mb-5">
                        <button type="button" class="btn btn-primary btn-lg rounded-lg shadow-sm" wire:click="add">
                            <i class="mr-2 icon-plus-circle2"></i>Tambah History Transaksi Jurnal
                        </button>
                    </div>
                @endif

                {{-- Filter and Search Section --}}
                <div class="p-4 mb-5 bg-white border border-gray-200 rounded-xl">
                    <div class="row align-items-end">
                        <div class="mb-3 col-md-3 mb-md-0">
                            <label class="mb-1 text-sm font-medium text-gray-700">Cari Transaksi</label>
                            <div class="search-input">
                                <input type="search" class="form-control"
                                    placeholder="Cari Kode atau No. Transaksi..." wire:model.live='search'>
                            </div>
                        </div>
                        <div class="mb-3 col-md-2 mb-md-0">
                            <label class="mb-1 text-sm font-medium text-gray-700">Urutkan</label>
                            <div class="btn-group justify-content-center w-100">
                                <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                                    aria-expanded="false">
                                    @if ($order == 'DESC')
                                        Terbaru
                                    @else
                                        Terlama
                                    @endif
                                </a>
                                <div class="dropdown-menu" x-placement="bottom-start">
                                    <a class="dropdown-item" href="#" wire:click.prevent="sort('DESC')">Terbaru</a>
                                    <a class="dropdown-item" href="#" wire:click.prevent="sort('ASC')">Terlama</a>
                                </div>
                            </div>
                        </div>
                        <div class="mb-3 col-md-2 mb-md-0">
                            <label class="mb-1 text-sm font-medium text-gray-700">Tampilkan</label>
                            <div class="btn-group justify-content-center w-100">
                                <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                                    aria-expanded="false">
                                    {{ $limit }} item
                                </a>
                                <div class="dropdown-menu" x-placement="bottom-start">
                                    <a class="dropdown-item" href="#" wire:click.prevent="limits('10')">10 item</a>
                                    <a class="dropdown-item" href="#" wire:click.prevent="limits('30')">30 item</a>
                                    <a class="dropdown-item" href="#" wire:click.prevent="limits('50')">50 item</a>
                                    <a class="dropdown-item" href="#" wire:click.prevent="limits('100')">100 item</a>
                                </div>
                            </div>
                        </div>
                        <div class="mb-3 col-md-2 mb-md-0">
                            <label class="mb-1 text-sm font-medium text-gray-700">Filter Status</label>
                            <div class="btn-group justify-content-center w-100">
                                <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                                    aria-expanded="false">
                                    @if ($filter == 'b')
                                        Seimbang
                                    @elseif ($filter == 'u')
                                        Belum Seimbang
                                    @else
                                        Semua
                                    @endif
                                </a>
                                <div class="dropdown-menu" x-placement="bottom-start">
                                    <a class="dropdown-item" href="#" wire:click.prevent="filt('a')">Semua</a>
                                    <a class="dropdown-item" href="#" wire:click.prevent="filt('b')">Seimbang</a>
                                    <a class="dropdown-item" href="#" wire:click.prevent="filt('u')">Belum Seimbang</a>
                                </div>
                            </div>
                        </div>
                        <div class="col-md-3">
                            <label class="mb-1 text-sm font-medium text-gray-700">Bulan</label>
                            <div class="btn-group justify-content-center w-100">
                                <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                                    aria-expanded="false">
                                    @if ($this->bulan)
                                        <i class="mr-1 icon-calendar5"></i>
                                        {{ $this->bulan == 13 ? 'Adj' : \Carbon\Carbon::create()->month($this->bulan)->format('F') }}
                                        {{ App\Models\Parameter::first()->year }}
                                    @else
                                        <i class="mr-1 icon-calendar5"></i> Semua Bulan
                                    @endif
                                </a>
                                <div class="dropdown-menu" x-placement="bottom-start">
                                    <a class="dropdown-item" href="#" wire:click.prevent="month(0)">
                                        Semua Bulan
                                    </a>
                                    @foreach ($monthsAndYears as $item)
                                        <a class="dropdown-item" href="#"
                                            wire:click.prevent="month({{ $loop->iteration }})">
                                            {{ $item['month'] }} {{ App\Models\Parameter::first()->year }}
                                        </a>
                                    @endforeach
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                {{-- Data Table --}}
                <div class="datatable-scroll">
                    <table class="table table-hover">
                        <thead>
                            <tr role="row">
                                <th class="text-center" style="width: 40px">#</th>
                                <th>Periode</th>
                                <th>Kode Transaksi</th>
                                <th>No. Transaksi</th>
                                <th>Tanggal</th>
                                <th class="text-right">Total Debit</th>
                                <th class="text-right">Total Kredit</th>
                                <th class="text-center">Status Saldo</th>
                                @can('viewhistory_journal_detail')
                                    <th class="text-center">Detail</th>
                                @endcan
                                @if ($writable)
                                    <th class="text-center" style="width: 60px">Aksi</th>
                                @endif
                            </tr>
                        </thead>
                        <tbody>
                            @forelse ($data as $row)
                                @php
                                    $totalDebet = $row->transDetail->sum('debet');
                                    $totalCredit = $row->transDetail->sum('credit');
                                    $isBalanced = $totalDebet == $totalCredit && $totalDebet > 0;
                                    $hasEntries = $totalDebet > 0 || $totalCredit > 0;
                                @endphp
                                <tr role="row">
                                    <td class="text-center text-gray-400">{{ $loop->iteration }}</td>
                                    <td>
                                        <span class="px-2 py-1 rounded-md" style="font-size: 0.75rem; font-weight: 500; color: #475569; background-color: #f1f5f9; border: 1px solid #e2e8f0;">{{ $row->period }}</span>
                                    </td>
                                    <td>
                                        <span class="px-2 py-1 rounded-md" style="background-color: #dbeafe; color: #1e40af; font-size: 0.75rem; font-weight: 600; border: 1px solid #bfdbfe;">{{ $row->trans_code }}</span>
                                    </td>
                                    <td><span class="font-semibold text-gray-800">{{ $row->trans_no }}</span></td>
                                    <td class="text-gray-600">
                                        @php
                                            try {
                                                $formattedDate = \Carbon\Carbon::parse($row->trans_date)->format('d M Y');
                                            } catch (\Exception $e) {
                                                $formattedDate = $row->trans_date;
                                            }
                                        @endphp
                                        {{ $formattedDate }}
                                    </td>
                                    <td class="text-right font-semibold text-emerald-600">{{ number_format($totalDebet ?? 0, 2, ',', '.') }}</td>
                                    <td class="text-right font-semibold text-red-500">{{ number_format($totalCredit ?? 0, 2, ',', '.') }}</td>
                                    <td class="text-center">
                                        @if ($isBalanced)
                                            <span class="badge badge-success"><i class="mr-1 icon-check"></i> Seimbang</span>
                                        @elseif ($hasEntries)
                                            <span class="badge badge-warning"><i class="mr-1 icon-alert"></i> Belum Seimbang</span>
                                        @else
                                            <span class="badge badge-secondary">Tidak Ada Entri</span>
                                        @endif
                                    </td>
                                    @can('viewhistory_journal_detail')
                                        <td class="text-center">
                                            <a href="#" wire:click.prevent='detail("{{ Crypt::encrypt($row->trans_no) }}")'
                                                class="btn btn-sm btn-outline-info">
                                                <i class="mr-1 icon-list-unordered"></i> Lihat
                                            </a>
                                        </td>
                                    @endcan
                                    @if ($writable)
                                        <td class="text-center">
                                            <div class="list-icons">
                                                <div class="dropdown">
                                                    <a href="#" class="list-icons-item" data-toggle="dropdown">
                                                        <i class="icon-menu9"></i>
                                                    </a>
                                                    <div class="dropdown-menu dropdown-menu-right">
                                                        @if ($writable)
                                                            <a href="#" class="dropdown-item"
                                                                wire:click.prevent='add("{{ Crypt::encrypt($row->trans_no) }}")'>
                                                                <i class="icon-pencil"></i> Edit
                                                            </a>
                                                        @endif
                                                        @if ($writable)
                                                            <a href="#" class="dropdown-item text-danger"
                                                                wire:click.prevent='confirmDelete("{{ Crypt::encrypt($row->trans_no) }}")'>
                                                                <i class="icon-trash"></i> Hapus
                                                            </a>
                                                        @endif
                                                    </div>
                                                </div>
                                            </div>
                                        </td>
                                    @endif
                                </tr>
                            @empty
                                <tr>
                                    <td colspan="10" class="py-8 text-center">
                                        <div class="flex flex-col items-center justify-center">
                                            <i class="mb-3 icon-file-empty2" style="font-size: 2.5rem; color: #cbd5e1"></i>
                                            <p class="text-sm text-gray-400">Tidak ada data history transaksi jurnal yang ditemukan.</p>
                                        </div>
                                    </td>
                                </tr>
                            @endforelse
                        </tbody>
                    </table>
                </div>
                <div class="mt-4">
                    {{ $data->links() }}
                </div>
            @endif
        </div>
    </div>
</div>
