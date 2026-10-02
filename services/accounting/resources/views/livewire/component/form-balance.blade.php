<div>
    <table class="table table-bordered">
        <tr class="text-center">
            <td>Periode</td>
            <td>
                Debet
            </td>
            <td>
                Credit
            </td>
        </tr>
        @if (isset($asu) && count($asu) > 0)
            @foreach ($asu as $index => $row)
                <tr>
                    <td>{{ $row['periode'] == 0 ? 'Beginning' : str_pad($row['periode'], 2, '0', STR_PAD_LEFT) }}</td>
                    <td>
                        <input type="text" class="form-control" wire:model="asu.{{ $index }}.debet"
                            step="0.01">
                    </td>
                    <td>
                        <input type="text" class="form-control" wire:model="asu.{{ $index }}.credit"
                            step="0.01">
                    </td>
                </tr>
            @endforeach
        @else
            @for ($i = 0; $i <= 13; $i++)
                <tr>
                    <td>{{ $i == 0 ? 'Beginning' : str_pad($i, 2, '0', STR_PAD_LEFT) }}</td>
                    <td>
                        <input type="text" class="form-control" wire:model="asu.{{ $i }}.debet"
                            value="{{ isset($asu[$i]['debet']) ? $asu[$i]['debet'] : '' }}" step="0.01">
                    </td>
                    <td>
                        <input type="text" class="form-control" wire:model="asu.{{ $i }}.credit"
                            value="{{ isset($asu[$i]['credit']) ? $asu[$i]['credit'] : '' }}" step="0.01">
                    </td>
                </tr>
            @endfor
        @endif

    </table>

    <br>
    <div class="position-relative">
        @canany(['createbalance', 'editbalance'])
            <!-- Tombol Save -->
            <div wire:loading.remove wire:target="simpan">
                <button class="btn btn-primary w-100" wire:click="simpan">Save</button>
            </div>

            <!-- Tombol Spinner -->
            <div wire:loading.delay wire:target="simpan" class="position-absolute top-0 start-0 w-100 text-center">
                <button class="btn bg-slate-600 w-100" disabled>
                    saving... <i class="icon-spinner2 spinner"></i>
                </button>
            </div>
        @endcanany
    </div>
</div>
