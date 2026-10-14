<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="Posting Otomatis" :breadcrumb="['Transaction', 'Posting Otomatis']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Posting Otomatis (Billing &amp; Pembayaran)</h5>
        </div>
        <div class="card-body">
            <p class="text-muted small">
                Posting gagal diperbaiki di sumbernya (pemetaan akun, periode, data tagihan/pembayaran) lalu dikirim ulang dari SEKOCI;
                kiriman ulang yang berhasil mengubah status menjadi <em>posted</em>.
            </p>
            <div class="row mb-3">
                <div class="col-md-3">
                    <select class="form-control" wire:model.live="status">
                        <option value="failed">Gagal ({{ $counts['failed'] ?? 0 }})</option>
                        <option value="posted">Berhasil ({{ $counts['posted'] ?? 0 }})</option>
                        <option value="all">Semua</option>
                    </select>
                </div>
                <div class="col-md-4">
                    <input type="text" class="form-control" placeholder="Cari referensi / no. jurnal / pesan" wire:model.live.debounce.400ms="search">
                </div>
            </div>
            <table class="table table-sm table-hover">
                <thead class="thead-light">
                    <tr><th>Tanggal</th><th>Jenis</th><th>Referensi</th><th>Status</th><th class="text-right">Nilai</th><th>Jurnal</th><th>Pesan</th></tr>
                </thead>
                <tbody>
                    @forelse ($rows as $r)
                        <tr>
                            <td class="text-nowrap">{{ $r->posting_date }}</td>
                            <td>{{ $r->kind }}</td>
                            <td class="small">{{ $r->reference }}</td>
                            <td><span class="badge badge-{{ $r->status === 'failed' ? 'danger' : 'success' }}">{{ $r->status }}</span></td>
                            <td class="text-right">{{ number_format((float) $r->amount, 2, ',', '.') }}</td>
                            <td>
                                @if ($r->trans_no)
                                    <a href="{{ url('journal/'.rawurlencode($r->trans_no)) }}">{{ $r->trans_no }}</a>
                                @else - @endif
                            </td>
                            <td class="small">{{ $r->message }}</td>
                        </tr>
                    @empty
                        <tr><td colspan="7" class="text-center text-muted py-4">Tidak ada data.</td></tr>
                    @endforelse
                </tbody>
            </table>
            {{ $rows->links() }}
        </div>
    </div>
</div>
