<form action="#" wire:submit.prevent="save">
    <div class="p-6 mb-6 bg-white border border-gray-200 rounded-xl">
        <h6 class="flex items-center mb-5 text-sm font-semibold tracking-wide text-blue-700 uppercase">
            <i class="mr-2 icon-pencil7"></i> {{ $idNya ? 'Edit ' : 'Input ' }} Transaksi Jurnal
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
                        class="form-control daterange-ranges @error('jt.trans_date') is-invalid @enderror"
                        wire:model.live="jt.trans_date"
                        value="{{ !empty($jt['trans_date']) ? \Carbon\Carbon::parse($jt['trans_date'])->format('d-M-Y') : \Carbon\Carbon::now()->format('d-M-Y') }}">
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
