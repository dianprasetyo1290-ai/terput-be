<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Report" subjudul="General Ledger" :breadcrumb="['Report', 'General Ledger']" />
    </x-slot>

    <style>
        .gl-table {
            width: 100%;
            border-collapse: separate;
            border-spacing: 0;
            font-size: 13px;
        }
        .gl-table th {
            background-color: #f1f5f9;
            color: #475569;
            font-weight: 600;
            text-transform: uppercase;
            font-size: 11px;
            letter-spacing: 0.05em;
            padding: 10px 14px;
            border-bottom: 2px solid #e2e8f0;
            text-align: left;
            white-space: nowrap;
        }
        .gl-table td {
            padding: 8px 14px;
            border-bottom: 1px solid #f1f5f9;
            color: #374151;
            vertical-align: middle;
        }
        .gl-table tbody tr.row-data:hover {
            background-color: #f0f9ff;
        }
        .gl-table tbody tr.row-data:nth-child(even) {
            background-color: #fafbfc;
        }
        .gl-table .num {
            text-align: right;
            font-variant-numeric: tabular-nums;
        }
        .gl-table .row-bb td {
            background-color: #fffbeb;
            font-weight: 600;
            color: #92400e;
            border-left: 3px solid #f59e0b;
        }
        .gl-table .row-bb td:first-child {
            border-left: 3px solid #f59e0b;
        }
        .gl-table .row-total td {
            background-color: #f0fdf4;
            font-weight: 700;
            color: #166534;
            border-top: 2px solid #86efac;
        }
        .gl-table .row-ending td {
            background-color: #eff6ff;
            font-weight: 700;
            color: #1e40af;
            border-top: 2px solid #93c5fd;
        }

        /* Summary table */
        .summary-table th {
            background: linear-gradient(135deg, #1e40af 0%, #2563eb 100%);
            color: #ffffff;
            padding: 12px 14px;
            font-size: 11px;
            border-bottom: none;
        }
        .summary-table tbody tr:hover {
            background-color: #f0f9ff;
        }
        .summary-table tbody tr:nth-child(even) {
            background-color: #fafbfc;
        }
        .summary-table .row-grand td {
            background-color: #f0fdf4;
            font-weight: 700;
            color: #166534;
            border-top: 2px solid #86efac;
        }
    </style>

    {{-- Filter Card --}}
    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title"><i class="mr-2 icon-book"></i>General Ledger Report</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>
        <div class="card-body">
            {{-- Info Card --}}
            <div class="p-4 mb-5 border border-blue-100 bg-blue-50 rounded-xl">
                <div class="flex items-center mb-2">
                    <i class="mr-2 icon-info22" style="color: #2563eb;"></i>
                    <span class="text-sm font-semibold tracking-wide uppercase" style="color: #2563eb;">Petunjuk</span>
                </div>
                <p class="text-sm" style="color: #64748b;">
                    Pilih <strong>Tahun</strong>, <strong>Account Code</strong> (dari & sampai), dan <strong>Jenis Report</strong>, lalu klik <strong>Lihat Laporan</strong>.
                    Centang <em>"Report For This Period"</em> untuk melihat data transaksi periode berjalan (belum EOM).
                    Jika tidak dicentang, pilih <strong>Periode</strong> untuk melihat data histori yang sudah ditutup.
                </p>
            </div>

            @if ($errors->any())
                <div class="p-4 mb-4 rounded-lg" style="background-color: #fef2f2; border-left: 4px solid #ef4444;">
                    <div class="flex items-start">
                        <i class="mt-0.5 mr-3 icon-warning2" style="color: #dc2626;"></i>
                        <div>
                            <h6 class="mb-1 text-sm font-semibold" style="color: #991b1b;">Validasi Gagal</h6>
                            <ul class="text-sm" style="color: #b91c1c; list-style: disc; padding-left: 1.25rem;">
                                @foreach ($errors->all() as $error)
                                    <li>{{ $error }}</li>
                                @endforeach
                            </ul>
                        </div>
                    </div>
                </div>
            @endif

            <form action="#" wire:submit="sendReport">
                {{-- Tahun & This Period --}}
                <div class="form-group row">
                    <label class="col-form-label col-lg-2">{{ __('Tahun') }}</label>
                    <div class="col-lg-4">
                        <div wire:ignore>
                            <select class="form-control select-search2 year @error('year') is-invalid @enderror"
                                data-fouc wire:model.live="year">
                                <option value="">{{ __('Pilih Tahun') }}</option>
                                @foreach ($tahun as $p)
                                    <option value="{{ $p['tahun'] }}">{{ $p['tahun'] }}</option>
                                @endforeach
                            </select>
                        </div>
                        @error('year')
                            <span class="form-text text-danger">{{ $message }}</span>
                        @enderror
                    </div>
                    <div class="col-lg-4">
                        <div class="flex items-center h-100 px-4 py-2 rounded-lg" style="background-color: {{ $this_period ? '#eff6ff' : '#f8fafc' }}; border: 1px solid {{ $this_period ? '#bfdbfe' : '#e2e8f0' }};">
                            <label class="form-check-label d-flex align-items-center mb-0" style="cursor: pointer;">
                                <input type="checkbox" class="form-check-input mr-2" wire:model.live='this_period'>
                                <span class="text-sm font-medium" style="color: {{ $this_period ? '#1e40af' : '#64748b' }};">Report For This Period</span>
                            </label>
                        </div>
                    </div>
                </div>

                {{-- Periode --}}
                <div class="form-group row">
                    <label class="col-form-label col-lg-2">{{ __('Periode') }}</label>
                    <div class="col-lg-4">
                        <div wire:ignore>
                            <select id="periodeFrom"
                                class="form-control select-search2 periodeFrom @error('periodeFrom') is-invalid @enderror">
                                <option value="">{{ __('Period From') }}</option>
                                @foreach ($period as $p)
                                    <option value="{{ $p['p'] }}">
                                        {{ $p['p'] }} [{{ getMonthName($p['no']) }}]
                                    </option>
                                @endforeach
                            </select>
                        </div>
                        @error('periodeFrom')
                            <span class="form-text text-danger">{{ $message }}</span>
                        @enderror
                    </div>
                    <div class="col-lg-4">
                        <div wire:ignore>
                            <select
                                class="form-control select-search2 periodeTo @error('periodeTo') is-invalid @enderror">
                                <option value="">{{ __('Period To') }}</option>
                                @foreach ($period as $p)
                                    <option value="{{ $p['p'] }}">
                                        {{ $p['p'] }} [{{ getMonthName($p['no']) }}]
                                    </option>
                                @endforeach
                            </select>
                        </div>
                        @error('periodeTo')
                            <span class="form-text text-danger">{{ $message }}</span>
                        @enderror
                    </div>
                </div>

                {{-- Account Code --}}
                <div class="form-group row">
                    <label class="col-form-label col-lg-2">{{ __('Account Code') }}</label>
                    <div class="col-lg-4">
                        <div wire:ignore>
                            <select
                                class="form-control select-search2 acc_code @error('acc_code') is-invalid @enderror"
                                data-fouc wire:model.live="acc_code">
                                <option value="">{{ __('Account From') }}</option>
                                @foreach ($acc as $ac)
                                    <option value="{{ $ac->acc_code }}">
                                        {{ $ac->acc_code }} [{{ $ac->acc_name }}]
                                    </option>
                                @endforeach
                            </select>
                        </div>
                        @error('acc_code')
                            <span class="form-text text-danger">{{ $message }}</span>
                        @enderror
                    </div>
                    <div class="col-lg-4">
                        <div wire:ignore>
                            <select
                                class="form-control select-search2 acc_code2 @error('acc_code2') is-invalid @enderror"
                                data-fouc wire:model.live="acc_code2">
                                <option value="">{{ __('Account To') }}</option>
                                @foreach ($acc as $ac)
                                    <option value="{{ $ac->acc_code }}">
                                        {{ $ac->acc_code }} [{{ $ac->acc_name }}]
                                    </option>
                                @endforeach
                            </select>
                        </div>
                        @error('acc_code2')
                            <span class="form-text text-danger">{{ $message }}</span>
                        @enderror
                    </div>
                </div>

                {{-- Division --}}
                <div class="form-group row">
                    <label class="col-form-label col-lg-2">{{ __('Division') }}</label>
                    <div class="col-lg-4">
                        <div wire:ignore>
                            <select
                                class="form-control select-search2 codedivfrom @error('codedivfrom') is-invalid @enderror">
                                <option value="">{{ __('Division From') }}</option>
                                @foreach ($listDivision as $a)
                                    <option value="{{ $a['div_code'] }}">
                                        {{ $a['div_code'] }} [{{ $a['div_name'] }}]
                                    </option>
                                @endforeach
                            </select>
                        </div>
                        @error('codedivfrom')
                            <span class="form-text text-danger">{{ $message }}</span>
                        @enderror
                    </div>
                    <div class="col-lg-4">
                        <div wire:ignore>
                            <select
                                class="form-control select-search2 codedivto @error('codedivto') is-invalid @enderror">
                                <option value="">{{ __('Division To') }}</option>
                                @foreach ($listDivision as $a)
                                    <option value="{{ $a['div_code'] }}">
                                        {{ $a['div_code'] }} [{{ $a['div_name'] }}]
                                    </option>
                                @endforeach
                            </select>
                        </div>
                        @error('codedivto')
                            <span class="form-text text-danger">{{ $message }}</span>
                        @enderror
                    </div>
                </div>

                {{-- Report Type --}}
                <div class="form-group row">
                    <label class="col-form-label col-lg-2">{{ __('Report') }}</label>
                    <div class="col-lg-8">
                        <div wire:ignore>
                            <select class="form-control select-search2 report @error('report') is-invalid @enderror">
                                <option value="">{{ __('Pilih Report') }}</option>
                                <option value="generalledger">Detail General Ledger</option>
                                <option value="summarygeneralledger">Summary General Ledger</option>
                            </select>
                        </div>
                        @error('report')
                            <span class="form-text text-danger">{{ $message }}</span>
                        @enderror
                    </div>
                </div>

                {{-- Action Buttons --}}
                <div class="d-flex justify-content-between align-items-center my-3 pt-3" style="border-top: 1px solid #e2e8f0;">
                    <button type="button" class="btn btn-secondary rounded-lg" wire:click="batal">
                        <i class="mr-1 icon-cross2"></i> Cancel
                    </button>
                    <div class="d-flex" style="gap: 0.5rem;">
                        <button type="button" class="btn rounded-lg" wire:click="viewReport" wire:loading.attr="disabled" wire:target="viewReport"
                            style="background: linear-gradient(135deg, #0369a1 0%, #0ea5e9 100%); color: white; border: none;">
                            <span wire:loading.remove wire:target="viewReport">
                                <i class="mr-1 icon-eye"></i> Lihat Laporan
                            </span>
                            <span wire:loading wire:target="viewReport">
                                <i class="mr-1 icon-spinner2 spinner"></i> Memuat...
                            </span>
                        </button>
                        <button type="submit" class="btn btn-primary rounded-lg" wire:loading.attr="disabled" wire:target="sendReport">
                            <span wire:loading.remove wire:target="sendReport">
                                <i class="mr-1 icon-file-excel"></i> Export Excel
                            </span>
                            <span wire:loading wire:target="sendReport">
                                <i class="mr-1 icon-spinner2 spinner"></i> Exporting...
                            </span>
                        </button>
                    </div>
                </div>
            </form>
        </div>
    </div>

    {{-- Loading Overlay --}}
    <div wire:loading wire:target="viewReport">
        <div class="position-fixed w-100 h-100 d-flex align-items-center justify-content-center"
            style="top:0; left:0; background:rgba(255,255,255,0.7); z-index:2000;">
            <div class="text-center">
                <div class="mb-3">
                    <div class="spinner-border" role="status" style="width: 3rem; height: 3rem; color: #2563eb;">
                        <span class="sr-only">Loading...</span>
                    </div>
                </div>
                <div class="text-sm font-medium" style="color: #475569;">Memuat laporan...</div>
            </div>
        </div>
    </div>

    {{-- Report Output --}}
    @if ($showReport)
        <div class="card mt-3">
            <div class="card-header header-elements-inline" style="background: linear-gradient(135deg, #f0f9ff 0%, #e0f2fe 100%); border-bottom: 2px solid #bae6fd;">
                <h5 class="card-title" style="color: #0c4a6e;">
                    <i class="mr-2 icon-file-text2" style="color: #0369a1;"></i>
                    {{ $reportMode === 'detail' ? 'Detail General Ledger' : 'Summary General Ledger' }}
                </h5>
            </div>
            <div class="card-body">
                {{-- Filter Info Bar --}}
                <div class="d-flex flex-wrap mb-4 p-3 rounded-lg" style="background-color: #f8fafc; border: 1px solid #e2e8f0; gap: 1.5rem;">
                    <div>
                        <span class="d-block text-uppercase" style="font-size: 10px; font-weight: 600; color: #94a3b8; letter-spacing: 0.05em;">Tahun</span>
                        <span class="d-block font-weight-bold" style="font-size: 14px; color: #1e293b;">{{ $year }}</span>
                    </div>
                    <div>
                        <span class="d-block text-uppercase" style="font-size: 10px; font-weight: 600; color: #94a3b8; letter-spacing: 0.05em;">Periode</span>
                        <span class="d-block font-weight-bold" style="font-size: 14px; color: #1e293b;">
                            @if ($this_period)
                                <span class="px-2 py-1 rounded" style="background-color: #dbeafe; color: #1e40af; font-size: 12px;">Current Period</span>
                            @else
                                {{ $periodeFrom ?? '-' }} s/d {{ $periodeTo ?? '-' }}
                            @endif
                        </span>
                    </div>
                    <div>
                        <span class="d-block text-uppercase" style="font-size: 10px; font-weight: 600; color: #94a3b8; letter-spacing: 0.05em;">Account</span>
                        <span class="d-block font-weight-bold" style="font-size: 14px; color: #1e293b;">{{ $acc_code }} — {{ $acc_code2 ?: $acc_code }}</span>
                    </div>
                    @if ($codedivfrom)
                    <div>
                        <span class="d-block text-uppercase" style="font-size: 10px; font-weight: 600; color: #94a3b8; letter-spacing: 0.05em;">Division</span>
                        <span class="d-block font-weight-bold" style="font-size: 14px; color: #1e293b;">{{ $codedivfrom }} {{ $codedivto ? '— '.$codedivto : '' }}</span>
                    </div>
                    @endif
                </div>

                {{-- Detail General Ledger --}}
                @if ($reportMode === 'detail')
                    @forelse ($reportData as $account)
                        <div class="mb-5 overflow-hidden rounded-xl" style="border: 1px solid #e2e8f0; box-shadow: 0 1px 3px rgba(0,0,0,0.06);">
                            {{-- Account Header --}}
                            <div class="d-flex align-items-center px-4 py-3" style="background: linear-gradient(135deg, #eff6ff 0%, #dbeafe 100%); border-bottom: 2px solid #93c5fd;">
                                <span class="px-3 py-1 mr-3 rounded-lg" style="background-color: #1e40af; color: #ffffff; font-size: 12px; font-weight: 700; letter-spacing: 0.03em;">
                                    {{ $account['acc_code'] }}
                                </span>
                                <span style="font-size: 14px; font-weight: 600; color: #1e3a5f;">{{ $account['acc_name'] }}</span>
                                <span class="ml-auto px-2 py-1 rounded" style="background-color: #f0f9ff; color: #64748b; font-size: 11px;">
                                    {{ count($account['details']) }} transaksi
                                </span>
                            </div>

                            {{-- Table --}}
                            <div style="overflow-x: auto;">
                                <table class="gl-table">
                                    <thead>
                                        <tr>
                                            @if (! $this_period)
                                                <th style="width: 130px;">Period</th>
                                            @else
                                                <th style="width: 130px;">Trans Date</th>
                                            @endif
                                            <th style="width: 70px;">Type</th>
                                            <th style="width: 140px;">Voucher No</th>
                                            <th style="width: 70px;">Div</th>
                                            <th>Description</th>
                                            <th class="num" style="width: 140px;">Debit</th>
                                            <th class="num" style="width: 140px;">Credit</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {{-- Beginning Balance --}}
                                        <tr class="row-bb">
                                            <td colspan="5" style="text-align: right;">Beginning Balance</td>
                                            <td class="num">{{ cek_min($account['bb_debet']) }}</td>
                                            <td class="num">{{ cek_min($account['bb_credit']) }}</td>
                                        </tr>

                                        {{-- Transaction Details --}}
                                        @foreach ($account['details'] as $detail)
                                            <tr class="row-data">
                                                @if (! $this_period)
                                                    <td>{{ $detail['period'] ?? '' }} - {{ $detail['trans_date'] }}</td>
                                                @else
                                                    <td>{{ $detail['trans_date'] }}</td>
                                                @endif
                                                <td><span class="px-2 py-1 rounded" style="background-color: #f1f5f9; font-size: 11px; font-weight: 500;">{{ $detail['trans_code'] }}</span></td>
                                                <td style="font-size: 12px;">{{ $detail['trans_no'] }}</td>
                                                <td>{{ $detail['div_code'] }}</td>
                                                <td>{{ $detail['description'] }}</td>
                                                <td class="num">{{ cek_min($detail['debet']) }}</td>
                                                <td class="num">{{ cek_min($detail['credit']) }}</td>
                                            </tr>
                                        @endforeach

                                        {{-- Total --}}
                                        <tr class="row-total">
                                            <td colspan="5" style="text-align: right;">Total Mutasi</td>
                                            <td class="num">{{ cek_min($account['total_debet']) }}</td>
                                            <td class="num">{{ cek_min($account['total_credit']) }}</td>
                                        </tr>

                                        {{-- Ending Balance --}}
                                        <tr class="row-ending">
                                            <td colspan="5" style="text-align: right;">Ending Balance</td>
                                            <td class="num">{{ cek_min($account['ending_debet']) }}</td>
                                            <td class="num">{{ cek_min($account['ending_credit']) }}</td>
                                        </tr>
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    @empty
                        <div class="p-5 text-center rounded-xl" style="background-color: #f8fafc; border: 1px solid #e2e8f0;">
                            <i class="icon-file-empty2 mb-3" style="font-size: 2.5rem; color: #94a3b8;"></i>
                            <h5 class="mb-1" style="color: #64748b;">Tidak ada data</h5>
                            <p class="text-sm" style="color: #94a3b8;">Tidak ditemukan transaksi untuk filter yang dipilih.</p>
                        </div>
                    @endforelse

                {{-- Summary General Ledger --}}
                @elseif ($reportMode === 'summary')
                    <div class="overflow-hidden rounded-xl" style="border: 1px solid #e2e8f0; box-shadow: 0 1px 3px rgba(0,0,0,0.06);">
                        <div style="overflow-x: auto;">
                            <table class="gl-table summary-table">
                                <thead>
                                    <tr>
                                        <th style="width: 130px;">Account Code</th>
                                        <th>Account Name</th>
                                        <th class="num" style="width: 160px;">Beginning Balance</th>
                                        <th class="num" style="width: 140px;">Debit</th>
                                        <th class="num" style="width: 140px;">Credit</th>
                                        <th class="num" style="width: 160px;">Ending Balance</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    @php
                                        $grandBeg = 0;
                                        $grandDebet = 0;
                                        $grandCredit = 0;
                                        $grandEnd = 0;
                                    @endphp

                                    @forelse ($reportData as $row)
                                        @php
                                            $grandBeg += $row['beg_balance'];
                                            $grandDebet += $row['debet'];
                                            $grandCredit += $row['credit'];
                                            $grandEnd += $row['end_balance'];
                                        @endphp
                                        <tr>
                                            <td><span class="px-2 py-1 rounded" style="background-color: #f1f5f9; font-size: 12px; font-weight: 600;">{{ $row['acc_code'] }}</span></td>
                                            <td>{{ $row['acc_name'] }}</td>
                                            <td class="num">{{ cek_min($row['beg_balance']) }}</td>
                                            <td class="num">{{ cek_min($row['debet']) }}</td>
                                            <td class="num">{{ cek_min($row['credit']) }}</td>
                                            <td class="num">{{ cek_min($row['end_balance']) }}</td>
                                        </tr>
                                    @empty
                                        <tr>
                                            <td colspan="6" class="text-center" style="padding: 2rem;">
                                                <i class="icon-file-empty2" style="font-size: 1.5rem; color: #94a3b8;"></i>
                                                <div class="mt-2" style="color: #64748b;">Tidak ada data untuk filter yang dipilih.</div>
                                            </td>
                                        </tr>
                                    @endforelse

                                    @if (count($reportData) > 0)
                                        <tr class="row-grand">
                                            <td colspan="2" style="text-align: right; font-weight: 700;">Grand Total</td>
                                            <td class="num">{{ cek_min($grandBeg) }}</td>
                                            <td class="num">{{ cek_min($grandDebet) }}</td>
                                            <td class="num">{{ cek_min($grandCredit) }}</td>
                                            <td class="num">{{ cek_min($grandEnd) }}</td>
                                        </tr>
                                    @endif
                                </tbody>
                            </table>
                        </div>
                    </div>
                @endif
            </div>
        </div>
    @else
        {{-- Empty State --}}
        @if ($year || $acc_code || $report)
            <div class="p-5 mt-3 text-center border border-gray-200 rounded-xl" style="background-color: #f8fafc;">
                <i class="icon-file-empty2 mb-3" style="font-size: 2.5rem; color: #94a3b8;"></i>
                <h5 class="mb-2" style="color: #64748b;">Lengkapi Filter dan Klik "Lihat Laporan"</h5>
                <div class="text-sm" style="color: #94a3b8;">
                    @php
                        $missing = [];
                        if (!$year) $missing[] = 'Tahun';
                        if (!$acc_code) $missing[] = 'Account From';
                        if (!$acc_code2) $missing[] = 'Account To';
                        if (!$report) $missing[] = 'Jenis Report';
                        if (!$this_period && (!$periodeFrom || !$periodeTo)) $missing[] = 'Periode (From & To)';
                    @endphp
                    @if (count($missing) > 0)
                        Belum diisi: <strong>{{ implode(', ', $missing) }}</strong>
                    @else
                        Klik tombol <strong>"Lihat Laporan"</strong> untuk menampilkan data.
                    @endif
                </div>
            </div>
        @endif
    @endif
</div>
