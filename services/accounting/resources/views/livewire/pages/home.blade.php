<div>
    {{-- Welcome Header --}}
    <div style="margin-bottom: 1.5rem;">
        <h4 style="font-size: 1.5rem; font-weight: 700; color: #1e293b; margin: 0 0 0.25rem 0;">
            Welcome back, {{ $userName }}
        </h4>
        <p style="color: #64748b; margin: 0; font-size: 0.9rem;">
            {{ $companyName }} &mdash; General Ledger System
        </p>
    </div>

    @include('livewire.pages.partials.home-kpi')

    {{-- Summary Cards --}}
    <div class="row">
        {{-- Active Period --}}
        <div class="col-sm-6 col-xl-3">
            <div class="card" style="border: none; border-left: 4px solid #2563eb; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.08); overflow: hidden;">
                <div class="card-body" style="padding: 1.25rem;">
                    <div style="display: flex; align-items: flex-start; justify-content: space-between;">
                        <div>
                            <p style="font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #64748b; margin: 0 0 0.5rem 0;">
                                Active Period
                            </p>
                            <h3 style="font-size: 1.5rem; font-weight: 700; color: #1e293b; margin: 0 0 0.25rem 0;">
                                {{ $periodName ?? '-' }}
                            </h3>
                            <p style="font-size: 0.8rem; color: #94a3b8; margin: 0;">
                                Period {{ $parameter?->period ?? '-' }} / {{ $parameter?->year ?? '-' }}
                            </p>
                        </div>
                        <div style="width: 44px; height: 44px; background: linear-gradient(135deg, #dbeafe, #bfdbfe); border-radius: 10px; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                            <svg style="width: 22px; height: 22px; color: #2563eb;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/>
                            </svg>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        {{-- Total Accounts --}}
        <div class="col-sm-6 col-xl-3">
            <div class="card" style="border: none; border-left: 4px solid #059669; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.08); overflow: hidden;">
                <div class="card-body" style="padding: 1.25rem;">
                    <div style="display: flex; align-items: flex-start; justify-content: space-between;">
                        <div>
                            <p style="font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #64748b; margin: 0 0 0.5rem 0;">
                                Total Accounts
                            </p>
                            <h3 style="font-size: 1.5rem; font-weight: 700; color: #1e293b; margin: 0 0 0.25rem 0;">
                                {{ number_format($totalAccounts) }}
                            </h3>
                            <p style="font-size: 0.8rem; color: #94a3b8; margin: 0;">
                                Chart of Accounts
                            </p>
                        </div>
                        <div style="width: 44px; height: 44px; background: linear-gradient(135deg, #d1fae5, #a7f3d0); border-radius: 10px; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                            <svg style="width: 22px; height: 22px; color: #059669;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 7h6m0 10v-3m-3 3h.01M9 17h.01M9 14h.01M12 14h.01M15 11h.01M12 11h.01M9 11h.01M7 21h10a2 2 0 002-2V5a2 2 0 00-2-2H7a2 2 0 00-2 2v14a2 2 0 002 2z"/>
                            </svg>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        {{-- Total Transactions --}}
        <div class="col-sm-6 col-xl-3">
            <div class="card" style="border: none; border-left: 4px solid #d97706; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.08); overflow: hidden;">
                <div class="card-body" style="padding: 1.25rem;">
                    <div style="display: flex; align-items: flex-start; justify-content: space-between;">
                        <div>
                            <p style="font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #64748b; margin: 0 0 0.5rem 0;">
                                Transactions
                            </p>
                            <h3 style="font-size: 1.5rem; font-weight: 700; color: #1e293b; margin: 0 0 0.25rem 0;">
                                {{ number_format($totalTransactions) }}
                            </h3>
                            <p style="font-size: 0.8rem; color: #94a3b8; margin: 0;">
                                Current Period
                            </p>
                        </div>
                        <div style="width: 44px; height: 44px; background: linear-gradient(135deg, #fef3c7, #fde68a); border-radius: 10px; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                            <svg style="width: 22px; height: 22px; color: #d97706;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"/>
                            </svg>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        {{-- Total Divisions --}}
        <div class="col-sm-6 col-xl-3">
            <div class="card" style="border: none; border-left: 4px solid #7c3aed; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.08); overflow: hidden;">
                <div class="card-body" style="padding: 1.25rem;">
                    <div style="display: flex; align-items: flex-start; justify-content: space-between;">
                        <div>
                            <p style="font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #64748b; margin: 0 0 0.5rem 0;">
                                Divisions
                            </p>
                            <h3 style="font-size: 1.5rem; font-weight: 700; color: #1e293b; margin: 0 0 0.25rem 0;">
                                {{ number_format($totalDivisions) }}
                            </h3>
                            <p style="font-size: 0.8rem; color: #94a3b8; margin: 0;">
                                Organization Units
                            </p>
                        </div>
                        <div style="width: 44px; height: 44px; background: linear-gradient(135deg, #ede9fe, #ddd6fe); border-radius: 10px; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                            <svg style="width: 22px; height: 22px; color: #7c3aed;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4"/>
                            </svg>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>

    {{-- Trend & Operational Overview --}}
    <div class="row mt-4">
        <div class="col-12">
            <div class="card" style="border: none; border-radius: 10px; box-shadow: 0 1px 3px rgba(0,0,0,0.08);">
                <div class="card-header" style="background: #ffffff; border-bottom: 1px solid #e2e8f0; padding: 1rem 1.25rem; font-weight: 700; color: #1e293b; border-top-left-radius: 10px; border-top-right-radius: 10px; font-size: 1rem;">
                    <div style="display: flex; justify-content: space-between; align-items: center; gap: 1rem; flex-wrap: wrap;">
                        <span>Journal Trend ({{ $trendYear }}) — transaksi per bulan</span>
                        <div style="display:flex; align-items:center; gap: .5rem; font-weight: 500; color: #475569; font-size: .82rem;">
                            <label for="trendYearFilter" style="margin: 0; white-space: nowrap;">Filter Tahun:</label>
                            <select id="trendYearFilter" wire:model.live="trendYearFilter"
                                style="border: 1px solid #cbd5e1; border-radius: 8px; padding: .34rem .55rem; color: #0f172a; background: #fff;">
                                @foreach ($trendYearOptions as $yearOption)
                                    <option value="{{ $yearOption }}">{{ $yearOption }}</option>
                                @endforeach
                            </select>
                        </div>
                    </div>
                </div>
                <div class="card-body" style="padding: 1rem 1.1rem 1.15rem;">
                    @if ($trendYearHasData)
                        <p style="margin-top: -0.2rem; margin-bottom: 0.8rem; color: #64748b; font-size: .82rem;">
                            Menampilkan data transaksi untuk tahun buku <strong>{{ $trendYear }}</strong>.
                        </p>
                    @else
                        <p style="margin-top: -0.2rem; margin-bottom: 0.8rem; color: #dc2626; font-size: .82rem;">
                            Belum ada transaksi untuk tahun buku <strong>{{ $trendYear }}</strong>.
                        </p>
                    @endif
                    <div class="row mb-3">
                        <div class="col-xl-4">
                            <div style="border:1px solid #e2e8f0; border-radius: 8px; padding: 0.9rem; background: #f8fafc; min-height: 112px;">
                                <p style="margin: 0 0 .35rem; color:#64748b; font-size:.78rem; text-transform: uppercase; letter-spacing:.04em;">
                                    Periode aktif
                                </p>
                                <p style="font-size: 1.5rem; color:#0f172a; font-weight: 700; margin: 0 0 .35rem;">
                                    {{ $activeMonthLabel }} {{ $trendYear }}
                                </p>
                                <p style="margin: 0; color: #475569; font-size: .85rem;">
                                    {{ number_format($activeMonthTx) }} jurnal masuk bulan ini
                                </p>
                                <p style="margin: .35rem 0 0; color:#475569; font-size: .8rem;">
                                    Dibanding bulan sebelumnya: <strong>{{ number_format($activeMonthTx - $prevMonthTx) }}</strong> selisih jurnal
                                </p>
                            </div>
                        </div>
                        <div class="col-xl-4">
                            <div style="border:1px solid #e2e8f0; border-radius: 8px; padding: 0.9rem; background: #f8fafc; min-height: 112px;">
                                <p style="margin: 0 0 .35rem; color:#64748b; font-size:.78rem; text-transform: uppercase; letter-spacing:.04em;">
                                    YTD Net Movement (All Accounts)
                                </p>
                                <p style="font-size: 1.5rem; color:#0f172a; font-weight: 700; margin: 0 0 .35rem;">
                                    Rp {{ number_format(abs($yearlyNet), 2, ',', '.') }}
                                </p>
                                <p style="margin: 0 0 .35rem; color: #475569; font-size: .85rem;">
                                    {{ $yearlyNet >= 0 ? 'Positif' : 'Negatif' }} (Credit - Debet) — biasanya 0 karena jurnal double-entry.
                                </p>
                                <p style="margin: 0; color:#475569; font-size: .8rem;">
                                    Debet {{ number_format($yearlyDebit, 2, ',', '.') }} / Credit {{ number_format($yearlyCredit, 2, ',', '.') }}
                                </p>
                                <p style="margin: .35rem 0 0; color: #475569; font-size: .8rem;">
                                    YTD Net (Nominal Akun): <strong>Rp {{ number_format(abs($yearlyNominalNet), 2, ',', '.') }}</strong>
                                    ({{ $yearlyNominalNet >= 0 ? 'Positif' : 'Negatif' }})
                                </p>
                            </div>
                        </div>
                        <div class="col-xl-4">
                            <div style="border:1px solid #e2e8f0; border-radius: 8px; padding: 0.9rem; background: #f8fafc; min-height: 112px;">
                                <p style="margin: 0 0 .35rem; color:#64748b; font-size:.78rem; text-transform: uppercase; letter-spacing:.04em;">
                                    Audit Aktivitas
                                </p>
                                <p style="font-size: 1.05rem; color:#0f172a; font-weight: 700; margin: 0 0 .35rem;">
                                    {{ $periodClosedCount }} / 12 periode terdokumen
                                </p>
                                <p style="margin: 0; color:#475569; font-size: .85rem;">
                                    {{ number_format($uniqueAccountsUsed) }} akun dan {{ number_format($uniqueDivisionsUsed) }} divisi aktif
                                </p>
                                <p style="margin: .35rem 0 0; color:#475569; font-size: .8rem;">
                                    Top transaksi: {{ count($topTransactionTypes) }} jenis sudah muncul
                                </p>
                            </div>
                        </div>
                    </div>

                    <div style="position: relative; height: 320px;" wire:ignore>
                        <canvas id="trendChart"></canvas>
                    </div>
                    <script type="application/json" id="trendChartPayload">
                        {
                            "labels": @json(array_column($monthlyTrend, 'month_label')),
                            "txCounts": @json(array_map(function ($item) {
                                return (int) $item['tx_count'];
                            }, $monthlyTrend)),
                            "nominalNet": @json(array_map(function ($item) {
                                return (float) $item['nominal_net_movement'];
                            }, $monthlyTrend)),
                            "ytdAll": @json((float) $yearlyNet),
                            "ytdNominal": @json((float) $yearlyNominalNet),
                            "ytdAllLabel": "YTD All: Rp {{ number_format(abs($yearlyNet), 0, ',', '.') }} ({{ $yearlyNet >= 0 ? 'Positif' : 'Negatif' }})",
                            "ytdNominalLabel": "YTD Nominal: Rp {{ number_format(abs($yearlyNominalNet), 0, ',', '.') }} ({{ $yearlyNominalNet >= 0 ? 'Positif' : 'Negatif' }})"
                        }
                    </script>
                </div>
            </div>
        </div>
    </div>

    {{-- Quick Info --}}
    <div class="row">
        <div class="col-12">
            <div class="card" style="border: none; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.08);">
                <div class="card-body" style="padding: 1.5rem;">
                    <h5 style="font-weight: 600; color: #1e293b; margin: 0 0 1rem 0; font-size: 1rem;">
                        System Information
                    </h5>
                    <div class="row">
                        <div class="col-md-6">
                            <table style="width: 100%; font-size: 0.875rem;">
                                <tr>
                                    <td style="padding: 0.4rem 0; color: #64748b; width: 140px;">Company</td>
                                    <td style="padding: 0.4rem 0; font-weight: 500; color: #1e293b;">{{ $companyName }}</td>
                                </tr>
                                <tr>
                                    <td style="padding: 0.4rem 0; color: #64748b;">Fiscal Year</td>
                                    <td style="padding: 0.4rem 0; font-weight: 500; color: #1e293b;">{{ $parameter?->year ?? '-' }}</td>
                                </tr>
                                <tr>
                                    <td style="padding: 0.4rem 0; color: #64748b;">Active Period</td>
                                    <td style="padding: 0.4rem 0; font-weight: 500; color: #1e293b;">{{ $periodName ?? '-' }}</td>
                                </tr>
                            </table>
                        </div>
                        <div class="col-md-6">
                            <table style="width: 100%; font-size: 0.875rem;">
                                <tr>
                                    <td style="padding: 0.4rem 0; color: #64748b; width: 140px;">Logged in as</td>
                                    <td style="padding: 0.4rem 0; font-weight: 500; color: #1e293b;">{{ $userName }}</td>
                                </tr>
                                <tr>
                                    <td style="padding: 0.4rem 0; color: #64748b;">Role</td>
                                    <td style="padding: 0.4rem 0; font-weight: 500; color: #1e293b;">{{ auth()->user()->getRoleNames()->first() ?? '-' }}</td>
                                </tr>
                                <tr>
                                    <td style="padding: 0.4rem 0; color: #64748b;">Date</td>
                                    <td style="padding: 0.4rem 0; font-weight: 500; color: #1e293b;">{{ now()->format('d M Y') }}</td>
                                </tr>
                            </table>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>

    {{-- Chart scripts --}}
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.1/dist/chart.umd.min.js"></script>
    <script>
        (function () {
            function getTrendPayload() {
                const payloadElement = document.getElementById('trendChartPayload');
                if (!payloadElement) {
                    return null;
                }

                try {
                    return JSON.parse(payloadElement.textContent.trim());
                } catch (error) {
                    console.error('Failed to parse trend payload', error);
                    return null;
                }
            }

            function renderTrendChart() {
                const payload = getTrendPayload();
                if (!payload) {
                    return;
                }

                const labels = Array.isArray(payload.labels) ? payload.labels : [];
                const txCounts = Array.isArray(payload.txCounts) ? payload.txCounts : [];
                const nominalNet = Array.isArray(payload.nominalNet) ? payload.nominalNet : [];
                const ytdAll = Number(payload.ytdAll || 0);
                const ytdNominal = Number(payload.ytdNominal || 0);
                const ytdAllLabel = payload.ytdAllLabel || 'YTD All';
                const ytdNominalLabel = payload.ytdNominalLabel || 'YTD Nominal';
                const canvas = document.getElementById('trendChart');

                if (!canvas) {
                    return;
                }

                if (window.__trendChartInstance) {
                    window.__trendChartInstance.destroy();
                    window.__trendChartInstance = null;
                }

                window.__trendChartInstance = new window.Chart(canvas, {
                    type: 'bar',
                    data: {
                        labels,
                        datasets: [
                            {
                                type: 'bar',
                                label: 'Jumlah Jurnal',
                                data: txCounts,
                                backgroundColor: 'rgba(79, 70, 229, 0.28)',
                                borderColor: 'rgba(79, 70, 229, 0.9)',
                                borderWidth: 1,
                                yAxisID: 'y',
                                order: 2
                            },
                            {
                                type: 'line',
                                label: 'Net Movement Nominal (Rp)',
                                data: nominalNet,
                                borderColor: '#0f766e',
                                backgroundColor: 'rgba(15, 118, 110, 0.18)',
                                yAxisID: 'y1',
                                tension: 0.28,
                                fill: false,
                                pointBackgroundColor: '#0f766e',
                                pointRadius: 3,
                                order: 1
                            },
                            {
                                type: 'line',
                                label: ytdAllLabel,
                                data: labels.map((_, idx) => (idx === labels.length - 1 ? ytdAll : null)),
                                borderColor: '#4f46e5',
                                backgroundColor: 'rgba(79, 70, 229, 0.18)',
                                yAxisID: 'y1',
                                tension: 0.2,
                                borderWidth: 2,
                                pointRadius: 0,
                                spanGaps: false,
                                showLine: true,
                                order: 0
                            },
                            {
                                type: 'line',
                                label: ytdNominalLabel,
                                data: labels.map((_, idx) => (idx === labels.length - 1 ? ytdNominal : null)),
                                borderColor: '#16a34a',
                                backgroundColor: 'rgba(22, 163, 74, 0.18)',
                                yAxisID: 'y1',
                                tension: 0.2,
                                borderWidth: 2,
                                pointRadius: 0,
                                spanGaps: false,
                                showLine: true,
                                order: 0
                            }
                        ]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        interaction: {
                            mode: 'index',
                            intersect: false
                        },
                        scales: {
                            y: {
                                beginAtZero: true,
                                ticks: {
                                    precision: 0
                                },
                                title: {
                                    display: true,
                                    text: 'Jumlah Jurnal'
                                }
                            },
                            y1: {
                                beginAtZero: true,
                                position: 'right',
                                grid: {
                                    drawOnChartArea: false
                                },
                                ticks: {
                                    callback: function (value) {
                                        return 'Rp ' + new Intl.NumberFormat('id-ID').format(value);
                                    }
                                },
                                title: {
                                    display: true,
                                    text: 'Net Movement'
                                }
                            }
                        },
                        plugins: {
                            legend: {
                                position: 'bottom',
                                labels: {
                                    boxWidth: 10
                                }
                            },
                            tooltip: {
                                callbacks: {
                                    label: function (ctx) {
                                        const label = ctx.dataset.label || '';
                                        const value = ctx.parsed.y;
                                        if (ctx.dataset.label && ctx.dataset.label.includes('Nominal')) {
                                            return `${label}: Rp ${new Intl.NumberFormat('id-ID').format(value)}`;
                                        }
                                        return `${label}: ${value}`;
                                    }
                                }
                            }
                        }
                    }
                });
            }

            const init = () => renderTrendChart();

            if (document.readyState === 'loading') {
                document.addEventListener('DOMContentLoaded', init);
            } else {
                init();
            }

            function attachLivewireHook() {
                if (window.Livewire && window.Livewire.hook) {
                    window.Livewire.hook('message.processed', function () {
                        init();
                    });
                }
            }

            if (window.Livewire && window.Livewire.hook) {
                attachLivewireHook();
            } else {
                document.addEventListener('livewire:load', attachLivewireHook);
                document.addEventListener('livewire:init', attachLivewireHook);
            }
        })();
    </script>

    {{-- Top Transaction Type --}}
    <div class="row mt-4">
        <div class="col-12">
            <div class="card" style="border: none; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.08);">
                <div class="card-body" style="padding: 1.25rem;">
                    <h5 style="font-weight: 600; color: #1e293b; margin: 0 0 .85rem 0; font-size: 1rem;">
                        Top Transaction Type ({{ $trendYear }})
                    </h5>
                    <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: .75rem;">
                        @forelse($topTransactionTypes as $item)
                            <div style="padding: .7rem; border-radius: 8px; background: #f8fafc; border: 1px solid #e2e8f0;">
                                <p style="margin: 0; font-size: .75rem; color:#64748b; text-transform: uppercase; letter-spacing:.04em;">{{ $item->trans_code }}</p>
                                <p style="margin: .35rem 0 0; font-size: 1.1rem; font-weight: 700; color:#0f172a;">{{ number_format($item->tx_count) }} jurnal</p>
                            </div>
                        @empty
                            <div style="grid-column: 1 / -1; padding: .7rem; border-radius: 8px; background: #f8fafc; border: 1px solid #e2e8f0; color:#64748b; font-size: .88rem;">
                                Belum ada data transaksi untuk tahun {{ $trendYear }}.
                            </div>
                        @endforelse
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
