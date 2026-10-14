@php
    $fmt = fn ($v) => abs((float) $v) < 0.005 ? '-' : ((float) $v < 0 ? '('.number_format(-$v, 2, ',', '.').')' : number_format((float) $v, 2, ',', '.'));
    $unitLabel = $division ? ($divisions->firstWhere('div_code', $division)?->div_name ?? $division) : 'Semua unit';
    $label = $s['label'];
    $provisional = $s['provisional'];
    $generatedAt = $s['generated_at'];
    $act = $s['activity'];
    $pos = $s['position'];
    $cf = $s['cash_flow'];
    $na = $s['net_assets'];
    $tabs = ['activity' => 'Aktivitas', 'position' => 'Posisi Keuangan', 'cash_flow' => 'Arus Kas', 'net_assets' => 'Perubahan Aset Neto'];
@endphp
<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Report" subjudul="Laporan Keuangan" :breadcrumb="['Report', 'Laporan Keuangan']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Laporan Keuangan</h5>
        </div>
        <div class="card-body">
            @include('livewire.pages.report.partials.period-picker')
            @include('livewire.pages.report.partials.report-meta')

            <ul class="nav nav-tabs mb-3">
                @foreach ($tabs as $k => $name)
                    <li class="nav-item"><a href="#" class="nav-link {{ $tab === $k ? 'active' : '' }}" wire:click.prevent="setTab('{{ $k }}')">{{ $name }}</a></li>
                @endforeach
            </ul>

            @if ($tab === 'activity')
                <h6 class="font-weight-bold">Laporan Aktivitas (Surplus/Defisit)</h6>
                <table class="table table-sm">
                    <thead class="thead-light"><tr><th>Akun</th><th class="text-right">{{ $label }}</th><th class="text-right">{{ $s['prev_label'] }}</th><th class="text-right">s.d. {{ $label }}</th></tr></thead>
                    <tbody>
                        <tr class="table-active"><td colspan="4"><strong>Pendapatan</strong></td></tr>
                        @foreach ($act['revenue'] as $l)
                            <tr><td>{{ $l->acc_code }} {{ $l->acc_name }}</td><td class="text-right">{{ $fmt($l->period) }}</td><td class="text-right">{{ $fmt($l->prev) }}</td><td class="text-right">{{ $fmt($l->ytd) }}</td></tr>
                        @endforeach
                        <tr class="font-weight-bold"><td>Total Pendapatan</td><td class="text-right">{{ $fmt($act['total_revenue']['period']) }}</td><td class="text-right">{{ $fmt($act['total_revenue']['prev']) }}</td><td class="text-right">{{ $fmt($act['total_revenue']['ytd']) }}</td></tr>
                        <tr class="table-active"><td colspan="4"><strong>Beban</strong></td></tr>
                        @foreach ($act['expense'] as $l)
                            <tr><td>{{ $l->acc_code }} {{ $l->acc_name }}</td><td class="text-right">{{ $fmt($l->period) }}</td><td class="text-right">{{ $fmt($l->prev) }}</td><td class="text-right">{{ $fmt($l->ytd) }}</td></tr>
                        @endforeach
                        <tr class="font-weight-bold"><td>Total Beban</td><td class="text-right">{{ $fmt($act['total_expense']['period']) }}</td><td class="text-right">{{ $fmt($act['total_expense']['prev']) }}</td><td class="text-right">{{ $fmt($act['total_expense']['ytd']) }}</td></tr>
                    </tbody>
                    <tfoot class="font-weight-bold">
                        <tr><td>Surplus (Defisit)</td><td class="text-right">{{ $fmt($act['surplus']['period']) }}</td><td class="text-right">{{ $fmt($act['surplus']['prev']) }}</td><td class="text-right">{{ $fmt($act['surplus']['ytd']) }}</td></tr>
                    </tfoot>
                </table>
            @elseif ($tab === 'position')
                <h6 class="font-weight-bold">Laporan Posisi Keuangan per akhir {{ $label }}</h6>
                <table class="table table-sm">
                    <thead class="thead-light"><tr><th>Akun</th><th class="text-right">{{ $label }}</th><th class="text-right">{{ $s['prev_label'] }}</th></tr></thead>
                    <tbody>
                        <tr class="table-active"><td colspan="3"><strong>Aset</strong></td></tr>
                        @foreach ($pos['assets'] as $l)
                            <tr><td>{{ $l->acc_code }} {{ $l->acc_name }}</td><td class="text-right">{{ $fmt($l->amount) }}</td><td class="text-right">{{ $fmt($l->prev) }}</td></tr>
                        @endforeach
                        <tr class="font-weight-bold"><td>Total Aset</td><td class="text-right">{{ $fmt($pos['total_assets']['amount']) }}</td><td class="text-right">{{ $fmt($pos['total_assets']['prev']) }}</td></tr>
                        <tr class="table-active"><td colspan="3"><strong>Liabilitas</strong></td></tr>
                        @foreach ($pos['liabilities'] as $l)
                            <tr><td>{{ $l->acc_code }} {{ $l->acc_name }}</td><td class="text-right">{{ $fmt($l->amount) }}</td><td class="text-right">{{ $fmt($l->prev) }}</td></tr>
                        @endforeach
                        <tr class="font-weight-bold"><td>Total Liabilitas</td><td class="text-right">{{ $fmt($pos['total_liabilities']['amount']) }}</td><td class="text-right">{{ $fmt($pos['total_liabilities']['prev']) }}</td></tr>
                        <tr class="table-active"><td colspan="3"><strong>Aset Neto</strong></td></tr>
                        @foreach ($pos['equity'] as $l)
                            <tr><td>{{ $l->acc_code }} {{ $l->acc_name }}</td><td class="text-right">{{ $fmt($l->amount) }}</td><td class="text-right">{{ $fmt($l->prev) }}</td></tr>
                        @endforeach
                        <tr><td>Surplus (defisit) tahun berjalan</td><td class="text-right">{{ $fmt($pos['surplus']['amount']) }}</td><td class="text-right">{{ $fmt($pos['surplus']['prev']) }}</td></tr>
                        <tr class="font-weight-bold"><td>Total Aset Neto</td><td class="text-right">{{ $fmt($pos['total_net_assets']['amount']) }}</td><td class="text-right">{{ $fmt($pos['total_net_assets']['prev']) }}</td></tr>
                    </tbody>
                    <tfoot class="font-weight-bold">
                        <tr><td>Total Liabilitas dan Aset Neto</td>
                            <td class="text-right">{{ $fmt($pos['total_liabilities']['amount'] + $pos['total_net_assets']['amount']) }}</td>
                            <td class="text-right">{{ $fmt($pos['total_liabilities']['prev'] + $pos['total_net_assets']['prev']) }}</td></tr>
                    </tfoot>
                </table>
                @if (abs($pos['difference']) >= 0.01)
                    <div class="alert alert-danger py-2">Aset tidak sama dengan liabilitas + aset neto (selisih {{ $fmt($pos['difference']) }}). Periksa neraca saldo.</div>
                @endif
            @elseif ($tab === 'cash_flow')
                <h6 class="font-weight-bold">Laporan Arus Kas (metode langsung) {{ $label }}</h6>
                @unless ($cf['has_cash_accounts'])
                    <div class="alert alert-warning py-2">Belum ada akun bertanda Kas/Bank di Chart of Account.</div>
                @endunless
                <table class="table table-sm">
                    <tbody>
                        <tr class="font-weight-bold"><td>Kas dan setara kas awal periode</td><td class="text-right">{{ $fmt($cf['opening']) }}</td></tr>
                        <tr class="table-active"><td colspan="2"><strong>Penerimaan kas</strong></td></tr>
                        @foreach ($cf['receipts'] as $r)
                            <tr><td class="pl-4">{{ $r->acc_code }} {{ $r->acc_name }}</td><td class="text-right">{{ $fmt($r->amount) }}</td></tr>
                        @endforeach
                        <tr class="font-weight-bold"><td>Total penerimaan</td><td class="text-right">{{ $fmt($cf['total_receipts']) }}</td></tr>
                        <tr class="table-active"><td colspan="2"><strong>Pengeluaran kas</strong></td></tr>
                        @foreach ($cf['payments'] as $r)
                            <tr><td class="pl-4">{{ $r->acc_code }} {{ $r->acc_name }}</td><td class="text-right">{{ $fmt($r->amount) }}</td></tr>
                        @endforeach
                        <tr class="font-weight-bold"><td>Total pengeluaran</td><td class="text-right">{{ $fmt($cf['total_payments']) }}</td></tr>
                        @if (abs($cf['unattributed']) >= 0.01)
                            <tr><td>Mutasi kas jurnal antar unit</td><td class="text-right">{{ $fmt($cf['unattributed']) }}</td></tr>
                        @endif
                        <tr class="font-weight-bold"><td>Kenaikan (penurunan) kas</td><td class="text-right">{{ $fmt($cf['net']) }}</td></tr>
                    </tbody>
                    <tfoot class="font-weight-bold">
                        <tr><td>Kas dan setara kas akhir periode</td><td class="text-right">{{ $fmt($cf['ending']) }}</td></tr>
                    </tfoot>
                </table>
                @if (abs($cf['difference']) >= 0.01)
                    <div class="alert alert-warning py-2">Saldo awal + arus kas berbeda {{ $fmt($cf['difference']) }} dari saldo akhir kas (ada mutasi saldo kas tanpa jurnal).</div>
                @endif
            @else
                <h6 class="font-weight-bold">Laporan Perubahan Aset Neto {{ $label }}</h6>
                <table class="table table-sm">
                    <tbody>
                        <tr><td>Aset neto awal periode</td><td class="text-right">{{ $fmt($na['opening']) }}</td></tr>
                        <tr><td>Surplus (defisit) periode berjalan</td><td class="text-right">{{ $fmt($na['surplus']) }}</td></tr>
                        <tr><td>Perubahan langsung pada aset neto</td><td class="text-right">{{ $fmt($na['direct']) }}</td></tr>
                    </tbody>
                    <tfoot class="font-weight-bold">
                        <tr><td>Aset neto akhir periode</td><td class="text-right">{{ $fmt($na['ending']) }}</td></tr>
                    </tfoot>
                </table>
                <p class="text-muted small">Pemisahan aset neto dengan/tanpa pembatasan mengikuti akun ekuitas di Chart of Account.</p>
            @endif
        </div>
    </div>
</div>
