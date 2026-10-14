@php
    $fmt = fn ($v) => abs((float) $v) < 0.005 ? '-' : ((float) $v < 0 ? '('.number_format(-$v, 2, ',', '.').')' : number_format((float) $v, 2, ',', '.'));
    $withDivision = false;
    $unitLabel = 'Konsolidasi '.$c['units']->count().' unit';
@endphp
<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Report" subjudul="Konsolidasi" :breadcrumb="['Report', 'Konsolidasi']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Konsolidasi Antar Unit</h5>
        </div>
        <div class="card-body">
            @include('livewire.pages.report.partials.period-picker')
            @include('livewire.pages.report.partials.report-meta')

            @if ($c['interunit_accounts'] === 0)
                <div class="alert alert-info py-2">Belum ada akun bertanda <em>antar unit</em> di Chart of Account, sehingga tidak ada eliminasi.</div>
            @elseif (abs($c['interunit_residual']) >= 0.01)
                <div class="alert alert-danger py-2">
                    <strong>Akun antar unit tidak saling menghapus.</strong> Sisa {{ $fmt($c['interunit_residual']) }}
                    (saldo debit-positif). Periksa jurnal antar unit sebelum laporan konsolidasi dipakai.
                </div>
            @else
                <div class="alert alert-success py-2">Akun antar unit saling menghapus (eliminasi seimbang).</div>
            @endif

            <div class="table-responsive">
                <table class="table table-sm table-bordered">
                    <thead class="thead-light">
                        <tr>
                            <th>Akun</th>
                            @foreach ($c['units'] as $u)<th class="text-right">{{ $u->div_code }}</th>@endforeach
                            @if ($c['has_other'])<th class="text-right">Tanpa unit</th>@endif
                            <th class="text-right">Jumlah</th>
                            <th class="text-right">Eliminasi</th>
                            <th class="text-right">Konsolidasi</th>
                        </tr>
                    </thead>
                    <tbody>
                        @forelse ($c['rows'] as $r)
                            <tr class="{{ $r->is_interunit ? 'table-warning' : '' }}">
                                <td>{{ $r->acc_code }} {{ $r->acc_name }}</td>
                                @foreach ($c['units'] as $u)<td class="text-right">{{ $fmt($r->units[$u->div_code] ?? 0) }}</td>@endforeach
                                @if ($c['has_other'])<td class="text-right">{{ $fmt($r->other) }}</td>@endif
                                <td class="text-right">{{ $fmt($r->total) }}</td>
                                <td class="text-right">{{ $fmt($r->elimination) }}</td>
                                <td class="text-right font-weight-bold">{{ $fmt($r->consolidated) }}</td>
                            </tr>
                        @empty
                            <tr><td colspan="99" class="text-center text-muted py-4">Belum ada saldo pada periode ini.</td></tr>
                        @endforelse
                    </tbody>
                </table>
            </div>
            <p class="text-muted small mb-1">Saldo akhir dalam format debit-positif (kredit dalam kurung). Baris kuning = akun antar unit.</p>

            <div class="row mt-3">
                <div class="col-md-3"><div class="border rounded p-2"><div class="small text-muted">Total Aset</div><strong>{{ $fmt($c['summary']['assets']) }}</strong></div></div>
                <div class="col-md-3"><div class="border rounded p-2"><div class="small text-muted">Total Liabilitas</div><strong>{{ $fmt($c['summary']['liabilities']) }}</strong></div></div>
                <div class="col-md-3"><div class="border rounded p-2"><div class="small text-muted">Pendapatan s.d. periode</div><strong>{{ $fmt($c['summary']['revenue']) }}</strong></div></div>
                <div class="col-md-3"><div class="border rounded p-2"><div class="small text-muted">Beban s.d. periode</div><strong>{{ $fmt($c['summary']['expense']) }}</strong></div></div>
            </div>
        </div>
    </div>
</div>
