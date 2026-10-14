@php $fmt = fn ($v) => abs((float) $v) < 0.005 ? '-' : number_format((float) $v, 2, ',', '.'); @endphp
<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Jurnal" subjudul="Detail Jurnal" :breadcrumb="['Report', 'Buku Besar', 'Detail Jurnal']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Jurnal {{ $head->trans_no }}</h5>
            <a href="javascript:history.back()" class="btn btn-light btn-sm">Kembali</a>
        </div>
        <div class="card-body">
            <div class="row mb-3">
                <div class="col-md-6">
                    <table class="table table-sm table-borderless mb-0">
                        <tr><th class="w-25">Tanggal</th><td>{{ \Carbon\Carbon::parse($head->trans_date)->format('d M Y') }}</td></tr>
                        <tr><th>Tipe</th><td>{{ $head->trans_code }} {{ $type ? "- $type" : '' }}</td></tr>
                        <tr><th>Status</th><td>
                            <span class="badge badge-{{ ['posted' => 'success', 'reversed' => 'secondary', 'pending' => 'warning', 'draft' => 'light'][$head->status ?? 'posted'] ?? 'light' }}">{{ $head->status ?? 'posted' }}</span>
                            {{ $closed ? '(periode sudah ditutup)' : '(periode aktif)' }}
                        </td></tr>
                        <tr><th>Dibuat oleh</th><td>{{ $head->trans_user }}</td></tr>
                        @if (!empty($head->status_by))
                            <tr><th>Status oleh</th><td>{{ $head->status_by }} <span class="text-muted small">{{ $head->status_at }}</span></td></tr>
                        @endif
                    </table>
                </div>
                <div class="col-md-6">
                    <table class="table table-sm table-borderless mb-0">
                        <tr><th class="w-25">Sumber</th><td>{{ $head->source ?? 'manual' }}</td></tr>
                        <tr><th>Referensi</th><td>{{ $head->reference ?? '-' }}</td></tr>
                        @if (!empty($head->reversal_of))
                            <tr><th>Pembalik dari</th><td><a href="{{ url('journal/'.rawurlencode($head->reversal_of)) }}">{{ $head->reversal_of }}</a></td></tr>
                        @endif
                        @foreach ($reversedBy as $r)
                            <tr><th>Dibalik oleh</th><td><a href="{{ url('journal/'.rawurlencode($r->trans_no)) }}">{{ $r->trans_no }}</a></td></tr>
                        @endforeach
                        @if ($posting)
                            <tr><th>Posting otomatis</th><td>{{ $posting->kind }} &middot; {{ $posting->status }} &middot; {{ $posting->posting_date }}</td></tr>
                        @endif
                    </table>
                </div>
            </div>

            <table class="table table-sm table-bordered">
                <thead class="thead-light">
                    <tr><th>#</th><th>Akun</th><th>Unit</th><th>Keterangan</th><th class="text-right">Debit</th><th class="text-right">Kredit</th></tr>
                </thead>
                <tbody>
                    @foreach ($lines as $l)
                        <tr>
                            <td>{{ $l->trans_count }}</td>
                            <td>{{ $l->acc_code }} {{ $l->acc_name }}</td>
                            <td>{{ $l->div_code }} {{ $l->div_name }}</td>
                            <td>{{ $l->description }}</td>
                            <td class="text-right">{{ $fmt($l->debet) }}</td>
                            <td class="text-right">{{ $fmt($l->credit) }}</td>
                        </tr>
                    @endforeach
                </tbody>
                <tfoot class="font-weight-bold">
                    <tr><td colspan="4">Total</td>
                        <td class="text-right">{{ $fmt($lines->sum('debet')) }}</td>
                        <td class="text-right">{{ $fmt($lines->sum('credit')) }}</td></tr>
                </tfoot>
            </table>
        </div>
    </div>

    @if ($isAuto)
        <div class="card" wire:init="loadSources">
            <div class="card-header"><h5 class="card-title">Transaksi Sumber</h5></div>
            <div class="card-body">
                @if ($sourceIds)
                    <div class="mb-2 small">
                        <strong>ID yang dikirim sumber:</strong>
                        @foreach ($sourceIds as $t => $ids)
                            <div>{{ $t }}: {{ implode(', ', array_slice($ids, 0, 50)) }}{{ count($ids) > 50 ? ' … (+'.(count($ids) - 50).')' : '' }}</div>
                        @endforeach
                    </div>
                @endif
                @if ($sources === null)
                    <div class="text-muted" wire:loading.remove wire:target="loadSources">Memuat transaksi sumber…</div>
                    <div class="text-muted" wire:loading wire:target="loadSources">Memuat transaksi sumber dari billing…</div>
                @elseif ($sourcesError)
                    <div class="alert alert-secondary mb-0">{{ $sourcesError }}
                        @if (! $sourceIds) Jurnal ini tidak menyimpan ID sumber. @endif
                    </div>
                @elseif (count($sources) === 0)
                    <div class="text-muted">Billing tidak mengembalikan transaksi untuk referensi {{ $head->reference }}.</div>
                @else
                    <table class="table table-sm table-bordered mb-0">
                        <thead class="thead-light"><tr><th>Jenis</th><th>Nomor</th><th>Siswa</th><th>Unit</th><th>Tanggal</th><th>Status</th><th class="text-right">Nominal</th></tr></thead>
                        <tbody>
                            @foreach ($sources as $it)
                                <tr>
                                    <td>{{ $it['type'] ?? '-' }}</td>
                                    <td>{{ $it['number'] ?? ($it['id'] ?? '-') }}</td>
                                    <td>{{ $it['student_name'] ?? '-' }} @if (!empty($it['nis']))<span class="text-muted small">({{ $it['nis'] }})</span>@endif</td>
                                    <td>{{ $it['unit'] ?? '-' }}</td>
                                    <td>{{ $it['date'] ?? '-' }}</td>
                                    <td>{{ $it['status'] ?? '-' }}</td>
                                    <td class="text-right">{{ $fmt($it['amount_idr'] ?? 0) }}</td>
                                </tr>
                            @endforeach
                        </tbody>
                        <tfoot class="font-weight-bold"><tr><td colspan="6">Total sumber</td>
                            <td class="text-right">{{ $fmt(collect($sources)->sum(fn ($i) => (float) ($i['amount_idr'] ?? 0))) }}</td></tr></tfoot>
                    </table>
                @endif
            </div>
        </div>
    @endif
</div>
