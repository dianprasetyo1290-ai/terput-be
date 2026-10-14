{{-- Year / period / unit selector for ledger reports. $withDivision = false hides the unit. --}}
<div class="row mb-3">
    <div class="col-md-2">
        <label class="small text-muted mb-1">Tahun</label>
        <select class="form-control" wire:model.live="year">
            @foreach ($years as $y)
                <option value="{{ $y }}">{{ $y }}</option>
            @endforeach
        </select>
    </div>
    <div class="col-md-3">
        <label class="small text-muted mb-1">Periode</label>
        <select class="form-control" wire:model.live="period">
            @foreach ($periods as $p => $name)
                <option value="{{ $p }}">{{ $name }}</option>
            @endforeach
        </select>
    </div>
    @if ($withDivision ?? true)
        <div class="col-md-3">
            <label class="small text-muted mb-1">Unit</label>
            <select class="form-control" wire:model.live="division">
                <option value="">Semua unit</option>
                @foreach ($divisions as $d)
                    <option value="{{ $d->div_code }}">{{ $d->div_code }} - {{ $d->div_name }}</option>
                @endforeach
            </select>
        </div>
    @endif
    <div class="col d-flex align-items-end justify-content-end">
        <button type="button" class="btn btn-light" onclick="window.print()"><i class="icon-printer mr-1"></i> Cetak</button>
    </div>
</div>
