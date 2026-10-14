@can('createjournal_transaction')
<div class="mb-5">
    <button type="button" class="btn btn-primary btn-lg rounded-lg shadow-sm" wire:click="add">
        <i class="mr-2 icon-plus-circle2"></i>Tambah Transaksi Jurnal Baru
    </button>
</div>
@endcan
@if ($workflowError)
    <div class="alert alert-danger">{{ $workflowError }}</div>
@endif
@if ($lastReversal)
    <div class="alert alert-success">Jurnal pembalik {{ $lastReversal }} telah dibuat dan diposting.</div>
@endif

{{-- Filter and Search Section --}}
<div class="p-4 mb-5 bg-white border border-gray-200 rounded-xl">
    <div class="row align-items-end">
        <div class="mb-3 col-md-4 mb-md-0">
            <label class="mb-1 text-sm font-medium text-gray-700">Cari Transaksi</label>
            <div class="search-input">
                <input type="search" class="form-control"
                    placeholder="Cari Kode atau No. Transaksi..." wire:model.live='search'>
            </div>
        </div>
        <div class="mb-3 col-md-2 mb-md-0">
            <label class="mb-1 text-sm font-medium text-gray-700">Urutkan</label>
            <div class="btn-group justify-content-center w-100">
                <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                    aria-expanded="false">
                    @if ($order == 'DESC')
                        Terbaru
                    @else
                        Terlama
                    @endif
                </a>
                <div class="dropdown-menu" x-placement="bottom-start">
                    <a class="dropdown-item" href="#" wire:click.prevent="sort('DESC')">Terbaru</a>
                    <a class="dropdown-item" href="#" wire:click.prevent="sort('ASC')">Terlama</a>
                </div>
            </div>
        </div>
        <div class="mb-3 col-md-2 mb-md-0">
            <label class="mb-1 text-sm font-medium text-gray-700">Tampilkan</label>
            <div class="btn-group justify-content-center w-100">
                <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                    aria-expanded="false">
                    {{ $limit }} item
                </a>
                <div class="dropdown-menu" x-placement="bottom-start">
                    <a class="dropdown-item" href="#" wire:click.prevent="limits('10')">10 item</a>
                    <a class="dropdown-item" href="#" wire:click.prevent="limits('30')">30 item</a>
                    <a class="dropdown-item" href="#" wire:click.prevent="limits('50')">50 item</a>
                    <a class="dropdown-item" href="#" wire:click.prevent="limits('100')">100 item</a>
                </div>
            </div>
        </div>
        <div class="col-md-2">
            <label class="mb-1 text-sm font-medium text-gray-700">Filter Status</label>
            <div class="btn-group justify-content-center w-100">
                <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown"
                    aria-expanded="false">
                    @if ($filter == 'b')
                        Seimbang
                    @elseif ($filter == 'u')
                        Belum Seimbang
                    @else
                        Semua
                    @endif
                </a>
                <div class="dropdown-menu" x-placement="bottom-start">
                    <a class="dropdown-item" href="#" wire:click.prevent="filt('a')">Semua</a>
                    <a class="dropdown-item" href="#" wire:click.prevent="filt('b')">Seimbang</a>
                    <a class="dropdown-item" href="#" wire:click.prevent="filt('u')">Belum Seimbang</a>
                </div>
            </div>
        </div>
        <div class="col-md-2">
            <label class="mb-1 text-sm font-medium text-gray-700">Status Jurnal</label>
            <select class="form-control" wire:model.live="statusFilter">
                <option value="">Semua</option>
                <option value="draft">Draft</option>
                <option value="pending">Menunggu Approval</option>
                <option value="posted">Posted</option>
                <option value="reversed">Dibalik</option>
            </select>
        </div>
        <div class="col-md-2">
            <label class="mb-1 text-sm font-medium text-gray-700">Periode</label>
            <div class="px-3 py-2 text-sm text-gray-500 bg-gray-50 rounded-lg border border-gray-200">
                <i class="mr-1 icon-calendar5"></i>
                <span class="font-semibold text-gray-700">{{ \Carbon\Carbon::parse($dfrom)->format('d M Y') }}</span>
                <span class="mx-1">-</span>
                <span class="font-semibold text-gray-700">{{ \Carbon\Carbon::parse($dto)->format('d M Y') }}</span>
            </div>
        </div>
    </div>
</div>

<div class="datatable-scroll">
    <table class="table table-hover">
        <thead>
            <tr role="row">
                <th class="text-center" style="width: 40px">#</th>
                <th>Kode Transaksi</th>
                <th>No. Transaksi</th>
                <th>Tanggal</th>
                <th class="text-right">Total Debit</th>
                <th class="text-right">Total Kredit</th>
                <th class="text-center">Status Saldo</th>
                <th class="text-center">Status Jurnal</th>
                <th class="text-center">Detail</th>
                <th class="text-center" style="width: 60px">Aksi</th>
            </tr>
        </thead>
        <tbody>
            @forelse ($data as $row)
                @php
                    $totalDebet = $row->transDetail2->sum('debet');
                    $totalCredit = $row->transDetail2->sum('credit');
                    $isBalanced = $totalDebet == $totalCredit && $totalDebet > 0;
                    $hasEntries = $totalDebet > 0 || $totalCredit > 0;
                @endphp
                <tr role="row">
                    <td class="text-center text-gray-400">{{ $loop->iteration }}</td>
                    <td><span class="px-2 py-1 rounded-md" style="background-color: #dbeafe; color: #1e40af; font-size: 0.75rem; font-weight: 600; border: 1px solid #bfdbfe;">{{ $row->trans_code }}</span></td>
                    <td><span class="font-semibold text-gray-800">{{ $row->trans_no }}</span></td>
                    <td class="text-gray-600">{{ \Carbon\Carbon::parse($row->trans_date)->format('d M Y') }}</td>
                    <td class="text-right font-semibold text-emerald-600">{{ number_format($totalDebet ?? 0, 2, ',', '.') }}</td>
                    <td class="text-right font-semibold text-red-500">{{ number_format($totalCredit ?? 0, 2, ',', '.') }}</td>
                    <td class="text-center">
                        @if ($isBalanced)
                            <span class="badge badge-success"><i class="mr-1 icon-check"></i> Seimbang</span>
                        @elseif ($hasEntries)
                            <span class="badge badge-warning"><i class="mr-1 icon-alert"></i> Belum Seimbang</span>
                        @else
                            <span class="badge badge-secondary">Tidak Ada Entri</span>
                        @endif
                    </td>
                    <td class="text-center">
                        @php $st = $row->status ?? 'draft'; @endphp
                        <span class="badge {{ ['draft' => 'badge-secondary', 'pending' => 'badge-warning', 'posted' => 'badge-success', 'reversed' => 'badge-danger'][$st] ?? 'badge-secondary' }}">
                            {{ ['draft' => 'Draft', 'pending' => 'Menunggu Approval', 'posted' => 'Posted', 'reversed' => 'Dibalik'][$st] ?? $st }}
                        </span>
                    </td>
                    <td class="text-center">
                        <a href="#" wire:click.prevent='detail("{{ Crypt::encrypt($row->trans_no) }}")'
                            class="btn btn-sm btn-outline-info">
                            <i class="mr-1 icon-list-unordered"></i> Lihat
                        </a>
                    </td>
                    <td class="text-center">
                        {{-- BR-ACC-01: only drafts are editable; posted ones are corrected by a reversal (QA BUG-062). --}}
                        @php $st = $row->status ?? 'draft'; @endphp
                        <div class="list-icons">
                            <div class="dropdown">
                                <a href="#" class="list-icons-item" data-toggle="dropdown">
                                    <i class="icon-menu9"></i>
                                </a>
                                <div class="dropdown-menu dropdown-menu-right">
                                    @if ($st === 'draft')
                                        @can('editjournal_transaction')
                                            <a href="#" class="dropdown-item"
                                                wire:click.prevent='add("{{ Crypt::encrypt($row->trans_no) }}")'>
                                                <i class="icon-pencil"></i> Edit
                                            </a>
                                            <a href="#" class="dropdown-item"
                                                wire:click.prevent="submitJournal('{{ $row->trans_no }}')"
                                                wire:confirm="Ajukan jurnal {{ $row->trans_no }} untuk approval?">
                                                <i class="icon-paperplane"></i> Ajukan Approval
                                            </a>
                                        @endcan
                                        @can('deletejournal_transaction')
                                            <a href="#" class="dropdown-item text-danger"
                                                wire:click.prevent='confirmDelete("{{ Crypt::encrypt($row->trans_no) }}")'>
                                                <i class="icon-trash"></i> Hapus
                                            </a>
                                        @endcan
                                    @elseif ($st === 'pending')
                                        @can('editjournal_validation')
                                            <a href="#" class="dropdown-item"
                                                wire:click.prevent="approveJournal('{{ $row->trans_no }}')"
                                                wire:confirm="Setujui dan posting jurnal {{ $row->trans_no }}?">
                                                <i class="icon-checkmark"></i> Setujui &amp; Posting
                                            </a>
                                            <a href="#" class="dropdown-item"
                                                wire:click.prevent="rejectJournal('{{ $row->trans_no }}')"
                                                wire:confirm="Kembalikan jurnal {{ $row->trans_no }} ke draft?">
                                                <i class="icon-undo"></i> Tolak (ke Draft)
                                            </a>
                                        @else
                                            <span class="dropdown-item text-muted">Menunggu penyetuju</span>
                                        @endcan
                                    @elseif ($st === 'posted' && empty($row->reversal_of))
                                        @can('editjournal_validation')
                                            <a href="#" class="dropdown-item text-danger"
                                                wire:click.prevent="reverseJournal('{{ $row->trans_no }}')"
                                                wire:confirm="Buat jurnal pembalik untuk {{ $row->trans_no }}?">
                                                <i class="icon-undo2"></i> Buat Jurnal Pembalik
                                            </a>
                                        @else
                                            <span class="dropdown-item text-muted">Posted (koreksi via jurnal pembalik)</span>
                                        @endcan
                                    @else
                                        <span class="dropdown-item text-muted">Tidak ada aksi</span>
                                    @endif
                                </div>
                            </div>
                        </div>
                    </td>
                </tr>
            @empty
                <tr>
                    <td colspan="10" class="py-8 text-center">
                        <div class="flex flex-col items-center justify-center">
                            <i class="mb-3 icon-file-empty2" style="font-size: 2.5rem; color: #cbd5e1"></i>
                            <p class="text-sm text-gray-400">Tidak ada data transaksi jurnal yang ditemukan.</p>
                        </div>
                    </td>
                </tr>
            @endforelse
        </tbody>
    </table>
</div>
<div class="mt-4">
    {{ $data->links() }}
</div>
