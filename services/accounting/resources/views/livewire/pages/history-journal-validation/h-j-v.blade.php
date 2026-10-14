<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="History Journal Validation"
            :breadcrumb="['Transaction', 'History Journal Validation']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">History Journal Validation</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>
        <div class="card-body">
            {{-- QA round 2: the old "Process" button here called a method that
                 never existed. Closing a period is End of Month, reopening is
                 Buka Kembali Periode; this page only lists the closed periods. --}}
            <p class="text-muted">Periode tahun buku berjalan yang sudah ditutup. Tutup periode melalui
                <a href="{{ url('endofmonth') }}">End of Month</a>; koreksi periode tertutup melalui
                <a href="{{ url('period-reopen') }}">Buka Kembali Periode</a>.</p>
            <table class="table table-sm table-bordered">
                <thead><tr><th style="width:120px">Periode</th><th>Keterangan</th></tr></thead>
                <tbody>
                    @php($n = 0)
                    @foreach ($data as $row)
                        @foreach ($row as $key => $value)
                            @php($n++)
                            <tr><td>{{ substr($key, 1) }}</td><td>{{ $value }}</td></tr>
                        @endforeach
                    @endforeach
                    @if ($n === 0)
                        <tr><td colspan="2" class="text-center text-muted">Belum ada periode yang ditutup pada tahun buku berjalan.</td></tr>
                    @endif
                </tbody>
            </table>
        </div>
    </div>
</div>