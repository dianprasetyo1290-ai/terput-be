<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="End Of Month" :breadcrumb="['Transaction', 'End Of Month']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title"><i class="mr-2 icon-calendar3"></i>End Of Month</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>
        <div class="card-body">
            @if ($period == 13)
                {{-- End of Year Redirect --}}
                <div class="p-6 text-center border border-blue-100 bg-blue-50 rounded-xl">
                    <div class="flex items-center justify-center w-16 h-16 mx-auto mb-4 rounded-full" style="background-color: #dbeafe;">
                        <i class="icon-calendar52" style="color: #2563eb; font-size: 1.5rem;"></i>
                    </div>
                    <h4 class="mb-2 text-lg font-semibold" style="color: #1e40af;">Periode Sudah di Period 13 (Adjustment)</h4>
                    <p class="mb-5 text-sm" style="color: #64748b;">
                        Semua periode reguler telah ditutup. Silakan lanjutkan ke proses <strong>End of Year</strong> untuk menutup tahun buku {{ $year }}.
                    </p>
                    <a href="{{ url('endofyear') }}" class="btn btn-primary btn-lg rounded-lg shadow-sm">
                        <i class="mr-2 icon-arrow-right7"></i> Proses End Of Year
                    </a>
                </div>
            @else
                @php
                    $currentPeriod = $period;
                    $nextPeriod = $period + 1;
                    $currentPadded = str_pad($currentPeriod, 2, '0', STR_PAD_LEFT);
                    $nextPadded = str_pad($nextPeriod, 2, '0', STR_PAD_LEFT);
                    $periodModel = App\Models\Period::where('period_name', 'like', '%' . $year . '%')->first();
                    $currentMonthName = $periodModel ? $periodModel->{'p' . $currentPadded} : '-';
                    $nextMonthName = $periodModel ? $periodModel->{'p' . $nextPadded} : '-';
                @endphp

                {{-- Period Transition Info --}}
                <div class="p-6 mb-6 border border-blue-100 bg-blue-50 rounded-xl">
                    <div class="flex items-center mb-3">
                        <i class="mr-2 icon-calendar3" style="color: #2563eb;"></i>
                        <span class="text-sm font-semibold tracking-wide uppercase" style="color: #2563eb;">Transisi Periode</span>
                    </div>
                    <div class="flex flex-wrap items-center justify-center gap-4 py-4">
                        {{-- Current Period --}}
                        <div class="px-6 py-4 text-center bg-white border border-blue-200 rounded-xl shadow-sm">
                            <div class="text-xs font-medium tracking-wide uppercase" style="color: #64748b;">Periode Aktif</div>
                            <div class="mt-1 text-2xl font-bold" style="color: #1e40af;">[{{ $currentPadded }}]</div>
                            <div class="mt-1 text-sm font-medium" style="color: #3b82f6;">{{ $currentMonthName }}</div>
                        </div>

                        {{-- Arrow --}}
                        <div class="flex items-center">
                            <i class="icon-arrow-right7" style="color: #3b82f6; font-size: 1.5rem;"></i>
                        </div>

                        {{-- Next Period --}}
                        <div class="px-6 py-4 text-center bg-white border border-blue-200 rounded-xl shadow-sm">
                            <div class="text-xs font-medium tracking-wide uppercase" style="color: #64748b;">Periode Berikutnya</div>
                            <div class="mt-1 text-2xl font-bold" style="color: #1e40af;">[{{ $nextPadded }}]</div>
                            <div class="mt-1 text-sm font-medium" style="color: #3b82f6;">{{ $nextMonthName }}</div>
                        </div>
                    </div>
                    <div class="mt-2 text-sm text-center" style="color: #64748b;">
                        Tahun Buku: <strong style="color: #1e40af;">{{ $year }}</strong>
                    </div>
                </div>

                {{-- Process Checklist --}}
                <div class="p-6 mb-6 bg-white border border-gray-200 rounded-xl">
                    <div class="flex items-center mb-4">
                        <i class="mr-2 icon-clipboard3" style="color: #2563eb;"></i>
                        <span class="text-sm font-semibold tracking-wide uppercase" style="color: #2563eb;">Validasi Proses</span>
                    </div>
                    <div class="space-y-3">
                        @foreach ($checks as $check)
                            @if ($check['passed'])
                                <div class="flex items-center px-4 py-3 rounded-lg" style="background-color: #f0fdf4; border: 1px solid #bbf7d0;">
                                    <i class="mr-3 icon-checkmark3" style="color: #16a34a;"></i>
                                    <div>
                                        <span class="text-sm font-medium" style="color: #374151;">{{ $check['label'] }}</span>
                                        <span class="block text-xs" style="color: #6b7280;">{{ $check['desc'] }}</span>
                                    </div>
                                    <span class="ml-auto text-xs font-medium px-2 py-1 rounded-full" style="background-color: #dcfce7; color: #166534;">Passed</span>
                                </div>
                            @else
                                <div class="flex items-center px-4 py-3 rounded-lg" style="background-color: #fef2f2; border: 1px solid #fecaca;">
                                    <i class="mr-3 icon-cross2" style="color: #dc2626;"></i>
                                    <div>
                                        <span class="text-sm font-medium" style="color: #374151;">{{ $check['label'] }}</span>
                                        <span class="block text-xs" style="color: #ef4444;">{{ $check['issue'] }} masalah ditemukan</span>
                                    </div>
                                    <span class="ml-auto text-xs font-medium px-2 py-1 rounded-full" style="background-color: #fee2e2; color: #991b1b;">Failed</span>
                                </div>
                            @endif
                        @endforeach
                    </div>
                </div>

                {{-- Warning Alert --}}
                <div class="p-4 mb-6 rounded-lg" style="background-color: #fffbeb; border-left: 4px solid #f59e0b;">
                    <div class="flex items-start">
                        <i class="mt-0.5 mr-3 icon-warning2" style="color: #d97706;"></i>
                        <div>
                            <h6 class="mb-1 text-sm font-semibold" style="color: #92400e;">Perhatian</h6>
                            <p class="text-sm" style="color: #a16207;">
                                Proses End of Month bersifat <strong>irreversible</strong>. Semua transaksi jurnal pada periode aktif akan dipindahkan ke tabel history dan tidak dapat dikembalikan. Pastikan semua data sudah benar sebelum melanjutkan.
                            </p>
                        </div>
                    </div>
                </div>

                {{-- Action Button --}}
                <form wire:submit="confirmTransfer">
                    @can('createend_of_month')
                        <div class="text-center">
                            @if ($allPassed)
                                <div wire:loading.remove wire:target='transfer'>
                                    <button type="submit" class="btn btn-primary btn-lg rounded-lg shadow-sm">
                                        <i class="mr-2 icon-checkmark3"></i> Process End of Month
                                    </button>
                                </div>
                                <div wire:loading wire:target="transfer">
                                    <button type="button" class="btn btn-lg rounded-lg" style="background-color: #64748b; color: #fff;" disabled>
                                        <i class="mr-2 icon-spinner2 spinner"></i> Processing...
                                    </button>
                                </div>
                            @else
                                <button type="button" class="btn btn-lg rounded-lg" style="background-color: #94a3b8; color: #fff; cursor: not-allowed;" disabled>
                                    <i class="mr-2 icon-blocked"></i> Validasi Belum Lengkap
                                </button>
                            @endif
                        </div>
                    @endcan
                </form>
            @endif
        </div>
    </div>
</div>
