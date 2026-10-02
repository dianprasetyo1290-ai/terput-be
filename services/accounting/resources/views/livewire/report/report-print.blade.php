<div>
    @push('css')
        <link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@300;400;500;600;700&display=swap" rel="stylesheet">
        <style>
            :root {
                --color-primary: #1e293b;
                --color-secondary: #475569;
                --color-muted: #94a3b8;
                --color-accent: #3b82f6;
                --color-accent-light: #eff6ff;
                --color-border: #e2e8f0;
                --color-border-dark: #cbd5e1;
                --color-bg: #f8fafc;
                --color-white: #ffffff;
                --color-header-bg: #1e293b;
                --color-header-text: #f1f5f9;
                --color-total-bg: #f1f5f9;
                --color-grand-total-bg: #1e293b;
                --color-grand-total-text: #ffffff;
                --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.05);
                --shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
                --shadow-lg: 0 10px 15px -3px rgba(0, 0, 0, 0.08), 0 4px 6px -4px rgba(0, 0, 0, 0.04);
                --radius: 12px;
            }

            * { box-sizing: border-box; }

            body {
                font-family: 'Plus Jakarta Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
                background-color: var(--color-bg);
                margin: 0;
                padding: 0;
                color: var(--color-secondary);
                font-feature-settings: 'tnum' 1;
                -webkit-font-smoothing: antialiased;
            }

            .container {
                max-width: 1400px;
                margin: 0 auto;
                padding: 32px 48px;
                background-color: var(--color-white);
                min-height: 100vh;
            }

            /* Report Header */
            .report-header {
                padding: 32px 0;
                margin-bottom: 32px;
                position: relative;
            }

            .report-header::after {
                content: '';
                position: absolute;
                bottom: 0;
                left: 0;
                right: 0;
                height: 3px;
                background: linear-gradient(90deg, var(--color-accent), var(--color-accent-light));
                border-radius: 2px;
            }

            .report-title {
                font-size: 1.75em;
                font-weight: 700;
                color: var(--color-primary);
                margin: 0 0 8px 0;
                letter-spacing: -0.025em;
                line-height: 1.2;
            }

            .report-subtitle {
                font-size: 1.1em;
                font-weight: 500;
                color: var(--color-secondary);
                margin: 0 0 16px 0;
            }

            .report-meta {
                display: inline-flex;
                align-items: center;
                gap: 12px;
                flex-wrap: wrap;
            }

            .report-meta-item {
                display: inline-flex;
                align-items: center;
                gap: 6px;
                font-size: 0.85em;
                color: var(--color-muted);
                background: var(--color-bg);
                padding: 6px 14px;
                border-radius: 20px;
                font-weight: 500;
            }

            .report-meta-item strong {
                color: var(--color-primary);
                font-weight: 600;
            }

            /* Table */
            .table-wrapper {
                border-radius: var(--radius);
                overflow: hidden;
                box-shadow: var(--shadow-lg);
                border: 1px solid var(--color-border);
            }

            table {
                width: 100%;
                border-collapse: collapse;
                margin: 0;
                background-color: var(--color-white);
            }

            thead {
                position: sticky;
                top: 0;
                z-index: 10;
            }

            thead th {
                background-color: var(--color-header-bg);
                padding: 14px 20px;
                text-align: left;
                font-weight: 600;
                color: var(--color-header-text);
                font-size: 0.8em;
                text-transform: uppercase;
                letter-spacing: 0.06em;
                border-bottom: none;
                white-space: nowrap;
            }

            thead th.rata-kanan {
                text-align: right;
            }

            tbody td {
                padding: 10px 20px;
                text-align: right;
                font-size: 0.9em;
                color: var(--color-secondary);
                border-bottom: 1px solid var(--color-border);
                font-variant-numeric: tabular-nums;
                transition: background-color 0.15s ease;
            }

            tbody td:first-child {
                text-align: left;
                color: var(--color-primary);
                font-weight: 400;
            }

            tbody tr:last-child td {
                border-bottom: none;
            }

            tbody tr:hover td {
                background-color: #f8fafc;
            }

            .rata-kanan {
                text-align: right;
            }

            /* Header Row (section title) */
            tbody tr.header-row td {
                background-color: var(--color-accent-light);
                font-weight: 600;
                color: var(--color-accent);
                font-size: 0.92em;
                padding: 12px 20px;
                border-bottom: 2px solid #bfdbfe;
                letter-spacing: 0.01em;
            }

            tbody tr.header-row:hover td {
                background-color: var(--color-accent-light) !important;
            }

            /* Total Row */
            tbody tr.total-row td {
                background-color: var(--color-total-bg);
                font-weight: 600;
                color: var(--color-primary);
                font-size: 0.9em;
                border-top: 2px solid var(--color-border-dark);
                border-bottom: 2px solid var(--color-border-dark);
                padding: 12px 20px;
            }

            tbody tr.total-row:hover td {
                background-color: var(--color-total-bg) !important;
            }

            /* Grand Total Row */
            tbody tr.grand-total-row td {
                background-color: var(--color-grand-total-bg);
                color: var(--color-grand-total-text);
                font-weight: 700;
                font-size: 0.95em;
                padding: 14px 20px;
                border-top: none;
                border-bottom: none;
            }

            tbody tr.grand-total-row:hover td {
                background-color: var(--color-grand-total-bg) !important;
            }

            /* Section Spacer */
            .section-spacer {
                height: 24px;
                background-color: var(--color-bg) !important;
                border-bottom: none !important;
            }

            .section-spacer td {
                background-color: var(--color-bg) !important;
                border-bottom: none !important;
                padding: 0 !important;
            }

            /* Error & No Data */
            .error-message {
                background-color: #fef2f2;
                color: #dc2626;
                padding: 16px 24px;
                border: 1px solid #fecaca;
                border-radius: var(--radius);
                margin: 0 0 24px 0;
                text-align: center;
                font-weight: 500;
                font-size: 0.9em;
            }

            .no-data-message {
                text-align: center;
                padding: 48px 24px;
                color: var(--color-muted);
                font-style: normal;
                font-size: 0.95em;
            }

            /* Action Toolbar */
            .report-actions {
                display: flex;
                align-items: center;
                gap: 10px;
                margin-bottom: 24px;
            }

            .btn-action {
                display: inline-flex;
                align-items: center;
                gap: 8px;
                padding: 10px 20px;
                font-size: 0.85em;
                font-weight: 600;
                font-family: inherit;
                border: none;
                border-radius: 8px;
                cursor: pointer;
                transition: all 0.2s ease;
                text-decoration: none;
                line-height: 1;
            }

            .btn-action svg {
                width: 16px;
                height: 16px;
                flex-shrink: 0;
            }

            .btn-action-print {
                background-color: var(--color-primary);
                color: var(--color-white);
            }

            .btn-action-print:hover {
                background-color: #334155;
                box-shadow: var(--shadow-md);
            }

            .btn-action-excel {
                background-color: #16a34a;
                color: var(--color-white);
            }

            .btn-action-excel:hover {
                background-color: #15803d;
                box-shadow: var(--shadow-md);
            }

            /* Print Styles */
            @media print {
                .report-actions { display: none !important; }
                :root {
                    --color-header-bg: #f1f5f9;
                    --color-header-text: #1e293b;
                    --color-grand-total-bg: #e2e8f0;
                    --color-grand-total-text: #1e293b;
                }

                body {
                    background-color: #fff;
                    padding: 10mm;
                    margin: 0;
                    font-size: 9pt;
                }

                .container {
                    max-width: none;
                    padding: 0;
                }

                .report-header { padding: 8px 0 12px 0; margin-bottom: 12px; }
                .report-header::after { height: 1px; background: #333; }
                .report-title { font-size: 1.4em; }
                .report-subtitle { font-size: 0.95em; }
                .report-meta-item { background: none; padding: 2px 0; font-size: 0.8em; }

                .table-wrapper {
                    box-shadow: none;
                    border: 1px solid #ccc;
                    border-radius: 0;
                }

                thead th {
                    padding: 8px 12px;
                    font-size: 0.75em;
                    -webkit-print-color-adjust: exact;
                    print-color-adjust: exact;
                }

                tbody td {
                    padding: 5px 12px;
                    font-size: 0.82em;
                }

                .header-row td { font-size: 0.85em; padding: 7px 12px; }
                .total-row td { font-size: 0.83em; padding: 7px 12px; }
                .grand-total-row td { font-size: 0.88em; padding: 8px 12px; }

                .section-spacer, .section-spacer td {
                    height: 8px !important;
                    background-color: #fff !important;
                }

                thead { display: table-header-group; }

                tbody tr {
                    break-inside: avoid;
                    page-break-inside: avoid;
                }

                .header-row {
                    page-break-after: avoid;
                    break-after: avoid;
                }

                .section-spacer {
                    page-break-before: avoid;
                    break-before: avoid;
                }

                .header-row td,
                .total-row td,
                .grand-total-row td,
                thead th {
                    -webkit-print-color-adjust: exact;
                    print-color-adjust: exact;
                }
            }
        </style>
    @endpush

    <div class="container">
        <div class="report-header">
            <h1 class="report-title">{{ $report->print_title }}</h1>
            @if ($report->sub_title)
                <h3 class="report-subtitle">{{ $report->sub_title }}</h3>
            @endif
            <div class="report-meta">
                <span class="report-meta-item">
                    Periode <strong>{{ $periodeLabel ?? $periode }}</strong>
                </span>
                <span class="report-meta-item">
                    Tahun <strong>{{ $tahun }}</strong>
                </span>
                <span class="report-meta-item">
                    Divisi <strong>{{ $divisi ?: 'Semua' }}</strong>
                </span>
            </div>
        </div>

        <div class="report-actions">
            <button type="button" class="btn-action btn-action-print" onclick="window.print()">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6.72 13.829c-.24.03-.48.062-.72.096m.72-.096a42.415 42.415 0 0 1 10.56 0m-10.56 0L6.34 18m10.94-4.171c.24.03.48.062.72.096m-.72-.096L17.66 18m0 0 .229 2.523a1.125 1.125 0 0 1-1.12 1.227H7.231c-.662 0-1.18-.568-1.12-1.227L6.34 18m11.318 0h1.091A2.25 2.25 0 0 0 21 15.75V9.456c0-1.081-.768-2.015-1.837-2.175a48.055 48.055 0 0 0-1.913-.247M6.34 18H5.25A2.25 2.25 0 0 1 3 15.75V9.456c0-1.081.768-2.015 1.837-2.175a48.041 48.041 0 0 1 1.913-.247m10.5 0a48.536 48.536 0 0 0-10.5 0m10.5 0V3.375c0-.621-.504-1.125-1.125-1.125h-8.25c-.621 0-1.125.504-1.125 1.125v3.659M18.25 7.034l-.001.006a47.84 47.84 0 0 0-3.999-.242" /></svg>
                Cetak
            </button>
            <button type="button" class="btn-action btn-action-excel" onclick="exportToExcel()">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M3 16.5v2.25A2.25 2.25 0 0 0 5.25 21h13.5A2.25 2.25 0 0 0 21 18.75V16.5M16.5 12 12 16.5m0 0L7.5 12m4.5 4.5V3" /></svg>
                Export Excel
            </button>
        </div>

        @if (session()->has('error'))
            <div class="error-message">
                {{ session('error') }}
            </div>
        @endif

        @if ($report)
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr>
                            <th>Deskripsi</th>
                            @foreach ($columnDefinitions as $col)
                                <th class="rata-kanan">{{ $col['col_title'] }}</th>
                            @endforeach
                        </tr>
                    </thead>
                    <tbody>
                        @forelse ($report->reportData as $val)
                            <livewire:report.report-row-print
                                :key="$val->id"
                                periode="{{ $periode }}"
                                tahun="{{ $tahun }}"
                                divisi="{{ $divisi }}"
                                :idHead="$val->report_row_head_id"
                                :columnDefinitions="$columnDefinitions"
                                :isBalanceSheetReport="$isBalanceSheetReport"
                            />
                            <tr class="section-spacer">
                                <td colspan="{{ count($columnDefinitions) + 1 }}"></td>
                            </tr>
                        @empty
                            <tr>
                                <td colspan="{{ count($columnDefinitions) + 1 }}" class="no-data-message">Tidak ada bagian laporan ditemukan.</td>
                            </tr>
                        @endforelse
                    </tbody>
                </table>
            </div>
        @else
            <div class="no-data-message">
                Data laporan tidak dapat dimuat. Mohon periksa ID yang diberikan.
            </div>
        @endif

        <div id="report-meta" style="display:none"
            data-title="{{ $report->print_title ?? 'Report' }}"
            data-subtitle="{{ $report->sub_title ?? '' }}"
            data-periode-label="{{ $periodeLabel ?? $periode }}"
            data-periode="{{ $periode }}"
            data-tahun="{{ $tahun }}"
            data-divisi="{{ $divisi ?: 'Semua' }}"
        ></div>
    </div>

    @script
    <script>
        window.exportToExcel = function() {
            var meta = document.getElementById('report-meta');
            if (!meta) return;

            var table = document.querySelector('.table-wrapper table');
            if (!table) return;

            var title = meta.dataset.title;
            var subtitle = meta.dataset.subtitle;
            var periodeLabel = meta.dataset.periodeLabel;
            var periode = meta.dataset.periode;
            var tahun = meta.dataset.tahun;
            var divisi = meta.dataset.divisi;

            var clone = table.cloneNode(true);

            clone.querySelectorAll('.section-spacer').forEach(function(row) {
                row.remove();
            });

            clone.querySelectorAll('td.rata-kanan').forEach(function(td) {
                var text = td.textContent.trim();
                if (text === '') return;

                var isPercent = text.endsWith('%');
                if (isPercent) text = text.replace('%', '').trim();

                var num = text.replace(/\./g, '').replace(',', '.');
                if (!isNaN(parseFloat(num)) && isFinite(num)) {
                    td.textContent = isPercent ? parseFloat(num).toFixed(2) + '%' : parseFloat(num).toFixed(2);
                    td.setAttribute('style', 'text-align:right;mso-number-format:\\#\\,\\#\\#0\\.00');
                }
            });

            var colCount = clone.rows[0].cells.length;
            var headerHtml = '<tr><td colspan="' + colCount + '" style="font-size:16pt;font-weight:bold">' + title + '</td></tr>';
            if (subtitle) {
                headerHtml += '<tr><td colspan="' + colCount + '" style="font-size:12pt">' + subtitle + '</td></tr>';
            }
            headerHtml += '<tr><td colspan="' + colCount + '">Periode: ' + periodeLabel + ' | Tahun: ' + tahun + ' | Divisi: ' + divisi + '</td></tr>';
            headerHtml += '<tr><td colspan="' + colCount + '"></td></tr>';

            var xlOpen = '<' + 'x:ExcelWorkbook><' + 'x:ExcelWorksheets><' + 'x:ExcelWorksheet>';
            var xlName = '<' + 'x:Name>Report</' + 'x:Name>';
            var xlOpts = '<' + 'x:WorksheetOptions><' + 'x:DisplayGridlines/></' + 'x:WorksheetOptions>';
            var xlClose = '</' + 'x:ExcelWorksheet></' + 'x:ExcelWorksheets></' + 'x:ExcelWorkbook>';

            var html = '<html xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:spreadsheet" xmlns="http://www.w3.org/TR/REC-html40">'
                + '<head><meta charset="utf-8">'
                + '<!--[if gte mso 9]><xml>' + xlOpen + xlName + xlOpts + xlClose + '</xml><![endif]-->'
                + '<style>'
                + 'table{border-collapse:collapse}'
                + 'th,td{border:1px solid #ccc;padding:6px 10px;font-size:10pt}'
                + 'th{background-color:#1e293b;color:#fff;font-weight:bold;text-align:center}'
                + '.header-row td{background-color:#eff6ff;font-weight:bold;color:#3b82f6}'
                + '.total-row td{background-color:#f1f5f9;font-weight:bold;border-top:2px solid #94a3b8;border-bottom:2px solid #94a3b8}'
                + '.grand-total-row td{background-color:#1e293b;color:#fff;font-weight:bold}'
                + '</style>'
                + '</head><body>'
                + '<table>' + headerHtml + clone.innerHTML + '</table>'
                + '</body></html>';

            var blob = new Blob([html], {type: 'application/vnd.ms-excel;charset=utf-8'});
            var url = URL.createObjectURL(blob);
            var a = document.createElement('a');
            a.href = url;
            a.download = title.replace(/[^a-zA-Z0-9\s]/g, '').replace(/\s+/g, '_') + '_' + tahun + '_P' + periode + '.xls';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
        }
    </script>
    @endscript
</div>
