@php
    $fmt = fn ($v) => abs($v) < 0.005 ? '-' : number_format($v, 2, ',', '.');
    $unitLabel = $division ? ($divisions->firstWhere('div_code', $division)?->div_name ?? $division) : 'Semua unit';
@endphp
<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Report" subjudul="Neraca Saldo" :breadcrumb="['Report', 'Neraca Saldo']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Neraca Saldo (Trial Balance)</h5>
        </div>
        <div class="card-body">
            @include('livewire.pages.report.partials.period-picker')
            @include('livewire.pages.report.partials.report-meta')

            <div class="form-check mb-2">
                <input type="checkbox" class="form-check-input" id="tbzero" wire:model.live="showZero">
                <label class="form-check-label small" for="tbzero">Tampilkan akun bersaldo nol</label>
            </div>

            <div class="table-responsive">
                <table class="table table-sm table-bordered">
                    <thead class="thead-light">
                        <tr>
                            <th rowspan="2">Kode</th>
                            <th rowspan="2">Nama Akun</th>
                            <th colspan="2" class="text-center">Saldo Awal</th>
                            <th colspan="2" class="text-center">Mutasi Periode</th>
                            <th colspan="2" class="text-center">Saldo Akhir</th>
                        </tr>
                        <tr>
                            <th class="text-right">Debit</th><th class="text-right">Kredit</th>
                            <th class="text-right">Debit</th><th class="text-right">Kredit</th>
                            <th class="text-right">Debit</th><th class="text-right">Kredit</th>
                        </tr>
                    </thead>
                    <tbody>
                        @forelse ($rows as $r)
                            <tr>
                                <td>
                                    @can('viewgeneral_ledger')
                                        <a href="{{ url('reportgeneralledger') }}?acc={{ urlencode($r->acc_code) }}">{{ $r->acc_code }}</a>
                                    @else
                                        {{ $r->acc_code }}
                                    @endcan
                                </td>
                                <td>{{ $r->acc_name }} @unless ($r->is_active)<span class="badge badge-secondary">nonaktif</span>@endunless</td>
                                <td class="text-right">{{ $fmt($r->open_debit) }}</td>
                                <td class="text-right">{{ $fmt($r->open_credit) }}</td>
                                <td class="text-right">{{ $fmt($r->debit) }}</td>
                                <td class="text-right">{{ $fmt($r->credit) }}</td>
                                <td class="text-right">{{ $fmt($r->end_debit) }}</td>
                                <td class="text-right">{{ $fmt($r->end_credit) }}</td>
                            </tr>
                        @empty
                            <tr><td colspan="8" class="text-center text-muted py-4">Belum ada saldo atau jurnal posted pada periode ini.</td></tr>
                        @endforelse
                    </tbody>
                    <tfoot class="font-weight-bold">
                        <tr>
                            <td colspan="2">Total</td>
                            <td class="text-right">{{ $fmt($totals['open_debit']) }}</td>
                            <td class="text-right">{{ $fmt($totals['open_credit']) }}</td>
                            <td class="text-right">{{ $fmt($totals['debit']) }}</td>
                            <td class="text-right">{{ $fmt($totals['credit']) }}</td>
                            <td class="text-right">{{ $fmt($totals['end_debit']) }}</td>
                            <td class="text-right">{{ $fmt($totals['end_credit']) }}</td>
                        </tr>
                    </tfoot>
                </table>
            </div>

            @if ($totals['balanced'])
                <div class="alert alert-success py-2 mb-0">Seimbang: total debit = total kredit.</div>
            @else
                <div class="alert alert-danger py-2 mb-0">
                    <strong>Tidak seimbang.</strong>
                    Selisih saldo akhir {{ number_format($totals['difference'], 2, ',', '.') }};
                    selisih mutasi {{ number_format($totals['movement_difference'], 2, ',', '.') }}.
                    @if ($division)
                        Pada tampilan per unit, jurnal antar unit yang barisnya berbeda unit dapat menimbulkan selisih.
                    @else
                        Periksa saldo awal (Balance) dan jurnal pada periode ini.
                    @endif
                </div>
            @endif
        </div>
    </div>
</div>
