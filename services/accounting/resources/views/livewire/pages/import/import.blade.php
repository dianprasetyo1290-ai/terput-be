<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Master" subjudul="Import Data" :breadcrumb="['Master', 'Import COA / Saldo Awal']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Import COA / Saldo Awal (CSV)</h5>
        </div>
        <div class="card-body">
            <div class="row">
                <div class="col-md-6">
                    <div class="form-group">
                        <label>Jenis data</label>
                        <select class="form-control" wire:model.live="kind">
                            <option value="coa">Chart of Account</option>
                            <option value="opening">Saldo awal tahun {{ $year }}</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>File CSV (maks 2 MB, pemisah koma atau titik koma)</label>
                        <input type="file" class="form-control" wire:model="file" accept=".csv,text/csv">
                        @error('file') <span class="text-danger small">{{ $message }}</span> @enderror
                    </div>
                    @if ($kind === 'opening' && $hasOpening)
                        <div class="form-check mb-3">
                            <input class="form-check-input" type="checkbox" id="confirmReplace" wire:model="confirmReplace">
                            <label class="form-check-label" for="confirmReplace">
                                Saldo awal tahun {{ $year }} sudah ada &mdash; ganti seluruhnya dengan isi file ini.
                            </label>
                        </div>
                    @endif
                    @can('createimport_data')
                        <button class="btn btn-light" wire:click="preview" wire:loading.attr="disabled">Pratinjau / Validasi</button>
                        <button class="btn btn-primary" wire:click="import" wire:loading.attr="disabled"
                            wire:confirm="Import file ini? Semua baris divalidasi dan disimpan sekaligus.">Import</button>
                    @endcan
                </div>
                <div class="col-md-6 small">
                    @if ($kind === 'coa')
                        <p class="mb-1"><strong>Kolom:</strong> <code>acc_code, acc_name, acc_group, parent_code, normal_balance, is_cash, is_interunit, is_control</code></p>
                        <ul class="pl-3 mb-2">
                            <li><code>acc_group</code>: 1 Aset, 2 Liabilitas, 3 Aset Neto/Ekuitas, 4 Pendapatan, 5 HPP, 6 Beban, 7 Pendapatan lain, 8 Beban lain.</li>
                            <li><code>normal_balance</code> D/K (kosong = otomatis dari kelompok). Flag: 1/0, ya/tidak.</li>
                            <li>Akun yang sudah ada diperbarui; kelompok dan saldo normal terkunci jika akun sudah dipakai.</li>
                            <li>Template COA per jenjang (TK/SD/SMP/SMA) dapat disiapkan sebagai CSV dan diimpor di sini.</li>
                        </ul>
<pre class="bg-light p-2">acc_code,acc_name,acc_group,parent_code,normal_balance,is_cash,is_interunit,is_control
1100,Kas dan Bank,1,,D,0,0,0
1101,Kas Sekolah,1,1100,D,1,0,0
1201,Piutang Siswa,1,,D,0,0,1</pre>
                    @else
                        <p class="mb-1"><strong>Kolom:</strong> <code>acc_code, div_code, debit, credit</code></p>
                        <ul class="pl-3 mb-2">
                            <li>File adalah neraca saldo awal lengkap tahun {{ $year }}: total debit harus sama dengan total kredit.</li>
                            <li>Hanya akun detail yang aktif; unit (<code>div_code</code>) harus terdaftar.</li>
                            <li>Saldo awal akun yang tidak ada di file menjadi nol.</li>
                            <li>Rincian piutang per siswa dikelola di modul Billing SEKOCI; di sini cukup total akun kontrol piutang.</li>
                        </ul>
<pre class="bg-light p-2">acc_code,div_code,debit,credit
1101,SD,25000000,0
1201,SD,12500000,0
3101,SD,0,37500000</pre>
                    @endif
                </div>
            </div>

            @if ($result)
                <hr>
                @if (!empty($result['errors']))
                    <div class="alert alert-danger">
                        <strong>{{ count($result['errors']) }} masalah &mdash; tidak ada data yang disimpan.</strong>
                        <ul class="mb-0 pl-3">
                            @foreach (array_slice($result['errors'], 0, 50) as $e)<li>{{ $e }}</li>@endforeach
                        </ul>
                        @if (count($result['errors']) > 50)<div>... dan {{ count($result['errors']) - 50 }} lainnya.</div>@endif
                    </div>
                @elseif ($result['dry'])
                    <div class="alert alert-info">
                        Validasi berhasil, belum disimpan.
                        @isset($result['created']) {{ $result['created'] }} akun baru, {{ $result['updated'] }} diperbarui. @endisset
                        @isset($result['debit']) {{ $result['rows'] }} baris; debit {{ number_format($result['debit'], 2, ',', '.') }} = kredit {{ number_format($result['credit'], 2, ',', '.') }}. @endisset
                    </div>
                @else
                    <div class="alert alert-success">
                        Import selesai.
                        @isset($result['created']) {{ $result['created'] }} akun baru, {{ $result['updated'] }} diperbarui. @endisset
                        @isset($result['debit']) {{ $result['rows'] }} saldo awal tahun {{ $result['year'] }} tersimpan (total {{ number_format($result['debit'], 2, ',', '.') }}). @endisset
                    </div>
                @endif
            @endif
        </div>
    </div>

    @php $money = fn ($v) => $v === null ? '-' : number_format((float) $v, 2, ',', '.'); @endphp
    <div class="card">
        <div class="card-header"><h5 class="card-title">Kontrol Total Piutang (Migrasi)</h5></div>
        <div class="card-body">
            <p class="small text-muted">Piutang per siswa disimpan di Billing; totalnya harus sama dengan saldo akun kontrol piutang di akuntansi.
                Bandingkan setelah impor saldo awal dan impor tagihan awal di Billing, lalu lakukan sign-off.</p>
            <button type="button" class="btn btn-outline-primary" wire:click="reconcile" wire:loading.attr="disabled">Cek ke Billing</button>
            @if ($recon)
                <table class="table table-sm mt-3">
                    <tbody>
                        @foreach ($recon['accounts'] as $acc)
                            <tr><td>Akun kontrol {{ $acc['acc_code'] }} {{ $acc['acc_name'] }}</td><td class="text-right">{{ $money($acc['ending']) }}</td></tr>
                        @endforeach
                        <tr><th>Saldo awal akun kontrol (periode 0, {{ $recon['year'] }})</th><td class="text-right">{{ $money($recon['opening']) }}</td></tr>
                        <tr><th>Saldo akun kontrol saat ini</th><td class="text-right">{{ $money($recon['ledger']) }}</td></tr>
                        <tr><th>Total piutang per siswa di Billing @if ($recon['students'] !== null)({{ $recon['students'] }} siswa)@endif</th><td class="text-right">{{ $money($recon['billing']) }}</td></tr>
                        @if ($recon['difference'] !== null)
                            <tr class="{{ abs($recon['difference']) < 0.01 ? 'table-success' : 'table-danger' }}"><th>Selisih</th><td class="text-right">{{ $money($recon['difference']) }}</td></tr>
                        @endif
                    </tbody>
                </table>
                @if ($recon['error'])
                    <div class="alert alert-secondary">{{ $recon['error'] }}</div>
                @endif
                @can('createimport_data')
                    @if ($recon['billing'] !== null)
                        <div class="form-group">
                            <label>Catatan sign-off @if (abs((float) $recon['difference']) >= 0.01)<span class="text-danger">(wajib: jelaskan selisih)</span>@endif</label>
                            <textarea class="form-control" rows="2" wire:model="signOffNote" maxlength="1000"></textarea>
                            @foreach ($errors->get('signOffNote') as $m)<div class="text-danger small">{{ $m }}</div>@endforeach
                        </div>
                        <button type="button" class="btn btn-success" wire:click="signOff" wire:confirm="Catat sign-off migrasi piutang?">Sign-off Migrasi Piutang</button>
                    @endif
                @endcan
            @endif
            @foreach ($errors->get('signOffNote') as $m)
                @if (! $recon || $recon['billing'] === null)<div class="text-danger small mt-2">{{ $m }}</div>@endif
            @endforeach
            @if ($signOffMessage)
                <div class="alert alert-success mt-3">{{ $signOffMessage }}</div>
            @endif
            @if ($lastSignOff)
                <div class="small text-muted mt-3">Sign-off terakhir: {{ $lastSignOff->user_id }} pada {{ $lastSignOff->created_at }} —
                    akuntansi {{ $money($lastSignOff->detail['ledger'] ?? null) }}, billing {{ $money($lastSignOff->detail['billing'] ?? null) }},
                    selisih {{ $money($lastSignOff->detail['difference'] ?? null) }}@if (!empty($lastSignOff->detail['note'])) ({{ $lastSignOff->detail['note'] }})@endif.</div>
            @endif
        </div>
    </div>
</div>
