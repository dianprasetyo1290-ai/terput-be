<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Approval Jurnal" subjudul="Jurnal menunggu approval"
            :breadcrumb="['Transaction', 'Approval Jurnal']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Jurnal Menunggu Approval</h5>
        </div>
        <div class="card-body">
            <p class="text-muted">
                Penyetuju harus berbeda dengan pembuat/pengaju jurnal. Jurnal yang disetujui berstatus
                <strong>Posted</strong> dan masuk buku besar; jurnal yang ditolak kembali ke <strong>Draft</strong>.
            </p>
            @if ($workflowError)
                <div class="alert alert-danger">{{ $workflowError }}</div>
            @endif

            <table class="table table-hover">
                <thead>
                    <tr>
                        <th>No. Jurnal</th>
                        <th>Kode</th>
                        <th>Tanggal</th>
                        <th>Pembuat</th>
                        <th>Diajukan</th>
                        <th class="text-right">Debit</th>
                        <th class="text-right">Kredit</th>
                        <th class="text-center">Aksi</th>
                    </tr>
                </thead>
                <tbody>
                    @forelse ($pending as $j)
                        @php $balanced = abs($j->debit - $j->credit) < 0.01 && $j->lines > 0; @endphp
                        <tr>
                            <td><a href="#" wire:click.prevent="toggle('{{ $j->trans_no }}')">{{ $j->trans_no }}</a></td>
                            <td>{{ $j->trans_code }}</td>
                            <td>{{ \Carbon\Carbon::parse($j->trans_date)->format('d M Y') }}</td>
                            <td>{{ $j->trans_user }}</td>
                            <td>{{ $j->status_by }} <span class="text-muted small">{{ $j->status_at }}</span></td>
                            <td class="text-right">{{ number_format($j->debit, 2, ',', '.') }}</td>
                            <td class="text-right">{{ number_format($j->credit, 2, ',', '.') }}
                                @unless ($balanced)<span class="badge badge-warning">Tidak seimbang</span>@endunless
                            </td>
                            <td class="text-center text-nowrap">
                                @can('editjournal_validation')
                                    @if (in_array($me, [(string) $j->trans_user, (string) $j->status_by], true))
                                        <span class="text-muted small">Anda pembuat/pengaju</span>
                                    @else
                                        <button class="btn btn-sm btn-success" wire:click="approveJournal('{{ $j->trans_no }}')"
                                            wire:confirm="Setujui dan posting jurnal {{ $j->trans_no }}?">Setujui</button>
                                        <button class="btn btn-sm btn-outline-secondary" wire:click="rejectJournal('{{ $j->trans_no }}')"
                                            wire:confirm="Kembalikan jurnal {{ $j->trans_no }} ke draft?">Tolak</button>
                                    @endif
                                @else
                                    <span class="text-muted small">Hanya lihat</span>
                                @endcan
                            </td>
                        </tr>
                        @if ($open === $j->trans_no)
                            <tr>
                                <td colspan="8" class="bg-light">
                                    <table class="table table-sm mb-0">
                                        <thead><tr><th>#</th><th>Akun</th><th>Divisi</th><th>Keterangan</th><th class="text-right">Debit</th><th class="text-right">Kredit</th></tr></thead>
                                        <tbody>
                                            @foreach ($lines as $l)
                                                <tr>
                                                    <td>{{ $l->trans_count }}</td>
                                                    <td>{{ $l->acc_code }} {{ $l->acc_name }}</td>
                                                    <td>{{ $l->div_code }}</td>
                                                    <td>{{ $l->description }}</td>
                                                    <td class="text-right">{{ number_format($l->debet, 2, ',', '.') }}</td>
                                                    <td class="text-right">{{ number_format($l->credit, 2, ',', '.') }}</td>
                                                </tr>
                                            @endforeach
                                        </tbody>
                                    </table>
                                </td>
                            </tr>
                        @endif
                    @empty
                        <tr><td colspan="8" class="text-center text-muted py-4">Tidak ada jurnal yang menunggu approval.</td></tr>
                    @endforelse
                </tbody>
            </table>
        </div>
    </div>
</div>
