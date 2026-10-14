<div>
    @if ($message)
        <div class="alert alert-success">{{ $message }}</div>
    @endif
    @if ($error)
        <div class="alert alert-danger">{{ $error }}</div>
    @endif
    <table class="table table-bordered">
        <tr class="text-center">
            <td>Periode</td>
            <td>Debet</td>
            <td>Credit</td>
        </tr>
        @foreach ($asu as $index => $row)
            <tr>
                <td>{{ $row['periode'] == 0 ? 'Beginning' : str_pad($row['periode'], 2, '0', STR_PAD_LEFT) }}</td>
                @if ($row['periode'] == 0 && $editable)
                    @can('editbalance')
                        <td><input type="text" class="form-control" wire:model="asu.{{ $index }}.debet"></td>
                        <td><input type="text" class="form-control" wire:model="asu.{{ $index }}.credit"></td>
                    @else
                        <td class="text-right">{{ number_format($row['debet'], 2, ',', '.') }}</td>
                        <td class="text-right">{{ number_format($row['credit'], 2, ',', '.') }}</td>
                    @endcan
                @else
                    <td class="text-right">{{ number_format($row['debet'], 2, ',', '.') }}</td>
                    <td class="text-right">{{ number_format($row['credit'], 2, ',', '.') }}</td>
                @endif
            </tr>
        @endforeach
    </table>
    <div class="small text-muted mb-2">Saldo periode 1–13 dihitung dari jurnal yang diposting (End of Month) dan hanya dapat dilihat.
        Hanya saldo awal (Beginning) tahun buku aktif yang dapat diubah.</div>

    @can('editbalance')
        @if ($editable)
            <div class="position-relative">
                <div wire:loading.remove wire:target="simpan">
                    <button class="btn btn-primary w-100" wire:click="simpan">Simpan Saldo Awal</button>
                </div>
                <div wire:loading.delay wire:target="simpan" class="position-absolute top-0 start-0 w-100 text-center">
                    <button class="btn bg-slate-600 w-100" disabled>saving... <i class="icon-spinner2 spinner"></i></button>
                </div>
            </div>
        @endif
    @endcan
</div>
