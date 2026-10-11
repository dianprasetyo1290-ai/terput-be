<form action="#" wire:submit.prevent="saveDetail">
    @php $locked = ($jtdetail['status'] ?? 'draft') !== 'draft'; @endphp
    @if ($locked)
        <div class="alert alert-warning mb-4">
            Jurnal ini berstatus <strong>{{ $jtdetail['status'] }}</strong> sehingga tidak dapat diubah.
            Koreksi jurnal posted dilakukan melalui jurnal pembalik di menu Jurnal.
        </div>
    @endif
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
                        {{ number_format((float) ($this->sisasaldodebet ?? 0), 2, ',', '.') }}
                    </div>
                </div>
            </div>
            <div class="form-group col-md-4">
                <div class="p-3 border-l-4 {{ $newBalanceDifferenceCredit == 0 ? 'border-green-400 bg-green-50' : 'border-amber-400 bg-amber-50' }} rounded-lg">
                    <label class="mb-1 text-xs font-medium text-gray-500">Sisa Saldo Kredit</label>
                    <div class="text-lg font-bold {{ $newBalanceDifferenceCredit == 0 ? 'text-green-700' : 'text-amber-700' }} text-right">
                        {{ number_format((float) ($this->sisasaldokredit ?? 0), 2, ',', '.') }}
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

    {{-- Action Buttons --}}
    <div class="mb-5 d-flex justify-content-between align-items-center">
        <button type="button" class="btn btn-light border" wire:click="batal">
            <i class="mr-1 icon-arrow-left16"></i>Batal
        </button>
        @unless ($locked)
        <button type="submit" class="btn btn-primary" wire:loading.attr="disabled">
            <i class="mr-1 icon-floppy-disk"></i>Simpan Detail
            <span wire:loading wire:target="saveDetail" class="spinner-border spinner-border-sm ml-1"></span>
        </button>
        @endunless
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
                    <th class="text-center" style="width: 60px">Aksi</th>
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
                        <td class="text-center">
                            @unless ($locked)
                            <div class="list-icons">
                                <div class="dropdown">
                                    <a href="#" class="list-icons-item" data-toggle="dropdown">
                                        <i class="icon-menu9"></i>
                                    </a>
                                    <div class="dropdown-menu dropdown-menu-right">
                                        <a href="#" class="dropdown-item"
                                            wire:click.prevent='detail("{{ Crypt::encrypt($row['trans_no']) }}","{{ $row['id'] }}")'>
                                            <i class="icon-pencil"></i> Edit
                                        </a>
                                        <a href="#" class="dropdown-item text-danger"
                                            wire:click.prevent='confirmDeleteDetail("{{ $row['id'] }}")'>
                                            <i class="icon-trash"></i> Hapus
                                        </a>
                                    </div>
                                </div>
                            </div>
                            @endunless
                        </td>
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
