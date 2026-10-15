<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="Buka Kembali Periode" :breadcrumb="['Transaction', 'Buka Kembali Periode']" />
    </x-slot>

    @if ($message)
        <div class="alert alert-success">{{ $message }}</div>
    @endif
    @if ($error)
        <div class="alert alert-danger">{{ $error }}</div>
    @endif

    <div class="card">
        <div class="card-header"><h5 class="card-title">Buka Kembali Periode yang Sudah Ditutup</h5></div>
        <div class="card-body">
            <p>Periode aktif: <strong>{{ $activeLabel }}</strong>.
                @if ($target)
                    Periode terakhir yang ditutup: <strong>{{ $targetLabel }}</strong>.
                @endif
            </p>
            <ul class="small text-muted pl-3">
                <li>Hanya periode yang <strong>terakhir</strong> ditutup, dan hanya selama periode aktif belum berisi jurnal.</li>
                <li>Permintaan wajib disertai alasan dan harus disetujui pengguna lain dengan hak persetujuan (Finance Manager).</li>
                <li>Setelah disetujui, jurnal periode tersebut kembali ke periode aktif; tutup bulan dijalankan ulang setelah koreksi.</li>
                <li>Semua langkah tercatat di audit trail.</li>
            </ul>

            @if ($pending)
                <div class="alert alert-warning">
                    <div><strong>Menunggu keputusan:</strong> buka periode {{ $pending->period }}/{{ $pending->year }}, diajukan oleh {{ $pending->requested_by }} pada {{ $pending->requested_at }}.</div>
                    <div class="mb-2">Alasan: {{ $pending->reason }}</div>
                    @can('editperiod_reopen')
                        @if ($pending->requested_by !== $me)
                            <div class="form-group">
                                <input type="text" class="form-control" placeholder="Catatan keputusan (wajib bila menolak)" wire:model="note">
                            </div>
                            <button class="btn btn-success" wire:click="approve({{ $pending->id }})"
                                wire:confirm="Setujui dan buka kembali periode {{ $pending->period }}/{{ $pending->year }}?">Setujui &amp; Buka Periode</button>
                            <button class="btn btn-danger" wire:click="reject({{ $pending->id }})">Tolak</button>
                        @else
                            <div class="small">Permintaan Anda sendiri harus diputuskan pengguna lain.</div>
                        @endif
                    @endcan
                    @can('createperiod_reopen')
                        @if ($pending->requested_by === $me)
                            <button class="btn btn-light" wire:click="cancelRequest({{ $pending->id }})">Batalkan Permintaan</button>
                        @endif
                    @endcan
                </div>
            @elseif ($blockers)
                <div class="alert alert-secondary mb-0">
                    @foreach ($blockers as $b)
                        <div>{{ $b }}</div>
                    @endforeach
                </div>
            @else
                @can('createperiod_reopen')
                    <div class="form-group">
                        <label>Alasan membuka kembali periode {{ $targetLabel }}</label>
                        <textarea class="form-control" rows="3" wire:model="reason" maxlength="2000"></textarea>
                    </div>
                    <button class="btn btn-primary" wire:click="requestReopen" wire:loading.attr="disabled">Ajukan Permintaan</button>
                @else
                    <div class="text-muted">Anda tidak memiliki hak mengajukan pembukaan periode.</div>
                @endcan
            @endif
        </div>
    </div>

    <div class="card">
        <div class="card-header"><h5 class="card-title">Riwayat Permintaan</h5></div>
        <div class="card-body">
            <table class="table table-sm">
                <thead class="thead-light"><tr><th>Periode</th><th>Alasan</th><th>Diajukan</th><th>Status</th><th>Diputuskan</th><th>Catatan</th><th class="text-right">Jurnal</th></tr></thead>
                <tbody>
                    @forelse ($history as $h)
                        <tr>
                            <td>{{ $h->period }}/{{ $h->year }}</td>
                            <td class="small">{{ $h->reason }}</td>
                            <td class="small">{{ $h->requested_by }}<br>{{ $h->requested_at }}</td>
                            <td><span class="badge badge-{{ ['approved' => 'success', 'rejected' => 'danger', 'pending' => 'warning'][$h->status] ?? 'secondary' }}">{{ $h->status }}</span></td>
                            <td class="small">{{ $h->decided_by }}<br>{{ $h->decided_at }}</td>
                            <td class="small">{{ $h->decision_note }}</td>
                            <td class="text-right">{{ $h->moved_journals ?? '-' }}</td>
                        </tr>
                    @empty
                        <tr><td colspan="7" class="text-center text-muted py-3">Belum ada permintaan.</td></tr>
                    @endforelse
                </tbody>
            </table>
        </div>
    </div>
</div>
