<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="End Of Year" :breadcrumb="['Transaction', 'End Of Year']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title"><i class="mr-2 icon-calendar52"></i>End Of Year</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>
        <div class="card-body">
            {{-- Loading Overlay --}}
            <div wire:loading wire:target="generate">
                <div class="position-fixed w-100 h-100 d-flex align-items-center justify-content-center"
                    style="top:0; left:0; background:rgba(255,255,255,0.7); z-index:2000;">
                    <div class="bg-white rounded p-4 shadow border" style="min-width:300px;">
                        <div class="d-flex flex-column justify-content-center align-items-center my-2">
                            <div class="dot-loader">
                                <div></div>
                                <div></div>
                                <div></div>
                            </div>
                            <div class="mt-3 text-dark font-weight-bold">Memproses End of Year, mohon tunggu...</div>
                        </div>
                    </div>

                    <style>
                        .dot-loader {
                            display: flex;
                            gap: 0.4rem;
                        }

                        .dot-loader div {
                            width: 14px;
                            height: 14px;
                            background: #007bff;
                            border-radius: 50%;
                            animation: dot-flash 1.1s infinite alternate;
                        }

                        .dot-loader div:nth-child(2) {
                            animation-delay: 0.2s;
                        }

                        .dot-loader div:nth-child(3) {
                            animation-delay: 0.4s;
                        }

                        @keyframes dot-flash {
                            0% {
                                opacity: 0.2;
                                transform: scale(1);
                            }

                            100% {
                                opacity: 1;
                                transform: scale(1.3);
                            }
                        }
                    </style>
                </div>
            </div>

            @if ($justRunMessage)
                <div class="p-4 mb-6 rounded-lg" style="background-color: #f0fdf4; border: 1px solid #bbf7d0;">
                    <div class="flex items-center">
                        <i class="mr-3 icon-checkmark-circle2" style="color: #16a34a; font-size: 1.25rem;"></i>
                        <div>
                            <span class="text-sm font-semibold" style="color: #166534;">End of Year berhasil!</span>
                            <span class="block text-xs" style="color: #15803d;">{{ $justRunMessage }} Silakan refresh halaman untuk melihat data periode baru.</span>
                        </div>
                    </div>
                </div>
            @endif

            @if ($failureMessage)
                <div class="p-4 mb-6 rounded-lg" style="background-color: #fef2f2; border: 1px solid #fecaca;">
                    <div class="flex items-start">
                        <i class="mt-1 mr-3 icon-cross2" style="color: #dc2626; font-size: 1.25rem;"></i>
                        <div>
                            <span class="text-sm font-semibold" style="color: #991b1b;">End of Year gagal</span>
                            <span class="block text-xs" style="color: #b91c1c;">{{ $failureMessage }}</span>
                        </div>
                    </div>
                </div>
            @endif

            @if ((int) $periode->period === 13)
                {{-- Year Transition Info --}}
                @php
                    $currentYear = (int) $periode->year;
                    $nextYear = $currentYear + 1;
                @endphp

                <div class="p-6 mb-6 border border-blue-100 bg-blue-50 rounded-xl">
                    <div class="flex items-center mb-3">
                        <i class="mr-2 icon-calendar52" style="color: #2563eb;"></i>
                        <span class="text-sm font-semibold tracking-wide uppercase" style="color: #2563eb;">Tutup Tahun Buku</span>
                    </div>
                    <div class="flex flex-wrap items-center justify-center gap-4 py-4">
                        {{-- Current Year --}}
                        <div class="px-6 py-4 text-center bg-white border border-blue-200 rounded-xl shadow-sm">
                            <div class="text-xs font-medium tracking-wide uppercase" style="color: #64748b;">Tahun Berjalan</div>
                            <div class="mt-1 text-2xl font-bold" style="color: #1e40af;">{{ $currentYear }}</div>
                            <div class="mt-1 text-sm font-medium" style="color: #3b82f6;">Periode 13 (Adjustment)</div>
                        </div>

                        {{-- Arrow --}}
                        <div class="flex items-center">
                            <i class="icon-arrow-right7" style="color: #3b82f6; font-size: 1.5rem;"></i>
                        </div>

                        {{-- Next Year --}}
                        <div class="px-6 py-4 text-center bg-white border border-blue-200 rounded-xl shadow-sm">
                            <div class="text-xs font-medium tracking-wide uppercase" style="color: #64748b;">Tahun Baru</div>
                            <div class="mt-1 text-2xl font-bold" style="color: #1e40af;">{{ $nextYear }}</div>
                            <div class="mt-1 text-sm font-medium" style="color: #3b82f6;">Periode 1</div>
                        </div>
                    </div>
                </div>

                {{-- Validation Checklist --}}
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
                                        <span class="block text-xs" style="color: #ef4444;">{{ $check['desc'] }} &mdash; {{ $check['issue'] }} masalah</span>
                                    </div>
                                    <span class="ml-auto text-xs font-medium px-2 py-1 rounded-full" style="background-color: #fee2e2; color: #991b1b;">Failed</span>
                                </div>
                            @endif
                        @endforeach
                    </div>
                </div>

                {{-- Preview Card --}}
                @if ($allPassed)
                    <div class="p-6 mb-6 bg-white border border-gray-200 rounded-xl">
                        <div class="flex items-center mb-4">
                            <i class="mr-2 icon-stats-dots" style="color: #2563eb;"></i>
                            <span class="text-sm font-semibold tracking-wide uppercase" style="color: #2563eb;">Ringkasan Closing</span>
                        </div>
                        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                            <div class="px-4 py-3 rounded-lg" style="background-color: #f0f9ff; border: 1px solid #bae6fd;">
                                <div class="text-xs font-medium uppercase" style="color: #64748b;">Akun Real (Carry Forward)</div>
                                <div class="mt-1 text-xl font-bold" style="color: #0369a1;">{{ $realAccountCount }}</div>
                                <div class="text-xs" style="color: #64748b;">Asset, Liability, Equity</div>
                            </div>
                            <div class="px-4 py-3 rounded-lg" style="background-color: #fef3c7; border: 1px solid #fde68a;">
                                <div class="text-xs font-medium uppercase" style="color: #64748b;">Akun Nominal (Di-nolkan)</div>
                                <div class="mt-1 text-xl font-bold" style="color: #b45309;">{{ $nominalAccountCount }}</div>
                                <div class="text-xs" style="color: #64748b;">Income, Expense, CoS, Other</div>
                            </div>
                            <div class="px-4 py-3 rounded-lg" style="background-color: {{ $netIncome >= 0 ? '#f0fdf4' : '#fef2f2' }}; border: 1px solid {{ $netIncome >= 0 ? '#bbf7d0' : '#fecaca' }};">
                                <div class="text-xs font-medium uppercase" style="color: #64748b;">Net Income &rarr; Retained Earnings</div>
                                <div class="mt-1 text-xl font-bold" style="color: {{ $netIncome >= 0 ? '#166534' : '#991b1b' }};">
                                    Rp {{ number_format($netIncome, 2, ',', '.') }}
                                </div>
                                <div class="text-xs" style="color: #64748b;">{{ $netIncome >= 0 ? 'Laba Bersih' : 'Rugi Bersih' }} Tahun {{ $currentYear }}</div>
                            </div>
                        </div>
                    </div>
                @endif

                {{-- Info Card - Proses EOY --}}
                <div class="p-4 mb-6 rounded-lg" style="background-color: #eff6ff; border-left: 4px solid #3b82f6;">
                    <div class="flex items-start">
                        <i class="mt-0.5 mr-3 icon-info22" style="color: #2563eb;"></i>
                        <div>
                            <h6 class="mb-2 text-sm font-semibold" style="color: #1e40af;">Proses End of Year</h6>
                            <ul class="text-sm space-y-1" style="color: #3b82f6; list-style: disc; padding-left: 1.25rem;">
                                <li>Akun <strong>REAL</strong> (Asset, Liability, Equity) di-carry forward per divisi ke saldo awal tahun baru</li>
                                <li>Akun <strong>NOMINAL</strong> (Income, Expense, Cost of Sales, Other Income/Expense) di-nolkan</li>
                                <li>Net Income dari seluruh akun nominal <strong>diagregasi ke divisi 0000</strong> (level perusahaan) dan dimasukkan ke akun Retained Earnings</li>
                                <li>Jurnal adjustment periode 13 (jika ada) akan di-transfer ke history terlebih dahulu</li>
                            </ul>
                            <p class="mt-2 text-xs" style="color: #64748b;">
                                Laba/rugi bersih merupakan hasil keseluruhan perusahaan, sehingga dicatat pada divisi induk (0000) untuk menjaga integritas laporan keuangan per divisi.
                            </p>
                        </div>
                    </div>
                </div>

                {{-- Warning Alert --}}
                <div class="p-4 mb-6 rounded-lg" style="background-color: #fffbeb; border-left: 4px solid #f59e0b;">
                    <div class="flex items-start">
                        <i class="mt-0.5 mr-3 icon-warning2" style="color: #d97706;"></i>
                        <div>
                            <h6 class="mb-1 text-sm font-semibold" style="color: #92400e;">Perhatian</h6>
                            <p class="text-sm" style="color: #a16207;">
                                Proses End of Year bersifat <strong>irreversible</strong>. Saldo awal tahun {{ $nextYear }} akan dibuat, akun nominal di-nolkan, dan net income dipindahkan ke Retained Earnings. Pastikan semua data sudah benar sebelum melanjutkan.
                            </p>
                        </div>
                    </div>
                </div>

                {{-- Action Button --}}
                @if (!$disabled)
                    <div>
                        @can('createend_of_year')
                            <div class="text-center">
                                @if ($allPassed)
                                    <div wire:loading.remove wire:target='generate'>
                                        <button type="button" wire:click="confirmGenerate"
                                            wire:loading.attr="disabled"
                                            wire:target="confirmGenerate,generate"
                                            class="btn btn-primary btn-lg rounded-lg shadow-sm">
                                            <i class="mr-2 icon-checkmark3"></i> Process End of Year
                                        </button>
                                    </div>
                                    <div wire:loading wire:target="generate">
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
                    </div>
                @endif

                @if ($answer)
                    <div class="p-4 mt-6 rounded-lg" style="background-color: #f8fafc; border: 1px solid #e2e8f0;">
                        <div class="flex items-center mb-3">
                            <i class="mr-2 icon-file-text2" style="color: #2563eb;"></i>
                            <span class="text-sm font-semibold" style="color: #2563eb;">Log Proses</span>
                        </div>
                        <pre class="text-sm" style="color: #374151; white-space: pre-wrap; font-family: monospace;" wire:stream="answer">{{ $answer }}</pre>
                    </div>
                @endif
            @else
                {{-- Period != 13: Redirect to EOM --}}
                <div class="p-6 text-center border border-blue-100 bg-blue-50 rounded-xl">
                    <div class="flex items-center justify-center w-16 h-16 mx-auto mb-4 rounded-full" style="background-color: #dbeafe;">
                        <i class="icon-calendar3" style="color: #2563eb; font-size: 1.5rem;"></i>
                    </div>
                    <h4 class="mb-2 text-lg font-semibold" style="color: #1e40af;">Periode Belum di Period 13</h4>
                    <p class="mb-2 text-sm" style="color: #64748b;">
                        Saat ini periode aktif adalah <strong style="color: #1e40af;">{{ $periode->period }}</strong> (Tahun {{ $periode->year }}).
                    </p>
                    <p class="mb-5 text-sm" style="color: #64748b;">
                        Selesaikan proses <strong>End of Month</strong> untuk semua periode hingga periode 13 (Adjustment) sebelum menjalankan End of Year.
                    </p>
                    <a href="{{ url('endofmonth') }}" class="btn btn-primary btn-lg rounded-lg shadow-sm">
                        <i class="mr-2 icon-arrow-right7"></i> Proses End Of Month
                    </a>
                </div>
            @endif
        </div>
    </div>

    @if ($showConfirmModal)
        <div class="modal fade show d-block" tabindex="-1" style="background: rgba(15, 23, 42, .55);">
            <div class="modal-dialog modal-dialog-centered">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">Finalize End of Year</h5>
                        <button type="button" class="close" wire:click="cancelGenerate">×</button>
                    </div>
                    <div class="modal-body">
                        <p class="mb-2">
                            Proses End of Year akan menutup tahun buku {{ $periode->year }} dan membuat saldo awal tahun {{ (int) $periode->year + 1 }}.
                        </p>
                        <div class="rounded p-3" style="background:#fffbeb; border:1px solid #fde68a;">
                            Proses ini bersifat irreversible. Pastikan semua jurnal periode 13 sudah benar dan semua validasi sudah passed.
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-light" wire:click="cancelGenerate">Cancel</button>
                        <button type="button" class="btn btn-primary" wire:click="generate"
                            wire:loading.attr="disabled"
                            wire:target="generate">
                            <span wire:loading.remove wire:target="generate">
                                <i class="icon-checkmark3 mr-2"></i>Process End of Year
                            </span>
                            <span wire:loading wire:target="generate">
                                <i class="icon-spinner2 spinner mr-2"></i>Processing...
                            </span>
                        </button>
                    </div>
                </div>
            </div>
        </div>
    @endif
</div>
