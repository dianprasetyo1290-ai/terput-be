<div>
    {{-- Slot for Page Header --}}
    <x-slot name="header">
        <livewire:global.page-header judul="Transaksi Jurnal" subjudul="Input Transaksi Keuangan" :breadcrumb="['Transaksi', 'Jurnal']" />
    </x-slot>

    {{-- Main Card for the Form --}}
    <div class="rounded-lg shadow-lg card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Form Input Transaksi Jurnal</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>

        <div class="card-body">
            {{-- Session Feedback Messages (success/error) --}}
            @if (session()->has('message'))
                <div class="alert alert-success alert-dismissible fade show" role="alert">
                    {{ session('message') }}
                    <button type="button" class="close" data-dismiss="alert" aria-label="Close">
                        <span aria-hidden="true">&times;</span>
                    </button>
                </div>
            @endif
            @if (session()->has('error'))
                <div class="alert alert-danger alert-dismissible fade show" role="alert">
                    {{ session('error') }}
                    <button type="button" class="close" data-dismiss="alert" aria-label="Close">
                        <span aria-hidden="true">&times;</span>
                    </button>
                </div>
            @endif

            {{-- Main Form with Livewire Submit --}}
            <form wire:submit.prevent="saveTransaction">
                {{-- Transaction Header Section --}}
                <fieldset class="mb-3">
                    <legend class="text-uppercase font-size-sm font-weight-bold">Informasi Header Transaksi</legend>
                    <div class="row">
                        <div class="col-md-4">
                            <div class="form-group">
                                <label>Nomor Transaksi:</label>
                                {{-- Hapus atribut 'readonly' di sini --}}
                                <input type="text" wire:model.defer="transNo" class="form-control">
                                @error('transNo')
                                    <span class="text-danger">{{ $message }}</span>
                                @enderror
                            </div>
                        </div>
                        <div class="col-md-4">
                            <div class="form-group">
                                <label>Tanggal Transaksi:</label>
                                <input type="date" wire:model.defer="transDate" class="form-control">
                                @error('transDate')
                                    <span class="text-danger">{{ $message }}</span>
                                @enderror
                            </div>
                        </div>
                        <div class="col-md-4">
                            <div class="form-group">
                                <label>Kode Transaksi:</label>
                                <select wire:model.defer="transCode" class="form-control"
                                    data-placeholder="Pilih Kode Transaksi">
                                    <option value="">Pilih Kode Transaksi</option>
                                    @foreach ($masterTransactionTypes as $type)
                                        <option value="{{ $type->trans_code }}">{{ $type->trans_code }} -
                                            {{ $type->trans_name }}</option>
                                    @endforeach
                                </select>
                                @error('transCode')
                                    <span class="text-danger">{{ $message }}</span>
                                @enderror
                            </div>
                        </div>
                    </div>

                    <div class="form-group">
                        <label>Deskripsi Header (Opsional):</label>
                        <textarea wire:model.defer="headerDescription" class="form-control" rows="3"
                            placeholder="Deskripsi umum transaksi"></textarea>
                        @error('headerDescription')
                            <span class="text-danger">{{ $message }}</span>
                        @enderror
                    </div>

                    <div class="row">
                        <div class="col-md-6">
                            <div class="form-group">
                                <label>Total Debet (Header):</label>
                                <input type="text" wire:model.defer="debetAmountHeader"
                                    class="text-right form-control" placeholder="0.00" readonly>
                                @error('debetAmountHeader')
                                    <span class="text-danger">{{ $message }}</span>
                                @enderror
                            </div>
                        </div>
                        <div class="col-md-6">
                            <div class="form-group">
                                <label>Total Kredit (Header):</label>
                                <input type="text" wire:model.defer="creditAmountHeader"
                                    class="text-right form-control" placeholder="0.00" readonly>
                                @error('creditAmountHeader')
                                    <span class="text-danger">{{ $message }}</span>
                                @enderror
                            </div>
                        </div>
                    </div>
                </fieldset>

                {{-- Transaction Detail Section --}}
                <fieldset class="mb-3">
                    <legend class="text-uppercase font-size-sm font-weight-bold">Detail Transaksi</legend>

                    <div class="table-responsive">
                        <table class="table table-bordered">
                            <thead>
                                <tr>
                                    <th style="width: 50px;">#</th>
                                    <th style="width: 25%;">Kode Akun</th>
                                    <th style="width: 15%;">Divisi</th>
                                    <th>Deskripsi Detail</th>
                                    <th style="width: 15%;">Debet</th>
                                    <th style="width: 15%;">Kredit</th>
                                    <th style="width: 80px;">Aksi</th>
                                </tr>
                            </thead>
                            <tbody>
                                @forelse($details as $index => $detail)
                                    <tr wire:key="detail-row-{{ $index }}">
                                        <td>{{ $index + 1 }}</td>
                                        <td>
                                            {{-- Select2 for Account --}}
                                            <div wire:ignore> {{-- PENTING: wire:ignore di sini --}}
                                                <select class="form-control select2-account"
                                                    data-index="{{ $index }}" data-placeholder="Pilih Akun"
                                                    x-init="// Inisialisasi Select2
                                                    $($el).select2({
                                                        placeholder: 'Pilih Akun',
                                                        allowClear: true,
                                                        ajax: {
                                                            url: '{{ route('api.search-accounts') }}',
                                                            dataType: 'json',
                                                            delay: 250,
                                                            data: function(params) {
                                                                return {
                                                                    search: params.term,
                                                                    page: params.page
                                                                };
                                                            },
                                                            processResults: function(data, params) {
                                                                params.page = params.page || 1;
                                                                return {
                                                                    results: $.map(data.data, function(item) {
                                                                        return {
                                                                            id: item.acc_code,
                                                                            text: item.acc_code + ' - ' + item.acc_name
                                                                        }
                                                                    }),
                                                                    pagination: {
                                                                        more: data.current_page < data.last_page
                                                                    }
                                                                };
                                                            },
                                                            cache: true
                                                        }
                                                    }).on('change', function() {
                                                        // Update Livewire property on change
                                                        @this.set('details.{{ $index }}.acc_code', $(this).val());
                                                    });

                                                    // Set nilai awal jika sudah ada
                                                    const initialAccCode = @this.details[{{ $index }}].acc_code;
                                                    if (initialAccCode) {
                                                        // Fetch display text from Livewire component
                                                        Livewire.find(this.$el.closest('[wire\\:id]').getAttribute('wire:id'))
                                                            .getAccountDisplayName(initialAccCode)
                                                            .then(text => {
                                                                if (text) {
                                                                    // Menggunakan template literals untuk string concatenation yang aman
                                                                    if (!$el.find(`option[value='${initialAccCode}']`).length) {
                                                                        const newOption = new Option(text, initialAccCode, true, true);
                                                                        $el.append(newOption);
                                                                    }
                                                                    $el.val(initialAccCode).trigger('change');
                                                                }
                                                            });
                                                    }" id="account-select-{{ $index }}">
                                                    <option value="">Pilih Akun</option>
                                                    {{-- Opsi ini penting untuk inisialisasi awal Select2 --}}
                                                    @if ($detail['acc_code'])
                                                        <option value="{{ $detail['acc_code'] }}" selected>
                                                            {{ $detail['acc_code'] }} -
                                                            {{ optional($masterAccounts->firstWhere('acc_code', $detail['acc_code']))->acc_name }}
                                                        </option>
                                                    @endif
                                                </select>
                                            </div>
                                            @error('details.' . $index . '.acc_code')
                                                <span class="text-danger d-block">{{ $message }}</span>
                                            @enderror
                                        </td>
                                        <td>
                                            {{-- Select2 for Division --}}
                                            <div wire:ignore> {{-- PENTING: wire:ignore di sini --}}
                                                <select class="form-control select2-division"
                                                    data-index="{{ $index }}" data-placeholder="Pilih Divisi"
                                                    x-init="// Inisialisasi Select2
                                                    $($el).select2({
                                                        placeholder: 'Pilih Divisi',
                                                        allowClear: true,
                                                        ajax: {
                                                            url: '{{ route('api.search-divisions') }}',
                                                            dataType: 'json',
                                                            delay: 250,
                                                            data: function(params) {
                                                                return {
                                                                    search: params.term,
                                                                    page: params.page
                                                                };
                                                            },
                                                            processResults: function(data, params) {
                                                                params.page = params.page || 1;
                                                                return {
                                                                    results: $.map(data.data, function(item) {
                                                                        return {
                                                                            id: item.div_code,
                                                                            text: item.div_code + ' - ' + item.div_name
                                                                        }
                                                                    }),
                                                                    pagination: {
                                                                        more: data.current_page < data.last_page
                                                                    }
                                                                };
                                                            },
                                                            cache: true
                                                        }
                                                    }).on('change', function() {
                                                        // Update Livewire property on change
                                                        @this.set('details.{{ $index }}.div_code', $(this).val());
                                                    });

                                                    // Set nilai awal jika sudah ada
                                                    const initialDivCode = @this.details[{{ $index }}].div_code;
                                                    if (initialDivCode) {
                                                        Livewire.find(this.$el.closest('[wire\\:id]').getAttribute('wire:id'))
                                                            .getDivisionDisplayName(initialDivCode)
                                                            .then(text => {
                                                                if (text) {
                                                                    if (!$el.find(`option[value='${initialDivCode}']`).length) {
                                                                        const newOption = new Option(text, initialDivCode, true, true);
                                                                        $el.append(newOption);
                                                                    }
                                                                    $el.val(initialDivCode).trigger('change');
                                                                }
                                                            });
                                                    }" id="division-select-{{ $index }}">
                                                    <option value="">Pilih Divisi</option>
                                                    {{-- Opsi ini penting untuk inisialisasi awal Select2 --}}
                                                    @if ($detail['div_code'])
                                                        <option value="{{ $detail['div_code'] }}" selected>
                                                            {{ $detail['div_code'] }} -
                                                            {{ optional($masterDivisions->firstWhere('div_code', $detail['div_code']))->div_name }}
                                                        </option>
                                                    @endif
                                                </select>
                                            </div>
                                            @error('details.' . $index . '.div_code')
                                                <span class="text-danger d-block">{{ $message }}</span>
                                            @enderror
                                        </td>
                                        <td>
                                            <textarea wire:model.defer="details.{{ $index }}.description" class="form-control" rows="2"
                                                placeholder="Deskripsi untuk baris ini"></textarea>
                                            @error('details.' . $index . '.description')
                                                <span class="text-danger d-block">{{ $message }}</span>
                                            @enderror
                                        </td>
                                        <td>
                                            {{-- DEBET INPUT DENGAN TYPE="NUMBER" --}}
                                            <input type="number" wire:model.live="details.{{ $index }}.debet"
                                                wire:key="debet-{{ $index }}" class="text-right form-control"
                                                placeholder="0" step="any" min="0">
                                            @error('details.' . $index . '.debet')
                                                <span class="text-danger d-block">{{ $message }}</span>
                                            @enderror
                                        </td>
                                        <td>
                                            {{-- CREDIT INPUT DENGAN TYPE="NUMBER" --}}
                                            <input type="number"
                                                wire:model.live="details.{{ $index }}.credit"
                                                wire:key="credit-{{ $index }}" class="text-right form-control"
                                                placeholder="0" step="any" min="0">
                                            @error('details.' . $index . '.credit')
                                                <span class="text-danger d-block">{{ $message }}</span>
                                            @enderror
                                        </td>
                                        <td>
                                            <button type="button" wire:click="removeDetailRow({{ $index }})"
                                                class="btn btn-danger btn-sm"><i class="icon-trash"></i></button>
                                        </td>
                                    </tr>
                                @empty
                                    <tr>
                                        <td colspan="7" class="text-center">Tidak ada detail transaksi. Klik
                                            "Tambah Baris Detail" untuk menambahkan.</td>
                                    </tr>
                                @endforelse
                            </tbody>
                        </table>
                    </div>
                    <div class="mt-3 text-right">
                        <button type="button" wire:click="addDetailRow" class="btn btn-info btn-sm"><i
                                class="mr-2 icon-plus22"></i> Tambah Baris Detail</button>
                    </div>
                </fieldset>

                {{-- Save Button --}}
                <div class="text-right">
                    <button type="submit" class="btn btn-primary">Simpan Transaksi <i
                            class="ml-2 icon-paperplane"></i></button>
                </div>
            </form>
        </div>
    </div>

    {{-- Custom Scripts --}}
    @push('scripts')
        <script>
            function formatNumber(number) {
                if (number === null || number === undefined || number === '') return '';
                const num = parseFloat(String(number).replace(/[^0-9.]/g, ''));
                if (isNaN(num)) return '';
                return new Intl.NumberFormat('id-ID').format(num);
            }

            document.addEventListener('livewire:load', function() {
                Livewire.hook('element.removed', (el) => {
                    const selectElement = $(el).find('.select2-account, .select2-division');
                    if (selectElement.length > 0 && selectElement.data('select2')) {
                        selectElement.select2('destroy');
                    }
                });
            });
        </script>
    @endpush
</div>
