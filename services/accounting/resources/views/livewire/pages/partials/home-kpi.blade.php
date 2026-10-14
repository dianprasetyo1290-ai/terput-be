{{-- QA BUG-140: ledger KPIs (posted only), journal workflow status, failed auto postings. --}}
@php $rp = fn ($v) => 'Rp '.number_format((float) $v, 0, ',', '.'); @endphp
<div class="card" style="border: none; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.08);">
    <div class="card-body">
        <div class="d-flex justify-content-between align-items-center mb-2">
            <h6 class="mb-0 font-weight-bold">Ringkasan Keuangan &mdash; {{ $kpiLabel }}</h6>
            <span class="text-muted small">Hanya jurnal posted; periode berjalan bersifat sementara.</span>
        </div>
        <div class="row">
            @foreach ([
                ['Pendapatan bulan ini', $kpi['revenue']],
                ['Beban bulan ini', $kpi['expense']],
                ['Surplus (defisit) bulan ini', $kpi['surplus']],
                ['Kas & Bank', $kpi['cash']],
                ['Piutang (akun kontrol)', $kpi['receivables']],
                ['Aset Neto', $kpi['net_assets']],
            ] as [$name, $value])
                <div class="col-6 col-md-4 col-xl-2 mb-2">
                    <div class="border rounded p-2 h-100">
                        <div class="small text-muted">{{ $name }}</div>
                        <div class="font-weight-bold {{ $value < 0 ? 'text-danger' : '' }}">{{ $rp($value) }}</div>
                    </div>
                </div>
            @endforeach
        </div>
        <div class="row mt-2">
            <div class="col-md-6 mb-2">
                <div class="small text-muted mb-1">Status jurnal periode aktif</div>
                @foreach (['draft' => 'Draft', 'pending' => 'Menunggu approval', 'posted' => 'Posted', 'reversed' => 'Dibalik'] as $k => $name)
                    <span class="badge badge-{{ ['draft' => 'light', 'pending' => 'warning', 'posted' => 'success', 'reversed' => 'secondary'][$k] }} mr-1">
                        {{ $name }}: {{ $journalStatus[$k] ?? 0 }}
                    </span>
                @endforeach
                @if (($journalStatus['pending'] ?? 0) > 0)
                    @can('viewjournal_validation')
                        <a href="{{ url('journalvalidation') }}" class="small ml-1">Buka approval &rarr;</a>
                    @endcan
                @endif
            </div>
            <div class="col-md-6 mb-2">
                <div class="small text-muted mb-1">Posting otomatis gagal</div>
                @if ($failedPostingCount === 0)
                    <span class="badge badge-success">Tidak ada</span>
                @else
                    <span class="badge badge-danger">{{ $failedPostingCount }} gagal</span>
                    @can('viewauto_posting')
                        <a href="{{ url('autoposting') }}" class="small ml-1">Lihat semua &rarr;</a>
                    @endcan
                    <ul class="small mb-0 pl-3 mt-1">
                        @foreach ($failedPostings as $f)
                            <li>{{ $f->posting_date }} &middot; {{ $f->kind }} &middot; {{ $f->reference }} &mdash; {{ \Illuminate\Support\Str::limit($f->message, 80) }}</li>
                        @endforeach
                    </ul>
                @endif
            </div>
        </div>
    </div>
</div>
